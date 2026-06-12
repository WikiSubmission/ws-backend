package db

import (
	"context"
	"errors"
	"testing"
)

// Integration: drives the full Quran editorial write path against a live DB —
// draft upsert, chapter editor read, publish request (incl. duplicate guard),
// approve-sync into the editorial published tables, and changelog. Skipped when
// no DATABASE_NAME is set (see setupTestDB).

func TestEditorialQuranPublishRoundTrip_Integration(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// A real language from the restored corpus.
	var langID int16
	if err := db.Pool.QueryRow(ctx, `SELECT id FROM languages ORDER BY id LIMIT 1`).Scan(&langID); err != nil {
		t.Fatalf("pick language: %v", err)
	}

	// A throwaway version. Clean any leftovers from a prior run first.
	_, _ = db.Pool.Exec(ctx, `DELETE FROM quran_versions WHERE slug LIKE 'rt-test-%'`)
	var versionID int
	if err := db.Pool.QueryRow(ctx,
		`INSERT INTO quran_versions (slug, name, lang_id) VALUES ('rt-test-version', 'RT Test', $1) RETURNING id`,
		langID).Scan(&versionID); err != nil {
		t.Fatalf("create version: %v", err)
	}

	chapter, err := db.FetchQuranChapterByNumber(ctx, 1)
	if err != nil || chapter == nil {
		t.Fatalf("fetch chapter 1: %v (chapter=%v)", err, chapter)
	}
	verseID, ok, err := db.FetchVerseID(ctx, chapter.ID, 1)
	if err != nil || !ok {
		t.Fatalf("resolve verse 1:1: %v (ok=%v)", err, ok)
	}

	// A throwaway editor with module + version write + approve grants.
	var userID int
	if err := db.Pool.QueryRow(ctx,
		`INSERT INTO users (auth_id, email) VALUES ($1, $2)
		 ON CONFLICT (email) DO UPDATE SET auth_id = EXCLUDED.auth_id RETURNING id`,
		"test:editorial-rt", "editorial-rt-test@example.com").Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	mustExec(t, ctx, db,
		`INSERT INTO editor_module_grants (user_id, module, can_write) VALUES ($1, 'quran', true)
		 ON CONFLICT (user_id, module) DO UPDATE SET can_write = true`, userID)
	mustExec(t, ctx, db,
		`INSERT INTO editor_quran_version_grants (user_id, version_id, can_write, can_approve)
		 VALUES ($1, $2, true, true)
		 ON CONFLICT (user_id, version_id) DO UPDATE SET can_write = true, can_approve = true`,
		userID, versionID)

	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM verse_text_drafts WHERE version_id = $1`, versionID)
		_, _ = db.Pool.Exec(ctx, `DELETE FROM chapter_text_drafts WHERE version_id = $1`, versionID)
		_, _ = db.Pool.Exec(ctx, `DELETE FROM quran_versions WHERE id = $1`, versionID) // cascades published/requests/changelog/grants
		_, _ = db.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	// Permissions reconstruct from the grants.
	perms, err := db.FetchEditorPermissions(ctx, userID, false)
	if err != nil {
		t.Fatalf("fetch perms: %v", err)
	}
	if !perms.CanWriteQuranVersion(versionID) || !perms.CanApproveQuranVersion(versionID) {
		t.Fatalf("expected write+approve on version %d", versionID)
	}

	// Save a verse draft and a chapter-title draft. The draft-save changelog is
	// the handler's responsibility (best-effort, non-fatal), so mirror that here.
	if _, err := db.UpsertVerseDraft(ctx, versionID, verseID, int(langID),
		QuranText{Subtitle: new("Sub"), Content: new("Draft content"), Footer: new("Note")}, userID); err != nil {
		t.Fatalf("upsert verse draft: %v", err)
	}
	ch1, vn1 := 1, 1
	if err := db.AppendChangelog(ctx, ChangelogEntry{
		VersionID: versionID, ChapterNumber: &ch1, VerseNumber: &vn1, Action: ActionVerseDraftSaved, ActorID: userID,
	}); err != nil {
		t.Fatalf("append verse changelog: %v", err)
	}
	if err := db.UpsertChapterDraft(ctx, versionID, chapter.ID, int(langID), new("Draft Title"), userID); err != nil {
		t.Fatalf("upsert chapter draft: %v", err)
	}
	if err := db.AppendChangelog(ctx, ChangelogEntry{
		VersionID: versionID, ChapterNumber: &ch1, Action: ActionChapterDraftSaved, ActorID: userID,
	}); err != nil {
		t.Fatalf("append chapter changelog: %v", err)
	}

	// The editor read reflects the draft.
	verses, err := db.FetchQuranChapterEditorVerses(ctx, versionID, chapter.ID, nil, nil)
	if err != nil {
		t.Fatalf("fetch editor verses: %v", err)
	}
	if len(verses) != chapter.VerseCount {
		t.Fatalf("expected %d verses, got %d", chapter.VerseCount, len(verses))
	}
	if verses[0].Draft == nil || verses[0].Draft.Content == nil || *verses[0].Draft.Content != "Draft content" {
		t.Fatalf("verse 1 draft content not reflected: %+v", verses[0].Draft)
	}
	if verses[0].Published != nil {
		t.Fatalf("verse 1 should not be published yet")
	}

	// Submit for publishing; a second submission while pending is a conflict.
	req, err := db.CreatePublishRequest(ctx, versionID, 1, userID, new("please review"))
	if err != nil {
		t.Fatalf("create publish request: %v", err)
	}
	if _, err := db.CreatePublishRequest(ctx, versionID, 1, userID, nil); !errors.Is(err, ErrPublishRequestPending) {
		t.Fatalf("expected ErrPublishRequestPending on duplicate, got %v", err)
	}

	// Approve: drafts sync into the editorial published tables.
	updated, err := db.ApprovePublishRequest(ctx, req.ID, userID, new("approved"))
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if updated.Status != "approved" || updated.ReviewedBy == nil || *updated.ReviewedBy != userID {
		t.Fatalf("unexpected approved request: %+v", updated)
	}

	var publishedContent *string
	if err := db.Pool.QueryRow(ctx,
		`SELECT content FROM quran_verse_text_published WHERE version_id = $1 AND verse_id = $2`,
		versionID, verseID).Scan(&publishedContent); err != nil {
		t.Fatalf("read published verse: %v", err)
	}
	if publishedContent == nil || *publishedContent != "Draft content" {
		t.Fatalf("published verse content = %v, want 'Draft content'", publishedContent)
	}
	var publishedTitle *string
	if err := db.Pool.QueryRow(ctx,
		`SELECT title FROM quran_chapter_text_published WHERE version_id = $1 AND chapter_id = $2`,
		versionID, chapter.ID).Scan(&publishedTitle); err != nil {
		t.Fatalf("read published chapter title: %v", err)
	}
	if publishedTitle == nil || *publishedTitle != "Draft Title" {
		t.Fatalf("published title = %v, want 'Draft Title'", publishedTitle)
	}

	// The editor read now shows the published text alongside the draft.
	verses, err = db.FetchQuranChapterEditorVerses(ctx, versionID, chapter.ID, nil, nil)
	if err != nil {
		t.Fatalf("re-fetch editor verses: %v", err)
	}
	if verses[0].Published == nil || verses[0].Published.Content == nil || *verses[0].Published.Content != "Draft content" {
		t.Fatalf("verse 1 published not reflected after approve: %+v", verses[0].Published)
	}

	// Changelog recorded the lifecycle.
	changelog, err := db.ListChangelog(ctx, versionID, 1, 50)
	if err != nil {
		t.Fatalf("list changelog: %v", err)
	}
	seen := map[string]bool{}
	for _, e := range changelog {
		seen[e.Action] = true
	}
	for _, want := range []string{ActionVerseDraftSaved, ActionChapterDraftSaved, ActionPublishRequested, ActionPublishApproved, ActionPublished} {
		if !seen[want] {
			t.Errorf("changelog missing action %q (saw %v)", want, seen)
		}
	}

	// Re-approving the now-resolved request is a conflict.
	if _, err := db.ApprovePublishRequest(ctx, req.ID, userID, nil); !errors.Is(err, ErrPublishRequestNotPending) {
		t.Fatalf("expected ErrPublishRequestNotPending on re-approve, got %v", err)
	}
}

func mustExec(t *testing.T, ctx context.Context, db *DB, sql string, args ...any) {
	t.Helper()
	if _, err := db.Pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec failed: %v\nsql: %s", err, sql)
	}
}
