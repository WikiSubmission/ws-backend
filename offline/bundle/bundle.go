// Package bundle writes offline content bundles: per-scripture+language SQLite
// files containing verse text, chapter metadata, and an FTS5 full-text index,
// plus the manifest that the client polls to discover and verify them.
//
// The schema is frozen at SchemaVersion (see org/docs/offline-architecture.md).
// This package is deliberately decoupled from the source database: callers pass
// already-fetched Verse/Chapter slices, so the writer is unit-testable with
// synthetic data and never needs a live Postgres. The cmd/build-offline-bundles
// command is the thin layer that reads from the DB and calls Write.
package bundle

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/wikiSubmission/ws-backend/offline/textnorm"
)

// SchemaVersion is the frozen content-DB schema version. A client refuses a
// bundle whose schema_version exceeds the version it understands.
const SchemaVersion = 1

// Tokenizer choices recorded in meta.fts_tokenizer and the manifest.
const (
	// TokenizerTrigram suits Arabic: no stemming, substring-capable.
	TokenizerTrigram = "trigram"
	// TokenizerLatin stems Latin translations; remove_diacritics is a backstop
	// since the indexed `norm` column is already diacritic-free.
	TokenizerLatin = "porter unicode61 remove_diacritics 2"
	// TokenizerNone marks bundles that carry no FTS index (word bundles).
	TokenizerNone = "none"
)

// Bundle kinds recorded in meta.kind and the manifest. An absent kind means
// KindText, so pre-kind manifests and bundles stay valid.
const (
	// KindText is a verse-text bundle: verses + chapters + FTS index.
	KindText = "text"
	// KindWords is a word-by-word bundle: per-word text/root/meaning rows for
	// the languages needed to render the word breakdown in one target language.
	KindWords = "words"
)

// Verse is one verse in one language. Subtitle and Footnote are empty when absent.
type Verse struct {
	VK       string // verse key, e.g. "2:255"
	CN       int    // chapter number
	VN       int    // verse number (verse_index)
	Text     string // original text, stored verbatim for display
	Subtitle string
	Footnote string
}

// Chapter is the per-bundle chapter metadata that lets the sidebar render offline.
type Chapter struct {
	CN              int
	Title           string
	TitleTranslit   string
	VerseCount      int
}

// Word is one word's text in one language, plus its root and meaning when the
// source rows carry them (roots ride on Arabic rows, meanings on translation
// rows). Root and Meaning are empty when absent.
type Word struct {
	CN      int    // chapter number
	VN      int    // verse number (verse_index)
	WI      int    // word_index within the verse (0-based)
	GI      int    // global_index across the whole Quran (1-based)
	Lang    string // language of Text ("ar", "en", "tl", ...)
	Text    string
	Root    string
	Meaning string
}

// Meta describes a bundle. NormalizationVersion is always textnorm.Version.
type Meta struct {
	Scripture   string // "quran"
	Lang        string // "ar", "en", ...
	Kind        string // KindText (default when empty) or KindWords
	DataVersion int    // bumped when content changes; drives client updates
	Tokenizer   string // one of the Tokenizer* constants
	BuiltAt     string // RFC3339; passed in by the caller (deterministic in tests)
}

// kindOrDefault resolves the empty-Kind default so callers predating the kind
// field keep producing text bundles.
func (m Meta) kindOrDefault() string {
	if m.Kind == "" {
		return KindText
	}
	return m.Kind
}

// Write creates a content bundle at path with the frozen schema, computing the
// normalized search column for every verse and building the FTS index. Any
// existing file at path is replaced, since bundles are immutable artifacts.
func Write(path string, meta Meta, chapters []Chapter, verses []Verse) (err error) {
	if meta.Scripture == "" || meta.Lang == "" {
		return fmt.Errorf("bundle: meta.Scripture and meta.Lang are required")
	}
	if meta.Tokenizer == "" {
		return fmt.Errorf("bundle: meta.Tokenizer is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if err := createSchema(db, meta.Tokenizer); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if err := populate(db, meta, chapters, verses); err != nil {
		return fmt.Errorf("populate: %w", err)
	}
	return nil
}

// WriteWords creates a word-by-word bundle at path: per-word rows for every
// language passed in (typically Arabic + the target translation + the
// transliteration), keyed so the client can rebuild the API's per-verse word
// arrays offline. No FTS index — word search stays online-only for now.
func WriteWords(path string, meta Meta, words []Word) (err error) {
	if meta.Scripture == "" || meta.Lang == "" {
		return fmt.Errorf("bundle: meta.Scripture and meta.Lang are required")
	}
	if meta.kindOrDefault() != KindWords {
		return fmt.Errorf("bundle: WriteWords requires meta.Kind = %q", KindWords)
	}
	if len(words) == 0 {
		return fmt.Errorf("bundle: no word rows to write")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	stmts := []string{
		`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT) WITHOUT ROWID;`,
		`CREATE TABLE words (
			cn INTEGER NOT NULL,
			vn INTEGER NOT NULL,
			wi INTEGER NOT NULL,
			gi INTEGER NOT NULL,
			lang TEXT NOT NULL,
			text TEXT NOT NULL,
			root TEXT,
			meaning TEXT,
			PRIMARY KEY (cn, vn, wi, lang)
		) WITHOUT ROWID;`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("exec %q: %w", s, err)
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := insertMeta(tx, meta); err != nil {
		return err
	}

	wstmt, err := tx.Prepare(
		`INSERT INTO words(cn, vn, wi, gi, lang, text, root, meaning) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer wstmt.Close()
	for _, w := range words {
		if _, err := wstmt.Exec(
			w.CN, w.VN, w.WI, w.GI, w.Lang, w.Text,
			nullIfEmpty(w.Root), nullIfEmpty(w.Meaning),
		); err != nil {
			return fmt.Errorf("insert word %d:%d[%d] %s: %w", w.CN, w.VN, w.WI, w.Lang, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	if _, err := db.Exec(`VACUUM`); err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	return nil
}

func createSchema(db *sql.DB, tokenizer string) error {
	stmts := []string{
		`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT) WITHOUT ROWID;`,
		`CREATE TABLE verses (
			vk TEXT PRIMARY KEY,
			cn INTEGER NOT NULL,
			vn INTEGER NOT NULL,
			text TEXT NOT NULL,
			subtitle TEXT,
			footnote TEXT,
			norm TEXT NOT NULL
		);`,
		`CREATE INDEX idx_verses_cn ON verses(cn, vn);`,
		`CREATE TABLE chapters (
			cn INTEGER PRIMARY KEY,
			title TEXT NOT NULL,
			title_transliterated TEXT,
			verse_count INTEGER NOT NULL
		) WITHOUT ROWID;`,
		// External-content FTS over the normalized column. Built once via
		// 'rebuild' after the content rows are inserted (see populate).
		fmt.Sprintf(`CREATE VIRTUAL TABLE verses_fts USING fts5(
			norm, subtitle, footnote,
			content='verses', content_rowid='rowid',
			tokenize='%s'
		);`, tokenizer),
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("exec %q: %w", s, err)
		}
	}
	return nil
}

func populate(db *sql.DB, meta Meta, chapters []Chapter, verses []Verse) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := insertMeta(tx, meta); err != nil {
		return err
	}

	for _, c := range chapters {
		if _, err := tx.Exec(
			`INSERT INTO chapters(cn, title, title_transliterated, verse_count) VALUES(?, ?, ?, ?)`,
			c.CN, c.Title, nullIfEmpty(c.TitleTranslit), c.VerseCount,
		); err != nil {
			return fmt.Errorf("insert chapter %d: %w", c.CN, err)
		}
	}

	vstmt, err := tx.Prepare(
		`INSERT INTO verses(vk, cn, vn, text, subtitle, footnote, norm) VALUES(?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer vstmt.Close()
	for _, v := range verses {
		if _, err := vstmt.Exec(
			v.VK, v.CN, v.VN, v.Text,
			nullIfEmpty(v.Subtitle), nullIfEmpty(v.Footnote),
			textnorm.ForSearch(v.Text),
		); err != nil {
			return fmt.Errorf("insert verse %s: %w", v.VK, err)
		}
	}

	// Build the FTS index from the content table now that rows exist, then
	// merge b-tree segments so queries are fast and the file is compact.
	if _, err := tx.Exec(`INSERT INTO verses_fts(verses_fts) VALUES('rebuild')`); err != nil {
		return fmt.Errorf("fts rebuild: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO verses_fts(verses_fts) VALUES('optimize')`); err != nil {
		return fmt.Errorf("fts optimize: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	// VACUUM cannot run inside a transaction; compact the immutable artifact.
	if _, err := db.Exec(`VACUUM`); err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	return nil
}

func insertMeta(tx *sql.Tx, meta Meta) error {
	metaRows := [][2]string{
		{"schema_version", fmt.Sprintf("%d", SchemaVersion)},
		{"data_version", fmt.Sprintf("%d", meta.DataVersion)},
		{"normalization_version", fmt.Sprintf("%d", textnorm.Version)},
		{"scripture", meta.Scripture},
		{"lang", meta.Lang},
		{"kind", meta.kindOrDefault()},
		{"fts_tokenizer", meta.Tokenizer},
		{"built_at", meta.BuiltAt},
	}
	for _, kv := range metaRows {
		if _, err := tx.Exec(`INSERT INTO meta(key, value) VALUES(?, ?)`, kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ── Manifest ───────────────────────────────────────────────────────────────

// ManifestEntry is one bundle's record in the manifest. Mirrors the JSON shape
// in org/docs/offline-architecture.md.
type ManifestEntry struct {
	ID                   string `json:"id"`
	Scripture            string `json:"scripture"`
	Lang                 string `json:"lang"`
	Kind                 string `json:"kind"`
	URL                  string `json:"url"`
	Bytes                int64  `json:"bytes"`
	SHA256               string `json:"sha256"`
	DataVersion          int    `json:"data_version"`
	SchemaVersion        int    `json:"schema_version"`
	NormalizationVersion int    `json:"normalization_version"`
	FTSTokenizer         string `json:"fts_tokenizer"`
}

// Manifest is the document served at GET /offline/manifest.
type Manifest struct {
	Bundles []ManifestEntry `json:"bundles"`
}

// NewManifestEntry hashes the bundle file at path and assembles its manifest
// record. url is the fully-formed (versioned) download URL for the file.
func NewManifestEntry(path, id, url string, meta Meta) (ManifestEntry, error) {
	sum, size, err := checksum(path)
	if err != nil {
		return ManifestEntry{}, err
	}
	return ManifestEntry{
		ID:                   id,
		Scripture:            meta.Scripture,
		Lang:                 meta.Lang,
		Kind:                 meta.kindOrDefault(),
		URL:                  url,
		Bytes:                size,
		SHA256:               sum,
		DataVersion:          meta.DataVersion,
		SchemaVersion:        SchemaVersion,
		NormalizationVersion: textnorm.Version,
		FTSTokenizer:         meta.Tokenizer,
	}, nil
}

// WriteManifest serializes the manifest as pretty JSON to path.
func WriteManifest(path string, m Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func checksum(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
