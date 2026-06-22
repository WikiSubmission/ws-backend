package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// CheckIdempotency returns the recorded outcome for a prior mutation key (scoped
// to the user) and whether it was found. Used by POST /me/sync to make replays
// safe.
func (db *DB) CheckIdempotency(ctx context.Context, authID, key string) (string, bool, error) {
	var status string
	err := db.Pool.QueryRow(ctx,
		`SELECT ik.status FROM idempotency_keys ik
		 JOIN users u ON u.id = ik.user_id
		 WHERE ik.key = $1 AND u.auth_id = $2`, key, authID).Scan(&status)
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return status, true, nil
}

// RecordIdempotency stores the outcome of a processed mutation key. Idempotent
// itself (ON CONFLICT DO NOTHING) so concurrent replays cannot error.
func (db *DB) RecordIdempotency(ctx context.Context, authID, key, status string) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO idempotency_keys (key, user_id, status)
		 SELECT $1, u.id, $3 FROM users u WHERE u.auth_id = $2
		 ON CONFLICT (key) DO NOTHING`, key, authID, status)
	return err
}

// CleanupIdempotencyKeys deletes keys older than the retention window.
func (db *DB) CleanupIdempotencyKeys(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM idempotency_keys WHERE created_at < now() - make_interval(secs => $1)`,
		olderThan.Seconds())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// DeleteBookmarkEntryByVerse removes a bookmark entry by natural key
// (category + scripture + verse). Returns true if a row was deleted; false means
// it was already absent, which is still a successful no-op for sync.
func (db *DB) DeleteBookmarkEntryByVerse(ctx context.Context, authID string, categoryID int64, scripture, verseKey string) (bool, error) {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM bookmark_entries
		 USING bookmark_categories bc, users u
		 WHERE bookmark_entries.category_id = $1
		   AND bookmark_entries.scripture = $3
		   AND bookmark_entries.verse_key = $4
		   AND bookmark_entries.category_id = bc.id
		   AND bc.user_id = u.id
		   AND u.auth_id = $2`, categoryID, authID, scripture, verseKey)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// DeleteNoteByVerse removes a user's note for a verse by natural key.
func (db *DB) DeleteNoteByVerse(ctx context.Context, authID, scripture, verseKey string) (bool, error) {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM notes
		 USING users u
		 WHERE notes.user_id = u.id AND u.auth_id = $1
		   AND notes.scripture = $2 AND notes.verse_key = $3`, authID, scripture, verseKey)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// UpsertNoteLWW upserts a note, keeping the existing row when its updated_at is
// newer than the client's timestamp (last-write-wins across the user's devices).
// Returns true when the write was applied, false when the server copy was newer
// (the mutation is superseded and the client should drop it).
func (db *DB) UpsertNoteLWW(ctx context.Context, authID, scripture, verseKey, content string, tags []string, clientTsMs int64) (bool, error) {
	if tags == nil {
		tags = []string{}
	}
	ts := time.UnixMilli(clientTsMs).UTC()
	var id int64
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO notes (user_id, scripture, verse_key, content, tags, updated_at)
		 SELECT u.id, $2, $3, $4, $5, $6 FROM users u WHERE u.auth_id = $1
		 ON CONFLICT (user_id, scripture, verse_key) DO UPDATE
		   SET content = EXCLUDED.content, tags = EXCLUDED.tags, updated_at = EXCLUDED.updated_at
		   WHERE notes.updated_at <= EXCLUDED.updated_at
		 RETURNING id`, authID, scripture, verseKey, content, tags, ts).Scan(&id)
	if err == pgx.ErrNoRows {
		return false, nil // server copy is newer → superseded
	}
	if err != nil {
		return false, fmt.Errorf("upsert note lww: %w", err)
	}
	return true, nil
}
