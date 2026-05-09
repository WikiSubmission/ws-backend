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
	Lang      string    `db:"lang"`
	Content   string    `db:"content"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

const noteCols = `n.id, n.user_id, n.scripture, n.verse_key, n.lang, n.content, n.created_at, n.updated_at`

func (db *DB) FetchNotes(ctx context.Context, authID, scripture string, verseKey, lang *string) ([]NoteRow, error) {
	query := `SELECT ` + noteCols + `
	          FROM notes n
	          JOIN users u ON u.id = n.user_id
	          WHERE u.auth_id = $1 AND n.scripture = $2`
	args := []any{authID, scripture}
	if verseKey != nil {
		args = append(args, *verseKey)
		query += fmt.Sprintf(" AND n.verse_key = $%d", len(args))
	}
	if lang != nil {
		args = append(args, *lang)
		query += fmt.Sprintf(" AND n.lang = $%d", len(args))
	}
	query += ` ORDER BY n.created_at DESC`
	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[NoteRow])
}

func (db *DB) CreateNote(ctx context.Context, authID, scripture, verseKey, lang, content string) (*NoteRow, error) {
	query := `INSERT INTO notes (user_id, scripture, verse_key, lang, content)
	          SELECT u.id, $2, $3, $4, $5 FROM users u WHERE u.auth_id = $1
	          RETURNING id, user_id, scripture, verse_key, lang, content, created_at, updated_at`
	rows, err := db.Pool.Query(ctx, query, authID, scripture, verseKey, lang, content)
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

func (db *DB) UpdateNote(ctx context.Context, id int64, authID, content string) (*NoteRow, error) {
	query := `UPDATE notes SET content = $3, updated_at = NOW()
	          FROM users u
	          WHERE notes.id = $1 AND u.auth_id = $2 AND notes.user_id = u.id
	          RETURNING notes.id, notes.user_id, notes.scripture, notes.verse_key,
	                    notes.lang, notes.content, notes.created_at, notes.updated_at`
	rows, err := db.Pool.Query(ctx, query, id, authID, content)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	n, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[NoteRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
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
	          ORDER BY n.verse_key, n.lang, n.created_at DESC`
	rows, err := db.Pool.Query(ctx, query, authID, scripture, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[NoteRow])
}
