package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/wikiSubmission/ws-backend/db"
)

func (s *Server) GetQuran(ctx context.Context, request GetQuranRequestObject) (GetQuranResponseObject, error) {
	p := request.Params

	// 1. Parameter Resolution
	versesMode := p.Verses != nil && *p.Verses != ""

	var startCh, endCh, startV, endV int
	var refs []VerseRef

	// Lang validation — common to both modes
	for _, l := range p.Langs {
		if _, exists := s.Languages[l]; !exists {
			return GetQuran400JSONResponse{BadRequestJSONResponse{Message: "unsupported language: " + l}}, nil
		}
	}

	if versesMode {
		specs, err := ParseVerseRefs(*p.Verses)
		if err != nil {
			return GetQuran400JSONResponse{BadRequestJSONResponse{Message: err.Error()}}, nil
		}
		refs, err = s.ExpandVerseRefs(specs, 300)
		if err != nil {
			return GetQuran400JSONResponse{BadRequestJSONResponse{Message: err.Error()}}, nil
		}
	} else {
		if p.ChapterNumberStart == nil {
			return GetQuran400JSONResponse{BadRequestJSONResponse{
				Message: "chapter_number_start is required when verses is not provided",
			}}, nil
		}
		startCh = *p.ChapterNumberStart
		endCh = startCh
		if p.ChapterNumberEnd != nil {
			endCh = *p.ChapterNumberEnd
		}
		startV, endV = 0, 9999
		if p.VerseStart != nil {
			startV = *p.VerseStart
		}
		if p.VerseEnd != nil {
			endV = *p.VerseEnd
		}
		var err error
		startCh, endCh, startV, endV, err = s.ValidateQuranRange(startCh, endCh, startV, endV)
		if err != nil {
			return GetQuran400JSONResponse{BadRequestJSONResponse{Message: err.Error()}}, nil
		}
		if endCh-startCh+1 > 20 {
			return GetQuran400JSONResponse{BadRequestJSONResponse{
				Message: fmt.Sprintf("chapter range too large (%d chapters); max 20 per request", endCh-startCh+1),
			}}, nil
		}
	}

	// 2. Fetch Core Data
	chapterMap := make(map[int]*ChapterData)
	verseRefMap := make(map[string]*VerseData)

	var versesRaw []db.VerseRow
	var fetchErr error
	if versesMode {
		chNums, viNums := refsToArrays(refs)
		versesRaw, fetchErr = s.DB.FetchVersesWithTranslationsByRefs(ctx, chNums, viNums, p.Langs)
	} else {
		versesRaw, fetchErr = s.DB.FetchVersesWithTranslations(ctx, startCh, endCh, startV, endV, p.Langs)
	}
	if fetchErr != nil {
		slog.Error("FetchVersesWithTranslations failed", "error", fetchErr)
		return GetQuran500JSONResponse{InternalServerErrrorJSONResponse{Message: "failed to fetch verses"}}, nil
	}

	// 3. First Pass: Build Chapters and Verses (Minified Fields)
	for _, row := range versesRaw {
		if _, ok := chapterMap[row.ChapterNumber]; !ok {
			chNum := row.ChapterNumber
			chapterMap[row.ChapterNumber] = &ChapterData{
				Cn:     &chNum,
				Titles: &map[string]string{},
				Verses: &[]VerseData{},
			}
		}
		(*chapterMap[row.ChapterNumber].Titles)[row.LangCode] = row.ChapterTitle

		vKey := fmt.Sprintf("%d:%d", row.ChapterNumber, row.VerseIndex)
		if _, ok := verseRefMap[vKey]; !ok {
			vIdx, vK := row.VerseIndex, vKey
			verseRefMap[vKey] = &VerseData{
				Vi: &vIdx,
				Vk: &vK,
				Tr: &map[string]TranslationContent{},
			}
		}

		dir := TranslationContentD(row.Direction)
		lID, lCode, content := row.LangID, row.LangCode, row.VerseContent
		(*verseRefMap[vKey].Tr)[row.LangCode] = TranslationContent{
			Li: &lID,
			Lc: &lCode,
			D:  &dir,
			Tx: &content,
			S:  row.VerseSubtitle,
			F:  row.VerseFooter,
		}
	}

	// 4. Second Pass: Attach Words (Minified Fields)
	if p.IncludeWords != nil && *p.IncludeWords {
		wordLangs := p.Langs
		if p.WordLangs != nil && len(*p.WordLangs) > 0 {
			wordLangs = *p.WordLangs
		}

		var wordsRaw []db.WordRow
		var wErr error
		if versesMode {
			chNums, viNums := refsToArrays(refs)
			wordsRaw, wErr = s.DB.FetchWordsForVersesByRefs(ctx, chNums, viNums, wordLangs)
		} else {
			wordsRaw, wErr = s.DB.FetchWordsForVerses(ctx, startCh, endCh, startV, endV, wordLangs)
		}
		if wErr != nil {
			slog.Warn("Failed to fetch words for verses", "from", fmt.Sprintf("%d:%d", startCh, startV), "to", fmt.Sprintf("%d:%d", endCh, endV), "error", wErr)
		} else {
			includeRoot := p.IncludeRoot != nil && *p.IncludeRoot
			includeMeaning := p.IncludeMeaning != nil && *p.IncludeMeaning
			attachWordsToVerses(wordsRaw, verseRefMap, includeRoot, includeMeaning)
		}
	}

	// 5. Final Assembly
	for vKey, vData := range verseRefMap {
		var chNum int
		_, err := fmt.Sscanf(vKey, "%d:", &chNum)
		if err != nil {
			slog.Warn("Failed to parse verse key, skipping", "key", vKey, "error", err)
		}
		if ch, ok := chapterMap[chNum]; ok {
			*ch.Verses = append(*ch.Verses, *vData)
		}
	}

	// 6. Final Sort and Return
	var finalChapters []ChapterData
	for _, ch := range chapterMap {
		sort.Slice(*ch.Verses, func(i, j int) bool {
			return *(*ch.Verses)[i].Vi < *(*ch.Verses)[j].Vi
		})
		finalChapters = append(finalChapters, *ch)
	}
	sort.Slice(finalChapters, func(i, j int) bool {
		return *finalChapters[i].Cn < *finalChapters[j].Cn
	})

	count := len(verseRefMap)
	info := &QuranResponseInfo{ResultCount: &count}
	if !versesMode {
		info.ChapterStart = &startCh
		info.ChapterEnd = &endCh
		info.VerseStart = &startV
		info.VerseEnd = &endV
	}

	return GetQuran200JSONResponse{
		Chapters: &finalChapters,
		Info:     info,
	}, nil
}
