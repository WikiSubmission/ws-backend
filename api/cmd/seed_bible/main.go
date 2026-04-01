package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"strconv"

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

	// 3. Parse footnotes CSV into map: "globalChNum:verseNum" → footnoteText
	fmt.Println("📝 Parsing footnotes CSV...")
	footnoteMap, err := parseFootnotesCSV("assets/bible/ws_bible_footnotes_rows.csv")
	if err != nil {
		slog.Error("Failed to parse footnotes CSV", slog.Any("error", err))
		os.Exit(1)
	}
	fmt.Printf("   Found %d footnote entries\n", len(footnoteMap))

	// 4. Determine unique books (ordered by first appearance)
	bookOrder := []string{}
	seenBooks := map[string]bool{}
	for _, row := range chapterRows {
		if !seenBooks[row.BookName] {
			seenBooks[row.BookName] = true
			bookOrder = append(bookOrder, row.BookName)
		}
	}

	// 5. Insert books
	fmt.Println("📚 Inserting books...")
	bookNameToID := make(map[string]int16)
	for i, bookName := range bookOrder {
		bookNum := i + 1
		testament := "OT"
		if newTestamentBooks[bookName] {
			testament = "NT"
		}
		var bookID int16
		err := database.Pool.QueryRow(ctx,
			`INSERT INTO bible_books (book_number, name, testament) VALUES ($1, $2, $3)
			 ON CONFLICT (book_number) DO UPDATE SET name = EXCLUDED.name, testament = EXCLUDED.testament
			 RETURNING id`,
			bookNum, bookName, testament,
		).Scan(&bookID)
		if err != nil {
			slog.Error("Failed to insert book", "book", bookName, slog.Any("error", err))
			os.Exit(1)
		}
		bookNameToID[bookName] = bookID
	}
	fmt.Printf("   Inserted %d books\n", len(bookOrder))

	// 6. Insert chapters, track global chapter → chapter DB id
	fmt.Println("📑 Inserting chapters...")
	globalChToChapterID := make(map[int]int)
	chapterInBook := make(map[string]int) // running chapter-within-book counter per book

	for _, row := range chapterRows {
		bookID := bookNameToID[row.BookName]
		chapterInBook[row.BookName]++
		chNum := chapterInBook[row.BookName]

		var chapterID int
		err := database.Pool.QueryRow(ctx,
			`INSERT INTO bible_chapters (book_id, chapter_number, verse_count) VALUES ($1, $2, $3)
			 ON CONFLICT (book_id, chapter_number) DO UPDATE SET verse_count = EXCLUDED.verse_count
			 RETURNING id`,
			bookID, chNum, row.VerseCount,
		).Scan(&chapterID)
		if err != nil {
			slog.Error("Failed to insert chapter", "globalCh", row.GlobalChNum, slog.Any("error", err))
			os.Exit(1)
		}
		globalChToChapterID[row.GlobalChNum] = chapterID
	}
	fmt.Printf("   Inserted %d chapters\n", len(chapterRows))

	// 7. Parse text CSV and insert verses + verse_text
	fmt.Println("📜 Inserting verses and verse text...")
	textFile, err := os.Open("assets/bible/ws_bible_text_rows.csv")
	if err != nil {
		slog.Error("Failed to open text CSV", slog.Any("error", err))
		os.Exit(1)
	}
	defer textFile.Close()

	reader := csv.NewReader(textFile)
	if _, err := reader.Read(); err != nil { // skip header
		slog.Error("Failed to read text CSV header", slog.Any("error", err))
		os.Exit(1)
	}

	count := 0
	errors := 0
	for {
		record, err := reader.Read()
		if err != nil {
			break // EOF
		}
		// columns: verse_index, verse_id, english, chapter_number, verse_number
		globalChNum, _ := strconv.Atoi(record[3])
		verseNum, _ := strconv.Atoi(record[4])
		content := record[2]

		chapterID, exists := globalChToChapterID[globalChNum]
		if !exists {
			slog.Warn("Chapter ID not found for global chapter", "globalChNum", globalChNum)
			errors++
			continue
		}

		var verseID int
		err = database.Pool.QueryRow(ctx,
			`INSERT INTO bible_verses (chapter_id, verse_number) VALUES ($1, $2)
			 ON CONFLICT (chapter_id, verse_number) DO UPDATE SET verse_number = EXCLUDED.verse_number
			 RETURNING id`,
			chapterID, verseNum,
		).Scan(&verseID)
		if err != nil {
			slog.Error("Failed to insert verse", "ch", globalChNum, "v", verseNum, slog.Any("error", err))
			errors++
			continue
		}

		// Look up footnote for this verse
		footnoteKey := fmt.Sprintf("%d:%d", globalChNum, verseNum)
		footnote := footnoteMap[footnoteKey]
		var footnotePtr *string
		if footnote != "" {
			footnotePtr = &footnote
		}

		_, err = database.Pool.Exec(ctx,
			`INSERT INTO bible_verse_text (verse_id, lang_id, content, footer) VALUES ($1, $2, $3, $4)
			 ON CONFLICT (verse_id, lang_id) DO UPDATE SET content = EXCLUDED.content, footer = EXCLUDED.footer`,
			verseID, enLangID, content, footnotePtr,
		)
		if err != nil {
			slog.Error("Failed to insert verse_text", "ch", globalChNum, "v", verseNum, slog.Any("error", err))
			errors++
			continue
		}

		count++
		if count%1000 == 0 {
			fmt.Printf("\r   Progress: %d verses...", count)
		}
	}

	fmt.Printf("\r✅ Bible seed complete: %d verses inserted, %d errors\n", count, errors)
}

func parseChaptersCSV(path string) ([]chapterCSVRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	if _, err := reader.Read(); err != nil { // skip header
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

// parseFootnotesCSV builds a map from "globalChNum:verseNum" → footnote text.
// Multiple footnotes for the same verse are concatenated with a space.
func parseFootnotesCSV(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	if _, err := reader.Read(); err != nil { // skip header
		return nil, fmt.Errorf("read header: %w", err)
	}

	result := make(map[string]string)
	for {
		record, err := reader.Read()
		if err != nil {
			break
		}
		// columns: verse_index, verse_id, english, chapter_number, verse_number
		chNum := record[3]
		verseNum := record[4]
		footnote := record[2]
		key := chNum + ":" + verseNum

		if existing, ok := result[key]; ok {
			result[key] = existing + " " + footnote
		} else {
			result[key] = footnote
		}
	}
	return result, nil
}
