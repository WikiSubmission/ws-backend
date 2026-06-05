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
