package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

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

	// 2. Resolve and validate langs
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
		queryLabel  string // for logging
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
		// --- FTS word scope ---
		q := *p.Q
		queryLabel = q
		go func() {
			n, err := s.DB.CountSearchWords(ctx, q, langs)
			countCh <- countResult{n, err}
		}()
		wordRows, err := s.DB.SearchWords(ctx, q, langs, limit, offset)
		searchErr = err
		if err == nil {
			searchErr = groupWordResults(wordRows, p, chapterMap, verseRefMap)
		}
	} else {
		// --- FTS verse scope (default) ---
		q := *p.Q
		queryLabel = q
		go func() {
			n, err := s.DB.CountSearchVerses(ctx, q, langs)
			countCh <- countResult{n, err}
		}()
		verseRows, err := s.DB.SearchVerses(ctx, q, langs, limit, offset)
		searchErr = err
		if err == nil {
			groupVerseResults(verseRows, p, chapterMap, verseRefMap)
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
	// Chapters ordered by highest verse score first (search relevance), then chapter number
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

// groupVerseResults populates chapterMap and verseRefMap from verse search rows.
func groupVerseResults(
	rows []db.SearchVerseRow,
	p SearchParams,
	chapterMap map[int]*ChapterData,
	verseRefMap map[string]*VerseData,
) {
	for _, row := range rows {
		// Chapter
		if _, ok := chapterMap[row.ChapterNumber]; !ok {
			cn := row.ChapterNumber
			tm := row.TitleMatch
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
			sc := row.Score
			verseRefMap[vKey] = &VerseData{
				Vi: &vi,
				Vk: &vk,
				Sc: &sc,
				Tr: &map[string]TranslationContent{},
			}
		} else if row.Score > *verseRefMap[vKey].Sc {
			*verseRefMap[vKey].Sc = row.Score
		}

		// Translation with highlight
		dir := TranslationContentD(row.Direction)
		lID, lCode, hl, content := row.LangID, row.LangCode, row.Headline, row.Content
		(*verseRefMap[vKey].Tr)[row.LangCode] = TranslationContent{
			Li: &lID,
			Lc: &lCode,
			D:  &dir,
			Tx: &content,
			S:  row.Subtitle,
			F:  row.Footer,
			Hl: &hl,
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
