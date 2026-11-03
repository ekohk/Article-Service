// Package main seeds the database with development/test author data.
// Run with: go run ./scripts/seed/seed.go
// Required env: POSTGRES_DSN
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/kumparan/article-service/internal/config"
	"github.com/kumparan/article-service/internal/platform/postgres"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	db, err := postgres.Open(context.Background(), cfg.DB)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db open:", err)
		os.Exit(1)
	}
	defer db.Close()

	authors := []struct{ name string }{
		{"Budi Santoso"},
		{"Siti Rahayu"},
		{"Ahmad Fauzi"},
		// Duplicate name intentional — tests D-01 behaviour.
		{"Budi Santoso"},
	}

	for _, a := range authors {
		var id int64
		err := db.QueryRowContext(context.Background(),
			`INSERT INTO authors (name) VALUES ($1) RETURNING id`, a.name).Scan(&id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "insert author %q: %v\n", a.name, err)
			os.Exit(1)
		}
		fmt.Printf("inserted author id=%d name=%q\n", id, a.name)
	}
}
