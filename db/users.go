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
	Roles       []string        `db:"roles"        json:"roles"`
	Permissions json.RawMessage `db:"permissions"  json:"permissions"`
	IsActive    bool            `db:"is_active"    json:"is_active"`
	CreatedAt   time.Time       `db:"created_at"   json:"created_at"`
	UpdatedAt   time.Time       `db:"updated_at"   json:"updated_at"`
}

const userColumns = `id, auth_id, email, display_name, role,
	COALESCE(ARRAY(
		SELECT ur.role::text FROM user_roles ur WHERE ur.user_id = users.id
		ORDER BY CASE ur.role::text
			WHEN 'member' THEN 0 WHEN 'editor' THEN 1
			WHEN 'game_editor' THEN 2 WHEN 'admin' THEN 3 END
	), ARRAY[]::text[]) AS roles,
	permissions, is_active, created_at, updated_at`

func (u *UserRow) HasRole(role string) bool {
	if u == nil {
		return false
	}
	for _, candidate := range u.Roles {
		if candidate == role {
			return true
		}
	}
	// Compatibility for rows read while an older migration state is being
	// repaired. The additive migration normally guarantees Roles is populated.
	return (role == "member" && len(u.Roles) == 0) || u.Role == role
}

func (db *DB) FetchAllUsers(ctx context.Context, limit, offset int) ([]UserRow, error) {
	defer observe("fetch_all_users", time.Now())
	query := `SELECT ` + userColumns + `
	          FROM users ORDER BY id LIMIT $1 OFFSET $2`
	rows, err := db.Pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[UserRow])
}

func (db *DB) FetchUserByAuthID(ctx context.Context, authID string) (*UserRow, error) {
	defer observe("fetch_user_by_auth_id", time.Now())
	query := `SELECT ` + userColumns + `
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
	defer observe("fetch_user_by_id", time.Now())
	query := `SELECT ` + userColumns + `
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
// and land on the same account. display_name is backfilled from the incoming
// claim only when the existing row has no name set; a name set by the user
// via PATCH /users/:id is never overwritten by the SSO claim.
func (db *DB) UpsertUser(ctx context.Context, authID, email string, displayName *string) (*UserRow, error) {
	defer observe("upsert_user", time.Now())
	query := `INSERT INTO users (auth_id, email, display_name)
	          VALUES ($1, $2, $3)
	          ON CONFLICT (email) DO UPDATE
	              SET auth_id = EXCLUDED.auth_id,
	                  display_name = COALESCE(users.display_name, EXCLUDED.display_name),
	                  updated_at = NOW()
	          RETURNING id`
	var id int
	if err := db.Pool.QueryRow(ctx, query, authID, email, displayName).Scan(&id); err != nil {
		return nil, err
	}
	return db.FetchUserByID(ctx, id)
}

// UpdateUserDisplayName sets users.display_name for the given id. Used to
// backfill the name on existing rows after the SSO `name` claim becomes
// available (i.e. user signed up before the claim was wired through).
func (db *DB) UpdateUserDisplayName(ctx context.Context, id int, displayName string) error {
	query := `UPDATE users SET display_name = $1, updated_at = NOW() WHERE id = $2`
	tag, err := db.Pool.Exec(ctx, query, displayName, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user %d not found", id)
	}
	return nil
}

// IsLastActiveAdmin reports whether the given user is an active admin and no
// other active admin exists. Callers use it to refuse changes that would leave
// nobody able to reach the admin surfaces — notably /admin/access, which is now
// the single place all access is granted from. Recovering from that state needs
// direct database access, so it warrants a guard rather than a warning.
//
// Both halves are evaluated in one statement so a concurrent demotion cannot
// slip between them and take the second-to-last admin with it.
func (db *DB) IsLastActiveAdmin(ctx context.Context, id int) (bool, error) {
	var isLast bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM users u JOIN user_roles ur ON ur.user_id = u.id
			WHERE u.id = $1 AND ur.role = 'admin' AND u.is_active
		) AND NOT EXISTS (
			SELECT 1 FROM users u JOIN user_roles ur ON ur.user_id = u.id
			WHERE u.id <> $1 AND ur.role = 'admin' AND u.is_active
		)`, id).Scan(&isLast)
	if err != nil {
		return false, err
	}
	return isLast, nil
}

func (db *DB) UpdateUserRole(ctx context.Context, id int, role string) error {
	roles := []string{"member"}
	if role != "member" {
		roles = append(roles, role)
	}
	return db.UpdateUserRoles(ctx, id, roles)
}

// UpdateUserRoles replaces an account's role set atomically. Member is always
// retained, and users.role is kept as a legacy highest-role projection for old
// application versions during the staged rollout.
func (db *DB) UpdateUserRoles(ctx context.Context, id int, roles []string) error {
	requested := map[string]bool{"member": true}
	for _, role := range roles {
		switch role {
		case "member", "editor", "game_editor", "admin":
			requested[role] = true
		default:
			return fmt.Errorf("unknown role %q", role)
		}
	}

	effective := "member"
	if requested["admin"] {
		effective = "admin"
	} else if requested["editor"] {
		effective = "editor"
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE users SET role = $1::user_role, updated_at = NOW() WHERE id = $2`,
		effective, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user %d not found", id)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, id); err != nil {
		return err
	}
	for _, role := range []string{"member", "editor", "game_editor", "admin"} {
		if !requested[role] {
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_roles (user_id, role) VALUES ($1, $2::access_role)`,
			id, role); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
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
	defer observe("count_users", time.Now())
	var count int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}
