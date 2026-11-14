package db

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationLockKey is the pg_advisory_lock key that serializes migration runs,
// so two processes booting against the same database (a rolling deploy, or a
// seed command started next to the server) never apply the same file twice.
const migrationLockKey int64 = 0x7773_6d69_6772 // "wsmigr"

// ledgerBaseline is the last migration that existed before the ledger was
// introduced, and ledgerBaselineTable is a table that file creates. A database
// with an empty ledger but this table present was migrated by the old
// replay-everything runner, so every file up to and including the baseline is
// recorded as applied without running it again. Files after the baseline are
// always executed normally.
const (
	ledgerBaseline      = "031_quran_topic_index.sql"
	ledgerBaselineTable = "public.topic_index_entries"
)

const createLedgerSQL = `CREATE TABLE IF NOT EXISTS public.schema_migrations (
    name       text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
)`

// migration is one SQL file from db/migrations.
type migration struct {
	Name string
	SQL  string
}

// loadMigrations returns the embedded .sql files sorted by name, so numeric
// prefixes set the order.
func loadMigrations() ([]migration, error) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}
	var out []migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		content, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", entry.Name(), err)
		}
		out = append(out, migration{Name: entry.Name(), SQL: string(content)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// RunMigrations applies every embedded migration that is not yet recorded in
// the schema_migrations ledger. Each file runs once, inside its own
// transaction together with its ledger row, so a failure leaves neither a
// partial schema change nor a ledger entry behind.
func (db *DB) RunMigrations(ctx context.Context) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	return applyMigrations(ctx, db.Pool, migrations)
}

// applyMigrations is the ledger-driven runner behind RunMigrations. It takes
// the migration list as an argument so tests can drive it with their own files.
func applyMigrations(ctx context.Context, pool *pgxpool.Pool, migrations []migration) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	// The advisory lock is session scoped, so every statement below must use
	// this same connection. It is released before the connection goes back to
	// the pool (defers run in reverse order).
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		if _, err := conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockKey); err != nil {
			slog.Warn("Failed to release migration lock", slog.Any("error", err))
		}
	}()

	if _, err := conn.Exec(ctx, createLedgerSQL); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := appliedMigrations(ctx, conn.Conn())
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		if applied, err = bootstrapLedger(ctx, conn.Conn(), migrations); err != nil {
			return err
		}
	}

	for _, m := range migrations {
		if applied[m.Name] {
			continue
		}
		slog.Info("Applying database migration", "file", m.Name)
		if err := applyOne(ctx, conn.Conn(), m); err != nil {
			return err
		}
	}
	return nil
}

func appliedMigrations(ctx context.Context, conn *pgx.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, `SELECT name FROM public.schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	applied := make(map[string]bool, len(names))
	for _, n := range names {
		applied[n] = true
	}
	return applied, nil
}

// bootstrapLedger handles a database migrated before the ledger existed. When
// the baseline table is present, every file up to ledgerBaseline is recorded
// as applied without being executed. A fresh database (no baseline table) is
// left alone and gets every file run in order.
func bootstrapLedger(ctx context.Context, conn *pgx.Conn, migrations []migration) (map[string]bool, error) {
	applied := map[string]bool{}

	var exists bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, ledgerBaselineTable).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check ledger baseline: %w", err)
	}
	if !exists {
		return applied, nil
	}

	var names []string
	for _, m := range migrations {
		if m.Name <= ledgerBaseline {
			names = append(names, m.Name)
		}
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO public.schema_migrations (name) SELECT unnest($1::text[]) ON CONFLICT DO NOTHING`,
		names); err != nil {
		return nil, fmt.Errorf("bootstrap schema_migrations: %w", err)
	}
	for _, n := range names {
		applied[n] = true
	}
	slog.Info("Recorded existing schema in migration ledger", "files", len(names), "baseline", ledgerBaseline)
	return applied, nil
}

// applyOne runs a single migration file and records it, atomically. Files are
// executed with the simple protocol (no arguments), which permits several
// statements per file. Statements that cannot run inside a transaction block,
// such as CREATE INDEX CONCURRENTLY, are therefore not allowed in migrations.
func applyOne(ctx context.Context, conn *pgx.Conn, m migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", m.Name, err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, m.SQL); err != nil {
		return fmt.Errorf("migration failed in %s: %w", m.Name, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.schema_migrations (name) VALUES ($1)`, m.Name); err != nil {
		return fmt.Errorf("record migration %s: %w", m.Name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", m.Name, err)
	}
	return nil
}
