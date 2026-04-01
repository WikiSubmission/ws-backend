package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/wikiSubmission/ws-backend/db"
)

func (s *Server) GetBibleBooks(ctx context.Context, request GetBibleBooksRequestObject) (GetBibleBooksResponseObject, error) {
	books, err := s.DB.FetchBibleBooks(ctx)
	if err != nil {
		slog.Error("FetchBibleBooks failed", "error", err)
		return GetBibleBooks500JSONResponse{InternalServerErrrorJSONResponse{Message: "failed to fetch bible books"}}, nil
	}

	result := make([]BibleBook, 0, len(books))
	for _, b := range books {
		bn, bk, t, cc, vc := b.BookNumber, b.Name, b.Testament, b.ChapterCount, b.VerseCount
		result = append(result, BibleBook{
			Bn: &bn,
			Bk: &bk,
			T:  &t,
			Cc: &cc,
			Vc: &vc,
		})
	}
	return GetBibleBooks200JSONResponse(result), nil
}

func (s *Server) GetBible(ctx context.Context, request GetBibleRequestObject) (GetBibleResponseObject, error) {
	p := request.Params

	// 1. Validate book
	if _, exists := s.BibleBooks[p.Book]; !exists {
		return GetBible400JSONResponse{BadRequestJSONResponse{
			Message: fmt.Sprintf("unknown book number: %d", p.Book),
		}}, nil
	}

	// 2. Validate languages
	for _, l := range p.Langs {
		if _, exists := s.Languages[l]; !exists {
			return GetBible400JSONResponse{BadRequestJSONResponse{
				Message: "unsupported language: " + l,
			}}, nil
		}
	}

	// 3. Resolve chapter/verse range with defaults
	startCh := p.ChapterStart
	endCh := startCh
	if p.ChapterEnd != nil {
		endCh = *p.ChapterEnd
	}
	startV, endV := 1, 9999
	if p.VerseStart != nil {
		startV = *p.VerseStart
	}
	if p.VerseEnd != nil {
		endV = *p.VerseEnd
	}

	if err := validateBibleRange(p.Book, startCh, endCh, startV, endV, s.BibleBooks); err != nil {
		return GetBible400JSONResponse{BadRequestJSONResponse{Message: err.Error()}}, nil
	}

	// 4. Fetch
	versesRaw, err := s.DB.FetchBibleVerses(ctx, p.Book, startCh, endCh, startV, endV, p.Langs)
	if err != nil {
		slog.Error("FetchBibleVerses failed", "book", p.Book, "error", err)
		return GetBible500JSONResponse{InternalServerErrrorJSONResponse{Message: "failed to fetch bible verses"}}, nil
	}

	// 5. Assemble and return
	books := assembleBibleBooks(versesRaw)
	count := countBibleVerses(books)
	bookNum := p.Book

	return GetBible200JSONResponse{
		Info: &BibleResponseInfo{
			ResultCount:  &count,
			Book:         &bookNum,
			ChapterStart: &startCh,
			ChapterEnd:   &endCh,
			VerseStart:   &startV,
			VerseEnd:     &endV,
		},
		Books: &books,
	}, nil
}

func (s *Server) SearchBible(ctx context.Context, request SearchBibleRequestObject) (SearchBibleResponseObject, error) {
	p := request.Params

	// 1. Validate query
	if len(p.Q) < 2 {
		return SearchBible400JSONResponse{BadRequestJSONResponse{Message: "q must be at least 2 characters"}}, nil
	}
	if len(p.Q) > 200 {
		return SearchBible400JSONResponse{BadRequestJSONResponse{Message: "q must be 200 characters or fewer"}}, nil
	}

	// 2. Resolve response languages (what the caller wants returned).
	var responseLangs []string
	if p.Langs != nil && len(*p.Langs) > 0 {
		for _, lc := range *p.Langs {
			if _, ok := s.Languages[lc]; !ok {
				return SearchBible400JSONResponse{BadRequestJSONResponse{
					Message: "unsupported language: " + lc,
				}}, nil
			}
		}
		responseLangs = *p.Langs
	}
	if responseLangs == nil {
		// Default to all available Bible languages.
		responseLangs = make([]string, 0, len(s.BibleLangs))
		for lc := range s.BibleLangs {
			responseLangs = append(responseLangs, lc)
		}
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

	// 4. Detect search language(s) from query script.
	searchLangs := detectQueryLangs(p.Q, s.BibleLangs)

	// 5. Search + count concurrently (both use search langs for FTS).
	type countResult struct {
		n   int
		err error
	}
	countCh := make(chan countResult, 1)
	go func() {
		n, err := s.DB.CountSearchBibleVerses(ctx, p.Q, searchLangs)
		countCh <- countResult{n, err}
	}()

	// Phase A: FTS in detected language(s) → verse refs + scores + highlights.
	phaseARows, err := s.DB.SearchBibleVerses(ctx, p.Q, searchLangs, limit, offset)
	if err != nil {
		slog.Error("SearchBibleVerses failed (Phase A)", "q", p.Q, "error", err)
		<-countCh
		return SearchBible500JSONResponse{InternalServerErrrorJSONResponse{Message: "search failed"}}, nil
	}

	cr := <-countCh
	if cr.err != nil {
		slog.Warn("CountSearchBibleVerses failed", "error", cr.err)
	}

	if len(phaseARows) == 0 {
		total := cr.n
		q := p.Q
		resultCount := 0
		emptyBooks := []BibleBookData{}
		return SearchBible200JSONResponse{
			Info: &BibleResponseInfo{
				Q:           &q,
				Total:       &total,
				Limit:       &limit,
				Offset:      &offset,
				ResultCount: &resultCount,
			},
			Books: &emptyBooks,
		}, nil
	}

	// Extract refs, scores, highlights from Phase A.
	verseScores := make(map[string]float32)
	verseHighlights := make(map[string]map[string]string) // vKey → langCode → headline
	var bookNums, chNums, verseNums []int32
	refSeen := make(map[string]bool)

	for _, row := range phaseARows {
		vKey := fmt.Sprintf("%d:%d:%d", row.BookNumber, row.ChapterNumber, row.VerseNumber)
		if row.Score > verseScores[vKey] {
			verseScores[vKey] = row.Score
		}
		if verseHighlights[vKey] == nil {
			verseHighlights[vKey] = make(map[string]string)
		}
		verseHighlights[vKey][row.LangCode] = row.Headline
		if !refSeen[vKey] {
			refSeen[vKey] = true
			bookNums = append(bookNums, int32(row.BookNumber))
			chNums = append(chNums, int32(row.ChapterNumber))
			verseNums = append(verseNums, int32(row.VerseNumber))
		}
	}

	// Phase B: fetch content in response language(s).
	phaseBRows, err := s.DB.FetchBibleVersesByRefs(ctx, bookNums, chNums, verseNums, responseLangs)
	if err != nil {
		slog.Error("FetchBibleVersesByRefs failed (Phase B)", "q", p.Q, "error", err)
		return SearchBible500JSONResponse{InternalServerErrrorJSONResponse{Message: "search failed"}}, nil
	}

	// 6. Assemble and return.
	books := assembleBibleBooksWithEnrichment(phaseBRows, verseScores, verseHighlights)
	total := cr.n
	q := p.Q
	resultCount := countBibleVerses(books)

	return SearchBible200JSONResponse{
		Info: &BibleResponseInfo{
			Q:           &q,
			Total:       &total,
			Limit:       &limit,
			Offset:      &offset,
			ResultCount: &resultCount,
		},
		Books: &books,
	}, nil
}

// assembleBibleBooks groups flat BibleVerseRow results into the nested
// book → chapter → verse structure.
func assembleBibleBooks(rows []db.BibleVerseRow) []BibleBookData {
	bookMap := make(map[int]*BibleBookData)
	chapterMap := make(map[string]*BibleChapterData) // "bookNum:chNum"
	verseMap := make(map[string]*BibleVerseData)     // "bookNum:chNum:verseNum"

	for _, row := range rows {
		bKey := row.BookNumber
		cKey := fmt.Sprintf("%d:%d", row.BookNumber, row.ChapterNumber)
		vKey := fmt.Sprintf("%d:%d:%d", row.BookNumber, row.ChapterNumber, row.VerseNumber)

		if _, ok := bookMap[bKey]; !ok {
			bn, bk, t := row.BookNumber, row.BookName, row.Testament
			bookMap[bKey] = &BibleBookData{
				Bn:       &bn,
				Bk:       &bk,
				T:        &t,
				Chapters: &[]BibleChapterData{},
			}
		}

		if _, ok := chapterMap[cKey]; !ok {
			cn := row.ChapterNumber
			chapterMap[cKey] = &BibleChapterData{
				Cn:     &cn,
				Verses: &[]BibleVerseData{},
			}
		}

		if _, ok := verseMap[vKey]; !ok {
			vn, vk := row.VerseNumber, vKey
			verseMap[vKey] = &BibleVerseData{
				Vn: &vn,
				Vk: &vk,
				Tr: &map[string]BibleTranslationContent{},
			}
		}

		dir := BibleTranslationContentD(row.Direction)
		lID, lCode, content := row.LangID, row.LangCode, row.Content
		(*verseMap[vKey].Tr)[row.LangCode] = BibleTranslationContent{
			Li: &lID,
			Lc: &lCode,
			D:  &dir,
			Tx: &content,
			F:  row.Footer,
		}
	}

	return finalizeBibleBooks(bookMap, chapterMap, verseMap)
}

// assembleBibleBooksWithEnrichment builds the nested book → chapter → verse
// structure from Phase B content rows, enriched with scores and highlights
// from Phase A. Highlights are only set on translations whose lang code
// appeared in the Phase A FTS search.
func assembleBibleBooksWithEnrichment(
	rows []db.BibleVerseRow,
	verseScores map[string]float32,
	verseHighlights map[string]map[string]string, // vKey → langCode → headline
) []BibleBookData {
	bookMap := make(map[int]*BibleBookData)
	chapterMap := make(map[string]*BibleChapterData)
	verseMap := make(map[string]*BibleVerseData)

	for _, row := range rows {
		bKey := row.BookNumber
		cKey := fmt.Sprintf("%d:%d", row.BookNumber, row.ChapterNumber)
		vKey := fmt.Sprintf("%d:%d:%d", row.BookNumber, row.ChapterNumber, row.VerseNumber)

		if _, ok := bookMap[bKey]; !ok {
			bn, bk, t := row.BookNumber, row.BookName, row.Testament
			bookMap[bKey] = &BibleBookData{
				Bn:       &bn,
				Bk:       &bk,
				T:        &t,
				Chapters: &[]BibleChapterData{},
			}
		}

		if _, ok := chapterMap[cKey]; !ok {
			cn := row.ChapterNumber
			chapterMap[cKey] = &BibleChapterData{
				Cn:     &cn,
				Verses: &[]BibleVerseData{},
			}
		}

		if _, ok := verseMap[vKey]; !ok {
			vn := row.VerseNumber
			vkCopy := vKey
			sc := verseScores[vKey]
			verseMap[vKey] = &BibleVerseData{
				Vn: &vn,
				Vk: &vkCopy,
				Sc: &sc,
				Tr: &map[string]BibleTranslationContent{},
			}
		}

		dir := BibleTranslationContentD(row.Direction)
		lID, lCode, content := row.LangID, row.LangCode, row.Content
		var hl *string
		if hlMap, ok := verseHighlights[vKey]; ok {
			if h, ok2 := hlMap[lCode]; ok2 {
				hl = &h
			}
		}
		(*verseMap[vKey].Tr)[lCode] = BibleTranslationContent{
			Li: &lID,
			Lc: &lCode,
			D:  &dir,
			Tx: &content,
			F:  row.Footer,
			Hl: hl,
		}
	}

	books := finalizeBibleBooks(bookMap, chapterMap, verseMap)

	// Sort by relevance score descending, then by number.
	for i := range books {
		sort.Slice(*books[i].Chapters, func(a, b int) bool {
			sa := bestBibleChapterScore((*books[i].Chapters)[a])
			sb := bestBibleChapterScore((*books[i].Chapters)[b])
			if sa != sb {
				return sa > sb
			}
			return *(*books[i].Chapters)[a].Cn < *(*books[i].Chapters)[b].Cn
		})
		for j := range *books[i].Chapters {
			ch := &(*books[i].Chapters)[j]
			n := len(*ch.Verses)
			ch.Hits = &n
		}
	}
	sort.Slice(books, func(i, j int) bool {
		si := bestBibleBookScore(books[i])
		sj := bestBibleBookScore(books[j])
		if si != sj {
			return si > sj
		}
		return *books[i].Bn < *books[j].Bn
	})

	return books
}

// finalizeBibleBooks attaches verses → chapters → books, sorts by number, and
// returns the assembled slice.
func finalizeBibleBooks(
	bookMap map[int]*BibleBookData,
	chapterMap map[string]*BibleChapterData,
	verseMap map[string]*BibleVerseData,
) []BibleBookData {
	// Attach verses to chapters
	for vKey, vData := range verseMap {
		var bookNum, chNum, verseNum int
		fmt.Sscanf(vKey, "%d:%d:%d", &bookNum, &chNum, &verseNum)
		cKey := fmt.Sprintf("%d:%d", bookNum, chNum)
		if ch, ok := chapterMap[cKey]; ok {
			*ch.Verses = append(*ch.Verses, *vData)
		}
	}

	// Sort verses within each chapter
	for _, ch := range chapterMap {
		sort.Slice(*ch.Verses, func(i, j int) bool {
			return *(*ch.Verses)[i].Vn < *(*ch.Verses)[j].Vn
		})
	}

	// Attach chapters to books
	for cKey, chData := range chapterMap {
		var bookNum, chNum int
		fmt.Sscanf(cKey, "%d:%d", &bookNum, &chNum)
		if bk, ok := bookMap[bookNum]; ok {
			*bk.Chapters = append(*bk.Chapters, *chData)
		}
	}

	// Sort chapters within each book and collect
	var books []BibleBookData
	for _, bk := range bookMap {
		sort.Slice(*bk.Chapters, func(i, j int) bool {
			return *(*bk.Chapters)[i].Cn < *(*bk.Chapters)[j].Cn
		})
		books = append(books, *bk)
	}

	// Sort books by book number
	sort.Slice(books, func(i, j int) bool {
		return *books[i].Bn < *books[j].Bn
	})

	return books
}

func countBibleVerses(books []BibleBookData) int {
	count := 0
	for _, bk := range books {
		if bk.Chapters == nil {
			continue
		}
		for _, ch := range *bk.Chapters {
			if ch.Verses != nil {
				count += len(*ch.Verses)
			}
		}
	}
	return count
}

func bestBibleChapterScore(ch BibleChapterData) float32 {
	var best float32
	if ch.Verses == nil {
		return best
	}
	for _, v := range *ch.Verses {
		if v.Sc != nil && *v.Sc > best {
			best = *v.Sc
		}
	}
	return best
}

func bestBibleBookScore(bk BibleBookData) float32 {
	var best float32
	if bk.Chapters == nil {
		return best
	}
	for _, ch := range *bk.Chapters {
		if s := bestBibleChapterScore(ch); s > best {
			best = s
		}
	}
	return best
}
