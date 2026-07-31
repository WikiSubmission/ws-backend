package db

import (
	"context"
	"testing"
)

// ── pure decision-logic tests (no database required) ─────────────────────────

func TestEditorPermissions_NilSafe(t *testing.T) {
	var p *EditorPermissions
	if p.CanReadModule(ModuleQuran) {
		t.Error("nil perms must not grant module read")
	}
	if p.CanWriteModule(ModuleQuran) {
		t.Error("nil perms must not grant module write")
	}
	if p.CanReadQuranVersion(1) || p.CanWriteQuranVersion(1) {
		t.Error("nil perms must not grant quran version access")
	}
	if p.CanReadBibleVersion(1) || p.CanWriteBibleVersion(1) {
		t.Error("nil perms must not grant bible version access")
	}
	if p.HasAnyEditorialAccess() {
		t.Error("nil perms must not have editorial access")
	}
}

func TestEditorPermissions_Module(t *testing.T) {
	tests := []struct {
		name      string
		perms     *EditorPermissions
		module    EditorModule
		wantRead  bool
		wantWrite bool
	}{
		{
			name:      "admin reads and writes any module",
			perms:     &EditorPermissions{IsAdmin: true},
			module:    ModuleArticle,
			wantRead:  true,
			wantWrite: true,
		},
		{
			name:      "read-only grant: read yes, write no",
			perms:     &EditorPermissions{Modules: map[EditorModule]bool{ModuleArticle: false}},
			module:    ModuleArticle,
			wantRead:  true,
			wantWrite: false,
		},
		{
			name:      "write grant: read and write",
			perms:     &EditorPermissions{Modules: map[EditorModule]bool{ModuleArticle: true}},
			module:    ModuleArticle,
			wantRead:  true,
			wantWrite: true,
		},
		{
			name:      "no grant for module: neither",
			perms:     &EditorPermissions{Modules: map[EditorModule]bool{ModuleQuran: true}},
			module:    ModuleArticle,
			wantRead:  false,
			wantWrite: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.perms.CanReadModule(tt.module); got != tt.wantRead {
				t.Errorf("CanReadModule = %v, want %v", got, tt.wantRead)
			}
			if got := tt.perms.CanWriteModule(tt.module); got != tt.wantWrite {
				t.Errorf("CanWriteModule = %v, want %v", got, tt.wantWrite)
			}
		})
	}
}

func TestEditorPermissions_QuranVersion(t *testing.T) {
	tests := []struct {
		name      string
		perms     *EditorPermissions
		versionID int
		wantRead  bool
		wantWrite bool
	}{
		{
			name:      "admin bypasses module and version grants",
			perms:     &EditorPermissions{IsAdmin: true},
			versionID: 42,
			wantRead:  true,
			wantWrite: true,
		},
		{
			name: "version write requires the quran module grant",
			perms: &EditorPermissions{
				// version grant present but NO module grant -> denied
				QuranVersions: map[int]bool{7: true},
			},
			versionID: 7,
			wantRead:  false,
			wantWrite: false,
		},
		{
			name: "module read + version read-only",
			perms: &EditorPermissions{
				Modules:       map[EditorModule]bool{ModuleQuran: false},
				QuranVersions: map[int]bool{7: false},
			},
			versionID: 7,
			wantRead:  true,
			wantWrite: false,
		},
		{
			name: "module read + version write",
			perms: &EditorPermissions{
				Modules:       map[EditorModule]bool{ModuleQuran: false},
				QuranVersions: map[int]bool{7: true},
			},
			versionID: 7,
			wantRead:  true,
			wantWrite: true,
		},
		{
			name: "module granted but version not granted",
			perms: &EditorPermissions{
				Modules:       map[EditorModule]bool{ModuleQuran: true},
				QuranVersions: map[int]bool{7: true},
			},
			versionID: 99,
			wantRead:  false,
			wantWrite: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.perms.CanReadQuranVersion(tt.versionID); got != tt.wantRead {
				t.Errorf("CanReadQuranVersion = %v, want %v", got, tt.wantRead)
			}
			if got := tt.perms.CanWriteQuranVersion(tt.versionID); got != tt.wantWrite {
				t.Errorf("CanWriteQuranVersion = %v, want %v", got, tt.wantWrite)
			}
		})
	}
}

func TestEditorPermissions_BibleVersion(t *testing.T) {
	tests := []struct {
		name      string
		perms     *EditorPermissions
		versionID int
		wantRead  bool
		wantWrite bool
	}{
		{
			name:      "admin bypasses",
			perms:     &EditorPermissions{IsAdmin: true},
			versionID: 3,
			wantRead:  true,
			wantWrite: true,
		},
		{
			name: "version grant without bible module is denied",
			perms: &EditorPermissions{
				BibleVersions: map[int]bool{3: true},
			},
			versionID: 3,
			wantRead:  false,
			wantWrite: false,
		},
		{
			name: "module read + version write",
			perms: &EditorPermissions{
				Modules:       map[EditorModule]bool{ModuleBible: false},
				BibleVersions: map[int]bool{3: true},
			},
			versionID: 3,
			wantRead:  true,
			wantWrite: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.perms.CanReadBibleVersion(tt.versionID); got != tt.wantRead {
				t.Errorf("CanReadBibleVersion = %v, want %v", got, tt.wantRead)
			}
			if got := tt.perms.CanWriteBibleVersion(tt.versionID); got != tt.wantWrite {
				t.Errorf("CanWriteBibleVersion = %v, want %v", got, tt.wantWrite)
			}
		})
	}
}

func TestEditorPermissions_ApproveQuranVersion(t *testing.T) {
	tests := []struct {
		name       string
		perms      *EditorPermissions
		versionID  int
		wantApprov bool
	}{
		{"nil perms deny", nil, 1, false},
		{"admin approves any version", &EditorPermissions{IsAdmin: true}, 42, true},
		{
			name: "approver grant on the version (with module grant)",
			perms: &EditorPermissions{
				Modules:               map[EditorModule]bool{ModuleQuran: false},
				QuranApproverVersions: map[int]bool{7: true},
			},
			versionID:  7,
			wantApprov: true,
		},
		{
			name: "approver grant but no quran module grant is denied",
			perms: &EditorPermissions{
				QuranApproverVersions: map[int]bool{7: true},
			},
			versionID:  7,
			wantApprov: false,
		},
		{
			name: "approver grant on a different version",
			perms: &EditorPermissions{
				Modules:               map[EditorModule]bool{ModuleQuran: false},
				QuranApproverVersions: map[int]bool{7: true},
			},
			versionID:  8,
			wantApprov: false,
		},
		{
			name: "write but not approve",
			perms: &EditorPermissions{
				Modules:       map[EditorModule]bool{ModuleQuran: true},
				QuranVersions: map[int]bool{7: true},
			},
			versionID:  7,
			wantApprov: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.perms.CanApproveQuranVersion(tt.versionID); got != tt.wantApprov {
				t.Errorf("CanApproveQuranVersion = %v, want %v", got, tt.wantApprov)
			}
		})
	}
}

func TestEditorPermissions_ApprovableQuranVersionIDs(t *testing.T) {
	var nilPerms *EditorPermissions
	if got := nilPerms.ApprovableQuranVersionIDs(); got != nil {
		t.Errorf("nil perms = %v, want nil", got)
	}
	perms := &EditorPermissions{
		Modules:               map[EditorModule]bool{ModuleQuran: false},
		QuranApproverVersions: map[int]bool{3: true, 5: true, 9: false},
	}
	got := perms.ApprovableQuranVersionIDs()
	if noModule := (&EditorPermissions{QuranApproverVersions: map[int]bool{3: true}}).ApprovableQuranVersionIDs(); noModule != nil {
		t.Errorf("without quran module grant, approvable ids must be nil, got %v", noModule)
	}
	set := map[int]bool{}
	for _, id := range got {
		set[id] = true
	}
	if !set[3] || !set[5] {
		t.Errorf("expected 3 and 5 in %v", got)
	}
	if set[9] {
		t.Errorf("version with can_approve=false must be excluded: %v", got)
	}
}

func TestEditorPermissions_HasAnyEditorialAccess(t *testing.T) {
	tests := []struct {
		name  string
		perms *EditorPermissions
		want  bool
	}{
		{"admin", &EditorPermissions{IsAdmin: true}, true},
		{"one module grant", &EditorPermissions{Modules: map[EditorModule]bool{ModuleCommunity: false}}, true},
		{"empty maps", newEditorPermissions(false), false},
		{
			name: "only a version grant, no module -> no access surface",
			perms: &EditorPermissions{
				Modules:       map[EditorModule]bool{},
				QuranVersions: map[int]bool{1: true},
			},
			want: false,
		},
		{
			// Games live under /admin/games and have no content documents, so a
			// games-only editor must not be treated as having editorial access —
			// /editor would render them an empty workspace.
			name:  "games module only is not editorial access",
			perms: &EditorPermissions{Modules: map[EditorModule]bool{ModuleGame: true}},
			want:  false,
		},
		{
			name: "games module alongside a real module still counts",
			perms: &EditorPermissions{
				Modules: map[EditorModule]bool{ModuleGame: true, ModuleArticle: false},
			},
			want: true,
		},
		{
			name:  "per-game grant only is not editorial access",
			perms: &EditorPermissions{Games: map[string]bool{"fill-blank": true}},
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.perms.HasAnyEditorialAccess(); got != tt.want {
				t.Errorf("HasAnyEditorialAccess = %v, want %v", got, tt.want)
			}
		})
	}
}

// Game access is the one UNION in this model: unlike a version grant, which is
// only honored alongside its module grant, a per-game grant stands alone and the
// global ModuleGame grant covers every game. These tests pin that asymmetry —
// it is the whole point of per-game access.
func TestEditorPermissions_Game(t *testing.T) {
	tests := []struct {
		name      string
		perms     *EditorPermissions
		gameKey   string
		wantRead  bool
		wantWrite bool
	}{
		{
			name:      "admin reads and writes any game",
			perms:     &EditorPermissions{IsAdmin: true},
			gameKey:   "fill-blank",
			wantRead:  true,
			wantWrite: true,
		},
		{
			name:      "global read-only grant covers a game it has no row for",
			perms:     &EditorPermissions{Modules: map[EditorModule]bool{ModuleGame: false}},
			gameKey:   "some-future-game",
			wantRead:  true,
			wantWrite: false,
		},
		{
			name:      "global write grant writes any game",
			perms:     &EditorPermissions{Modules: map[EditorModule]bool{ModuleGame: true}},
			gameKey:   "some-future-game",
			wantRead:  true,
			wantWrite: true,
		},
		{
			// The union: no ModuleGame grant at all, yet the game is reachable.
			name:      "per-game read grant works without the module grant",
			perms:     &EditorPermissions{Games: map[string]bool{"fill-blank": false}},
			gameKey:   "fill-blank",
			wantRead:  true,
			wantWrite: false,
		},
		{
			name:      "per-game write grant works without the module grant",
			perms:     &EditorPermissions{Games: map[string]bool{"fill-blank": true}},
			gameKey:   "fill-blank",
			wantRead:  true,
			wantWrite: true,
		},
		{
			// The isolation guarantee the old single boolean could not give.
			name:      "grant on one game does not leak to another",
			perms:     &EditorPermissions{Games: map[string]bool{"fill-blank": true}},
			gameKey:   "other-game",
			wantRead:  false,
			wantWrite: false,
		},
		{
			name:      "global read plus per-game write escalates that one game",
			perms:     &EditorPermissions{Modules: map[EditorModule]bool{ModuleGame: false}, Games: map[string]bool{"fill-blank": true}},
			gameKey:   "fill-blank",
			wantRead:  true,
			wantWrite: true,
		},
		{
			name:      "no grants",
			perms:     newEditorPermissions(false),
			gameKey:   "fill-blank",
			wantRead:  false,
			wantWrite: false,
		},
		{
			name:      "unrelated module grant does not grant games",
			perms:     &EditorPermissions{Modules: map[EditorModule]bool{ModuleQuran: true}},
			gameKey:   "fill-blank",
			wantRead:  false,
			wantWrite: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.perms.CanReadGame(tt.gameKey); got != tt.wantRead {
				t.Errorf("CanReadGame(%q) = %v, want %v", tt.gameKey, got, tt.wantRead)
			}
			if got := tt.perms.CanWriteGame(tt.gameKey); got != tt.wantWrite {
				t.Errorf("CanWriteGame(%q) = %v, want %v", tt.gameKey, got, tt.wantWrite)
			}
		})
	}
}

func TestEditorPermissions_GameNilSafe(t *testing.T) {
	var p *EditorPermissions
	if p.CanReadGame("fill-blank") || p.CanWriteGame("fill-blank") {
		t.Error("nil perms must not grant game access")
	}
	if p.HasAnyGameAccess() || p.HasGlobalGameAccess() {
		t.Error("nil perms must not report game access")
	}
	if got := p.AccessibleGameKeys(); got != nil {
		t.Errorf("AccessibleGameKeys on nil = %v, want nil", got)
	}
}

func TestEditorPermissions_HasAnyGameAccess(t *testing.T) {
	tests := []struct {
		name  string
		perms *EditorPermissions
		want  bool
	}{
		{"admin", &EditorPermissions{IsAdmin: true}, true},
		{"global grant", &EditorPermissions{Modules: map[EditorModule]bool{ModuleGame: false}}, true},
		{"per-game grant", &EditorPermissions{Games: map[string]bool{"fill-blank": false}}, true},
		{"no grants", newEditorPermissions(false), false},
		{"other modules only", &EditorPermissions{Modules: map[EditorModule]bool{ModuleArticle: true}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.perms.HasAnyGameAccess(); got != tt.want {
				t.Errorf("HasAnyGameAccess = %v, want %v", got, tt.want)
			}
		})
	}
}

// A global games editor holds no per-game rows, so callers must not read an
// empty AccessibleGameKeys as "no games" — HasGlobalGameAccess answers that.
func TestEditorPermissions_AccessibleGameKeys(t *testing.T) {
	global := &EditorPermissions{Modules: map[EditorModule]bool{ModuleGame: true}}
	if got := global.AccessibleGameKeys(); len(got) != 0 {
		t.Errorf("global grant should hold no per-game rows, got %v", got)
	}
	if !global.HasGlobalGameAccess() {
		t.Error("global grant must report HasGlobalGameAccess")
	}

	perGame := &EditorPermissions{Games: map[string]bool{"fill-blank": true}}
	if got := perGame.AccessibleGameKeys(); len(got) != 1 || got[0] != "fill-blank" {
		t.Errorf("AccessibleGameKeys = %v, want [fill-blank]", got)
	}
	if perGame.HasGlobalGameAccess() {
		t.Error("a per-game grant must not report global access")
	}
}

// ── integration tests (require a live database; skipped otherwise) ────────────

// TestFetchEditorPermissions_Integration inserts a temporary user with module
// grants and verifies FetchEditorPermissions reconstructs them. Cleans up via
// ON DELETE CASCADE by removing the user.
func TestFetchEditorPermissions_Integration(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	authID := "test:editorial-perms-" + t.Name()
	var userID int
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO users (auth_id, email) VALUES ($1, $2)
		 ON CONFLICT (email) DO UPDATE SET auth_id = EXCLUDED.auth_id
		 RETURNING id`,
		authID, "editorial-perms-test@example.com").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	_, err = db.Pool.Exec(ctx,
		`INSERT INTO editor_module_grants (user_id, module, can_write) VALUES
		   ($1, 'article', true),
		   ($1, 'quran', false)
		 ON CONFLICT (user_id, module) DO UPDATE SET can_write = EXCLUDED.can_write`,
		userID)
	if err != nil {
		t.Fatalf("insert module grants: %v", err)
	}

	perms, err := db.FetchEditorPermissions(ctx, userID, false)
	if err != nil {
		t.Fatalf("FetchEditorPermissions: %v", err)
	}
	if !perms.CanWriteModule(ModuleArticle) {
		t.Error("expected write on article")
	}
	if perms.CanWriteModule(ModuleQuran) {
		t.Error("expected no write on quran (read-only grant)")
	}
	if !perms.CanReadModule(ModuleQuran) {
		t.Error("expected read on quran")
	}
	if perms.CanReadModule(ModuleBible) {
		t.Error("expected no access to bible")
	}
}

func TestFetchQuranVersions_Integration(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	// Should not error even when empty.
	if _, err := db.FetchQuranVersions(ctx); err != nil {
		t.Fatalf("FetchQuranVersions: %v", err)
	}
	if _, err := db.FetchBibleVersions(ctx); err != nil {
		t.Fatalf("FetchBibleVersions: %v", err)
	}
}
