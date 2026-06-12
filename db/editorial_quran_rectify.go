package db

import (
	"context"
	"fmt"
	"time"
)

// Editorial Quran rectification (admin). In the Postgres model the canonical
// corpus (chapters / verses / words) is complete and version-agnostic, so the
// Sanity-era "missing shell" problem largely disappears: the editor renders the
// canonical word list and LEFT-JOINs drafts, so a word with no draft just shows
// an empty field. Rectification therefore reduces to three admin operations:
//   1. a read-only integrity + coverage report,
//   2. syncing the roots master from the corpus (the one real repair -- without
//      it, words whose root was added after the one-time backfill cannot have a
//      root meaning attached, since the word->meaning JOIN keys on quran_roots),
//   3. optionally pre-seeding empty draft shells so coverage materializes.
// Authorization (admin-only) is enforced in the handlers.

// ActionDraftShellsSeeded labels a draft-shell seeding event in the changelog.
const ActionDraftShellsSeeded = "draft_shells_seeded"

// QuranIntegrityReport is the admin diagnostic snapshot for one version. The
// corpus fields are whole-corpus (version-agnostic); the coverage fields are
// scoped to the requested version.
type QuranIntegrityReport struct {
	MissingRoots          int
	OrphanRoots           int
	WordsMissingArabic    int
	TotalWords            int
	WordsWithDraft        int
	WordsPublished        int
	TotalRoots            int
	RootMeaningDrafts     int
	RootMeaningsPublished int
}

// FetchQuranIntegrityReport assembles the rectification report in one round trip.
// The corpus checks scan word_text / quran_roots; they are admin-only and run on
// demand, so the heavier DISTINCT/EXCEPT is acceptable here.
func (db *DB) FetchQuranIntegrityReport(ctx context.Context, versionID int) (*QuranIntegrityReport, error) {
	defer observe("fetch_quran_integrity_report", time.Now())
	var r QuranIntegrityReport
	err := db.Pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM (
		      SELECT DISTINCT word_root FROM word_text
		      WHERE word_root IS NOT NULL AND word_root <> ''
		      EXCEPT SELECT letters FROM quran_roots) m)                          AS missing_roots,
		  (SELECT count(*) FROM quran_roots r
		      WHERE NOT EXISTS (SELECT 1 FROM word_text wt WHERE wt.word_root = r.letters)) AS orphan_roots,
		  (SELECT count(*) FROM words w
		      WHERE NOT EXISTS (
		          SELECT 1 FROM word_text wt
		          JOIN languages l ON wt.lang_id = l.id AND l.code = 'ar'
		          WHERE wt.word_id = w.id))                                        AS words_missing_arabic,
		  (SELECT count(*) FROM words)                                            AS total_words,
		  (SELECT count(*) FROM word_text_drafts WHERE version_id = $1)            AS words_with_draft,
		  (SELECT count(*) FROM quran_word_text_published WHERE version_id = $1)   AS words_published,
		  (SELECT count(*) FROM quran_roots)                                      AS total_roots,
		  (SELECT count(*) FROM quran_root_meaning_drafts WHERE version_id = $1)   AS root_meaning_drafts,
		  (SELECT count(*) FROM quran_root_meanings_published WHERE version_id = $1) AS root_meanings_published`,
		versionID).Scan(
		&r.MissingRoots, &r.OrphanRoots, &r.WordsMissingArabic,
		&r.TotalWords, &r.WordsWithDraft, &r.WordsPublished,
		&r.TotalRoots, &r.RootMeaningDrafts, &r.RootMeaningsPublished)
	if err != nil {
		return nil, fmt.Errorf("fetch_quran_integrity_report: %w", err)
	}
	return &r, nil
}

// SyncQuranRoots inserts every canonical Arabic root present in the corpus but
// missing from the roots master and returns the number inserted. Idempotent and
// version-agnostic; a no-op once the master is in step with the corpus.
func (db *DB) SyncQuranRoots(ctx context.Context) (int, error) {
	defer observe("sync_quran_roots", time.Now())
	tag, err := db.Pool.Exec(ctx, `
		INSERT INTO quran_roots (letters)
		SELECT DISTINCT word_root FROM word_text
		WHERE word_root IS NOT NULL AND word_root <> ''
		ON CONFLICT (letters) DO NOTHING`)
	if err != nil {
		return 0, fmt.Errorf("sync_quran_roots: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// SeedDraftShells creates empty draft rows for a version so its coverage
// materializes: one word draft per canonical word (optionally limited to one
// chapter) and, when seedRoots is set, one root meaning draft per root. Existing
// drafts are never overwritten (insert-if-absent), so seeding is safe to repeat
// and cannot clobber editor work. langID is the version's language. Records a
// changelog entry and returns the number of word and root shells actually created.
func (db *DB) SeedDraftShells(ctx context.Context, versionID, langID int, chapterNumber *int, seedRoots bool, actorID int) (wordShells, rootShells int, err error) {
	defer observe("seed_draft_shells", time.Now())
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Insert-if-absent against the partial unique index (version_id, word_id).
	wtag, err := tx.Exec(ctx, `
		INSERT INTO word_text_drafts (word_id, lang_id, version_id, status, author_id)
		SELECT w.id, $2, $1, 'draft', $3
		FROM words w
		JOIN verses v ON v.id = w.verse_id
		JOIN chapters c ON c.id = v.chapter_id
		WHERE ($4::int IS NULL OR c.chapter_number = $4)
		ON CONFLICT (version_id, word_id) WHERE version_id IS NOT NULL DO NOTHING`,
		versionID, langID, actorID, chapterNumber)
	if err != nil {
		return 0, 0, fmt.Errorf("seed_draft_shells: words: %w", err)
	}
	wordShells = int(wtag.RowsAffected())

	if seedRoots {
		rtag, err := tx.Exec(ctx, `
			INSERT INTO quran_root_meaning_drafts (version_id, root_id, status, author_id)
			SELECT $1, r.id, 'draft', $2
			FROM quran_roots r
			ON CONFLICT (version_id, root_id) DO NOTHING`,
			versionID, actorID)
		if err != nil {
			return 0, 0, fmt.Errorf("seed_draft_shells: roots: %w", err)
		}
		rootShells = int(rtag.RowsAffected())
	}

	// A repeat seed that inserts nothing is a no-op; don't pollute the changelog.
	if wordShells == 0 && rootShells == 0 {
		return 0, 0, nil
	}

	detail := map[string]any{"word_shells": wordShells, "root_shells": rootShells}
	if chapterNumber != nil {
		detail["chapter_number"] = *chapterNumber
	}
	if err := insertChangelog(ctx, tx, ChangelogEntry{
		VersionID: versionID, ChapterNumber: chapterNumber, Action: ActionDraftShellsSeeded,
		ActorID: actorID, Detail: detail,
	}); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("seed_draft_shells: commit: %w", err)
	}
	return wordShells, rootShells, nil
}
