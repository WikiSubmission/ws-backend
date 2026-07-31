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
	// ModuleGame is the global tier of games access: it grants every game at
	// once. Added in migration 027; unlike the other modules it has no content
	// documents of its own, so it is deliberately excluded from the content
	// module taxonomy (see validEditorModules in api/handlers/editorial_admin.go).
	ModuleGame EditorModule = "game"
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
//   - A game appears in Games iff the user has READ on that game; the bool
//     reports WRITE. Games are the one exception to the rule above: game access
//     is a UNION, not an intersection — either the global ModuleGame grant or a
//     per-game grant is sufficient on its own. That is what lets an admin give
//     someone one game without handing them the whole games surface.
type EditorPermissions struct {
	IsAdmin                 bool
	Modules                 map[EditorModule]bool
	QuranVersions           map[int]bool
	QuranApproverVersions   map[int]bool
	BibleVersions           map[int]bool
	Games                   map[string]bool
	QuranReferenceVersionID *int
}

// newEditorPermissions returns an EditorPermissions with initialized maps so
// callers and methods never dereference nil maps.
func newEditorPermissions(isAdmin bool) *EditorPermissions {
	return &EditorPermissions{
		IsAdmin:               isAdmin,
		Modules:               make(map[EditorModule]bool),
		QuranVersions:         make(map[int]bool),
		QuranApproverVersions: make(map[int]bool),
		BibleVersions:         make(map[int]bool),
		Games:                 make(map[string]bool),
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

// CanApproveQuranVersion reports whether the user may approve / reject publish
// requests for the given Quran version. Approval is a dedicated grant (the
// can_approve flag on the version grant), independent of write; admins approve
// every version.
func (p *EditorPermissions) CanApproveQuranVersion(versionID int) bool {
	if p == nil {
		return false
	}
	if p.IsAdmin {
		return true
	}
	// Defense in depth: an approve grant only counts while the quran module
	// grant is also present, so revoking module access immediately revokes the
	// ability to approve even if a stale version grant row lingers.
	if !p.CanReadModule(ModuleQuran) {
		return false
	}
	return p.QuranApproverVersions[versionID]
}

// ApprovableQuranVersionIDs returns the version ids the user may approve. Empty
// for admins (callers must treat admin as "all" via IsAdmin, since admins hold
// no grant rows) and empty when the quran module grant is absent (mirrors
// CanApproveQuranVersion).
func (p *EditorPermissions) ApprovableQuranVersionIDs() []int {
	if p == nil || !p.CanReadModule(ModuleQuran) {
		return nil
	}
	ids := make([]int, 0, len(p.QuranApproverVersions))
	for id, ok := range p.QuranApproverVersions {
		if ok {
			ids = append(ids, id)
		}
	}
	return ids
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

// CanReadGame reports whether the user may view the given game's editorial
// surface. Unlike version access this is a UNION: the global ModuleGame grant
// covers every game, and a per-game grant stands on its own without it. Both
// paths are checked because an admin may grant either.
func (p *EditorPermissions) CanReadGame(gameKey string) bool {
	if p == nil {
		return false
	}
	if p.IsAdmin {
		return true
	}
	if _, ok := p.Modules[ModuleGame]; ok {
		return true
	}
	_, ok := p.Games[gameKey]
	return ok
}

// CanWriteGame reports whether the user may make changes to the given game
// (approve/reject passages, curate, run maintenance jobs). Union semantics as
// in CanReadGame: global write covers every game, per-game write covers one.
func (p *EditorPermissions) CanWriteGame(gameKey string) bool {
	if p == nil {
		return false
	}
	if p.IsAdmin {
		return true
	}
	if p.Modules[ModuleGame] {
		return true
	}
	return p.Games[gameKey]
}

// HasGlobalGameAccess reports whether the user holds the global ModuleGame
// grant, i.e. access to every game including ones added later. Callers listing
// games must treat this (and IsAdmin) as "all", since neither produces per-game
// rows in Games.
func (p *EditorPermissions) HasGlobalGameAccess() bool {
	if p == nil {
		return false
	}
	if p.IsAdmin {
		return true
	}
	_, ok := p.Modules[ModuleGame]
	return ok
}

// AccessibleGameKeys returns the game keys explicitly granted to the user.
// Empty for admins and global games editors — they reach every game, so check
// HasGlobalGameAccess first rather than treating an empty slice as "none".
func (p *EditorPermissions) AccessibleGameKeys() []string {
	if p == nil {
		return nil
	}
	keys := make([]string, 0, len(p.Games))
	for key := range p.Games {
		keys = append(keys, key)
	}
	return keys
}

// HasAnyGameAccess reports whether the user can reach any games surface at all
// (admin, the global grant, or at least one per-game grant). Backs the coarse
// isEditor flag the web and mobile sessions carry for nav gating.
func (p *EditorPermissions) HasAnyGameAccess() bool {
	if p == nil {
		return false
	}
	return p.IsAdmin || p.HasGlobalGameAccess() || len(p.Games) > 0
}

// HasAnyEditorialAccess reports whether the user can reach the editor at all
// (admin, or holds at least one module grant). Used to gate the /editor surface.
//
// ModuleGame does not count: games have no content documents and their studio
// lives under /admin/games, so a games-only editor would land on an empty
// workspace. Callers gating the shared permission snapshot (rather than /editor
// itself) should also accept HasAnyGameAccess — see GetEditorialSession.
func (p *EditorPermissions) HasAnyEditorialAccess() bool {
	if p == nil {
		return false
	}
	if p.IsAdmin {
		return true
	}
	for module := range p.Modules {
		if module != ModuleGame {
			return true
		}
	}
	return false
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

	// Quran version grants (write + the dedicated approve capability).
	if err := db.scanQuranVersionGrants(ctx, userID, perms); err != nil {
		return nil, fmt.Errorf("fetch_editor_permissions: quran version grants: %w", err)
	}

	// Bible version grants.
	if err := db.scanVersionGrants(ctx,
		`SELECT version_id, can_write FROM editor_bible_version_grants WHERE user_id = $1`,
		userID, perms.BibleVersions); err != nil {
		return nil, fmt.Errorf("fetch_editor_permissions: bible version grants: %w", err)
	}

	// Per-game grants (the global tier arrives with the module grants above).
	if err := db.scanGameGrants(ctx, userID, perms.Games); err != nil {
		return nil, fmt.Errorf("fetch_editor_permissions: game grants: %w", err)
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

// scanQuranVersionGrants fills the caller's Quran version write grants and the
// dedicated approve grants in one pass.
func (db *DB) scanQuranVersionGrants(ctx context.Context, userID int, perms *EditorPermissions) error {
	rows, err := db.Pool.Query(ctx,
		`SELECT version_id, can_write, can_approve FROM editor_quran_version_grants WHERE user_id = $1`,
		userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var versionID int
		var canWrite, canApprove bool
		if err := rows.Scan(&versionID, &canWrite, &canApprove); err != nil {
			return err
		}
		perms.QuranVersions[versionID] = canWrite
		if canApprove {
			perms.QuranApproverVersions[versionID] = true
		}
	}
	return rows.Err()
}

// scanGameGrants fills the caller's per-game grants. The global games grant is
// a module grant and is read alongside the other modules, not here.
func (db *DB) scanGameGrants(ctx context.Context, userID int, dst map[string]bool) error {
	rows, err := db.Pool.Query(ctx,
		`SELECT game_key, can_write FROM editor_game_grants WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var gameKey string
		var canWrite bool
		if err := rows.Scan(&gameKey, &canWrite); err != nil {
			return err
		}
		dst[gameKey] = canWrite
	}
	return rows.Err()
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

// EditorGameRow mirrors a row of public.editor_games — the registry of games
// that have an editorial surface. Key matches the {gameKey} route segment.
type EditorGameRow struct {
	Key       string `db:"key"        json:"key"`
	Name      string `db:"name"       json:"name"`
	SortOrder int    `db:"sort_order" json:"sort_order"`
	IsActive  bool   `db:"is_active"  json:"is_active"`
}

// FetchEditorGames returns the active game registry in display order. Used to
// render the grant rows in the access console.
func (db *DB) FetchEditorGames(ctx context.Context) ([]EditorGameRow, error) {
	defer observe("fetch_editor_games", time.Now())
	rows, err := db.Pool.Query(ctx,
		`SELECT key, name, sort_order, is_active FROM editor_games
		 WHERE is_active ORDER BY sort_order, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[EditorGameRow])
}

// FetchEditorGameKeys returns every registered game key, including inactive
// ones. Grant validation uses this rather than FetchEditorGames: a deactivated
// game is hidden from the console but existing grants on it are still valid
// input, so validating against the active list alone would reject every save for
// a user who happens to hold one.
func (db *DB) FetchEditorGameKeys(ctx context.Context) (map[string]bool, error) {
	defer observe("fetch_editor_game_keys", time.Now())
	rows, err := db.Pool.Query(ctx, `SELECT key FROM editor_games`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make(map[string]bool)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys[key] = true
	}
	return keys, rows.Err()
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
