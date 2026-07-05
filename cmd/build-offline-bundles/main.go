// Command build-offline-bundles renders the offline content bundles (one
// SQLite file per scripture+language) plus the manifest, reading from the same
// Postgres the API uses. It targets the Quran (ar, en text bundles by default)
// and additionally builds word-by-word bundles for the languages in
// -word-langs (en by default; each carries ar + target + tl word rows). It
// writes to a local output directory; with -upload it then publishes the
// bundles and manifest to the public CDN through ws-lib's store-public
// endpoint (see offline/publish), so they become fetchable at -base-url.
//
// Usage:
//
//	go run ./cmd/build-offline-bundles -out offline-bundles -langs ar,en -word-langs en -data-version 1
//	go run ./cmd/build-offline-bundles -langs ar,en -data-version 1 -upload
//
// Environment: same DATABASE_* / DATABASE_URL vars as the API (loaded from .env
// when present), since it connects via db.ConnectDB. -upload additionally
// requires WSLIB_BASE_URL and STORE_AUTH_SECRET (same pair as the data-export
// uploader in api/server.go).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/wikiSubmission/ws-backend/db"
	"github.com/wikiSubmission/ws-backend/offline/bundle"
	"github.com/wikiSubmission/ws-backend/offline/publish"
)

func main() {
	out := flag.String("out", "offline-bundles", "output directory for bundle files and manifest")
	langsCSV := flag.String("langs", "ar,en", "comma-separated Quran language codes to build")
	wordLangsCSV := flag.String("word-langs", "en", "comma-separated languages to build word-by-word bundles for (empty to skip)")
	baseURL := flag.String("base-url", "https://cdn.wikisubmission.org/offline", "base URL the manifest points bundle downloads at")
	dataVersion := flag.Int("data-version", 1, "data_version stamped into every bundle and its versioned filename")
	upload := flag.Bool("upload", false, "publish bundles and manifest to the CDN via ws-lib (requires WSLIB_BASE_URL and STORE_AUTH_SECRET)")
	flag.Parse()

	if err := godotenv.Load(); err != nil {
		fmt.Println("info: no .env file found, using system environment variables")
	}

	var store *publish.WSLibPublicStore
	if *upload {
		wslibURL := strings.TrimRight(os.Getenv("WSLIB_BASE_URL"), "/")
		secret := os.Getenv("STORE_AUTH_SECRET")
		if wslibURL == "" || secret == "" {
			log.Fatal("build-offline-bundles: -upload requires WSLIB_BASE_URL and STORE_AUTH_SECRET to be set")
		}
		store = &publish.WSLibPublicStore{
			BaseURL: wslibURL,
			Secret:  []byte(secret),
			Client:  &http.Client{Timeout: 2 * time.Minute},
		}
	}

	if err := run(*out, splitCSV(*langsCSV), splitCSV(*wordLangsCSV), strings.TrimRight(*baseURL, "/"), *dataVersion, store); err != nil {
		log.Fatalf("build-offline-bundles: %v", err)
	}
}

func run(outDir string, langs, wordLangs []string, baseURL string, dataVersion int, store *publish.WSLibPublicStore) error {
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

	for _, code := range wordLangs {
		if _, ok := langByCode[code]; !ok {
			return fmt.Errorf("word language %q not found in languages table", code)
		}
		entry, err := buildQuranWordsBundle(ctx, database, code, outDir, baseURL, dataVersion, builtAt)
		if err != nil {
			return fmt.Errorf("build quran-words-%s: %w", code, err)
		}
		manifest.Bundles = append(manifest.Bundles, entry)
		log.Printf("built %s: %d bytes, sha256=%s", entry.ID, entry.Bytes, entry.SHA256[:12])
	}

	manifestPath := filepath.Join(outDir, "manifest.json")
	if err := bundle.WriteManifest(manifestPath, manifest); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	log.Printf("wrote manifest with %d bundle(s) to %s", len(manifest.Bundles), manifestPath)

	if store != nil {
		if err := uploadArtifacts(ctx, store, baseURL, outDir, manifest); err != nil {
			return fmt.Errorf("upload: %w", err)
		}
	}
	return nil
}

// uploadArtifacts publishes the bundle files first and the manifest last, so
// a live manifest never references a bundle that has not landed yet. The key
// prefix is derived from baseURL, keeping uploaded keys consistent with the
// URLs the manifest advertises.
func uploadArtifacts(
	ctx context.Context,
	store *publish.WSLibPublicStore,
	baseURL, outDir string,
	manifest bundle.Manifest,
) error {
	prefix, err := publish.KeyPrefixFromBaseURL(baseURL)
	if err != nil {
		return err
	}

	for _, entry := range manifest.Bundles {
		filename := filepath.Base(entry.URL)
		payload, err := os.ReadFile(filepath.Join(outDir, filename))
		if err != nil {
			return fmt.Errorf("read bundle %s: %w", filename, err)
		}
		key := prefix + "/" + filename
		url, err := store.Upload(ctx, key, "application/octet-stream", publish.BundleCacheControl, payload)
		if err != nil {
			return fmt.Errorf("upload %s: %w", key, err)
		}
		log.Printf("uploaded %s (%d bytes) -> %s", key, len(payload), url)
	}

	payload, err := os.ReadFile(filepath.Join(outDir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	key := prefix + "/manifest.json"
	url, err := store.Upload(ctx, key, "application/json", publish.ManifestCacheControl, payload)
	if err != nil {
		return fmt.Errorf("upload %s: %w", key, err)
	}
	log.Printf("uploaded %s (%d bytes) -> %s", key, len(payload), url)
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

// buildQuranWordsBundle renders the word-by-word bundle for one target
// language. The bundle carries every language the reader's word display needs:
// the Arabic word text, the target-language word text (with meanings), and the
// transliteration — mirroring the reader's word_langs of [ar, <lang>, tl].
func buildQuranWordsBundle(
	ctx context.Context,
	database *db.DB,
	langCode string,
	outDir, baseURL string,
	dataVersion int,
	builtAt string,
) (bundle.ManifestEntry, error) {
	wordLangs := dedup([]string{"ar", langCode, "tl"})

	// (1,1)..(114,300) spans every verse; 300 safely exceeds the longest chapter.
	rows, err := database.FetchWordsForVerses(ctx, 1, 114, 1, 300, wordLangs)
	if err != nil {
		return bundle.ManifestEntry{}, fmt.Errorf("fetch words: %w", err)
	}

	words := make([]bundle.Word, 0, len(rows))
	targetRows := 0
	for _, r := range rows {
		if r.LangCode == langCode {
			targetRows++
		}
		words = append(words, bundle.Word{
			CN:      r.ChapterNumber,
			VN:      r.VerseIndex,
			WI:      r.WordIndex,
			GI:      r.GlobalIndex,
			Lang:    r.LangCode,
			Text:    r.WordText,
			Root:    deref(r.WordRoot),
			Meaning: deref(r.Meaning),
		})
	}
	if targetRows == 0 {
		return bundle.ManifestEntry{}, fmt.Errorf("no word rows for language %q", langCode)
	}

	meta := bundle.Meta{
		Scripture:   "quran",
		Lang:        langCode,
		Kind:        bundle.KindWords,
		DataVersion: dataVersion,
		Tokenizer:   bundle.TokenizerNone,
		BuiltAt:     builtAt,
	}

	id := "quran-words-" + langCode
	filename := fmt.Sprintf("%s-v%d.db", id, dataVersion)
	path := filepath.Join(outDir, filename)
	if err := bundle.WriteWords(path, meta, words); err != nil {
		return bundle.ManifestEntry{}, err
	}

	url := baseURL + "/" + filename
	return bundle.NewManifestEntry(path, id, url, meta)
}

func dedup(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
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
