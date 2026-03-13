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
