// Package integration contains end-to-end tests that require a live PostgreSQL
// instance. Set POSTGRES_DSN to run; tests are skipped otherwise.
package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	appsvc "github.com/kumparan/article-service/internal/application/article"
	"github.com/kumparan/article-service/internal/config"
	domain "github.com/kumparan/article-service/internal/domain/article"
	"github.com/kumparan/article-service/internal/platform/migrator"
	"github.com/kumparan/article-service/internal/platform/postgres"
	pgrepo "github.com/kumparan/article-service/internal/repository/postgres"
)

// ── test setup ────────────────────────────────────────────────────────────────

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set; skipping integration test")
	}
	cfg := config.DBConfig{DSN: dsn, MaxConns: 5, MinConns: 1}
	db, err := postgres.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	return db
}

func migrationsDir(t *testing.T) string {
	t.Helper()
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

// setupDB ensures the schema is at the latest version and returns a cleanup func.
func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openDB(t)
	t.Cleanup(func() { db.Close() })

	dir := migrationsDir(t)
	// Ensure schema is current.
	if err := migrator.Up(db, dir); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return db
}

// seedAuthor inserts an author and returns its ID.
func seedAuthor(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(),
		`INSERT INTO authors (name) VALUES ($1) RETURNING id`, name).Scan(&id)
	if err != nil {
		t.Fatalf("seed author %q: %v", name, err)
	}
	t.Cleanup(func() {
		// Clean up to keep tests independent.
		db.ExecContext(context.Background(), `DELETE FROM outbox_events WHERE aggregate_type='article'`)
		db.ExecContext(context.Background(), `DELETE FROM articles`)
		db.ExecContext(context.Background(), `DELETE FROM authors WHERE id = $1`, id)
	})
	return id
}

func buildSvc(db *sql.DB) *appsvc.Service {
	return appsvc.New(db,
		pgrepo.NewArticleRepository(db),
		pgrepo.NewAuthorRepository(db),
		pgrepo.NewOutboxRepository(db, 5*time.Minute, 10),
	)
}

// ── T4: valid request → article and outbox event in DB ───────────────────────

func TestCreate_Integration_ValidRequest(t *testing.T) {
	db := setupDB(t)
	authorID := seedAuthor(t, db, "Integration Author")
	svc := buildSvc(db)

	result, err := svc.Create(context.Background(), appsvc.CreateRequest{
		AuthorID: authorID,
		Title:    "Integration Test Article",
		Body:     "This body was written by an integration test.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if result.ID == 0 {
		t.Error("article ID must be non-zero")
	}
	if result.Version != 1 {
		t.Errorf("version = %d, want 1", result.Version)
	}
	if result.CreatedAt.IsZero() {
		t.Error("created_at must be set by DB")
	}

	// Verify article is in DB.
	var dbTitle string
	var dbVersion int64
	err = db.QueryRowContext(context.Background(),
		`SELECT title, version FROM articles WHERE id = $1`, result.ID).Scan(&dbTitle, &dbVersion)
	if err != nil {
		t.Fatalf("read back article: %v", err)
	}
	if dbTitle != "Integration Test Article" {
		t.Errorf("title = %q, want %q", dbTitle, "Integration Test Article")
	}
	if dbVersion != 1 {
		t.Errorf("db version = %d, want 1", dbVersion)
	}

	// Verify outbox event exists in same state.
	var eventCount int
	db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM outbox_events WHERE aggregate_id = $1 AND event_type = 'article.created' AND processed_at IS NULL`,
		result.ID).Scan(&eventCount)
	if eventCount != 1 {
		t.Errorf("outbox events = %d, want 1", eventCount)
	}
}

// ── T5: outbox failure → article rollback ─────────────────────────────────────

// Simulate outbox insert failure by dropping the outbox_events table temporarily.
// After the test the table is restored via migration.
func TestCreate_Integration_OutboxFailureRollsBackArticle(t *testing.T) {
	db := setupDB(t)
	authorID := seedAuthor(t, db, "Rollback Author")
	svc := buildSvc(db)

	// Rename table to force outbox insert to fail.
	_, err := db.ExecContext(context.Background(), `ALTER TABLE outbox_events RENAME TO outbox_events_disabled`)
	if err != nil {
		t.Fatalf("rename outbox table: %v", err)
	}
	t.Cleanup(func() {
		db.ExecContext(context.Background(), `ALTER TABLE outbox_events_disabled RENAME TO outbox_events`)
	})

	_, err = svc.Create(context.Background(), appsvc.CreateRequest{
		AuthorID: authorID,
		Title:    "Should be rolled back",
		Body:     "This article must not persist.",
	})
	if err == nil {
		t.Fatal("expected error when outbox table missing, got nil")
	}

	// Restore table before verifying.
	db.ExecContext(context.Background(), `ALTER TABLE outbox_events_disabled RENAME TO outbox_events`)

	var count int
	db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM articles WHERE title = $1`, "Should be rolled back").Scan(&count)
	if count != 0 {
		t.Errorf("article count = %d after rollback, want 0", count)
	}
}

// ── T6: author not found ──────────────────────────────────────────────────────

func TestCreate_Integration_AuthorNotFound(t *testing.T) {
	db := setupDB(t)
	svc := buildSvc(db)

	_, err := svc.Create(context.Background(), appsvc.CreateRequest{
		AuthorID: 999999,
		Title:    "Title",
		Body:     "Body",
	})
	if !isDomainErr(err, domain.ErrAuthorNotFound) {
		t.Errorf("expected ErrAuthorNotFound, got %v", err)
	}
}

// ── T7: validation errors (no DB touch) ──────────────────────────────────────

func TestCreate_Integration_ValidationErrors(t *testing.T) {
	db := setupDB(t)
	svc := buildSvc(db)
	ctx := context.Background()

	cases := []struct {
		name string
		req  appsvc.CreateRequest
	}{
		{"blank title", appsvc.CreateRequest{AuthorID: 1, Title: "   ", Body: "b"}},
		{"blank body", appsvc.CreateRequest{AuthorID: 1, Title: "t", Body: ""}},
		{"zero author", appsvc.CreateRequest{AuthorID: 0, Title: "t", Body: "b"}},
		{"title too long", appsvc.CreateRequest{AuthorID: 1, Title: strings.Repeat("x", 501), Body: "b"}},
		{"body too long", appsvc.CreateRequest{AuthorID: 1, Title: "t", Body: strings.Repeat("x", 50001)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(ctx, tc.req)
			if !isDomainErr(err, domain.ErrInvalidInput) {
				t.Errorf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

func isDomainErr(err, target error) bool {
	return errors.Is(err, target)
}
