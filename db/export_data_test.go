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

	account, ok := out["account"].(map[string]any)
	if !ok || account["email"] != email {
		t.Fatalf("expected account.email=%q, got %#v", email, out["account"])
	}
	if out["exported_at"] == "" || out["exported_at"] == nil {
		t.Fatalf("expected exported_at to be set, got %#v", out["exported_at"])
	}
	if out["activity_tracking_enabled"] != true {
		t.Fatalf("expected activity_tracking_enabled=true, got %#v", out["activity_tracking_enabled"])
	}

	notes, ok := out["notes"].(map[string]any)
	if !ok {
		t.Fatalf("expected notes to be a per-scripture map, got %#v", out["notes"])
	}
	quranNotes, ok := notes["quran"].([]map[string]any)
	if !ok || len(quranNotes) == 0 {
		t.Fatalf("expected at least one quran note, got %#v", notes["quran"])
	}
	if quranNotes[0]["text"] != "my export note" {
		t.Fatalf("expected friendly note key 'text', got %#v", quranNotes[0])
	}
	if quranNotes[0]["verse"] != "1:1" {
		t.Fatalf("expected friendly note key 'verse', got %#v", quranNotes[0])
	}

	// Every section the aggregator promises should be present (null on failure,
	// never missing), so the export shape is stable for consumers.
	for _, key := range []string{
		"bookmark_folders", "saved_verses", "cover_to_cover_position", "collections",
		"reading_progress", "reading_streaks", "activity_history", "reading_preferences",
	} {
		if _, present := out[key]; !present {
			t.Fatalf("expected section %q to be present in export", key)
		}
	}

	// Gameplay data is intentionally excluded from the export.
	for _, key := range []string{"game_attempts", "game_stats", "games"} {
		if _, present := out[key]; present {
			t.Fatalf("gameplay section %q must not appear in export", key)
		}
	}
}
