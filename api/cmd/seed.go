package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/wikiSubmission/ws-backend/db"
)

const batchSize = 500

func main() {
	ctx := context.Background()
	database, err := db.DbSetup()
	if err != nil {
		slog.Error("Database setup failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer database.Close()

	// 1. Map Languages
	langMap := make(map[string]int16)
	rows, _ := database.Pool.Query(ctx, "SELECT id, code FROM languages")
	for rows.Next() {
		var id int16
		var code string
		rows.Scan(&id, &code)
		langMap[code] = id
	}
	rows.Close()

	// -------------------------------------------------------------------------
	// PHASE 1: Read all source data into memory (1 query per table)
	// -------------------------------------------------------------------------

	// Chapters
	fmt.Println("📖 Reading source chapters...")
	type chapterSrc struct {
		chNum, chVerses, revOrder int
		en, ar, fr, tr, de, id, fa, ta, sv, ru, bn, ur, es, tl *string
	}
	cRows, err := database.Pool.Query(ctx, `
		SELECT chapter_number, chapter_verses, revelation_order,
		       title_english, title_arabic, title_french, title_turkish, title_german,
		       title_bahasa, title_persian, title_tamil, title_swedish, title_russian,
		       title_bengali, title_urdu, title_spanish, title_transliterated
		FROM public.ws_quran_chapters
		ORDER BY chapter_number`)
	if err != nil {
		slog.Error("Failed to query source chapters", slog.Any("error", err))
		os.Exit(1)
	}
	var chapters []chapterSrc
	for cRows.Next() {
		var c chapterSrc
		cRows.Scan(&c.chNum, &c.chVerses, &c.revOrder,
			&c.en, &c.ar, &c.fr, &c.tr, &c.de, &c.id, &c.fa, &c.ta, &c.sv, &c.ru, &c.bn, &c.ur, &c.es, &c.tl)
		chapters = append(chapters, c)
	}
	cRows.Close()
	fmt.Printf("   Found %d chapters\n", len(chapters))

	// Verses + translations + subtitles + footnotes
	fmt.Println("📖 Reading source verses (this may take a moment)...")
	type verseSrc struct {
		chNum, vNum int
		vID         string
		// content
		tEn, tAr, tFr, tTr, tDe, tId, tFa, tTa, tSv, tRu, tBn, tUr, tEs, tTl, tAc *string
		// subtitles
		sEn, sFr, sTr, sDe, sId, sFa, sTa, sSv, sRu, sBn, sUr, sEs *string
		// footers
		fEn, fFr, fTr, fDe, fId, fFa, fTa, fSv, fRu, fBn, fUr, fEs *string
	}
	vRows, err := database.Pool.Query(ctx, `
		SELECT i.chapter_number, i.verse_number, i.verse_id,
		       t.english, t.arabic, t.french, t.turkish, t.german, t.bahasa, t.persian, t.tamil, t.swedish, t.russian, t.bengali, t.urdu, t.spanish, t.transliterated, t.arabic_clean,
		       s.english, s.french, s.turkish, s.german, s.bahasa, s.persian, s.tamil, s.swedish, s.russian, s.bengali, s.urdu, s.spanish,
		       f.english, f.french, f.turkish, f.german, f.bahasa, f.persian, f.tamil, f.swedish, f.russian, f.bengali, f.urdu, f.spanish
		FROM public.ws_quran_index i
		JOIN public.ws_quran_text t ON i.verse_id = t.verse_id
		LEFT JOIN public.ws_quran_subtitles s ON i.chapter_number = s.chapter_number AND i.verse_number = s.verse_number
		LEFT JOIN public.ws_quran_footnotes f ON i.chapter_number = f.chapter_number AND i.verse_number = f.verse_number
		ORDER BY i.chapter_number, i.verse_number`)
	if err != nil {
		slog.Error("Failed to query source verses", slog.Any("error", err))
		os.Exit(1)
	}
	var verses []verseSrc
	for vRows.Next() {
		var v verseSrc
		if err := vRows.Scan(&v.chNum, &v.vNum, &v.vID,
			&v.tEn, &v.tAr, &v.tFr, &v.tTr, &v.tDe, &v.tId, &v.tFa, &v.tTa, &v.tSv, &v.tRu, &v.tBn, &v.tUr, &v.tEs, &v.tTl, &v.tAc,
			&v.sEn, &v.sFr, &v.sTr, &v.sDe, &v.sId, &v.sFa, &v.sTa, &v.sSv, &v.sRu, &v.sBn, &v.sUr, &v.sEs,
			&v.fEn, &v.fFr, &v.fTr, &v.fDe, &v.fId, &v.fFa, &v.fTa, &v.fSv, &v.fRu, &v.fBn, &v.fUr, &v.fEs); err != nil {
			slog.Error("Verse scan failed", slog.Any("error", err))
			os.Exit(1)
		}
		verses = append(verses, v)
	}
	if err := vRows.Err(); err != nil {
		slog.Error("Verse query failed", slog.Any("error", err))
		os.Exit(1)
	}
	vRows.Close()
	fmt.Printf("   Found %d verses\n", len(verses))

	// Words (single bulk read)
	fmt.Println("📖 Reading source words...")
	type wordSrc struct {
		vID                            string
		gIdx, wIdx                     int
		root, ar, en, tl, mean *string
	}
	wRows, err := database.Pool.Query(ctx,
		`SELECT verse_id, index, word_index, root_word, arabic, english, transliterated, meanings
		 FROM public.ws_quran_word_by_word
		 ORDER BY verse_id, word_index`)
	if err != nil {
		slog.Error("Failed to query source words", slog.Any("error", err))
		os.Exit(1)
	}
	var allWords []wordSrc
	for wRows.Next() {
		var w wordSrc
		if err := wRows.Scan(&w.vID, &w.gIdx, &w.wIdx, &w.root, &w.ar, &w.en, &w.tl, &w.mean); err != nil {
			slog.Error("Word scan failed", slog.Any("error", err))
			os.Exit(1)
		}
		allWords = append(allWords, w)
	}
	if err := wRows.Err(); err != nil {
		slog.Error("Words query failed", slog.Any("error", err))
		os.Exit(1)
	}
	wRows.Close()
	fmt.Printf("   Found %d words\n", len(allWords))

	// -------------------------------------------------------------------------
	// PHASE 2: Batch insert everything
	// -------------------------------------------------------------------------
	val := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}

	// --- Chapters ---
	fmt.Println("📑 Inserting chapters...")
	chapterIDMap := make(map[int]int) // chNum → new id
	{
		b := &pgx.Batch{}
		for _, c := range chapters {
			b.Queue(
				`INSERT INTO chapters (chapter_number, verse_count, revelation_order)
				 VALUES ($1, $2, $3)
				 ON CONFLICT (chapter_number) DO UPDATE SET verse_count = EXCLUDED.verse_count, revelation_order = EXCLUDED.revelation_order
				 RETURNING id`,
				c.chNum, c.chVerses, c.revOrder)
		}
		br := database.Pool.SendBatch(ctx, b)
		for _, c := range chapters {
			var id int
			if err := br.QueryRow().Scan(&id); err != nil {
				slog.Error("Chapter insert failed", "chNum", c.chNum, slog.Any("error", err))
				os.Exit(1)
			}
			chapterIDMap[c.chNum] = id
		}
		br.Close()
		fmt.Printf("   Inserted %d chapters\n", len(chapters))
	}

	// --- Chapter text ---
	fmt.Println("📑 Inserting chapter_text...")
	type chTextRow struct {
		chID   int
		langID int16
		title  string
	}
	var chTexts []chTextRow
	for _, c := range chapters {
		chID := chapterIDMap[c.chNum]
		for _, t := range []struct {
			code  string
			title *string
		}{
			{"en", c.en}, {"ar", c.ar}, {"fr", c.fr}, {"tr", c.tr}, {"de", c.de},
			{"id", c.id}, {"fa", c.fa}, {"ta", c.ta}, {"sv", c.sv}, {"ru", c.ru},
			{"bn", c.bn}, {"ur", c.ur}, {"es", c.es}, {"tl", c.tl},
		} {
			if t.title != nil && langMap[t.code] != 0 {
				chTexts = append(chTexts, chTextRow{chID, langMap[t.code], *t.title})
			}
		}
	}
	sendBatchExec(ctx, database, chTexts, func(b *pgx.Batch, row chTextRow) {
		b.Queue(
			`INSERT INTO chapter_text (chapter_id, lang_id, title) VALUES ($1, $2, $3)
			 ON CONFLICT DO NOTHING`,
			row.chID, row.langID, row.title)
	})
	fmt.Printf("   Inserted %d chapter_text rows\n", len(chTexts))

	// --- Verses ---
	fmt.Printf("📤 Inserting %d verses...\n", len(verses))
	verseIDMap := make(map[string]int) // "chNum:vNum" → new id
	{
		for start := 0; start < len(verses); start += batchSize {
			end := start + batchSize
			if end > len(verses) {
				end = len(verses)
			}
			chunk := verses[start:end]

			b := &pgx.Batch{}
			for _, v := range chunk {
				b.Queue(
					`INSERT INTO verses (chapter_id, verse_index)
					 VALUES ($1, $2)
					 ON CONFLICT (chapter_id, verse_index) DO UPDATE SET verse_index = EXCLUDED.verse_index
					 RETURNING id`,
					chapterIDMap[v.chNum], v.vNum)
			}
			br := database.Pool.SendBatch(ctx, b)
			for _, v := range chunk {
				var id int
				if err := br.QueryRow().Scan(&id); err != nil {
					slog.Error("Verse insert failed", "vID", v.vID, slog.Any("error", err))
					os.Exit(1)
				}
				verseIDMap[v.vID] = id
			}
			br.Close()
			fmt.Printf("\r   Progress: %d / %d", end, len(verses))
		}
		fmt.Println()
	}

	// --- Verse text ---
	fmt.Println("📤 Inserting verse_text...")
	type verseTextRow struct {
		verseID int
		langID  int16
		content, subtitle, footer string
	}
	var verseTexts []verseTextRow
	for _, v := range verses {
		newID := verseIDMap[v.vID]
		if newID == 0 {
			continue
		}
		for _, p := range []struct {
			code          string
			content, sub, foot *string
		}{
			{"en", v.tEn, v.sEn, v.fEn}, {"ar", v.tAr, nil, nil}, {"fr", v.tFr, v.sFr, v.fFr},
			{"tr", v.tTr, v.sTr, v.fTr}, {"de", v.tDe, v.sDe, v.fDe}, {"id", v.tId, v.sId, v.fId},
			{"fa", v.tFa, v.sFa, v.fFa}, {"ta", v.tTa, v.sTa, v.fTa}, {"sv", v.tSv, v.sSv, v.fSv},
			{"ru", v.tRu, v.sRu, v.fRu}, {"bn", v.tBn, v.sBn, v.fBn}, {"ur", v.tUr, v.sUr, v.fUr},
			{"es", v.tEs, v.sEs, v.fEs}, {"tl", v.tTl, nil, nil}, {"ac", v.tAc, nil, nil},
		} {
			langID := langMap[p.code]
			if langID == 0 {
				continue
			}
			c, s, f := val(p.content), val(p.sub), val(p.foot)
			if c == "" && s == "" && f == "" {
				continue
			}
			verseTexts = append(verseTexts, verseTextRow{newID, langID, c, s, f})
		}
	}
	sendBatchExec(ctx, database, verseTexts, func(b *pgx.Batch, row verseTextRow) {
		b.Queue(
			`INSERT INTO verse_text (verse_id, lang_id, content, subtitle, footer)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (verse_id, lang_id) DO UPDATE SET content = EXCLUDED.content, subtitle = EXCLUDED.subtitle, footer = EXCLUDED.footer`,
			row.verseID, row.langID, row.content, row.subtitle, row.footer)
	})
	fmt.Printf("   Inserted %d verse_text rows\n", len(verseTexts))

	// --- Words ---
	fmt.Printf("📤 Inserting %d words...\n", len(allWords))
	type wordRow struct {
		verseID, gIdx, wIdx int
	}
	type wordMeta struct {
		ar, en, tl, root, mean string
	}
	var wordRows []wordRow
	var wordMetas []wordMeta
	for _, w := range allWords {
		vID := verseIDMap[w.vID]
		if vID == 0 {
			// Try building the vID key from "chNum:vNum" stored format
			// ws_quran_word_by_word.verse_id uses "chapter_number:verse_number" format
			vID = verseIDMap[w.vID]
			if vID == 0 {
				continue
			}
		}
		wordRows = append(wordRows, wordRow{vID, w.gIdx, w.wIdx})
		wordMetas = append(wordMetas, wordMeta{val(w.ar), val(w.en), val(w.tl), val(w.root), val(w.mean)})
	}

	wordIDMap := make([]int, len(wordRows)) // parallel slice: wordRows[i] → new word id
	for start := 0; start < len(wordRows); start += batchSize {
		end := start + batchSize
		if end > len(wordRows) {
			end = len(wordRows)
		}
		chunk := wordRows[start:end]

		b := &pgx.Batch{}
		for _, w := range chunk {
			b.Queue(
				`INSERT INTO words (verse_id, global_index, word_index)
				 VALUES ($1, $2, $3)
				 ON CONFLICT (verse_id, word_index) DO UPDATE SET global_index = EXCLUDED.global_index
				 RETURNING id`,
				w.verseID, w.gIdx, w.wIdx)
		}
		br := database.Pool.SendBatch(ctx, b)
		for i := range chunk {
			var id int
			if err := br.QueryRow().Scan(&id); err != nil {
				slog.Warn("Word insert failed", "idx", start+i, slog.Any("error", err))
			}
			wordIDMap[start+i] = id
		}
		br.Close()
		fmt.Printf("\r   Progress: %d / %d", end, len(wordRows))
	}
	fmt.Println()

	// --- Word text ---
	fmt.Println("📤 Inserting word_text...")
	type wordTextRow struct {
		wordID int
		langID int16
		word, root, meaning string
	}
	var wordTexts []wordTextRow
	for i, wid := range wordIDMap {
		if wid == 0 {
			continue
		}
		m := wordMetas[i]
		// Arabic
		if m.ar != "" && langMap["ar"] != 0 {
			wordTexts = append(wordTexts, wordTextRow{wid, langMap["ar"], m.ar, m.root, ""})
		}
		// English + meaning
		if (m.en != "" || m.mean != "") && langMap["en"] != 0 {
			wordTexts = append(wordTexts, wordTextRow{wid, langMap["en"], m.en, m.root, m.mean})
		}
		// Transliteration
		if m.tl != "" && langMap["tl"] != 0 {
			wordTexts = append(wordTexts, wordTextRow{wid, langMap["tl"], m.tl, m.root, ""})
		}
	}
	sendBatchExec(ctx, database, wordTexts, func(b *pgx.Batch, row wordTextRow) {
		b.Queue(
			`INSERT INTO word_text (word_id, lang_id, word, word_root, meaning)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (word_id, lang_id) DO UPDATE SET word = EXCLUDED.word, word_root = EXCLUDED.word_root, meaning = EXCLUDED.meaning`,
			row.wordID, row.langID, row.word, row.root, row.meaning)
	})
	fmt.Printf("   Inserted %d word_text rows\n", len(wordTexts))

	fmt.Println("\n✅ Seeding complete. Clustering...")
	database.Pool.Exec(ctx, "CLUSTER verses USING idx_verses_nav")
	fmt.Println("✅ Done.")
}

// sendBatchExec sends a slice of items as batched Exec queries (no return values needed).
func sendBatchExec[T any](ctx context.Context, database *db.DB, items []T, queue func(*pgx.Batch, T)) {
	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}
		b := &pgx.Batch{}
		for _, item := range items[start:end] {
			queue(b, item)
		}
		br := database.Pool.SendBatch(ctx, b)
		for range items[start:end] {
			if _, err := br.Exec(); err != nil {
				// Log but don't abort — ON CONFLICT DO NOTHING means duplicates are fine
				if !strings.Contains(err.Error(), "duplicate") {
					slog.Warn("Batch exec error", slog.Any("error", err))
				}
			}
		}
		br.Close()
	}
}
