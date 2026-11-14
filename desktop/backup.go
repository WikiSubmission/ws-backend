package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BackupKind and BackupFormat identify a desktop backup file.
const (
	BackupKind   = "wikisubmission-desktop-backup"
	BackupFormat = 1
)

// BackupTable is one user-data table in a backup. A table is either owned
// directly (UserColumn) or through a parent table (Parent and ParentColumn).
type BackupTable struct {
	Name         string
	UserColumn   string
	Parent       string
	ParentColumn string
}

// BackupTables lists the user's data in parent-before-child order. Row ids are
// kept on restore so the child rows still point at their parents.
var BackupTables = []BackupTable{
	{Name: "bookmark_categories", UserColumn: "user_id"},
	{Name: "bookmark_entries", Parent: "bookmark_categories", ParentColumn: "category_id"},
	{Name: "bookmarks", UserColumn: "user_id"},
	{Name: "notes", UserColumn: "user_id"},
	{Name: "collections", UserColumn: "user_id"},
	{Name: "collection_verses", Parent: "collections", ParentColumn: "collection_id"},
	{Name: "preferences", UserColumn: "user_id"},
	{Name: "reading_progress", UserColumn: "user_id"},
	{Name: "reading_log", UserColumn: "user_id"},
	{Name: "reading_events", UserColumn: "user_id"},
	{Name: "user_activity", UserColumn: "user_id"},
}

// Backup is the file written by ExportBackup and read by RestoreBackup. Each
// table holds its rows as JSON objects keyed by column name.
type Backup struct {
	Kind      string                     `json:"kind"`
	Format    int                        `json:"format"`
	CreatedAt time.Time                  `json:"created_at"`
	Tables    map[string]json.RawMessage `json:"tables"`
}

// ExportBackup returns every row of the user's data tables.
func ExportBackup(ctx context.Context, pool *pgxpool.Pool, userID int) (*Backup, error) {
	b := &Backup{
		Kind:      BackupKind,
		Format:    BackupFormat,
		CreatedAt: time.Now().UTC(),
		Tables:    map[string]json.RawMessage{},
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // read-only

	for _, t := range BackupTables {
		cols, err := writableColumns(ctx, tx, t.Name)
		if err != nil {
			return nil, err
		}
		where, err := ownerFilter(t)
		if err != nil {
			return nil, err
		}
		sql := fmt.Sprintf(
			`SELECT COALESCE(json_agg(r ORDER BY %s), '[]'::json) FROM (SELECT %s FROM %s WHERE %s) r`,
			quoteIdents(cols[:1]), quoteIdents(cols), qualified(t.Name), where)
		var rows json.RawMessage
		if err := tx.QueryRow(ctx, sql, userID).Scan(&rows); err != nil {
			return nil, fmt.Errorf("desktop: back up %s: %w", t.Name, err)
		}
		b.Tables[t.Name] = rows
	}
	return b, nil
}

// RestoreResult reports what RestoreBackup did.
type RestoreResult struct {
	Restored map[string]int `json:"restored"`
	// Skipped lists tables in the file that this version does not restore.
	Skipped []string `json:"skipped,omitempty"`
}

// RestoreBackup replaces the user's data with the backup's, in a single
// transaction: on any error nothing changes. Rows keep their ids so child
// rows still reference their parents, and every owner column is rewritten to
// userID, so a backup taken under another account restores to this one.
func RestoreBackup(ctx context.Context, pool *pgxpool.Pool, userID int, b *Backup) (*RestoreResult, error) {
	if b.Kind != BackupKind {
		return nil, fmt.Errorf("not a WikiSubmission desktop backup")
	}
	if b.Format < 1 || b.Format > BackupFormat {
		return nil, fmt.Errorf("backup format %d is not supported by this version", b.Format)
	}
	res := &RestoreResult{Restored: map[string]int{}}
	for name := range b.Tables {
		if !slices.ContainsFunc(BackupTables, func(t BackupTable) bool { return t.Name == name }) {
			res.Skipped = append(res.Skipped, name)
		}
	}
	slices.Sort(res.Skipped)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	// Children first, so parents can go without relying on cascades.
	for i := len(BackupTables) - 1; i >= 0; i-- {
		t := BackupTables[i]
		where, err := ownerFilter(t)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE %s`, qualified(t.Name), where), userID); err != nil {
			return nil, fmt.Errorf("clear %s: %w", t.Name, err)
		}
	}

	for _, t := range BackupTables {
		raw, ok := b.Tables[t.Name]
		if !ok || len(raw) == 0 {
			continue
		}
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, fmt.Errorf("%s: rows are not a JSON array of objects: %w", t.Name, err)
		}
		if len(rows) == 0 {
			continue
		}
		tableCols, err := writableColumns(ctx, tx, t.Name)
		if err != nil {
			return nil, err
		}
		// Restore the columns the file has and this schema knows. A column
		// missing from an older backup takes its default.
		present := map[string]bool{}
		for _, r := range rows {
			for k := range r {
				present[k] = true
			}
		}
		var cols, exprs []string
		for _, c := range tableCols {
			if !present[c] && c != t.UserColumn {
				continue
			}
			cols = append(cols, c)
			if c == t.UserColumn {
				exprs = append(exprs, "$2::integer")
			} else {
				exprs = append(exprs, "r."+pgx.Identifier{c}.Sanitize())
			}
		}
		sql := fmt.Sprintf(
			`INSERT INTO %s (%s) OVERRIDING SYSTEM VALUE SELECT %s FROM json_populate_recordset(NULL::%s, $1::json) r`,
			qualified(t.Name), quoteIdents(cols), strings.Join(exprs, ", "), qualified(t.Name))
		args := []any{string(raw)}
		if t.UserColumn != "" {
			args = append(args, userID)
		}
		tag, err := tx.Exec(ctx, sql, args...)
		if err != nil {
			return nil, fmt.Errorf("restore %s: %w", t.Name, err)
		}
		if err := resetIDSequence(ctx, tx, t.Name); err != nil {
			return nil, fmt.Errorf("reset sequence for %s: %w", t.Name, err)
		}
		res.Restored[t.Name] = int(tag.RowsAffected())
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return res, nil
}

// ownerFilter selects the rows that belong to $1, the user id.
func ownerFilter(t BackupTable) (string, error) {
	switch {
	case t.UserColumn != "":
		return pgx.Identifier{t.UserColumn}.Sanitize() + " = $1", nil
	case t.Parent != "":
		var parent *BackupTable
		for i := range BackupTables {
			if BackupTables[i].Name == t.Parent {
				parent = &BackupTables[i]
			}
		}
		if parent == nil || parent.UserColumn == "" {
			return "", fmt.Errorf("desktop: %s has no directly owned parent", t.Name)
		}
		return fmt.Sprintf("%s IN (SELECT id FROM %s WHERE %s = $1)",
			pgx.Identifier{t.ParentColumn}.Sanitize(), qualified(parent.Name),
			pgx.Identifier{parent.UserColumn}.Sanitize()), nil
	default:
		return "", fmt.Errorf("desktop: %s has no owner", t.Name)
	}
}

// writableColumns lists a table's columns in order, leaving out generated
// ones, which can be neither exported usefully nor inserted.
func writableColumns(ctx context.Context, tx pgx.Tx, table string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1 AND is_generated = 'NEVER'
		ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, err
	}
	cols, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("desktop: table %s not found", table)
	}
	return cols, nil
}

func qualified(table string) string {
	return pgx.Identifier{"public", table}.Sanitize()
}
