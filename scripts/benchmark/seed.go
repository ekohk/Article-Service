// Command seed creates a configurable synthetic article dataset.
//
// Example:
//
//	go run ./scripts/benchmark/seed.go -dsn "postgres://..." -articles 10000
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strings"

	"github.com/kumparan/article-service/internal/config"
	"github.com/kumparan/article-service/internal/platform/postgres"
)

func main() {
	var dsn string
	var articleCount, authorCount, batchSize, seed int
	flag.StringVar(&dsn, "dsn", "", "PostgreSQL DSN (required)")
	flag.IntVar(&articleCount, "articles", 10000, "number of articles to create")
	flag.IntVar(&authorCount, "authors", 100, "number of authors to create")
	flag.IntVar(&batchSize, "batch", 500, "insert batch size")
	flag.IntVar(&seed, "seed", 19, "random seed")
	flag.Parse()
	if dsn == "" {
		log.Fatal("-dsn is required")
	}
	if articleCount < 1 || authorCount < 2 || batchSize < 1 {
		log.Fatal("articles must be positive, authors must be at least 2, and batch must be positive")
	}

	db, err := postgres.Open(context.Background(), config.DBConfig{DSN: dsn, MaxConns: 10, MinConns: 1})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	authors, err := insertAuthors(ctx, db, authorCount)
	if err != nil {
		log.Fatal(err)
	}
	if err := insertArticles(ctx, db, authors, articleCount, batchSize, seed); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("seeded articles=%d authors=%d dominant_author_id=%d dominant_share=%.2f%% long_tail_authors=%d\n",
		articleCount, authorCount, authors[0], float64((articleCount+2)/3)/float64(articleCount)*100, authorCount-1)
}

func insertAuthors(ctx context.Context, db *sql.DB, count int) ([]int64, error) {
	ids := make([]int64, 0, count)
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("Benchmark Author %04d", i+1)
		var id int64
		if err := db.QueryRowContext(ctx,
			`INSERT INTO authors (name) VALUES ($1) RETURNING id`, name).Scan(&id); err != nil {
			return nil, fmt.Errorf("insert author %d: %w", i, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func insertArticles(ctx context.Context, db *sql.DB, authors []int64, count, batchSize, seed int) error {
	rng := rand.New(rand.NewSource(int64(seed)))
	for start := 0; start < count; start += batchSize {
		end := start + batchSize
		if end > count {
			end = count
		}
		args := make([]any, 0, (end-start)*3)
		values := make([]string, 0, end-start)
		for i := start; i < end; i++ {
			authorID := authors[0]
			if i >= (count+2)/3 {
				authorID = authors[1+rng.Intn(len(authors)-1)]
			}
			n := len(args)
			values = append(values, fmt.Sprintf("($%d,$%d,$%d)", n+1, n+2, n+3))
			args = append(args,
				authorID,
				fmt.Sprintf("benchmark keyword article %06d", i+1),
				fmt.Sprintf("Synthetic benchmark body %06d with searchable content.", i+1))
		}
		query := `INSERT INTO articles (author_id, title, body) VALUES ` + strings.Join(values, ",")
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("insert articles %d-%d: %w", start, end, err)
		}
	}
	return nil
}
