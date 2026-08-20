package db

import (
	"context"
	"testing"
)

// These tests exercise the topical index against a live database, using the
// package-shared pool from db_test.go (skipped when DATABASE_NAME is unset). They
// assume `just seed-topic-index` has run — every one of them skips rather than
// fails on an empty index, so an unseeded database does not look broken.

func seededTopicIndex(t *testing.T, db *DB) {
	t.Helper()
	langs, err := db.FetchTopicIndexLangs(context.Background())
	if err != nil {
		t.Fatalf("FetchTopicIndexLangs failed: %v", err)
	}
	if len(langs) == 0 {
		t.Skip("Skipping: topical index not seeded (run `just seed-topic-index`)")
	}
}

func TestTopicIndexLetterCounts(t *testing.T) {
	db := setupTestDB(t)
	seededTopicIndex(t, db)
	ctx := context.Background()

	counts, err := db.TopicIndexLetterCounts(ctx, "en")
	if err != nil {
		t.Fatalf("TopicIndexLetterCounts failed: %v", err)
	}
	if len(counts) == 0 {
		t.Fatal("expected at least one populated letter")
	}

	total := 0
	for _, c := range counts {
		if len(c.Letter) != 1 || c.Letter < "A" || c.Letter > "Z" {
			t.Errorf("letter %q is not a single A-Z character", c.Letter)
		}
		if c.Count < 1 {
			t.Errorf("letter %q reported count %d", c.Letter, c.Count)
		}
		// The printed index has no X. A future edition might, so this is only a
		// note if it appears — but the English one must not invent it.
		if c.Letter == "X" {
			t.Errorf("letter X should not exist in the printed English index")
		}
		total += c.Count
	}
	if total < 1000 {
		t.Errorf("expected over a thousand entries across all letters, got %d", total)
	}
}

func TestTopicIndexByLetter(t *testing.T) {
	db := setupTestDB(t)
	seededTopicIndex(t, db)
	ctx := context.Background()

	entries, total, err := db.TopicIndexByLetter(ctx, "en", "A", 500, 0)
	if err != nil {
		t.Fatalf("TopicIndexByLetter failed: %v", err)
	}
	if total == 0 || len(entries) == 0 {
		t.Fatal("expected entries under letter A")
	}
	if len(entries) != total {
		t.Errorf("a 500-entry page should cover letter A entirely: got %d of %d", len(entries), total)
	}

	// Printed order, and every entry actually belongs to the letter asked for.
	for i, e := range entries {
		if e.Letter != "A" {
			t.Errorf("entry %q has letter %q, expected A", e.Title, e.Letter)
		}
		if e.Title == "" || e.Slug == "" {
			t.Errorf("entry %d has an empty title or slug", i)
		}
		if i > 0 && entries[i-1].Position > e.Position {
			t.Errorf("entries are not in printed order at index %d", i)
		}
		// Slices must never come back nil — the API layer relies on this so the
		// JSON carries [] rather than null.
		if e.Refs == nil || e.Subentries == nil {
			t.Errorf("entry %q has a nil Refs or Subentries slice", e.Title)
		}
	}

	// Abraham is the canonical nested entry: no references of its own, many
	// sub-entries that each carry some.
	var abraham *TopicIndexEntry
	for i := range entries {
		if entries[i].Title == "Abraham" {
			abraham = &entries[i]
			break
		}
	}
	if abraham == nil {
		t.Fatal(`expected an "Abraham" entry under letter A`)
	}
	if len(abraham.Subentries) < 10 {
		t.Errorf("Abraham should have many sub-entries, got %d", len(abraham.Subentries))
	}
	subRefs := 0
	for _, sub := range abraham.Subentries {
		if sub.Label == "" {
			t.Error("Abraham has an unlabelled sub-entry")
		}
		subRefs += len(sub.Refs)
	}
	if subRefs == 0 {
		t.Error("Abraham's sub-entries carry no verse references")
	}
}

func TestTopicIndexByLetterPaging(t *testing.T) {
	db := setupTestDB(t)
	seededTopicIndex(t, db)
	ctx := context.Background()

	first, total, err := db.TopicIndexByLetter(ctx, "en", "S", 5, 0)
	if err != nil {
		t.Fatalf("TopicIndexByLetter page 1 failed: %v", err)
	}
	if total <= 5 {
		t.Skipf("letter S has only %d entries; nothing to page", total)
	}
	if len(first) != 5 {
		t.Fatalf("expected 5 entries on the first page, got %d", len(first))
	}

	second, secondTotal, err := db.TopicIndexByLetter(ctx, "en", "S", 5, 5)
	if err != nil {
		t.Fatalf("TopicIndexByLetter page 2 failed: %v", err)
	}
	if secondTotal != total {
		t.Errorf("total changed between pages: %d then %d", total, secondTotal)
	}
	if len(second) == 0 {
		t.Fatal("expected a second page of entries")
	}
	// total is the count for the whole letter, not the page.
	if first[len(first)-1].Position >= second[0].Position {
		t.Error("the second page does not continue where the first ended")
	}
}

func TestTopicIndexByLetterEmpty(t *testing.T) {
	db := setupTestDB(t)
	seededTopicIndex(t, db)
	ctx := context.Background()

	// X has no entries in the printed index. An empty letter must return an empty
	// slice and a zero total, not an error.
	entries, total, err := db.TopicIndexByLetter(ctx, "en", "X", 500, 0)
	if err != nil {
		t.Fatalf("TopicIndexByLetter for an empty letter failed: %v", err)
	}
	if total != 0 || len(entries) != 0 {
		t.Errorf("expected nothing under X, got %d entries (total %d)", len(entries), total)
	}
}

func TestSearchTopicIndex(t *testing.T) {
	db := setupTestDB(t)
	seededTopicIndex(t, db)
	ctx := context.Background()

	results, err := db.SearchTopicIndex(ctx, "alimony", "en", 10, 0)
	if err != nil {
		t.Fatalf("SearchTopicIndex failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal(`expected a hit for "alimony"`)
	}

	found := false
	for _, r := range results {
		if r.Title == "Alimony" {
			found = true
			if len(r.Refs) == 0 {
				t.Error("the Alimony entry came back without its verse references")
			}
		}
		if r.Score <= 0 {
			t.Errorf("hit %q has a non-positive score %v", r.Title, r.Score)
		}
		if r.Headline == "" {
			t.Errorf("hit %q came back without a headline", r.Title)
		}
	}
	if !found {
		t.Errorf(`"Alimony" was not among the hits for "alimony"`)
	}

	total, err := db.CountSearchTopicIndex(ctx, "alimony", "en")
	if err != nil {
		t.Fatalf("CountSearchTopicIndex failed: %v", err)
	}
	if total < len(results) {
		t.Errorf("count %d is below the %d results returned", total, len(results))
	}
}

func TestSearchTopicIndexMatchesSubentryLabels(t *testing.T) {
	db := setupTestDB(t)
	seededTopicIndex(t, db)
	ctx := context.Background()

	// "monotheist" is a sub-entry label under Abraham, not an entry title. It
	// should still be findable, via the band-B weighting on subentry_text.
	results, err := db.SearchTopicIndex(ctx, "monotheist", "en", 10, 0)
	if err != nil {
		t.Fatalf("SearchTopicIndex failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal(`expected a hit for the sub-entry label "monotheist"`)
	}
}

func TestSearchTopicIndexTrigramFallback(t *testing.T) {
	db := setupTestDB(t)
	seededTopicIndex(t, db)
	ctx := context.Background()

	// A typo finds nothing through full text, so the trigram leg has to answer.
	results, err := db.SearchTopicIndex(ctx, "abrogaton", "en", 10, 0)
	if err != nil {
		t.Fatalf("SearchTopicIndex failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal(`expected the trigram fallback to answer "abrogaton"`)
	}

	// The count covers the full-text leg only, so a typo-only match legitimately
	// reports zero. Documented in the OpenAPI schema description.
	total, err := db.CountSearchTopicIndex(ctx, "abrogaton", "en")
	if err != nil {
		t.Fatalf("CountSearchTopicIndex failed: %v", err)
	}
	if total != 0 {
		t.Logf("note: %q now matches full text too (total %d)", "abrogaton", total)
	}
}

func TestSearchTopicIndexAllEditions(t *testing.T) {
	db := setupTestDB(t)
	seededTopicIndex(t, db)
	ctx := context.Background()

	// An empty lang means every edition, the same way a nil langs slice does in
	// db/search.go.
	results, err := db.SearchTopicIndex(ctx, "abraham", "", 10, 0)
	if err != nil {
		t.Fatalf("SearchTopicIndex across editions failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal(`expected a hit for "abraham" across every edition`)
	}
}

func TestTopicIndexEntryBySlug(t *testing.T) {
	db := setupTestDB(t)
	seededTopicIndex(t, db)
	ctx := context.Background()

	entry, err := db.TopicIndexEntryBySlug(ctx, "en", "abraham")
	if err != nil {
		t.Fatalf("TopicIndexEntryBySlug failed: %v", err)
	}
	if entry == nil {
		t.Fatal(`expected to find the "abraham" slug`)
	}
	if entry.Title != "Abraham" {
		t.Errorf("slug abraham resolved to %q", entry.Title)
	}

	missing, err := db.TopicIndexEntryBySlug(ctx, "en", "not-a-real-topic-slug")
	if err != nil {
		t.Fatalf("TopicIndexEntryBySlug for a missing slug errored: %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil for an unknown slug, got %q", missing.Title)
	}
}

func TestTopicIndexCrossReferences(t *testing.T) {
	db := setupTestDB(t)
	seededTopicIndex(t, db)
	ctx := context.Background()

	// "Agreements see Covenant, Promises" is the canonical cross-reference: the
	// printed text points at two other entries, and it also has sub-entries.
	entry, err := db.TopicIndexEntryBySlug(ctx, "en", "agreements")
	if err != nil {
		t.Fatalf("TopicIndexEntryBySlug failed: %v", err)
	}
	if entry == nil {
		t.Skip(`no "agreements" entry in this edition`)
	}
	if len(entry.CrossRefs) < 2 {
		t.Errorf("expected Agreements to cross-reference two entries, got %v", entry.CrossRefs)
	}
	if len(entry.Subentries) == 0 {
		t.Error("expected Agreements to keep its own sub-entries alongside the cross-references")
	}
}

func TestTopicIndexForVerse(t *testing.T) {
	db := setupTestDB(t)
	seededTopicIndex(t, db)
	ctx := context.Background()

	// 2:240 is cited by Alimony (2:240-241), so the range has to match
	// inclusively at its start.
	entries, err := db.TopicIndexForVerse(ctx, "en", 2, 240)
	if err != nil {
		t.Fatalf("TopicIndexForVerse failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one topic citing 2:240")
	}

	// And inside the range, not just at its edges.
	inside, err := db.TopicIndexForVerse(ctx, "en", 2, 241)
	if err != nil {
		t.Fatalf("TopicIndexForVerse failed: %v", err)
	}
	if len(inside) == 0 {
		t.Error("expected a range citation to match its interior, not only its start")
	}

	// A verse nothing cites must return an empty slice, not an error.
	none, err := db.TopicIndexForVerse(ctx, "en", 114, 6)
	if err != nil {
		t.Fatalf("TopicIndexForVerse for an uncited verse failed: %v", err)
	}
	_ = none
}

func TestSeedTopicIndexIsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	first, err := db.SeedTopicIndex(ctx)
	if err != nil {
		t.Fatalf("first SeedTopicIndex failed: %v", err)
	}
	if first == 0 {
		t.Fatal("seed wrote no entries")
	}

	before, _, err := db.TopicIndexByLetter(ctx, "en", "A", 500, 0)
	if err != nil {
		t.Fatalf("TopicIndexByLetter failed: %v", err)
	}

	second, err := db.SeedTopicIndex(ctx)
	if err != nil {
		t.Fatalf("second SeedTopicIndex failed: %v", err)
	}
	if second != first {
		t.Errorf("re-seeding wrote %d entries, first run wrote %d", second, first)
	}

	// Full-replace, so a second run must not accumulate duplicates.
	after, _, err := db.TopicIndexByLetter(ctx, "en", "A", 500, 0)
	if err != nil {
		t.Fatalf("TopicIndexByLetter failed: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("letter A went from %d entries to %d after re-seeding", len(before), len(after))
	}
}

func TestCountTopicIndex(t *testing.T) {
	db := setupTestDB(t)
	seededTopicIndex(t, db)
	ctx := context.Background()

	// Every edition.
	all, err := db.CountTopicIndex(ctx, "")
	if err != nil {
		t.Fatalf("CountTopicIndex failed: %v", err)
	}
	if all.Entries == 0 || all.Subentries == 0 || all.Refs == 0 {
		t.Fatalf("census came back empty for a seeded index: %+v", all)
	}
	// References outnumber entries by roughly eleven to one in the printed index.
	if all.Refs < all.Entries {
		t.Errorf("expected more references (%d) than entries (%d)", all.Refs, all.Entries)
	}

	// Scoped to the one edition that exists, so the numbers must agree.
	en, err := db.CountTopicIndex(ctx, "en")
	if err != nil {
		t.Fatalf("CountTopicIndex(en) failed: %v", err)
	}
	if en != all {
		t.Errorf("en census %+v differs from the all-editions census %+v", en, all)
	}

	// An edition that does not exist censuses to zero rather than erroring.
	none, err := db.CountTopicIndex(ctx, "zz")
	if err != nil {
		t.Fatalf("CountTopicIndex for an unknown edition failed: %v", err)
	}
	if none.Entries != 0 || none.Subentries != 0 || none.Refs != 0 {
		t.Errorf("expected an empty census for an unknown edition, got %+v", none)
	}
}
