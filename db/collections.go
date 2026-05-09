package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type CollectionRow struct {
	ID          int64      `db:"id"`
	UserID      int        `db:"user_id"`
	Name        string     `db:"name"`
	Description string     `db:"description"`
	IsPublic    bool       `db:"is_public"`
	ShareToken  *string    `db:"share_token"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}

type CollectionVerseRow struct {
	ID           int64     `db:"id"`
	CollectionID int64     `db:"collection_id"`
	Scripture    string    `db:"scripture"`
	VerseKey     string    `db:"verse_key"`
	Note         string    `db:"note"`
	Position     int       `db:"position"`
	CreatedAt    time.Time `db:"created_at"`
}

func generateShareToken() (string, error) {
	b := make([]byte, 12)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

const collectionCols = `c.id, c.user_id, c.name, c.description, c.is_public, c.share_token, c.created_at, c.updated_at`

func (db *DB) ListCollections(ctx context.Context, authID string) ([]CollectionRow, error) {
	query := `SELECT ` + collectionCols + `
	          FROM collections c
	          JOIN users u ON u.id = c.user_id
	          WHERE u.auth_id = $1
	          ORDER BY c.updated_at DESC`
	rows, err := db.Pool.Query(ctx, query, authID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[CollectionRow])
}

func (db *DB) CreateCollection(ctx context.Context, authID, name, description string, isPublic bool) (*CollectionRow, error) {
	var shareToken *string
	if isPublic {
		tok, err := generateShareToken()
		if err != nil {
			return nil, fmt.Errorf("generate share token: %w", err)
		}
		shareToken = &tok
	}
	query := `INSERT INTO collections (user_id, name, description, is_public, share_token)
	          SELECT u.id, $2, $3, $4, $5 FROM users u WHERE u.auth_id = $1
	          RETURNING id, user_id, name, description, is_public, share_token, created_at, updated_at`
	rows, err := db.Pool.Query(ctx, query, authID, name, description, isPublic, shareToken)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[CollectionRow])
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (db *DB) UpdateCollection(ctx context.Context, id int64, authID, name, description string, isPublic bool, regenerateToken bool) (*CollectionRow, error) {
	var shareToken *string
	if isPublic {
		tok, err := generateShareToken()
		if err != nil && regenerateToken {
			return nil, fmt.Errorf("generate share token: %w", err)
		}
		if err == nil {
			shareToken = &tok
		}
	}

	var query string
	var args []any
	if regenerateToken || isPublic {
		query = `UPDATE collections SET name = $3, description = $4, is_public = $5, share_token = $6, updated_at = NOW()
		         FROM users u
		         WHERE collections.id = $1 AND u.auth_id = $2 AND collections.user_id = u.id
		         RETURNING collections.id, collections.user_id, collections.name, collections.description,
		                   collections.is_public, collections.share_token, collections.created_at, collections.updated_at`
		args = []any{id, authID, name, description, isPublic, shareToken}
	} else {
		query = `UPDATE collections SET name = $3, description = $4, is_public = $5, share_token = NULL, updated_at = NOW()
		         FROM users u
		         WHERE collections.id = $1 AND u.auth_id = $2 AND collections.user_id = u.id
		         RETURNING collections.id, collections.user_id, collections.name, collections.description,
		                   collections.is_public, collections.share_token, collections.created_at, collections.updated_at`
		args = []any{id, authID, name, description, isPublic}
	}

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[CollectionRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (db *DB) DeleteCollection(ctx context.Context, id int64, authID string) (bool, error) {
	query := `DELETE FROM collections
	          USING users u
	          WHERE collections.id = $1 AND u.auth_id = $2 AND collections.user_id = u.id`
	tag, err := db.Pool.Exec(ctx, query, id, authID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (db *DB) GetCollectionByShareToken(ctx context.Context, token string) (*CollectionRow, error) {
	query := `SELECT ` + collectionCols + ` FROM collections c WHERE c.share_token = $1`
	rows, err := db.Pool.Query(ctx, query, token)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[CollectionRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// ── Collection verses ─────────────────────────────────────────────────────

func (db *DB) ListCollectionVerses(ctx context.Context, collectionID int64) ([]CollectionVerseRow, error) {
	query := `SELECT id, collection_id, scripture, verse_key, note, position, created_at
	          FROM collection_verses
	          WHERE collection_id = $1
	          ORDER BY position, created_at`
	rows, err := db.Pool.Query(ctx, query, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[CollectionVerseRow])
}

func (db *DB) AddVerseToCollection(ctx context.Context, collectionID int64, authID, scripture, verseKey, note string) (*CollectionVerseRow, error) {
	// Verify ownership first.
	var ownerCheck int
	err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM collections c JOIN users u ON u.id = c.user_id WHERE c.id = $1 AND u.auth_id = $2`,
		collectionID, authID).Scan(&ownerCheck)
	if err != nil || ownerCheck == 0 {
		return nil, fmt.Errorf("collection not found or not owned by user")
	}

	query := `INSERT INTO collection_verses (collection_id, scripture, verse_key, note)
	          VALUES ($1, $2, $3, $4)
	          ON CONFLICT (collection_id, scripture, verse_key) DO UPDATE SET note = EXCLUDED.note
	          RETURNING id, collection_id, scripture, verse_key, note, position, created_at`
	rows, err := db.Pool.Query(ctx, query, collectionID, scripture, verseKey, note)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cv, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[CollectionVerseRow])
	if err != nil {
		return nil, err
	}
	return &cv, nil
}

func (db *DB) RemoveVerseFromCollection(ctx context.Context, verseID int64, authID string) (bool, error) {
	query := `DELETE FROM collection_verses cv
	          USING collections c
	          JOIN users u ON u.id = c.user_id
	          WHERE cv.id = $1 AND cv.collection_id = c.id AND u.auth_id = $2`
	tag, err := db.Pool.Exec(ctx, query, verseID, authID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
