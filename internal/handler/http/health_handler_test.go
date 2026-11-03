package http_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/kumparan/article-service/internal/config"
	httphandler "github.com/kumparan/article-service/internal/handler/http"
	"github.com/kumparan/article-service/internal/platform/postgres"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ── /health ──────────────────────────────────────────────────────────────────

// T3: /health always returns 200 regardless of DB state.
func TestHealthHandler_AlwaysOK(t *testing.T) {
	r := gin.New()
	r.GET("/health", httphandler.HealthHandler())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("body.status = %q, want %q", body["status"], "ok")
	}
}

// ── /ready with live DB ───────────────────────────────────────────────────────

// T1: PostgreSQL up → /ready returns 200.
func TestReadyHandler_DBUp(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set; skipping integration test")
	}

	db, err := postgres.Open(context.Background(), config.DBConfig{DSN: dsn, MaxConns: 2, MinConns: 1})
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	defer db.Close()

	r := gin.New()
	r.GET("/ready", httphandler.ReadyHandler(db))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ready", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// ── /ready with broken DB ─────────────────────────────────────────────────────

// T2: PostgreSQL down → /ready returns 503 without leaking connection detail.
// We test this without a live database by wiring a *sql.DB pointed at a
// port that never accepts connections (127.0.0.1:1).
func TestReadyHandler_DBDown(t *testing.T) {
	cfg := config.DBConfig{
		DSN:      "postgres://nobody:nobody@127.0.0.1:1/nobody?sslmode=disable&connect_timeout=1",
		MaxConns: 1,
		MinConns: 1,
	}
	// Open does a Ping internally; here we call it and expect it to fail at
	// connection time, giving us a *sql.DB that will always fail Ping.
	// If Open itself returns an error (due to Ping), we create the pool
	// directly via pgx stdlib without the ping check.
	db, _ := openBrokenDB(cfg)
	if db == nil {
		t.Skip("could not create broken DB for test")
	}
	defer db.Close()

	r := gin.New()
	r.GET("/ready", httphandler.ReadyHandler(db))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ready", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "unavailable" {
		t.Errorf("body.status = %q, want %q", body["status"], "unavailable")
	}
	// Verify the response body does not contain connection details.
	raw := w.Body.String()
	for _, leak := range []string{"postgres://", "127.0.0.1", "password", "DSN"} {
		if contains(raw, leak) {
			t.Errorf("response body leaks connection detail %q: %s", leak, raw)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && indexStr(s, sub) >= 0)
}

func indexStr(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// openBrokenDB creates a *sql.DB pointed at an unreachable address using pgx stdlib.
// The returned db will always fail Ping. Returns nil if the DSN cannot be parsed.
func openBrokenDB(cfg config.DBConfig) (*sql.DB, error) {
	pgxCfg, err := pgx.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, err
	}
	db := stdlib.OpenDB(*pgxCfg)
	db.SetMaxOpenConns(cfg.MaxConns)
	return db, nil
}
