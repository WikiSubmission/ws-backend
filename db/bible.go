package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// BibleBookRow holds one book's metadata, including aggregate counts.
type BibleBookRow struct {
	BookNumber   int    `db:"book_number"`
	Name         string `db:"name"`
	Testament    string `db:"testament"`
	ChapterCount int    `db:"chapter_count"`
	VerseCount   int    `db:"verse_count"`
}

// BibleVerseRow holds one language-row from a Bible verse fetch.
type BibleVerseRow struct {
	BookNumber     int      `db:"book_number"`
	BookName       string   `db:"book_name"`
	Testament      string   `db:"testament"`
	ChapterNumber  int      `db:"chapter_number"`
	VerseNumber    int      `db:"verse_number"`
	LangID         int      `db:"lang_id"`
	LangCode       string   `db:"lang_code"`
	Direction      string   `db:"direction"`
	Content        string   `db:"content"`
	Footer         *string  `db:"footer"`
	TheoFootnotes  []string `db:"theo_footnotes"`
}

// SearchBibleVerseRow holds one language-row from a Bible FTS query.
type SearchBibleVerseRow struct {
	BookNumber    int      `db:"book_number"`
	BookName      string   `db:"book_name"`
	Testament     string   `db:"testament"`
	ChapterNumber int      `db:"chapter_number"`
	VerseNumber   int      `db:"verse_number"`
	LangID        int      `db:"lang_id"`
	LangCode      string   `db:"lang_code"`
	Direction     string   `db:"direction"`
	Content       string   `db:"content"`
	Footer        *string  `db:"footer"`
	TheoFootnotes []string `db:"theo_footnotes"`
	Score         float32  `db:"score"`
	Headline      string   `db:"headline"`
}

// FetchBibleBooks returns all Bible books ordered by book_number, with
// precomputed chapter and verse counts.
func (db *DB) FetchBibleBooks(ctx context.Context) ([]BibleBookRow, error) {
	defer observe("fetch_bible_books", time.Now())
	sql := `
		SELECT
			bb.book_number,
			bb.name,
			bb.testament,
			COUNT(bc.id)::int                    AS chapter_count,
			COALESCE(SUM(bc.verse_count), 0)::int AS verse_count
		FROM bible_books bb
		LEFT JOIN bible_chapters bc ON bc.book_id = bb.id
		GROUP BY bb.id, bb.book_number, bb.name, bb.testament
		ORDER BY bb.book_number
	`
	rows, err := db.Pool.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[BibleBookRow])
}

// FetchBibleVerses returns verses (with translations) for the given book and
// chapter/verse range, filtered to the requested language codes.
func (db *DB) FetchBibleVerses(ctx context.Context, bookNum, startCh, endCh, startV, endV int, langs []string) ([]BibleVerseRow, error) {
	defer observe("fetch_bible_verses", time.Now())
	sql := `
		SELECT
			bb.book_number,
			bb.name       AS book_name,
			bb.testament,
			bc.chapter_number,
			bv.verse_number,
			l.id          AS lang_id,
			l.code        AS lang_code,
			l.direction,
			bvt.content,
			bvt.footer,
			COALESCE(bvt.theo_footnotes, '{}') AS theo_footnotes
		FROM bible_verse_text bvt
		JOIN bible_verses   bv  ON bvt.verse_id   = bv.id
		JOIN bible_chapters bc  ON bv.chapter_id  = bc.id
		JOIN bible_books    bb  ON bc.book_id      = bb.id
		JOIN languages      l   ON bvt.lang_id     = l.id
		WHERE bb.book_number    = $1
		  AND bc.chapter_number >= $2
		  AND bc.chapter_number <= $3
		  AND bv.verse_number   >= $4
		  AND bv.verse_number   <= $5
		  AND l.code = ANY($6)
		ORDER BY bc.chapter_number, bv.verse_number, l.id
	`
	rows, err := db.Pool.Query(ctx, sql, bookNum, startCh, endCh, startV, endV, langs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[BibleVerseRow])
}

// SearchBibleVerses performs full-text search over bible_verse_text using the
// 'simple' dictionary (consistent with Quran FTS). Pass nil for langs to search
// all languages.
func (db *DB) SearchBibleVerses(ctx context.Context, query string, langs []string, limit, offset int) ([]SearchBibleVerseRow, error) {
	defer observe("search_bible_verses", time.Now())
	sql := `
		WITH tsq AS (
			SELECT websearch_to_tsquery('simple', $1) AS q
		)
		SELECT
			bb.book_number,
			bb.name       AS book_name,
			bb.testament,
			bc.chapter_number,
			bv.verse_number,
			l.id          AS lang_id,
			l.code        AS lang_code,
			l.direction,
			bvt.content,
			bvt.footer,
			COALESCE(bvt.theo_footnotes, '{}') AS theo_footnotes,
			ts_rank_cd(bvt.content_tsv, tsq.q)::real AS score,
			ts_headline(
				'simple', bvt.content, tsq.q,
				'MaxWords=20,MinWords=8,MaxFragments=2,StartSel=<b>,StopSel=</b>'
			) AS headline
		FROM bible_verse_text bvt
		JOIN bible_verses   bv  ON bvt.verse_id  = bv.id
		JOIN bible_chapters bc  ON bv.chapter_id = bc.id
		JOIN bible_books    bb  ON bc.book_id     = bb.id
		JOIN languages      l   ON bvt.lang_id    = l.id
		CROSS JOIN tsq
		WHERE bvt.content_tsv @@ tsq.q
		  AND ($2::text[] IS NULL OR l.code = ANY($2::text[]))
		ORDER BY score DESC, bb.book_number, bc.chapter_number, bv.verse_number
		LIMIT $3 OFFSET $4
	`
	rows, err := db.Pool.Query(ctx, sql, query, langs, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[SearchBibleVerseRow])
}

// FetchBibleVersesByRefs fetches Bible verse content for a disjoint set of
// (book_number, chapter_number, verse_number) triples using a parallel unnest join.
// Use this for Phase B of a two-phase search (find refs → fetch in response langs).
func (db *DB) FetchBibleVersesByRefs(ctx context.Context, bookNums, chapterNums, verseNums []int32, langs []string) ([]BibleVerseRow, error) {
	defer observe("fetch_bible_verses_by_refs", time.Now())
	sql := `
		SELECT
			bb.book_number,
			bb.name       AS book_name,
			bb.testament,
			bc.chapter_number,
			bv.verse_number,
			l.id          AS lang_id,
			l.code        AS lang_code,
			l.direction,
			bvt.content,
			bvt.footer,
			COALESCE(bvt.theo_footnotes, '{}') AS theo_footnotes
		FROM unnest($1::int[], $2::int[], $3::int[]) AS ref(bn, cn, vn)
		JOIN bible_books      bb  ON bb.book_number    = ref.bn
		JOIN bible_chapters   bc  ON bc.book_id        = bb.id AND bc.chapter_number = ref.cn
		JOIN bible_verses     bv  ON bv.chapter_id     = bc.id AND bv.verse_number   = ref.vn
		JOIN bible_verse_text bvt ON bvt.verse_id      = bv.id
		JOIN languages        l   ON l.id              = bvt.lang_id
		WHERE ($4::text[] IS NULL OR l.code = ANY($4))
		ORDER BY bb.book_number, bc.chapter_number, bv.verse_number, l.id
	`
	rows, err := db.Pool.Query(ctx, sql, bookNums, chapterNums, verseNums, langs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[BibleVerseRow])
}

// CountSearchBibleVerses returns the total number of distinct Bible verses
// matching the query, for pagination metadata.
func (db *DB) CountSearchBibleVerses(ctx context.Context, query string, langs []string) (int, error) {
	defer observe("count_search_bible_verses", time.Now())
	sql := `
		WITH tsq AS (
			SELECT websearch_to_tsquery('simple', $1) AS q
		)
		SELECT COUNT(DISTINCT bv.id)
		FROM bible_verse_text bvt
		JOIN bible_verses bv ON bvt.verse_id = bv.id
		JOIN languages    l  ON bvt.lang_id  = l.id
		CROSS JOIN tsq
		WHERE bvt.content_tsv @@ tsq.q
		  AND ($2::text[] IS NULL OR l.code = ANY($2::text[]))
	`
	var count int
	err := db.Pool.QueryRow(ctx, sql, query, langs).Scan(&count)
	return count, err
}
