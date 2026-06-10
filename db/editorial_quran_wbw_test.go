package db

import (
	"context"
	"testing"
)

// Integration: drives the Phase 2 Quran word-by-word + root-meaning path against
// a live DB — word draft upsert, chapter words read, root-meaning draft with
// broadcast-by-JOIN, dedicated root publish, and word publish folded into the
// chapter approve. Skipped when no DATABASE_NAME is set (see setupTestDB).

func TestEditorialQuranWordsAndRoots_Integration(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	var langID int16
	if err := db.Pool.QueryRow(ctx, `SELECT id FROM languages ORDER BY id LIMIT 1`).Scan(&langID); err != nil {
		t.Fatalf("pick language: %v", err)
	}

	_, _ = db.Pool.Exec(ctx, `DELETE FROM quran_versions WHERE slug LIKE 'wbw-test-%'`)
	var versionID int
	if err := db.Pool.QueryRow(ctx,
		`INSERT INTO quran_versions (slug, name, lang_id) VALUES ('wbw-test-version', 'WBW Test', $1) RETURNING id`,
		langID).Scan(&versionID); err != nil {
		t.Fatalf("create version: %v", err)
	}

	chapter, err := db.FetchQuranChapterByNumber(ctx, 1)
	if err != nil || chapter == nil {
		t.Fatalf("fetch chapter 1: %v (chapter=%v)", err, chapter)
	}

	// Pick a real word in chapter 1 that carries a canonical Arabic root, so we
	// can exercise the broadcast-by-JOIN of its root meaning.
	var (
		wordID      int
		verseNumber int
		rootLetters string
	)
	err = db.Pool.QueryRow(ctx, `
		SELECT w.id, v.verse_index, arwt.word_root
		FROM words w
		JOIN verses v ON v.id = w.verse_id
		JOIN chapters c ON c.id = v.chapter_id
		JOIN word_text arwt ON arwt.word_id = w.id
		JOIN languages l ON l.id = arwt.lang_id AND l.code = 'ar'
		WHERE c.chapter_number = 1 AND arwt.word_root IS NOT NULL AND arwt.word_root <> ''
		ORDER BY v.verse_index, w.word_index
		LIMIT 1`).Scan(&wordID, &verseNumber, &rootLetters)
	if err != nil {
		t.Skipf("no rooted word in chapter 1 (corpus not seeded?): %v", err)
	}
	var rootID int
	if err := db.Pool.QueryRow(ctx, `SELECT id FROM quran_roots WHERE letters = $1`, rootLetters).Scan(&rootID); err != nil {
		t.Fatalf("resolve root id for %q: %v", rootLetters, err)
	}

	var userID int
	if err := db.Pool.QueryRow(ctx,
		`INSERT INTO users (auth_id, email) VALUES ($1, $2)
		 ON CONFLICT (email) DO UPDATE SET auth_id = EXCLUDED.auth_id RETURNING id`,
		"test:editorial-wbw", "editorial-wbw-test@example.com").Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	mustExec(t, ctx, db,
		`INSERT INTO editor_module_grants (user_id, module, can_write) VALUES ($1, 'quran', true)
		 ON CONFLICT (user_id, module) DO UPDATE SET can_write = true`, userID)
	mustExec(t, ctx, db,
		`INSERT INTO editor_quran_version_grants (user_id, version_id, can_write, can_approve)
		 VALUES ($1, $2, true, true)
		 ON CONFLICT (user_id, version_id) DO UPDATE SET can_write = true, can_approve = true`,
		userID, versionID)

	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM word_text_drafts WHERE version_id = $1`, versionID)
		_, _ = db.Pool.Exec(ctx, `DELETE FROM quran_versions WHERE id = $1`, versionID) // cascades root/word published, root drafts, requests, changelog, grants
		_, _ = db.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	// --- word draft ---
	if err := db.UpsertWordDraft(ctx, versionID, wordID, int(langID),
		QuranWordFields{Word: new("translit"), Meaning: new("gloss")}, userID); err != nil {
		t.Fatalf("upsert word draft: %v", err)
	}

	words, err := db.FetchQuranChapterWords(ctx, versionID, chapter.ID, 0, 0)
	if err != nil {
		t.Fatalf("fetch chapter words: %v", err)
	}
	w := findWord(words, verseNumber, wordID)
	if w == nil {
		t.Fatalf("word %d not found in chapter words", wordID)
	}
	if w.Draft == nil || w.Draft.Word == nil || *w.Draft.Word != "translit" {
		t.Fatalf("word draft text not reflected: %+v", w.Draft)
	}
	if w.DraftStatus == nil || *w.DraftStatus != "draft" {
		t.Fatalf("word draft status = %v, want draft", w.DraftStatus)
	}
	if w.Published != nil {
		t.Fatalf("word should not be published yet")
	}
	if w.Arabic == nil || *w.Arabic == "" {
		t.Fatalf("canonical arabic surface missing for word %d", wordID)
	}

	// --- root meaning draft broadcasts by JOIN ---
	if err := db.UpsertRootMeaningDraft(ctx, versionID, rootID, new("the-root-meaning"), userID); err != nil {
		t.Fatalf("upsert root meaning draft: %v", err)
	}
	root, err := db.FetchRootMeaning(ctx, versionID, rootID)
	if err != nil || root == nil {
		t.Fatalf("fetch root meaning: %v (root=%v)", err, root)
	}
	if root.MeaningDraft == nil || *root.MeaningDraft != "the-root-meaning" {
		t.Fatalf("root meaning draft = %v, want the-root-meaning", root.MeaningDraft)
	}
	if root.MeaningPublished != nil {
		t.Fatalf("root meaning should not be published yet")
	}

	words, err = db.FetchQuranChapterWords(ctx, versionID, chapter.ID, 0, 0)
	if err != nil {
		t.Fatalf("re-fetch chapter words: %v", err)
	}
	w = findWord(words, verseNumber, wordID)
	if w == nil || w.RootMeaning == nil || *w.RootMeaning != "the-root-meaning" {
		t.Fatalf("root meaning did not broadcast to word by JOIN: %+v", w)
	}
	if w.RootMeaningPublished != nil {
		t.Fatalf("word root_meaning_published should be nil before publish")
	}

	// --- dedicated root publish ---
	count, err := db.PublishRootMeanings(ctx, versionID, userID, new("ship roots"))
	if err != nil {
		t.Fatalf("publish root meanings: %v", err)
	}
	if count < 1 {
		t.Fatalf("expected at least 1 root meaning published, got %d", count)
	}
	root, err = db.FetchRootMeaning(ctx, versionID, rootID)
	if err != nil || root == nil {
		t.Fatalf("re-fetch root meaning: %v", err)
	}
	if root.MeaningPublished == nil || *root.MeaningPublished != "the-root-meaning" {
		t.Fatalf("root meaning not published: %v", root.MeaningPublished)
	}
	if root.DraftStatus == nil || *root.DraftStatus != "published" {
		t.Fatalf("root draft status after publish = %v, want published", root.DraftStatus)
	}

	// --- word publish folds into chapter approve ---
	req, err := db.CreatePublishRequest(ctx, versionID, 1, userID, nil)
	if err != nil {
		t.Fatalf("create publish request: %v", err)
	}
	if _, err := db.ApprovePublishRequest(ctx, req.ID, userID, nil); err != nil {
		t.Fatalf("approve: %v", err)
	}
	var publishedWord *string
	if err := db.Pool.QueryRow(ctx,
		`SELECT word FROM quran_word_text_published WHERE version_id = $1 AND word_id = $2`,
		versionID, wordID).Scan(&publishedWord); err != nil {
		t.Fatalf("read published word: %v", err)
	}
	if publishedWord == nil || *publishedWord != "translit" {
		t.Fatalf("published word = %v, want translit", publishedWord)
	}

	words, err = db.FetchQuranChapterWords(ctx, versionID, chapter.ID, 0, 0)
	if err != nil {
		t.Fatalf("final fetch chapter words: %v", err)
	}
	w = findWord(words, verseNumber, wordID)
	if w == nil || w.Published == nil || w.Published.Word == nil || *w.Published.Word != "translit" {
		t.Fatalf("word published text not reflected after approve: %+v", w)
	}
	if w.RootMeaningPublished == nil || *w.RootMeaningPublished != "the-root-meaning" {
		t.Fatalf("word root_meaning_published not reflected: %+v", w)
	}

	// Changelog recorded the root publish and the chapter publish.
	changelog, err := db.ListChangelog(ctx, versionID, 1, 50)
	if err != nil {
		t.Fatalf("list changelog: %v", err)
	}
	seen := map[string]bool{}
	for _, e := range changelog {
		seen[e.Action] = true
	}
	// chapter-scoped actions appear under chapter 1; root publish is version-scoped
	// (chapter_number NULL) so it will not show in this chapter-filtered list.
	for _, want := range []string{ActionPublishRequested, ActionPublishApproved, ActionPublished} {
		if !seen[want] {
			t.Errorf("changelog missing action %q (saw %v)", want, seen)
		}
	}
}

// findWord locates a word by verse number + word id in the assembled result.
func findWord(verses []QuranVerseWordsRow, verseNumber, wordID int) *QuranWordEditorRow {
	for vi := range verses {
		if verses[vi].VerseNumber != verseNumber {
			continue
		}
		for wi := range verses[vi].Words {
			if verses[vi].Words[wi].WordID == wordID {
				return &verses[vi].Words[wi]
			}
		}
	}
	return nil
}
