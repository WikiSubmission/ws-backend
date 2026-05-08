package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type UserRow struct {
	ID          int             `db:"id"           json:"id"`
	AuthID      string          `db:"auth_id"      json:"auth_id"`
	Email       string          `db:"email"        json:"email"`
	DisplayName *string         `db:"display_name" json:"display_name,omitempty"`
	Role        string          `db:"role"         json:"role"`
	Permissions json.RawMessage `db:"permissions"  json:"permissions"`
	IsActive    bool            `db:"is_active"    json:"is_active"`
	CreatedAt   time.Time       `db:"created_at"   json:"created_at"`
	UpdatedAt   time.Time       `db:"updated_at"   json:"updated_at"`
}

func (db *DB) FetchAllUsers(ctx context.Context, limit, offset int) ([]UserRow, error) {
	query := `SELECT id, auth_id, email, display_name, role, permissions, is_active, created_at, updated_at
	          FROM users ORDER BY id LIMIT $1 OFFSET $2`
	rows, err := db.Pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[UserRow])
}

func (db *DB) FetchUserByAuthID(ctx context.Context, authID string) (*UserRow, error) {
	query := `SELECT id, auth_id, email, display_name, role, permissions, is_active, created_at, updated_at
	          FROM users WHERE auth_id = $1`
	rows, err := db.Pool.Query(ctx, query, authID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	user, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[UserRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (db *DB) FetchUserByID(ctx context.Context, id int) (*UserRow, error) {
	query := `SELECT id, auth_id, email, display_name, role, permissions, is_active, created_at, updated_at
	          FROM users WHERE id = $1`
	rows, err := db.Pool.Query(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	user, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[UserRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// UpsertUser creates or updates a user, keyed by email.
// If a row with this email already exists, its auth_id is updated to the new
// value — this lets the same person sign in via different OAuth providers
// and land on the same account.
func (db *DB) UpsertUser(ctx context.Context, authID, email string, displayName *string) (*UserRow, error) {
	query := `INSERT INTO users (auth_id, email, display_name)
	          VALUES ($1, $2, $3)
	          ON CONFLICT (email) DO UPDATE
	              SET auth_id = EXCLUDED.auth_id,
	                  updated_at = NOW()
	          RETURNING id, auth_id, email, display_name, role, permissions, is_active, created_at, updated_at`
	rows, err := db.Pool.Query(ctx, query, authID, email, displayName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	user, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[UserRow])
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (db *DB) UpdateUserRole(ctx context.Context, id int, role string) error {
	query := `UPDATE users SET role = $1::user_role, updated_at = NOW() WHERE id = $2`
	tag, err := db.Pool.Exec(ctx, query, role, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user %d not found", id)
	}
	return nil
}

func (db *DB) UpdateUserPermissions(ctx context.Context, id int, permissions json.RawMessage) error {
	query := `UPDATE users SET permissions = $1, updated_at = NOW() WHERE id = $2`
	tag, err := db.Pool.Exec(ctx, query, permissions, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user %d not found", id)
	}
	return nil
}

func (db *DB) DeactivateUser(ctx context.Context, id int) error {
	query := `UPDATE users SET is_active = FALSE, updated_at = NOW() WHERE id = $1`
	tag, err := db.Pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user %d not found", id)
	}
	return nil
}

func (db *DB) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}
