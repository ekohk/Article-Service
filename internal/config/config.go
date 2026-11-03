// Package config loads and validates application configuration from environment variables.
// The process exits at startup if any required variable is missing or invalid.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all validated application configuration.
type Config struct {
	AppEnv string

	HTTP   HTTPConfig
	DB     DBConfig
	ES     ESConfig
	Redis  RedisConfig
	Outbox OutboxConfig
}

type HTTPConfig struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

type DBConfig struct {
	DSN      string
	MaxConns int
	MinConns int
}

type ESConfig struct {
	URL   string
	Index string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	CacheTTL time.Duration
}

type OutboxConfig struct {
	BatchSize    int
	PollInterval time.Duration
	MaxAttempts  int
}

// Load reads all required environment variables and returns a validated Config.
// Returns an error listing every missing or invalid variable so the caller can fail fast.
func Load() (*Config, error) {
	var errs []string

	get := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			errs = append(errs, fmt.Sprintf("missing required env: %s", key))
		}
		return v
	}

	getDefault := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}

	parseInt := func(key string, def int) int {
		v := os.Getenv(key)
		if v == "" {
			return def
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid int for %s: %v", key, err))
			return def
		}
		return n
	}

	parseDuration := func(key string, def time.Duration) time.Duration {
		v := os.Getenv(key)
		if v == "" {
			return def
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid duration for %s: %v", key, err))
			return def
		}
		return d
	}

	cfg := &Config{
		AppEnv: getDefault("APP_ENV", "development"),
		HTTP: HTTPConfig{
			Addr:         getDefault("HTTP_ADDR", ":8080"),
			ReadTimeout:  parseDuration("HTTP_READ_TIMEOUT", 5*time.Second),
			WriteTimeout: parseDuration("HTTP_WRITE_TIMEOUT", 10*time.Second),
			IdleTimeout:  parseDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		},
		DB: DBConfig{
			DSN:      get("POSTGRES_DSN"),
			MaxConns: parseInt("POSTGRES_MAX_CONNS", 20),
			MinConns: parseInt("POSTGRES_MIN_CONNS", 2),
		},
		ES: ESConfig{
			URL:   get("ELASTICSEARCH_URL"),
			Index: getDefault("ELASTICSEARCH_INDEX", "articles-v1"),
		},
		Redis: RedisConfig{
			Addr:     get("REDIS_ADDR"),
			Password: os.Getenv("REDIS_PASSWORD"), // optional
			DB:       parseInt("REDIS_DB", 0),
			CacheTTL: parseDuration("CACHE_TTL", 45*time.Second),
		},
		Outbox: OutboxConfig{
			BatchSize:    parseInt("OUTBOX_BATCH_SIZE", 100),
			PollInterval: parseDuration("OUTBOX_POLL_INTERVAL", 1*time.Second),
			MaxAttempts:  parseInt("OUTBOX_MAX_ATTEMPTS", 10),
		},
	}

	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "\n"))
	}
	return cfg, nil
}
