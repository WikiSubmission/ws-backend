package db

import "context"

// QuranVerseOrdinal returns the 1-based absolute position of a Quran verse
// across the whole scripture (e.g. 1:1 -> 1, 2:1 -> 8 since chapter 1 has 7
// verses). It sums the verse_count of every earlier chapter and adds the
// verse number. Used to count how many verses a reading marker has advanced.
func (db *DB) QuranVerseOrdinal(ctx context.Context, chapter, verse int) (int, error) {
	const q = `SELECT COALESCE((SELECT SUM(verse_count) FROM chapters WHERE chapter_number < $1), 0)::int + $2`
	var ord int
	if err := db.Pool.QueryRow(ctx, q, chapter, verse).Scan(&ord); err != nil {
		return 0, err
	}
	return ord, nil
}

// BibleVerseOrdinal returns the 1-based absolute position of a Bible verse
// across the whole canon, summing the verse_count of every chapter that
// precedes (book, chapter) in book/chapter order and adding the verse number.
func (db *DB) BibleVerseOrdinal(ctx context.Context, book, chapter, verse int) (int, error) {
	const q = `SELECT COALESCE((
		SELECT SUM(bc.verse_count)
		FROM bible_chapters bc
		JOIN bible_books bb ON bb.id = bc.book_id
		WHERE bb.book_number < $1 OR (bb.book_number = $1 AND bc.chapter_number < $2)
	), 0)::int + $3`
	var ord int
	if err := db.Pool.QueryRow(ctx, q, book, chapter, verse).Scan(&ord); err != nil {
		return 0, err
	}
	return ord, nil
}
