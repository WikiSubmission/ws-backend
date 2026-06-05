package db

import (
	"context"
	"testing"
	"time"
)

func TestPurgeUserCategories_DeletesOnlySelected(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	authID := "test-purge-" + time.Now().Format("150405.000000")
	email := authID + "@example.com"
	if _, err := db.UpsertUser(ctx, authID, email, nil); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM users WHERE auth_id = $1`, authID)
	})

	// Activity requires consent before InsertActivity will store anything.
	if _, err := db.SetActivityConsent(ctx, authID, true); err != nil {
		t.Fatalf("SetActivityConsent: %v", err)
	}

	// Seed one row in each of several categories.
	if _, err := db.UpsertNote(ctx, authID, "quran", "1:1", "note to delete", nil); err != nil {
		t.Fatalf("UpsertNote: %v", err)
	}
	if _, err := db.CreateBookmark(ctx, authID, "quran", "2:255", "bm", "#fff", "normal"); err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}
	if _, err := db.UpsertCoverToCover(ctx, authID, "quran", "3:1"); err != nil {
		t.Fatalf("UpsertCoverToCover: %v", err)
	}
	if _, err := db.UpsertReadingProgress(ctx, authID, "quran", "4:1"); err != nil {
		t.Fatalf("UpsertReadingProgress: %v", err)
	}
	if ok, err := db.InsertActivity(ctx, authID, "search", "quran", nil, ptr("mercy")); err != nil || !ok {
		t.Fatalf("InsertActivity: ok=%v err=%v", ok, err)
	}

	// Purge only notes + bookmarks. Reading progress, cover-to-cover, and
	// activity must survive.
	counts, err := db.PurgeUserCategories(ctx, authID, []string{"notes", "bookmarks"})
	if err != nil {
		t.Fatalf("PurgeUserCategories: %v", err)
	}
	if counts["notes"] < 1 {
		t.Errorf("expected notes count >= 1, got %d", counts["notes"])
	}
	if counts["bookmarks"] < 1 {
		t.Errorf("expected bookmarks count >= 1, got %d", counts["bookmarks"])
	}

	// Notes gone.
	notes, err := db.FetchNotes(ctx, authID, "quran", nil)
	if err != nil {
		t.Fatalf("FetchNotes: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("expected notes deleted, got %d", len(notes))
	}

	// Saved bookmark gone, but cover-to-cover (reading_progress) preserved.
	c2c, err := db.FetchCoverToCover(ctx, authID, "quran")
	if err != nil {
		t.Fatalf("FetchCoverToCover: %v", err)
	}
	if c2c == nil {
		t.Error("cover-to-cover must NOT be deleted by the bookmarks category")
	}
	bms, err := db.FetchBookmarks(ctx, authID, "quran")
	if err != nil {
		t.Fatalf("FetchBookmarks: %v", err)
	}
	for _, b := range bms {
		if b.Kind == "normal" {
			t.Errorf("expected saved bookmark deleted, found %s", b.VerseKey)
		}
	}

	// Reading progress preserved.
	rp, err := db.GetReadingProgress(ctx, authID, "quran")
	if err != nil {
		t.Fatalf("GetReadingProgress: %v", err)
	}
	if rp == nil {
		t.Error("reading progress must survive when not selected")
	}

	// Activity preserved.
	acts, err := db.ListActivity(ctx, authID, 10, 0)
	if err != nil {
		t.Fatalf("ListActivity: %v", err)
	}
	if len(acts) == 0 {
		t.Error("activity must survive when not selected")
	}
}

func TestPurgeUserCategories_RejectsUnknown(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	if _, err := db.PurgeUserCategories(ctx, "whoever", []string{"notes", "bogus"}); err == nil {
		t.Fatal("expected error for unknown category, got nil")
	}
}

func ptr[T any](v T) *T { return &v }
