package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/kumparan/article-service/internal/config"
	"github.com/kumparan/article-service/internal/platform/postgres"
)

// TestOpen_Integration requires a live PostgreSQL instance.
// It is skipped automatically when POSTGRES_DSN is not set so that
// the test suite passes in environments without a database.
//
// To run manually:
//
//	POSTGRES_DSN="postgres://app:app@localhost:5432/articles_test?sslmode=disable" \
//	go test -v ./internal/platform/postgres/
func TestOpen_Integration(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set; skipping integration test")
	}

	cfg := config.DBConfig{
		DSN:      dsn,
		MaxConns: 5,
		MinConns: 1,
	}

	db, err := postgres.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("Ping after Open: %v", err)
	}

	// Verify pool settings are applied.
	stats := db.Stats()
	if stats.MaxOpenConnections != cfg.MaxConns {
		t.Errorf("MaxOpenConnections = %d, want %d", stats.MaxOpenConnections, cfg.MaxConns)
	}
}

// TestOpen_InvalidDSN verifies that an unparseable DSN returns an error
// without panicking. This runs without a live database.
func TestOpen_InvalidDSN(t *testing.T) {
	cfg := config.DBConfig{
		DSN:      "not-a-valid-dsn://???",
		MaxConns: 5,
		MinConns: 1,
	}
	_, err := postgres.Open(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for invalid DSN, got nil")
	}
}
