package db

import (
	"context"
	"testing"
	"time"
)

func TestBuildUserExport_IncludesUserData(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	authID := "test-export-" + time.Now().Format("150405.000000")
	email := authID + "@example.com"
	user, err := db.UpsertUser(ctx, authID, email, nil)
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM users WHERE auth_id = $1`, authID)
	})

	if _, err := db.SetActivityConsent(ctx, authID, true); err != nil {
		t.Fatalf("SetActivityConsent: %v", err)
	}
	if _, err := db.UpsertNote(ctx, authID, "quran", "1:1", "my export note", nil); err != nil {
		t.Fatalf("UpsertNote: %v", err)
	}

	out := db.BuildUserExport(ctx, user.ID, authID, email)

	profile, ok := out["profile"].(map[string]any)
	if !ok || profile["email"] != email {
		t.Fatalf("expected profile.email=%q, got %#v", email, out["profile"])
	}
	if out["exported_at"] == "" || out["exported_at"] == nil {
		t.Fatalf("expected exported_at to be set, got %#v", out["exported_at"])
	}
	if out["consent"] != true {
		t.Fatalf("expected consent=true, got %#v", out["consent"])
	}

	notes, ok := out["notes"].(map[string]any)
	if !ok {
		t.Fatalf("expected notes to be a per-scripture map, got %#v", out["notes"])
	}
	quranNotes, ok := notes["quran"].([]NoteRow)
	if !ok || len(quranNotes) == 0 {
		t.Fatalf("expected at least one quran note, got %#v", notes["quran"])
	}

	// Every section the aggregator promises should be present (null on failure,
	// never missing), so the export shape is stable for consumers.
	for _, key := range []string{
		"bookmark_categories", "bookmarks", "cover_to_cover", "collections",
		"reading_progress", "reading_streaks", "activity", "game_attempts", "game_stats", "preferences",
	} {
		if _, present := out[key]; !present {
			t.Fatalf("expected section %q to be present in export", key)
		}
	}
}
