// Package worker processes transactional outbox events into Elasticsearch.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/kumparan/article-service/internal/domain/article"
	"github.com/kumparan/article-service/internal/platform/elasticsearch"
	"github.com/kumparan/article-service/internal/platform/metrics"
)

const (
	defaultLeaseTTL    = 5 * time.Minute
	defaultBackoffBase = time.Second
	defaultBackoffMax  = time.Minute
)

type articleLoader interface {
	GetByID(context.Context, int64) (*article.Article, error)
}

type authorLoader interface {
	GetByID(context.Context, int64) (*article.Author, error)
}

type outboxRepository interface {
	ClaimBatch(context.Context, int, string) ([]*article.OutboxEvent, error)
	MarkProcessed(context.Context, int64) error
	MarkFailed(context.Context, int64, string, interface{}) error
}

type indexer interface {
	IndexArticle(context.Context, *article.Article, *article.Author) error
}

type Config struct {
	BatchSize    int
	PollInterval time.Duration
	WorkerID     string
	LeaseTTL     time.Duration
	Metrics      *metrics.Registry
}

type Worker struct {
	articles     articleLoader
	authors      authorLoader
	outbox       outboxRepository
	indexer      indexer
	batchSize    int
	pollInterval time.Duration
	workerID     string
	leaseTTL     time.Duration
	backoffBase  time.Duration
	backoffMax   time.Duration
	random       func() float64
	now          func() time.Time
	logger       *slog.Logger
	metrics      *metrics.Registry
}

func New(
	cfg Config,
	articles articleLoader,
	authors authorLoader,
	outbox outboxRepository,
	indexer indexer,
	logger *slog.Logger,
) (*Worker, error) {
	if cfg.BatchSize < 1 {
		return nil, errors.New("worker: batch size must be positive")
	}
	if cfg.PollInterval <= 0 {
		return nil, errors.New("worker: poll interval must be positive")
	}
	if cfg.WorkerID == "" {
		return nil, errors.New("worker: worker ID is required")
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = defaultLeaseTTL
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		articles:     articles,
		authors:      authors,
		outbox:       outbox,
		indexer:      indexer,
		batchSize:    cfg.BatchSize,
		pollInterval: cfg.PollInterval,
		workerID:     cfg.WorkerID,
		leaseTTL:     cfg.LeaseTTL,
		backoffBase:  defaultBackoffBase,
		backoffMax:   defaultBackoffMax,
		random:       rand.Float64,
		now:          time.Now,
		logger:       logger,
		metrics:      cfg.Metrics,
	}, nil
}

// Run polls until ctx is canceled. A claimed batch is completed before return.
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		if err := w.processBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.logger.Error("outbox batch failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *Worker) processBatch(ctx context.Context) error {
	events, err := w.outbox.ClaimBatch(ctx, w.batchSize, w.workerID)
	if err != nil {
		return fmt.Errorf("claim batch: %w", err)
	}
	if len(events) == 0 {
		w.updatePending(ctx)
		return nil
	}

	batchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.leaseTTL)
	defer cancel()
	for _, event := range events {
		if err := w.processEvent(batchCtx, event); err != nil {
			w.logger.Error("outbox event failed", "event_id", event.ID, "article_id", event.AggregateID, "attempts", event.Attempts, "error_category", errorCategory(err), "error", err)
		}
	}
	w.updatePending(ctx)
	return nil
}

func (w *Worker) ProcessEvent(ctx context.Context, event *article.OutboxEvent) error {
	return w.processEvent(ctx, event)
}

func (w *Worker) processEvent(ctx context.Context, event *article.OutboxEvent) error {
	if event.AggregateType != "article" {
		return w.fail(ctx, event, fmt.Errorf("unsupported aggregate type %q", event.AggregateType))
	}

	a, err := w.articles.GetByID(ctx, event.AggregateID)
	if err != nil {
		if errors.Is(err, article.ErrNotFound) {
			if markErr := w.outbox.MarkProcessed(ctx, event.ID); markErr != nil {
				return fmt.Errorf("mark missing article processed: %w", markErr)
			}
			return nil
		}
		return w.fail(ctx, event, fmt.Errorf("load article: %w", err))
	}
	author, err := w.authors.GetByID(ctx, a.AuthorID)
	if err != nil {
		return w.fail(ctx, event, fmt.Errorf("load author: %w", err))
	}
	if err := w.indexer.IndexArticle(ctx, a, author); err != nil {
		return w.fail(ctx, event, err)
	}
	if err := w.outbox.MarkProcessed(ctx, event.ID); err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}
	w.logger.Info("outbox event processed", "event_id", event.ID, "article_id", event.AggregateID, "attempts", event.Attempts)
	return nil
}

type pendingCounter interface {
	PendingCount(context.Context) (int64, error)
}

func (w *Worker) updatePending(ctx context.Context) {
	if w.metrics == nil {
		return
	}
	counter, ok := w.outbox.(pendingCounter)
	if !ok {
		return
	}
	n, err := counter.PendingCount(ctx)
	if err != nil {
		w.logger.Warn("outbox pending count failed", "error_category", "database", "error", err)
		return
	}
	w.metrics.SetOutboxPending(n)
}

func errorCategory(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "context"
	}
	return "dependency"
}

func (w *Worker) fail(ctx context.Context, event *article.OutboxEvent, err error) error {
	delay := w.retryDelay(event.Attempts + 1)
	next := w.now().Add(delay)
	if markErr := w.outbox.MarkFailed(ctx, event.ID, err.Error(), next); markErr != nil {
		return fmt.Errorf("%w; mark failed: %v", err, markErr)
	}
	return err
}

func (w *Worker) retryDelay(attempt int) time.Duration {
	delay := w.backoffBase
	for i := 1; i < attempt && delay < w.backoffMax; i++ {
		delay *= 2
	}
	if delay > w.backoffMax {
		delay = w.backoffMax
	}
	return delay + time.Duration(float64(delay)*0.25*w.random())
}

var _ indexer = (*elasticsearch.Client)(nil)
