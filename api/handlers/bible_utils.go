package handlers

import "fmt"

// validateBibleRange checks that the requested book/chapter/verse range is
// well-formed and within known bounds. The DB query handles out-of-range
// gracefully (returns empty), so this is a fast-fail for obvious bad input.
func validateBibleRange(bookNum, startCh, endCh, startV, endV int, bibleBooks map[int]BibleBookCache) error {
	book, exists := bibleBooks[bookNum]
	if !exists {
		return fmt.Errorf("unknown book number: %d", bookNum)
	}
	if startCh < 1 {
		return fmt.Errorf("chapter_start must be >= 1")
	}
	if endCh < startCh {
		return fmt.Errorf("chapter_end must be >= chapter_start")
	}
	if book.ChapterCount > 0 && startCh > book.ChapterCount {
		return fmt.Errorf("chapter_start %d exceeds %q chapter count (%d)", startCh, book.Name, book.ChapterCount)
	}
	if endCh-startCh+1 > 10 {
		return fmt.Errorf("chapter range too large (%d chapters); max 10 per request", endCh-startCh+1)
	}
	if startV < 1 {
		return fmt.Errorf("verse_start must be >= 1")
	}
	if endV < startV {
		return fmt.Errorf("verse_end must be >= verse_start")
	}
	return nil
}
