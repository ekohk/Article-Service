package migrator_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kumparan/article-service/internal/config"
	"github.com/kumparan/article-service/internal/platform/migrator"
	"github.com/kumparan/article-service/internal/platform/postgres"
)

// migrationsDir resolves to the project-root /migrations directory regardless
// of where the test binary is executed from.
func migrationsDir(t *testing.T) string {
	t.Helper()
	// Walk up from this file's directory to find the migrations folder.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		candidate := filepath.Join(dir, "migrations")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate migrations directory")
		}
		dir = parent
	}
}

func openTestDB(t *testing.T) interface{ Close() error } {
	t.Helper()
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set; skipping migration integration test")
	}
	cfg := config.DBConfig{DSN: dsn, MaxConns: 5, MinConns: 1}
	db, err := postgres.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestMigration_UpDownUp covers T1, T2, T3 from IMPL-02 acceptance criteria.
//
// T1: goose up from empty database (or already-clean state) → all tables created.
// T2: goose up called again with no new migrations → no error (idempotent).
// T3: goose down → tables removed cleanly.
// Extra: goose up again → back to latest.
func TestMigration_UpDownUp(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set; skipping migration integration test")
	}

	cfg := config.DBConfig{DSN: dsn, MaxConns: 5, MinConns: 1}
	db, err := postgres.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	defer db.Close()

	dir := migrationsDir(t)

	// Ensure we start from a clean state by rolling back everything first.
	_ = migrator.DownTo(db, dir, 0) // ignore error if already clean

	// T1: up from empty database.
	t.Run("T1_UpFromEmpty", func(t *testing.T) {
		if err := migrator.Up(db, dir); err != nil {
			t.Fatalf("goose up: %v", err)
		}
		// Verify tables exist.
		tables := []string{"authors", "articles", "outbox_events"}
		for _, tbl := range tables {
			var exists bool
			row := db.QueryRow(
				`SELECT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema='public' AND table_name=$1
				)`, tbl)
			if err := row.Scan(&exists); err != nil || !exists {
				t.Errorf("table %q not found after migration up", tbl)
			}
		}
		// Verify version column exists on articles.
		var colExists bool
		row := db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name='articles' AND column_name='version'
			)`)
		if err := row.Scan(&colExists); err != nil || !colExists {
			t.Error("column articles.version not found after migration up")
		}
		// Verify locked_by column exists on outbox_events.
		row = db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name='outbox_events' AND column_name='locked_by'
			)`)
		if err := row.Scan(&colExists); err != nil || !colExists {
			t.Error("column outbox_events.locked_by not found after migration up")
		}
	})

	// T2: up again with no new migrations → idempotent, no error.
	t.Run("T2_UpIdempotent", func(t *testing.T) {
		if err := migrator.Up(db, dir); err != nil {
			t.Fatalf("goose up (second call) returned error: %v", err)
		}
	})

	// T3: down → tables removed.
	t.Run("T3_DownRollback", func(t *testing.T) {
		if err := migrator.DownTo(db, dir, 0); err != nil {
			t.Fatalf("goose down-to 0: %v", err)
		}
		for _, tbl := range []string{"articles", "authors", "outbox_events"} {
			var exists bool
			row := db.QueryRow(
				`SELECT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema='public' AND table_name=$1
				)`, tbl)
			_ = row.Scan(&exists)
			if exists {
				t.Errorf("table %q still exists after full rollback", tbl)
			}
		}
	})

	// Restore to latest so subsequent tests have a clean schema.
	t.Run("T4_UpAfterRollback", func(t *testing.T) {
		if err := migrator.Up(db, dir); err != nil {
			t.Fatalf("goose up after rollback: %v", err)
		}
	})
}
