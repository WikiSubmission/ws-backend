package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type LanguageRow struct {
	ID        int    `db:"id"`
	Code      string `db:"code"`
	Direction string `db:"direction"`
}

type ChapterMetaRow struct {
	ChapterNumber   int `db:"chapter_number"`
	VerseCount      int `db:"verse_count"`
	RevelationOrder int `db:"revelation_order"`
}

type AppendixRow struct {
	Code    int    `db:"code"`
	Title   string `db:"text"`
	Snippet string `db:"snippet"`
}

type ChapterRow struct {
	ChapterNumber   int    `db:"chapter_number"`
	VerseCount      int    `db:"verse_count"`
	RevelationOrder int    `db:"revelation_order"`
	Title           string `db:"title"`
}

func (db *DB) FetchAllLanguages(ctx context.Context) ([]LanguageRow, error) {
	query := `SELECT id, code, direction FROM languages`
	rows, err := db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[LanguageRow])
}

func (db *DB) FetchAllChapterMeta(ctx context.Context) ([]ChapterMetaRow, error) {
	query := `SELECT chapter_number, verse_count, revelation_order FROM chapters ORDER BY chapter_number`
	rows, err := db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[ChapterMetaRow])
}

func (db *DB) FetchAllAppendices(ctx context.Context, lang string) ([]AppendixRow, error) {
	query := `SELECT a.code, a.title, a.snippet 
        FROM appendices a
        JOIN languages l ON a.lang_id = l.id
        WHERE l.code = $1 
        ORDER BY a.code`
	rows, err := db.Pool.Query(ctx, query, lang)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[AppendixRow])
}

func (db *DB) FetchAllChapters(ctx context.Context, lang string) ([]ChapterRow, error) {
	query := `SELECT 
			c.chapter_number, 
			c.verse_count, 
			c.revelation_order, 
			ct.title 
		FROM chapters c
		INNER JOIN chapter_text ct ON c.id = ct.chapter_id
		INNER JOIN languages l ON ct.lang_id = l.id
		WHERE l.code = $1 
		ORDER BY c.chapter_number`
	rows, err := db.Pool.Query(ctx, query, lang)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[ChapterRow])
}
