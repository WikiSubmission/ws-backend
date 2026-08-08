package handlers

import (
	"testing"

	"github.com/wikiSubmission/ws-backend/db"
)

func testContentServer() *Server {
	return &Server{Languages: map[string]LanguageCache{
		"en": {ID: 1, Code: "en", Direction: "ltr"},
		"ar": {ID: 2, Code: "ar", Direction: "rtl"},
	}}
}

func TestContentModuleFromParam(t *testing.T) {
	cases := map[EditorialContentModule]db.EditorModule{
		EditorialContentModuleArticle:   db.ModuleArticle,
		EditorialContentModuleAuthor:    db.ModuleAuthor,
		EditorialContentModuleCategory:  db.ModuleCategory,
		EditorialContentModuleCommunity: db.ModuleCommunity,
		EditorialContentModuleAppendix:  db.ModuleAppendix,
	}
	for wire, want := range cases {
		got, ok := contentModuleFromParam(wire)
		if !ok || got != want {
			t.Errorf("contentModuleFromParam(%q) = (%q, %v), want (%q, true)", wire, got, ok, want)
		}
	}
	if _, ok := contentModuleFromParam(EditorialContentModule("quran")); ok {
		t.Error("quran is not a content module and must be rejected")
	}
}

func TestValidateArticleFields(t *testing.T) {
	s := testContentServer()

	t.Run("valid article extracts scalars", func(t *testing.T) {
		scalars, err := s.validateContentFields(db.ModuleArticle, map[string]any{
			"title":     "The Creator's Signature",
			"slug":      "creators-signature",
			"language":  "en",
			"author_id": float64(4),
			"excerpt":   "On the mathematical structure.",
			"body":      []any{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if scalars.Slug == nil || *scalars.Slug != "creators-signature" {
			t.Errorf("slug not extracted: %#v", scalars.Slug)
		}
		if scalars.LangID == nil || *scalars.LangID != 1 {
			t.Errorf("lang_id not extracted: %#v", scalars.LangID)
		}
		if scalars.AuthorID == nil || *scalars.AuthorID != 4 {
			t.Errorf("author_id not extracted: %#v", scalars.AuthorID)
		}
	})

	t.Run("enable_scripture_refs is accepted", func(t *testing.T) {
		if _, err := s.validateContentFields(db.ModuleArticle, map[string]any{
			"title": "T", "slug": "t", "language": "en",
			"enable_scripture_refs": false,
		}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	fail := func(name string, fields map[string]any) {
		t.Run(name, func(t *testing.T) {
			if _, err := s.validateContentFields(db.ModuleArticle, fields); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}

	fail("enable_scripture_refs must be a boolean", map[string]any{
		"title": "T", "slug": "t", "language": "en",
		"enable_scripture_refs": "yes",
	})
	fail("missing title", map[string]any{"slug": "x", "language": "en"})
	fail("missing slug", map[string]any{"title": "T", "language": "en"})
	fail("bad slug format", map[string]any{"title": "T", "slug": "Bad Slug!", "language": "en"})
	fail("unknown language", map[string]any{"title": "T", "slug": "t", "language": "xx"})
	fail("unknown key", map[string]any{"title": "T", "slug": "t", "language": "en", "bogus": 1})
	fail("author_id not an int", map[string]any{"title": "T", "slug": "t", "language": "en", "author_id": "four"})
	fail("body not an array", map[string]any{"title": "T", "slug": "t", "language": "en", "body": "text"})
}

func TestValidateAuthorCategoryFields(t *testing.T) {
	s := testContentServer()

	scalars, err := s.validateContentFields(db.ModuleAuthor, map[string]any{
		"first_name": "Rashad",
		"last_name":  "Khalifa",
		"slug":       "rashad-khalifa",
		"bio":        "Translator.",
		"user_id":    float64(9),
	})
	if err != nil {
		t.Fatalf("author: unexpected error: %v", err)
	}
	if scalars.UserID == nil || *scalars.UserID != 9 {
		t.Errorf("author user_id not extracted: %#v", scalars.UserID)
	}

	// last_name is optional so single-name (mononym) authors are valid.
	if _, err := s.validateContentFields(db.ModuleAuthor, map[string]any{
		"first_name": "R", "slug": "r",
	}); err != nil {
		t.Errorf("author without last_name must be accepted: %v", err)
	}

	if _, err := s.validateContentFields(db.ModuleAuthor, map[string]any{
		"last_name": "Khalifa", "slug": "khalifa",
	}); err == nil {
		t.Error("author without first_name must fail")
	}

	if _, err := s.validateContentFields(db.ModuleCategory, map[string]any{
		"name": "Practices", "slug": "practices", "description": "How to worship.",
	}); err != nil {
		t.Errorf("category: unexpected error: %v", err)
	}
	if _, err := s.validateContentFields(db.ModuleCategory, map[string]any{"slug": "x"}); err == nil {
		t.Error("category without name must fail")
	}
}

func TestValidateCommunityFields(t *testing.T) {
	s := testContentServer()

	if _, err := s.validateContentFields(db.ModuleCommunity, map[string]any{
		"kind": "online", "name": "Discord", "slug": "discord",
		"platform": "discord", "url": "https://discord.gg/x",
		"tags": []any{"chat"}, "is_active": true, "member_count": float64(1200),
	}); err != nil {
		t.Errorf("online community: unexpected error: %v", err)
	}

	if _, err := s.validateContentFields(db.ModuleCommunity, map[string]any{
		"kind": "meetup", "name": "N", "slug": "n",
	}); err == nil {
		t.Error("invalid kind must fail")
	}
	if _, err := s.validateContentFields(db.ModuleCommunity, map[string]any{
		"kind": "physical", "name": "N", "slug": "n", "member_count": "many",
	}); err == nil {
		t.Error("non-integer member_count must fail")
	}
}

func TestValidateAppendixFields(t *testing.T) {
	s := testContentServer()

	scalars, err := s.validateContentFields(db.ModuleAppendix, map[string]any{
		"version_id": float64(3), "code": "19", "title": "One of the Great Miracles",
		"snippet": "…", "body": "## One of the Great Miracles\n\nText.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scalars.VersionID == nil || *scalars.VersionID != 3 {
		t.Errorf("version_id not extracted: %#v", scalars.VersionID)
	}
	if scalars.Code == nil || *scalars.Code != "19" {
		t.Errorf("code not extracted: %#v", scalars.Code)
	}

	if _, err := s.validateContentFields(db.ModuleAppendix, map[string]any{
		"code": "19", "title": "T",
	}); err == nil {
		t.Error("appendix without version_id must fail")
	}
	if _, err := s.validateContentFields(db.ModuleAppendix, map[string]any{
		"version_id": float64(3), "code": "Not Valid!", "title": "T",
	}); err == nil {
		t.Error("bad appendix code must fail")
	}
}

func TestPrepareContentWrite(t *testing.T) {
	s := testContentServer()

	t.Run("nil body rejected", func(t *testing.T) {
		if _, _, _, err := s.prepareContentWrite(db.ModuleCategory, nil); err == nil {
			t.Error("expected error for nil input")
		}
	})

	t.Run("translation_group rejected outside articles", func(t *testing.T) {
		group := "3b241101-e2bb-4255-8caf-4136c566a962"
		_, _, _, err := s.prepareContentWrite(db.ModuleCategory, &EditorialContentInput{
			Fields:           map[string]any{"name": "N", "slug": "n"},
			TranslationGroup: &group,
		})
		if err == nil {
			t.Error("expected error for translation_group on category")
		}
	})

	t.Run("bad uuid rejected", func(t *testing.T) {
		group := "not-a-uuid"
		_, _, _, err := s.prepareContentWrite(db.ModuleArticle, &EditorialContentInput{
			Fields:           map[string]any{"title": "T", "slug": "t", "language": "en"},
			TranslationGroup: &group,
		})
		if err == nil {
			t.Error("expected error for malformed translation_group")
		}
	})

	t.Run("valid group accepted", func(t *testing.T) {
		group := "3b241101-e2bb-4255-8caf-4136c566a962"
		draft, _, gotGroup, err := s.prepareContentWrite(db.ModuleArticle, &EditorialContentInput{
			Fields:           map[string]any{"title": "T", "slug": "t", "language": "ar"},
			TranslationGroup: &group,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotGroup == nil || *gotGroup != group {
			t.Errorf("group not carried: %#v", gotGroup)
		}
		if len(draft) == 0 {
			t.Error("draft payload must be serialized")
		}
	})
}

func TestEditorFromGrantsRow(t *testing.T) {
	ref := 5
	name := "Editor One"
	row := &db.EditorGrantsRow{
		UserID:      12,
		Email:       "editor@example.org",
		DisplayName: &name,
		Role:        "editor",
		IsActive:    true,
		Modules:     map[string]bool{"article": true, "category": false},
		QuranVersions: []db.EditorVersionGrantRow{
			{VersionID: 1, CanWrite: true, CanApprove: true},
		},
		BibleVersions: []db.EditorVersionGrantRow{
			{VersionID: 2, CanWrite: false},
		},
		QuranReferenceVersionID: &ref,
	}
	got := editorFromGrantsRow(row)
	if got.UserId != 12 || got.Email != "editor@example.org" || !got.IsActive {
		t.Errorf("identity not mapped: %#v", got)
	}
	if got.Modules["article"] != true || got.Modules["category"] != false {
		t.Errorf("modules not mapped: %#v", got.Modules)
	}
	if len(got.QuranVersions) != 1 || got.QuranVersions[0].VersionId != 1 ||
		got.QuranVersions[0].CanApprove == nil || !*got.QuranVersions[0].CanApprove {
		t.Errorf("quran grants not mapped: %#v", got.QuranVersions)
	}
	if len(got.BibleVersions) != 1 || got.BibleVersions[0].CanWrite {
		t.Errorf("bible grants not mapped: %#v", got.BibleVersions)
	}
	if got.QuranReferenceVersionId == nil || *got.QuranReferenceVersionId != 5 {
		t.Errorf("reference pref not mapped: %#v", got.QuranReferenceVersionId)
	}
}
