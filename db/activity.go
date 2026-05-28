package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// ActivityRow is one entry in the per-user activity feed.
type ActivityRow struct {
	ID        int64     `db:"id"         json:"id"`
	Kind      string    `db:"kind"       json:"kind"`
	Scripture string    `db:"scripture"  json:"scripture"`
	VerseKey  *string   `db:"verse_key"  json:"verse_key,omitempty"`
	Query     *string   `db:"query"      json:"query,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// GetActivityConsent returns the current consent flag for the user. A missing
// user row reports consent as false so callers can safely default to drop.
func (db *DB) GetActivityConsent(ctx context.Context, authID string) (bool, error) {
	const q = `SELECT activity_consent FROM users WHERE auth_id = $1`
	var consent bool
	err := db.Pool.QueryRow(ctx, q, authID).Scan(&consent)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return consent, nil
}

// SetActivityConsent flips the consent flag. Returns the new value.
func (db *DB) SetActivityConsent(ctx context.Context, authID string, consent bool) (bool, error) {
	const q = `UPDATE users SET activity_consent = $2, updated_at = now() WHERE auth_id = $1 RETURNING activity_consent`
	var v bool
	if err := db.Pool.QueryRow(ctx, q, authID, consent).Scan(&v); err != nil {
		return false, err
	}
	return v, nil
}

// InsertActivity records one activity event for the user. Returns false (with
// nil error) if consent is currently off, so handlers can no-op silently.
func (db *DB) InsertActivity(ctx context.Context, authID, kind, scripture string, verseKey, query *string) (bool, error) {
	const q = `WITH uid AS (
	        SELECT id, activity_consent FROM users WHERE auth_id = $1
	    )
	    INSERT INTO user_activity (user_id, kind, scripture, verse_key, query)
	    SELECT id, $2, $3, $4, $5 FROM uid WHERE activity_consent = true
	    RETURNING id`
	var id int64
	err := db.Pool.QueryRow(ctx, q, authID, kind, scripture, verseKey, query).Scan(&id)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ListActivity returns the user's most recent activity, newest first.
func (db *DB) ListActivity(ctx context.Context, authID string, limit, offset int) ([]ActivityRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	const q = `SELECT a.id, a.kind, a.scripture, a.verse_key, a.query, a.created_at
	    FROM user_activity a
	    JOIN users u ON u.id = a.user_id
	    WHERE u.auth_id = $1
	    ORDER BY a.created_at DESC
	    LIMIT $2 OFFSET $3`
	rows, err := db.Pool.Query(ctx, q, authID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[ActivityRow])
}

// ClearActivity deletes every activity row for the user. Returns the number
// of rows removed.
func (db *DB) ClearActivity(ctx context.Context, authID string) (int64, error) {
	const q = `DELETE FROM user_activity a USING users u WHERE a.user_id = u.id AND u.auth_id = $1`
	tag, err := db.Pool.Exec(ctx, q, authID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
