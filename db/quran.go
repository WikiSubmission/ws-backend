package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Internal structs for scanning database results.
// These avoid the import cycle and handle NULLs via pgx's native support.
type VerseRow struct {
	ChapterNumber int     `db:"chapter_number"`
	ChapterTitle  string  `db:"chapter_title"`
	VerseIndex    int     `db:"verse_index"`
	LangID        int     `db:"lang_id"`
	LangCode      string  `db:"lang_code"`
	Direction     string  `db:"direction"`
	VerseContent  string  `db:"verse_content"`
	VerseSubtitle *string `db:"verse_subtitle"` // pgx scans null into *string automatically
	VerseFooter   *string `db:"verse_footer"`
}

type WordRow struct {
	ChapterNumber int     `db:"chapter_number"`
	VerseIndex    int     `db:"verse_index"`
	WordIndex     int     `db:"word_index"`
	GlobalIndex   int     `db:"global_index"`
	LangCode      string  `db:"lang_code"`
	WordText      string  `db:"word_text"`
	WordRoot      *string `db:"word_root"`
	Meaning       *string `db:"meaning"`
}

func (db *DB) FetchVersesWithTranslations(ctx context.Context, startCh, endCh, startV, endV int, langs []string) ([]VerseRow, error) {
	defer observe("fetch_verses_with_translations", time.Now())
	query := `
        SELECT 
            c.chapter_number, COALESCE(ct.title, '') as chapter_title, v.verse_index, 
            l.id as lang_id, l.code as lang_code, l.direction,
            vt.content as verse_content, vt.subtitle as verse_subtitle, vt.footer as verse_footer
        FROM verses v
        JOIN chapters c ON v.chapter_id = c.id
        JOIN verse_text vt ON v.id = vt.verse_id
        JOIN languages l ON vt.lang_id = l.id
        LEFT JOIN chapter_text ct ON c.id = ct.chapter_id AND ct.lang_id = l.id
        WHERE (c.chapter_number, v.verse_index) >= ($1, $3)
          AND (c.chapter_number, v.verse_index) <= ($2, $4)
          AND l.code = ANY($5)
        ORDER BY c.chapter_number, v.verse_index, l.id;
    `
	rows, err := db.Pool.Query(ctx, query, startCh, endCh, startV, endV, langs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[VerseRow])
}

// FetchVersesWithTranslationsByRefs fetches verses for a disjoint set of
// (chapter, verse_index) pairs using a parallel unnest join. Use this instead
// of FetchVersesWithTranslations when the verse set is non-contiguous.
func (db *DB) FetchVersesWithTranslationsByRefs(ctx context.Context, chapterNums, verseIdxs []int32, langs []string) ([]VerseRow, error) {
	defer observe("fetch_verses_with_translations_by_refs", time.Now())
	query := `
        SELECT
            c.chapter_number, COALESCE(ct.title, '') AS chapter_title, v.verse_index,
            l.id AS lang_id, l.code AS lang_code, l.direction,
            vt.content AS verse_content, vt.subtitle AS verse_subtitle, vt.footer AS verse_footer
        FROM unnest($1::int[], $2::int[]) AS ref(ch, vi)
        JOIN chapters c    ON c.chapter_number = ref.ch
        JOIN verses v      ON v.chapter_id = c.id AND v.verse_index = ref.vi
        JOIN verse_text vt ON vt.verse_id = v.id
        JOIN languages l   ON l.id = vt.lang_id
        LEFT JOIN chapter_text ct ON ct.chapter_id = c.id AND ct.lang_id = l.id
        WHERE l.code = ANY($3)
        ORDER BY c.chapter_number, v.verse_index, l.id;
    `
	rows, err := db.Pool.Query(ctx, query, chapterNums, verseIdxs, langs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[VerseRow])
}

// FetchWordsForVersesByRefs fetches word data for a disjoint set of
// (chapter, verse_index) pairs using a parallel unnest join.
func (db *DB) FetchWordsForVersesByRefs(ctx context.Context, chapterNums, verseIdxs []int32, langs []string) ([]WordRow, error) {
	defer observe("fetch_words_for_verses_by_refs", time.Now())
	query := `
        SELECT
            c.chapter_number, v.verse_index, w.word_index, COALESCE(w.global_index, 0) AS global_index,
            l.code AS lang_code, wt.word AS word_text, wt.word_root, wt.meaning
        FROM unnest($1::int[], $2::int[]) AS ref(ch, vi)
        JOIN chapters c  ON c.chapter_number = ref.ch
        JOIN verses v    ON v.chapter_id = c.id AND v.verse_index = ref.vi
        JOIN words w     ON w.verse_id = v.id
        JOIN word_text wt ON wt.word_id = w.id
        JOIN languages l  ON l.id = wt.lang_id
        WHERE l.code = ANY($3)
        ORDER BY c.chapter_number, v.verse_index, w.word_index;
    `
	rows, err := db.Pool.Query(ctx, query, chapterNums, verseIdxs, langs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[WordRow])
}

func (db *DB) FetchWordsForVerses(ctx context.Context, startCh, endCh, startV, endV int, langs []string) ([]WordRow, error) {
	defer observe("fetch_words_for_verses", time.Now())
	query := `
        SELECT 
            c.chapter_number, v.verse_index, w.word_index, COALESCE(w.global_index, 0) as global_index,
            l.code as lang_code, wt.word as word_text, wt.word_root, wt.meaning
        FROM words w
        JOIN verses v ON w.verse_id = v.id
        JOIN chapters c ON v.chapter_id = c.id
        JOIN word_text wt ON w.id = wt.word_id
        JOIN languages l ON wt.lang_id = l.id
        WHERE (c.chapter_number, v.verse_index) >= ($1, $3) -- Corrected: Range logic
          AND (c.chapter_number, v.verse_index) <= ($2, $4) -- Corrected: Range logic
          AND l.code = ANY($5)
        ORDER BY c.chapter_number, v.verse_index, w.word_index;
    `
	rows, err := db.Pool.Query(ctx, query, startCh, endCh, startV, endV, langs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[WordRow])
}
