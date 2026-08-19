package auth

import (
	"encoding/json"
	"testing"

	"github.com/wikiSubmission/ws-backend/db"
)

func TestProviderFromAuthID(t *testing.T) {
	cases := map[string]string{
		"google:12345":   "google",
		"apple:001234":   "apple",
		"email:user.com": "email",
		"":               "unknown",
		"missing-prefix": "unknown",
		":missing":       "unknown",
	}
	for input, want := range cases {
		if got := ProviderFromAuthID(input); got != want {
			t.Fatalf("ProviderFromAuthID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestHasPermission(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		key  string
		want bool
	}{
		{"granted", `{"games_editor":true}`, "games_editor", true},
		{"explicitly denied", `{"games_editor":false}`, "games_editor", false},
		{"missing key", `{"other":true}`, "games_editor", false},
		{"empty blob", `{}`, "games_editor", false},
		{"null raw", ``, "games_editor", false},
		{"non-bool value", `{"games_editor":"yes"}`, "games_editor", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var raw json.RawMessage
			if c.raw != "" {
				raw = json.RawMessage(c.raw)
			}
			if got := HasPermission(raw, c.key); got != c.want {
				t.Errorf("HasPermission(%q, %q) = %v, want %v", c.raw, c.key, got, c.want)
			}
		})
	}
}

// Games access now comes from the grant tables, not the legacy
// permissions.games_editor JSONB flag — so a user carrying only that flag is
// denied, and a user with a grant is allowed regardless of the flag.
func TestHasGamesEditorAccess(t *testing.T) {
	globalGrant := &db.EditorPermissions{Modules: map[db.EditorModule]bool{db.ModuleGame: false}}
	perGameGrant := &db.EditorPermissions{Games: map[string]bool{"fill-blank": true}}
	unrelatedGrant := &db.EditorPermissions{Modules: map[db.EditorModule]bool{db.ModuleArticle: true}}

	cases := []struct {
		name  string
		user  *db.UserRow
		perms *db.EditorPermissions
		want  bool
	}{
		{"nil user", nil, globalGrant, false},
		{"admin needs no grants", &db.UserRow{Role: "admin", IsActive: true}, nil, true},
		{"admin but inactive", &db.UserRow{Role: "admin", IsActive: false}, globalGrant, false},
		{"global games grant", &db.UserRow{Role: "member", Roles: []string{"member", "game_editor"}, IsActive: true}, globalGrant, true},
		{"per-game grant only", &db.UserRow{Role: "member", Roles: []string{"member", "game_editor"}, IsActive: true}, perGameGrant, true},
		{"grant without game editor role", &db.UserRow{Role: "member", Roles: []string{"member"}, IsActive: true}, globalGrant, false},
		{"unrelated module grant", &db.UserRow{Role: "editor", IsActive: true}, unrelatedGrant, false},
		{"no grants at all", &db.UserRow{Role: "editor", IsActive: true}, &db.EditorPermissions{}, false},
		{"nil perms denies non-admin", &db.UserRow{Role: "member", IsActive: true}, nil, false},
		{
			"legacy games_editor flag alone is no longer enough",
			&db.UserRow{Role: "member", IsActive: true, Permissions: json.RawMessage(`{"games_editor":true}`)},
			&db.EditorPermissions{},
			false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasGamesEditorAccess(c.user, c.perms); got != c.want {
				t.Errorf("HasGamesEditorAccess = %v, want %v", got, c.want)
			}
		})
	}
}
