// Command build-offline-bundles renders the offline content bundles (one
// SQLite file per scripture+language) plus the manifest, reading from the same
// Postgres the API uses. For Phase 1 it targets the Quran (ar, en by default)
// and writes to a local output directory; S3/CloudFront upload is deferred.
//
// Usage:
//
//	go run ./cmd/build-offline-bundles -out offline-bundles -langs ar,en -data-version 1
//
// Environment: same DATABASE_* / DATABASE_URL vars as the API (loaded from .env
// when present), since it connects via db.ConnectDB.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/wikiSubmission/ws-backend/db"
	"github.com/wikiSubmission/ws-backend/offline/bundle"
)

func main() {
	out := flag.String("out", "offline-bundles", "output directory for bundle files and manifest")
	langsCSV := flag.String("langs", "ar,en", "comma-separated Quran language codes to build")
	baseURL := flag.String("base-url", "https://cdn.wikisubmission.org/offline", "base URL the manifest points bundle downloads at")
	dataVersion := flag.Int("data-version", 1, "data_version stamped into every bundle and its versioned filename")
	flag.Parse()

	if err := godotenv.Load(); err != nil {
		fmt.Println("info: no .env file found, using system environment variables")
	}

	if err := run(*out, splitCSV(*langsCSV), strings.TrimRight(*baseURL, "/"), *dataVersion); err != nil {
		log.Fatalf("build-offline-bundles: %v", err)
	}
}

func run(outDir string, langs []string, baseURL string, dataVersion int) error {
	if len(langs) == 0 {
		return fmt.Errorf("no languages requested")
	}

	database, err := db.ConnectDB()
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer database.Close()

	ctx := context.Background()
	builtAt := time.Now().UTC().Format(time.RFC3339)

	langRows, err := database.FetchAllLanguages(ctx)
	if err != nil {
		return fmt.Errorf("fetch languages: %w", err)
	}
	langByCode := make(map[string]db.LanguageRow, len(langRows))
	for _, l := range langRows {
		langByCode[l.Code] = l
	}

	available, err := database.FetchQuranLangs(ctx)
	if err != nil {
		return fmt.Errorf("fetch quran langs: %w", err)
	}

	var manifest bundle.Manifest
	for _, code := range langs {
		lang, ok := langByCode[code]
		if !ok {
			return fmt.Errorf("language %q not found in languages table", code)
		}
		if !available[code] {
			return fmt.Errorf("language %q has no verse_text rows", code)
		}

		entry, err := buildQuranBundle(ctx, database, lang, outDir, baseURL, dataVersion, builtAt)
		if err != nil {
			return fmt.Errorf("build quran-%s: %w", code, err)
		}
		manifest.Bundles = append(manifest.Bundles, entry)
		log.Printf("built %s: %d bytes, sha256=%s", entry.ID, entry.Bytes, entry.SHA256[:12])
	}

	manifestPath := filepath.Join(outDir, "manifest.json")
	if err := bundle.WriteManifest(manifestPath, manifest); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	log.Printf("wrote manifest with %d bundle(s) to %s", len(manifest.Bundles), manifestPath)
	return nil
}

func buildQuranBundle(
	ctx context.Context,
	database *db.DB,
	lang db.LanguageRow,
	outDir, baseURL string,
	dataVersion int,
	builtAt string,
) (bundle.ManifestEntry, error) {
	chapterRows, err := database.FetchAllChapters(ctx, lang.ID)
	if err != nil {
		return bundle.ManifestEntry{}, fmt.Errorf("fetch chapters: %w", err)
	}
	chapters := make([]bundle.Chapter, 0, len(chapterRows))
	for _, c := range chapterRows {
		chapters = append(chapters, bundle.Chapter{
			CN:         c.ChapterNumber,
			Title:      c.Title,
			VerseCount: c.VerseCount,
		})
	}

	// (1,1)..(114,300) spans every verse; 300 safely exceeds the longest chapter.
	verseRows, err := database.FetchVersesWithTranslations(ctx, 1, 114, 1, 300, []string{lang.Code})
	if err != nil {
		return bundle.ManifestEntry{}, fmt.Errorf("fetch verses: %w", err)
	}
	verses := make([]bundle.Verse, 0, len(verseRows))
	for _, r := range verseRows {
		verses = append(verses, bundle.Verse{
			VK:       fmt.Sprintf("%d:%d", r.ChapterNumber, r.VerseIndex),
			CN:       r.ChapterNumber,
			VN:       r.VerseIndex,
			Text:     r.VerseContent,
			Subtitle: deref(r.VerseSubtitle),
			Footnote: deref(r.VerseFooter),
		})
	}
	if len(verses) == 0 {
		return bundle.ManifestEntry{}, fmt.Errorf("no verses returned for %q", lang.Code)
	}

	tokenizer := bundle.TokenizerLatin
	if lang.Code == "ar" {
		tokenizer = bundle.TokenizerTrigram
	}
	meta := bundle.Meta{
		Scripture:   "quran",
		Lang:        lang.Code,
		DataVersion: dataVersion,
		Tokenizer:   tokenizer,
		BuiltAt:     builtAt,
	}

	id := "quran-" + lang.Code
	filename := fmt.Sprintf("%s-v%d.db", id, dataVersion)
	path := filepath.Join(outDir, filename)
	if err := bundle.Write(path, meta, chapters, verses); err != nil {
		return bundle.ManifestEntry{}, err
	}

	url := baseURL + "/" + filename
	return bundle.NewManifestEntry(path, id, url, meta)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
