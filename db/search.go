package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	pgvector "github.com/pgvector/pgvector-go"
)

// SearchVerseRow holds one language-row from a verse FTS query.
type SearchVerseRow struct {
	ChapterNumber int     `db:"chapter_number"`
	ChapterTitle  string  `db:"chapter_title"`
	VerseIndex    int     `db:"verse_index"`
	LangID        int     `db:"lang_id"`
	LangCode      string  `db:"lang_code"`
	Direction     string  `db:"direction"`
	Content       string  `db:"content"`
	Subtitle      *string `db:"subtitle"`
	Footer        *string `db:"footer"`
	Score         float32 `db:"score"`
	Headline      string  `db:"headline"`
	TitleMatch    bool    `db:"title_match"`
}

// SearchWordRow holds one language-row from a word FTS query.
type SearchWordRow struct {
	ChapterNumber int     `db:"chapter_number"`
	VerseIndex    int     `db:"verse_index"`
	WordIndex     int     `db:"word_index"`
	GlobalIndex   int     `db:"global_index"`
	LangCode      string  `db:"lang_code"`
	WordText      string  `db:"word_text"`
	WordRoot      *string `db:"word_root"`
	Meaning       *string `db:"meaning"`
	Score         float32 `db:"score"`
	Headline      string  `db:"headline"`
}

// SearchVerses performs full-text search over verse_text (content + subtitle +
// footer) and boosts chapters whose title also matches the query.
//
// Pass nil for langs to search across all languages.
func (db *DB) SearchVerses(ctx context.Context, query string, langs []string, limit, offset int) ([]SearchVerseRow, error) {
	sql := `
        WITH
          tsq AS (
            SELECT websearch_to_tsquery('simple', $1) AS q
          ),
          title_hits AS (
            SELECT DISTINCT ct.chapter_id
            FROM   chapter_text ct
            CROSS  JOIN tsq
            WHERE  ct.title_tsv @@ tsq.q
              AND  ($2::text[] IS NULL OR EXISTS (
                     SELECT 1 FROM languages lx
                     WHERE  lx.id = ct.lang_id
                       AND  lx.code = ANY($2::text[])
                   ))
          )
        SELECT
            c.chapter_number,
            COALESCE(ct.title, '') AS chapter_title,
            v.verse_index,
            l.id   AS lang_id,
            l.code AS lang_code,
            l.direction,
            vt.content,
            vt.subtitle,
            vt.footer,
            (
              ts_rank_cd(vt.content_tsv, tsq.q) +
              CASE WHEN c.id IN (SELECT chapter_id FROM title_hits) THEN 0.3 ELSE 0 END
            )::real AS score,
            ts_headline(
              'simple', vt.content, tsq.q,
              'MaxWords=20,MinWords=8,MaxFragments=2,StartSel=<b>,StopSel=</b>'
            ) AS headline,
            (c.id IN (SELECT chapter_id FROM title_hits)) AS title_match
        FROM  verse_text vt
        JOIN  verses       v  ON vt.verse_id  = v.id
        JOIN  chapters     c  ON v.chapter_id = c.id
        JOIN  languages    l  ON vt.lang_id   = l.id
        LEFT  JOIN chapter_text ct
              ON  c.id = ct.chapter_id AND ct.lang_id = l.id
        CROSS JOIN tsq
        WHERE vt.content_tsv @@ tsq.q
          AND ($2::text[] IS NULL OR l.code = ANY($2::text[]))
        ORDER BY score DESC, c.chapter_number, v.verse_index
        LIMIT  $3
        OFFSET $4
	`
	rows, err := db.Pool.Query(ctx, sql, query, langs, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[SearchVerseRow])
}

// CountSearchVerses returns the total number of distinct verses matching the
// query (across all requested languages), for pagination metadata.
func (db *DB) CountSearchVerses(ctx context.Context, query string, langs []string) (int, error) {
	sql := `
        WITH
          tsq AS (
            SELECT websearch_to_tsquery('simple', $1) AS q
          ),
          title_hits AS (
            SELECT DISTINCT ct.chapter_id
            FROM   chapter_text ct
            CROSS  JOIN tsq
            WHERE  ct.title_tsv @@ tsq.q
              AND  ($2::text[] IS NULL OR EXISTS (
                     SELECT 1 FROM languages lx
                     WHERE  lx.id = ct.lang_id
                       AND  lx.code = ANY($2::text[])
                   ))
          )
        SELECT COUNT(DISTINCT v.id)
        FROM  verse_text vt
        JOIN  verses    v  ON vt.verse_id  = v.id
        JOIN  chapters  c  ON v.chapter_id = c.id
        JOIN  languages l  ON vt.lang_id   = l.id
        CROSS JOIN tsq
        WHERE vt.content_tsv @@ tsq.q
          AND ($2::text[] IS NULL OR l.code = ANY($2::text[]))
	`
	var count int
	err := db.Pool.QueryRow(ctx, sql, query, langs).Scan(&count)
	return count, err
}

// SearchVersesHybrid combines FTS and vector search via Reciprocal Rank Fusion
// (RRF). When queryEmbedding is nil or empty it falls back to SearchVerses.
//
// RRF formula: score = Σ 1/(60 + rank_i) across the FTS and vector legs.
// Both legs are capped at 200 candidates before merging, keeping the query fast.
func (db *DB) SearchVersesHybrid(
	ctx context.Context,
	query string,
	queryEmbedding []float32,
	langs []string,
	limit, offset int,
) ([]SearchVerseRow, error) {
	if len(queryEmbedding) == 0 {
		return db.SearchVerses(ctx, query, langs, limit, offset)
	}

	vec := pgvector.NewVector(queryEmbedding)

	sql := `
        WITH
          tsq AS (SELECT websearch_to_tsquery('simple', $1) AS q),
          fts AS (
            SELECT
              vt.verse_id,
              vt.lang_id,
              ROW_NUMBER() OVER (ORDER BY ts_rank_cd(vt.content_tsv, tsq.q) DESC) AS rnk,
              ts_headline(
                'simple', vt.content, tsq.q,
                'MaxWords=20,MinWords=8,MaxFragments=2,StartSel=<b>,StopSel=</b>'
              ) AS headline,
              EXISTS (
                SELECT 1 FROM chapter_text ct2
                JOIN verses v2 ON v2.chapter_id = ct2.chapter_id
                WHERE v2.id = vt.verse_id AND ct2.title_tsv @@ tsq.q
              ) AS title_match
            FROM verse_text vt CROSS JOIN tsq
            JOIN languages l ON l.id = vt.lang_id
            WHERE vt.content_tsv @@ tsq.q
              AND ($3::text[] IS NULL OR l.code = ANY($3::text[]))
            LIMIT 200
          ),
          vec AS (
            SELECT
              vt.verse_id,
              vt.lang_id,
              ROW_NUMBER() OVER (ORDER BY vt.embedding <=> $2) AS rnk
            FROM verse_text vt
            JOIN languages l ON l.id = vt.lang_id
            WHERE vt.embedding IS NOT NULL
              AND ($3::text[] IS NULL OR l.code = ANY($3::text[]))
            ORDER BY vt.embedding <=> $2
            LIMIT 200
          ),
          rrf AS (
            SELECT
              COALESCE(f.verse_id, v.verse_id) AS verse_id,
              COALESCE(f.lang_id,  v.lang_id)  AS lang_id,
              COALESCE(1.0/(60.0+f.rnk), 0) + COALESCE(1.0/(60.0+v.rnk), 0) AS score,
              COALESCE(f.headline, '')         AS headline,
              COALESCE(f.title_match, false)   AS title_match
            FROM fts f
            FULL JOIN vec v ON v.verse_id = f.verse_id AND v.lang_id = f.lang_id
          )
        SELECT
          c.chapter_number,
          COALESCE(ct.title, '')  AS chapter_title,
          vr.verse_index,
          l.id                    AS lang_id,
          l.code                  AS lang_code,
          l.direction,
          vt.content,
          vt.subtitle,
          vt.footer,
          r.score::real           AS score,
          r.headline,
          r.title_match
        FROM rrf r
        JOIN verse_text  vt ON vt.verse_id = r.verse_id AND vt.lang_id = r.lang_id
        JOIN verses      vr ON vr.id = vt.verse_id
        JOIN chapters    c  ON c.id  = vr.chapter_id
        JOIN languages   l  ON l.id  = vt.lang_id
        LEFT JOIN chapter_text ct ON ct.chapter_id = c.id AND ct.lang_id = l.id
        ORDER BY r.score DESC, c.chapter_number, vr.verse_index
        LIMIT  $4
        OFFSET $5
	`

	rows, err := db.Pool.Query(ctx, sql, query, vec, langs, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[SearchVerseRow])
}

// SearchWords performs full-text search over word_text (word + meaning).
//
// Pass nil for langs to search across all languages.
func (db *DB) SearchWords(ctx context.Context, query string, langs []string, limit, offset int) ([]SearchWordRow, error) {
	sql := `
        WITH tsq AS (
          SELECT websearch_to_tsquery('simple', $1) AS q
        )
        SELECT
            c.chapter_number,
            v.verse_index,
            w.word_index,
            COALESCE(w.global_index, 0) AS global_index,
            l.code  AS lang_code,
            wt.word AS word_text,
            wt.word_root,
            wt.meaning,
            ts_rank_cd(wt.word_tsv, tsq.q)::real AS score,
            ts_headline(
              'simple', wt.word, tsq.q,
              'MaxWords=5,MinWords=1,MaxFragments=1,StartSel=<b>,StopSel=</b>'
            ) AS headline
        FROM  word_text wt
        JOIN  words   w  ON wt.word_id  = w.id
        JOIN  verses  v  ON w.verse_id  = v.id
        JOIN  chapters c ON v.chapter_id = c.id
        JOIN  languages l ON wt.lang_id  = l.id
        CROSS JOIN tsq
        WHERE wt.word_tsv @@ tsq.q
          AND ($2::text[] IS NULL OR l.code = ANY($2::text[]))
        ORDER BY score DESC, c.chapter_number, v.verse_index, w.word_index
        LIMIT  $3
        OFFSET $4
	`
	rows, err := db.Pool.Query(ctx, sql, query, langs, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[SearchWordRow])
}

// SearchWordsByRoot fetches all word occurrences whose word_root exactly equals
// the given root string. No full-text scoring — all results have score 1.0.
//
// Pass nil for langs to return all languages.
func (db *DB) SearchWordsByRoot(ctx context.Context, root string, langs []string, limit, offset int) ([]SearchWordRow, error) {
	sql := `
        SELECT
            c.chapter_number,
            v.verse_index,
            w.word_index,
            COALESCE(w.global_index, 0) AS global_index,
            l.code  AS lang_code,
            wt.word AS word_text,
            wt.word_root,
            wt.meaning,
            1.0::real AS score,
            wt.word  AS headline
        FROM  word_text wt
        JOIN  words    w  ON wt.word_id   = w.id
        JOIN  verses   v  ON w.verse_id   = v.id
        JOIN  chapters c  ON v.chapter_id = c.id
        JOIN  languages l ON wt.lang_id   = l.id
        WHERE wt.word_root = $1
          AND ($2::text[] IS NULL OR l.code = ANY($2::text[]))
        ORDER BY c.chapter_number, v.verse_index, w.word_index
        LIMIT  $3
        OFFSET $4
	`
	rows, err := db.Pool.Query(ctx, sql, root, langs, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[SearchWordRow])
}

// CountSearchWordsByRoot returns the total number of distinct words whose
// word_root exactly equals root, for pagination metadata.
func (db *DB) CountSearchWordsByRoot(ctx context.Context, root string, langs []string) (int, error) {
	sql := `
        SELECT COUNT(DISTINCT w.id)
        FROM  word_text wt
        JOIN  words    w  ON wt.word_id = w.id
        JOIN  languages l ON wt.lang_id = l.id
        WHERE wt.word_root = $1
          AND ($2::text[] IS NULL OR l.code = ANY($2::text[]))
	`
	var count int
	err := db.Pool.QueryRow(ctx, sql, root, langs).Scan(&count)
	return count, err
}

// CountSearchWords returns the total number of distinct words matching the
// query (across all requested languages), for pagination metadata.
func (db *DB) CountSearchWords(ctx context.Context, query string, langs []string) (int, error) {
	sql := `
        WITH tsq AS (
          SELECT websearch_to_tsquery('simple', $1) AS q
        )
        SELECT COUNT(DISTINCT w.id)
        FROM  word_text wt
        JOIN  words    w  ON wt.word_id = w.id
        JOIN  languages l ON wt.lang_id = l.id
        CROSS JOIN tsq
        WHERE wt.word_tsv @@ tsq.q
          AND ($2::text[] IS NULL OR l.code = ANY($2::text[]))
	`
	var count int
	err := db.Pool.QueryRow(ctx, sql, query, langs).Scan(&count)
	return count, err
}
