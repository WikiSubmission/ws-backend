// Command build-desktop-snapshot extracts the Quran content shipped with the
// offline desktop app from a plain-SQL Postgres backup (pg_dump or
// pg_dumpall, optionally gzipped).
//
// Only the tables in desktop.SnapshotTables are written, with their cleaning
// rules applied, so user data in the backup never reaches the snapshot. The
// output directory gets one gzipped COPY text file per table and a
// manifest.json that the desktop sidecar verifies and loads.
//
//	go run ./cmd/build-desktop-snapshot --dump backup.sql.gz --database ws_backend --out ./snapshot
package main

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/wikiSubmission/ws-backend/desktop"
)

var (
	dbHeaderRe = regexp.MustCompile(`^-- Database "([^"]+)" dump`)
	connectRe  = regexp.MustCompile(`^\\connect\s+(?:-reuse-previous=on\s+)?(?:"dbname='?)?([A-Za-z0-9_]+)`)
	copyRe     = regexp.MustCompile(`^COPY (?:public\.)?("?[A-Za-z0-9_]+"?) \((.*)\) FROM stdin;$`)
)

func main() {
	dumpPath := flag.String("dump", "", "plain-SQL backup (.sql or .gz)")
	database := flag.String("database", "ws_backend", "database to read in a pg_dumpall backup; ignored for a single-database pg_dump")
	outDir := flag.String("out", "", "output directory (created; must be empty)")
	flag.Parse()
	if *dumpPath == "" || *outDir == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*dumpPath, *database, *outDir); err != nil {
		log.Fatal(err)
	}
}

type tableWriter struct {
	spec    desktop.SnapshotTable
	file    *os.File
	gz      *gzip.Writer
	buf     *bufio.Writer
	srcCols []string
	outIdx  []int
	outCols []string
	blank   map[int]string
	require []int
	rows    int64
	skipped int64
}

func run(dumpPath, database, outDir string) error {
	if entries, err := os.ReadDir(outDir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s is not empty", outDir)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	in, err := os.Open(dumpPath)
	if err != nil {
		return err
	}
	defer in.Close()
	var r io.Reader = in
	if strings.HasSuffix(strings.ToLower(dumpPath), ".gz") {
		gz, err := gzip.NewReader(in)
		if err != nil {
			return err
		}
		defer gz.Close()
		r = gz
	}

	specs := map[string]desktop.SnapshotTable{}
	for _, t := range desktop.SnapshotTables {
		specs[t.Name] = t
	}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)

	// pg_dumpall output is split into per-database sections; a plain pg_dump
	// has no section header, so an empty current database means "the only one".
	current := ""
	sawSections := false
	done := map[string]*tableWriter{}
	var w *tableWriter

	for sc.Scan() {
		line := sc.Text()
		if w != nil {
			if line == `\.` {
				if err := w.close(); err != nil {
					return err
				}
				done[w.spec.Name] = w
				w = nil
				continue
			}
			if err := w.writeRow(line); err != nil {
				return err
			}
			continue
		}

		if m := dbHeaderRe.FindStringSubmatch(line); m != nil {
			current, sawSections = m[1], true
			continue
		}
		if m := connectRe.FindStringSubmatch(line); m != nil {
			current, sawSections = m[1], true
			continue
		}
		m := copyRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if sawSections && current != database {
			continue
		}
		name := strings.Trim(m[1], `"`)
		spec, ok := specs[name]
		if !ok {
			continue
		}
		if _, dup := done[name]; dup {
			return fmt.Errorf("table %s appears twice in database %s", name, database)
		}
		w, err = newTableWriter(outDir, spec, parseColumns(m[2]))
		if err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if w != nil {
		return fmt.Errorf("backup ended inside the COPY block for %s", w.spec.Name)
	}

	manifest := desktop.SnapshotManifest{
		Format:  desktop.SnapshotFormat,
		BuiltAt: time.Now().UTC(),
		Source:  filepath.Base(dumpPath),
	}
	versionHash := sha256.New()
	for _, spec := range desktop.SnapshotTables {
		tw, ok := done[spec.Name]
		if !ok {
			return fmt.Errorf("table %s not found in database %q of the backup", spec.Name, database)
		}
		sum, err := desktop.FileSHA256(tw.file.Name())
		if err != nil {
			return err
		}
		fmt.Fprintf(versionHash, "%s:%s:%s\n", spec.Name, strings.Join(tw.outCols, ","), sum)
		manifest.Tables = append(manifest.Tables, desktop.SnapshotManifestTable{
			Name:    spec.Name,
			File:    filepath.Base(tw.file.Name()),
			Columns: tw.outCols,
			Rows:    tw.rows,
			SHA256:  sum,
		})
		note := ""
		if tw.skipped > 0 {
			note = fmt.Sprintf(" (%d rows skipped)", tw.skipped)
		}
		fmt.Printf("%-24s %9d rows%s\n", spec.Name, tw.rows, note)
	}
	// The content version depends only on the content, so rebuilding from an
	// unchanged backup does not force every install to reload.
	manifest.ContentVersion = hex.EncodeToString(versionHash.Sum(nil))[:16]

	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("content version %s written to %s\n", manifest.ContentVersion, outDir)
	return nil
}

func newTableWriter(outDir string, spec desktop.SnapshotTable, srcCols []string) (*tableWriter, error) {
	w := &tableWriter{spec: spec, srcCols: srcCols, blank: map[int]string{}}
	for i, c := range srcCols {
		if slices.Contains(spec.Drop, c) {
			continue
		}
		w.outIdx = append(w.outIdx, i)
		w.outCols = append(w.outCols, c)
		if v, ok := spec.Blank[c]; ok {
			w.blank[i] = v
		}
		if slices.Contains(spec.RequireNonNull, c) {
			w.require = append(w.require, i)
		}
	}
	for c := range spec.Blank {
		if !slices.Contains(srcCols, c) {
			return nil, fmt.Errorf("%s: blank column %q not in backup", spec.Name, c)
		}
	}
	for _, c := range spec.RequireNonNull {
		if !slices.Contains(srcCols, c) {
			return nil, fmt.Errorf("%s: required column %q not in backup", spec.Name, c)
		}
	}
	f, err := os.Create(filepath.Join(outDir, spec.Name+".copy.gz"))
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		f.Close()
		return nil, err
	}
	// A fixed header keeps the file, and so the content version, identical
	// across rebuilds of the same backup.
	gz.ModTime = time.Time{}
	gz.Name = ""
	w.file, w.gz, w.buf = f, gz, bufio.NewWriterSize(gz, 1<<20)
	return w, nil
}

// writeRow rewrites one COPY text-format row. Tabs inside values are escaped
// as \t in this format, so splitting on raw tabs is exact.
func (w *tableWriter) writeRow(line string) error {
	fields := strings.Split(line, "\t")
	if len(fields) != len(w.srcCols) {
		return fmt.Errorf("%s: row has %d fields, header has %d", w.spec.Name, len(fields), len(w.srcCols))
	}
	for _, i := range w.require {
		if fields[i] == `\N` {
			w.skipped++
			return nil
		}
	}
	for n, i := range w.outIdx {
		if n > 0 {
			if err := w.buf.WriteByte('\t'); err != nil {
				return err
			}
		}
		v := fields[i]
		if b, ok := w.blank[i]; ok {
			v = b
		}
		if _, err := w.buf.WriteString(v); err != nil {
			return err
		}
	}
	w.rows++
	return w.buf.WriteByte('\n')
}

func (w *tableWriter) close() error {
	if err := w.buf.Flush(); err != nil {
		return err
	}
	if err := w.gz.Close(); err != nil {
		return err
	}
	return w.file.Close()
}

// parseColumns splits a COPY column list, unquoting identifiers such as
// "position".
func parseColumns(list string) []string {
	parts := strings.Split(list, ",")
	out := make([]string, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, `"`) && strings.HasSuffix(p, `"`) {
			p = strings.ReplaceAll(p[1:len(p)-1], `""`, `"`)
		}
		out[i] = p
	}
	return out
}
