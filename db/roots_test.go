package db

import (
	"context"
	"strings"
	"testing"
)

func TestRootsIndex_FrequencySort(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	rows, total, err := db.RootsIndex(ctx, nil, "frequency", 50, 0)
	if err != nil {
		t.Fatalf("RootsIndex: %v", err)
	}
	if total < len(rows) {
		t.Fatalf("total (%d) < len(rows) (%d)", total, len(rows))
	}
	if len(rows) == 0 {
		t.Skip("matview empty — refresh root_index_mv before running this test")
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].Occurrences > rows[i-1].Occurrences {
			t.Errorf("frequency sort violated at index %d: %d > %d",
				i, rows[i].Occurrences, rows[i-1].Occurrences)
		}
	}
}

func TestRootsIndex_AbjadiVsReverse(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	asc, _, err := db.RootsIndex(ctx, nil, "abjadi", 100, 0)
	if err != nil {
		t.Fatalf("abjadi: %v", err)
	}
	desc, _, err := db.RootsIndex(ctx, nil, "reverse", 100, 0)
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if len(asc) == 0 || len(desc) == 0 {
		t.Skip("matview empty")
	}
	// First of asc should not equal first of desc (unless degenerate case)
	if len(asc) > 1 && asc[0].Letters == desc[0].Letters {
		t.Errorf("abjadi and reverse produced identical first row: %q", asc[0].Letters)
	}
}

func TestRootsIndex_PrefixFilter(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	prefix := "ك"
	rows, _, err := db.RootsIndex(ctx, &prefix, "frequency", 100, 0)
	if err != nil {
		t.Fatalf("RootsIndex with prefix: %v", err)
	}
	if len(rows) == 0 {
		t.Skip("no roots match prefix — corpus may not be seeded")
	}
	for _, r := range rows {
		if !strings.HasPrefix(r.Letters, prefix) {
			t.Errorf("row %q does not start with %q", r.Letters, prefix)
		}
	}
}

func TestRootDetail_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	root := "ك ت ب"
	summary, derivs, occ, _, err := db.RootDetail(ctx, root)
	if err != nil {
		t.Skipf("ك ت ب not present (matview not refreshed?): %v", err)
	}
	if summary.Letters != root {
		t.Errorf("summary.Letters = %q, want %q", summary.Letters, root)
	}
	if summary.Occurrences <= 0 {
		t.Errorf("summary.Occurrences = %d, want > 0", summary.Occurrences)
	}
	if len(derivs) == 0 {
		t.Errorf("expected at least one derivative for %q", root)
	}
	// At least one derivative of a well-known root must have an English gloss
	// populated from word_text. Verifies the en_wt join wired through.
	hasEn := false
	for _, d := range derivs {
		if d.En != nil && *d.En != "" {
			hasEn = true
			break
		}
	}
	if !hasEn {
		t.Errorf("expected at least one derivative with non-empty En for %q", root)
	}
	if len(occ) == 0 {
		t.Errorf("expected at least one occurrence for %q", root)
	}
	if len(occ) > 20 {
		t.Errorf("occurrences should be capped at 20, got %d", len(occ))
	}
	for _, o := range occ {
		if o.ChapterNumber < 1 || o.ChapterNumber > 114 {
			t.Errorf("invalid chapter_number %d", o.ChapterNumber)
		}
		if o.Ar == "" {
			t.Errorf("empty Arabic verse content")
		}
	}
}

func TestRootDetail_NotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, _, _, _, err := db.RootDetail(ctx, "ز ز ز ز ز")
	if err == nil {
		t.Fatal("expected error for non-existent root, got nil")
	}
}

func TestRootOccurrences_Pagination(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	root := "ك ت ب"
	page1, total1, err := db.RootOccurrences(ctx, root, "", 5, 0)
	if err != nil {
		t.Skipf("RootOccurrences page1 failed (corpus not seeded?): %v", err)
	}
	if total1 == 0 {
		t.Skip("no occurrences for ك ت ب — corpus not seeded")
	}
	if len(page1) > 5 {
		t.Errorf("page1 returned %d, want <= 5", len(page1))
	}

	page2, total2, err := db.RootOccurrences(ctx, root, "", 5, 5)
	if err != nil {
		t.Fatalf("RootOccurrences page2: %v", err)
	}
	if total1 != total2 {
		t.Errorf("total mismatch between pages: %d vs %d", total1, total2)
	}

	// page1 and page2 should not overlap (different verse keys)
	seen := make(map[string]bool)
	for _, o := range page1 {
		seen[o.VerseKey()] = true
	}
	for _, o := range page2 {
		if seen[o.VerseKey()] {
			t.Errorf("page2 overlaps with page1 at vk=%s", o.VerseKey())
		}
	}
}
