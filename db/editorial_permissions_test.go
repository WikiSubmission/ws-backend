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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.perms.HasAnyEditorialAccess(); got != tt.want {
				t.Errorf("HasAnyEditorialAccess = %v, want %v", got, tt.want)
			}
		})
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
