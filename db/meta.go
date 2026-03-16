package db

import (
	"context"
	"log/slog"

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
	Title   string `db:"title"`
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

func (db *DB) FetchAllAppendices(ctx context.Context, langID int) ([]AppendixRow, error) {
	slog.Debug("FetchAllAppendices", "lang_id", langID)

	// Verify the table has any data at all
	var total int
	_ = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM appendices`).Scan(&total)
	slog.Debug("FetchAllAppendices table count", "total_rows", total)

	query := `SELECT code, title, snippet FROM appendices WHERE lang_id = $1 ORDER BY code`
	rows, err := db.Pool.Query(ctx, query, langID)
	if err != nil {
		slog.Error("FetchAllAppendices query failed", "lang_id", langID, "error", err)
		return nil, err
	}
	defer rows.Close()

	result, err := pgx.CollectRows(rows, pgx.RowToStructByName[AppendixRow])
	slog.Debug("FetchAllAppendices result", "lang_id", langID, "rows_returned", len(result), "error", err)
	return result, err
}

func (db *DB) FetchAllChapters(ctx context.Context, langID int) ([]ChapterRow, error) {
	query := `SELECT c.chapter_number, c.verse_count, c.revelation_order, ct.title
		FROM chapters c
		INNER JOIN chapter_text ct ON c.id = ct.chapter_id
		WHERE ct.lang_id = $1
		ORDER BY c.chapter_number`
	rows, err := db.Pool.Query(ctx, query, langID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[ChapterRow])
}
