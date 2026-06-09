package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// EditorModule identifies a top-level editorial surface. Values mirror the
// public.editor_module enum in migration 018 and the Sanity editorProfile
// contentType taxonomy being replaced.
type EditorModule string

const (
	ModuleArticle   EditorModule = "article"
	ModuleQuran     EditorModule = "quran"
	ModuleBible     EditorModule = "bible"
	ModuleCommunity EditorModule = "community"
	ModuleAuthor    EditorModule = "author"
	ModuleCategory  EditorModule = "category"
	ModuleAppendix  EditorModule = "appendix"
)

// EditorPermissions is the resolved, in-memory authorization snapshot for one
// user. It is deliberately a plain data structure with pure decision methods so
// the security-critical access logic can be exhaustively unit-tested without a
// database. Build one with DB.FetchEditorPermissions.
//
// Semantics:
//   - Admins bypass every check (IsAdmin == true).
//   - A module appears in Modules iff the user has at least READ on it; the bool
//     value reports whether they also have WRITE.
//   - A version appears in QuranVersions/BibleVersions iff the user has READ on
//     that version; the bool reports WRITE. Version access additionally requires
//     READ on the owning module (quran/bible).
type EditorPermissions struct {
	IsAdmin                 bool
	Modules                 map[EditorModule]bool
	QuranVersions           map[int]bool
	BibleVersions           map[int]bool
	QuranReferenceVersionID *int
}

// newEditorPermissions returns an EditorPermissions with initialized maps so
// callers and methods never dereference nil maps.
func newEditorPermissions(isAdmin bool) *EditorPermissions {
	return &EditorPermissions{
		IsAdmin:       isAdmin,
		Modules:       make(map[EditorModule]bool),
		QuranVersions: make(map[int]bool),
		BibleVersions: make(map[int]bool),
	}
}

// CanReadModule reports whether the user may view the given module.
func (p *EditorPermissions) CanReadModule(m EditorModule) bool {
	if p == nil {
		return false
	}
	if p.IsAdmin {
		return true
	}
	_, ok := p.Modules[m]
	return ok
}

// CanWriteModule reports whether the user may edit the given module.
func (p *EditorPermissions) CanWriteModule(m EditorModule) bool {
	if p == nil {
		return false
	}
	if p.IsAdmin {
		return true
	}
	return p.Modules[m]
}

// CanReadQuranVersion reports whether the user may view the given Quran version.
// Requires read on the quran module and a grant on the version.
func (p *EditorPermissions) CanReadQuranVersion(versionID int) bool {
	if p == nil {
		return false
	}
	if p.IsAdmin {
		return true
	}
	if !p.CanReadModule(ModuleQuran) {
		return false
	}
	_, ok := p.QuranVersions[versionID]
	return ok
}

// CanWriteQuranVersion reports whether the user may edit the given Quran version.
func (p *EditorPermissions) CanWriteQuranVersion(versionID int) bool {
	if p == nil {
		return false
	}
	if p.IsAdmin {
		return true
	}
	if !p.CanReadModule(ModuleQuran) {
		return false
	}
	return p.QuranVersions[versionID]
}

// CanReadBibleVersion reports whether the user may view the given Bible version.
func (p *EditorPermissions) CanReadBibleVersion(versionID int) bool {
	if p == nil {
		return false
	}
	if p.IsAdmin {
		return true
	}
	if !p.CanReadModule(ModuleBible) {
		return false
	}
	_, ok := p.BibleVersions[versionID]
	return ok
}

// CanWriteBibleVersion reports whether the user may edit the given Bible version.
func (p *EditorPermissions) CanWriteBibleVersion(versionID int) bool {
	if p == nil {
		return false
	}
	if p.IsAdmin {
		return true
	}
	if !p.CanReadModule(ModuleBible) {
		return false
	}
	return p.BibleVersions[versionID]
}

// HasAnyEditorialAccess reports whether the user can reach the editor at all
// (admin, or holds at least one module grant). Used to gate the /editor surface.
func (p *EditorPermissions) HasAnyEditorialAccess() bool {
	if p == nil {
		return false
	}
	return p.IsAdmin || len(p.Modules) > 0
}

// FetchEditorPermissions assembles the resolved permission snapshot for a user
// from the grant tables. isAdmin is supplied by the caller (derived from the
// already-fetched user row) so admins are not special-cased with extra queries.
func (db *DB) FetchEditorPermissions(ctx context.Context, userID int, isAdmin bool) (*EditorPermissions, error) {
	defer observe("fetch_editor_permissions", time.Now())

	perms := newEditorPermissions(isAdmin)

	// Module grants.
	moduleRows, err := db.Pool.Query(ctx,
		`SELECT module, can_write FROM editor_module_grants WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("fetch_editor_permissions: query module grants: %w", err)
	}
	defer moduleRows.Close()
	for moduleRows.Next() {
		var module string
		var canWrite bool
		if err := moduleRows.Scan(&module, &canWrite); err != nil {
			return nil, fmt.Errorf("fetch_editor_permissions: scan module grant: %w", err)
		}
		perms.Modules[EditorModule(module)] = canWrite
	}
	if err := moduleRows.Err(); err != nil {
		return nil, fmt.Errorf("fetch_editor_permissions: iterate module grants: %w", err)
	}

	// Quran version grants.
	if err := db.scanVersionGrants(ctx,
		`SELECT version_id, can_write FROM editor_quran_version_grants WHERE user_id = $1`,
		userID, perms.QuranVersions); err != nil {
		return nil, fmt.Errorf("fetch_editor_permissions: quran version grants: %w", err)
	}

	// Bible version grants.
	if err := db.scanVersionGrants(ctx,
		`SELECT version_id, can_write FROM editor_bible_version_grants WHERE user_id = $1`,
		userID, perms.BibleVersions); err != nil {
		return nil, fmt.Errorf("fetch_editor_permissions: bible version grants: %w", err)
	}

	// Quran word-by-word reference preference (optional row).
	var refVersionID *int
	err = db.Pool.QueryRow(ctx,
		`SELECT reference_version_id FROM editor_quran_reference_pref WHERE user_id = $1`, userID).
		Scan(&refVersionID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("fetch_editor_permissions: reference preference: %w", err)
	}
	perms.QuranReferenceVersionID = refVersionID

	return perms, nil
}

// scanVersionGrants runs a (version_id, can_write) query and fills dst.
// query must be a trusted literal (it is interpolated only via parameterized
// $1 for userID); never pass externally-influenced SQL here.
func (db *DB) scanVersionGrants(ctx context.Context, query string, userID int, dst map[int]bool) error {
	rows, err := db.Pool.Query(ctx, query, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var versionID int
		var canWrite bool
		if err := rows.Scan(&versionID, &canWrite); err != nil {
			return err
		}
		dst[versionID] = canWrite
	}
	return rows.Err()
}

// QuranVersionRow mirrors a row of public.quran_versions.
type QuranVersionRow struct {
	ID                 int             `db:"id"                   json:"id"`
	Slug               string          `db:"slug"                 json:"slug"`
	Name               string          `db:"name"                 json:"name"`
	LangID             int16           `db:"lang_id"              json:"lang_id"`
	Authors            json.RawMessage `db:"authors"              json:"authors"`
	Direction          string          `db:"direction"            json:"direction"`
	IsCanonicalEnglish bool            `db:"is_canonical_english" json:"is_canonical_english"`
	ReferenceVersionID *int            `db:"reference_version_id" json:"reference_version_id,omitempty"`
	CreatedAt          time.Time       `db:"created_at"           json:"created_at"`
	UpdatedAt          time.Time       `db:"updated_at"           json:"updated_at"`
}

// FetchQuranVersions returns all Quran versions ordered by id.
func (db *DB) FetchQuranVersions(ctx context.Context) ([]QuranVersionRow, error) {
	defer observe("fetch_quran_versions", time.Now())
	query := `SELECT id, slug, name, lang_id, authors, direction, is_canonical_english,
	                 reference_version_id, created_at, updated_at
	          FROM quran_versions ORDER BY id`
	rows, err := db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[QuranVersionRow])
}

// BibleVersionRow mirrors a row of public.bible_versions.
type BibleVersionRow struct {
	ID        int             `db:"id"         json:"id"`
	Slug      string          `db:"slug"       json:"slug"`
	Name      string          `db:"name"       json:"name"`
	LangID    int16           `db:"lang_id"    json:"lang_id"`
	Authors   json.RawMessage `db:"authors"    json:"authors"`
	Direction string          `db:"direction"  json:"direction"`
	CreatedAt time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt time.Time       `db:"updated_at" json:"updated_at"`
}

// FetchBibleVersions returns all Bible versions ordered by id.
func (db *DB) FetchBibleVersions(ctx context.Context) ([]BibleVersionRow, error) {
	defer observe("fetch_bible_versions", time.Now())
	query := `SELECT id, slug, name, lang_id, authors, direction, created_at, updated_at
	          FROM bible_versions ORDER BY id`
	rows, err := db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[BibleVersionRow])
}
