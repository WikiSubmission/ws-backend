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

func TestHasGamesEditorAccess(t *testing.T) {
	cases := []struct {
		name string
		user *db.UserRow
		want bool
	}{
		{"nil", nil, false},
		{"admin", &db.UserRow{Role: "admin", IsActive: true}, true},
		{"editor with permission", &db.UserRow{Role: "editor", IsActive: true, Permissions: json.RawMessage(`{"games_editor":true}`)}, true},
		{"member with permission", &db.UserRow{Role: "member", IsActive: true, Permissions: json.RawMessage(`{"games_editor":true}`)}, true},
		{"editor without permission", &db.UserRow{Role: "editor", IsActive: true}, false},
		{"admin but inactive", &db.UserRow{Role: "admin", IsActive: false}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasGamesEditorAccess(c.user); got != c.want {
				t.Errorf("HasGamesEditorAccess = %v, want %v", got, c.want)
			}
		})
	}
}
