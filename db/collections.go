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
	ID               int64      `db:"id"`
	UserID           int        `db:"user_id"`
	Name             string     `db:"name"`
	Description      string     `db:"description"`
	IsPublic         bool       `db:"is_public"`
	EditPolicy       string     `db:"edit_policy"`
	ShareToken       *string    `db:"share_token"`
	CreatedAt        time.Time  `db:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at"`
	Relation         string     `db:"relation"`           // "owner" or "subscriber"
	OwnerDisplayName string     `db:"owner_display_name"` // set for subscriber rows
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

const collectionSelectCols = `c.id, c.user_id, c.name, c.description, c.is_public, c.edit_policy, c.share_token, c.created_at, c.updated_at`

func (db *DB) ListCollections(ctx context.Context, authID string) ([]CollectionRow, error) {
	query := `
		SELECT ` + collectionSelectCols + `, 'owner' AS relation, '' AS owner_display_name
		FROM collections c
		JOIN users u ON u.id = c.user_id
		WHERE u.auth_id = $1

		UNION ALL

		SELECT ` + collectionSelectCols + `, 'subscriber' AS relation,
		       COALESCE(uo.display_name, uo.email) AS owner_display_name
		FROM collection_subscriptions cs
		JOIN users uc ON uc.auth_id = $1 AND uc.id = cs.user_id
		JOIN collections c ON c.id = cs.collection_id
		JOIN users uo ON uo.id = c.user_id

		ORDER BY updated_at DESC`
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
	          RETURNING id, user_id, name, description, is_public, edit_policy, share_token, created_at, updated_at,
	                    'owner' AS relation, '' AS owner_display_name`
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

func (db *DB) UpdateCollection(ctx context.Context, id int64, authID, name, description string, isPublic bool, editPolicy string, regenerateToken bool) (*CollectionRow, error) {
	if editPolicy == "" {
		editPolicy = "owner_only"
	}
	// Reset edit policy to owner_only when collection goes private.
	if !isPublic {
		editPolicy = "owner_only"
	}

	// Generate a fresh token whenever explicitly requested or when the
	// collection is going public for the first time (COALESCE keeps the
	// existing token if one is already present).
	tok, err := generateShareToken()
	if err != nil {
		return nil, fmt.Errorf("generate share token: %w", err)
	}
	newToken := tok

	var query string
	var args []any
	if !isPublic {
		// Going private: clear the token.
		query = `UPDATE collections
		         SET name = $3, description = $4, is_public = $5, edit_policy = $6, share_token = NULL, updated_at = NOW()
		         FROM users u
		         WHERE collections.id = $1 AND u.auth_id = $2 AND collections.user_id = u.id
		         RETURNING collections.id, collections.user_id, collections.name, collections.description,
		                   collections.is_public, collections.edit_policy, collections.share_token,
		                   collections.created_at, collections.updated_at,
		                   'owner' AS relation, '' AS owner_display_name`
		args = []any{id, authID, name, description, isPublic, editPolicy}
	} else if regenerateToken {
		// Explicit regeneration: always overwrite the token.
		query = `UPDATE collections
		         SET name = $3, description = $4, is_public = $5, edit_policy = $6, share_token = $7, updated_at = NOW()
		         FROM users u
		         WHERE collections.id = $1 AND u.auth_id = $2 AND collections.user_id = u.id
		         RETURNING collections.id, collections.user_id, collections.name, collections.description,
		                   collections.is_public, collections.edit_policy, collections.share_token,
		                   collections.created_at, collections.updated_at,
		                   'owner' AS relation, '' AS owner_display_name`
		args = []any{id, authID, name, description, isPublic, editPolicy, newToken}
	} else {
		// Going public without regeneration: keep existing token, generate only if null.
		query = `UPDATE collections
		         SET name = $3, description = $4, is_public = $5, edit_policy = $6,
		             share_token = COALESCE(collections.share_token, $7), updated_at = NOW()
		         FROM users u
		         WHERE collections.id = $1 AND u.auth_id = $2 AND collections.user_id = u.id
		         RETURNING collections.id, collections.user_id, collections.name, collections.description,
		                   collections.is_public, collections.edit_policy, collections.share_token,
		                   collections.created_at, collections.updated_at,
		                   'owner' AS relation, '' AS owner_display_name`
		args = []any{id, authID, name, description, isPublic, editPolicy, newToken}
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
	// is_public guard is defence-in-depth: a non-null token on a private collection
	// (if that invariant were ever violated) must not grant public access.
	query := `SELECT ` + collectionSelectCols + `, 'owner' AS relation, '' AS owner_display_name
	          FROM collections c WHERE c.share_token = $1 AND c.is_public = true`
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

// GetCollectionForUser returns a collection by ID, including subscriber access.
// Returns nil if the collection does not exist or the caller has no access.
func (db *DB) GetCollectionForUser(ctx context.Context, id int64, authID string) (*CollectionRow, error) {
	query := `
		SELECT ` + collectionSelectCols + `, 'owner' AS relation, '' AS owner_display_name
		FROM collections c
		JOIN users u ON u.id = c.user_id
		WHERE c.id = $1 AND u.auth_id = $2

		UNION ALL

		SELECT ` + collectionSelectCols + `, 'subscriber' AS relation,
		       COALESCE(uo.display_name, uo.email) AS owner_display_name
		FROM collection_subscriptions cs
		JOIN users uc ON uc.auth_id = $2 AND uc.id = cs.user_id
		JOIN collections c ON c.id = cs.collection_id AND c.id = $1
		JOIN users uo ON uo.id = c.user_id

		LIMIT 1`
	rows, err := db.Pool.Query(ctx, query, id, authID)
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

// ── Collection subscriptions ───────────────────────────────────────────────

// SubscribeToCollection subscribes authID to the public collection identified
// by shareToken. Returns the collection ID on success.
func (db *DB) SubscribeToCollection(ctx context.Context, authID, shareToken string) (int64, error) {
	var collectionID int64
	var ownerAuthID string
	err := db.Pool.QueryRow(ctx,
		`SELECT c.id, u.auth_id FROM collections c JOIN users u ON u.id = c.user_id WHERE c.share_token = $1 AND c.is_public = true`,
		shareToken,
	).Scan(&collectionID, &ownerAuthID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, fmt.Errorf("collection not found or not public")
		}
		return 0, err
	}
	if ownerAuthID == authID {
		return 0, fmt.Errorf("cannot subscribe to your own collection")
	}

	_, err = db.Pool.Exec(ctx,
		`INSERT INTO collection_subscriptions (user_id, collection_id)
		 SELECT u.id, $2 FROM users u WHERE u.auth_id = $1
		 ON CONFLICT DO NOTHING`,
		authID, collectionID,
	)
	if err != nil {
		return 0, err
	}
	return collectionID, nil
}

// UnsubscribeFromCollection removes a subscription. Only the subscriber can do this.
func (db *DB) UnsubscribeFromCollection(ctx context.Context, authID string, collectionID int64) (bool, error) {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM collection_subscriptions cs
		 USING users u
		 WHERE u.auth_id = $1 AND cs.user_id = u.id AND cs.collection_id = $2`,
		authID, collectionID,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// IsSubscribed reports whether authID has a live subscription to collectionID.
func (db *DB) IsSubscribed(ctx context.Context, authID string, collectionID int64) (bool, error) {
	var n int
	err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM collection_subscriptions cs
		 JOIN users u ON u.id = cs.user_id
		 WHERE u.auth_id = $1 AND cs.collection_id = $2`,
		authID, collectionID,
	).Scan(&n)
	return n > 0, err
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

// canEditCollection reports whether authID is allowed to add/remove verses in the given collection.
// Owner always can. Any authenticated user can if the collection is public and edit_policy='everyone'.
func (db *DB) canEditCollection(ctx context.Context, collectionID int64, authID string) (bool, error) {
	var isOwner, isPublicEveryone bool
	err := db.Pool.QueryRow(ctx, `
		SELECT
		  EXISTS(
		    SELECT 1 FROM collections c JOIN users u ON u.id = c.user_id
		    WHERE c.id = $1 AND u.auth_id = $2
		  ) AS is_owner,
		  EXISTS(
		    SELECT 1 FROM collections c
		    WHERE c.id = $1 AND c.is_public = true AND c.edit_policy = 'everyone'
		  ) AS is_public_everyone`,
		collectionID, authID,
	).Scan(&isOwner, &isPublicEveryone)
	if err != nil {
		return false, err
	}
	return isOwner || isPublicEveryone, nil
}

func (db *DB) AddVerseToCollection(ctx context.Context, collectionID int64, authID, scripture, verseKey, note string) (*CollectionVerseRow, error) {
	ok, err := db.canEditCollection(ctx, collectionID, authID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("collection not found or not editable by user")
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
	// Resolve the collection this verse belongs to.
	var collectionID int64
	err := db.Pool.QueryRow(ctx,
		`SELECT collection_id FROM collection_verses WHERE id = $1`, verseID,
	).Scan(&collectionID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	ok, err := db.canEditCollection(ctx, collectionID, authID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	tag, err := db.Pool.Exec(ctx, `DELETE FROM collection_verses WHERE id = $1`, verseID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
