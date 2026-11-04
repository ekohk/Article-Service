package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/kumparan/article-service/internal/config"
	domain "github.com/kumparan/article-service/internal/domain/article"
	"github.com/kumparan/article-service/internal/platform/elasticsearch"
	pgrepo "github.com/kumparan/article-service/internal/repository/postgres"
	"github.com/kumparan/article-service/internal/worker"
)

func TestOutboxWorker_IntegrationIndexesLatestArticle(t *testing.T) {
	db := setupDB(t)
	authorID := seedAuthor(t, db, "Worker Integration Author")
	if _, err := db.ExecContext(context.Background(), `DELETE FROM outbox_events`); err != nil {
		t.Fatalf("clear outbox events: %v", err)
	}
	articleRepo := pgrepo.NewArticleRepository(db)
	authorRepo := pgrepo.NewAuthorRepository(db)
	outboxRepo := pgrepo.NewOutboxRepository(db, 5*time.Minute, 10)

	articleID := time.Now().UnixNano()
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO articles (id, author_id, title, body, version)
		VALUES ($1, $2, $3, $4, 1)
		RETURNING id`, articleID, authorID, "Worker Integration Article", "worker body").Scan(&articleID)
	if err != nil {
		t.Fatalf("insert article: %v", err)
	}
	t.Cleanup(func() {
		db.ExecContext(context.Background(), `DELETE FROM outbox_events WHERE aggregate_id = $1`, articleID)
		db.ExecContext(context.Background(), `DELETE FROM articles WHERE id = $1`, articleID)
	})
	_, err = db.ExecContext(context.Background(), `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload)
		VALUES ('article', $1, 'article.created', '{}'::jsonb)`, articleID)
	if err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}

	esURL := os.Getenv("ELASTICSEARCH_URL")
	if esURL == "" {
		t.Skip("ELASTICSEARCH_URL not set; skipping Elasticsearch integration test")
	}
	esClient, err := elasticsearch.New(config.ESConfig{URL: esURL, Index: "articles-v1"})
	if err != nil {
		t.Fatalf("create Elasticsearch client: %v", err)
	}
	if err := esClient.EnsureIndex(context.Background()); err != nil {
		t.Fatalf("ensure Elasticsearch index: %v", err)
	}
	w, err := worker.New(worker.Config{
		BatchSize: 1, PollInterval: time.Second, WorkerID: "integration-worker",
	}, articleRepo, authorRepo, outboxRepo, esClient, nil)
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	events, err := outboxRepo.ClaimBatch(context.Background(), 1, "integration-worker")
	if err != nil || len(events) != 1 {
		t.Fatalf("claim event: count=%d err=%v", len(events), err)
	}
	if err := w.ProcessEvent(context.Background(), events[0]); err != nil {
		t.Fatalf("process event: %v", err)
	}
	if _, err := esClient.Raw().Indices.Refresh(
		esClient.Raw().Indices.Refresh.WithIndex(esClient.Index()),
	); err != nil {
		t.Fatalf("refresh index: %v", err)
	}

	res, err := esClient.Raw().Get(esClient.Index(), strconv.FormatInt(articleID, 10))
	if err != nil {
		t.Fatalf("get indexed article: %v", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		t.Fatalf("get indexed article: %s", res)
	}
	var document struct {
		Source struct {
			ArticleID int64 `json:"article_id"`
			Version   int64 `json:"version"`
		} `json:"_source"`
	}
	if err := json.NewDecoder(res.Body).Decode(&document); err != nil {
		t.Fatalf("decode indexed article: %v", err)
	}
	if document.Source.ArticleID != articleID || document.Source.Version != 1 {
		t.Fatalf("indexed document = %+v, want article_id=%d version=1", document.Source, articleID)
	}

	var processed bool
	if err := db.QueryRowContext(context.Background(),
		`SELECT processed_at IS NOT NULL FROM outbox_events WHERE aggregate_id = $1`, articleID).Scan(&processed); err != nil {
		t.Fatalf("read processed event: %v", err)
	}
	if !processed {
		t.Fatal("outbox event was not marked processed")
	}

	if _, err := db.ExecContext(context.Background(), `
		UPDATE articles SET title = $1, version = 2, updated_at = NOW() WHERE id = $2`,
		"Worker Integration Article v2", articleID); err != nil {
		t.Fatalf("update article: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload)
		VALUES ('article', $1, 'article.updated', '{}'::jsonb)`, articleID); err != nil {
		t.Fatalf("insert update event: %v", err)
	}
	events, err = outboxRepo.ClaimBatch(context.Background(), 1, "integration-worker")
	if err != nil || len(events) != 1 {
		t.Fatalf("claim update event: count=%d err=%v", len(events), err)
	}
	if err := w.ProcessEvent(context.Background(), events[0]); err != nil {
		t.Fatalf("process update event: %v", err)
	}
	if _, err := esClient.Raw().Indices.Refresh(
		esClient.Raw().Indices.Refresh.WithIndex(esClient.Index()),
	); err != nil {
		t.Fatalf("refresh updated index: %v", err)
	}

	updated, err := articleRepo.GetByID(context.Background(), articleID)
	if err != nil {
		t.Fatalf("load updated article: %v", err)
	}
	author, err := authorRepo.GetByID(context.Background(), authorID)
	if err != nil {
		t.Fatalf("load author: %v", err)
	}
	if err := esClient.IndexArticle(context.Background(), &domain.Article{
		ID: articleID, AuthorID: authorID, Title: "stale", Body: "stale",
		Version: 1, CreatedAt: updated.CreatedAt, UpdatedAt: updated.UpdatedAt,
	}, author); err != nil {
		t.Fatalf("index stale article: %v", err)
	}
	if _, err := esClient.Raw().Indices.Refresh(
		esClient.Raw().Indices.Refresh.WithIndex(esClient.Index()),
	); err != nil {
		t.Fatalf("refresh stale index: %v", err)
	}
	res, err = esClient.Raw().Get(esClient.Index(), strconv.FormatInt(articleID, 10))
	if err != nil {
		t.Fatalf("get updated indexed article: %v", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		t.Fatalf("get updated indexed article: %s", res)
	}
	document = struct {
		Source struct {
			ArticleID int64 `json:"article_id"`
			Version   int64 `json:"version"`
		} `json:"_source"`
	}{}
	if err := json.NewDecoder(res.Body).Decode(&document); err != nil {
		t.Fatalf("decode updated indexed article: %v", err)
	}
	if document.Source.Version != 2 {
		t.Fatalf("stale event overwrote version: got %d, want 2", document.Source.Version)
	}

}
