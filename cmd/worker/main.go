package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kumparan/article-service/internal/config"
	"github.com/kumparan/article-service/internal/platform/elasticsearch"
	"github.com/kumparan/article-service/internal/platform/logger"
	"github.com/kumparan/article-service/internal/platform/metrics"
	"github.com/kumparan/article-service/internal/platform/postgres"
	pgrepo "github.com/kumparan/article-service/internal/repository/postgres"
	"github.com/kumparan/article-service/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:")
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	log := logger.New(cfg.AppEnv, os.Getenv("LOG_LEVEL"))
	registry := &metrics.Registry{}
	log.Info("article-service worker starting", "env", cfg.AppEnv)

	esClient, err := elasticsearch.New(cfg.ES)
	if err != nil {
		log.Error("failed to create Elasticsearch client", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := esClient.Ping(ctx); err != nil {
		log.Error("failed to connect to Elasticsearch", "error", err)
		os.Exit(1)
	}
	if err := esClient.EnsureIndex(ctx); err != nil {
		log.Error("failed to ensure Elasticsearch index", "error", err)
		os.Exit(1)
	}
	log.Info("Elasticsearch index ready", "index", esClient.Index(), "alias", esClient.Alias())

	db, err := postgres.Open(ctx, cfg.DB)
	if err != nil {
		log.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	articleRepo := pgrepo.NewArticleRepository(db)
	authorRepo := pgrepo.NewAuthorRepository(db)
	outboxRepo := pgrepo.NewOutboxRepository(db, 5*time.Minute, cfg.Outbox.MaxAttempts)
	outboxWorker, err := worker.New(worker.Config{
		BatchSize:    cfg.Outbox.BatchSize,
		PollInterval: cfg.Outbox.PollInterval,
		WorkerID:     fmt.Sprintf("worker-%d", os.Getpid()),
		LeaseTTL:     5 * time.Minute,
		Metrics:      registry,
	}, articleRepo, authorRepo, outboxRepo, esClient, log)
	if err != nil {
		log.Error("failed to create outbox worker", "error", err)
		os.Exit(1)
	}

	log.Info("outbox worker started")
	if err := outboxWorker.Run(ctx); err != nil {
		log.Error("outbox worker stopped with error", "error", err)
		os.Exit(1)
	}
	log.Info("outbox worker stopped")
}
