package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type BookmarkCategoryRow struct {
	ID         int64     `db:"id"`
	UserID     int       `db:"user_id"`
	Name       string    `db:"name"`
	Color      string    `db:"color"`
	EntryCount int       `db:"entry_count"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

const categoryCols = `
    bc.id, bc.user_id, bc.name, bc.color,
    COUNT(be.id) AS entry_count,
    bc.created_at, bc.updated_at`

func (db *DB) ListBookmarkCategories(ctx context.Context, authID string) ([]BookmarkCategoryRow, error) {
	query := `SELECT` + categoryCols + `
	          FROM bookmark_categories bc
	          JOIN users u ON u.id = bc.user_id
	          LEFT JOIN bookmark_entries be ON be.category_id = bc.id
	          WHERE u.auth_id = $1
	          GROUP BY bc.id
	          ORDER BY bc.created_at ASC`
	rows, err := db.Pool.Query(ctx, query, authID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[BookmarkCategoryRow])
}

func (db *DB) CreateBookmarkCategory(ctx context.Context, authID, name, color string) (*BookmarkCategoryRow, error) {
	query := `WITH ins AS (
	              INSERT INTO bookmark_categories (user_id, name, color)
	              SELECT u.id, $2, $3 FROM users u WHERE u.auth_id = $1
	              RETURNING id, user_id, name, color, created_at, updated_at
	          )
	          SELECT ins.id, ins.user_id, ins.name, ins.color, 0 AS entry_count,
	                 ins.created_at, ins.updated_at
	          FROM ins`
	rows, err := db.Pool.Query(ctx, query, authID, name, color)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[BookmarkCategoryRow])
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (db *DB) UpdateBookmarkCategory(ctx context.Context, id int64, authID, name, color string) (*BookmarkCategoryRow, error) {
	query := `WITH upd AS (
	              UPDATE bookmark_categories SET name = $3, color = $4, updated_at = NOW()
	              FROM users u
	              WHERE bookmark_categories.id = $1 AND u.auth_id = $2
	                AND bookmark_categories.user_id = u.id
	              RETURNING bookmark_categories.id, bookmark_categories.user_id,
	                        bookmark_categories.name, bookmark_categories.color,
	                        bookmark_categories.created_at, bookmark_categories.updated_at
	          )
	          SELECT upd.id, upd.user_id, upd.name, upd.color,
	                 COUNT(be.id) AS entry_count,
	                 upd.created_at, upd.updated_at
	          FROM upd
	          LEFT JOIN bookmark_entries be ON be.category_id = upd.id
	          GROUP BY upd.id, upd.user_id, upd.name, upd.color, upd.created_at, upd.updated_at`
	rows, err := db.Pool.Query(ctx, query, id, authID, name, color)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[BookmarkCategoryRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// EnsureDefaultBookmarkCategory creates a default "My Bookmarks" category for
// a user when they have none. Idempotent — safe to call on every /me write.
func (db *DB) EnsureDefaultBookmarkCategory(ctx context.Context, authID string) error {
	query := `INSERT INTO bookmark_categories (user_id, name, color)
	          SELECT u.id, 'My Bookmarks', '#f59e0b'
	          FROM users u
	          WHERE u.auth_id = $1
	            AND NOT EXISTS (
	                SELECT 1 FROM bookmark_categories WHERE user_id = u.id
	            )`
	_, err := db.Pool.Exec(ctx, query, authID)
	return err
}

func (db *DB) DeleteBookmarkCategory(ctx context.Context, id int64, authID string) (bool, error) {
	query := `DELETE FROM bookmark_categories
	          USING users u
	          WHERE bookmark_categories.id = $1 AND u.auth_id = $2
	            AND bookmark_categories.user_id = u.id`
	tag, err := db.Pool.Exec(ctx, query, id, authID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
