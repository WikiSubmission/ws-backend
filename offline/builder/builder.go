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
	// LibraryLangs are the library-documents bundle languages (introduction,
	// proclamation, appendices with FTS). Empty skips library bundles.
	LibraryLangs []string
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

	for _, code := range opts.LibraryLangs {
		entry, err := buildLibraryBundle(ctx, database, code, opts, builtAt)
		if err != nil {
			return bundle.Manifest{}, fmt.Errorf("build library-%s: %w", code, err)
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

// fetchPublishedManifest reads the currently published manifest. found is
// false when nothing is published yet (S3/CloudFront surface missing keys as
// 403 or 404); any other failure is an error so a transient CDN problem is
// never mistaken for an empty CDN.
func fetchPublishedManifest(ctx context.Context, client *http.Client, manifestURL string) (bundle.Manifest, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return bundle.Manifest{}, false, err
	}
	// The manifest is short-TTL, but callers must see the latest version.
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		return bundle.Manifest{}, false, fmt.Errorf("fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return bundle.Manifest{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return bundle.Manifest{}, false, fmt.Errorf("fetch manifest: unexpected status %d", resp.StatusCode)
	}

	var m bundle.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return bundle.Manifest{}, false, fmt.Errorf("decode manifest: %w", err)
	}
	return m, true, nil
}

// NextDataVersion fetches the currently published manifest and returns
// max(data_version)+1, so every rebuild lands on fresh immutable URLs. A
// missing manifest starts at 1 (first publish).
func NextDataVersion(ctx context.Context, client *http.Client, manifestURL string) (int, error) {
	m, found, err := fetchPublishedManifest(ctx, client, manifestURL)
	if err != nil {
		return 0, err
	}
	if !found {
		return 1, nil
	}
	next := 1
	for _, b := range m.Bundles {
		if b.DataVersion >= next {
			next = b.DataVersion + 1
		}
	}
	return next, nil
}

// PublishedFile is one artifact found on the CDN.
type PublishedFile struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Bytes        int64  `json:"bytes"`
	LastModified string `json:"last_modified,omitempty"` // RFC1123 from the CDN, verbatim
}

// PublishedVersion groups the artifacts of one data_version.
type PublishedVersion struct {
	DataVersion int             `json:"data_version"`
	Live        bool            `json:"live"` // referenced by the live manifest.json
	Files       []PublishedFile `json:"files"`
}

// PublishedState is everything the admin UI shows about the CDN's contents.
type PublishedState struct {
	ManifestURL string             `json:"manifest_url"`
	LiveVersion int                `json:"live_version"` // 0 when nothing is published
	Versions    []PublishedVersion `json:"versions"`     // newest first
}

// ListPublished discovers what actually exists on the CDN. Bundle URLs are
// deterministic ({id}-v{N}.db, manifest-v{N}.json), so it reads the live
// manifest for the id set and the highest version, then HEAD-probes every
// version from 1 through live+1 (the +1 catches artifacts a failed rebuild
// uploaded before dying, which no manifest references). Only versions with at
// least one existing file are returned. Ids that were dropped from the
// manifest entirely are invisible to this probe; acceptable while the id set
// only grows.
func ListPublished(ctx context.Context, client *http.Client, baseURL string) (PublishedState, error) {
	manifestURL := baseURL + "/manifest.json"
	m, found, err := fetchPublishedManifest(ctx, client, manifestURL)
	if err != nil {
		return PublishedState{}, err
	}
	state := PublishedState{ManifestURL: manifestURL}
	if !found {
		return state, nil
	}

	idSet := map[string]bool{}
	var ids []string
	for _, b := range m.Bundles {
		if b.DataVersion > state.LiveVersion {
			state.LiveVersion = b.DataVersion
		}
		if !idSet[b.ID] {
			idSet[b.ID] = true
			ids = append(ids, b.ID)
		}
	}

	for v := state.LiveVersion + 1; v >= 1; v-- {
		names := make([]string, 0, len(ids)+1)
		for _, id := range ids {
			names = append(names, fmt.Sprintf("%s-v%d.db", id, v))
		}
		names = append(names, fmt.Sprintf("manifest-v%d.json", v))

		var files []PublishedFile
		for _, name := range names {
			file, ok, err := probeFile(ctx, client, baseURL+"/"+name, name)
			if err != nil {
				return PublishedState{}, err
			}
			if ok {
				files = append(files, file)
			}
		}
		if len(files) > 0 {
			state.Versions = append(state.Versions, PublishedVersion{
				DataVersion: v,
				Live:        v == state.LiveVersion,
				Files:       files,
			})
		}
	}
	return state, nil
}

// probeFile HEADs one CDN URL; ok is false when the file does not exist.
func probeFile(ctx context.Context, client *http.Client, url, name string) (PublishedFile, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return PublishedFile{}, false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return PublishedFile{}, false, fmt.Errorf("probe %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return PublishedFile{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return PublishedFile{}, false, fmt.Errorf("probe %s: unexpected status %d", name, resp.StatusCode)
	}
	return PublishedFile{
		Name:         name,
		URL:          url,
		Bytes:        resp.ContentLength,
		LastModified: resp.Header.Get("Last-Modified"),
	}, true, nil
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

	// A versioned, immutable snapshot first (manifest-v{N}.json) so past
	// manifests stay inspectable (e.g. in the admin bundles page), then the
	// live manifest last so it never references a missing artifact.
	versionKey := fmt.Sprintf("%s/manifest-v%d.json", prefix, opts.DataVersion)
	if _, err := opts.Store.Upload(ctx, versionKey, "application/json", publish.BundleCacheControl, payload); err != nil {
		return fmt.Errorf("upload %s: %w", versionKey, err)
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

// buildLibraryBundle renders the library-documents bundle for one language:
// every section of the introduction, proclamation, and appendices, with an
// FTS index so the client can search them offline. Scripture is "library" so
// the id scheme ({scripture}-{lang}) yields library-en.
func buildLibraryBundle(
	ctx context.Context,
	database *db.DB,
	langCode string,
	opts Options,
	builtAt string,
) (bundle.ManifestEntry, error) {
	rows, err := database.FetchLibrarySections(ctx, langCode)
	if err != nil {
		return bundle.ManifestEntry{}, fmt.Errorf("fetch library sections: %w", err)
	}
	if len(rows) == 0 {
		return bundle.ManifestEntry{}, fmt.Errorf("no library sections for language %q (run the library seed)", langCode)
	}

	docs := make([]bundle.Doc, 0, len(rows))
	for _, r := range rows {
		docNumber := 0
		if r.DocNumber != nil {
			docNumber = *r.DocNumber
		}
		heading := ""
		if r.Heading != nil {
			heading = *r.Heading
		}
		docs = append(docs, bundle.Doc{
			DocType:      r.DocType,
			DocNumber:    docNumber,
			Title:        r.Title,
			SectionIndex: r.SectionIndex,
			Heading:      heading,
			Body:         r.Body,
		})
	}

	meta := bundle.Meta{
		Scripture:   "library",
		Lang:        langCode,
		Kind:        bundle.KindLibrary,
		DataVersion: opts.DataVersion,
		Tokenizer:   bundle.TokenizerLatin,
		BuiltAt:     builtAt,
	}

	id := "library-" + langCode
	filename := fmt.Sprintf("%s-v%d.db", id, opts.DataVersion)
	path := filepath.Join(opts.OutDir, filename)
	if err := bundle.WriteLibrary(path, meta, docs); err != nil {
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
