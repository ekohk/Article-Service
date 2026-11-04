package integration_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	appsvc "github.com/kumparan/article-service/internal/application/article"
	"github.com/kumparan/article-service/internal/config"
	"github.com/kumparan/article-service/internal/platform/elasticsearch"
	pgrepo "github.com/kumparan/article-service/internal/repository/postgres"
)

func TestArticleSearch_Integration_KeywordAuthorAndFallback(t *testing.T) {
	db := setupDB(t)
	esURL := os.Getenv("ELASTICSEARCH_URL")
	if esURL == "" {
		t.Skip("ELASTICSEARCH_URL not set; skipping search integration test")
	}

	authorName := fmt.Sprintf("Budi Search %d", time.Now().UnixNano())
	var authorOne, authorTwo int64
	if err := db.QueryRowContext(context.Background(),
		`INSERT INTO authors (name) VALUES ($1) RETURNING id`, authorName,
	).Scan(&authorOne); err != nil {
		t.Fatalf("insert first author: %v", err)
	}
	if err := db.QueryRowContext(context.Background(),
		`INSERT INTO authors (name) VALUES ($1) RETURNING id`, " "+authorName+" ",
	).Scan(&authorTwo); err != nil {
		t.Fatalf("load second author: %v", err)
	}
	articleOneID := time.Now().UnixNano()
	articleTwoID := articleOneID + 1
	articleThreeID := articleOneID + 2
	for _, row := range []struct {
		id, authorID int64
		title, body  string
	}{
		{articleOneID, authorOne, "golang search", "keyword body"},
		{articleTwoID, authorTwo, "golang search", "second author body"},
		{articleThreeID, authorOne, "unrelated", "different content"},
	} {
		if _, err := db.ExecContext(context.Background(), `
			INSERT INTO articles (id, author_id, title, body, version)
			VALUES ($1, $2, $3, $4, 1)`,
			row.id, row.authorID, row.title, row.body,
		); err != nil {
			t.Fatalf("insert article %d: %v", row.id, err)
		}
	}
	t.Cleanup(func() {
		db.ExecContext(context.Background(), `DELETE FROM outbox_events WHERE aggregate_id IN ($1, $2, $3)`, articleOneID, articleTwoID, articleThreeID)
		db.ExecContext(context.Background(), `DELETE FROM articles WHERE id IN ($1, $2, $3)`, articleOneID, articleTwoID, articleThreeID)
		db.ExecContext(context.Background(), `DELETE FROM authors WHERE id IN ($1, $2)`, authorOne, authorTwo)
	})

	esClient, err := elasticsearch.New(config.ESConfig{URL: esURL, Index: "articles-v1"})
	if err != nil {
		t.Fatalf("create Elasticsearch client: %v", err)
	}
	if err := esClient.EnsureIndex(context.Background()); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	authorRepo := pgrepo.NewAuthorRepository(db)
	articleRepo := pgrepo.NewArticleRepository(db)
	for _, id := range []int64{articleOneID, articleTwoID, articleThreeID} {
		item, err := articleRepo.GetByID(context.Background(), id)
		if err != nil {
			t.Fatalf("load article %d: %v", id, err)
		}
		author, err := authorRepo.GetByID(context.Background(), item.AuthorID)
		if err != nil {
			t.Fatalf("load author for article %d: %v", id, err)
		}
		if err := esClient.IndexArticle(context.Background(), item, author); err != nil {
			t.Fatalf("index article %d: %v", id, err)
		}
	}
	refresh, err := esClient.Raw().Indices.Refresh(
		esClient.Raw().Indices.Refresh.WithIndex(esClient.Index()),
	)
	if err != nil {
		t.Fatalf("refresh index: %v", err)
	}
	refresh.Body.Close()

	svc := appsvc.NewWithSearch(
		db, articleRepo, authorRepo,
		pgrepo.NewOutboxRepository(db, 5*time.Minute, 10),
		esClient, nil,
	)
	result, err := svc.Search(context.Background(), appsvc.SearchRequest{
		Query: "golang", Author: " " + strings.ToUpper(authorName) + " ", Limit: 20,
	})
	if err != nil {
		t.Fatalf("keyword plus author search: %v", err)
	}
	if len(result.Articles) != 2 {
		t.Fatalf("keyword plus author results = %d, want 2", len(result.Articles))
	}

	authorOnly, err := svc.Search(context.Background(), appsvc.SearchRequest{
		Query: " ", Author: authorName, Limit: 20,
	})
	if err != nil {
		t.Fatalf("author-only fallback: %v", err)
	}
	if len(authorOnly.Articles) != 3 {
		t.Fatalf("author-only results = %d, want 3", len(authorOnly.Articles))
	}

	none, err := svc.Search(context.Background(), appsvc.SearchRequest{
		Query: "golang", Author: "nobody", Limit: 20,
	})
	if err != nil {
		t.Fatalf("no-match search: %v", err)
	}
	if len(none.Articles) != 0 {
		t.Fatalf("no-match results = %d, want 0", len(none.Articles))
	}

}
