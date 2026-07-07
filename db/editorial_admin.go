package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Admin-side editorial grant management. Reads and writes the grant tables
// introduced in migrations 018/019 (editor_module_grants,
// editor_quran_version_grants incl. can_approve, editor_bible_version_grants,
// editor_quran_reference_pref). Admin-only surface; callers gate on IsAdmin.

// ErrUserNotFound signals an unknown user id (HTTP 404).
var ErrUserNotFound = errors.New("user not found")

// EditorVersionGrantRow is one per-version grant (Quran or Bible).
type EditorVersionGrantRow struct {
	VersionID  int
	CanWrite   bool
	CanApprove bool // Quran only; always false for Bible
}

// EditorGrantsRow aggregates one user's editorial grants.
type EditorGrantsRow struct {
	UserID                  int
	Email                   string
	DisplayName             *string
	Role                    string
	IsActive                bool
	Modules                 map[string]bool
	QuranVersions           []EditorVersionGrantRow
	BibleVersions           []EditorVersionGrantRow
	QuranReferenceVersionID *int
}

// ListEditorGrants returns users (ordered by id) with their editorial grants
// and the total user count before limit/offset.
func (db *DB) ListEditorGrants(ctx context.Context, limit, offset int) ([]EditorGrantsRow, int, error) {
	defer observe("list_editor_grants", time.Now())

	rows, err := db.Pool.Query(ctx, `
		SELECT id, email, display_name, role, is_active, COUNT(*) OVER() AS total
		FROM users
		ORDER BY id
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []EditorGrantsRow
	byID := map[int]*EditorGrantsRow{}
	ids := []int{}
	total := 0
	for rows.Next() {
		r := EditorGrantsRow{Modules: map[string]bool{}}
		if err := rows.Scan(&r.UserID, &r.Email, &r.DisplayName, &r.Role, &r.IsActive, &total); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
		ids = append(ids, r.UserID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for i := range out {
		byID[out[i].UserID] = &out[i]
	}
	if len(ids) == 0 {
		return out, total, nil
	}

	if err := db.attachGrants(ctx, ids, byID); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// GetEditorGrants returns one user's grants, or ErrUserNotFound.
func (db *DB) GetEditorGrants(ctx context.Context, userID int) (*EditorGrantsRow, error) {
	defer observe("get_editor_grants", time.Now())

	r := EditorGrantsRow{Modules: map[string]bool{}}
	err := db.Pool.QueryRow(ctx, `
		SELECT id, email, display_name, role, is_active
		FROM users WHERE id = $1`, userID,
	).Scan(&r.UserID, &r.Email, &r.DisplayName, &r.Role, &r.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := db.attachGrants(ctx, []int{userID}, map[int]*EditorGrantsRow{userID: &r}); err != nil {
		return nil, err
	}
	return &r, nil
}

// attachGrants loads all grant tables for the given user ids into byID.
func (db *DB) attachGrants(ctx context.Context, ids []int, byID map[int]*EditorGrantsRow) error {
	mods, err := db.Pool.Query(ctx, `
		SELECT user_id, module::text, can_write
		FROM editor_module_grants WHERE user_id = ANY($1)`, ids)
	if err != nil {
		return err
	}
	defer mods.Close()
	for mods.Next() {
		var uid int
		var module string
		var canWrite bool
		if err := mods.Scan(&uid, &module, &canWrite); err != nil {
			return err
		}
		if u := byID[uid]; u != nil {
			u.Modules[module] = canWrite
		}
	}
	if err := mods.Err(); err != nil {
		return err
	}

	quran, err := db.Pool.Query(ctx, `
		SELECT user_id, version_id, can_write, can_approve
		FROM editor_quran_version_grants WHERE user_id = ANY($1)
		ORDER BY version_id`, ids)
	if err != nil {
		return err
	}
	defer quran.Close()
	for quran.Next() {
		var uid int
		var g EditorVersionGrantRow
		if err := quran.Scan(&uid, &g.VersionID, &g.CanWrite, &g.CanApprove); err != nil {
			return err
		}
		if u := byID[uid]; u != nil {
			u.QuranVersions = append(u.QuranVersions, g)
		}
	}
	if err := quran.Err(); err != nil {
		return err
	}

	bible, err := db.Pool.Query(ctx, `
		SELECT user_id, version_id, can_write
		FROM editor_bible_version_grants WHERE user_id = ANY($1)
		ORDER BY version_id`, ids)
	if err != nil {
		return err
	}
	defer bible.Close()
	for bible.Next() {
		var uid int
		var g EditorVersionGrantRow
		if err := bible.Scan(&uid, &g.VersionID, &g.CanWrite); err != nil {
			return err
		}
		if u := byID[uid]; u != nil {
			u.BibleVersions = append(u.BibleVersions, g)
		}
	}
	if err := bible.Err(); err != nil {
		return err
	}

	prefs, err := db.Pool.Query(ctx, `
		SELECT user_id, reference_version_id
		FROM editor_quran_reference_pref WHERE user_id = ANY($1)`, ids)
	if err != nil {
		return err
	}
	defer prefs.Close()
	for prefs.Next() {
		var uid int
		var ref *int
		if err := prefs.Scan(&uid, &ref); err != nil {
			return err
		}
		if u := byID[uid]; u != nil {
			u.QuranReferenceVersionID = ref
		}
	}
	return prefs.Err()
}

// ReplaceEditorGrants swaps out one user's editorial grants in a single
// transaction: module grants, per-version Quran grants (write + approve),
// per-version Bible grants and the word-by-word reference preference. Grants
// absent from the input are revoked.
func (db *DB) ReplaceEditorGrants(
	ctx context.Context,
	userID int,
	modules map[EditorModule]bool,
	quranVersions []EditorVersionGrantRow,
	bibleVersions []EditorVersionGrantRow,
	quranReferenceVersionID *int,
) (*EditorGrantsRow, error) {
	defer observe("replace_editor_grants", time.Now())

	var exists bool
	if err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, userID,
	).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrUserNotFound
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM editor_module_grants WHERE user_id = $1`, userID); err != nil {
		return nil, err
	}
	for module, canWrite := range modules {
		if _, err := tx.Exec(ctx, `
			INSERT INTO editor_module_grants (user_id, module, can_write)
			VALUES ($1, $2, $3)`, userID, string(module), canWrite); err != nil {
			return nil, translateContentPgError(err)
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM editor_quran_version_grants WHERE user_id = $1`, userID); err != nil {
		return nil, err
	}
	for _, g := range quranVersions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO editor_quran_version_grants (user_id, version_id, can_write, can_approve)
			VALUES ($1, $2, $3, $4)`, userID, g.VersionID, g.CanWrite, g.CanApprove); err != nil {
			return nil, translateContentPgError(err)
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM editor_bible_version_grants WHERE user_id = $1`, userID); err != nil {
		return nil, err
	}
	for _, g := range bibleVersions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO editor_bible_version_grants (user_id, version_id, can_write)
			VALUES ($1, $2, $3)`, userID, g.VersionID, g.CanWrite); err != nil {
			return nil, translateContentPgError(err)
		}
	}

	if quranReferenceVersionID == nil {
		if _, err := tx.Exec(ctx, `DELETE FROM editor_quran_reference_pref WHERE user_id = $1`, userID); err != nil {
			return nil, err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			INSERT INTO editor_quran_reference_pref (user_id, reference_version_id, updated_at)
			VALUES ($1, $2, now())
			ON CONFLICT (user_id) DO UPDATE
			SET reference_version_id = EXCLUDED.reference_version_id, updated_at = now()`,
			userID, *quranReferenceVersionID); err != nil {
			return nil, translateContentPgError(err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return db.GetEditorGrants(ctx, userID)
}
