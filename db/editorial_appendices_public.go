package db

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// Public, unauthenticated reads of published editorial appendices. Mirrors
// editorial_public.go (articles): only the `published` snapshot is exposed,
// never the working draft.
//
// Appendices are scoped to a Quran version, not to a language directly, so the
// version registry is joined in and its language code is what callers filter
// on. A language may carry more than one version; when it does, the canonical
// English version wins and the oldest row breaks any remaining tie, so the
// resolution is deterministic. Callers that care can pin one explicitly with
// the version slug.
//
// `body` is markdown text (see ws-backend commit c54d4db), not Portable Text.

// PublishedAppendix is one published appendix shaped for the public reader.
type PublishedAppendix struct {
	ID          int
	Code        string
	Title       string
	Snippet     string
	Body        string
	Language    string // language code of the owning version, e.g. "en"
	VersionSlug string
	VersionName string
	Direction   string // "ltr" | "rtl"
	PublishedAt *time.Time
	UpdatedAt   time.Time
}

// AppendixFilter narrows a listing. Empty fields disable that filter.
type AppendixFilter struct {
	Language    string // language code of the owning Quran version
	VersionSlug string // exact quran_versions.slug
}

// publishedAppendixColumns is the shared SELECT list. `full` includes the
// markdown body (single-appendix view); the listing omits it to keep the
// payload small.
func publishedAppendixColumns(full bool) string {
	body := "''::text"
	if full {
		body = "COALESCE(a.published->>'body', '')"
	}
	return `
		a.id,
		a.code,
		COALESCE(a.published->>'title', ''),
		COALESCE(a.published->>'snippet', ''),
		` + body + ` AS body,
		l.code,
		v.slug,
		v.name,
		v.direction,
		a.published_at,
		a.updated_at`
}

const publishedAppendixJoins = `
	JOIN quran_versions v ON v.id = a.version_id
	JOIN languages l ON l.id = v.lang_id`

// appendixOrder sorts numeric codes numerically ("2" before "10") and pushes
// non-numeric codes (e.g. "intro") to the end in lexical order. The digit
// bound keeps an absurdly long numeric code from overflowing the int cast.
const appendixOrder = `
	ORDER BY v.is_canonical_english DESC, v.id ASC,
	         (CASE WHEN a.code ~ '^[0-9]{1,6}$' THEN a.code::int END) ASC NULLS LAST,
	         a.code ASC`

func scanPublishedAppendix(row pgx.Row) (*PublishedAppendix, error) {
	var a PublishedAppendix
	if err := row.Scan(
		&a.ID, &a.Code, &a.Title, &a.Snippet, &a.Body,
		&a.Language, &a.VersionSlug, &a.VersionName, &a.Direction,
		&a.PublishedAt, &a.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &a, nil
}

// appendixWhere builds the shared filter clause, appending to args.
func appendixWhere(f AppendixFilter, args *[]any) string {
	where := "a.published IS NOT NULL"
	if f.Language != "" {
		*args = append(*args, f.Language)
		where += " AND l.code = $" + strconv.Itoa(len(*args))
	}
	if f.VersionSlug != "" {
		*args = append(*args, f.VersionSlug)
		where += " AND v.slug = $" + strconv.Itoa(len(*args))
	}
	return where
}

// ListPublishedAppendices returns published appendices matching the filter, in
// reading order. Body is omitted. Never returns nil on success, so the endpoint
// encodes an empty list rather than null.
func (db *DB) ListPublishedAppendices(ctx context.Context, f AppendixFilter) ([]PublishedAppendix, error) {
	defer observe("list_published_appendices", time.Now())

	args := []any{}
	query := `SELECT ` + publishedAppendixColumns(false) + `
		FROM editorial_appendices a` + publishedAppendixJoins + `
		WHERE ` + appendixWhere(f, &args) + appendixOrder

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PublishedAppendix{}
	for rows.Next() {
		a, err := scanPublishedAppendix(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// GetPublishedAppendixByCode returns one published appendix, with its markdown
// body, or ErrContentNotFound. When the filter matches several versions the
// canonical English one wins, then the oldest.
func (db *DB) GetPublishedAppendixByCode(ctx context.Context, code string, f AppendixFilter) (*PublishedAppendix, error) {
	defer observe("get_published_appendix", time.Now())

	args := []any{code}
	where := "a.code = $1 AND " + appendixWhere(f, &args)
	query := `SELECT ` + publishedAppendixColumns(true) + `
		FROM editorial_appendices a` + publishedAppendixJoins + `
		WHERE ` + where + appendixOrder + `
		LIMIT 1`

	a, err := scanPublishedAppendix(db.Pool.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrContentNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}
