// Package postgres provides the PostgreSQL connection pool used by all
// repository implementations. It wraps pgx/v5's stdlib adapter so repositories
// can use the standard database/sql interface with parameterised queries.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/kumparan/article-service/internal/config"
)

// Open creates and validates a *sql.DB backed by pgx.
// It applies pool sizing from cfg and performs a ping before returning.
// The caller is responsible for calling Close when the pool is no longer needed.
func Open(ctx context.Context, cfg config.DBConfig) (*sql.DB, error) {
	pgxCfg, err := pgx.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse DSN: %w", err)
	}

	db := stdlib.OpenDB(*pgxCfg)

	db.SetMaxOpenConns(cfg.MaxConns)
	db.SetMaxIdleConns(cfg.MinConns)
	// Idle connections are recycled after 5 minutes to avoid stale connections
	// behind load balancers or firewalls.
	db.SetConnMaxIdleTime(5 * time.Minute)
	// Absolute max lifetime prevents long-lived connections from accumulating
	// server-side state.
	db.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres: ping failed: %w", err)
	}

	return db, nil
}
