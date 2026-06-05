package db

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

// exportSectionCap bounds the number of rows pulled for the unbounded,
// append-only sections (activity log). Real accounts stay well below this;
// hitting it is logged so a truncated export is never silent.
const exportSectionCap = 100000

// exportScriptures is the set of scriptures every scripture-scoped section is
// gathered for, so the export covers both Quran and Bible data.
var exportScriptures = []string{"quran", "bible"}

// rfc3339 renders a timestamp in the export's canonical format.
func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// BuildUserExport gathers a user's personal data into a JSON-serialisable map
// for the data export. Keys and field names are human-readable labels, not
// database column names, so the file makes sense to a non-technical reader.
//
// Gameplay data (attempts, stats) is intentionally excluded: it is not treated
// as private user data and is therefore neither exported nor part of the export.
//
// It is best-effort: a failure fetching one section is logged and that section
// is left null rather than failing the whole export, so a single broken query
// cannot deny the user the rest of their data.
func (db *DB) BuildUserExport(ctx context.Context, userID int, authID, email string) map[string]any {
	out := map[string]any{
		"account": map[string]any{
			"account_id": userID,
			"sign_in_id": authID,
			"email":      email,
		},
		"exported_at": rfc3339(time.Now()),
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

	section("activity_tracking_enabled", func() (any, error) {
		return db.GetActivityConsent(ctx, authID)
	})

	section("reading_preferences", func() (any, error) {
		return byScripture(func(s string) (any, error) {
			p, err := db.GetPreferences(ctx, authID, s)
			if err != nil || p == nil {
				return nil, err
			}
			// payload is stored as raw JSON; surface it as JSON, not base64.
			var settings any
			if len(p.Payload) > 0 {
				if jsonErr := json.Unmarshal(p.Payload, &settings); jsonErr != nil {
					settings = nil
				}
			}
			return map[string]any{
				"settings":   settings,
				"updated_at": rfc3339(p.UpdatedAt),
			}, nil
		})
	})

	section("bookmark_folders", func() (any, error) {
		cats, err := db.ListBookmarkCategories(ctx, authID)
		if err != nil {
			return nil, err
		}
		result := make([]map[string]any, 0, len(cats))
		for _, c := range cats {
			entries, err := db.ListBookmarkCategoryEntries(ctx, c.ID, authID)
			if err != nil {
				return nil, err
			}
			verses := make([]map[string]any, 0, len(entries))
			for _, e := range entries {
				verses = append(verses, map[string]any{
					"verse":     e.VerseKey,
					"scripture": e.Scripture,
					"added_at":  rfc3339(e.CreatedAt),
				})
			}
			result = append(result, map[string]any{
				"name":        c.Name,
				"color":       c.Color,
				"verse_count": c.EntryCount,
				"created_at":  rfc3339(c.CreatedAt),
				"verses":      verses,
			})
		}
		return result, nil
	})

	section("saved_verses", func() (any, error) {
		return byScripture(func(s string) (any, error) {
			rows, err := db.FetchBookmarks(ctx, authID, s)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(rows))
			for _, b := range rows {
				// The cover-to-cover reading position is stored as a bookmark
				// but belongs under reading progress, not saved verses.
				if b.Kind == "cover_to_cover" {
					continue
				}
				out = append(out, map[string]any{
					"verse":    b.VerseKey,
					"label":    b.Name,
					"color":    b.Color,
					"saved_at": rfc3339(b.CreatedAt),
				})
			}
			return out, nil
		})
	})

	section("cover_to_cover_position", func() (any, error) {
		return byScripture(func(s string) (any, error) {
			b, err := db.FetchCoverToCover(ctx, authID, s)
			if err != nil || b == nil {
				return nil, err
			}
			return map[string]any{
				"verse":      b.VerseKey,
				"updated_at": rfc3339(b.UpdatedAt),
			}, nil
		})
	})

	section("notes", func() (any, error) {
		return byScripture(func(s string) (any, error) {
			rows, err := db.FetchNotes(ctx, authID, s, nil)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(rows))
			for _, n := range rows {
				out = append(out, map[string]any{
					"verse":      n.VerseKey,
					"text":       n.Content,
					"tags":       n.Tags,
					"created_at": rfc3339(n.CreatedAt),
					"updated_at": rfc3339(n.UpdatedAt),
				})
			}
			return out, nil
		})
	})

	section("collections", func() (any, error) {
		cols, err := db.ListCollections(ctx, authID)
		if err != nil {
			return nil, err
		}
		result := make([]map[string]any, 0, len(cols))
		for _, c := range cols {
			verseRows, err := db.ListCollectionVerses(ctx, c.ID)
			if err != nil {
				return nil, err
			}
			verses := make([]map[string]any, 0, len(verseRows))
			for _, v := range verseRows {
				verses = append(verses, map[string]any{
					"verse":     v.VerseKey,
					"scripture": v.Scripture,
					"note":      v.Note,
					"position":  v.Position,
				})
			}
			visibility := "private"
			if c.IsPublic {
				visibility = "public"
			}
			result = append(result, map[string]any{
				"name":        c.Name,
				"description": c.Description,
				"visibility":  visibility,
				"relation":    c.Relation,
				"created_at":  rfc3339(c.CreatedAt),
				"verses":      verses,
			})
		}
		return result, nil
	})

	section("reading_progress", func() (any, error) {
		return byScripture(func(s string) (any, error) {
			p, err := db.GetReadingProgress(ctx, authID, s)
			if err != nil || p == nil {
				return nil, err
			}
			return map[string]any{
				"last_verse": p.VerseKey,
				"updated_at": rfc3339(p.UpdatedAt),
			}, nil
		})
	})

	section("reading_streaks", func() (any, error) {
		return byScripture(func(s string) (any, error) {
			st, err := db.GetStreak(ctx, authID, s)
			if err != nil || st == nil {
				return nil, err
			}
			return map[string]any{
				"current_streak":    st.CurrentStreak,
				"longest_streak":    st.LongestStreak,
				"last_active_day":   st.LastActiveDay,
				"total_verses_read": st.TotalVersesRead,
			}, nil
		})
	})

	section("activity_history", func() (any, error) {
		rows, err := db.ListActivity(ctx, authID, exportSectionCap, 0)
		if err != nil {
			return nil, err
		}
		if len(rows) == exportSectionCap {
			slog.Warn("me-export activity truncated at cap", "auth_id", authID, "cap", exportSectionCap)
		}
		out := make([]map[string]any, 0, len(rows))
		for _, a := range rows {
			entry := map[string]any{
				"type":        a.Kind,
				"scripture":   a.Scripture,
				"recorded_at": rfc3339(a.CreatedAt),
			}
			if a.VerseKey != nil {
				entry["verse"] = *a.VerseKey
			}
			if a.Query != nil {
				entry["search"] = *a.Query
			}
			out = append(out, entry)
		}
		return out, nil
	})

	return out
}
