package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// DeletionCategories is the canonical set of user-data categories that can be
// erased through the "delete my content" flow. Preferences and the activity
// consent flag are deliberately absent: they are functional settings, not
// erasable content.
var DeletionCategories = []string{
	"bookmarks",
	"notes",
	"collections",
	"activity",
	"reading_progress",
	"games",
}

// IsValidDeletionCategory reports whether name is an erasable category.
func IsValidDeletionCategory(name string) bool {
	return slices.Contains(DeletionCategories, name)
}

type UserDataDeletionRow struct {
	ID          int64            `db:"id" json:"id"`
	UserID      int              `db:"user_id" json:"user_id"`
	Status      string           `db:"status" json:"status"`
	Categories  []string         `db:"categories" json:"categories"`
	Counts      *json.RawMessage `db:"counts" json:"counts,omitempty"`
	Error       *string          `db:"error" json:"error,omitempty"`
	RequestedAt time.Time        `db:"requested_at" json:"requested_at"`
	CompletedAt *time.Time       `db:"completed_at" json:"completed_at,omitempty"`
}

const deletionCols = `id, user_id, status, categories, counts, error, requested_at, completed_at`

func (db *DB) CreateQueuedDeletion(ctx context.Context, userID int, categories []string) (*UserDataDeletionRow, error) {
	rows, err := db.Pool.Query(ctx, `INSERT INTO user_data_deletions (user_id, status, categories)
		VALUES ($1, 'queued', $2)
		RETURNING `+deletionCols, userID, categories)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[UserDataDeletionRow])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (db *DB) LatestDeletion(ctx context.Context, userID int) (*UserDataDeletionRow, error) {
	rows, err := db.Pool.Query(ctx, `SELECT `+deletionCols+`
		FROM user_data_deletions WHERE user_id = $1 ORDER BY requested_at DESC LIMIT 1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[UserDataDeletionRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// FindDeletionBlocker returns an in-flight (queued/running) deletion for the
// user, if any, so callers can refuse to start a second one concurrently.
func (db *DB) FindDeletionBlocker(ctx context.Context, userID int) (*UserDataDeletionRow, error) {
	rows, err := db.Pool.Query(ctx, `SELECT `+deletionCols+`
		FROM user_data_deletions
		WHERE user_id = $1 AND status IN ('queued','running')
		ORDER BY requested_at DESC LIMIT 1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[UserDataDeletionRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (db *DB) SetDeletionRunning(ctx context.Context, id int64) error {
	_, err := db.Pool.Exec(ctx, `UPDATE user_data_deletions SET status = 'running' WHERE id = $1`, id)
	return err
}

func (db *DB) SetDeletionDone(ctx context.Context, id int64, counts map[string]int64) error {
	raw, err := json.Marshal(counts)
	if err != nil {
		return err
	}
	_, err = db.Pool.Exec(ctx, `UPDATE user_data_deletions
		SET status = 'done', counts = $2, completed_at = now(), error = NULL
		WHERE id = $1`, id, raw)
	return err
}

func (db *DB) SetDeletionFailed(ctx context.Context, id int64, errMsg string) error {
	_, err := db.Pool.Exec(ctx, `UPDATE user_data_deletions
		SET status = 'failed', error = $2, completed_at = now() WHERE id = $1`, id, errMsg)
	return err
}

// PurgeUserCategories erases the selected categories of a user's data inside a
// single transaction, returning the number of rows removed per category. An
// unknown category is rejected before any deletion runs. The whole operation is
// atomic: any failure rolls back every delete.
func (db *DB) PurgeUserCategories(ctx context.Context, authID string, categories []string) (map[string]int64, error) {
	for _, c := range categories {
		if !IsValidDeletionCategory(c) {
			return nil, fmt.Errorf("unknown deletion category %q", c)
		}
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var userID int
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE auth_id = $1`, authID).Scan(&userID); err != nil {
		return nil, fmt.Errorf("resolve user: %w", err)
	}

	exec := func(sql string, args ...any) (int64, error) {
		tag, err := tx.Exec(ctx, sql, args...)
		if err != nil {
			return 0, err
		}
		return tag.RowsAffected(), nil
	}

	counts := make(map[string]int64, len(categories))
	for _, category := range categories {
		var total int64
		switch category {
		case "bookmarks":
			// Folders cascade to their entries; saved verses exclude the
			// cover-to-cover position (that belongs to reading_progress).
			n, err := exec(`DELETE FROM bookmark_categories WHERE user_id = $1`, userID)
			if err != nil {
				return nil, err
			}
			total += n
			n, err = exec(`DELETE FROM bookmarks WHERE user_id = $1 AND kind <> 'cover_to_cover'`, userID)
			if err != nil {
				return nil, err
			}
			total += n

		case "notes":
			n, err := exec(`DELETE FROM notes WHERE user_id = $1`, userID)
			if err != nil {
				return nil, err
			}
			total += n

		case "collections":
			// Subscriptions to other users' collections, then owned
			// collections (their verses cascade).
			n, err := exec(`DELETE FROM collection_subscriptions WHERE user_id = $1`, userID)
			if err != nil {
				return nil, err
			}
			total += n
			n, err = exec(`DELETE FROM collections WHERE user_id = $1`, userID)
			if err != nil {
				return nil, err
			}
			total += n

		case "activity":
			n, err := exec(`DELETE FROM user_activity WHERE user_id = $1`, userID)
			if err != nil {
				return nil, err
			}
			total += n

		case "reading_progress":
			for _, q := range []string{
				`DELETE FROM reading_progress WHERE user_id = $1`,
				`DELETE FROM reading_log WHERE user_id = $1`,
				`DELETE FROM reading_events WHERE user_id = $1`,
				`DELETE FROM bookmarks WHERE user_id = $1 AND kind = 'cover_to_cover'`,
			} {
				n, err := exec(q, userID)
				if err != nil {
					return nil, err
				}
				total += n
			}

		case "games":
			// game_blank_attempts has no FK cascade from game_sessions, so it
			// must be cleared first to avoid orphans. Sessions key off auth_id.
			n, err := exec(`DELETE FROM game_blank_attempts
				WHERE session_id IN (SELECT session_id FROM game_sessions WHERE auth_id = $1)`, authID)
			if err != nil {
				return nil, err
			}
			total += n
			n, err = exec(`DELETE FROM game_sessions WHERE auth_id = $1`, authID)
			if err != nil {
				return nil, err
			}
			total += n
			n, err = exec(`DELETE FROM game_attempts WHERE user_id = $1`, userID)
			if err != nil {
				return nil, err
			}
			total += n
		}
		counts[category] = total
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return counts, nil
}

// PurgeUserAccount erases a user's entire account: every personal record plus
// the users row itself, in a single atomic transaction. Any failure rolls back
// the whole operation. It returns per-table counts of the rows explicitly
// removed (the FK-cascaded tables clean up when the users row is deleted and are
// not counted individually).
//
// Two classes of rows need explicit handling before the users row can go:
//
//   - auth_id-keyed tables have no FK to users(id), so they never cascade:
//     game_sessions, game_blank_attempts (orphans, no real FK), push_subscriptions,
//     and fcm_devices.
//   - editorial audit rows reference users(id) with NO "on delete" clause, so a
//     dangling reference would block the delete. Nullable references are detached
//     (SET NULL) to preserve the org content; NOT NULL audit rows the user
//     authored are removed. Published editorial content lives in separate tables
//     that do not reference users and is left untouched.
//
// Everything else (bookmarks, notes, collections, reading data, preferences,
// activity, game_attempts/variants, exports, deletions, idempotency keys, editor
// grants, …) references users(id) ON DELETE CASCADE and is cleaned by the final
// DELETE FROM users.
func (db *DB) PurgeUserAccount(ctx context.Context, authID string) (map[string]int64, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var userID int
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE auth_id = $1`, authID).Scan(&userID); err != nil {
		return nil, fmt.Errorf("resolve user: %w", err)
	}

	exec := func(sql string, args ...any) (int64, error) {
		tag, err := tx.Exec(ctx, sql, args...)
		if err != nil {
			return 0, err
		}
		return tag.RowsAffected(), nil
	}

	counts := make(map[string]int64, 8)

	// Editorial audit rows that would otherwise block the users delete. Detach
	// what is nullable, remove the NOT NULL rows the user authored.
	if _, err := exec(`UPDATE quran_publish_requests SET reviewed_by = NULL WHERE reviewed_by = $1`, userID); err != nil {
		return nil, err
	}
	if _, err := exec(`UPDATE quran_root_meaning_drafts SET author_id = NULL WHERE author_id = $1`, userID); err != nil {
		return nil, err
	}
	// changelog before publish_requests: changelog.publish_request_id is ON
	// DELETE SET NULL, so removing the user's requests can't strand it, but
	// clearing the user's own audit rows first keeps the intent explicit.
	n, err := exec(`DELETE FROM quran_editorial_changelog WHERE actor_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	counts["editorial_changelog"] = n
	n, err = exec(`DELETE FROM quran_publish_requests WHERE requested_by = $1`, userID)
	if err != nil {
		return nil, err
	}
	counts["publish_requests"] = n

	// auth_id-keyed tables (no FK to users, never cascade).
	n, err = exec(`DELETE FROM game_blank_attempts
		WHERE session_id IN (SELECT session_id FROM game_sessions WHERE auth_id = $1)`, authID)
	if err != nil {
		return nil, err
	}
	counts["game_blank_attempts"] = n
	n, err = exec(`DELETE FROM game_sessions WHERE auth_id = $1`, authID)
	if err != nil {
		return nil, err
	}
	counts["game_sessions"] = n
	n, err = exec(`DELETE FROM push_subscriptions WHERE auth_id = $1`, authID)
	if err != nil {
		return nil, err
	}
	counts["push_subscriptions"] = n
	n, err = exec(`DELETE FROM fcm_devices WHERE auth_id = $1`, authID)
	if err != nil {
		return nil, err
	}
	counts["fcm_devices"] = n

	// The account itself — cascades every remaining personal table.
	n, err = exec(`DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		return nil, err
	}
	counts["users"] = n

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return counts, nil
}
