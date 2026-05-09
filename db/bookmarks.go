package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type BookmarkRow struct {
	ID        int64     `db:"id"`
	UserID    int       `db:"user_id"`
	Scripture string    `db:"scripture"`
	VerseKey  string    `db:"verse_key"`
	Name      string    `db:"name"`
	Color     string    `db:"color"`
	Kind      string    `db:"kind"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

const bookmarkCols = `b.id, b.user_id, b.scripture, b.verse_key, b.name, b.color, b.kind, b.created_at, b.updated_at`

func (db *DB) FetchBookmarks(ctx context.Context, authID, scripture string) ([]BookmarkRow, error) {
	query := `SELECT ` + bookmarkCols + `
	          FROM bookmarks b
	          JOIN users u ON u.id = b.user_id
	          WHERE u.auth_id = $1 AND b.scripture = $2
	          ORDER BY b.created_at DESC`
	rows, err := db.Pool.Query(ctx, query, authID, scripture)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[BookmarkRow])
}

func (db *DB) CreateBookmark(ctx context.Context, authID, scripture, verseKey, name, color, kind string) (*BookmarkRow, error) {
	query := `INSERT INTO bookmarks (user_id, scripture, verse_key, name, color, kind)
	          SELECT u.id, $2, $3, $4, $5, $6 FROM users u WHERE u.auth_id = $1
	          RETURNING id, user_id, scripture, verse_key, name, color, kind, created_at, updated_at`
	rows, err := db.Pool.Query(ctx, query, authID, scripture, verseKey, name, color, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	b, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[BookmarkRow])
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (db *DB) UpdateBookmark(ctx context.Context, id int64, authID, name, color string) (*BookmarkRow, error) {
	query := `UPDATE bookmarks SET name = $3, color = $4, updated_at = NOW()
	          FROM users u
	          WHERE bookmarks.id = $1 AND u.auth_id = $2 AND bookmarks.user_id = u.id
	          RETURNING bookmarks.id, bookmarks.user_id, bookmarks.scripture, bookmarks.verse_key,
	                    bookmarks.name, bookmarks.color, bookmarks.kind,
	                    bookmarks.created_at, bookmarks.updated_at`
	rows, err := db.Pool.Query(ctx, query, id, authID, name, color)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	b, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[BookmarkRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

func (db *DB) DeleteBookmark(ctx context.Context, id int64, authID string) (bool, error) {
	query := `DELETE FROM bookmarks
	          USING users u
	          WHERE bookmarks.id = $1 AND u.auth_id = $2 AND bookmarks.user_id = u.id`
	tag, err := db.Pool.Exec(ctx, query, id, authID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (db *DB) UpsertCoverToCover(ctx context.Context, authID, scripture, verseKey string) (*BookmarkRow, error) {
	// Partial unique index: bookmarks_one_c2c ON bookmarks (user_id, scripture) WHERE kind = 'cover_to_cover'
	query := `WITH uid AS (SELECT id FROM users WHERE auth_id = $1)
	          INSERT INTO bookmarks (user_id, scripture, verse_key, name, color, kind)
	          SELECT id, $2, $3, 'Cover to Cover', 'blue', 'cover_to_cover' FROM uid
	          ON CONFLICT (user_id, scripture) WHERE kind = 'cover_to_cover'
	          DO UPDATE SET verse_key = EXCLUDED.verse_key, updated_at = NOW()
	          RETURNING id, user_id, scripture, verse_key, name, color, kind, created_at, updated_at`
	rows, err := db.Pool.Query(ctx, query, authID, scripture, verseKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	b, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[BookmarkRow])
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// FetchScriptureState returns all bookmarks for a specific chapter
// (verse_key matches "{chapter}:*"), used for the bulk-read endpoint.
func (db *DB) FetchChapterBookmarks(ctx context.Context, authID, scripture string, chapter int) ([]BookmarkRow, error) {
	prefix := fmt.Sprintf("%d:%%", chapter)
	query := `SELECT ` + bookmarkCols + `
	          FROM bookmarks b
	          JOIN users u ON u.id = b.user_id
	          WHERE u.auth_id = $1 AND b.scripture = $2 AND b.verse_key LIKE $3
	          ORDER BY b.verse_key`
	rows, err := db.Pool.Query(ctx, query, authID, scripture, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[BookmarkRow])
}
