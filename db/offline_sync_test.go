package db

import (
	"context"
	"testing"
	"time"
)

// seedSyncUser creates an isolated user + one bookmark category, returning the
// authID and category id. Cleans up on test end.
func seedSyncUser(t *testing.T, db *DB) (string, int64) {
	t.Helper()
	ctx := context.Background()
	authID := "test-sync-" + time.Now().Format("150405.000000000")
	if _, err := db.UpsertUser(ctx, authID, authID+"@example.com", nil); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM users WHERE auth_id = $1`, authID)
	})
	cat, err := db.CreateBookmarkCategory(ctx, authID, "Test", "amber")
	if err != nil {
		t.Fatalf("CreateBookmarkCategory: %v", err)
	}
	return authID, cat.ID
}

func TestIdempotencyRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	authID, _ := seedSyncUser(t, db)

	if _, found, err := db.CheckIdempotency(ctx, authID, "key-1"); err != nil || found {
		t.Fatalf("expected unseen key, found=%v err=%v", found, err)
	}
	if err := db.RecordIdempotency(ctx, authID, "key-1", "applied"); err != nil {
		t.Fatalf("RecordIdempotency: %v", err)
	}
	status, found, err := db.CheckIdempotency(ctx, authID, "key-1")
	if err != nil || !found || status != "applied" {
		t.Fatalf("expected applied/found, got %q found=%v err=%v", status, found, err)
	}
	// Re-record is a no-op, never an error.
	if err := db.RecordIdempotency(ctx, authID, "key-1", "applied"); err != nil {
		t.Fatalf("re-record: %v", err)
	}
	// Keys are scoped per user.
	other, _ := seedSyncUser(t, db)
	if _, found, _ := db.CheckIdempotency(ctx, other, "key-1"); found {
		t.Fatal("idempotency key leaked across users")
	}
}

func TestDeleteBookmarkEntryByVerse(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	authID, catID := seedSyncUser(t, db)

	if _, err := db.CreateBookmarkEntry(ctx, catID, authID, "quran", "2:255"); err != nil {
		t.Fatalf("CreateBookmarkEntry: %v", err)
	}
	deleted, err := db.DeleteBookmarkEntryByVerse(ctx, authID, catID, "quran", "2:255")
	if err != nil || !deleted {
		t.Fatalf("expected delete, deleted=%v err=%v", deleted, err)
	}
	// Deleting an absent entry is a successful no-op.
	deleted, err = db.DeleteBookmarkEntryByVerse(ctx, authID, catID, "quran", "2:255")
	if err != nil || deleted {
		t.Fatalf("expected no-op, deleted=%v err=%v", deleted, err)
	}
}

func TestUpsertNoteLWW(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	authID, _ := seedSyncUser(t, db)

	now := time.Now().UnixMilli()

	// First write applies.
	applied, err := db.UpsertNoteLWW(ctx, authID, "quran", "1:1", "first", nil, now)
	if err != nil || !applied {
		t.Fatalf("first upsert: applied=%v err=%v", applied, err)
	}
	// An older mutation is superseded by the newer server copy.
	applied, err = db.UpsertNoteLWW(ctx, authID, "quran", "1:1", "stale", nil, now-10_000)
	if err != nil {
		t.Fatalf("stale upsert err: %v", err)
	}
	if applied {
		t.Fatal("expected stale write to be superseded")
	}
	// A newer mutation wins.
	applied, err = db.UpsertNoteLWW(ctx, authID, "quran", "1:1", "newer", nil, now+10_000)
	if err != nil || !applied {
		t.Fatalf("newer upsert: applied=%v err=%v", applied, err)
	}

	notes, err := db.FetchNotes(ctx, authID, "quran", strPtr("1:1"))
	if err != nil {
		t.Fatalf("FetchNotes: %v", err)
	}
	if len(notes) != 1 || notes[0].Content != "newer" {
		t.Fatalf("expected content 'newer', got %+v", notes)
	}

	// Delete by natural key.
	deleted, err := db.DeleteNoteByVerse(ctx, authID, "quran", "1:1")
	if err != nil || !deleted {
		t.Fatalf("DeleteNoteByVerse: deleted=%v err=%v", deleted, err)
	}
}

func strPtr(s string) *string { return &s }
