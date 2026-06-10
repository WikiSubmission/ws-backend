package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Editorial Quran word-by-word reads and draft upserts (Phase 2). Word drafts
// live in the version-keyed word_text_drafts table; publishing folds into the
// chapter publish workflow (see syncWordPublished in editorial_quran_publish.go)
// and writes the version-keyed quran_word_text_published table. The live reader
// corpus (word_text) is never written here. The per-word root meaning is read by
// JOIN from quran_root_meaning_drafts / quran_root_meanings_published, so a
// single root-meaning edit is reflected for every word sharing the root.

// QuranWordFields holds the editable per-word text for a version.
type QuranWordFields struct {
	Word    *string
	Meaning *string
}

// QuranWordEditorRow is the assembled per-word editing surface.
type QuranWordEditorRow struct {
	WordID               int
	WordIndex            int
	GlobalIndex          *int
	Arabic               *string
	Root                 *string
	Draft                *QuranWordFields
	DraftStatus          *string
	Published            *QuranWordFields
	Reference            *QuranWordFields
	RootMeaning          *string
	RootMeaningPublished *string
}

// QuranVerseWordsRow groups the words of one verse in reading order.
type QuranVerseWordsRow struct {
	VerseNumber int
	Words       []QuranWordEditorRow
}

// WordLocation is a canonical word's place in the corpus.
type WordLocation struct {
	WordID        int
	ChapterNumber int
	VerseNumber   int
}

// wordSelectColumns is shared by the chapter assembly and the single-word fetch.
// refLangID ($3) and refVersionID ($4) may be 0 when no reference version is
// configured; the reference joins then match nothing. versionID is $2.
const wordSelectColumns = `
	w.id, w.word_index, w.global_index, v.verse_index,
	arwt.word AS arabic, arwt.word_root AS root,
	d.word AS draft_word, d.meaning AS draft_meaning, d.status::text AS draft_status,
	(p.id IS NOT NULL) AS has_published, p.word AS pub_word, p.meaning AS pub_meaning,
	COALESCE(refp.word, refwt.word)       AS ref_word,
	COALESCE(refp.meaning, refwt.meaning) AS ref_meaning,
	rmd.meaning AS root_meaning_draft,
	rmp.meaning AS root_meaning_published`

const wordSelectFrom = `
	FROM words w
	JOIN verses v ON v.id = w.verse_id
	LEFT JOIN languages arl ON arl.code = 'ar'
	LEFT JOIN word_text arwt ON arwt.word_id = w.id AND arwt.lang_id = arl.id
	LEFT JOIN word_text_drafts d ON d.word_id = w.id AND d.version_id = $2
	LEFT JOIN quran_word_text_published p ON p.word_id = w.id AND p.version_id = $2
	LEFT JOIN word_text refwt ON refwt.word_id = w.id AND refwt.lang_id = $3
	LEFT JOIN quran_word_text_published refp ON refp.word_id = w.id AND refp.version_id = $4
	LEFT JOIN quran_roots r ON r.letters = arwt.word_root
	LEFT JOIN quran_root_meaning_drafts rmd ON rmd.root_id = r.id AND rmd.version_id = $2
	LEFT JOIN quran_root_meanings_published rmp ON rmp.root_id = r.id AND rmp.version_id = $2`

// scanWordRow consumes one wordSelectColumns row.
func scanWordRow(rows pgx.Rows) (int, QuranWordEditorRow, error) {
	var (
		verseIndex   int
		row          QuranWordEditorRow
		draftWord    *string
		draftMeaning *string
		draftStatus  *string
		hasPublished bool
		pubWord      *string
		pubMeaning   *string
		refWord      *string
		refMeaning   *string
	)
	if err := rows.Scan(
		&row.WordID, &row.WordIndex, &row.GlobalIndex, &verseIndex,
		&row.Arabic, &row.Root,
		&draftWord, &draftMeaning, &draftStatus,
		&hasPublished, &pubWord, &pubMeaning,
		&refWord, &refMeaning,
		&row.RootMeaning, &row.RootMeaningPublished,
	); err != nil {
		return 0, QuranWordEditorRow{}, err
	}
	if draftStatus != nil {
		row.Draft = &QuranWordFields{Word: draftWord, Meaning: draftMeaning}
		row.DraftStatus = draftStatus
	}
	if hasPublished {
		row.Published = &QuranWordFields{Word: pubWord, Meaning: pubMeaning}
	}
	if refWord != nil || refMeaning != nil {
		row.Reference = &QuranWordFields{Word: refWord, Meaning: refMeaning}
	}
	return verseIndex, row, nil
}

// FetchQuranChapterWords assembles the fixed canonical word list for every verse
// of a chapter, each word carrying canonical Arabic + root, this version's draft
// and published text, the reference version's text, and the per-version root
// meaning. referenceVersionID / referenceLangID are 0 when no reference is set.
func (db *DB) FetchQuranChapterWords(ctx context.Context, versionID, chapterID, referenceVersionID, referenceLangID int) ([]QuranVerseWordsRow, error) {
	defer observe("fetch_quran_chapter_words", time.Now())
	rows, err := db.Pool.Query(ctx,
		`SELECT`+wordSelectColumns+wordSelectFrom+`
		 WHERE v.chapter_id = $1
		 ORDER BY v.verse_index, w.word_index`,
		chapterID, versionID, referenceLangID, referenceVersionID)
	if err != nil {
		return nil, fmt.Errorf("fetch_quran_chapter_words: %w", err)
	}
	defer rows.Close()

	var out []QuranVerseWordsRow
	idxByVerse := make(map[int]int)
	for rows.Next() {
		verseIndex, row, err := scanWordRow(rows)
		if err != nil {
			return nil, fmt.Errorf("fetch_quran_chapter_words: scan: %w", err)
		}
		pos, ok := idxByVerse[verseIndex]
		if !ok {
			out = append(out, QuranVerseWordsRow{VerseNumber: verseIndex})
			pos = len(out) - 1
			idxByVerse[verseIndex] = pos
		}
		out[pos].Words = append(out[pos].Words, row)
	}
	return out, rows.Err()
}

// FetchWordLocation resolves a canonical word id to its chapter+verse number, or
// (zero, false) when the word id is unknown.
func (db *DB) FetchWordLocation(ctx context.Context, wordID int) (WordLocation, bool, error) {
	defer observe("fetch_word_location", time.Now())
	var loc WordLocation
	err := db.Pool.QueryRow(ctx, `
		SELECT w.id, c.chapter_number, v.verse_index
		FROM words w
		JOIN verses v ON v.id = w.verse_id
		JOIN chapters c ON c.id = v.chapter_id
		WHERE w.id = $1`, wordID).Scan(&loc.WordID, &loc.ChapterNumber, &loc.VerseNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return WordLocation{}, false, nil
	}
	if err != nil {
		return WordLocation{}, false, fmt.Errorf("fetch_word_location: %w", err)
	}
	return loc, true, nil
}

// FetchQuranWord returns one assembled word row for a version, or (nil, nil) when
// the word id is unknown.
func (db *DB) FetchQuranWord(ctx context.Context, versionID, wordID, referenceVersionID, referenceLangID int) (*QuranWordEditorRow, error) {
	defer observe("fetch_quran_word", time.Now())
	rows, err := db.Pool.Query(ctx,
		`SELECT`+wordSelectColumns+wordSelectFrom+`
		 WHERE w.id = $1`,
		wordID, versionID, referenceLangID, referenceVersionID)
	if err != nil {
		return nil, fmt.Errorf("fetch_quran_word: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("fetch_quran_word: %w", err)
		}
		return nil, nil
	}
	_, row, err := scanWordRow(rows)
	if err != nil {
		return nil, fmt.Errorf("fetch_quran_word: scan: %w", err)
	}
	return &row, nil
}

// UpsertWordDraft creates or updates one word draft and resets it to 'draft'.
// Sending nil fields clears them.
func (db *DB) UpsertWordDraft(ctx context.Context, versionID, wordID, langID int, fields QuranWordFields, authorID int) error {
	defer observe("upsert_word_draft", time.Now())
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO word_text_drafts (word_id, lang_id, version_id, word, meaning, status, author_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'draft', $6, now())
		ON CONFLICT (version_id, word_id) WHERE version_id IS NOT NULL
		DO UPDATE SET word = EXCLUDED.word,
		              meaning = EXCLUDED.meaning,
		              status = 'draft',
		              author_id = EXCLUDED.author_id,
		              updated_at = now()`,
		wordID, langID, versionID, fields.Word, fields.Meaning, authorID)
	if err != nil {
		return fmt.Errorf("upsert_word_draft: %w", err)
	}
	return nil
}
