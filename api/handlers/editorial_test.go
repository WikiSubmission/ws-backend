package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wikiSubmission/ws-backend/db"
)

func TestEditorialSessionFromPerms(t *testing.T) {
	ref := 5
	perms := &db.EditorPermissions{
		IsAdmin:                 false,
		Modules:                 map[db.EditorModule]bool{db.ModuleQuran: true, db.ModuleArticle: false},
		QuranVersions:           map[int]bool{7: true, 9: false},
		BibleVersions:           map[int]bool{3: false},
		QuranReferenceVersionID: &ref,
	}

	got := editorialSessionFromPerms(perms)

	if got.IsAdmin {
		t.Error("IsAdmin should be false")
	}
	if got.Modules["quran"] != true || got.Modules["article"] != false {
		t.Errorf("modules not mapped: %#v", got.Modules)
	}
	// integer version ids must become string keys
	if got.QuranVersions["7"] != true || got.QuranVersions["9"] != false {
		t.Errorf("quran versions not mapped to string keys: %#v", got.QuranVersions)
	}
	if got.BibleVersions["3"] != false {
		t.Errorf("bible versions not mapped: %#v", got.BibleVersions)
	}
	if got.QuranReferenceVersionId == nil || *got.QuranReferenceVersionId != 5 {
		t.Errorf("reference version id not carried: %#v", got.QuranReferenceVersionId)
	}
}

func TestApplyUserRolesSeparatesCMSAndGames(t *testing.T) {
	makePerms := func() *db.EditorPermissions {
		return &db.EditorPermissions{
			Modules:       map[db.EditorModule]bool{db.ModuleArticle: true, db.ModuleAuthor: true, db.ModuleCategory: true, db.ModuleGame: true},
			QuranVersions: map[int]bool{1: true},
			BibleVersions: map[int]bool{2: true},
			Games:         map[string]bool{"fill-blank": true},
		}
	}

	member := makePerms()
	applyUserRoles(member, &db.UserRow{Roles: []string{"member"}})
	if len(member.Modules) != 0 || len(member.Games) != 0 || len(member.QuranVersions) != 0 {
		t.Errorf("member retained grants: %#v", member)
	}

	editor := makePerms()
	applyUserRoles(editor, &db.UserRow{Roles: []string{"member", "editor"}})
	if !editor.CanWriteModule(db.ModuleArticle) || len(editor.Games) != 0 {
		t.Errorf("editor role partition incorrect: %#v", editor)
	}
	if editor.CanReadModule(db.ModuleAuthor) || editor.CanReadModule(db.ModuleCategory) {
		t.Errorf("standalone author/category grants must be ignored: %#v", editor.Modules)
	}

	gameEditor := makePerms()
	applyUserRoles(gameEditor, &db.UserRow{Roles: []string{"member", "game_editor"}})
	if !gameEditor.CanWriteGame("fill-blank") || gameEditor.CanReadModule(db.ModuleArticle) {
		t.Errorf("game editor role partition incorrect: %#v", gameEditor)
	}

	admin := makePerms()
	admin.IsAdmin = true
	applyUserRoles(admin, &db.UserRow{Roles: []string{"member", "admin"}})
	if !admin.CanReadModule(db.ModuleAuthor) || !admin.CanWriteGame("anything") {
		t.Errorf("admin bypass changed: %#v", admin)
	}
}

func TestParseVersionAuthors(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantLen int
	}{
		{"empty bytes", "", 0},
		{"empty array", "[]", 0},
		{"one author", `[{"name":"R. Khalifa","bio":"translator"}]`, 1},
		{"two authors", `[{"name":"A"},{"name":"B"}]`, 2},
		{"malformed falls back to none", `{not json`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var raw json.RawMessage
			if tt.raw != "" {
				raw = json.RawMessage(tt.raw)
			}
			got := parseVersionAuthors(raw)
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestQuranVersionFromRow(t *testing.T) {
	now := time.Now()
	refID := 2
	row := &db.QuranVersionRow{
		ID:                 1,
		Slug:               "submission-es",
		Name:               "Submission · Español",
		LangID:             13,
		Authors:            json.RawMessage(`[{"name":"I. Mansour"}]`),
		Direction:          "ltr",
		IsCanonicalEnglish: false,
		ReferenceVersionID: &refID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	got := quranVersionFromRow(row)

	if got.Id != 1 || got.Slug != "submission-es" || got.LangId != 13 {
		t.Errorf("scalar fields not mapped: %#v", got)
	}
	if got.ReferenceVersionId == nil || *got.ReferenceVersionId != 2 {
		t.Errorf("reference version id not mapped: %#v", got.ReferenceVersionId)
	}
	if got.Authors == nil || len(*got.Authors) != 1 {
		t.Fatalf("authors not parsed: %#v", got.Authors)
	}
	if (*got.Authors)[0].Name == nil || *(*got.Authors)[0].Name != "I. Mansour" {
		t.Errorf("author name not parsed: %#v", (*got.Authors)[0])
	}
}

func TestBibleVersionFromRow_NoAuthors(t *testing.T) {
	row := &db.BibleVersionRow{ID: 4, Slug: "kjv", Name: "KJV", LangID: 2, Direction: "ltr"}
	got := bibleVersionFromRow(row)
	if got.Id != 4 || got.Slug != "kjv" {
		t.Errorf("scalar fields not mapped: %#v", got)
	}
	if got.Authors != nil {
		t.Errorf("authors should be nil when empty, got %#v", got.Authors)
	}
}
