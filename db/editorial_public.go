package db

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Public, unauthenticated reads of published editorial articles. These serve the
// public blog (Sanity replacement): only the `published` snapshot is exposed,
// never drafts. Author name/photo and the primary category are denormalized from
// their own modules so the frontend renders in one round-trip. Author and
// category display fields fall back to the draft when unpublished, so a freshly
// seeded author still shows.

// PublishedArticle is one published article shaped for the public blog. Body and
// ThumbnailText are Portable Text arrays passed through verbatim.
type PublishedArticle struct {
	ID               int
	TranslationGroup string
	Slug             string
	Language         string // language code, e.g. "en"
	Title            string
	Excerpt          string
	ThumbnailURL     string
	ThumbnailText    json.RawMessage
	Body             json.RawMessage
	// Defaults to true when absent so articles predating the field keep
	// linkifying scripture references exactly as they did before.
	EnableScriptureRefs bool
	CategoryName        string
	CategorySlug        string
	AuthorName          string
	AuthorPhotoURL      string
	AuthorSlug          string
	PublishedAt         *time.Time
	UpdatedAt           time.Time
}

// publishedArticleColumns is the shared SELECT list. `full` includes the body
// (single-article view); list/related/search omit it to keep payloads small.
func publishedArticleColumns(full bool) string {
	body := "'[]'::jsonb"
	if full {
		body = "COALESCE(a.published->'body', '[]'::jsonb)"
	}
	return `
		a.id,
		a.translation_group::text,
		a.slug,
		l.code,
		COALESCE(a.published->>'title', ''),
		COALESCE(a.published->>'excerpt', ''),
		COALESCE(a.published->>'thumbnail_url', ''),
		COALESCE((a.published->>'enable_scripture_refs')::boolean, true),
		COALESCE(a.published->'thumbnail_text', '[]'::jsonb),
		` + body + ` AS body,
		COALESCE(cat.name, ''),
		COALESCE(cat.slug, ''),
		COALESCE(NULLIF(TRIM(CONCAT_WS(' ',
			COALESCE(au.published, au.draft)->>'first_name',
			COALESCE(au.published, au.draft)->>'last_name')), ''), ''),
		COALESCE(COALESCE(au.published, au.draft)->>'photo_url', ''),
		COALESCE(au.slug, ''),
		a.published_at,
		a.updated_at`
}

// publishedArticleJoins denormalizes the author (by author_id) and the first
// category (by the first id in the published `categories` array). Author and
// category display data fall back to the draft when not separately published.
const publishedArticleJoins = `
	JOIN languages l ON l.id = a.lang_id
	LEFT JOIN editorial_authors au ON au.id = a.author_id
	LEFT JOIN LATERAL (
		SELECT COALESCE(c.published, c.draft)->>'name' AS name, c.slug AS slug
		FROM editorial_categories c
		WHERE jsonb_typeof(a.published->'categories') = 'array'
		  AND jsonb_array_length(a.published->'categories') > 0
		  AND c.id = (a.published->'categories'->>0)::int
		LIMIT 1
	) cat ON true`

func scanPublishedArticle(row pgx.Row) (*PublishedArticle, error) {
	var a PublishedArticle
	if err := row.Scan(
		&a.ID, &a.TranslationGroup, &a.Slug, &a.Language,
		&a.Title, &a.Excerpt, &a.ThumbnailURL, &a.EnableScriptureRefs, &a.ThumbnailText, &a.Body,
		&a.CategoryName, &a.CategorySlug,
		&a.AuthorName, &a.AuthorPhotoURL, &a.AuthorSlug,
		&a.PublishedAt, &a.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &a, nil
}

// GetPublishedArticleBySlug returns one published article by language code and
// slug, or ErrContentNotFound.
func (db *DB) GetPublishedArticleBySlug(ctx context.Context, lang, slug string) (*PublishedArticle, error) {
	defer observe("get_published_article", time.Now())
	query := `SELECT ` + publishedArticleColumns(true) + `
		FROM editorial_articles a` + publishedArticleJoins + `
		WHERE a.published IS NOT NULL AND l.code = $1 AND a.slug = $2`
	a, err := scanPublishedArticle(db.Pool.QueryRow(ctx, query, lang, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrContentNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

// ListPublishedArticles returns published articles, newest first, optionally
// filtered by language code. Body is omitted.
func (db *DB) ListPublishedArticles(ctx context.Context, lang string, limit, offset int) ([]PublishedArticle, error) {
	defer observe("list_published_articles", time.Now())
	where := "a.published IS NOT NULL"
	args := []any{limit, offset}
	if lang != "" {
		where += " AND l.code = $3"
		args = append(args, lang)
	}
	query := `SELECT ` + publishedArticleColumns(false) + `
		FROM editorial_articles a` + publishedArticleJoins + `
		WHERE ` + where + `
		ORDER BY a.published_at DESC NULLS LAST, a.id DESC
		LIMIT $1 OFFSET $2`
	return db.queryPublishedArticles(ctx, query, args...)
}

// ListRelatedPublishedArticles returns other published articles sharing the
// primary category, same language, excluding excludeID.
func (db *DB) ListRelatedPublishedArticles(ctx context.Context, lang, categorySlug string, excludeID, limit int) ([]PublishedArticle, error) {
	defer observe("list_related_published_articles", time.Now())
	if categorySlug == "" {
		return []PublishedArticle{}, nil
	}
	query := `SELECT ` + publishedArticleColumns(false) + `
		FROM editorial_articles a` + publishedArticleJoins + `
		WHERE a.published IS NOT NULL AND l.code = $1 AND a.id <> $2
		  AND cat.slug = $3
		ORDER BY a.published_at DESC NULLS LAST, a.id DESC
		LIMIT $4`
	return db.queryPublishedArticles(ctx, query, lang, excludeID, categorySlug, limit)
}

// SearchPublishedArticles matches q (case-insensitive) against title, excerpt
// and the serialized body text, optionally scoped to a language code.
func (db *DB) SearchPublishedArticles(ctx context.Context, q, lang string, limit int) ([]PublishedArticle, error) {
	defer observe("search_published_articles", time.Now())
	if strings.TrimSpace(q) == "" {
		return []PublishedArticle{}, nil
	}
	pattern := "%" + strings.TrimSpace(q) + "%"
	where := `a.published IS NOT NULL AND (
		a.published->>'title' ILIKE $1
		OR a.published->>'excerpt' ILIKE $1
		OR a.published->>'body' ILIKE $1)`
	args := []any{pattern, limit}
	if lang != "" {
		where += " AND l.code = $3"
		args = append(args, lang)
	}
	// Include the body so callers can build match snippets.
	query := `SELECT ` + publishedArticleColumns(true) + `
		FROM editorial_articles a` + publishedArticleJoins + `
		WHERE ` + where + `
		ORDER BY a.published_at DESC NULLS LAST, a.id DESC
		LIMIT $2`
	return db.queryPublishedArticles(ctx, query, args...)
}

func (db *DB) queryPublishedArticles(ctx context.Context, query string, args ...any) ([]PublishedArticle, error) {
	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PublishedArticle{}
	for rows.Next() {
		a, err := scanPublishedArticle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}
