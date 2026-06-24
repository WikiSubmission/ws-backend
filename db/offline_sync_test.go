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

// An equal client timestamp must apply (the LWW guard is updated_at <= client_ts),
// so a re-sent identical-clock edit is not spuriously dropped as superseded.
func TestUpsertNoteLWWEqualTimestampApplies(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	authID, _ := seedSyncUser(t, db)

	ts := time.Now().UnixMilli()
	if applied, err := db.UpsertNoteLWW(ctx, authID, "quran", "1:1", "first", nil, ts); err != nil || !applied {
		t.Fatalf("first write: applied=%v err=%v", applied, err)
	}
	applied, err := db.UpsertNoteLWW(ctx, authID, "quran", "1:1", "second", nil, ts)
	if err != nil || !applied {
		t.Fatalf("equal-timestamp write should apply: applied=%v err=%v", applied, err)
	}
	notes, err := db.FetchNotes(ctx, authID, "quran", strPtr("1:1"))
	if err != nil || len(notes) != 1 || notes[0].Content != "second" {
		t.Fatalf("expected content 'second', got %+v err=%v", notes, err)
	}
}

// Tags round-trip through upsert: a set then a clear (nil -> empty array).
func TestUpsertNoteLWWTagsRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	authID, _ := seedSyncUser(t, db)

	now := time.Now().UnixMilli()
	if _, err := db.UpsertNoteLWW(ctx, authID, "quran", "1:1", "c", []string{"a", "b"}, now); err != nil {
		t.Fatalf("upsert with tags: %v", err)
	}
	notes, err := db.FetchNotes(ctx, authID, "quran", strPtr("1:1"))
	if err != nil || len(notes) != 1 {
		t.Fatalf("fetch after set: %+v err=%v", notes, err)
	}
	if len(notes[0].Tags) != 2 || notes[0].Tags[0] != "a" || notes[0].Tags[1] != "b" {
		t.Fatalf("expected tags [a b], got %v", notes[0].Tags)
	}

	// A later nil-tags upsert clears the tags to an empty array.
	if _, err := db.UpsertNoteLWW(ctx, authID, "quran", "1:1", "c", nil, now+1); err != nil {
		t.Fatalf("upsert clearing tags: %v", err)
	}
	notes, err = db.FetchNotes(ctx, authID, "quran", strPtr("1:1"))
	if err != nil || len(notes) != 1 {
		t.Fatalf("fetch after clear: %+v err=%v", notes, err)
	}
	if len(notes[0].Tags) != 0 {
		t.Fatalf("expected tags cleared, got %v", notes[0].Tags)
	}
}

// Deleting by natural key is scoped to the calling user: another user's entry at
// the same scripture/verse in their own category must be untouched.
func TestDeleteBookmarkEntryByVerseCrossUserIsolation(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	authA, catA := seedSyncUser(t, db)
	authB, catB := seedSyncUser(t, db)

	if _, err := db.CreateBookmarkEntry(ctx, catA, authA, "quran", "2:255"); err != nil {
		t.Fatalf("A create: %v", err)
	}
	if _, err := db.CreateBookmarkEntry(ctx, catB, authB, "quran", "2:255"); err != nil {
		t.Fatalf("B create: %v", err)
	}

	// B deletes its own entry; A's must remain.
	deleted, err := db.DeleteBookmarkEntryByVerse(ctx, authB, catB, "quran", "2:255")
	if err != nil || !deleted {
		t.Fatalf("B delete own: deleted=%v err=%v", deleted, err)
	}
	entriesA, err := db.ListBookmarkCategoryEntries(ctx, catA, authA)
	if err != nil || len(entriesA) != 1 {
		t.Fatalf("A entry should survive B's delete, got %+v err=%v", entriesA, err)
	}

	// B attempting to delete via A's category id is a scoped no-op.
	deleted, err = db.DeleteBookmarkEntryByVerse(ctx, authB, catA, "quran", "2:255")
	if err != nil || deleted {
		t.Fatalf("cross-user delete must be a no-op: deleted=%v err=%v", deleted, err)
	}
	entriesA, err = db.ListBookmarkCategoryEntries(ctx, catA, authA)
	if err != nil || len(entriesA) != 1 {
		t.Fatalf("A entry must still survive cross-user delete attempt, got %+v err=%v", entriesA, err)
	}
}

func strPtr(s string) *string { return &s }
