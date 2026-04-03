package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/wikiSubmission/ws-backend/db"
)

var newTestamentBooks = map[string]bool{
	"Matthew": true, "Mark": true, "Luke": true, "John": true,
	"Acts": true, "Romans": true, "1 Corinthians": true, "2 Corinthians": true,
	"Galatians": true, "Ephesians": true, "Philippians": true, "Colossians": true,
	"1 Thessalonians": true, "2 Thessalonians": true, "1 Timothy": true,
	"2 Timothy": true, "Titus": true, "Philemon": true, "Hebrews": true,
	"James": true, "1 Peter": true, "2 Peter": true, "1 John": true,
	"2 John": true, "3 John": true, "Jude": true, "Revelation": true,
}

type chapterCSVRow struct {
	GlobalChNum int
	VerseCount  int
	BookName    string
}

type verseData struct {
	chapterID     int
	verseNum      int
	content       string
	footnote      *string  // kept for footer column (unused in new seeds)
	theoFootnotes []string // theological footnotes → theo_footnotes column
}

const batchSize = 500

func main() {
	ctx := context.Background()
	database, err := db.DbSetup()
	if err != nil {
		slog.Error("Database setup failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer database.Close()

	// 1. Language map
	langMap := make(map[string]int16)
	rows, _ := database.Pool.Query(ctx, "SELECT id, code FROM languages")
	for rows.Next() {
		var id int16
		var code string
		rows.Scan(&id, &code)
		langMap[code] = id
	}
	rows.Close()

	enLangID := langMap["en"]
	if enLangID == 0 {
		slog.Error("English language not found in languages table — run the main seed first")
		os.Exit(1)
	}

	// 2. Parse chapters CSV
	fmt.Println("📖 Parsing chapters CSV...")
	chapterRows, err := parseChaptersCSV("assets/bible/ws_bible_chapters_rows.csv")
	if err != nil {
		slog.Error("Failed to parse chapters CSV", slog.Any("error", err))
		os.Exit(1)
	}
	fmt.Printf("   Found %d chapter rows\n", len(chapterRows))

	// 3. Parse footnotes CSV into map: "globalChNum:verseNum" → []footnoteText
	fmt.Println("📝 Parsing footnotes CSV...")
	footnoteMap, err := parseFootnotesCSV("assets/bible/ws_bible_footnotes_rows.csv")
	if err != nil {
		slog.Error("Failed to parse footnotes CSV", slog.Any("error", err))
		os.Exit(1)
	}
	fmt.Printf("   Found %d footnote entries\n", len(footnoteMap))

	// 4. Parse entire text CSV into memory (4.5 MB — fine to hold in RAM)
	fmt.Println("📜 Parsing verse text CSV...")
	allVerses, globalChToVerses, err := parseTextCSV("assets/bible/ws_bible_text_rows.csv")
	if err != nil {
		slog.Error("Failed to parse text CSV", slog.Any("error", err))
		os.Exit(1)
	}
	fmt.Printf("   Found %d verses\n", len(allVerses))
	_ = globalChToVerses

	// 5. Determine unique books (ordered by first appearance)
	bookOrder := []string{}
	seenBooks := map[string]bool{}
	for _, row := range chapterRows {
		if !seenBooks[row.BookName] {
			seenBooks[row.BookName] = true
			bookOrder = append(bookOrder, row.BookName)
		}
	}

	// 6. Insert books — single batch (66 queries → 1 round-trip)
	fmt.Println("📚 Inserting books...")
	bookNameToID := make(map[string]int16)
	booksBatch := &pgx.Batch{}
	for i, bookName := range bookOrder {
		bookNum := i + 1
		testament := "OT"
		if newTestamentBooks[bookName] {
			testament = "NT"
		}
		booksBatch.Queue(
			`INSERT INTO bible_books (book_number, name, testament) VALUES ($1, $2, $3)
			 ON CONFLICT (book_number) DO UPDATE SET name = EXCLUDED.name, testament = EXCLUDED.testament
			 RETURNING id`,
			bookNum, bookName, testament,
		)
	}
	bbr := database.Pool.SendBatch(ctx, booksBatch)
	for _, bookName := range bookOrder {
		var bookID int16
		if err := bbr.QueryRow().Scan(&bookID); err != nil {
			slog.Error("Failed to insert book", "book", bookName, slog.Any("error", err))
			os.Exit(1)
		}
		bookNameToID[bookName] = bookID
	}
	bbr.Close()
	fmt.Printf("   Inserted %d books\n", len(bookOrder))

	// 7. Insert chapters — single batch (1189 queries → 1 round-trip)
	fmt.Println("📑 Inserting chapters...")
	globalChToChapterID := make(map[int]int)
	chapterInBook := make(map[string]int)

	// Build ordered list of (globalChNum, bookID, chNum, verseCount) for batch
	type chapterInsert struct {
		globalChNum int
		bookID      int16
		chNum       int
		verseCount  int
	}
	chInserts := make([]chapterInsert, 0, len(chapterRows))
	for _, row := range chapterRows {
		bookID := bookNameToID[row.BookName]
		chapterInBook[row.BookName]++
		chInserts = append(chInserts, chapterInsert{
			globalChNum: row.GlobalChNum,
			bookID:      bookID,
			chNum:       chapterInBook[row.BookName],
			verseCount:  row.VerseCount,
		})
	}

	chBatch := &pgx.Batch{}
	for _, ci := range chInserts {
		chBatch.Queue(
			`INSERT INTO bible_chapters (book_id, chapter_number, verse_count) VALUES ($1, $2, $3)
			 ON CONFLICT (book_id, chapter_number) DO UPDATE SET verse_count = EXCLUDED.verse_count
			 RETURNING id`,
			ci.bookID, ci.chNum, ci.verseCount,
		)
	}
	cbr := database.Pool.SendBatch(ctx, chBatch)
	for _, ci := range chInserts {
		var chapterID int
		if err := cbr.QueryRow().Scan(&chapterID); err != nil {
			slog.Error("Failed to insert chapter", "globalCh", ci.globalChNum, slog.Any("error", err))
			os.Exit(1)
		}
		globalChToChapterID[ci.globalChNum] = chapterID
	}
	cbr.Close()
	fmt.Printf("   Inserted %d chapters\n", len(chapterRows))

	// 8. Build full verse records (resolve chapter IDs, attach footnotes)
	fmt.Println("🔗 Resolving verse data...")
	type rawVerse struct {
		globalChNum int
		verseNum    int
		content     string
	}
	var resolved []verseData
	for _, rv := range allVerses {
		chID, ok := globalChToChapterID[rv.globalChNum]
		if !ok {
			slog.Warn("Chapter ID not found", "globalChNum", rv.globalChNum)
			continue
		}
		key := fmt.Sprintf("%d:%d", rv.globalChNum, rv.verseNum)
		resolved = append(resolved, verseData{
			chapterID:     chID,
			verseNum:      rv.verseNum,
			content:       rv.content,
			theoFootnotes: footnoteMap[key],
		})
	}

	// 9. Batch-insert verses + verse_text — 2 round-trips per batch of 500
	fmt.Printf("📤 Inserting %d verses in batches of %d...\n", len(resolved), batchSize)
	totalInserted := 0
	totalErrors := 0

	for start := 0; start < len(resolved); start += batchSize {
		end := start + batchSize
		if end > len(resolved) {
			end = len(resolved)
		}
		chunk := resolved[start:end]

		// Step A: insert bible_verses, collect returned IDs
		verseBatch := &pgx.Batch{}
		for _, v := range chunk {
			verseBatch.Queue(
				`INSERT INTO bible_verses (chapter_id, verse_number) VALUES ($1, $2)
				 ON CONFLICT (chapter_id, verse_number) DO UPDATE SET verse_number = EXCLUDED.verse_number
				 RETURNING id`,
				v.chapterID, v.verseNum,
			)
		}
		br := database.Pool.SendBatch(ctx, verseBatch)
		verseIDs := make([]int, len(chunk))
		batchOK := true
		for i := range chunk {
			if err := br.QueryRow().Scan(&verseIDs[i]); err != nil {
				slog.Error("Verse batch insert failed", "idx", start+i, slog.Any("error", err))
				batchOK = false
				totalErrors++
			}
		}
		br.Close()
		if !batchOK {
			continue
		}

		// Step B: insert bible_verse_text
		textBatch := &pgx.Batch{}
		for i, v := range chunk {
			var fn interface{} = nil
			if len(v.theoFootnotes) > 0 {
				fn = v.theoFootnotes
			}
			textBatch.Queue(
				`INSERT INTO bible_verse_text (verse_id, lang_id, content, theo_footnotes) VALUES ($1, $2, $3, $4)
				 ON CONFLICT (verse_id, lang_id) DO UPDATE SET content = EXCLUDED.content, theo_footnotes = EXCLUDED.theo_footnotes`,
				verseIDs[i], enLangID, v.content, fn,
			)
		}
		br2 := database.Pool.SendBatch(ctx, textBatch)
		for i := range chunk {
			if _, err := br2.Exec(); err != nil {
				slog.Error("Verse text batch insert failed", "idx", start+i, slog.Any("error", err))
				totalErrors++
			}
		}
		br2.Close()

		totalInserted += len(chunk)
		fmt.Printf("\r   Progress: %d / %d verses", totalInserted, len(resolved))
	}

	fmt.Printf("\r✅ Bible seed complete: %d verses inserted, %d errors\n", totalInserted, totalErrors)
}

// rawVerseRecord is used during CSV parsing before chapter IDs are resolved.
type rawVerseRecord struct {
	globalChNum int
	verseNum    int
	content     string
}

func parseTextCSV(path string) ([]rawVerseRecord, map[int][]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	if _, err := reader.Read(); err != nil {
		return nil, nil, fmt.Errorf("read header: %w", err)
	}

	chToVerses := make(map[int][]int)
	var records []rawVerseRecord
	for {
		record, err := reader.Read()
		if err != nil {
			break
		}
		// columns: verse_index, verse_id, english, chapter_number, verse_number
		globalChNum, _ := strconv.Atoi(record[3])
		verseNum, _ := strconv.Atoi(record[4])
		records = append(records, rawVerseRecord{
			globalChNum: globalChNum,
			verseNum:    verseNum,
			content:     record[2],
		})
		chToVerses[globalChNum] = append(chToVerses[globalChNum], verseNum)
	}
	return records, chToVerses, nil
}

func parseChaptersCSV(path string) ([]chapterCSVRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	if _, err := reader.Read(); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	var rows []chapterCSVRow
	for {
		record, err := reader.Read()
		if err != nil {
			break
		}
		// columns: chapter_number, chapter_verses, revelation_order, title_english
		globalChNum, _ := strconv.Atoi(record[0])
		verseCount, _ := strconv.Atoi(record[1])
		bookName := record[3]
		rows = append(rows, chapterCSVRow{
			GlobalChNum: globalChNum,
			VerseCount:  verseCount,
			BookName:    bookName,
		})
	}
	return rows, nil
}

// parseFootnotesCSV builds a map from "globalChNum:verseNum" → []footnote texts.
// Multiple footnotes for the same verse are preserved as separate slice entries.
func parseFootnotesCSV(path string) (map[string][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	if _, err := reader.Read(); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	result := make(map[string][]string)
	for {
		record, err := reader.Read()
		if err != nil {
			break
		}
		// columns: verse_index, verse_id, english, chapter_number, verse_number
		key := record[3] + ":" + record[4]
		result[key] = append(result[key], record[2])
	}
	return result, nil
}
