// Package builder orchestrates a full offline-bundle build: it renders the
// text and word-by-word bundles from Postgres, writes the manifest, and
// optionally publishes everything to the CDN through ws-lib. It is shared by
// the build-offline-bundles CLI and the admin rebuild endpoint, so both paths
// produce identical artifacts.
package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/wikiSubmission/ws-backend/db"
	"github.com/wikiSubmission/ws-backend/offline/bundle"
	"github.com/wikiSubmission/ws-backend/offline/publish"
)

// Options selects what to build and where it goes.
type Options struct {
	// Langs are the Quran text-bundle languages (e.g. ar, en).
	Langs []string
	// WordLangs are the word-by-word bundle target languages (each bundle
	// carries ar + target + tl word rows). Empty skips word bundles.
	WordLangs []string
	// OutDir receives the bundle files and manifest.json.
	OutDir string
	// BaseURL is the public URL prefix the manifest points downloads at,
	// e.g. https://cdn.wikisubmission.org/offline.
	BaseURL string
	// DataVersion is stamped into every bundle and its versioned filename.
	DataVersion int
	// Store publishes the artifacts to the CDN when non-nil.
	Store *publish.WSLibPublicStore
}

// Run builds every requested bundle plus the manifest into opts.OutDir and,
// when opts.Store is set, uploads them (bundles first, manifest last, so a
// live manifest never references a bundle that has not landed yet).
func Run(ctx context.Context, database *db.DB, opts Options) (bundle.Manifest, error) {
	if len(opts.Langs) == 0 {
		return bundle.Manifest{}, fmt.Errorf("no languages requested")
	}

	builtAt := time.Now().UTC().Format(time.RFC3339)

	langRows, err := database.FetchAllLanguages(ctx)
	if err != nil {
		return bundle.Manifest{}, fmt.Errorf("fetch languages: %w", err)
	}
	langByCode := make(map[string]db.LanguageRow, len(langRows))
	for _, l := range langRows {
		langByCode[l.Code] = l
	}

	available, err := database.FetchQuranLangs(ctx)
	if err != nil {
		return bundle.Manifest{}, fmt.Errorf("fetch quran langs: %w", err)
	}

	var manifest bundle.Manifest
	for _, code := range opts.Langs {
		lang, ok := langByCode[code]
		if !ok {
			return bundle.Manifest{}, fmt.Errorf("language %q not found in languages table", code)
		}
		if !available[code] {
			return bundle.Manifest{}, fmt.Errorf("language %q has no verse_text rows", code)
		}

		entry, err := buildQuranBundle(ctx, database, lang, opts, builtAt)
		if err != nil {
			return bundle.Manifest{}, fmt.Errorf("build quran-%s: %w", code, err)
		}
		manifest.Bundles = append(manifest.Bundles, entry)
		slog.Info("built offline bundle", "id", entry.ID, "bytes", entry.Bytes, "sha256", entry.SHA256[:12])
	}

	for _, code := range opts.WordLangs {
		if _, ok := langByCode[code]; !ok {
			return bundle.Manifest{}, fmt.Errorf("word language %q not found in languages table", code)
		}
		entry, err := buildQuranWordsBundle(ctx, database, code, opts, builtAt)
		if err != nil {
			return bundle.Manifest{}, fmt.Errorf("build quran-words-%s: %w", code, err)
		}
		manifest.Bundles = append(manifest.Bundles, entry)
		slog.Info("built offline bundle", "id", entry.ID, "bytes", entry.Bytes, "sha256", entry.SHA256[:12])
	}

	manifestPath := filepath.Join(opts.OutDir, "manifest.json")
	if err := bundle.WriteManifest(manifestPath, manifest); err != nil {
		return bundle.Manifest{}, fmt.Errorf("write manifest: %w", err)
	}
	slog.Info("wrote offline manifest", "bundles", len(manifest.Bundles), "path", manifestPath)

	if opts.Store != nil {
		if err := uploadArtifacts(ctx, opts, manifest); err != nil {
			return bundle.Manifest{}, fmt.Errorf("upload: %w", err)
		}
	}
	return manifest, nil
}

// NextDataVersion fetches the currently published manifest and returns
// max(data_version)+1, so every rebuild lands on fresh immutable URLs. A
// missing or unreadable manifest starts at 1 (first publish); any other
// failure is returned so a transient CDN error cannot silently reset
// versioning.
func NextDataVersion(ctx context.Context, client *http.Client, manifestURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return 0, err
	}
	// The manifest is short-TTL, but a rebuild must see the latest version.
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		// Not published yet (S3/CloudFront surface missing keys as 403 or 404).
		return 1, nil
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("fetch manifest: unexpected status %d", resp.StatusCode)
	}

	var m bundle.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return 0, fmt.Errorf("decode manifest: %w", err)
	}
	next := 1
	for _, b := range m.Bundles {
		if b.DataVersion >= next {
			next = b.DataVersion + 1
		}
	}
	return next, nil
}

// uploadArtifacts publishes the bundle files first and the manifest last. The
// key prefix is derived from BaseURL, keeping uploaded keys consistent with
// the URLs the manifest advertises.
func uploadArtifacts(ctx context.Context, opts Options, manifest bundle.Manifest) error {
	prefix, err := publish.KeyPrefixFromBaseURL(opts.BaseURL)
	if err != nil {
		return err
	}

	for _, entry := range manifest.Bundles {
		filename := filepath.Base(entry.URL)
		payload, err := os.ReadFile(filepath.Join(opts.OutDir, filename))
		if err != nil {
			return fmt.Errorf("read bundle %s: %w", filename, err)
		}
		key := prefix + "/" + filename
		url, err := opts.Store.Upload(ctx, key, "application/octet-stream", publish.BundleCacheControl, payload)
		if err != nil {
			return fmt.Errorf("upload %s: %w", key, err)
		}
		slog.Info("uploaded offline bundle", "key", key, "bytes", len(payload), "url", url)
	}

	payload, err := os.ReadFile(filepath.Join(opts.OutDir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	key := prefix + "/manifest.json"
	url, err := opts.Store.Upload(ctx, key, "application/json", publish.ManifestCacheControl, payload)
	if err != nil {
		return fmt.Errorf("upload %s: %w", key, err)
	}
	slog.Info("uploaded offline manifest", "key", key, "bytes", len(payload), "url", url)
	return nil
}

func buildQuranBundle(
	ctx context.Context,
	database *db.DB,
	lang db.LanguageRow,
	opts Options,
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
		DataVersion: opts.DataVersion,
		Tokenizer:   tokenizer,
		BuiltAt:     builtAt,
	}

	id := "quran-" + lang.Code
	filename := fmt.Sprintf("%s-v%d.db", id, opts.DataVersion)
	path := filepath.Join(opts.OutDir, filename)
	if err := bundle.Write(path, meta, chapters, verses); err != nil {
		return bundle.ManifestEntry{}, err
	}

	url := opts.BaseURL + "/" + filename
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
	opts Options,
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
		DataVersion: opts.DataVersion,
		Tokenizer:   bundle.TokenizerNone,
		BuiltAt:     builtAt,
	}

	id := "quran-words-" + langCode
	filename := fmt.Sprintf("%s-v%d.db", id, opts.DataVersion)
	path := filepath.Join(opts.OutDir, filename)
	if err := bundle.WriteWords(path, meta, words); err != nil {
		return bundle.ManifestEntry{}, err
	}

	url := opts.BaseURL + "/" + filename
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
