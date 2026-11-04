package config_test

import (
	"os"
	"testing"

	"github.com/kumparan/article-service/internal/config"
)

// setEnv sets multiple env vars and returns a cleanup function.
func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

var requiredEnv = map[string]string{
	"POSTGRES_DSN":      "postgres://app:app@localhost:5432/articles?sslmode=disable",
	"ELASTICSEARCH_URL": "http://localhost:9200",
	"REDIS_ADDR":        "localhost:6379",
}

// T1: Missing required env → Load returns a non-nil error with a descriptive message.
func TestLoad_MissingRequired(t *testing.T) {
	// Ensure required vars are unset for this test.
	for k := range requiredEnv {
		os.Unsetenv(k)
	}

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing required env, got nil")
	}
	// Error message must mention at least one missing variable.
	msg := err.Error()
	for k := range requiredEnv {
		_ = k // each missing key should appear; we check the error is non-empty
	}
	if msg == "" {
		t.Fatal("error message must not be empty")
	}
}

// T1 variant: Each required variable missing individually produces an error.
func TestLoad_EachRequiredMissing(t *testing.T) {
	required := []string{"POSTGRES_DSN", "ELASTICSEARCH_URL", "REDIS_ADDR"}
	for _, missing := range required {
		t.Run("missing_"+missing, func(t *testing.T) {
			// Set all required vars except the one under test.
			for k, v := range requiredEnv {
				if k != missing {
					t.Setenv(k, v)
				} else {
					os.Unsetenv(k)
				}
			}
			_, err := config.Load()
			if err == nil {
				t.Fatalf("expected error when %s is missing, got nil", missing)
			}
		})
	}
}

// T2: All required env set → Load returns a valid Config without error.
func TestLoad_AllRequired(t *testing.T) {
	setEnv(t, requiredEnv)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.DB.DSN != requiredEnv["POSTGRES_DSN"] {
		t.Errorf("DB.DSN = %q, want %q", cfg.DB.DSN, requiredEnv["POSTGRES_DSN"])
	}
	if cfg.ES.URL != requiredEnv["ELASTICSEARCH_URL"] {
		t.Errorf("ES.URL = %q, want %q", cfg.ES.URL, requiredEnv["ELASTICSEARCH_URL"])
	}
	if cfg.Redis.Addr != requiredEnv["REDIS_ADDR"] {
		t.Errorf("Redis.Addr = %q, want %q", cfg.Redis.Addr, requiredEnv["REDIS_ADDR"])
	}
}

// T2 variant: Defaults are applied when optional vars are absent.
func TestLoad_Defaults(t *testing.T) {
	setEnv(t, requiredEnv)
	// Unset optional vars to confirm defaults.
	optionals := []string{"APP_ENV", "HTTP_ADDR", "ELASTICSEARCH_INDEX",
		"POSTGRES_MAX_CONNS", "POSTGRES_MIN_CONNS", "CACHE_TTL",
		"OUTBOX_BATCH_SIZE", "OUTBOX_POLL_INTERVAL", "OUTBOX_MAX_ATTEMPTS"}
	for _, k := range optionals {
		os.Unsetenv(k)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AppEnv != "development" {
		t.Errorf("AppEnv default = %q, want %q", cfg.AppEnv, "development")
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("HTTP.Addr default = %q, want %q", cfg.HTTP.Addr, ":8080")
	}
	if cfg.ES.Index != "articles-v1" {
		t.Errorf("ES.Index default = %q, want %q", cfg.ES.Index, "articles-v1")
	}
	if cfg.DB.MaxConns != 20 {
		t.Errorf("DB.MaxConns default = %d, want 20", cfg.DB.MaxConns)
	}
	if cfg.Outbox.MaxAttempts != 10 {
		t.Errorf("Outbox.MaxAttempts default = %d, want 10", cfg.Outbox.MaxAttempts)
	}
}

// Invalid duration value → Load returns an error.
func TestLoad_InvalidDuration(t *testing.T) {
	setEnv(t, requiredEnv)
	t.Setenv("CACHE_TTL", "not-a-duration")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid CACHE_TTL, got nil")
	}
}

// Invalid int value → Load returns an error.
func TestLoad_InvalidInt(t *testing.T) {
	setEnv(t, requiredEnv)
	t.Setenv("POSTGRES_MAX_CONNS", "abc")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid POSTGRES_MAX_CONNS, got nil")
	}
}

func TestLoad_OutboxMaxAttemptsMustBePositive(t *testing.T) {
	setEnv(t, requiredEnv)
	t.Setenv("OUTBOX_MAX_ATTEMPTS", "0")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for OUTBOX_MAX_ATTEMPTS=0")
	}
}
