package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Editorial Quran reads and draft upserts (Phase 1 write path). Published reads
// come from the version-keyed editorial tables introduced in migration 019
// (quran_verse_text_published / quran_chapter_text_published); the live public
// reader corpus (verse_text / chapter_text) is never written here. The
// reference rail falls back to the legacy reader corpus by lang_id so an
// as-yet-unmigrated reference version (e.g. canonical English) still renders.

// QuranText holds the three editable verse fields.
type QuranText struct {
	Subtitle *string
	Content  *string
	Footer   *string
}

// QuranChapterMeta is canonical chapter metadata.
type QuranChapterMeta struct {
	ID            int
	ChapterNumber int
	VerseCount    int
}

// QuranVerseEditorRow is the assembled per-verse editing surface.
type QuranVerseEditorRow struct {
	VerseNumber    int
	Draft          *QuranText
	DraftStatus    *string
	DraftUpdatedAt *time.Time
	Published      *QuranText
	Reference      *QuranText
}

// QuranChapterProgress summarizes one chapter for the navigation list.
type QuranChapterProgress struct {
	ChapterNumber   int
	VerseCount      int
	Title           *string
	HasTitleDraft   bool
	DraftVerseCount int
}

// FetchQuranVersionByID returns one version, or (nil, nil) if absent.
func (db *DB) FetchQuranVersionByID(ctx context.Context, id int) (*QuranVersionRow, error) {
	defer observe("fetch_quran_version_by_id", time.Now())
	rows, err := db.Pool.Query(ctx,
		`SELECT id, slug, name, lang_id, authors, direction, is_canonical_english,
		        reference_version_id, created_at, updated_at
		 FROM quran_versions WHERE id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("fetch_quran_version_by_id: %w", err)
	}
	v, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[QuranVersionRow])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetch_quran_version_by_id: %w", err)
	}
	return &v, nil
}

// FetchQuranChapterByNumber returns canonical chapter metadata, or (nil, nil).
func (db *DB) FetchQuranChapterByNumber(ctx context.Context, chapterNumber int) (*QuranChapterMeta, error) {
	defer observe("fetch_quran_chapter_by_number", time.Now())
	var m QuranChapterMeta
	err := db.Pool.QueryRow(ctx,
		`SELECT id, chapter_number, verse_count FROM chapters WHERE chapter_number = $1`,
		chapterNumber).Scan(&m.ID, &m.ChapterNumber, &m.VerseCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetch_quran_chapter_by_number: %w", err)
	}
	return &m, nil
}

// FetchVerseID resolves the verses.id for a chapter+verse number, or (0, false).
func (db *DB) FetchVerseID(ctx context.Context, chapterID, verseNumber int) (int, bool, error) {
	var id int
	err := db.Pool.QueryRow(ctx,
		`SELECT id FROM verses WHERE chapter_id = $1 AND verse_index = $2`,
		chapterID, verseNumber).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("fetch_verse_id: %w", err)
	}
	return id, true, nil
}

// FetchQuranChapterProgress returns one summary per chapter for a version: draft
// title (if any) and how many verses have a draft.
func (db *DB) FetchQuranChapterProgress(ctx context.Context, versionID int) ([]QuranChapterProgress, error) {
	defer observe("fetch_quran_chapter_progress", time.Now())
	rows, err := db.Pool.Query(ctx, `
		SELECT c.chapter_number,
		       c.verse_count,
		       cd.title,
		       (cd.id IS NOT NULL)                 AS has_title_draft,
		       COALESCE(vd.draft_verses, 0)        AS draft_verse_count
		FROM chapters c
		LEFT JOIN chapter_text_drafts cd
		       ON cd.chapter_id = c.id AND cd.version_id = $1
		LEFT JOIN (
		    SELECT v.chapter_id, count(*) AS draft_verses
		    FROM verse_text_drafts d
		    JOIN verses v ON v.id = d.verse_id
		    WHERE d.version_id = $1
		    GROUP BY v.chapter_id
		) vd ON vd.chapter_id = c.id
		ORDER BY c.chapter_number`, versionID)
	if err != nil {
		return nil, fmt.Errorf("fetch_quran_chapter_progress: %w", err)
	}
	defer rows.Close()
	var out []QuranChapterProgress
	for rows.Next() {
		var p QuranChapterProgress
		if err := rows.Scan(&p.ChapterNumber, &p.VerseCount, &p.Title, &p.HasTitleDraft, &p.DraftVerseCount); err != nil {
			return nil, fmt.Errorf("fetch_quran_chapter_progress: scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// FetchChapterTitleDraft returns the chapter-title draft for a version+chapter.
func (db *DB) FetchChapterTitleDraft(ctx context.Context, versionID, chapterID int) (*string, error) {
	var title *string
	err := db.Pool.QueryRow(ctx,
		`SELECT title FROM chapter_text_drafts WHERE version_id = $1 AND chapter_id = $2`,
		versionID, chapterID).Scan(&title)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetch_chapter_title_draft: %w", err)
	}
	return title, nil
}

// FetchChapterTitlePublished returns the published editorial chapter title.
func (db *DB) FetchChapterTitlePublished(ctx context.Context, versionID, chapterID int) (*string, error) {
	var title *string
	err := db.Pool.QueryRow(ctx,
		`SELECT title FROM quran_chapter_text_published WHERE version_id = $1 AND chapter_id = $2`,
		versionID, chapterID).Scan(&title)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetch_chapter_title_published: %w", err)
	}
	return title, nil
}

// FetchQuranChapterEditorVerses assembles draft + published + reference text for
// every verse of a chapter, keyed by verse number. referenceVersionID and
// referenceLangID may be nil (no reference version configured).
func (db *DB) FetchQuranChapterEditorVerses(ctx context.Context, versionID, chapterID int, referenceVersionID, referenceLangID *int) ([]QuranVerseEditorRow, error) {
	defer observe("fetch_quran_chapter_editor_verses", time.Now())

	verseCount, byNum, err := db.scanChapterVerseNumbers(ctx, chapterID)
	if err != nil {
		return nil, err
	}
	if err := db.scanChapterDrafts(ctx, versionID, chapterID, byNum); err != nil {
		return nil, err
	}
	if err := db.scanChapterPublished(ctx, versionID, chapterID, byNum); err != nil {
		return nil, err
	}
	if referenceVersionID != nil && referenceLangID != nil {
		if err := db.scanChapterReference(ctx, *referenceVersionID, *referenceLangID, chapterID, byNum); err != nil {
			return nil, err
		}
	}

	out := make([]QuranVerseEditorRow, 0, verseCount)
	for n := 1; n <= verseCount; n++ {
		if row, ok := byNum[n]; ok {
			out = append(out, *row)
		} else {
			out = append(out, QuranVerseEditorRow{VerseNumber: n})
		}
	}
	return out, nil
}

// scanChapterVerseNumbers seeds the per-verse-number map from canonical verses.
func (db *DB) scanChapterVerseNumbers(ctx context.Context, chapterID int) (int, map[int]*QuranVerseEditorRow, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT verse_index FROM verses WHERE chapter_id = $1 ORDER BY verse_index`, chapterID)
	if err != nil {
		return 0, nil, fmt.Errorf("scan_chapter_verse_numbers: %w", err)
	}
	defer rows.Close()
	byNum := make(map[int]*QuranVerseEditorRow)
	count := 0
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return 0, nil, fmt.Errorf("scan_chapter_verse_numbers: scan: %w", err)
		}
		byNum[n] = &QuranVerseEditorRow{VerseNumber: n}
		count++
	}
	return count, byNum, rows.Err()
}

// scanChapterDrafts fills draft text/status/updated_at by verse number.
func (db *DB) scanChapterDrafts(ctx context.Context, versionID, chapterID int, byNum map[int]*QuranVerseEditorRow) error {
	rows, err := db.Pool.Query(ctx, `
		SELECT v.verse_index, d.subtitle, d.content, d.footer, d.status::text, d.updated_at
		FROM verse_text_drafts d
		JOIN verses v ON v.id = d.verse_id
		WHERE d.version_id = $1 AND v.chapter_id = $2`, versionID, chapterID)
	if err != nil {
		return fmt.Errorf("scan_chapter_drafts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n int
		var t QuranText
		var status string
		var updated time.Time
		if err := rows.Scan(&n, &t.Subtitle, &t.Content, &t.Footer, &status, &updated); err != nil {
			return fmt.Errorf("scan_chapter_drafts: scan: %w", err)
		}
		if row, ok := byNum[n]; ok {
			text := t
			st := status
			up := updated
			row.Draft = &text
			row.DraftStatus = &st
			row.DraftUpdatedAt = &up
		}
	}
	return rows.Err()
}

// scanChapterPublished fills published editorial text by verse number.
func (db *DB) scanChapterPublished(ctx context.Context, versionID, chapterID int, byNum map[int]*QuranVerseEditorRow) error {
	rows, err := db.Pool.Query(ctx, `
		SELECT v.verse_index, p.subtitle, p.content, p.footer
		FROM quran_verse_text_published p
		JOIN verses v ON v.id = p.verse_id
		WHERE p.version_id = $1 AND v.chapter_id = $2`, versionID, chapterID)
	if err != nil {
		return fmt.Errorf("scan_chapter_published: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n int
		var t QuranText
		if err := rows.Scan(&n, &t.Subtitle, &t.Content, &t.Footer); err != nil {
			return fmt.Errorf("scan_chapter_published: scan: %w", err)
		}
		if row, ok := byNum[n]; ok {
			text := t
			row.Published = &text
		}
	}
	return rows.Err()
}

// scanChapterReference fills the reference rail: published editorial text of the
// reference version, falling back to the legacy reader corpus by lang for verses
// the reference version has not published editorially.
func (db *DB) scanChapterReference(ctx context.Context, refVersionID, refLangID, chapterID int, byNum map[int]*QuranVerseEditorRow) error {
	// Legacy fallback first, so editorial values overwrite it.
	legacy, err := db.Pool.Query(ctx, `
		SELECT v.verse_index, vt.subtitle, vt.content, vt.footer
		FROM verse_text vt
		JOIN verses v ON v.id = vt.verse_id
		WHERE vt.lang_id = $1 AND v.chapter_id = $2`, refLangID, chapterID)
	if err != nil {
		return fmt.Errorf("scan_chapter_reference: legacy: %w", err)
	}
	if err := applyReferenceRows(legacy, byNum); err != nil {
		return err
	}
	editorial, err := db.Pool.Query(ctx, `
		SELECT v.verse_index, p.subtitle, p.content, p.footer
		FROM quran_verse_text_published p
		JOIN verses v ON v.id = p.verse_id
		WHERE p.version_id = $1 AND v.chapter_id = $2`, refVersionID, chapterID)
	if err != nil {
		return fmt.Errorf("scan_chapter_reference: editorial: %w", err)
	}
	return applyReferenceRows(editorial, byNum)
}

// applyReferenceRows consumes a (verse_index, subtitle, content, footer) result
// set into the Reference field of the matching rows.
func applyReferenceRows(rows pgx.Rows, byNum map[int]*QuranVerseEditorRow) error {
	defer rows.Close()
	for rows.Next() {
		var n int
		var t QuranText
		if err := rows.Scan(&n, &t.Subtitle, &t.Content, &t.Footer); err != nil {
			return fmt.Errorf("apply_reference_rows: scan: %w", err)
		}
		if row, ok := byNum[n]; ok {
			text := t
			row.Reference = &text
		}
	}
	return rows.Err()
}

// UpsertVerseDraft creates or updates a verse draft and returns its current
// state. Sending nil fields clears them. The draft is reset to 'draft' status.
func (db *DB) UpsertVerseDraft(ctx context.Context, versionID, verseID, langID int, fields QuranText, authorID int) (*QuranVerseEditorRow, error) {
	defer observe("upsert_verse_draft", time.Now())
	var status string
	var updated time.Time
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO verse_text_drafts (verse_id, lang_id, version_id, subtitle, content, footer, status, author_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'draft', $7, now())
		ON CONFLICT (version_id, verse_id) WHERE version_id IS NOT NULL
		DO UPDATE SET subtitle = EXCLUDED.subtitle,
		              content  = EXCLUDED.content,
		              footer   = EXCLUDED.footer,
		              status   = 'draft',
		              author_id = EXCLUDED.author_id,
		              updated_at = now()
		RETURNING status::text, updated_at`,
		verseID, langID, versionID, fields.Subtitle, fields.Content, fields.Footer, authorID).
		Scan(&status, &updated)
	if err != nil {
		return nil, fmt.Errorf("upsert_verse_draft: %w", err)
	}
	text := fields
	return &QuranVerseEditorRow{Draft: &text, DraftStatus: &status, DraftUpdatedAt: &updated}, nil
}

// UpsertChapterDraft creates or updates the chapter-title draft.
func (db *DB) UpsertChapterDraft(ctx context.Context, versionID, chapterID, langID int, title *string, authorID int) error {
	defer observe("upsert_chapter_draft", time.Now())
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO chapter_text_drafts (chapter_id, lang_id, version_id, title, status, author_id, updated_at)
		VALUES ($1, $2, $3, $4, 'draft', $5, now())
		ON CONFLICT (version_id, chapter_id) WHERE version_id IS NOT NULL
		DO UPDATE SET title = EXCLUDED.title,
		              status = 'draft',
		              author_id = EXCLUDED.author_id,
		              updated_at = now()`,
		chapterID, langID, versionID, title, authorID)
	if err != nil {
		return fmt.Errorf("upsert_chapter_draft: %w", err)
	}
	return nil
}
