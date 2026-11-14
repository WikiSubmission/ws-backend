package db

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestRunMigrations_SecondBootKeepsNotes boots twice against the same database
// and checks that user notes and normal bookmarks survive the second boot.
// The old runner replayed 004 on every start, which ran DELETE FROM notes.
func TestRunMigrations_SecondBootKeepsNotes(t *testing.T) {
	db := setupTestDB(t) // first boot
	ctx := context.Background()

	authID := "test-ledger-" + time.Now().Format("150405.000000")
	if _, err := db.UpsertUser(ctx, authID, authID+"@example.com", nil); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM users WHERE auth_id = $1`, authID)
	})

	if _, err := db.UpsertNote(ctx, authID, "quran", "2:255", "keep me", nil); err != nil {
		t.Fatalf("UpsertNote: %v", err)
	}
	if _, err := db.CreateBookmark(ctx, authID, "quran", "1:1", "bm", "amber", "normal"); err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}

	// Second boot.
	if err := db.RunMigrations(ctx); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}

	notes, err := db.FetchNotes(ctx, authID, "quran", nil)
	if err != nil {
		t.Fatalf("FetchNotes: %v", err)
	}
	if len(notes) != 1 || notes[0].Content != "keep me" {
		t.Fatalf("note did not survive a second boot: %+v", notes)
	}

	var bookmarks int
	if err := db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM bookmarks b JOIN users u ON u.id = b.user_id
		 WHERE u.auth_id = $1 AND b.kind = 'normal'`, authID).Scan(&bookmarks); err != nil {
		t.Fatalf("count bookmarks: %v", err)
	}
	if bookmarks != 1 {
		t.Fatalf("normal bookmarks after second boot = %d, want 1", bookmarks)
	}

	// Every embedded file must be in the ledger.
	all, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	applied, err := ledgerNames(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if !applied[m.Name] {
			t.Errorf("migration %s missing from schema_migrations", m.Name)
		}
	}
}

// TestRunMigrations_BootstrapsExistingSchema simulates a production database
// migrated by the old runner: full schema, empty ledger. The bootstrap must
// record every file up to the baseline without executing any of them, and a
// user's note must still be there afterwards.
func TestRunMigrations_BootstrapsExistingSchema(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Save and restore the ledger so the shared test database is unchanged.
	saved, err := db.Pool.Query(ctx, `SELECT name, applied_at FROM public.schema_migrations`)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	type row struct {
		name string
		at   time.Time
	}
	var rows []row
	for saved.Next() {
		var r row
		if err := saved.Scan(&r.name, &r.at); err != nil {
			t.Fatalf("scan ledger: %v", err)
		}
		rows = append(rows, r)
	}
	saved.Close()
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM public.schema_migrations`)
		for _, r := range rows {
			_, _ = db.Pool.Exec(ctx,
				`INSERT INTO public.schema_migrations (name, applied_at) VALUES ($1, $2)
				 ON CONFLICT DO NOTHING`, r.name, r.at)
		}
	})

	authID := "test-ledger-boot-" + time.Now().Format("150405.000000")
	if _, err := db.UpsertUser(ctx, authID, authID+"@example.com", nil); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM users WHERE auth_id = $1`, authID)
	})
	if _, err := db.UpsertNote(ctx, authID, "quran", "36:1", "survives bootstrap", nil); err != nil {
		t.Fatalf("UpsertNote: %v", err)
	}

	if _, err := db.Pool.Exec(ctx, `DELETE FROM public.schema_migrations`); err != nil {
		t.Fatalf("empty ledger: %v", err)
	}

	all, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	// Only the pre-ledger files: later files would legitimately run, and this
	// test is about the bootstrap path alone.
	var pre []migration
	for _, m := range all {
		if m.Name <= ledgerBaseline {
			pre = append(pre, m)
		}
	}

	// Swap each file's SQL for one that fails, so the test fails loudly if the
	// bootstrap ever executes a file instead of just recording it.
	poisoned := make([]migration, len(pre))
	for i, m := range pre {
		poisoned[i] = migration{Name: m.Name, SQL: "SELECT 1/0"}
	}
	if err := applyMigrations(ctx, db.Pool, poisoned); err != nil {
		t.Fatalf("bootstrap executed a migration instead of recording it: %v", err)
	}

	applied, err := ledgerNames(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != len(pre) {
		t.Errorf("ledger has %d rows after bootstrap, want %d", len(applied), len(pre))
	}
	for _, m := range pre {
		if !applied[m.Name] {
			t.Errorf("bootstrap did not record %s", m.Name)
		}
	}

	notes, err := db.FetchNotes(ctx, authID, "quran", nil)
	if err != nil {
		t.Fatalf("FetchNotes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("note count after bootstrap = %d, want 1", len(notes))
	}
}

// TestApplyMigrations_RunsOnceAndRollsBack checks the two ledger guarantees on
// throwaway migrations: an applied file is skipped on the next run, and a
// failing file leaves neither its changes nor a ledger row behind.
func TestApplyMigrations_RunsOnceAndRollsBack(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	suffix := time.Now().Format("150405000000")
	table := "ledger_test_" + suffix
	failTable := "ledger_test_fail_" + suffix
	okName := "zz_ledger_test_ok_" + suffix + ".sql"
	failName := "zz_ledger_test_fail_" + suffix + ".sql"
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s, %s`, table, failTable))
		_, _ = db.Pool.Exec(ctx, `DELETE FROM public.schema_migrations WHERE name = ANY($1)`,
			[]string{okName, failName})
	})

	ok := migration{Name: okName, SQL: fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (n int); INSERT INTO %s VALUES (1);`, table, table)}

	for i := 0; i < 2; i++ {
		if err := applyMigrations(ctx, db.Pool, []migration{ok}); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	var n int
	if err := db.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s`, table)).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("migration ran %d times, want 1", n)
	}

	bad := migration{Name: failName, SQL: fmt.Sprintf(
		`CREATE TABLE %s (n int); SELECT 1/0;`, failTable)}
	if err := applyMigrations(ctx, db.Pool, []migration{bad}); err == nil {
		t.Fatal("expected failing migration to return an error")
	}
	var exists bool
	if err := db.Pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, failTable).Scan(&exists); err != nil {
		t.Fatalf("to_regclass: %v", err)
	}
	if exists {
		t.Error("failed migration left its table behind")
	}
	applied, err := ledgerNames(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if applied[failName] {
		t.Error("failed migration was recorded in the ledger")
	}
}

func ledgerNames(ctx context.Context, db *DB) (map[string]bool, error) {
	rows, err := db.Pool.Query(ctx, `SELECT name FROM public.schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read ledger: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}
