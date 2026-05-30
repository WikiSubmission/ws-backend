package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"
	"github.com/wikiSubmission/ws-backend/db"
)

// maxVariantsPerGroup caps the catalog size per (passage, language, difficulty,
// size). Once exceeded, the next startVariant call cycles back to the oldest
// variant with replay = true instead of minting a new one.
const maxVariantsPerGroup = 20

// ── GET /games/fill-blank/passages ─────────────────────────────────────────

func (s *Server) ListGameFillBlankPassages(ctx context.Context, request ListGameFillBlankPassagesRequestObject) (ListGameFillBlankPassagesResponseObject, error) {
	var theme *string
	if request.Params.Theme != nil && *request.Params.Theme != "" {
		t := *request.Params.Theme
		theme = &t
	}
	var chapter *int
	if request.Params.Chapter != nil {
		c := *request.Params.Chapter
		chapter = &c
	}

	rows, err := s.DB.FetchApprovedPassages(ctx, theme, chapter)
	if err != nil {
		slog.Error("ListGameFillBlankPassages: db error", "error", err)
		return ListGameFillBlankPassages500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}
	out := make([]GamePassage, len(rows))
	for i, r := range rows {
		out[i] = passageToAPI(&r)
	}
	return ListGameFillBlankPassages200JSONResponse{Data: out}, nil
}

func passageToAPI(p *db.GamePassageRow) GamePassage {
	themes := p.Themes
	if themes == nil {
		themes = []string{}
	}
	return GamePassage{
		Id:           p.ID,
		ChapterStart: p.ChapterStart,
		VerseStart:   p.VerseStart,
		ChapterEnd:   p.ChapterEnd,
		VerseEnd:     p.VerseEnd,
		Label:        p.Label,
		Themes:       themes,
	}
}

// ── POST /games/fill-blank/variants ────────────────────────────────────────

func (s *Server) StartGameFillBlankVariant(ctx context.Context, request StartGameFillBlankVariantRequestObject) (StartGameFillBlankVariantResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return StartGameFillBlankVariant500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}
	if err := s.ensureCurrentUser(ctx, authID); err != nil {
		slog.Error("StartGameFillBlankVariant: ensureCurrentUser failed", "error", err)
		return StartGameFillBlankVariant500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}
	if request.Body == nil {
		return StartGameFillBlankVariant400JSONResponse{
			BadRequestJSONResponse: BadRequestJSONResponse{Message: "request body is required"},
		}, nil
	}
	req := *request.Body
	difficulty := string(req.Difficulty)
	language := string(req.Language)
	size := string(req.Size)
	if language != "en" {
		return StartGameFillBlankVariant400JSONResponse{
			BadRequestJSONResponse: BadRequestJSONResponse{Message: "language must be 'en' in v1"},
		}, nil
	}

	// Resolve adaptive difficulty/size to concrete values before any further
	// logic. Both can use the same cross-tier accuracy query.
	if difficulty == "adaptive" || size == "adaptive" {
		overall, err := s.DB.FetchRecentAccuracyAll(ctx, authID, 10)
		if err != nil {
			slog.Warn("StartGameFillBlankVariant: FetchRecentAccuracyAll failed, using defaults", "error", err)
			overall = &db.RecentAccuracyResult{}
		}
		if difficulty == "adaptive" {
			band := computeSkillBand(overall.Count, overall.MeanAccuracy)
			difficulty = adaptiveDifficulty(band)
		}
		if size == "adaptive" {
			size = adaptiveSize(overall.Count)
		}
	}

	// Resume: caller named a specific variant — passage_id must also be provided.
	if req.VariantId != nil && *req.VariantId != "" {
		if req.PassageId == nil {
			return StartGameFillBlankVariant400JSONResponse{
				BadRequestJSONResponse: BadRequestJSONResponse{Message: "passage_id is required when variant_id is provided"},
			}, nil
		}
		passage, err := s.DB.FetchPassageByID(ctx, *req.PassageId)
		if err != nil {
			slog.Error("StartGameFillBlankVariant: FetchPassageByID (resume)", "error", err)
			return StartGameFillBlankVariant500JSONResponse{
				InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
			}, nil
		}
		if passage == nil || passage.Status != "approved" {
			return StartGameFillBlankVariant404JSONResponse{
				NotFoundJSONResponse: NotFoundJSONResponse{Message: "passage not found"},
			}, nil
		}
		v, err := s.DB.FetchVariantByVariantID(ctx, *req.VariantId)
		if err != nil {
			slog.Error("StartGameFillBlankVariant: FetchVariantByVariantID", "error", err)
			return StartGameFillBlankVariant500JSONResponse{
				InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
			}, nil
		}
		if v == nil || v.PassageID != passage.ID {
			return StartGameFillBlankVariant404JSONResponse{
				NotFoundJSONResponse: NotFoundJSONResponse{Message: "variant not found"},
			}, nil
		}
		api, err := variantToAPI(v, passage)
		if err != nil {
			slog.Error("StartGameFillBlankVariant: variantToAPI (resume)", "error", err)
			return StartGameFillBlankVariant500JSONResponse{
				InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
			}, nil
		}
		return StartGameFillBlankVariant200JSONResponse{Data: api}, nil
	}

	// Compute the player's skill band from their recent accuracy at this tier.
	recentAcc, err := s.DB.FetchRecentAccuracy(ctx, authID, difficulty, 10)
	if err != nil {
		slog.Warn("StartGameFillBlankVariant: FetchRecentAccuracy failed, defaulting to medium", "error", err)
		recentAcc = &db.RecentAccuracyResult{Count: 0}
	}
	skillBand := computeSkillBand(recentAcc.Count, recentAcc.MeanAccuracy)

	// Resolve the passage: use the caller's explicit choice if provided,
	// otherwise pick one that matches the player's skill band.
	var passage *db.GamePassageRow
	if req.PassageId != nil {
		passage, err = s.DB.FetchPassageByID(ctx, *req.PassageId)
		if err != nil {
			slog.Error("StartGameFillBlankVariant: FetchPassageByID", "error", err)
			return StartGameFillBlankVariant500JSONResponse{
				InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
			}, nil
		}
		if passage == nil || passage.Status != "approved" {
			return StartGameFillBlankVariant404JSONResponse{
				NotFoundJSONResponse: NotFoundJSONResponse{Message: "passage not found"},
			}, nil
		}
	} else {
		passage, err = s.DB.FetchPassageForBand(ctx, skillBand)
		if err != nil {
			slog.Error("StartGameFillBlankVariant: FetchPassageForBand", "error", err)
			return StartGameFillBlankVariant500JSONResponse{
				InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
			}, nil
		}
		if passage == nil {
			return StartGameFillBlankVariant404JSONResponse{
				NotFoundJSONResponse: NotFoundJSONResponse{Message: "no passages available"},
			}, nil
		}
	}

	// Auto-pick: lowest-counter variant the user has not yet attempted.
	v, err := s.DB.FetchNextUnseenVariant(ctx, authID, passage.ID, language, difficulty, size, skillBand)
	if err != nil {
		slog.Error("StartGameFillBlankVariant: FetchNextUnseenVariant", "error", err)
		return StartGameFillBlankVariant500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}
	if v != nil {
		api, err := variantToAPI(v, passage)
		if err != nil {
			slog.Error("StartGameFillBlankVariant: variantToAPI", "error", err)
			return StartGameFillBlankVariant500JSONResponse{
				InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
			}, nil
		}
		return StartGameFillBlankVariant200JSONResponse{Data: api}, nil
	}

	// No unseen variant exists. Mint a new one if we still have headroom;
	// otherwise cycle back to the oldest variant in the band group.
	maxCounter, err := s.DB.MaxVariantCounter(ctx, passage.ID, language, difficulty, size, skillBand)
	if err != nil {
		slog.Error("StartGameFillBlankVariant: MaxVariantCounter", "error", err)
		return StartGameFillBlankVariant500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}
	if maxCounter >= maxVariantsPerGroup {
		oldest, err := s.DB.OldestVariantInGroup(ctx, passage.ID, language, difficulty, size, skillBand)
		if err != nil {
			slog.Error("StartGameFillBlankVariant: OldestVariantInGroup", "error", err)
			return StartGameFillBlankVariant500JSONResponse{
				InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
			}, nil
		}
		if oldest == nil {
			return StartGameFillBlankVariant500JSONResponse{
				InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
			}, nil
		}
		api, err := variantToAPI(oldest, passage)
		if err != nil {
			slog.Error("StartGameFillBlankVariant: variantToAPI replay", "error", err)
			return StartGameFillBlankVariant500JSONResponse{
				InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
			}, nil
		}
		api.Replay = true
		return StartGameFillBlankVariant200JSONResponse{Data: api}, nil
	}

	minted, err := s.mintVariant(ctx, passage, language, difficulty, size, skillBand, maxCounter+1)
	if err != nil {
		slog.Error("StartGameFillBlankVariant: mintVariant", "error", err)
		return StartGameFillBlankVariant500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}
	api, err := variantToAPI(minted, passage)
	if err != nil {
		slog.Error("StartGameFillBlankVariant: variantToAPI new", "error", err)
		return StartGameFillBlankVariant500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}
	return StartGameFillBlankVariant200JSONResponse{Data: api}, nil
}

// mintVariant fetches passage verses, runs the generator, and persists the
// result.
func (s *Server) mintVariant(ctx context.Context, passage *db.GamePassageRow, language, difficulty, size, skillBand string, counter int) (*db.GameVariantRow, error) {
	verses, err := s.DB.FetchVersesWithTranslations(ctx,
		passage.ChapterStart, passage.ChapterEnd, passage.VerseStart, passage.VerseEnd,
		[]string{language})
	if err != nil {
		return nil, fmt.Errorf("fetch passage verses: %w", err)
	}
	if len(verses) == 0 {
		return nil, fmt.Errorf("passage %d returned no verses", passage.ID)
	}

	genVerses := make([]generatorVerse, 0, len(verses))
	uniqueWords := map[string]struct{}{}
	for _, v := range verses {
		key := fmt.Sprintf("%d:%d", v.ChapterNumber, v.VerseIndex)
		words := tokenizeVerse(v.VerseContent)
		genVerses = append(genVerses, generatorVerse{VerseKey: key, Words: words})
		for _, w := range words {
			if w.Normalized != "" {
				uniqueWords[w.Normalized] = struct{}{}
			}
		}
	}
	wordList := make([]string, 0, len(uniqueWords))
	for w := range uniqueWords {
		wordList = append(wordList, w)
	}

	freqs, err := s.DB.FetchFrequencies(ctx, language, wordList)
	if err != nil {
		return nil, fmt.Errorf("fetch frequencies: %w", err)
	}

	var pool []string
	if difficulty == "easy" {
		contents, err := s.DB.FetchChapterVerseContents(ctx, language, passage.ChapterStart)
		if err != nil {
			return nil, fmt.Errorf("fetch chapter contents: %w", err)
		}
		seen := map[string]struct{}{}
		for _, content := range contents {
			for _, w := range tokenizeVerse(content) {
				if w.Normalized == "" {
					continue
				}
				if _, dup := seen[w.Normalized]; dup {
					continue
				}
				seen[w.Normalized] = struct{}{}
				pool = append(pool, w.Normalized)
			}
		}
	}

	v, err := generateVariant(generatorInputs{
		PassageID:   passage.ID,
		Language:    language,
		Difficulty:  difficulty,
		Size:        size,
		SkillBand:   skillBand,
		Counter:     counter,
		Verses:      genVerses,
		Frequencies: freqs,
		ChapterPool: pool,
	})
	if err != nil {
		return nil, fmt.Errorf("generate variant: %w", err)
	}
	return s.DB.InsertVariant(ctx, v)
}

// variantToAPI converts a stored variant into the client-facing shape. Strips
// the `answer` / `answer_lemma` fields from every blank.
func variantToAPI(v *db.GameVariantRow, passage *db.GamePassageRow) (GameVariant, error) {
	var blanks []blankRecord
	if err := json.Unmarshal(v.Blanks, &blanks); err != nil {
		return GameVariant{}, fmt.Errorf("unmarshal blanks: %w", err)
	}
	var rendered []renderedVerse
	if err := json.Unmarshal(v.Rendered, &rendered); err != nil {
		return GameVariant{}, fmt.Errorf("unmarshal rendered: %w", err)
	}
	apiBlanks := make([]GameBlank, len(blanks))
	for i, b := range blanks {
		gb := GameBlank{
			Index:        b.Index,
			VerseKey:     b.VerseKey,
			WordPosition: b.WordPosition,
		}
		if len(b.Options) > 0 {
			opts := append([]string(nil), b.Options...)
			gb.Options = &opts
		}
		if v.Difficulty == "medium" {
			gb.Hint = blankHint(b.Answer)
		}
		apiBlanks[i] = gb
	}
	apiRendered := make([]GameRenderedVerse, len(rendered))
	for i, rv := range rendered {
		apiRendered[i] = GameRenderedVerse{VerseKey: rv.VerseKey, Text: rv.Text}
	}
	band := GameVariantSkillBand(v.SkillBand)
	return GameVariant{
		VariantId:      v.VariantID,
		Passage:        passageToAPI(passage),
		Language:       GameVariantLanguage(v.Language),
		Difficulty:     GameVariantDifficulty(v.Difficulty),
		Size:           GameVariantSize(v.Size),
		RenderedVerses: apiRendered,
		Blanks:         apiBlanks,
		Replay:         false,
		SkillBand:      &band,
	}, nil
}

// blankHint derives the medium-tier hint metadata from an answer. It exposes
// the first character, the first two characters, and the character count —
// never the full answer.
func blankHint(answer string) *GameBlankHint {
	runes := []rune(answer)
	firstLetter := ""
	if len(runes) >= 1 {
		firstLetter = string(runes[0])
	}
	firstTwo := firstLetter
	if len(runes) >= 2 {
		firstTwo = string(runes[:2])
	}
	return &GameBlankHint{
		FirstLetter: firstLetter,
		FirstTwo:    firstTwo,
		Length:      utf8.RuneCountInString(answer),
	}
}

// ── POST /games/fill-blank/submit ──────────────────────────────────────────

// difficultyMultipliers per the spec §8 score formula.
var difficultyMultipliers = map[string]float64{
	"easy":         1.0,
	"medium":       1.5,
	"hard":         2.0,
	"professional": 3.0,
}

// bandBonuses rewards players who receive harder blanks within their tier.
var bandBonuses = map[string]float64{
	"easy":   1.0,
	"medium": 1.0,
	"hard":   1.05,
}

// adaptiveDifficulty maps a skill band to a concrete difficulty tier.
func adaptiveDifficulty(band string) string {
	switch band {
	case "easy":
		return "easy"
	case "hard":
		return "hard"
	default:
		return "medium"
	}
}

// adaptiveSize maps a total-attempt count to a concrete round size. New
// players get short rounds; experienced players get longer ones.
func adaptiveSize(totalAttempts int) string {
	if totalAttempts < 5 {
		return "short"
	}
	if totalAttempts <= 20 {
		return "medium"
	}
	return "long"
}

// computeSkillBand derives the adaptive difficulty band from a user's recent
// accuracy. count is the number of same-tier attempts considered; accuracy is
// their mean correct fraction over those attempts.
func computeSkillBand(count int, accuracy float64) string {
	if count < 3 {
		return "medium" // cold start
	}
	if accuracy < 0.50 {
		return "easy"
	}
	if accuracy >= 0.85 {
		return "hard"
	}
	return "medium"
}

const hintPenaltyPerHint = 25

func (s *Server) SubmitGameFillBlankAttempt(ctx context.Context, request SubmitGameFillBlankAttemptRequestObject) (SubmitGameFillBlankAttemptResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return SubmitGameFillBlankAttempt500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}
	if err := s.ensureCurrentUser(ctx, authID); err != nil {
		slog.Error("SubmitGameFillBlankAttempt: ensureCurrentUser failed", "error", err)
		return SubmitGameFillBlankAttempt500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}
	if request.Body == nil {
		return SubmitGameFillBlankAttempt400JSONResponse{
			BadRequestJSONResponse: BadRequestJSONResponse{Message: "request body is required"},
		}, nil
	}
	req := *request.Body
	if req.VariantId == "" {
		return SubmitGameFillBlankAttempt400JSONResponse{
			BadRequestJSONResponse: BadRequestJSONResponse{Message: "variant_id is required"},
		}, nil
	}
	if req.HintsUsed < 0 {
		return SubmitGameFillBlankAttempt400JSONResponse{
			BadRequestJSONResponse: BadRequestJSONResponse{Message: "hints_used must be non-negative"},
		}, nil
	}

	limits := s.gamesLimitsOrInit()
	if !limits.perUserMinute.Allow(authID) {
		return SubmitGameFillBlankAttempt429JSONResponse{
			Message: "rate limit exceeded — max 10 submissions per minute per user",
		}, nil
	}
	if !limits.perUserVariant.Allow(authID + "|" + req.VariantId) {
		return SubmitGameFillBlankAttempt429JSONResponse{
			Message: "rate limit exceeded — max 5 submissions per hour per variant",
		}, nil
	}

	variant, err := s.DB.FetchVariantByVariantID(ctx, req.VariantId)
	if err != nil {
		slog.Error("SubmitGameFillBlankAttempt: FetchVariantByVariantID", "error", err)
		return SubmitGameFillBlankAttempt500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}
	if variant == nil {
		return SubmitGameFillBlankAttempt404JSONResponse{
			NotFoundJSONResponse: NotFoundJSONResponse{Message: "variant not found"},
		}, nil
	}

	var blanks []blankRecord
	if err := json.Unmarshal(variant.Blanks, &blanks); err != nil {
		slog.Error("SubmitGameFillBlankAttempt: unmarshal blanks", "error", err)
		return SubmitGameFillBlankAttempt500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}

	// For Hard tier acceptance, load lemmas for every guess and every answer.
	var lemmaMap map[string]string
	if variant.Difficulty == "hard" {
		words := make([]string, 0, len(blanks)*2)
		for _, b := range blanks {
			words = append(words, normalizeAnswer(b.Answer))
		}
		for _, g := range req.Guesses {
			words = append(words, normalizeAnswer(g.Value))
		}
		m, err := s.DB.FetchLemmasFor(ctx, variant.Language, words)
		if err != nil {
			slog.Error("SubmitGameFillBlankAttempt: FetchLemmasFor", "error", err)
		} else {
			lemmaMap = m
		}
	}

	guessByIndex := make(map[int]string, len(req.Guesses))
	for _, g := range req.Guesses {
		guessByIndex[g.Index] = g.Value
	}

	per := make([]GamePerBlankResult, len(blanks))
	correctCount := 0
	for i, b := range blanks {
		guess := guessByIndex[b.Index]
		ok := scoreGuess(guess, &b, variant.Difficulty, lemmaMap)
		if ok {
			correctCount++
		}
		answer := b.Answer
		per[i] = GamePerBlankResult{Index: b.Index, Correct: ok, AcceptedAnswer: &answer}
	}

	base := float64(correctCount) / float64(len(blanks)) * 1000.0
	mult := difficultyMultipliers[variant.Difficulty]
	bandBonus, ok := bandBonuses[variant.SkillBand]
	if !ok {
		bandBonus = 1.0
	}
	penalty := req.HintsUsed * hintPenaltyPerHint
	score := int(base*mult*bandBonus) - penalty
	if score < 0 {
		score = 0
	}

	perJSON, err := json.Marshal(per)
	if err != nil {
		slog.Error("SubmitGameFillBlankAttempt: marshal per_blank", "error", err)
		return SubmitGameFillBlankAttempt500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}

	var elapsedMs *int
	if req.ElapsedMs != nil && *req.ElapsedMs >= 0 {
		v := *req.ElapsedMs
		elapsedMs = &v
	}

	attemptID := newAttemptID()
	row := &db.GameAttemptRow{
		AttemptID:    attemptID,
		VariantID:    variant.VariantID,
		Size:         variant.Size,
		Difficulty:   variant.Difficulty,
		CorrectCount: correctCount,
		TotalCount:   len(blanks),
		HintsUsed:    req.HintsUsed,
		Score:        score,
		ElapsedMs:    elapsedMs,
		PerBlank:     perJSON,
	}
	saved, err := s.DB.InsertAttempt(ctx, authID, row)
	if err != nil {
		slog.Error("SubmitGameFillBlankAttempt: InsertAttempt", "error", err)
		return SubmitGameFillBlankAttempt500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}

	return SubmitGameFillBlankAttempt200JSONResponse{
		Data: GameSubmitResult{
			AttemptId:            saved.AttemptID,
			Score:                saved.Score,
			CorrectCount:         saved.CorrectCount,
			TotalCount:           saved.TotalCount,
			PerBlank:             per,
			DifficultyMultiplier: mult,
			HintPenalty:          penalty,
		},
	}, nil
}

// scoreGuess applies the per-difficulty acceptance rule. Returns true if the
// guess should be accepted.
func scoreGuess(guess string, b *blankRecord, difficulty string, lemmaMap map[string]string) bool {
	g := normalizeAnswer(guess)
	a := normalizeAnswer(b.Answer)
	if g == "" {
		return false
	}
	if g == a {
		return true
	}
	if difficulty == "hard" {
		// Lemma-based acceptance when available.
		gl, gok := lemmaMap[g]
		al, aok := lemmaMap[a]
		if gok && aok && gl == al {
			return true
		}
		// Fall back to recorded answer_lemma if the DB has not been seeded.
		if b.AnswerLemma != "" && gl == b.AnswerLemma {
			return true
		}
	}
	return false
}

// normalizeAnswer lowercases and strips non-letter/digit runes for matching.
// Apostrophes are preserved so "God's" survives as "gods" only via fallback —
// we strip them here to avoid that wart.
func normalizeAnswer(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func newAttemptID() string {
	id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if err != nil {
		// Extremely unlikely; degrade to a base32 random string.
		var buf [12]byte
		_, _ = rand.Read(buf[:])
		return strings.ToUpper(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf[:]))
	}
	return id.String()
}

// ── GET /games/fill-blank/leaderboard ──────────────────────────────────────

func (s *Server) GetGameFillBlankLeaderboard(ctx context.Context, request GetGameFillBlankLeaderboardRequestObject) (GetGameFillBlankLeaderboardResponseObject, error) {
	params := request.Params
	limit := 25
	if params.Limit != nil {
		limit = *params.Limit
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}

	var rows []db.GameLeaderboardRow
	var err error
	switch params.Scope {
	case "global", "weekly":
		var size, difficulty string
		if params.Size != nil {
			size = string(*params.Size)
		}
		if params.Difficulty != nil {
			difficulty = string(*params.Difficulty)
		}
		if params.Scope == "global" {
			rows, err = s.DB.FetchGlobalLeaderboard(ctx, size, difficulty, limit)
		} else {
			rows, err = s.DB.FetchWeeklyLeaderboard(ctx, size, difficulty, limit)
		}
	case "variant":
		if params.VariantId == nil || *params.VariantId == "" {
			return GetGameFillBlankLeaderboard400JSONResponse{
				BadRequestJSONResponse: BadRequestJSONResponse{Message: "variant_id is required for variant leaderboard"},
			}, nil
		}
		rows, err = s.DB.FetchVariantLeaderboard(ctx, *params.VariantId, limit)
	default:
		return GetGameFillBlankLeaderboard400JSONResponse{
			BadRequestJSONResponse: BadRequestJSONResponse{Message: "scope must be one of global, weekly, variant"},
		}, nil
	}
	if err != nil {
		slog.Error("GetGameFillBlankLeaderboard: db error", "error", err)
		return GetGameFillBlankLeaderboard500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}

	out := make([]GameLeaderboardEntry, len(rows))
	for i, r := range rows {
		variantID := r.VariantID
		entry := GameLeaderboardEntry{
			Rank:        i + 1,
			UserId:      int64(r.UserID),
			DisplayName: r.DisplayName,
			Score:       r.Score,
			Size:        r.Size,
			Difficulty:  r.Difficulty,
			ElapsedMs:   r.ElapsedMs,
			VariantId:   &variantID,
			AttemptedAt: r.CompletedAt.UTC(),
		}
		out[i] = entry
	}
	return GetGameFillBlankLeaderboard200JSONResponse{Data: out}, nil
}

// ── GET /me/games/history ──────────────────────────────────────────────────

func (s *Server) ListMeGameHistory(ctx context.Context, request ListMeGameHistoryRequestObject) (ListMeGameHistoryResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return ListMeGameHistory500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}
	limit := 25
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	var cursor *time.Time
	if request.Params.Cursor != nil && *request.Params.Cursor != "" {
		t, err := decodeHistoryCursor(*request.Params.Cursor)
		if err != nil {
			return ListMeGameHistory400JSONResponse{
				BadRequestJSONResponse: BadRequestJSONResponse{Message: "invalid cursor"},
			}, nil
		}
		cursor = &t
	}

	rows, err := s.DB.FetchUserAttempts(ctx, authID, limit, cursor)
	if err != nil {
		slog.Error("ListMeGameHistory: db error", "error", err)
		return ListMeGameHistory500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}
	out := make([]GameHistoryEntry, len(rows))
	for i, r := range rows {
		out[i] = GameHistoryEntry{
			AttemptId:   r.AttemptID,
			VariantId:   r.VariantID,
			Score:       r.Score,
			Size:        r.Size,
			Difficulty:  r.Difficulty,
			CompletedAt: r.CompletedAt.UTC(),
		}
	}
	var next *string
	if len(rows) == limit {
		c := encodeHistoryCursor(rows[len(rows)-1].CompletedAt)
		next = &c
	}
	return ListMeGameHistory200JSONResponse{Data: out, NextCursor: next}, nil
}

// History cursor is the RFC3339Nano timestamp of the last item on the previous
// page. Opaque to the client.
func encodeHistoryCursor(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func decodeHistoryCursor(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, errors.New("invalid timestamp")
	}
	return t, nil
}

// ── GET /me/games/stats ────────────────────────────────────────────────────

func (s *Server) GetMeGameStats(ctx context.Context, _ GetMeGameStatsRequestObject) (GetMeGameStatsResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return GetMeGameStats500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}
	stats, err := s.DB.FetchUserGameStats(ctx, authID)
	if err != nil {
		slog.Error("GetMeGameStats: db error", "error", err)
		return GetMeGameStats500JSONResponse{
			InternalServerErrrorJSONResponse: InternalServerErrrorJSONResponse{Message: "internal server error"},
		}, nil
	}

	body := GameStats{
		TotalAttempts:     stats.TotalAttempts,
		BestScore:         stats.BestScore,
		VariantsCompleted: stats.VariantsCompleted,
	}
	for _, b := range stats.ByDifficulty {
		entry := GameStatsByDifficulty{Attempts: b.Attempts, BestScore: b.BestScore}
		switch b.Difficulty {
		case "easy":
			body.ByDifficulty.Easy = &entry
		case "medium":
			body.ByDifficulty.Medium = &entry
		case "hard":
			body.ByDifficulty.Hard = &entry
		case "professional":
			body.ByDifficulty.Professional = &entry
		}
	}
	return GetMeGameStats200JSONResponse{Data: body}, nil
}
