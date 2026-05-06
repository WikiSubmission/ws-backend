package db

import (
	"context"
	"errors"
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
	Ar    string  `db:"ar"`
	Tr    *string `db:"tr"`
	Count int     `db:"count"`
}

// OccurrenceRow is one verse occurrence of a root.
type OccurrenceRow struct {
	ChapterNumber int     `db:"chapter_number"`
	VerseIndex    int     `db:"verse_index"`
	Ar            string  `db:"ar"`
	En            *string `db:"en"`
	Hl            string  `db:"hl"`
	Tl            *string `db:"tl"`
	Wi            *int    `db:"wi"`
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

// RootDetail returns the summary record, top derivatives (capped at 20),
// first 20 occurrences, and the canonical per-root meaning for the given
// root letters. Returns pgx.ErrNoRows when the root is not present in the
// matview.
func (db *DB) RootDetail(ctx context.Context, letters string) (RootSummaryRow, []DerivativeRow, []OccurrenceRow, *string, error) {
	var summary RootSummaryRow
	err := db.Pool.QueryRow(ctx, `
        SELECT letters, occurrences
        FROM   root_index_mv
        WHERE  letters = $1
    `, letters).Scan(&summary.Letters, &summary.Occurrences)
	if err != nil {
		return RootSummaryRow{}, nil, nil, nil, err
	}

	derivRows, err := db.Pool.Query(ctx, `
        SELECT
            ar_wt.word                                            AS ar,
            mode() WITHIN GROUP (ORDER BY tl_wt.word)             AS tr,
            COUNT(*)::int                                         AS count
        FROM   word_text ar_wt
        JOIN   languages ar_l ON ar_wt.lang_id = ar_l.id AND ar_l.code = 'ar'
        LEFT   JOIN languages tl_l ON tl_l.code = 'tl'
        LEFT   JOIN word_text tl_wt
                 ON tl_wt.word_id = ar_wt.word_id
                AND tl_wt.lang_id = tl_l.id
        WHERE  ar_wt.word_root = $1
        GROUP  BY ar_wt.word
        ORDER  BY count DESC, ar_wt.word
        LIMIT  20
    `, letters)
	if err != nil {
		return RootSummaryRow{}, nil, nil, nil, fmt.Errorf("root derivatives query: %w", err)
	}
	defer derivRows.Close()
	derivs, err := pgx.CollectRows(derivRows, pgx.RowToStructByName[DerivativeRow])
	if err != nil {
		return RootSummaryRow{}, nil, nil, nil, fmt.Errorf("root derivatives scan: %w", err)
	}

	occ, _, err := db.rootOccurrencesPage(ctx, letters, "", 20, 0)
	if err != nil {
		return RootSummaryRow{}, nil, nil, nil, err
	}

	// Per-root meaning is denormalized across word_text rows for the root —
	// every row sharing this root carries the same string. Pull any one.
	var meaning *string
	err = db.Pool.QueryRow(ctx, `
        SELECT wt.meaning
        FROM   word_text wt
        JOIN   languages l ON wt.lang_id = l.id AND l.code = 'en'
        WHERE  wt.word_root = $1 AND wt.meaning IS NOT NULL AND wt.meaning <> ''
        LIMIT  1
    `, letters).Scan(&meaning)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return RootSummaryRow{}, nil, nil, nil, fmt.Errorf("root meaning query: %w", err)
	}

	return summary, derivs, occ, meaning, nil
}

// RootOccurrences returns paginated verse occurrences for the given root,
// ordered by canonical reading order (global_index). When surface is non-empty,
// the result set is narrowed to occurrences where the matched word equals the
// given Arabic surface form (used by the Word Lab "click derivative chip"
// flow). The total count reflects the filtered universe.
func (db *DB) RootOccurrences(ctx context.Context, letters, surface string, limit, offset int) ([]OccurrenceRow, int, error) {
	return db.rootOccurrencesPage(ctx, letters, surface, limit, offset)
}

func (db *DB) rootOccurrencesPage(ctx context.Context, letters, surface string, limit, offset int) ([]OccurrenceRow, int, error) {
	sql := `
        SELECT
            c.chapter_number,
            v.verse_index,
            ar_vt.content    AS ar,
            en_vt.content    AS en,
            ar_wt.word       AS hl,
            tl_wt.word       AS tl,
            w.word_index     AS wi
        FROM   word_text ar_wt
        JOIN   languages ar_l   ON ar_wt.lang_id = ar_l.id AND ar_l.code = 'ar'
        JOIN   words     w      ON ar_wt.word_id = w.id
        JOIN   verses    v      ON w.verse_id   = v.id
        JOIN   chapters  c      ON v.chapter_id = c.id
        JOIN   verse_text ar_vt ON ar_vt.verse_id = v.id AND ar_vt.lang_id = ar_l.id
        LEFT   JOIN languages  en_l  ON en_l.code = 'en'
        LEFT   JOIN verse_text en_vt ON en_vt.verse_id = v.id AND en_vt.lang_id = en_l.id
        LEFT   JOIN languages  tl_l  ON tl_l.code = 'tl'
        LEFT   JOIN word_text  tl_wt ON tl_wt.word_id = ar_wt.word_id AND tl_wt.lang_id = tl_l.id
        WHERE  ar_wt.word_root = $1
          AND  ($4::text = '' OR ar_wt.word = $4)
        ORDER  BY w.global_index
        LIMIT  $2 OFFSET $3
    `
	rows, err := db.Pool.Query(ctx, sql, letters, limit, offset, surface)
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
          AND  ($2::text = '' OR wt.word = $2)
    `
	if err := db.Pool.QueryRow(ctx, countSQL, letters, surface).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("root occurrences count: %w", err)
	}
	return items, total, nil
}
