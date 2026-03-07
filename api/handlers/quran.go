package handlers

import (
	"context"
	"fmt"
	"sort"
)

func (s *Server) GetQuran(ctx context.Context, request GetQuranRequestObject) (GetQuranResponseObject, error) {
	p := request.Params

	// 1. Resolve Parameters
	startCh := p.ChapterNumberStart
	endCh := startCh
	if p.ChapterNumberEnd != nil {
		endCh = *p.ChapterNumberEnd
	}

	startV := 1
	if p.VerseStart != nil {
		startV = *p.VerseStart
	}

	endV := 9999
	if p.VerseEnd != nil {
		endV = *p.VerseEnd
	}

	rangeInfo := fmt.Sprintf("%d:%d - %d:%d", startCh, startV, endCh, endV)

	// 2. Fetch from DB
	versesRaw, err := s.DB.FetchVersesWithTranslations(ctx, startCh, endCh, startV, endV, p.Langs)
	if err != nil {
		return GetQuran501JSONResponse{ // Using 501 or 500 based on your spec
			NotImplementedErrorJSONResponse{
				Message: "Database error: " + err.Error(),
				Code:    "DATABASE_ERROR",
			},
		}, nil
	}

	if len(versesRaw) == 0 {
		return GetQuran404JSONResponse{NotFoundJSONResponse{Message: "No verses found"}}, nil
	}

	// 3. Assemble Structure
	chapterMap := make(map[int]*ChapterData)
	verseRefMap := make(map[string]*VerseData)

	for _, row := range versesRaw {
		if _, ok := chapterMap[row.ChapterNumber]; !ok {
			titles := make(map[string]string)
			verses := []VerseData{}
			chapterNum := row.ChapterNumber // Local copy to take address
			chapterMap[row.ChapterNumber] = &ChapterData{
				ChapterNumber: &chapterNum,
				Titles:        &titles, // Corrected: Pointer to map
				Verses:        &verses, // Corrected: Pointer to slice
			}
		}
		// Corrected: Dereference pointer to index map
		(*chapterMap[row.ChapterNumber].Titles)[row.LangCode] = row.ChapterTitle

		vKey := fmt.Sprintf("%d:%d", row.ChapterNumber, row.VerseIndex)

		if _, ok := verseRefMap[vKey]; !ok {
			vIdx := row.VerseIndex
			vKeyStr := vKey
			translations := []TranslationContent{}
			vData := VerseData{
				VerseIndex:   &vIdx,
				VerseKey:     &vKeyStr,
				Translations: &translations,
			}
			verseRefMap[vKey] = &vData

			// Append the verse to the chapter's slice
			ch := chapterMap[row.ChapterNumber]
			*ch.Verses = append(*ch.Verses, vData)
		}

		// Map translations
		langID := row.LangID
		langCode := row.LangCode
		content := row.VerseContent
		dir := TranslationContentDirection(row.Direction)

		trans := TranslationContent{
			LangId:    &langID,
			LangCode:  &langCode,
			Direction: &dir,
			Text:      &content,
			Subtitle:  row.VerseSubtitle,
			Footer:    row.VerseFooter,
		}

		// Corrected: Dereference pointer to append to slice
		vData := verseRefMap[vKey]
		*vData.Translations = append(*vData.Translations, trans)
	}

	// 4. Handle Words
	if p.IncludeWords != nil && *p.IncludeWords {
		wordsRaw, err := s.DB.FetchWordsForVerses(ctx, startCh, endCh, startV, endV, p.Langs)
		if err == nil {
			for _, wRow := range wordsRaw {
				vKey := fmt.Sprintf("%d:%d", wRow.ChapterNumber, wRow.VerseIndex)
				vData, exists := verseRefMap[vKey]
				if !exists {
					continue
				}

				if vData.Words == nil {
					words := []WordData{}
					vData.Words = &words
				}

				var targetWord *WordData
				for i := range *vData.Words {
					if *(*vData.Words)[i].WordIndex == wRow.WordIndex {
						targetWord = &(*vData.Words)[i]
						break
					}
				}

				if targetWord == nil {
					wIdx := wRow.WordIndex
					gIdx := wRow.GlobalIndex
					textMap := make(map[string]string)
					newWord := WordData{
						WordIndex:   &wIdx,
						GlobalIndex: &gIdx,
						Text:        &textMap, // Pointer to map
						Root:        wRow.WordRoot,
					}
					*vData.Words = append(*vData.Words, newWord)
					targetWord = &(*vData.Words)[len(*vData.Words)-1]
				}

				(*targetWord.Text)[wRow.LangCode] = wRow.WordText
			}
		}
	}

	// 5. Finalize Response
	var finalChapters []ChapterData
	for _, ch := range chapterMap {
		// Sort verses inside the chapter
		sort.Slice(*ch.Verses, func(i, j int) bool {
			return *(*ch.Verses)[i].VerseIndex < *(*ch.Verses)[j].VerseIndex
		})
		finalChapters = append(finalChapters, *ch)
	}

	sort.Slice(finalChapters, func(i, j int) bool {
		return *finalChapters[i].ChapterNumber < *finalChapters[j].ChapterNumber
	})

	count := len(verseRefMap)
	return GetQuran200JSONResponse{
		Info: &struct {
			RequestedRange *string `json:"requested_range,omitempty"`
			ResultsCount   *int    `json:"results_count,omitempty"`
		}{
			RequestedRange: &rangeInfo,
			ResultsCount:   &count,
		},
		Chapters: &finalChapters,
	}, nil
}
