// Package migrator wraps Goose to provide programmatic migration execution.
// Used by integration tests and startup scripts; not called in the hot path.
package migrator

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

// Up runs all pending migrations from the given directory against db.
func Up(db *sql.DB, migrationsDir string) error {
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrator: set dialect: %w", err)
	}
	if err := goose.Up(db, migrationsDir); err != nil {
		return fmt.Errorf("migrator: up: %w", err)
	}
	return nil
}

// Down rolls back the last applied migration.
func Down(db *sql.DB, migrationsDir string) error {
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrator: set dialect: %w", err)
	}
	if err := goose.Down(db, migrationsDir); err != nil {
		return fmt.Errorf("migrator: down: %w", err)
	}
	return nil
}

// DownTo rolls back all migrations down to (but not including) the target version.
// Pass version 0 to roll back everything.
func DownTo(db *sql.DB, migrationsDir string, version int64) error {
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrator: set dialect: %w", err)
	}
	if err := goose.DownTo(db, migrationsDir, version); err != nil {
		return fmt.Errorf("migrator: down-to: %w", err)
	}
	return nil
}
