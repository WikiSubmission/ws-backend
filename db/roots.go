package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// RootSummaryRow is one row from the materialized roots index.
type RootSummaryRow struct {
	Letters     string `db:"letters"`
	Occurrences int    `db:"occurrences"`
}

// DerivativeRow is one distinct surface form for a root.
type DerivativeRow struct {
	Ar    string `db:"ar"`
	Count int    `db:"count"`
}

// OccurrenceRow is one verse occurrence of a root.
type OccurrenceRow struct {
	ChapterNumber int     `db:"chapter_number"`
	VerseIndex    int     `db:"verse_index"`
	Ar            string  `db:"ar"`
	En            *string `db:"en"`
	Hl            string  `db:"hl"`
}

// VerseKey returns the canonical verse_key (e.g. "2:282") matching the
// convention used elsewhere in the codebase (see handlers/quran.go).
func (r OccurrenceRow) VerseKey() string {
	return fmt.Sprintf("%d:%d", r.ChapterNumber, r.VerseIndex)
}

// rootSortClause translates a validated sort key into a SQL ORDER BY clause.
// The caller MUST whitelist the sort string before calling this function;
// any unknown value falls back to frequency.
func rootSortClause(sort string) string {
	switch sort {
	case "abjadi":
		return "abjadi_rank ASC, letters ASC"
	case "reverse":
		return "abjadi_rank DESC, letters DESC"
	default:
		return "occurrences DESC, letters ASC"
	}
}

// RootsIndex returns a paginated slice of the roots index with optional
// Arabic prefix filter and one of three sort modes ("frequency", "abjadi",
// "reverse"). The returned total reflects the filtered universe.
func (db *DB) RootsIndex(ctx context.Context, q *string, sort string, limit, offset int) ([]RootSummaryRow, int, error) {
	orderBy := rootSortClause(sort)

	sql := `
        SELECT letters, occurrences
        FROM   root_index_mv
        WHERE  ($1::text IS NULL OR letters LIKE $1 || '%')
        ORDER  BY ` + orderBy + `
        LIMIT  $2 OFFSET $3
    `
	rows, err := db.Pool.Query(ctx, sql, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("roots index query: %w", err)
	}
	defer rows.Close()
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[RootSummaryRow])
	if err != nil {
		return nil, 0, fmt.Errorf("roots index scan: %w", err)
	}

	var total int
	countSQL := `
        SELECT COUNT(*)
        FROM   root_index_mv
        WHERE  ($1::text IS NULL OR letters LIKE $1 || '%')
    `
	if err := db.Pool.QueryRow(ctx, countSQL, q).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("roots index count: %w", err)
	}
	return items, total, nil
}

// RootDetail returns the summary record, top derivatives (capped at 20), and
// first 20 occurrences for the given root letters. Returns pgx.ErrNoRows
// when the root is not present in the matview.
func (db *DB) RootDetail(ctx context.Context, letters string) (RootSummaryRow, []DerivativeRow, []OccurrenceRow, error) {
	var summary RootSummaryRow
	err := db.Pool.QueryRow(ctx, `
        SELECT letters, occurrences
        FROM   root_index_mv
        WHERE  letters = $1
    `, letters).Scan(&summary.Letters, &summary.Occurrences)
	if err != nil {
		return RootSummaryRow{}, nil, nil, err
	}

	derivRows, err := db.Pool.Query(ctx, `
        SELECT wt.word AS ar, COUNT(*)::int AS count
        FROM   word_text wt
        JOIN   languages l ON wt.lang_id = l.id
        WHERE  l.code = 'ar' AND wt.word_root = $1
        GROUP  BY wt.word
        ORDER  BY count DESC, wt.word
        LIMIT  20
    `, letters)
	if err != nil {
		return RootSummaryRow{}, nil, nil, fmt.Errorf("root derivatives query: %w", err)
	}
	defer derivRows.Close()
	derivs, err := pgx.CollectRows(derivRows, pgx.RowToStructByName[DerivativeRow])
	if err != nil {
		return RootSummaryRow{}, nil, nil, fmt.Errorf("root derivatives scan: %w", err)
	}

	occ, _, err := db.rootOccurrencesPage(ctx, letters, 20, 0)
	if err != nil {
		return RootSummaryRow{}, nil, nil, err
	}
	return summary, derivs, occ, nil
}

// RootOccurrences returns paginated verse occurrences for the given root,
// ordered by canonical reading order (global_index). The total count
// reflects all occurrences of the root.
func (db *DB) RootOccurrences(ctx context.Context, letters string, limit, offset int) ([]OccurrenceRow, int, error) {
	return db.rootOccurrencesPage(ctx, letters, limit, offset)
}

func (db *DB) rootOccurrencesPage(ctx context.Context, letters string, limit, offset int) ([]OccurrenceRow, int, error) {
	sql := `
        SELECT
            c.chapter_number,
            v.verse_index,
            ar_vt.content    AS ar,
            en_vt.content    AS en,
            ar_wt.word       AS hl
        FROM   word_text ar_wt
        JOIN   languages ar_l   ON ar_wt.lang_id = ar_l.id AND ar_l.code = 'ar'
        JOIN   words     w      ON ar_wt.word_id = w.id
        JOIN   verses    v      ON w.verse_id   = v.id
        JOIN   chapters  c      ON v.chapter_id = c.id
        JOIN   verse_text ar_vt ON ar_vt.verse_id = v.id AND ar_vt.lang_id = ar_l.id
        LEFT   JOIN languages  en_l  ON en_l.code = 'en'
        LEFT   JOIN verse_text en_vt ON en_vt.verse_id = v.id AND en_vt.lang_id = en_l.id
        WHERE  ar_wt.word_root = $1
        ORDER  BY w.global_index
        LIMIT  $2 OFFSET $3
    `
	rows, err := db.Pool.Query(ctx, sql, letters, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("root occurrences query: %w", err)
	}
	defer rows.Close()
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[OccurrenceRow])
	if err != nil {
		return nil, 0, fmt.Errorf("root occurrences scan: %w", err)
	}

	var total int
	countSQL := `
        SELECT COUNT(*)
        FROM   word_text wt
        JOIN   languages l ON wt.lang_id = l.id AND l.code = 'ar'
        WHERE  wt.word_root = $1
    `
	if err := db.Pool.QueryRow(ctx, countSQL, letters).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("root occurrences count: %w", err)
	}
	return items, total, nil
}
