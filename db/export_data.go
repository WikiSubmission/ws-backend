package db

import (
	"context"
	"log/slog"
	"time"
)

// exportSectionCap bounds the number of rows pulled for the unbounded,
// append-only sections (activity log, game attempts). Real accounts stay well
// below this; hitting it is logged so a truncated export is never silent.
const exportSectionCap = 100000

// exportScriptures is the set of scriptures every scripture-scoped section is
// gathered for, so the export covers both Quran and Bible data.
var exportScriptures = []string{"quran", "bible"}

// BuildUserExport gathers all stored data for a user into a JSON-serialisable
// map for the data export. It is best-effort: a failure fetching one section is
// logged and that section is left null rather than failing the whole export, so
// a single broken query cannot deny the user the rest of their data.
func (db *DB) BuildUserExport(ctx context.Context, userID int, authID, email string) map[string]any {
	out := map[string]any{
		"profile":     map[string]any{"user_id": userID, "auth_id": authID, "email": email},
		"exported_at": time.Now().UTC().Format(time.RFC3339),
	}

	section := func(name string, fn func() (any, error)) {
		v, err := fn()
		if err != nil {
			slog.Error("me-export section failed", "section", name, "auth_id", authID, "error", err)
			out[name] = nil
			return
		}
		out[name] = v
	}

	// Helper for scripture-scoped sections: returns a {quran:..., bible:...} map.
	byScripture := func(fetch func(scripture string) (any, error)) (any, error) {
		m := map[string]any{}
		for _, s := range exportScriptures {
			v, err := fetch(s)
			if err != nil {
				return nil, err
			}
			m[s] = v
		}
		return m, nil
	}

	section("consent", func() (any, error) {
		return db.GetActivityConsent(ctx, authID)
	})

	section("preferences", func() (any, error) {
		return byScripture(func(s string) (any, error) { return db.GetPreferences(ctx, authID, s) })
	})

	section("bookmark_categories", func() (any, error) {
		cats, err := db.ListBookmarkCategories(ctx, authID)
		if err != nil {
			return nil, err
		}
		type categoryExport struct {
			Category BookmarkCategoryRow `json:"category"`
			Entries  []BookmarkEntryRow  `json:"entries"`
		}
		result := make([]categoryExport, 0, len(cats))
		for _, c := range cats {
			entries, err := db.ListBookmarkCategoryEntries(ctx, c.ID, authID)
			if err != nil {
				return nil, err
			}
			result = append(result, categoryExport{Category: c, Entries: entries})
		}
		return result, nil
	})

	section("bookmarks", func() (any, error) {
		return byScripture(func(s string) (any, error) { return db.FetchBookmarks(ctx, authID, s) })
	})

	section("cover_to_cover", func() (any, error) {
		return byScripture(func(s string) (any, error) { return db.FetchCoverToCover(ctx, authID, s) })
	})

	section("notes", func() (any, error) {
		return byScripture(func(s string) (any, error) { return db.FetchNotes(ctx, authID, s, nil) })
	})

	section("collections", func() (any, error) {
		cols, err := db.ListCollections(ctx, authID)
		if err != nil {
			return nil, err
		}
		type collectionExport struct {
			Collection CollectionRow        `json:"collection"`
			Verses     []CollectionVerseRow `json:"verses"`
		}
		result := make([]collectionExport, 0, len(cols))
		for _, c := range cols {
			verses, err := db.ListCollectionVerses(ctx, c.ID)
			if err != nil {
				return nil, err
			}
			result = append(result, collectionExport{Collection: c, Verses: verses})
		}
		return result, nil
	})

	section("reading_progress", func() (any, error) {
		return byScripture(func(s string) (any, error) { return db.GetReadingProgress(ctx, authID, s) })
	})

	section("reading_streaks", func() (any, error) {
		return byScripture(func(s string) (any, error) { return db.GetStreak(ctx, authID, s) })
	})

	section("activity", func() (any, error) {
		rows, err := db.ListActivity(ctx, authID, exportSectionCap, 0)
		if err != nil {
			return nil, err
		}
		if len(rows) == exportSectionCap {
			slog.Warn("me-export activity truncated at cap", "auth_id", authID, "cap", exportSectionCap)
		}
		return rows, nil
	})

	section("game_attempts", func() (any, error) {
		rows, err := db.FetchUserAttempts(ctx, authID, exportSectionCap, nil)
		if err != nil {
			return nil, err
		}
		if len(rows) == exportSectionCap {
			slog.Warn("me-export game attempts truncated at cap", "auth_id", authID, "cap", exportSectionCap)
		}
		return rows, nil
	})

	section("game_stats", func() (any, error) {
		return db.FetchUserGameStats(ctx, authID)
	})

	return out
}
