package handlers

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand"
	"sort"
	"strings"
	"unicode"

	"github.com/wikiSubmission/ws-backend/db"
)

// ── Public types stored inside game_variants.blanks / rendered ──────────────

// blankRecord is the JSON shape persisted in game_variants.blanks. The
// `Answer` and `AnswerLemma` fields are server-only; they are stripped before
// returning a variant to the client.
type blankRecord struct {
	Index        int      `json:"index"`
	VerseKey     string   `json:"verse_key"`
	WordPosition int      `json:"word_position"`
	Answer       string   `json:"answer"`
	AnswerLemma  string   `json:"answer_lemma,omitempty"`
	Options      []string `json:"options,omitempty"`
}

type renderedVerse struct {
	VerseKey string `json:"verse_key"`
	Text     string `json:"text"`
}

// ── Inputs ──────────────────────────────────────────────────────────────────

// generatorVerse is one verse worth of input to the generator: its key and a
// pre-tokenized list of words. The generator does not call the DB; the caller
// (the handler) fetches verses and tokenizes them.
type generatorVerse struct {
	VerseKey string
	Words    []verseWord
}

type verseWord struct {
	// Position is the 0-based word index in the verse.
	Position int
	// Surface is the original word with punctuation stripped, capitalization
	// preserved. Goes back into rendered output for un-blanked words.
	Surface string
	// Normalized is the lowercased letters/digits only form used for matching
	// and frequency lookups.
	Normalized string
	// LeadingPunct / TrailingPunct preserve original spacing around the word
	// (commas, periods, quotes) so the rendered verse reads correctly.
	LeadingPunct  string
	TrailingPunct string
}

// generatorInputs carries everything the pure generator needs.
type generatorInputs struct {
	PassageID  int64
	Language   string
	Difficulty string
	Size       string
	// SkillBand is the adaptive difficulty band: "easy", "medium", or "hard".
	// It is part of the variant group key so band-specific variants are cached
	// and shared across all players at the same band level — never per-user.
	SkillBand string
	Counter   int
	Verses    []generatorVerse
	// Frequencies maps normalized words to corpus counts. Missing entries are
	// treated as freq=1 (very rare).
	Frequencies map[string]int
	// ChapterPool is the de-duplicated, normalized vocabulary of the passage's
	// chapter, used to pick easy-tier distractors. Only required for difficulty
	// = "easy"; may be nil otherwise.
	ChapterPool []string
}

// ── Seeded RNG ──────────────────────────────────────────────────────────────

// seedFor produces a deterministic seed from the variant key. Stable across
// processes and architectures.
func seedFor(passageID int64, lang, difficulty, size, skillBand string, counter int) int64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "p%d|%s|%s|%s|%s|%d", passageID, lang, difficulty, size, skillBand, counter)
	return int64(h.Sum64()) //nolint:gosec // intentional truncation, only used as a PRNG seed
}

// ── Size / difficulty knobs ─────────────────────────────────────────────────

type sizeTargets struct{ minVerses, maxVerses, minBlanks, maxBlanks int }

var sizeTable = map[string]sizeTargets{
	"short":  {minVerses: 3, maxVerses: 5, minBlanks: 3, maxBlanks: 5},
	"medium": {minVerses: 6, maxVerses: 12, minBlanks: 6, maxBlanks: 10},
	"long":   {minVerses: 15, maxVerses: 25, minBlanks: 12, maxBlanks: 18},
}

// stopwords is the fixed small set used to skip blanks for easy/medium tiers.
var stopwords = map[string]struct{}{
	"the": {}, "of": {}, "and": {}, "to": {}, "is": {}, "a": {}, "in": {},
	"for": {}, "that": {}, "it": {}, "with": {}, "as": {}, "be": {}, "are": {},
	"was": {}, "were": {}, "by": {}, "on": {}, "from": {}, "this": {}, "or": {},
	"but": {}, "not": {}, "an": {}, "at": {}, "we": {}, "you": {}, "he": {},
	"she": {}, "they": {}, "his": {}, "her": {}, "their": {}, "them": {},
	"who": {}, "whom": {}, "which": {}, "what": {}, "shall": {}, "will": {},
	"have": {}, "has": {}, "had": {}, "do": {}, "does": {}, "did": {},
	"so": {}, "if": {}, "then": {}, "than": {}, "thus": {},
}

// ── Tokenization (used by the handler before calling the generator) ────────

// tokenizeVerse splits a verse string into ordered words plus surrounding
// punctuation so the rendered output can be reconstructed faithfully.
func tokenizeVerse(text string) []verseWord {
	out := []verseWord{}
	runes := []rune(text)
	i := 0
	position := 0
	for i < len(runes) {
		// Collect leading non-word runes.
		startLead := i
		for i < len(runes) && !isWordRune(runes[i]) {
			i++
		}
		lead := string(runes[startLead:i])
		if i >= len(runes) {
			// Trailing punctuation at end of text — append to the previous word.
			if len(out) > 0 {
				out[len(out)-1].TrailingPunct += lead
			}
			break
		}
		// Collect the word.
		startWord := i
		for i < len(runes) && isWordRune(runes[i]) {
			i++
		}
		surface := string(runes[startWord:i])
		out = append(out, verseWord{
			Position:     position,
			Surface:      surface,
			Normalized:   strings.ToLower(stripNonWord(surface)),
			LeadingPunct: lead,
		})
		position++
	}
	return out
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\''
}

func stripNonWord(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ── Core generator ──────────────────────────────────────────────────────────

func generateVariant(in generatorInputs) (*db.GameVariantRow, error) {
	targets, ok := sizeTable[in.Size]
	if !ok {
		return nil, fmt.Errorf("unknown size %q", in.Size)
	}
	if _, ok := map[string]struct{}{"easy": {}, "medium": {}, "hard": {}, "professional": {}}[in.Difficulty]; !ok {
		return nil, fmt.Errorf("unknown difficulty %q", in.Difficulty)
	}
	if in.Language != "en" {
		return nil, fmt.Errorf("unsupported language %q", in.Language)
	}
	if len(in.Verses) == 0 {
		return nil, fmt.Errorf("passage has no verses")
	}

	seed := seedFor(in.PassageID, in.Language, in.Difficulty, in.Size, in.SkillBand, in.Counter)
	rng := rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic by design

	// Pick a contiguous verse window matching the requested size.
	window := pickVerseWindow(in.Verses, targets.minVerses, targets.maxVerses, rng)

	// Pick the blank count target inside the band.
	targetBlanks := targets.minBlanks
	if targets.maxBlanks > targets.minBlanks {
		targetBlanks += rng.Intn(targets.maxBlanks - targets.minBlanks + 1)
	}

	// Score candidate words across the window.
	cands := scoreCandidates(window, in.Difficulty, in.SkillBand, in.Frequencies)
	blanks := selectBlanks(cands, targetBlanks)

	// Stable order: by appearance (verse order, then word position).
	sort.SliceStable(blanks, func(i, j int) bool {
		if blanks[i].verseIdx != blanks[j].verseIdx {
			return blanks[i].verseIdx < blanks[j].verseIdx
		}
		return blanks[i].word.Position < blanks[j].word.Position
	})

	// Build blank records.
	records := make([]blankRecord, 0, len(blanks))
	for i, b := range blanks {
		rec := blankRecord{
			Index:        i,
			VerseKey:     window[b.verseIdx].VerseKey,
			WordPosition: b.word.Position,
			Answer:       b.word.Surface,
			AnswerLemma:  b.word.Normalized,
		}
		if in.Difficulty == "easy" {
			rec.Options = pickDistractors(b.word, in.ChapterPool, in.Frequencies, rng)
		}
		records = append(records, rec)
	}

	// Build rendered verses with __BLANK_N__ tokens.
	blankByVerseAndPos := make(map[string]int, len(records))
	for _, r := range records {
		key := fmt.Sprintf("%s:%d", r.VerseKey, r.WordPosition)
		blankByVerseAndPos[key] = r.Index
	}
	rendered := make([]renderedVerse, 0, len(window))
	for _, v := range window {
		var b strings.Builder
		for _, w := range v.Words {
			b.WriteString(w.LeadingPunct)
			key := fmt.Sprintf("%s:%d", v.VerseKey, w.Position)
			if idx, hit := blankByVerseAndPos[key]; hit {
				fmt.Fprintf(&b, "__BLANK_%d__", idx)
			} else {
				b.WriteString(w.Surface)
			}
			b.WriteString(w.TrailingPunct)
		}
		rendered = append(rendered, renderedVerse{VerseKey: v.VerseKey, Text: b.String()})
	}

	blanksJSON, err := json.Marshal(records)
	if err != nil {
		return nil, fmt.Errorf("marshal blanks: %w", err)
	}
	renderedJSON, err := json.Marshal(rendered)
	if err != nil {
		return nil, fmt.Errorf("marshal rendered: %w", err)
	}

	variantID := fmt.Sprintf("p%d-%s-%s-%s-%s-%d", in.PassageID, in.Language, in.Difficulty, in.Size, in.SkillBand, in.Counter)
	return &db.GameVariantRow{
		VariantID:  variantID,
		PassageID:  in.PassageID,
		Language:   in.Language,
		Difficulty: in.Difficulty,
		Size:       in.Size,
		SkillBand:  in.SkillBand,
		Counter:    in.Counter,
		Seed:       seed,
		Blanks:     blanksJSON,
		Rendered:   renderedJSON,
	}, nil
}

// pickVerseWindow picks a contiguous sub-window of the passage matching the
// target verse count. The PRNG selects the start offset deterministically.
func pickVerseWindow(verses []generatorVerse, minV, maxV int, rng *rand.Rand) []generatorVerse {
	target := minV
	if maxV > minV {
		target += rng.Intn(maxV - minV + 1)
	}
	if target > len(verses) {
		target = len(verses)
	}
	if target < 1 {
		target = 1
	}
	maxStart := len(verses) - target
	start := 0
	if maxStart > 0 {
		start = rng.Intn(maxStart + 1)
	}
	return verses[start : start+target]
}

// ── Candidate scoring ──────────────────────────────────────────────────────

type candidate struct {
	verseIdx int
	word     verseWord
	score    float64
}

func scoreCandidates(window []generatorVerse, difficulty, skillBand string, freqs map[string]int) []candidate {
	skipStopwords := difficulty == "easy" || difficulty == "medium"
	minLen := 0
	switch difficulty {
	case "easy":
		minLen = 3
	case "medium":
		minLen = 4
	}
	midBonus := 10.0
	freqWeight := 1.0
	if difficulty == "easy" || difficulty == "medium" {
		midBonus = 50.0
		freqWeight = 2.0
	}

	// Layer skill band adjustments on top of the tier knobs. These shift the
	// picker toward easier or harder words within the player's chosen tier.
	switch skillBand {
	case "easy":
		midBonus *= 1.5  // stronger center-of-verse preference
		freqWeight *= 0.5 // de-emphasize word rarity
	case "hard":
		midBonus *= 0.3  // allow edge-of-verse words
		freqWeight *= 2.5 // heavily reward rarer words
		if minLen < 4 {
			minLen = 4 // enforce a minimum word length
		}
	}

	cands := make([]candidate, 0, len(window)*8)
	for vi, v := range window {
		for _, w := range v.Words {
			if w.Normalized == "" {
				continue
			}
			if len(w.Normalized) < minLen {
				continue
			}
			if skipStopwords {
				if _, hit := stopwords[w.Normalized]; hit {
					continue
				}
			}
			score := 100.0
			// Mid-verse position bonus.
			n := len(v.Words)
			if n > 1 {
				center := float64(n-1) / 2.0
				dist := absFloat(float64(w.Position) - center)
				closeness := 1.0 - (dist / center)
				if closeness < 0 {
					closeness = 0
				}
				score += midBonus * closeness
			}
			// Frequency bias: rarer = higher.
			f, ok := freqs[w.Normalized]
			if !ok || f < 1 {
				f = 1
			}
			score += freqWeight * (10.0 / float64(f))
			cands = append(cands, candidate{verseIdx: vi, word: w, score: score})
		}
	}
	return cands
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// selectBlanks picks the highest-scoring candidates while enforcing "no
// consecutive blanks in the same verse". Order of return is descending by
// score; the caller re-orders by appearance.
func selectBlanks(cands []candidate, target int) []candidate {
	// Sort by score desc, then stable secondary by (verseIdx, position) for
	// determinism across builds.
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		if cands[i].verseIdx != cands[j].verseIdx {
			return cands[i].verseIdx < cands[j].verseIdx
		}
		return cands[i].word.Position < cands[j].word.Position
	})

	type cellKey struct {
		verseIdx int
		position int
	}
	taken := map[cellKey]struct{}{}
	out := make([]candidate, 0, target)
	for _, c := range cands {
		if len(out) >= target {
			break
		}
		// Reject if a neighbour position is already a blank.
		if _, hit := taken[cellKey{c.verseIdx, c.word.Position - 1}]; hit {
			continue
		}
		if _, hit := taken[cellKey{c.verseIdx, c.word.Position + 1}]; hit {
			continue
		}
		taken[cellKey{c.verseIdx, c.word.Position}] = struct{}{}
		out = append(out, c)
	}
	return out
}

// ── Easy-tier distractor selection ──────────────────────────────────────────

func pickDistractors(answer verseWord, chapterPool []string, freqs map[string]int, rng *rand.Rand) []string {
	// Filter to candidates within the same frequency band as the answer.
	answerFreq := freqs[answer.Normalized]
	if answerFreq < 1 {
		answerFreq = 1
	}
	bandLow, bandHigh := bandFor(answerFreq, 0.20)

	pickFrom := make([]string, 0, len(chapterPool))
	add := func(threshold func(int) bool) {
		seen := map[string]struct{}{answer.Normalized: {}}
		for _, w := range chapterPool {
			if _, dup := seen[w]; dup {
				continue
			}
			if _, hit := stopwords[w]; hit {
				continue
			}
			f, ok := freqs[w]
			if !ok || f < 1 {
				f = 1
			}
			if threshold(f) {
				pickFrom = append(pickFrom, w)
				seen[w] = struct{}{}
			}
		}
	}
	add(func(f int) bool { return f >= bandLow && f <= bandHigh })

	// Widen the band by 10% increments up to 100% before falling back to "any
	// non-stopword from the chapter".
	for step := 0.30; len(pickFrom) < 3 && step <= 1.0; step += 0.10 {
		bandLow, bandHigh = bandFor(answerFreq, step)
		pickFrom = pickFrom[:0]
		add(func(f int) bool { return f >= bandLow && f <= bandHigh })
	}
	if len(pickFrom) < 3 {
		pickFrom = pickFrom[:0]
		add(func(int) bool { return true })
	}

	// Deterministic shuffle, then take 3.
	rng.Shuffle(len(pickFrom), func(i, j int) { pickFrom[i], pickFrom[j] = pickFrom[j], pickFrom[i] })
	chosen := []string{answer.Surface}
	for _, w := range pickFrom {
		if len(chosen) >= 4 {
			break
		}
		chosen = append(chosen, w)
	}
	// Shuffle the final options so the correct answer's position is also
	// variant-deterministic.
	rng.Shuffle(len(chosen), func(i, j int) { chosen[i], chosen[j] = chosen[j], chosen[i] })
	return chosen
}

func bandFor(freq int, ratio float64) (int, int) {
	delta := int(float64(freq) * ratio)
	if delta < 1 {
		delta = 1
	}
	low := freq - delta
	if low < 1 {
		low = 1
	}
	return low, freq + delta
}
