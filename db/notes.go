package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type NoteRow struct {
	ID        int64     `db:"id"`
	UserID    int       `db:"user_id"`
	Scripture string    `db:"scripture"`
	VerseKey  string    `db:"verse_key"`
	Content   string    `db:"content"`
	Tags      []string  `db:"tags"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

const noteCols = `n.id, n.user_id, n.scripture, n.verse_key, n.content, n.tags, n.created_at, n.updated_at`

func (db *DB) FetchNotes(ctx context.Context, authID, scripture string, verseKey *string) ([]NoteRow, error) {
	query := `SELECT ` + noteCols + `
	          FROM notes n
	          JOIN users u ON u.id = n.user_id
	          WHERE u.auth_id = $1 AND n.scripture = $2`
	args := []any{authID, scripture}
	if verseKey != nil {
		args = append(args, *verseKey)
		query += fmt.Sprintf(" AND n.verse_key = $%d", len(args))
	}
	query += ` ORDER BY n.created_at DESC`
	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[NoteRow])
}

// UpsertNote creates or updates the single note for a (user, scripture, verse_key) tuple.
// When tags is nil, existing tags on the row are left unchanged (only content is updated).
// When tags is non-nil (including empty slice), it replaces the existing tags.
func (db *DB) UpsertNote(ctx context.Context, authID, scripture, verseKey, content string, tags []string) (*NoteRow, error) {
	query := `INSERT INTO notes (user_id, scripture, verse_key, content, tags)
	          SELECT u.id, $2, $3, $4, COALESCE($5::text[], '{}'::text[])
	          FROM users u WHERE u.auth_id = $1
	          ON CONFLICT (user_id, scripture, verse_key)
	          DO UPDATE SET
	              content = EXCLUDED.content,
	              tags = CASE WHEN $5::text[] IS NULL THEN notes.tags ELSE EXCLUDED.tags END,
	              updated_at = NOW()
	          RETURNING id, user_id, scripture, verse_key, content, tags, created_at, updated_at`
	rows, err := db.Pool.Query(ctx, query, authID, scripture, verseKey, content, tags)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	n, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[NoteRow])
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (db *DB) DeleteNote(ctx context.Context, id int64, authID string) (bool, error) {
	query := `DELETE FROM notes
	          USING users u
	          WHERE notes.id = $1 AND u.auth_id = $2 AND notes.user_id = u.id`
	tag, err := db.Pool.Exec(ctx, query, id, authID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (db *DB) FetchChapterNotes(ctx context.Context, authID, scripture string, chapter int) ([]NoteRow, error) {
	prefix := fmt.Sprintf("%d:%%", chapter)
	query := `SELECT ` + noteCols + `
	          FROM notes n
	          JOIN users u ON u.id = n.user_id
	          WHERE u.auth_id = $1 AND n.scripture = $2 AND n.verse_key LIKE $3
	          ORDER BY n.verse_key`
	rows, err := db.Pool.Query(ctx, query, authID, scripture, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[NoteRow])
}

type NoteSearchResult struct {
	VerseKey  string `db:"verse_key"`
	Scripture string `db:"scripture"`
	Excerpt   string `db:"excerpt"`
}

// SearchNotes performs full-text search against the user's notes content.
// Returns verse_key, scripture, and a highlighted excerpt for each match.
func (db *DB) SearchNotes(ctx context.Context, authID, query string, scripture *string) ([]NoteSearchResult, error) {
	baseQuery := `SELECT n.verse_key, n.scripture,
	                     ts_headline('english', n.content, plainto_tsquery('english', $2),
	                                 'MaxWords=15, MinWords=5, ShortWord=3') AS excerpt
	              FROM notes n
	              JOIN users u ON u.id = n.user_id
	              WHERE u.auth_id = $1
	                AND n.content_tsv @@ plainto_tsquery('english', $2)`
	args := []any{authID, query}
	if scripture != nil {
		args = append(args, *scripture)
		baseQuery += fmt.Sprintf(" AND n.scripture = $%d", len(args))
	}
	baseQuery += ` ORDER BY ts_rank(n.content_tsv, plainto_tsquery('english', $2)) DESC LIMIT 20`

	rows, err := db.Pool.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[NoteSearchResult])
}
