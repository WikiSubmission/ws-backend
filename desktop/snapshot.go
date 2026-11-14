package desktop

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SnapshotFormat is bumped when the manifest or file layout changes.
const SnapshotFormat = 1

// SnapshotTable describes one content table shipped with the desktop app and
// how rows are cleaned when the snapshot is built from a backup.
type SnapshotTable struct {
	Name string
	// Drop lists columns left out of the snapshot entirely.
	Drop []string
	// Blank replaces a column's value with a COPY text literal (\N is NULL).
	// Used for editorial bookkeeping that must not ship, such as user ids.
	Blank map[string]string
	// RequireNonNull skips rows where any of these columns is NULL, for
	// example unpublished editorial rows.
	RequireNonNull []string
}

// SnapshotTables is the content shipped in the desktop app, in an order that
// satisfies foreign keys. It is the single source of truth for both the
// builder (cmd/build-desktop-snapshot) and the loader below. No user data
// table may ever appear here.
var SnapshotTables = []SnapshotTable{
	{Name: "languages"},
	{Name: "quran_versions"},
	{Name: "chapters"},
	{Name: "chapter_text"},
	{Name: "verses"},
	// The embedding column needs pgvector, which the desktop build lacks, and
	// semantic search is off there anyway.
	{Name: "verse_text", Drop: []string{"embedding"}},
	{Name: "words"},
	{Name: "word_text"},
	{Name: "quran_roots"},
	{Name: "appendices"},
	{
		Name:           "editorial_appendices",
		Blank:          map[string]string{"draft": "{}", "created_by": `\N`, "updated_by": `\N`},
		RequireNonNull: []string{"published"},
	},
	{Name: "topic_index_entries"},
	{Name: "topic_index_subentries"},
	{Name: "topic_index_refs"},
}

// clearableTables may be emptied when content is reloaded: the snapshot tables
// plus editorial and derived tables that reference them. The desktop app never
// writes to any of these. A reload aborts if clearing the snapshot tables would
// cascade into a table outside this set, which is how user data stays safe if a
// future migration adds a foreign key from a user table into content.
//
// This is the exact set found by the loader against migrations 001 to 031.
var clearableTables = []string{
	// Bible content and its editorial versions.
	"bible_book_text", "bible_verse_text", "bible_versions", "bible_version_references",
	// Quran editorial drafts, history and published copies.
	"chapter_text_drafts", "chapter_text_history",
	"verse_text_drafts", "verse_text_history",
	"word_text_drafts", "word_text_history",
	"quran_chapter_text_published", "quran_verse_text_published", "quran_word_text_published",
	"quran_root_meaning_drafts", "quran_root_meanings_published",
	"quran_editorial_changelog", "quran_publish_requests",
	// Blog articles and editor permissions or preferences. No editors exist on
	// desktop.
	"editorial_articles",
	"editor_quran_version_grants", "editor_bible_version_grants", "editor_quran_reference_pref",
}

// SnapshotManifest is manifest.json in a snapshot directory.
type SnapshotManifest struct {
	Format         int                     `json:"format"`
	ContentVersion string                  `json:"content_version"`
	BuiltAt        time.Time               `json:"built_at"`
	Source         string                  `json:"source"`
	Tables         []SnapshotManifestTable `json:"tables"`
}

// SnapshotManifestTable records one table file.
type SnapshotManifestTable struct {
	Name    string   `json:"name"`
	File    string   `json:"file"`
	Columns []string `json:"columns"`
	Rows    int64    `json:"rows"`
	SHA256  string   `json:"sha256"`
}

// ReadSnapshotManifest reads and sanity-checks dir/manifest.json.
func ReadSnapshotManifest(dir string) (*SnapshotManifest, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m SnapshotManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("desktop: manifest.json: %w", err)
	}
	if m.Format != SnapshotFormat {
		return nil, fmt.Errorf("desktop: snapshot format %d, want %d", m.Format, SnapshotFormat)
	}
	if m.ContentVersion == "" || len(m.Tables) == 0 {
		return nil, fmt.Errorf("desktop: manifest.json is incomplete")
	}
	for _, t := range m.Tables {
		if !slices.ContainsFunc(SnapshotTables, func(s SnapshotTable) bool { return s.Name == t.Name }) {
			return nil, fmt.Errorf("desktop: snapshot contains unexpected table %q", t.Name)
		}
		if filepath.Base(t.File) != t.File {
			return nil, fmt.Errorf("desktop: snapshot file name %q is not a plain name", t.File)
		}
	}
	return &m, nil
}

// FileSHA256 hashes a file's bytes.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

const createContentStateSQL = `
CREATE TABLE IF NOT EXISTS desktop_content_state (
    id integer PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    content_version text NOT NULL,
    loaded_at timestamptz NOT NULL DEFAULT now()
)`

// LoadSnapshot replaces the content tables with the snapshot in dir when its
// content version differs from the one already loaded. It runs in a single
// transaction, so a failure leaves the previous content in place, and it never
// touches user tables. It reports whether a load happened.
func LoadSnapshot(ctx context.Context, pool *pgxpool.Pool, dir string) (bool, error) {
	m, err := ReadSnapshotManifest(dir)
	if err != nil {
		return false, err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, createContentStateSQL); err != nil {
		return false, fmt.Errorf("desktop: create desktop_content_state: %w", err)
	}
	var current string
	err = conn.QueryRow(ctx, `SELECT content_version FROM desktop_content_state WHERE id = 1`).Scan(&current)
	if err != nil && err != pgx.ErrNoRows {
		return false, err
	}
	if current == m.ContentVersion {
		return false, nil
	}

	// Verify every file before changing anything.
	for _, t := range m.Tables {
		sum, err := FileSHA256(filepath.Join(dir, t.File))
		if err != nil {
			return false, err
		}
		if sum != t.SHA256 {
			return false, fmt.Errorf("desktop: snapshot file %s is corrupt (sha256 mismatch)", t.File)
		}
	}

	start := time.Now()
	slog.Info("Loading content snapshot", "version", m.ContentVersion, "previous", current, "tables", len(m.Tables))

	tx, err := conn.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	names := make([]string, 0, len(m.Tables))
	for _, t := range m.Tables {
		names = append(names, t.Name)
	}
	toClear, err := referencingClosure(ctx, tx, names)
	if err != nil {
		return false, err
	}
	allowed := append(slices.Clone(names), clearableTables...)
	var unknown []string
	for _, t := range toClear {
		if !slices.Contains(allowed, t) {
			unknown = append(unknown, t)
		}
	}
	if len(unknown) > 0 {
		return false, fmt.Errorf("desktop: refusing to reload content: tables %s reference content and are not known to be safe to clear", strings.Join(unknown, ", "))
	}
	if _, err := tx.Exec(ctx, "TRUNCATE "+quoteIdents(toClear)); err != nil {
		return false, fmt.Errorf("desktop: clear content tables: %w", err)
	}

	for _, t := range m.Tables {
		n, err := copyTable(ctx, tx, dir, t)
		if err != nil {
			return false, fmt.Errorf("desktop: load %s: %w", t.Name, err)
		}
		if n != t.Rows {
			return false, fmt.Errorf("desktop: load %s: loaded %d rows, manifest says %d", t.Name, n, t.Rows)
		}
		if err := resetIDSequence(ctx, tx, t.Name); err != nil {
			return false, fmt.Errorf("desktop: reset sequence for %s: %w", t.Name, err)
		}
	}

	if _, err := tx.Exec(ctx, `REFRESH MATERIALIZED VIEW root_index_mv`); err != nil {
		return false, fmt.Errorf("desktop: refresh root_index_mv: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO desktop_content_state (id, content_version, loaded_at) VALUES (1, $1, now())
		ON CONFLICT (id) DO UPDATE SET content_version = EXCLUDED.content_version, loaded_at = now()`,
		m.ContentVersion); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}

	if _, err := conn.Exec(ctx, "ANALYZE "+quoteIdents(names)); err != nil {
		slog.Warn("ANALYZE after snapshot load failed", slog.Any("error", err))
	}
	slog.Info("Content snapshot loaded", "version", m.ContentVersion, "elapsed", time.Since(start).String())
	return true, nil
}

// referencingClosure returns tables plus every table that references them by
// foreign key, transitively. TRUNCATE requires all of them in one statement.
func referencingClosure(ctx context.Context, tx pgx.Tx, tables []string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE closure(oid) AS (
			SELECT c.oid FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relname = ANY($1)
			UNION
			SELECT con.conrelid FROM pg_constraint con
			JOIN closure ON con.confrelid = closure.oid
			WHERE con.contype = 'f'
		)
		SELECT c.relname FROM closure JOIN pg_class c ON c.oid = closure.oid ORDER BY 1`, tables)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	if len(out) < len(tables) {
		return nil, fmt.Errorf("desktop: some snapshot tables do not exist in the schema")
	}
	return out, nil
}

func copyTable(ctx context.Context, tx pgx.Tx, dir string, t SnapshotManifestTable) (int64, error) {
	f, err := os.Open(filepath.Join(dir, t.File))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	defer gz.Close()

	sql := fmt.Sprintf("COPY %s (%s) FROM STDIN", pgx.Identifier{"public", t.Name}.Sanitize(), quoteIdents(t.Columns))
	tag, err := tx.Conn().PgConn().CopyFrom(ctx, gz, sql)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// resetIDSequence moves an id column's sequence past the loaded rows so any
// later insert (none are expected on desktop) cannot collide.
func resetIDSequence(ctx context.Context, tx pgx.Tx, table string) error {
	// Look the column up first: calling pg_get_serial_sequence on a missing
	// column raises an error, which would abort the whole load transaction.
	var seq *string
	err := tx.QueryRow(ctx, `
		SELECT pg_get_serial_sequence('public.' || quote_ident(table_name), 'id')
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1 AND column_name = 'id'`, table).Scan(&seq)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if seq == nil {
		return nil
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(
		`SELECT setval($1, COALESCE((SELECT MAX(id) FROM %s), 1), (SELECT MAX(id) FROM %s) IS NOT NULL)`,
		pgx.Identifier{"public", table}.Sanitize(), pgx.Identifier{"public", table}.Sanitize()), *seq)
	return err
}

func quoteIdents(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = pgx.Identifier{n}.Sanitize()
	}
	return strings.Join(q, ", ")
}
