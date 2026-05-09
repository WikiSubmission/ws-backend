package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type BookmarkEntryRow struct {
	ID         int64     `db:"id"`
	CategoryID int64     `db:"category_id"`
	Scripture  string    `db:"scripture"`
	VerseKey   string    `db:"verse_key"`
	CreatedAt  time.Time `db:"created_at"`
}

const entryCols = `be.id, be.category_id, be.scripture, be.verse_key, be.created_at`

func (db *DB) ListBookmarkCategoryEntries(ctx context.Context, categoryID int64, authID string) ([]BookmarkEntryRow, error) {
	query := `SELECT ` + entryCols + `
	          FROM bookmark_entries be
	          JOIN bookmark_categories bc ON bc.id = be.category_id
	          JOIN users u ON u.id = bc.user_id
	          WHERE be.category_id = $1 AND u.auth_id = $2
	          ORDER BY be.created_at DESC`
	rows, err := db.Pool.Query(ctx, query, categoryID, authID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[BookmarkEntryRow])
}

func (db *DB) CreateBookmarkEntry(ctx context.Context, categoryID int64, authID, scripture, verseKey string) (*BookmarkEntryRow, error) {
	// Verify category belongs to user before inserting.
	query := `INSERT INTO bookmark_entries (category_id, scripture, verse_key)
	          SELECT bc.id, $3, $4
	          FROM bookmark_categories bc
	          JOIN users u ON u.id = bc.user_id
	          WHERE bc.id = $1 AND u.auth_id = $2
	          ON CONFLICT (category_id, scripture, verse_key) DO NOTHING
	          RETURNING id, category_id, scripture, verse_key, created_at`
	rows, err := db.Pool.Query(ctx, query, categoryID, authID, scripture, verseKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	e, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[BookmarkEntryRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // already exists or category not found
		}
		return nil, err
	}
	return &e, nil
}

func (db *DB) DeleteBookmarkEntry(ctx context.Context, entryID int64, authID string) (bool, error) {
	query := `DELETE FROM bookmark_entries
	          USING bookmark_categories bc, users u
	          WHERE bookmark_entries.id = $1
	            AND bookmark_entries.category_id = bc.id
	            AND bc.user_id = u.id
	            AND u.auth_id = $2`
	tag, err := db.Pool.Exec(ctx, query, entryID, authID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// FetchChapterBookmarkEntries returns all bookmark entries for a chapter, keyed by verse_key.
// Used by scripture-state endpoint.
func (db *DB) FetchChapterBookmarkEntries(ctx context.Context, authID, scripture string, chapter int) ([]BookmarkEntryRow, error) {
	prefix := fmt.Sprintf("%d:%%", chapter)
	query := `SELECT ` + entryCols + `
	          FROM bookmark_entries be
	          JOIN bookmark_categories bc ON bc.id = be.category_id
	          JOIN users u ON u.id = bc.user_id
	          WHERE u.auth_id = $1 AND be.scripture = $2 AND be.verse_key LIKE $3
	          ORDER BY be.verse_key, be.created_at DESC`
	rows, err := db.Pool.Query(ctx, query, authID, scripture, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[BookmarkEntryRow])
}
