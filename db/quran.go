package db

import (
	"context"

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

func (db *DB) FetchWordsForVerses(ctx context.Context, startCh, endCh, startV, endV int, langs []string) ([]WordRow, error) {
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
