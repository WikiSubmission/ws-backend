package db

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"embed"
)

// Site documents: the site's own pages, indexed per UI locale for the command
// menu's content search (see migrations/029_site_documents.sql). Page bodies are
// seeded from db/seeds/site_docs_<lang>.json, extracted from the rendered site by
// org/apps/web/scripts/extract-site-text.ts.

// Wildcard embed so adding a locale needs no code change here, matching the
// pattern db/seeds.go uses for the games lemma seeds.
//
//go:embed seeds/site_docs_*.json
var siteDocSeeds embed.FS

type siteSeedRow struct {
	Route        string  `json:"route"`
	Anchor       *string `json:"anchor"`
	Kind         string  `json:"kind"`
	SectionIndex int     `json:"section_index"`
	Title        string  `json:"title"`
	Heading      *string `json:"heading"`
	Description  *string `json:"description"`
	Body         string  `json:"body"`
	Priority     int     `json:"priority"`
}

// langFromSeedName pulls "ar" out of "seeds/site_docs_ar.json".
func langFromSeedName(name string) string {
	base := strings.TrimSuffix(path.Base(name), ".json")
	return strings.TrimPrefix(base, "site_docs_")
}

// SeedSiteDocuments loads every embedded site seed, replacing all seed-sourced
// rows for each language present (full-replace keeps it idempotent: re-running
// after a content update is the update path). Derived and editorial rows are left
// alone. Returns the number of rows written.
func (db *DB) SeedSiteDocuments(ctx context.Context) (int, error) {
	defer observe("seed_site_documents", time.Now())

	entries, err := fs.Glob(siteDocSeeds, "seeds/site_docs_*.json")
	if err != nil {
		return 0, fmt.Errorf("glob site seeds: %w", err)
	}
	if len(entries) == 0 {
		return 0, fmt.Errorf("no site_docs_*.json seeds embedded")
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	written := 0
	for _, entry := range entries {
		lang := langFromSeedName(entry)
		if lang == "" {
			return 0, fmt.Errorf("cannot derive lang from seed name %q", entry)
		}

		raw, err := siteDocSeeds.ReadFile(entry)
		if err != nil {
			return 0, fmt.Errorf("read %s: %w", entry, err)
		}
		var rows []siteSeedRow
		if err := json.Unmarshal(raw, &rows); err != nil {
			return 0, fmt.Errorf("parse %s: %w", entry, err)
		}

		if _, err := tx.Exec(ctx,
			`DELETE FROM site_documents WHERE lang = $1 AND source = 'seed'`, lang,
		); err != nil {
			return 0, fmt.Errorf("clear site docs for %q: %w", lang, err)
		}

		batch := &pgx.Batch{}
		for _, r := range rows {
			batch.Queue(
				`INSERT INTO site_documents
				   (route, anchor, lang, kind, section_index, title, heading, description, body, priority, source)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'seed')
				 ON CONFLICT (route, lang, COALESCE(anchor, ''), section_index) DO UPDATE SET
				   kind = EXCLUDED.kind, title = EXCLUDED.title, heading = EXCLUDED.heading,
				   description = EXCLUDED.description, body = EXCLUDED.body,
				   priority = EXCLUDED.priority, source = EXCLUDED.source, updated_at = now()`,
				r.Route, r.Anchor, lang, r.Kind, r.SectionIndex,
				r.Title, r.Heading, r.Description, r.Body, r.Priority,
			)
		}
		results := tx.SendBatch(ctx, batch)
		for range rows {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return 0, fmt.Errorf("insert site doc for %q: %w", lang, err)
			}
		}
		if err := results.Close(); err != nil {
			return 0, fmt.Errorf("close batch for %q: %w", lang, err)
		}
		written += len(rows)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return written, nil
}

// RebuildDerivedSiteDocuments regenerates the chapter and appendix landing rows
// straight from chapter_text and appendices.
//
// These never go through a seed file: the titles already live in Postgres per
// language, so an INSERT ... SELECT keeps them permanently in sync and costs no
// extraction step. Returns the number of rows written.
func (db *DB) RebuildDerivedSiteDocuments(ctx context.Context) (int, error) {
	defer observe("rebuild_derived_site_documents", time.Now())

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM site_documents WHERE source = 'derived'`); err != nil {
		return 0, fmt.Errorf("clear derived site docs: %w", err)
	}

	// Chapter landing pages: /quran/<n>, titled in each language that has one.
	chapterTag, err := tx.Exec(ctx, `
        INSERT INTO site_documents
          (route, anchor, lang, kind, section_index, title, heading, description, body, priority, source)
        SELECT
          '/quran/' || c.chapter_number,
          NULL,
          l.code,
          'chapter',
          0,
          ct.title,
          NULL,
          'Quran chapter ' || c.chapter_number || ' · ' || c.verse_count || ' verses',
          '',
          45,
          'derived'
        FROM chapter_text ct
        JOIN chapters  c ON c.id = ct.chapter_id
        JOIN languages l ON l.id = ct.lang_id
        WHERE ct.title IS NOT NULL AND ct.title <> ''
        ON CONFLICT (route, lang, COALESCE(anchor, ''), section_index) DO UPDATE SET
          title = EXCLUDED.title, description = EXCLUDED.description,
          priority = EXCLUDED.priority, source = EXCLUDED.source, updated_at = now()
	`)
	if err != nil {
		return 0, fmt.Errorf("derive chapter site docs: %w", err)
	}

	// Appendix landing pages: /appendices/<code>. The snippet is a genuine
	// excerpt, so it is worth indexing as the body.
	appendixTag, err := tx.Exec(ctx, `
        INSERT INTO site_documents
          (route, anchor, lang, kind, section_index, title, heading, description, body, priority, source)
        SELECT
          '/appendices/' || a.code,
          NULL,
          l.code,
          'appendix',
          0,
          a.title,
          NULL,
          NULL,
          COALESCE(a.snippet, ''),
          55,
          'derived'
        FROM appendices a
        JOIN languages l ON l.id = a.lang_id
        WHERE a.code IS NOT NULL AND a.title IS NOT NULL AND a.title <> ''
        ON CONFLICT (route, lang, COALESCE(anchor, ''), section_index) DO UPDATE SET
          title = EXCLUDED.title, body = EXCLUDED.body,
          priority = EXCLUDED.priority, source = EXCLUDED.source, updated_at = now()
	`)
	if err != nil {
		return 0, fmt.Errorf("derive appendix site docs: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int(chapterTag.RowsAffected() + appendixTag.RowsAffected()), nil
}

// SiteHitRow is one full-text search hit across the site's pages.
type SiteHitRow struct {
	Route        string  `db:"route"`
	Anchor       *string `db:"anchor"`
	Lang         string  `db:"lang"`
	Kind         string  `db:"kind"`
	Title        string  `db:"title"`
	Heading      *string `db:"heading"`
	Description  *string `db:"description"`
	Score        float32 `db:"score"`
	Headline     string  `db:"headline"`
}

// SearchSiteDocuments searches the site's own pages plus the library corpus.
//
// Three legs, unioned: site_documents, library_sections (whose bodies are not
// duplicated into site_documents), and a trigram fallback that only fires when
// neither full-text leg matched — the same UNION ALL ... WHERE NOT EXISTS shape
// db/search.go uses for its trigram fallback.
//
// LIMIT/OFFSET are applied before ts_headline so the CPU-heavy highlighting only
// runs on the returned page. This is the trap db/library.go documents: putting
// ts_headline in the same SELECT as the candidate scan highlights all 200
// candidates instead of the 10 that are returned.
//
// Pass nil for langs to search every locale, and nil for kinds to search all.
func (db *DB) SearchSiteDocuments(
	ctx context.Context,
	query string,
	langs []string,
	kinds []string,
	limit, offset int,
) ([]SiteHitRow, error) {
	defer observe("search_site_documents", time.Now())

	sql := `
        WITH
          tsq AS (
            SELECT
              CASE
                WHEN $1 ~ '^[A-Za-z]+$'
                THEN to_tsquery('simple', lower($1) || ':*')
                ELSE websearch_to_tsquery('simple', $1)
              END AS q
          ),
          site_hits AS MATERIALIZED (
            SELECT d.route, d.anchor, d.lang, d.kind, d.title, d.heading,
                   d.description, d.body, d.priority, d.section_index,
                   (ts_rank_cd(d.content_tsv, tsq.q) + d.priority / 200.0)::real AS score
            FROM   site_documents d CROSS JOIN tsq
            WHERE  d.content_tsv @@ tsq.q
              AND  ($2::text[] IS NULL OR d.lang = ANY($2::text[]))
              AND  ($3::text[] IS NULL OR d.kind = ANY($3::text[]))
            ORDER  BY score DESC
            LIMIT  200
          ),
          lib_hits AS MATERIALIZED (
            -- Library bodies live in library_sections (022) and are not copied
            -- into site_documents; the route is computed the same way
            -- libraryHitHref does on the client
            -- (packages/shared/hooks/use-library-search.ts).
            SELECT
              CASE d.doc_type
                WHEN 'appendix'     THEN '/appendices/' || d.doc_number
                WHEN 'proclamation' THEN '/proclamation'
                ELSE '/introduction'
              END                       AS route,
              NULL::text                AS anchor,
              d.lang,
              'appendix'::text          AS kind,
              d.title, s.heading,
              NULL::text                AS description,
              s.body,
              55::smallint              AS priority,
              s.section_index::smallint AS section_index,
              (ts_rank_cd(s.content_tsv, tsq.q) + 0.275)::real AS score
            FROM   library_sections s
            JOIN   library_documents d ON d.id = s.document_id
            CROSS  JOIN tsq
            WHERE  s.content_tsv @@ tsq.q
              AND  ($2::text[] IS NULL OR d.lang = ANY($2::text[]))
              AND  ($3::text[] IS NULL OR 'appendix' = ANY($3::text[]))
            ORDER  BY score DESC
            LIMIT  200
          ),
          trgm_hits AS (
            SELECT d.route, d.anchor, d.lang, d.kind, d.title, d.heading,
                   d.description, d.body, d.priority, d.section_index,
                   (similarity(d.fuzzy_text, $1) + d.priority / 200.0)::real AS score
            FROM   site_documents d
            WHERE  NOT EXISTS (SELECT 1 FROM site_hits)
              AND  NOT EXISTS (SELECT 1 FROM lib_hits)
              AND  d.fuzzy_text %% $1
              AND  ($2::text[] IS NULL OR d.lang = ANY($2::text[]))
              AND  ($3::text[] IS NULL OR d.kind = ANY($3::text[]))
            ORDER  BY score DESC
            LIMIT  50
          ),
          ranked AS (
            SELECT * FROM (
              SELECT * FROM site_hits
              UNION ALL SELECT * FROM lib_hits
              UNION ALL SELECT * FROM trgm_hits
            ) u
            ORDER BY score DESC, priority DESC, route, section_index
            LIMIT  $4
            OFFSET $5
          )
        SELECT r.route, r.anchor, r.lang, r.kind, r.title, r.heading, r.description, r.score,
               ts_headline(
                 'simple', r.body, tsq.q,
                 'MaxWords=28,MinWords=10,MaxFragments=2,StartSel=<b>,StopSel=</b>'
               ) AS headline
        FROM   ranked r CROSS JOIN tsq
        ORDER  BY r.score DESC, r.priority DESC, r.route, r.section_index
	`

	rows, err := db.Pool.Query(ctx, sql, query, langs, kinds, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[SiteHitRow])
}

// FetchSiteLangs returns the UI locales that actually have indexed site content,
// for the handler's language validation. Mirrors FetchQuranLangs in db/meta.go.
func (db *DB) FetchSiteLangs(ctx context.Context) (map[string]bool, error) {
	defer observe("fetch_site_langs", time.Now())
	rows, err := db.Pool.Query(ctx, `SELECT DISTINCT lang FROM site_documents`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var lang string
		if err := rows.Scan(&lang); err != nil {
			return nil, err
		}
		out[lang] = true
	}
	return out, rows.Err()
}
