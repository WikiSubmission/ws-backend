package db

import (
	"context"
	"testing"
)

// Integration: drives the admin rectification path against a live DB — the
// integrity report, roots-master sync (repair after a deliberately removed
// root), and draft-shell pre-seeding (idempotent, never clobbering). Skipped
// when no DATABASE_NAME is set (see setupTestDB).

func TestEditorialQuranRectify_Integration(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	var langID int16
	if err := db.Pool.QueryRow(ctx, `SELECT id FROM languages ORDER BY id LIMIT 1`).Scan(&langID); err != nil {
		t.Fatalf("pick language: %v", err)
	}

	_, _ = db.Pool.Exec(ctx, `DELETE FROM quran_versions WHERE slug LIKE 'rectify-test-%'`)
	var versionID int
	if err := db.Pool.QueryRow(ctx,
		`INSERT INTO quran_versions (slug, name, lang_id) VALUES ('rectify-test-version', 'Rectify Test', $1) RETURNING id`,
		langID).Scan(&versionID); err != nil {
		t.Fatalf("create version: %v", err)
	}

	var userID int
	if err := db.Pool.QueryRow(ctx,
		`INSERT INTO users (auth_id, email) VALUES ($1, $2)
		 ON CONFLICT (email) DO UPDATE SET auth_id = EXCLUDED.auth_id RETURNING id`,
		"test:editorial-rectify", "editorial-rectify-test@example.com").Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Cleanup(func() {
		// word_text_drafts.version_id is ON DELETE SET NULL, so drop the version's
		// drafts explicitly before the version (otherwise they orphan).
		_, _ = db.Pool.Exec(ctx, `DELETE FROM word_text_drafts WHERE version_id = $1`, versionID)
		_, _ = db.Pool.Exec(ctx, `DELETE FROM quran_versions WHERE id = $1`, versionID)
		_, _ = db.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	// --- baseline report ---
	r0, err := db.FetchQuranIntegrityReport(ctx, versionID)
	if err != nil {
		t.Fatalf("report r0: %v", err)
	}
	if r0.TotalWords < 1 || r0.TotalRoots < 1 {
		t.Skipf("corpus not seeded (words=%d roots=%d)", r0.TotalWords, r0.TotalRoots)
	}
	if r0.WordsWithDraft != 0 || r0.RootMeaningDrafts != 0 || r0.WordsPublished != 0 {
		t.Fatalf("fresh version should have no coverage: %+v", r0)
	}

	// --- roots sync repairs a removed root ---
	var removedLetters string
	if err := db.Pool.QueryRow(ctx,
		`DELETE FROM quran_roots WHERE id = (SELECT id FROM quran_roots ORDER BY id LIMIT 1) RETURNING letters`).
		Scan(&removedLetters); err != nil {
		t.Fatalf("remove a root: %v", err)
	}
	r1, err := db.FetchQuranIntegrityReport(ctx, versionID)
	if err != nil {
		t.Fatalf("report r1: %v", err)
	}
	if r1.MissingRoots < 1 {
		t.Fatalf("expected >=1 missing root after removal, got %d", r1.MissingRoots)
	}
	if r1.TotalRoots != r0.TotalRoots-1 {
		t.Fatalf("total roots after removal = %d, want %d", r1.TotalRoots, r0.TotalRoots-1)
	}
	inserted, err := db.SyncQuranRoots(ctx)
	if err != nil {
		t.Fatalf("sync roots: %v", err)
	}
	if inserted < 1 {
		t.Fatalf("expected >=1 root inserted, got %d", inserted)
	}
	r2, err := db.FetchQuranIntegrityReport(ctx, versionID)
	if err != nil {
		t.Fatalf("report r2: %v", err)
	}
	if r2.MissingRoots != 0 {
		t.Fatalf("missing roots after sync = %d, want 0", r2.MissingRoots)
	}
	if r2.TotalRoots != r0.TotalRoots {
		t.Fatalf("total roots after sync = %d, want %d", r2.TotalRoots, r0.TotalRoots)
	}

	// --- count chapter 1 words so we can assert the seed creates exactly that many ---
	var chapter1Words int
	if err := db.Pool.QueryRow(ctx, `
		SELECT count(*) FROM words w
		JOIN verses v ON v.id = w.verse_id
		JOIN chapters c ON c.id = v.chapter_id
		WHERE c.chapter_number = 1`).Scan(&chapter1Words); err != nil {
		t.Fatalf("count chapter 1 words: %v", err)
	}
	if chapter1Words < 1 {
		t.Skip("chapter 1 has no words")
	}

	// --- seed chapter-1 word shells + all root shells ---
	ch := 1
	wordShells, rootShells, err := db.SeedDraftShells(ctx, versionID, int(langID), &ch, true, userID)
	if err != nil {
		t.Fatalf("seed shells: %v", err)
	}
	if wordShells != chapter1Words {
		t.Fatalf("word shells created = %d, want %d", wordShells, chapter1Words)
	}
	if rootShells != r2.TotalRoots {
		t.Fatalf("root shells created = %d, want %d", rootShells, r2.TotalRoots)
	}

	r3, err := db.FetchQuranIntegrityReport(ctx, versionID)
	if err != nil {
		t.Fatalf("report r3: %v", err)
	}
	if r3.WordsWithDraft != chapter1Words {
		t.Fatalf("words with draft after seed = %d, want %d", r3.WordsWithDraft, chapter1Words)
	}
	if r3.RootMeaningDrafts != r2.TotalRoots {
		t.Fatalf("root meaning drafts after seed = %d, want %d", r3.RootMeaningDrafts, r2.TotalRoots)
	}

	// --- re-seed is a no-op (never clobbers) ---
	wordShells2, rootShells2, err := db.SeedDraftShells(ctx, versionID, int(langID), &ch, true, userID)
	if err != nil {
		t.Fatalf("re-seed shells: %v", err)
	}
	if wordShells2 != 0 || rootShells2 != 0 {
		t.Fatalf("re-seed should create nothing, got words=%d roots=%d", wordShells2, rootShells2)
	}

	// --- changelog recorded the chapter-scoped seed ---
	changelog, err := db.ListChangelog(ctx, versionID, 1, 50)
	if err != nil {
		t.Fatalf("list changelog: %v", err)
	}
	seen := false
	for _, e := range changelog {
		if e.Action == ActionDraftShellsSeeded {
			seen = true
		}
	}
	if !seen {
		t.Errorf("changelog missing %q for chapter 1", ActionDraftShellsSeeded)
	}
}
