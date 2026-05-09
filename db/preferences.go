package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type PreferencesRow struct {
	UserID    int       `db:"user_id"`
	Scripture string    `db:"scripture"`
	Payload   []byte    `db:"payload"`
	UpdatedAt time.Time `db:"updated_at"`
}

func (db *DB) GetPreferences(ctx context.Context, authID, scripture string) (*PreferencesRow, error) {
	query := `SELECT p.user_id, p.scripture, p.payload, p.updated_at
	          FROM preferences p
	          JOIN users u ON u.id = p.user_id
	          WHERE u.auth_id = $1 AND p.scripture = $2`
	rows, err := db.Pool.Query(ctx, query, authID, scripture)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[PreferencesRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (db *DB) UpsertPreferences(ctx context.Context, authID, scripture string, payload []byte) (*PreferencesRow, error) {
	query := `INSERT INTO preferences (user_id, scripture, payload, updated_at)
	          SELECT u.id, $2, $3::jsonb, NOW() FROM users u WHERE u.auth_id = $1
	          ON CONFLICT (user_id, scripture) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()
	          RETURNING user_id, scripture, payload, updated_at`
	rows, err := db.Pool.Query(ctx, query, authID, scripture, string(payload))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[PreferencesRow])
	if err != nil {
		return nil, err
	}
	return &row, nil
}
