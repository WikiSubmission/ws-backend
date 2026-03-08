package handlers

import (
	"context"
	"fmt"
	"sort"
)

func (s *Server) GetQuran(ctx context.Context, request GetQuranRequestObject) (GetQuranResponseObject, error) {
	p := request.Params

	// 1. Parameter Resolution
	startCh, endCh := p.ChapterNumberStart, p.ChapterNumberStart
	if p.ChapterNumberEnd != nil {
		endCh = *p.ChapterNumberEnd
	}
	startV, endV := 0, 9999
	if p.VerseStart != nil {
		startV = *p.VerseStart
	}
	if p.VerseEnd != nil {
		endV = *p.VerseEnd
	}
	rangeInfo := fmt.Sprintf("%d:%d - %d:%d", startCh, startV, endCh, endV)
	startCh, endCh, startV, endV, err := s.ValidateQuranRange(startCh, endCh, startV, endV)
	if err != nil {
		return GetQuran400JSONResponse{BadRequestJSONResponse{Message: err.Error()}}, nil
	}

	for _, l := range p.Langs {
		if _, exists := s.Languages[l]; !exists {
			return GetQuran400JSONResponse{BadRequestJSONResponse{Message: "Unsupported language: " + l}}, nil
		}
	}

	// 2. Fetch Core Data
	versesRaw, err := s.DB.FetchVersesWithTranslations(ctx, startCh, endCh, startV, endV, p.Langs)
	if err != nil {
		return GetQuran501JSONResponse{NotImplementedErrorJSONResponse{Message: err.Error()}}, nil
	}

	chapterMap := make(map[int]*ChapterData)
	verseRefMap := make(map[string]*VerseData)

	// 3. First Pass: Build Chapters and Verses (Minified Fields)
	for _, row := range versesRaw {
		if _, ok := chapterMap[row.ChapterNumber]; !ok {
			chNum := row.ChapterNumber
			chapterMap[row.ChapterNumber] = &ChapterData{
				Cn:     &chNum, // chapter_number -> Cn
				Titles: &map[string]string{},
				Verses: &[]VerseData{},
			}
		}
		(*chapterMap[row.ChapterNumber].Titles)[row.LangCode] = row.ChapterTitle

		vKey := fmt.Sprintf("%d:%d", row.ChapterNumber, row.VerseIndex)
		if _, ok := verseRefMap[vKey]; !ok {
			vIdx, vK := row.VerseIndex, vKey
			verseRefMap[vKey] = &VerseData{
				Vi: &vIdx,                            // verse_index -> Vi
				Vk: &vK,                              // verse_key -> Vk
				Tr: &map[string]TranslationContent{}, // translations -> Tr
			}
		}

		dir := TranslationContentD(row.Direction)
		lID, lCode, content := row.LangID, row.LangCode, row.VerseContent
		(*verseRefMap[vKey].Tr)[row.LangCode] = TranslationContent{
			Li: &lID,              // lang_id -> Li
			Lc: &lCode,            // lang_code -> Lc
			D:  &dir,              // direction -> D
			Tx: &content,          // text -> Tx
			S:  row.VerseSubtitle, // subtitle -> S
			F:  row.VerseFooter,   // footer -> F
		}
	}

	// 4. Second Pass: Attach Words (Minified Fields)
	if p.IncludeWords != nil && *p.IncludeWords {
		wordLangs := p.Langs
		if p.WordLangs != nil && len(*p.WordLangs) > 0 {
			wordLangs = *p.WordLangs
		}

		wordsRaw, err := s.DB.FetchWordsForVerses(ctx, startCh, endCh, startV, endV, wordLangs)
		if err == nil {
			for _, wRow := range wordsRaw {
				vKey := fmt.Sprintf("%d:%d", wRow.ChapterNumber, wRow.VerseIndex)
				vData, exists := verseRefMap[vKey]
				if !exists {
					continue
				}

				if vData.W == nil { // words -> W
					vData.W = &[]WordData{}
				}

				var targetWord *WordData
				for i := range *vData.W {
					if (*vData.W)[i].Wi != nil && *(*vData.W)[i].Wi == wRow.WordIndex {
						targetWord = &(*vData.W)[i]
						break
					}
				}

				if targetWord == nil {
					wIdx, gIdx := wRow.WordIndex, wRow.GlobalIndex
					newWord := WordData{
						Wi: &wIdx,                // word_index -> Wi
						Gi: &gIdx,                // global_index -> Gi
						Tx: &map[string]string{}, // text -> Tx
					}

					if p.IncludeRoot != nil && *p.IncludeRoot {
						newWord.R = wRow.WordRoot // root -> R
					}

					if p.IncludeMeaning != nil && *p.IncludeMeaning {
						newWord.M = wRow.Meaning // meaning -> M
					}

					*vData.W = append(*vData.W, newWord)
					targetWord = &(*vData.W)[len(*vData.W)-1]
				}
				(*targetWord.Tx)[wRow.LangCode] = wRow.WordText
			}
		}
	}

	// 5. Final Assembly
	for vKey, vData := range verseRefMap {
		var chNum int
		fmt.Sscanf(vKey, "%d:", &chNum)
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
	return GetQuran200JSONResponse{
		Info: &struct {
			Rc *int    `json:"rc,omitempty"`
			Rr *string `json:"rr,omitempty"`
		}{Rc: &count, Rr: &rangeInfo}, // Note the swapped order
		Chapters: &finalChapters,
	}, nil
}
