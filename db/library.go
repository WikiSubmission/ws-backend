package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	_ "embed"
)

// Library documents: full text of the Introduction, Proclamation, and the 38
// appendices (see migrations/022_library_documents.sql). The seed JSON is
// generated from the frontend content by
// org/apps/web/scripts/extract-library-text.tsx.

//go:embed seeds/library_docs_en.json
var libraryDocsSeed []byte

type librarySeedSection struct {
	Idx     int     `json:"idx"`
	Heading *string `json:"heading"`
	Body    string  `json:"body"`
}

type librarySeedDoc struct {
	DocType   string               `json:"doc_type"`
	DocNumber *int                 `json:"doc_number"`
	Lang      string               `json:"lang"`
	Title     string               `json:"title"`
	Sections  []librarySeedSection `json:"sections"`
}

// SeedLibraryDocuments loads the embedded library seed, replacing every
// document of each language present in the seed file (full-replace keeps the
// operation idempotent: re-running after a content update is the update path).
// Returns the number of documents written.
func (db *DB) SeedLibraryDocuments(ctx context.Context) (int, error) {
	defer observe("seed_library_documents", time.Now())

	var docs []librarySeedDoc
	if err := json.Unmarshal(libraryDocsSeed, &docs); err != nil {
		return 0, fmt.Errorf("parse library seed: %w", err)
	}
	if len(docs) == 0 {
		return 0, fmt.Errorf("library seed is empty")
	}

	langs := map[string]bool{}
	for _, d := range docs {
		langs[d.Lang] = true
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for lang := range langs {
		if _, err := tx.Exec(ctx, `DELETE FROM library_documents WHERE lang = $1`, lang); err != nil {
			return 0, fmt.Errorf("clear library docs for %q: %w", lang, err)
		}
	}

	for _, d := range docs {
		var docID int
		if err := tx.QueryRow(ctx,
			`INSERT INTO library_documents (doc_type, doc_number, lang, title)
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			d.DocType, d.DocNumber, d.Lang, d.Title,
		).Scan(&docID); err != nil {
			return 0, fmt.Errorf("insert doc %s/%v: %w", d.DocType, d.DocNumber, err)
		}
		for _, s := range d.Sections {
			if _, err := tx.Exec(ctx,
				`INSERT INTO library_sections (document_id, section_index, heading, body)
				 VALUES ($1, $2, $3, $4)`,
				docID, s.Idx, s.Heading, s.Body,
			); err != nil {
				return 0, fmt.Errorf("insert section %s/%v[%d]: %w", d.DocType, d.DocNumber, s.Idx, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(docs), nil
}

// LibrarySectionRow is one full-text search hit in a library document section.
type LibrarySectionRow struct {
	DocType      string  `db:"doc_type"`
	DocNumber    *int    `db:"doc_number"`
	Lang         string  `db:"lang"`
	Title        string  `db:"title"`
	SectionIndex int     `db:"section_index"`
	Heading      *string `db:"heading"`
	Score        float32 `db:"score"`
	Headline     string  `db:"headline"`
}

// SearchLibrarySections performs full-text search over library document
// sections. Ranking happens in a subquery so ts_headline (CPU-heavy on long
// bodies) only runs on the returned page. Pass "" for lang to search all.
func (db *DB) SearchLibrarySections(ctx context.Context, query, lang string, limit, offset int) ([]LibrarySectionRow, error) {
	defer observe("search_library_sections", time.Now())
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
          ranked AS (
            SELECT
                d.doc_type, d.doc_number, d.lang, d.title,
                s.section_index, s.heading, s.body,
                ts_rank_cd(s.content_tsv, tsq.q)::real AS score
            FROM  library_sections s
            JOIN  library_documents d ON d.id = s.document_id
            CROSS JOIN tsq
            WHERE s.content_tsv @@ tsq.q
              AND ($2::text = '' OR d.lang = $2)
            ORDER BY score DESC, d.doc_type, COALESCE(d.doc_number, 0), s.section_index
            LIMIT  $3
            OFFSET $4
          )
        SELECT
            r.doc_type, r.doc_number, r.lang, r.title, r.section_index, r.heading, r.score,
            ts_headline(
              'simple', r.body, tsq.q,
              'MaxWords=25,MinWords=8,MaxFragments=2,StartSel=<b>,StopSel=</b>'
            ) AS headline
        FROM  ranked r
        CROSS JOIN tsq
        ORDER BY r.score DESC, r.doc_type, COALESCE(r.doc_number, 0), r.section_index
	`
	rows, err := db.Pool.Query(ctx, sql, query, lang, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[LibrarySectionRow])
}

// CountSearchLibrarySections returns the total matching sections for
// pagination metadata. Pass "" for lang to count across all languages.
func (db *DB) CountSearchLibrarySections(ctx context.Context, query, lang string) (int, error) {
	defer observe("count_search_library_sections", time.Now())
	sql := `
        WITH tsq AS (
          SELECT
            CASE
              WHEN $1 ~ '^[A-Za-z]+$'
              THEN to_tsquery('simple', lower($1) || ':*')
              ELSE websearch_to_tsquery('simple', $1)
            END AS q
        )
        SELECT COUNT(*)
        FROM  library_sections s
        JOIN  library_documents d ON d.id = s.document_id
        CROSS JOIN tsq
        WHERE s.content_tsv @@ tsq.q
          AND ($2::text = '' OR d.lang = $2)
	`
	var n int
	if err := db.Pool.QueryRow(ctx, sql, query, lang).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// LibraryBundleRow is one section row for the offline `library` bundle.
type LibraryBundleRow struct {
	DocType      string  `db:"doc_type"`
	DocNumber    *int    `db:"doc_number"`
	Title        string  `db:"title"`
	SectionIndex int     `db:"section_index"`
	Heading      *string `db:"heading"`
	Body         string  `db:"body"`
}

// FetchLibrarySections returns every section of every library document in one
// language, in stable document order, for the offline bundle builder.
func (db *DB) FetchLibrarySections(ctx context.Context, lang string) ([]LibraryBundleRow, error) {
	defer observe("fetch_library_sections", time.Now())
	sql := `
        SELECT d.doc_type, d.doc_number, d.title, s.section_index, s.heading, s.body
        FROM  library_sections s
        JOIN  library_documents d ON d.id = s.document_id
        WHERE d.lang = $1
        ORDER BY d.doc_type, COALESCE(d.doc_number, 0), s.section_index
	`
	rows, err := db.Pool.Query(ctx, sql, lang)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[LibraryBundleRow])
}
