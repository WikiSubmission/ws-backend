package bundle

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/wikiSubmission/ws-backend/offline/textnorm"
)

// search mimics the client query path: normalize the user's query identically
// to the indexed `norm` column, then run it as a quoted FTS MATCH.
func search(t *testing.T, path, query string) []string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	norm := textnorm.ForSearch(query)
	match := `"` + strings.ReplaceAll(norm, `"`, `""`) + `"`
	rows, err := db.Query(
		`SELECT v.vk FROM verses_fts f JOIN verses v ON v.rowid = f.rowid
		 WHERE verses_fts MATCH ? ORDER BY rank`, match)
	if err != nil {
		t.Fatalf("search %q (norm %q): %v", query, norm, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var vk string
		if err := rows.Scan(&vk); err != nil {
			t.Fatal(err)
		}
		out = append(out, vk)
	}
	return out
}

func has(vks []string, want string) bool {
	for _, vk := range vks {
		if vk == want {
			return true
		}
	}
	return false
}

func TestWriteArabicBundleAndSearch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quran-ar-v1.db")
	meta := Meta{Scripture: "quran", Lang: "ar", DataVersion: 1, Tokenizer: TokenizerTrigram, BuiltAt: "2026-06-22T00:00:00Z"}
	chapters := []Chapter{{CN: 1, Title: "الفاتحة", TitleTranslit: "Al-Faatihah", VerseCount: 7}}
	verses := []Verse{
		{VK: "1:1", CN: 1, VN: 1, Text: "بِسْمِ ٱللَّهِ ٱلرَّحْمَٰنِ ٱلرَّحِيمِ"},
		{VK: "1:2", CN: 1, VN: 2, Text: "ٱلْحَمْدُ لِلَّهِ رَبِّ ٱلْعَٰلَمِينَ"},
	}
	if err := Write(path, meta, chapters, verses); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Query carries full diacritics; the index stores the normalized form. They
	// must still match because both pass through textnorm.ForSearch.
	if got := search(t, path, "الرَّحْمَٰن"); !has(got, "1:1") {
		t.Errorf("diacritic query did not match 1:1, got %v", got)
	}
	// Bare query (no diacritics) must also match. "ٱلْحَمْدُ" normalizes to
	// "الحمد". (Note: words written with a superscript dagger alef, e.g.
	// "ٱلْعَٰلَمِينَ", normalize to "العلمين" — the mark is stripped like any
	// combining mark, which matches modern spelling for the majority of such
	// words; "العالمين" is a known minority recall gap.)
	if got := search(t, path, "الحمد"); !has(got, "1:2") {
		t.Errorf("bare query did not match 1:2, got %v", got)
	}
}

func TestWriteLatinBundleAndSearch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quran-en-v1.db")
	meta := Meta{Scripture: "quran", Lang: "en", DataVersion: 3, Tokenizer: TokenizerLatin, BuiltAt: "2026-06-22T00:00:00Z"}
	chapters := []Chapter{{CN: 1, Title: "The Key", VerseCount: 7}}
	verses := []Verse{
		{VK: "1:1", CN: 1, VN: 1, Text: "In the name of GOD, Most Gracious, Most Merciful."},
		{VK: "1:3", CN: 1, VN: 3, Text: "Most Gracious, Most Merciful.", Subtitle: "The Almighty"},
	}
	if err := Write(path, meta, chapters, verses); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Case-insensitive (query normalized to lowercase, index too).
	if got := search(t, path, "GOD"); !has(got, "1:1") {
		t.Errorf("GOD did not match 1:1, got %v", got)
	}
	// Porter stemming: querying "mercy" should reach "Merciful".
	if got := search(t, path, "mercy"); !has(got, "1:1") || !has(got, "1:3") {
		t.Errorf("stemmed query 'mercy' should match both verses, got %v", got)
	}
}

func TestBundleSchemaAndMeta(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quran-en-v3.db")
	meta := Meta{Scripture: "quran", Lang: "en", DataVersion: 3, Tokenizer: TokenizerLatin, BuiltAt: "2026-06-22T00:00:00Z"}
	chapters := []Chapter{{CN: 1, Title: "The Key", TitleTranslit: "Al-Faatihah", VerseCount: 7}}
	verses := []Verse{{VK: "1:1", CN: 1, VN: 1, Text: "Test verse."}}
	if err := Write(path, meta, chapters, verses); err != nil {
		t.Fatalf("Write: %v", err)
	}

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	wantMeta := map[string]string{
		"schema_version":        "1",
		"data_version":          "3",
		"normalization_version": "1",
		"scripture":             "quran",
		"lang":                  "en",
		"fts_tokenizer":         TokenizerLatin,
		"built_at":              "2026-06-22T00:00:00Z",
	}
	for k, want := range wantMeta {
		var got string
		if err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, k).Scan(&got); err != nil {
			t.Fatalf("meta[%s]: %v", k, err)
		}
		if got != want {
			t.Errorf("meta[%s] = %q, want %q", k, got, want)
		}
	}

	// Original text is stored verbatim; norm is the folded form.
	var text, norm string
	if err := db.QueryRow(`SELECT text, norm FROM verses WHERE vk = '1:1'`).Scan(&text, &norm); err != nil {
		t.Fatal(err)
	}
	if text != "Test verse." {
		t.Errorf("text not stored verbatim: %q", text)
	}
	if norm != textnorm.ForSearch("Test verse.") {
		t.Errorf("norm = %q, want %q", norm, textnorm.ForSearch("Test verse."))
	}

	var verseCount int
	if err := db.QueryRow(`SELECT verse_count FROM chapters WHERE cn = 1`).Scan(&verseCount); err != nil {
		t.Fatal(err)
	}
	if verseCount != 7 {
		t.Errorf("chapter verse_count = %d, want 7", verseCount)
	}
}

func TestNewManifestEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "quran-en-v3.db")
	meta := Meta{Scripture: "quran", Lang: "en", DataVersion: 3, Tokenizer: TokenizerLatin, BuiltAt: "2026-06-22T00:00:00Z"}
	if err := Write(path, meta, []Chapter{{CN: 1, Title: "K", VerseCount: 1}}, []Verse{{VK: "1:1", CN: 1, VN: 1, Text: "Hi."}}); err != nil {
		t.Fatal(err)
	}

	url := "https://cdn.example/offline/quran-en-v3.db"
	entry, err := NewManifestEntry(path, "quran-en", url, meta)
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID != "quran-en" || entry.Lang != "en" || entry.URL != url {
		t.Errorf("entry identity wrong: %+v", entry)
	}
	if entry.SchemaVersion != SchemaVersion || entry.NormalizationVersion != textnorm.Version {
		t.Errorf("entry versions wrong: %+v", entry)
	}
	if entry.DataVersion != 3 || entry.FTSTokenizer != TokenizerLatin {
		t.Errorf("entry meta wrong: %+v", entry)
	}
	if len(entry.SHA256) != 64 || entry.Bytes <= 0 {
		t.Errorf("checksum/bytes wrong: sha=%q bytes=%d", entry.SHA256, entry.Bytes)
	}

	// Hash is stable across re-reads of the same file.
	again, err := NewManifestEntry(path, "quran-en", url, meta)
	if err != nil {
		t.Fatal(err)
	}
	if again.SHA256 != entry.SHA256 || again.Bytes != entry.Bytes {
		t.Errorf("checksum not stable: %q/%d vs %q/%d", entry.SHA256, entry.Bytes, again.SHA256, again.Bytes)
	}
}

func TestWriteWordsBundle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quran-words-en-v1.db")
	meta := Meta{Scripture: "quran", Lang: "en", Kind: KindWords, DataVersion: 1, Tokenizer: TokenizerNone, BuiltAt: "2026-07-04T00:00:00Z"}
	words := []Word{
		{CN: 1, VN: 1, WI: 0, GI: 1, Lang: "ar", Text: "بِسْمِ", Root: "سمو"},
		{CN: 1, VN: 1, WI: 0, GI: 1, Lang: "en", Text: "In the name", Meaning: "name"},
		{CN: 1, VN: 1, WI: 0, GI: 1, Lang: "tl", Text: "bismi"},
		{CN: 1, VN: 1, WI: 1, GI: 2, Lang: "ar", Text: "ٱللَّهِ", Root: "اله"},
	}
	if err := WriteWords(path, meta, words); err != nil {
		t.Fatalf("WriteWords: %v", err)
	}

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	wantMeta := map[string]string{
		"schema_version": "1",
		"kind":           KindWords,
		"scripture":      "quran",
		"lang":           "en",
		"fts_tokenizer":  TokenizerNone,
	}
	for k, want := range wantMeta {
		var got string
		if err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, k).Scan(&got); err != nil {
			t.Fatalf("meta[%s]: %v", k, err)
		}
		if got != want {
			t.Errorf("meta[%s] = %q, want %q", k, got, want)
		}
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM words`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("words count = %d, want 4", n)
	}

	// Root rides on the Arabic row; meaning on the translation row. Empty
	// strings are stored as NULL.
	var root sql.NullString
	if err := db.QueryRow(
		`SELECT root FROM words WHERE cn = 1 AND vn = 1 AND wi = 0 AND lang = 'ar'`).Scan(&root); err != nil {
		t.Fatal(err)
	}
	if !root.Valid || root.String != "سمو" {
		t.Errorf("ar root = %+v, want سمو", root)
	}
	var meaning sql.NullString
	if err := db.QueryRow(
		`SELECT meaning FROM words WHERE cn = 1 AND vn = 1 AND wi = 0 AND lang = 'tl'`).Scan(&meaning); err != nil {
		t.Fatal(err)
	}
	if meaning.Valid {
		t.Errorf("tl meaning should be NULL, got %q", meaning.String)
	}

	// The manifest entry records the words kind.
	entry, err := NewManifestEntry(path, "quran-words-en", "https://cdn.example/offline/quran-words-en-v1.db", meta)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Kind != KindWords {
		t.Errorf("entry.Kind = %q, want %q", entry.Kind, KindWords)
	}
}

func TestWriteWordsBundleRejectsWrongKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.db")
	meta := Meta{Scripture: "quran", Lang: "en", DataVersion: 1, Tokenizer: TokenizerNone, BuiltAt: "2026-07-04T00:00:00Z"}
	if err := WriteWords(path, meta, []Word{{CN: 1, VN: 1, WI: 0, GI: 1, Lang: "en", Text: "x"}}); err == nil {
		t.Fatal("expected error for meta without KindWords")
	}
}

func TestTextBundleKindDefaultsToText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "quran-en-v1.db")
	meta := Meta{Scripture: "quran", Lang: "en", DataVersion: 1, Tokenizer: TokenizerLatin, BuiltAt: "2026-07-04T00:00:00Z"}
	if err := Write(path, meta, []Chapter{{CN: 1, Title: "K", VerseCount: 1}}, []Verse{{VK: "1:1", CN: 1, VN: 1, Text: "Hi."}}); err != nil {
		t.Fatal(err)
	}
	entry, err := NewManifestEntry(path, "quran-en", "https://cdn.example/offline/quran-en-v1.db", meta)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Kind != KindText {
		t.Errorf("entry.Kind = %q, want %q", entry.Kind, KindText)
	}
}

func TestWriteLibraryBundleAndSearch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library-en-v1.db")
	meta := Meta{Scripture: "library", Lang: "en", Kind: KindLibrary, DataVersion: 1, Tokenizer: TokenizerLatin, BuiltAt: "2026-07-05T00:00:00Z"}
	docs := []Doc{
		{DocType: "introduction", DocNumber: 0, Title: "Introduction", SectionIndex: 0, Body: "This is God's final message to humanity."},
		{DocType: "appendix", DocNumber: 19, Title: "Hadith and Sunna", SectionIndex: 0, Heading: "Satanic Innovations", Body: "Hadith and Sunna are fabrications."},
		{DocType: "appendix", DocNumber: 1, Title: "One of the Great Miracles", SectionIndex: 2, Body: "The mathematical miracle of the Quran."},
	}
	if err := WriteLibrary(path, meta, docs); err != nil {
		t.Fatalf("WriteLibrary: %v", err)
	}

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	wantMeta := map[string]string{
		"schema_version": "1",
		"kind":           KindLibrary,
		"scripture":      "library",
		"lang":           "en",
		"fts_tokenizer":  TokenizerLatin,
	}
	for k, want := range wantMeta {
		var got string
		if err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, k).Scan(&got); err != nil {
			t.Fatalf("meta[%s]: %v", k, err)
		}
		if got != want {
			t.Errorf("meta[%s] = %q, want %q", k, got, want)
		}
	}

	// The client query path: normalized quoted MATCH with a snippet, joined
	// back to docs (mirrors native-content-store searchDocs).
	norm := textnorm.ForSearch("hadith")
	match := `"` + strings.ReplaceAll(norm, `"`, `""`) + `"`
	rows, err := db.Query(
		`SELECT d.doc_type, d.doc_number, d.title, d.section_index,
		        snippet(docs_fts, 0, '<b>', '</b>', '…', 12) AS hl
		 FROM docs_fts f JOIN docs d ON d.rowid = f.rowid
		 WHERE docs_fts MATCH ? ORDER BY rank`, match)
	if err != nil {
		t.Fatalf("searchDocs: %v", err)
	}
	defer rows.Close()

	type hit struct {
		docType string
		num     int
		title   string
		idx     int
		hl      string
	}
	var hits []hit
	for rows.Next() {
		var h hit
		if err := rows.Scan(&h.docType, &h.num, &h.title, &h.idx, &h.hl); err != nil {
			t.Fatal(err)
		}
		hits = append(hits, h)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit for %q, got %d: %+v", "hadith", len(hits), hits)
	}
	if hits[0].docType != "appendix" || hits[0].num != 19 {
		t.Errorf("hit = %+v, want appendix 19", hits[0])
	}
	if !strings.Contains(hits[0].hl, "<b>") {
		t.Errorf("snippet missing highlight: %q", hits[0].hl)
	}

	// Heading text must be searchable too (norm indexes heading + body).
	if got := searchDocsCount(t, db, "satanic innovations"); got != 1 {
		t.Errorf("heading search hits = %d, want 1", got)
	}
}

func searchDocsCount(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	norm := textnorm.ForSearch(query)
	match := `"` + strings.ReplaceAll(norm, `"`, `""`) + `"`
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM docs_fts WHERE docs_fts MATCH ?`, match).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func TestWriteLibraryRejectsWrongKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library-en-v1.db")
	meta := Meta{Scripture: "library", Lang: "en", DataVersion: 1, Tokenizer: TokenizerLatin, BuiltAt: "2026-07-05T00:00:00Z"}
	err := WriteLibrary(path, meta, []Doc{{DocType: "appendix", DocNumber: 1, Title: "t", Body: "b"}})
	if err == nil {
		t.Fatal("expected error for missing KindLibrary")
	}
}
