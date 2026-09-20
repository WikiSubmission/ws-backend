package handlers

import (
	"testing"

	"github.com/wikiSubmission/ws-backend/db"
)

// Word and root scope search rows only ever carry the matched word, never the
// full verse translations — attachTranslationsToVerses is what grafts the
// complete verse content onto those results after the fact, per the /search
// `langs` contract ("returned verse objects always include the translations
// for these codes"). This exercises that graft directly, with no DB needed.
func TestAttachTranslationsToVerses(t *testing.T) {
	vi1 := 1
	vk1 := "1:1"
	noTitle := false
	chapterMap := map[int]*ChapterData{
		1: {
			Cn:     intPtr(1),
			Titles: &map[string]string{},
			Verses: &[]VerseData{},
			Tm:     &noTitle,
		},
	}
	verseRefMap := map[string]*VerseData{
		// Built the way groupWordResults leaves it: tr present but empty,
		// only the matched word populated in w.
		"1:1": {
			Vi: &vi1,
			Vk: &vk1,
			Tr: &map[string]TranslationContent{},
			W:  &[]WordData{{Wi: intPtr(0)}},
		},
	}

	rows := []db.VerseRow{
		{ChapterNumber: 1, ChapterTitle: "The Opening", VerseIndex: 1, LangID: 1, LangCode: "en", Direction: "ltr", VerseContent: "In the name of God, the Gracious, the Merciful."},
		{ChapterNumber: 1, ChapterTitle: "الفاتحة", VerseIndex: 1, LangID: 2, LangCode: "ar", Direction: "rtl", VerseContent: "بِسْمِ اللَّهِ الرَّحْمَٰنِ الرَّحِيمِ"},
		// A verse not present in verseRefMap: must be ignored, not inserted.
		{ChapterNumber: 1, ChapterTitle: "The Opening", VerseIndex: 2, LangID: 1, LangCode: "en", VerseContent: "Praise be to God, Lord of the Worlds."},
	}

	attachTranslationsToVerses(rows, chapterMap, verseRefMap)

	v, ok := verseRefMap["1:1"]
	if !ok {
		t.Fatalf("verse 1:1 unexpectedly removed from map")
	}
	tr := *v.Tr
	if len(tr) != 2 {
		t.Fatalf("got %d translations, want 2: %+v", len(tr), tr)
	}
	en, ok := tr["en"]
	if !ok {
		t.Fatalf("missing en translation")
	}
	if en.Tx == nil || *en.Tx != "In the name of God, the Gracious, the Merciful." {
		t.Errorf("en.Tx = %v, want full verse text", en.Tx)
	}
	if en.Lc == nil || *en.Lc != "en" {
		t.Errorf("en.Lc = %v, want \"en\"", en.Lc)
	}
	ar, ok := tr["ar"]
	if !ok || ar.Tx == nil || *ar.Tx != "بِسْمِ اللَّهِ الرَّحْمَٰنِ الرَّحِيمِ" {
		t.Errorf("ar translation missing or wrong: %+v", ar)
	}

	// The matched word from the original word/root search must survive
	// untouched — this graft adds tr, it must not disturb w.
	if v.W == nil || len(*v.W) != 1 {
		t.Errorf("matched word data lost or altered: %+v", v.W)
	}

	// Chapter title map on the chapter (not the verse) must also be filled.
	titles := *chapterMap[1].Titles
	if titles["en"] != "The Opening" || titles["ar"] != "الفاتحة" {
		t.Errorf("chapter titles not populated: %+v", titles)
	}

	// verse 1:2 (not in verseRefMap) must not have been inserted.
	if _, exists := verseRefMap["1:2"]; exists {
		t.Errorf("verse absent from original map must not be introduced by the graft")
	}
}

// attachTranslationsToVerses must not panic when a verse's Tr map is nil
// (e.g. a hand-built VerseData that never initialized it) — it should
// allocate one rather than dereferencing a nil map.
func TestAttachTranslationsToVerses_NilTr(t *testing.T) {
	vi1 := 1
	verseRefMap := map[string]*VerseData{
		"2:5": {Vi: &vi1},
	}
	chapterMap := map[int]*ChapterData{} // chapter absent entirely: must not panic either

	rows := []db.VerseRow{
		{ChapterNumber: 2, ChapterTitle: "The Cow", VerseIndex: 5, LangID: 1, LangCode: "en", VerseContent: "verse text"},
	}

	attachTranslationsToVerses(rows, chapterMap, verseRefMap)

	v := verseRefMap["2:5"]
	if v.Tr == nil {
		t.Fatalf("nil Tr map not allocated")
	}
	if (*v.Tr)["en"].Tx == nil || *(*v.Tr)["en"].Tx != "verse text" {
		t.Errorf("translation not attached after nil Tr allocation: %+v", *v.Tr)
	}
}

// verseRefsFromMap must extract the (chapter_number, verse_index) pairs
// already present in the map, parsing the "cn:vi" key format, so the *ByRefs
// fetch helpers can be called with them.
func TestVerseRefsFromMap(t *testing.T) {
	vi0, vi254 := 0, 254
	verseRefMap := map[string]*VerseData{
		"1:0":   {Vi: &vi0},
		"2:254": {Vi: &vi254},
		// Malformed key: must be skipped, not crash.
		"bad": {Vi: &vi0},
		// Vi nil: must be skipped.
		"3:0": {Vi: nil},
	}

	chNums, viNums := verseRefsFromMap(verseRefMap)

	if len(chNums) != 2 || len(viNums) != 2 {
		t.Fatalf("got %d refs, want 2: chNums=%v viNums=%v", len(chNums), chNums, viNums)
	}
	got := map[[2]int32]bool{}
	for i := range chNums {
		got[[2]int32{chNums[i], viNums[i]}] = true
	}
	if !got[[2]int32{1, 0}] || !got[[2]int32{2, 254}] {
		t.Errorf("missing expected refs, got chNums=%v viNums=%v", chNums, viNums)
	}
}

func intPtr(i int) *int { return &i }
