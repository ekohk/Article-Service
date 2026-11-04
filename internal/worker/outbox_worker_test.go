package worker

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kumparan/article-service/internal/domain/article"
)

type fakeArticles struct {
	value *article.Article
	err   error
}

func (f fakeArticles) GetByID(context.Context, int64) (*article.Article, error) {
	return f.value, f.err
}

type fakeAuthors struct {
	value *article.Author
	err   error
}

func (f fakeAuthors) GetByID(context.Context, int64) (*article.Author, error) {
	return f.value, f.err
}

type fakeOutbox struct {
	processed int
	failed    int
	next      interface{}
	events    []*article.OutboxEvent
}

func (f *fakeOutbox) ClaimBatch(context.Context, int, string) ([]*article.OutboxEvent, error) {
	events := f.events
	f.events = nil
	return events, nil
}

func (f *fakeOutbox) MarkProcessed(context.Context, int64) error {
	f.processed++
	return nil
}

func (f *fakeOutbox) MarkFailed(_ context.Context, _ int64, _ string, next interface{}) error {
	f.failed++
	f.next = next
	return nil
}

type fakeIndexer struct {
	err   error
	count int
}

func (f *fakeIndexer) IndexArticle(context.Context, *article.Article, *article.Author) error {
	f.count++
	return f.err
}

func newTestWorker(articles articleLoader, authors authorLoader, outbox outboxRepository, indexer indexer) *Worker {
	w, err := New(Config{
		BatchSize:    1,
		PollInterval: time.Hour,
		WorkerID:     "test-worker",
	}, articles, authors, outbox, indexer, slog.New(slog.NewTextHandler(testWriter{}, nil)))
	if err != nil {
		panic(err)
	}
	w.random = func() float64 { return 0 }
	w.now = func() time.Time { return time.Unix(100, 0) }
	return w
}

type testWriter struct{}

func (testWriter) Write([]byte) (int, error) { return 0, nil }

func TestProcessEventIndexesAndMarksProcessed(t *testing.T) {
	outbox := &fakeOutbox{}
	indexer := &fakeIndexer{}
	w := newTestWorker(
		fakeArticles{value: &article.Article{ID: 7, AuthorID: 3, Version: 2}},
		fakeAuthors{value: &article.Author{ID: 3, Name: " Alice "}},
		outbox,
		indexer,
	)

	err := w.ProcessEvent(context.Background(), &article.OutboxEvent{
		ID: 11, AggregateType: "article", AggregateID: 7,
	})
	if err != nil {
		t.Fatalf("ProcessEvent() error = %v", err)
	}
	if indexer.count != 1 || outbox.processed != 1 || outbox.failed != 0 {
		t.Fatalf("unexpected calls: indexed=%d processed=%d failed=%d", indexer.count, outbox.processed, outbox.failed)
	}
}

func TestProcessEventMissingArticleMarksProcessed(t *testing.T) {
	outbox := &fakeOutbox{}
	w := newTestWorker(
		fakeArticles{err: article.ErrNotFound},
		fakeAuthors{},
		outbox,
		&fakeIndexer{},
	)

	err := w.ProcessEvent(context.Background(), &article.OutboxEvent{
		ID: 11, AggregateType: "article", AggregateID: 7,
	})
	if err != nil {
		t.Fatalf("ProcessEvent() error = %v", err)
	}
	if outbox.processed != 1 || outbox.failed != 0 {
		t.Fatalf("unexpected calls: processed=%d failed=%d", outbox.processed, outbox.failed)
	}
}

func TestProcessEventIndexFailureMarksFailedWithBackoff(t *testing.T) {
	outbox := &fakeOutbox{}
	w := newTestWorker(
		fakeArticles{value: &article.Article{ID: 7, AuthorID: 3}},
		fakeAuthors{value: &article.Author{ID: 3, Name: "Alice"}},
		outbox,
		&fakeIndexer{err: errors.New("temporary failure")},
	)
	event := &article.OutboxEvent{ID: 11, AggregateType: "article", AggregateID: 7, Attempts: 1}

	err := w.ProcessEvent(context.Background(), event)
	if err == nil {
		t.Fatal("ProcessEvent() error = nil, want failure")
	}

	next, ok := outbox.next.(time.Time)
	if !ok {
		t.Fatalf("next retry type = %T, want time.Time", outbox.next)
	}
	if outbox.failed != 1 || outbox.processed != 0 || !next.Equal(time.Unix(102, 0)) {
		t.Fatalf("unexpected retry state: failed=%d processed=%d next=%v", outbox.failed, outbox.processed, next)
	}
}

type blockingIndexer struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingIndexer) IndexArticle(context.Context, *article.Article, *article.Author) error {
	close(b.started)
	<-b.release
	return nil
}

func TestRunCompletesClaimedBatchAfterShutdown(t *testing.T) {
	outbox := &fakeOutbox{events: []*article.OutboxEvent{{ID: 11, AggregateType: "article", AggregateID: 7}}}
	indexer := &blockingIndexer{started: make(chan struct{}), release: make(chan struct{})}
	w := newTestWorker(
		fakeArticles{value: &article.Article{ID: 7, AuthorID: 3}},
		fakeAuthors{value: &article.Author{ID: 3, Name: "Alice"}},
		outbox,
		indexer,
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	<-indexer.started
	cancel()
	close(indexer.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not finish claimed batch after shutdown")
	}
	if outbox.processed != 1 {
		t.Fatalf("processed = %d, want 1", outbox.processed)
	}
}
