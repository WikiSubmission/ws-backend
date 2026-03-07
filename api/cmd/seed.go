package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/wikiSubmission/ws-backend/db"
)

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

	// 2. Migrate Chapters
	fmt.Println("🚀 Migrating Chapters...")
	chapterIDMap := make(map[int]int)
	cRows, _ := database.Pool.Query(ctx, `
		SELECT chapter_number, chapter_verses, revelation_order, 
		title_english, title_arabic, title_french, title_turkish, title_german,
		title_bahasa, title_persian, title_tamil, title_swedish, title_russian,
		title_bengali, title_urdu, title_spanish, title_transliterated
		FROM public.ws_quran_chapters`)

	for cRows.Next() {
		var chNum, chVerses, revOrder int
		var en, ar, fr, tr, de, id, fa, ta, sv, ru, bn, ur, es, tl *string
		cRows.Scan(&chNum, &chVerses, &revOrder, &en, &ar, &fr, &tr, &de, &id, &fa, &ta, &sv, &ru, &bn, &ur, &es, &tl)

		var newChID int
		database.Pool.QueryRow(ctx, "INSERT INTO chapters (chapter_number, verse_count, revelation_order) VALUES ($1, $2, $3) RETURNING id",
			chNum, chVerses, revOrder).Scan(&newChID)
		chapterIDMap[chNum] = newChID

		titles := map[string]*string{
			"en": en, "ar": ar, "fr": fr, "tr": tr, "de": de,
			"id": id, "fa": fa, "ta": ta, "sv": sv, "ru": ru,
			"bn": bn, "ur": ur, "es": es, "tl": tl,
		}

		for code, title := range titles {
			if title != nil && langMap[code] != 0 {
				database.Pool.Exec(ctx, "INSERT INTO chapter_text (chapter_id, lang_id, title) VALUES ($1, $2, $3)", newChID, langMap[code], *title)
				database.Pool.Exec(ctx, "INSERT INTO chapter_text_drafts (chapter_id, lang_id, title, status) VALUES ($1, $2, $3, 'published')", newChID, langMap[code], *title)
			}
		}
	}
	cRows.Close()

	// 3. Migrate Verses
	var totalVerses int
	database.Pool.QueryRow(ctx, "SELECT count(*) FROM public.ws_quran_index").Scan(&totalVerses)
	fmt.Printf("📖 Migrating %d Verses...\n", totalVerses)

	vRows, err := database.Pool.Query(ctx, `
        SELECT 
            i.chapter_number, 
            i.verse_number, 
            i.verse_id, 
            t.english, t.arabic, t.french, t.turkish, t.german, t.bahasa, t.persian, t.tamil, t.swedish, t.russian, t.bengali, t.urdu, t.spanish, t.transliterated, t.arabic_clean,
            s.english, s.french, s.turkish, s.german, s.bahasa, s.persian, s.tamil, s.swedish, s.russian, s.bengali, s.urdu, s.spanish,
            -- ADD THESE COLUMNS TO MATCH YOUR 12 FOOTER SCAN VARIABLES
            f.english, f.french, f.turkish, f.german, f.bahasa, f.persian, f.tamil, f.swedish, f.russian, f.bengali, f.urdu, f.spanish
        FROM public.ws_quran_index i
        JOIN public.ws_quran_text t ON i.verse_id = t.verse_id
        LEFT JOIN public.ws_quran_subtitles s ON i.chapter_number = s.chapter_number AND i.verse_number = s.verse_number
        LEFT JOIN public.ws_quran_footnotes f ON i.chapter_number = f.chapter_number AND i.verse_number = f.verse_number
    `)
	if err != nil {
		slog.Error("Query failed", slog.Any("error", err))
		return
	}
	defer vRows.Close()

	count := 0
	for vRows.Next() {
		var chNum, vNum int
		var vID string
		// Content pointers
		var tEn, tAr, tFr, tTr, tDe, tId, tFa, tTa, tSv, tRu, tBn, tUr, tEs, tTl, tAc *string
		// Subtitle pointers
		var sEn, sFr, sTr, sDe, sId, sFa, sTa, sSv, sRu, sBn, sUr, sEs *string
		// Footer pointer
		var fEn, fFr, fTr, fDe, fId, fFa, fTa, fSv, fRu, fBn, fUr, fEs *string

		err := vRows.Scan(&chNum, &vNum, &vID,
			&tEn, &tAr, &tFr, &tTr, &tDe, &tId, &tFa, &tTa, &tSv, &tRu, &tBn, &tUr, &tEs, &tTl, &tAc,
			&sEn, &sFr, &sTr, &sDe, &sId, &sFa, &sTa, &sSv, &sRu, &sBn, &sUr, &sEs,
			&fEn, &fFr, &fTr, &fDe, &fId, &fFa, &fTa, &fSv, &fRu, &fBn, &fUr, &fEs)

		if err != nil {
			slog.Error("Scan failed", slog.Any("error", err))
			break // Stop and tell us why
		}

		var newVID int
		database.Pool.QueryRow(ctx, "INSERT INTO verses (chapter_id, verse_index) VALUES ($1, $2) RETURNING id", chapterIDMap[chNum], vNum).Scan(&newVID)

		val := func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		}

		// Pivot logic for 15+ variations
		langPivot := []struct {
			code               string
			content, sub, foot *string
		}{
			{"en", tEn, sEn, fEn}, {"ar", tAr, nil, nil}, {"fr", tFr, sFr, fFr},
			{"tr", tTr, sTr, fTr}, {"de", tDe, sDe, fDe}, {"id", tId, sId, fId},
			{"fa", tFa, sFa, fFa}, {"ta", tTa, sTa, fTa}, {"sv", tSv, sSv, fSv},
			{"ru", tRu, sRu, fRu}, {"bn", tBn, sBn, fBn}, {"ur", tUr, sUr, fUr},
			{"es", tEs, sEs, fEs}, {"tl", tTl, nil, nil}, {"ac", tAc, nil, nil},
		}

		for _, p := range langPivot {
			langID := langMap[p.code]
			if langID == 0 {
				continue
			}
			c, s, f := val(p.content), val(p.sub), val(p.foot)
			if c == "" && s == "" && f == "" {
				continue
			}

			database.Pool.Exec(ctx, "INSERT INTO verse_text (verse_id, lang_id, content, subtitle, footer) VALUES ($1, $2, $3, $4, $5)", newVID, langID, c, s, f)
			database.Pool.Exec(ctx, "INSERT INTO verse_text_drafts (verse_id, lang_id, content, subtitle, footer, status) VALUES ($1, $2, $3, $4, $5, 'published')", newVID, langID, c, s, f)
		}

		// 4. Migrate Words (Fixed Batch)
		wordBatch := &pgx.Batch{}
		type wMeta struct{ ar, en, root, mean string }
		var metas []wMeta

		wRows, err := database.Pool.Query(ctx, "SELECT index, word_index, root_word, arabic, english, meanings FROM public.ws_quran_word_by_word WHERE verse_id = $1", vID)
		if err == nil {
			for wRows.Next() {
				var gIdx, wIdx int
				var root, ar, en, mean *string
				wRows.Scan(&gIdx, &wIdx, &root, &ar, &en, &mean)

				metas = append(metas, wMeta{val(ar), val(en), val(root), val(mean)})
				wordBatch.Queue("INSERT INTO words (verse_id, global_index, word_index) VALUES ($1, $2, $3) RETURNING id", newVID, gIdx, wIdx)
			}
			wRows.Close()

			br := database.Pool.SendBatch(ctx, wordBatch)
			for i := 0; i < wordBatch.Len(); i++ {
				var newWordID int
				if err := br.QueryRow().Scan(&newWordID); err == nil {
					m := metas[i]
					// Arabic
					database.Pool.Exec(ctx, "INSERT INTO word_text (word_id, lang_id, word, word_root) VALUES ($1, $2, $3, $4)", newWordID, langMap["ar"], m.ar, m.root)
					database.Pool.Exec(ctx, "INSERT INTO word_text_drafts (word_id, lang_id, word, word_root, status) VALUES ($1, $2, $3, $4, 'published')", newWordID, langMap["ar"], m.ar, m.root)
					// English + Meaning
					database.Pool.Exec(ctx, "INSERT INTO word_text (word_id, lang_id, word, word_root, meaning) VALUES ($1, $2, $3, $4, $5)", newWordID, langMap["en"], m.en, m.root, m.mean)
					database.Pool.Exec(ctx, "INSERT INTO word_text_drafts (word_id, lang_id, word, word_root, meaning, status) VALUES ($1, $2, $3, $4, $5, 'published')", newWordID, langMap["en"], m.en, m.root, m.mean)
				}
			}
			br.Close()
		}

		count++
		if count%50 == 0 || count == totalVerses {
			fmt.Printf("\r✨ Progress: %.2f%% (%d/%d)", float64(count)/float64(totalVerses)*100, count, totalVerses)
		}
	}

	fmt.Println("\n✅ Seeding complete. Clustering...")
	database.Pool.Exec(ctx, "CLUSTER verses USING idx_verses_nav")
}
