package handlers

import (
	"testing"

	"github.com/wikiSubmission/ws-backend/db"
)

// enrichMatchedWordLangs must graft additional languages onto already-matched
// words (e.g. transliteration on a root match) without introducing sibling
// words or dropping the language that already matched.
func TestEnrichMatchedWordLangs(t *testing.T) {
	wi3 := 3
	vi := 1
	vk := "1:1"
	verseRefMap := map[string]*VerseData{
		"1:1": {
			Vi: &vi,
			Vk: &vk,
			W: &[]WordData{
				// Matched word from a root search: only Arabic present.
				{Wi: &wi3, Tx: &map[string]string{"ar": "ٱلرَّحْمَـٰنِ"}},
			},
		},
	}

	rows := []db.WordRow{
		// Transliteration for the matched word: should be grafted on.
		{ChapterNumber: 1, VerseIndex: 1, WordIndex: 3, LangCode: "tl", WordText: "l-raḥmāni"},
		// A sibling word in the same verse: must NOT be added (not a match).
		{ChapterNumber: 1, VerseIndex: 1, WordIndex: 4, LangCode: "tl", WordText: "l-raḥīmi"},
		// A word in a verse not in the map: ignored.
		{ChapterNumber: 2, VerseIndex: 5, WordIndex: 0, LangCode: "tl", WordText: "x"},
	}

	enrichMatchedWordLangs(rows, verseRefMap)

	words := *verseRefMap["1:1"].W
	if len(words) != 1 {
		t.Fatalf("sibling word introduced: got %d words, want 1", len(words))
	}
	tx := *words[0].Tx
	if tx["tl"] != "l-raḥmāni" {
		t.Errorf("transliteration not grafted: tx[tl] = %q, want %q", tx["tl"], "l-raḥmāni")
	}
	if tx["ar"] != "ٱلرَّحْمَـٰنِ" {
		t.Errorf("existing language lost: tx[ar] = %q", tx["ar"])
	}
}

// A matched word with no Tx map yet (word_langs not previously populated) must
// have one created rather than panicking on a nil map.
func TestEnrichMatchedWordLangs_NilTx(t *testing.T) {
	wi := 0
	verseRefMap := map[string]*VerseData{
		"1:1": {W: &[]WordData{{Wi: &wi}}},
	}
	rows := []db.WordRow{
		{ChapterNumber: 1, VerseIndex: 1, WordIndex: 0, LangCode: "tl", WordText: "bis'mi"},
	}

	enrichMatchedWordLangs(rows, verseRefMap)

	w := (*verseRefMap["1:1"].W)[0]
	if w.Tx == nil || (*w.Tx)["tl"] != "bis'mi" {
		t.Fatalf("nil Tx not handled: %+v", w.Tx)
	}
}
