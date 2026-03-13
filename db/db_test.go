package db

import (
	"context"
	"os"
	"testing"

	"github.com/joho/godotenv"
)

// setupTestDB connects to the database and skips the test if no env vars are set.
// All integration tests should call this helper.
func setupTestDB(t *testing.T) *DB {
	t.Helper()
	_ = godotenv.Load()
	if os.Getenv("DATABASE_NAME") == "" {
		t.Skip("Skipping integration test: no DATABASE_NAME env var set")
	}
	db, err := DbSetup()
	if err != nil {
		t.Fatalf("DbSetup failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestFetchAllLanguages(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	langs, err := db.FetchAllLanguages(ctx)
	if err != nil {
		t.Fatalf("FetchAllLanguages: %v", err)
	}
	if len(langs) == 0 {
		t.Fatal("expected at least one language, got none")
	}

	// Build a lookup for assertion convenience
	byCode := make(map[string]LanguageRow)
	for _, l := range langs {
		byCode[l.Code] = l
	}

	// Core languages that must always be present (seeded in 003_seed.sql)
	required := []struct {
		code      string
		direction string
	}{
		{"ar", "rtl"},
		{"en", "ltr"},
		{"fr", "ltr"},
		{"ur", "rtl"},
		{"fa", "rtl"},
	}
	for _, r := range required {
		l, ok := byCode[r.code]
		if !ok {
			t.Errorf("language %q missing from results", r.code)
			continue
		}
		if l.Direction != r.direction {
			t.Errorf("language %q: direction = %q, want %q", r.code, l.Direction, r.direction)
		}
	}
}

func TestFetchAllChapterMeta(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	chapters, err := db.FetchAllChapterMeta(ctx)
	if err != nil {
		t.Fatalf("FetchAllChapterMeta: %v", err)
	}
	if len(chapters) != 114 {
		t.Fatalf("expected 114 chapters, got %d", len(chapters))
	}

	byNum := make(map[int]ChapterMetaRow)
	for _, c := range chapters {
		byNum[c.ChapterNumber] = c
	}

	// Spot-check a few well-known chapters
	checks := []struct {
		num        int
		verseCount int
	}{
		{1, 7},
		{2, 286},
		{36, 83},
		{114, 6},
	}
	for _, c := range checks {
		ch, ok := byNum[c.num]
		if !ok {
			t.Errorf("chapter %d missing", c.num)
			continue
		}
		if ch.VerseCount != c.verseCount {
			t.Errorf("chapter %d: VerseCount = %d, want %d", c.num, ch.VerseCount, c.verseCount)
		}
	}

	// Chapters must be in ascending order
	for i := 1; i < len(chapters); i++ {
		if chapters[i].ChapterNumber <= chapters[i-1].ChapterNumber {
			t.Errorf("chapters not sorted at index %d: %d after %d",
				i, chapters[i].ChapterNumber, chapters[i-1].ChapterNumber)
		}
	}
}

func TestFetchVersesWithTranslations(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Al-Fatiha (1:1-7) in English — a well-known, stable dataset
	rows, err := db.FetchVersesWithTranslations(ctx, 1, 1, 1, 7, []string{"en"})
	if err != nil {
		t.Fatalf("FetchVersesWithTranslations: %v", err)
	}
	if len(rows) != 7 {
		t.Fatalf("expected 7 rows for chapter 1, got %d", len(rows))
	}
	for _, r := range rows {
		if r.ChapterNumber != 1 {
			t.Errorf("unexpected chapter_number %d in result", r.ChapterNumber)
		}
		if r.LangCode != "en" {
			t.Errorf("unexpected lang_code %q in result", r.LangCode)
		}
		if r.VerseContent == "" {
			t.Errorf("verse %d has empty content", r.VerseIndex)
		}
	}
}

func TestFetchVersesMultiLang(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	langs := []string{"en", "ar"}
	rows, err := db.FetchVersesWithTranslations(ctx, 1, 1, 1, 7, langs)
	if err != nil {
		t.Fatalf("FetchVersesWithTranslations: %v", err)
	}
	// 7 verses × 2 languages = 14 rows
	if len(rows) != 14 {
		t.Fatalf("expected 14 rows for 2 langs, got %d", len(rows))
	}

	langsSeen := make(map[string]bool)
	for _, r := range rows {
		langsSeen[r.LangCode] = true
	}
	for _, l := range langs {
		if !langsSeen[l] {
			t.Errorf("lang %q not found in results", l)
		}
	}
}

func TestFetchWordsForVerses(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Al-Fatiha (1:1) words in Arabic
	rows, err := db.FetchWordsForVerses(ctx, 1, 1, 1, 1, []string{"ar"})
	if err != nil {
		t.Fatalf("FetchWordsForVerses: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected words for 1:1, got none")
	}
	for _, r := range rows {
		if r.ChapterNumber != 1 || r.VerseIndex != 1 {
			t.Errorf("unexpected row chapter=%d verse=%d", r.ChapterNumber, r.VerseIndex)
		}
		if r.WordText == "" {
			t.Errorf("word at index %d has empty text", r.WordIndex)
		}
	}
}
