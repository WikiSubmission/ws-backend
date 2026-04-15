package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/wikiSubmission/ws-backend/db"
)

func (s *Server) Search(ctx context.Context, request SearchRequestObject) (SearchResponseObject, error) {
	p := request.Params

	// 1. Validate: q is required unless root is provided (root-based word lookup)
	rootSearch := p.Root != nil && *p.Root != ""
	if !rootSearch {
		if p.Q == nil || len(*p.Q) < 2 {
			return Search400JSONResponse{BadRequestJSONResponse{Message: "q must be at least 2 characters (or provide root for root-based word search)"}}, nil
		}
		if len(*p.Q) > 200 {
			return Search400JSONResponse{BadRequestJSONResponse{Message: "q must be 200 characters or fewer"}}, nil
		}
	}

	// 2. Resolve and validate response languages.
	// nil means "all languages" in the DB query; empty slice would match nothing.
	var langs []string
	if p.Langs != nil && len(*p.Langs) > 0 {
		for _, lc := range *p.Langs {
			if _, ok := s.Languages[lc]; !ok {
				return Search400JSONResponse{BadRequestJSONResponse{
					Message: "unsupported language: " + lc,
				}}, nil
			}
		}
		langs = *p.Langs
	}

	// 3. Pagination defaults
	limit := 20
	if p.Limit != nil {
		limit = *p.Limit
	}
	offset := 0
	if p.Offset != nil {
		offset = *p.Offset
	}

	// 4. Scope
	scope := string(Verses)
	if p.Scope != nil {
		scope = string(*p.Scope)
	}

	// 5. Search + count concurrently
	type countResult struct {
		n   int
		err error
	}
	countCh := make(chan countResult, 1)

	var (
		chapterMap  = make(map[int]*ChapterData)
		verseRefMap = make(map[string]*VerseData)
		searchErr   error
		queryLabel  string
	)

	if rootSearch {
		// --- Root-based word lookup (exact match on word_root) ---
		root := *p.Root
		queryLabel = "root:" + root
		go func() {
			n, err := s.DB.CountSearchWordsByRoot(ctx, root, langs)
			countCh <- countResult{n, err}
		}()
		wordRows, err := s.DB.SearchWordsByRoot(ctx, root, langs, limit, offset)
		searchErr = err
		if err == nil {
			searchErr = groupWordResults(wordRows, p, chapterMap, verseRefMap)
		}
	} else if scope == string(Words) {
		// --- FTS word scope with language detection ---
		q := *p.Q
		queryLabel = q
		// Detect search language; fall back to user-specified langs if detection
		// yields nil (Latin/ASCII query whose language is unambiguous to the user).
		searchLangs := detectQueryLangs(q, s.QuranLangs)
		if searchLangs == nil {
			searchLangs = langs
		}
		go func() {
			n, err := s.DB.CountSearchWords(ctx, q, searchLangs)
			countCh <- countResult{n, err}
		}()
		wordRows, err := s.DB.SearchWords(ctx, q, searchLangs, limit, offset)
		searchErr = err
		if err == nil {
			searchErr = groupWordResults(wordRows, p, chapterMap, verseRefMap)
		}
	} else {
		// --- FTS verse scope (default) — two-phase multilingual search ---
		q := *p.Q
		queryLabel = q

		// Phase A search language: auto-detected from query script.
		searchLangs := detectQueryLangs(q, s.QuranLangs)

		// Phase B response language: user preference, or all available langs.
		responseLangs := langs
		if responseLangs == nil {
			responseLangs = make([]string, 0, len(s.QuranLangs))
			for lc := range s.QuranLangs {
				responseLangs = append(responseLangs, lc)
			}
		}

		// Embed first so we can launch the matching count strategy concurrently.
		var queryEmbedding []float32
		if s.Embedder != nil && (p.Semantic == nil || *p.Semantic) {
			embedCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			vec, embedErr := s.Embedder.Embed(embedCtx, q)
			if embedErr != nil {
				slog.Warn("embedding failed, falling back to FTS", "error", embedErr)
			} else {
				queryEmbedding = vec
			}
		}

		// Cosine distance ceiling for the vector leg. Verses with distance >= threshold
		// are excluded before RRF fusion. Default 0.65 (similarity > 0.35).
		// Tune via VECTOR_DISTANCE_THRESHOLD env var (e.g. "0.55" for stricter).
		vecThreshold := 0.65
		if v := os.Getenv("VECTOR_DISTANCE_THRESHOLD"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				vecThreshold = f
			}
		}

		// Count concurrently using the same strategy as the search (hybrid or FTS).
		// CountSearchVersesHybrid delegates to CountSearchVerses when no embedding.
		go func() {
			n, err := s.DB.CountSearchVersesHybrid(ctx, q, queryEmbedding, searchLangs, vecThreshold)
			countCh <- countResult{n, err}
		}()

		// Phase A: hybrid (FTS + vector RRF) or pure FTS when no embedding available.
		// SearchVersesHybrid falls back to SearchVerses when queryEmbedding is nil.
		phaseARows, err := s.DB.SearchVersesHybrid(ctx, q, queryEmbedding, searchLangs, limit, offset, vecThreshold)
		if err != nil {
			searchErr = err
		} else {
			verseScores := make(map[string]float32)
			verseHighlights := make(map[string]map[string]string) // vKey → langCode → headline
			chapterTitleMatches := make(map[int]bool)
			var chNums, viNums []int32
			refSeen := make(map[string]bool)

			for _, row := range phaseARows {
				vKey := fmt.Sprintf("%d:%d", row.ChapterNumber, row.VerseIndex)
				if row.Score > verseScores[vKey] {
					verseScores[vKey] = row.Score
				}
				if verseHighlights[vKey] == nil {
					verseHighlights[vKey] = make(map[string]string)
				}
				verseHighlights[vKey][row.LangCode] = row.Headline
				if row.TitleMatch {
					chapterTitleMatches[row.ChapterNumber] = true
				}
				if !refSeen[vKey] {
					refSeen[vKey] = true
					chNums = append(chNums, int32(row.ChapterNumber))
					viNums = append(viNums, int32(row.VerseIndex))
				}
			}

			if len(chNums) > 0 {
				// Phase B: fetch content in response language(s)
				phaseBRows, phaseBErr := s.DB.FetchVersesWithTranslationsByRefs(ctx, chNums, viNums, responseLangs)
				if phaseBErr != nil {
					searchErr = phaseBErr
				} else {
					groupVerseResultsFromContent(phaseBRows, verseScores, verseHighlights, chapterTitleMatches, chapterMap, verseRefMap)
				}
			}
		}
	}

	cr := <-countCh
	if searchErr != nil {
		slog.Error("search query failed", "scope", scope, "q", queryLabel, "error", searchErr)
		return Search500JSONResponse{InternalServerErrrorJSONResponse{Message: "search failed"}}, nil
	}
	if cr.err != nil {
		slog.Warn("count query failed, pagination total unavailable", "error", cr.err)
	}

	// 6. Assemble chapters
	for vKey, vData := range verseRefMap {
		var chNum int
		if _, err := fmt.Sscanf(vKey, "%d:", &chNum); err != nil {
			slog.Warn("failed to parse verse key in search assembly", "key", vKey, "error", err)
			continue
		}
		if ch, ok := chapterMap[chNum]; ok {
			*ch.Verses = append(*ch.Verses, *vData)
		}
	}

	var finalChapters []ChapterData
	for _, ch := range chapterMap {
		sort.Slice(*ch.Verses, func(i, j int) bool {
			return *(*ch.Verses)[i].Vi < *(*ch.Verses)[j].Vi
		})
		hits := len(*ch.Verses)
		ch.Hits = &hits
		finalChapters = append(finalChapters, *ch)
	}
	// Chapters ordered by highest verse score first, then chapter number
	sort.Slice(finalChapters, func(i, j int) bool {
		si := bestScore(finalChapters[i])
		sj := bestScore(finalChapters[j])
		if si != sj {
			return si > sj
		}
		return *finalChapters[i].Cn < *finalChapters[j].Cn
	})

	total := cr.n
	resultCount := len(verseRefMap)
	// Hybrid/vector search can return results that FTS count doesn't cover.
	// Ensure total is at least as large as what we're returning on this page.
	if total < offset+resultCount {
		total = offset + resultCount
	}
	return Search200JSONResponse{
		Info: &QuranResponseInfo{
			Q:           p.Q,
			Scope:       &scope,
			Total:       &total,
			Limit:       &limit,
			Offset:      &offset,
			ResultCount: &resultCount,
		},
		Chapters: &finalChapters,
	}, nil
}

// groupVerseResultsFromContent builds chapter/verse maps from Phase B content
// rows, enriched with scores and highlights from Phase A.
// Highlights are only set for language codes that appeared in the Phase A search.
func groupVerseResultsFromContent(
	rows []db.VerseRow,
	verseScores map[string]float32,
	verseHighlights map[string]map[string]string, // vKey → langCode → headline
	chapterTitleMatches map[int]bool,
	chapterMap map[int]*ChapterData,
	verseRefMap map[string]*VerseData,
) {
	for _, row := range rows {
		// Chapter
		if _, ok := chapterMap[row.ChapterNumber]; !ok {
			cn := row.ChapterNumber
			tm := chapterTitleMatches[row.ChapterNumber]
			chapterMap[row.ChapterNumber] = &ChapterData{
				Cn:     &cn,
				Titles: &map[string]string{},
				Verses: &[]VerseData{},
				Tm:     &tm,
			}
		}
		(*chapterMap[row.ChapterNumber].Titles)[row.LangCode] = row.ChapterTitle

		// Verse
		vKey := fmt.Sprintf("%d:%d", row.ChapterNumber, row.VerseIndex)
		if _, ok := verseRefMap[vKey]; !ok {
			vi, vk := row.VerseIndex, vKey
			sc := verseScores[vKey]
			verseRefMap[vKey] = &VerseData{
				Vi: &vi,
				Vk: &vk,
				Sc: &sc,
				Tr: &map[string]TranslationContent{},
			}
		}

		// Translation — highlight only for langs that were part of Phase A
		dir := TranslationContentD(row.Direction)
		lID, lCode := row.LangID, row.LangCode
		content := row.VerseContent
		var hl *string
		if hlMap, ok := verseHighlights[vKey]; ok {
			if h, ok2 := hlMap[lCode]; ok2 {
				hl = &h
			}
		}
		(*verseRefMap[vKey].Tr)[lCode] = TranslationContent{
			Li: &lID,
			Lc: &lCode,
			D:  &dir,
			Tx: &content,
			S:  row.VerseSubtitle,
			F:  row.VerseFooter,
			Hl: hl,
		}
	}
}

// groupWordResults populates chapterMap and verseRefMap from word search rows.
func groupWordResults(
	rows []db.SearchWordRow,
	p SearchParams,
	chapterMap map[int]*ChapterData,
	verseRefMap map[string]*VerseData,
) error {
	for _, row := range rows {
		// Chapter
		if _, ok := chapterMap[row.ChapterNumber]; !ok {
			cn := row.ChapterNumber
			noTitle := false
			chapterMap[row.ChapterNumber] = &ChapterData{
				Cn:     &cn,
				Titles: &map[string]string{},
				Verses: &[]VerseData{},
				Tm:     &noTitle,
			}
		}

		// Verse
		vKey := fmt.Sprintf("%d:%d", row.ChapterNumber, row.VerseIndex)
		if _, ok := verseRefMap[vKey]; !ok {
			vi, vk := row.VerseIndex, vKey
			sc := row.Score
			verseRefMap[vKey] = &VerseData{
				Vi: &vi,
				Vk: &vk,
				Sc: &sc,
				Tr: &map[string]TranslationContent{},
				W:  &[]WordData{},
			}
		} else if row.Score > *verseRefMap[vKey].Sc {
			*verseRefMap[vKey].Sc = row.Score
		}

		vData := verseRefMap[vKey]

		// Find or create word entry
		var targetWord *WordData
		for i := range *vData.W {
			if (*vData.W)[i].Wi != nil && *(*vData.W)[i].Wi == row.WordIndex {
				targetWord = &(*vData.W)[i]
				break
			}
		}
		if targetWord == nil {
			wi, gi := row.WordIndex, row.GlobalIndex
			newWord := WordData{
				Wi: &wi,
				Gi: &gi,
				Tx: &map[string]string{},
				Hl: &map[string]string{},
			}
			if p.IncludeRoot != nil && *p.IncludeRoot {
				newWord.R = row.WordRoot
			}
			if p.IncludeMeaning != nil && *p.IncludeMeaning {
				newWord.M = row.Meaning
			}
			*vData.W = append(*vData.W, newWord)
			targetWord = &(*vData.W)[len(*vData.W)-1]
		}
		(*targetWord.Tx)[row.LangCode] = row.WordText
		(*targetWord.Hl)[row.LangCode] = row.Headline
	}
	return nil
}

// bestScore returns the highest verse score in a chapter (for chapter sorting).
func bestScore(ch ChapterData) float32 {
	var best float32
	for _, v := range *ch.Verses {
		if v.Sc != nil && *v.Sc > best {
			best = *v.Sc
		}
	}
	return best
}
