package article_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	appsvc "github.com/kumparan/article-service/internal/application/article"
	domain "github.com/kumparan/article-service/internal/domain/article"
)

// ── fakes ─────────────────────────────────────────────────────────────────────

// fakeTransactor calls fn with a nil *sql.Tx (safe because fake repos ignore it).
type fakeTransactor struct{ err error }

func (f *fakeTransactor) RunInTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if f.err != nil {
		return f.err
	}
	return fn(nil) // fakes accept nil tx
}

type fakeAuthorRepo struct {
	author *domain.Author
	err    error
}

func (f *fakeAuthorRepo) Create(_ context.Context, name string) (*domain.Author, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &domain.Author{ID: 1, Name: name, CreatedAt: time.Now()}, nil
}

func (f *fakeAuthorRepo) List(context.Context) ([]*domain.Author, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.author == nil {
		return []*domain.Author{}, nil
	}
	return []*domain.Author{f.author}, nil
}

func (f *fakeAuthorRepo) GetByID(_ context.Context, _ int64) (*domain.Author, error) {
	return f.author, f.err
}
func (f *fakeAuthorRepo) GetIDsByNormalizedName(_ context.Context, _ string) ([]int64, error) {
	return nil, nil
}

type fakeArticleRepo struct {
	err       error
	list      []*domain.Article
	lastList  domain.ListParams
	byIDs     []*domain.Article
	byID      *domain.Article
	updated   *domain.Article
	listCalls int
	listBlock <-chan struct{}
}

func (f *fakeArticleRepo) Insert(_ context.Context, _ *sql.Tx, a *domain.Article) (*domain.Article, error) {
	if f.err != nil {
		return nil, f.err
	}
	result := *a
	result.ID = 42
	result.Version = 1
	return &result, nil
}
func (f *fakeArticleRepo) Update(_ context.Context, _ *sql.Tx, a *domain.Article) (*domain.Article, error) {
	if f.err != nil {
		return nil, f.err
	}
	result := *a
	result.Version++
	f.updated = &result
	return &result, nil
}
func (f *fakeArticleRepo) GetByID(_ context.Context, _ int64) (*domain.Article, error) {
	return f.byID, nil
}
func (f *fakeArticleRepo) GetByIDs(_ context.Context, _ []int64) ([]*domain.Article, error) {
	return f.byIDs, nil
}
func (f *fakeArticleRepo) List(_ context.Context, p domain.ListParams) ([]*domain.Article, error) {
	f.listCalls++
	if f.listBlock != nil {
		<-f.listBlock
	}
	f.lastList = p
	if f.err != nil {
		return nil, f.err
	}
	return f.list, nil
}

type fakeCache struct {
	values map[string][]byte
	gets   int
	sets   int
	getErr error
}

func (f *fakeCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	f.gets++
	if f.getErr != nil {
		return nil, false, f.getErr
	}
	value, ok := f.values[key]
	return value, ok, nil
}
func (f *fakeCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	if f.values == nil {
		f.values = make(map[string][]byte)
	}
	f.values[key] = value
	f.sets++
	return nil
}

type fakeOutboxRepo struct {
	inserted bool
	err      error
	event    *domain.OutboxEvent
}

func (f *fakeOutboxRepo) Insert(_ context.Context, _ *sql.Tx, event *domain.OutboxEvent) error {
	if f.err != nil {
		return f.err
	}
	f.inserted = true
	f.event = event
	return nil
}
func (f *fakeOutboxRepo) ClaimBatch(_ context.Context, _ int, _ string) ([]*domain.OutboxEvent, error) {
	return nil, nil
}
func (f *fakeOutboxRepo) MarkProcessed(_ context.Context, _ int64) error { return nil }
func (f *fakeOutboxRepo) MarkFailed(_ context.Context, _ int64, _ string, _ interface{}) error {
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func validAuthor() *domain.Author { return &domain.Author{ID: 1, Name: "Budi"} }

func newSvc(tr appsvc.Transactor, art domain.ArticleRepository, aut domain.AuthorRepository, out domain.OutboxRepository) *appsvc.Service {
	return appsvc.NewWithTransactor(tr, art, aut, out)
}

type fakeSearcher struct {
	ids    []int64
	query  string
	author string
	calls  int
}

type fakePageSearcher struct{ err error }

type fakeMetrics struct {
	hits, misses, searches int
}

func (m *fakeMetrics) IncCacheHit()                { m.hits++ }
func (m *fakeMetrics) IncCacheMiss()               { m.misses++ }
func (m *fakeMetrics) ObserveSearch(time.Duration) { m.searches++ }

func (f *fakePageSearcher) Search(context.Context, string, string, int) ([]int64, error) {
	return nil, f.err
}
func (f *fakePageSearcher) SearchPage(context.Context, string, string, string, []json.RawMessage, int) (domain.SearchPage, error) {
	return domain.SearchPage{}, f.err
}
func (*fakePageSearcher) ClosePIT(context.Context, string) error { return nil }

func searchCursorForTest(t *testing.T) string {
	t.Helper()
	sum := sha256.Sum256([]byte("query\x00author"))
	raw, err := json.Marshal(map[string]any{
		"k": "es_search", "p": "pit-1", "s": []json.RawMessage{json.RawMessage(`1`),
			json.RawMessage(`2`)}, "f": fmt.Sprintf("%x", sum),
	})
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func (f *fakeSearcher) Search(_ context.Context, query, author string, _ int) ([]int64, error) {
	f.calls++
	f.query = query
	f.author = author
	return f.ids, nil
}

// ── T1: valid request ─────────────────────────────────────────────────────────

func TestCreate_ValidRequest(t *testing.T) {
	outbox := &fakeOutboxRepo{}
	svc := newSvc(&fakeTransactor{}, &fakeArticleRepo{}, &fakeAuthorRepo{author: validAuthor()}, outbox)

	result, err := svc.Create(context.Background(), appsvc.CreateRequest{
		AuthorID: 1, Title: "Hello World", Body: "Body content.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.ID != 42 {
		t.Errorf("expected id=42, got %v", result)
	}
	if !outbox.inserted {
		t.Error("outbox event was not inserted")
	}
}

func TestSearchHydratesAndPreservesSearchOrder(t *testing.T) {
	searcher := &fakeSearcher{ids: []int64{2, 1}}
	articles := &fakeArticleRepo{byIDs: []*domain.Article{
		{ID: 1, Title: "one"},
		{ID: 2, Title: "two"},
	}}
	svc := appsvc.NewWithTransactor(
		&fakeTransactor{}, articles, &fakeAuthorRepo{}, &fakeOutboxRepo{}, searcher,
	)

	result, err := svc.Search(context.Background(), appsvc.SearchRequest{
		Query: "programming", Author: "  BUDI ", Limit: 20,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if searcher.query != "programming" || searcher.author != "budi" || len(result.Articles) != 2 ||
		result.Articles[0].ID != 2 || result.Articles[1].ID != 1 {
		t.Fatalf("unexpected search result: %+v", result.Articles)
	}

}

func TestUpdateTitleOnlyWritesUpdatedArticleAndOutbox(t *testing.T) {
	title := "New Title"
	articles := &fakeArticleRepo{byID: &domain.Article{
		ID: 7, AuthorID: 1, Title: "Old", Body: "Keep", Version: 2,
	}}
	outbox := &fakeOutboxRepo{}
	svc := appsvc.NewWithTransactor(&fakeTransactor{}, articles, &fakeAuthorRepo{}, outbox)
	updated, err := svc.Update(context.Background(), appsvc.UpdateRequest{ID: 7, Title: &title})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != title || updated.Body != "Keep" || updated.Version != 3 {
		t.Fatalf("updated = %+v", updated)
	}
	if !outbox.inserted || outbox.event.EventType != domain.EventTypeArticleUpdated {
		t.Fatalf("outbox event = %+v", outbox.event)
	}
}

func TestUpdateRejectsEmptyPayload(t *testing.T) {
	svc := appsvc.NewWithTransactor(&fakeTransactor{}, &fakeArticleRepo{}, &fakeAuthorRepo{}, &fakeOutboxRepo{})
	_, err := svc.Update(context.Background(), appsvc.UpdateRequest{ID: 1})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("error = %v", err)
	}

}

func TestListCacheHitSkipsRepository(t *testing.T) {
	cache := &fakeCache{values: map[string][]byte{}}
	articles := &fakeArticleRepo{list: []*domain.Article{{ID: 1, Title: "cached"}}}
	svc := appsvc.NewWithSearch(nil, articles, &fakeAuthorRepo{}, &fakeOutboxRepo{}, &fakeSearcher{}, nil,
		appsvc.CacheConfig{Store: cache, TTL: time.Minute})
	first, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if articles.listCalls != 1 || cache.sets != 1 || first.Articles[0].ID != second.Articles[0].ID {
		t.Fatalf("calls=%d sets=%d first=%+v second=%+v", articles.listCalls, cache.sets, first, second)
	}
}

func TestListCacheMissUsesSingleflight(t *testing.T) {
	cache := &fakeCache{values: map[string][]byte{}}
	release := make(chan struct{})
	articles := &fakeArticleRepo{list: []*domain.Article{{ID: 1}}, listBlock: release}
	svc := appsvc.NewWithSearch(nil, articles, &fakeAuthorRepo{}, &fakeOutboxRepo{}, &fakeSearcher{}, nil,
		appsvc.CacheConfig{Store: cache, TTL: time.Minute})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 10})
			results <- err
		}()
	}
	time.Sleep(25 * time.Millisecond)
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if articles.listCalls != 1 {
		t.Fatalf("source calls = %d, want 1", articles.listCalls)
	}
}

func TestListCacheFailureFallsBackToSource(t *testing.T) {
	cache := &fakeCache{getErr: errors.New("redis unavailable")}
	articles := &fakeArticleRepo{list: []*domain.Article{{ID: 1}}}
	svc := appsvc.NewWithSearch(nil, articles, &fakeAuthorRepo{}, &fakeOutboxRepo{}, &fakeSearcher{}, nil,
		appsvc.CacheConfig{Store: cache, TTL: time.Minute})
	if _, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if articles.listCalls != 1 {
		t.Fatalf("source calls = %d, want 1", articles.listCalls)
	}
}

func TestSearchCacheHitSkipsSearcher(t *testing.T) {
	cache := &fakeCache{values: map[string][]byte{}}
	searcher := &fakeSearcher{ids: []int64{1}}
	articles := &fakeArticleRepo{byIDs: []*domain.Article{{ID: 1, Title: "result"}}}
	metrics := &fakeMetrics{}
	svc := appsvc.NewWithSearch(nil, articles, &fakeAuthorRepo{}, &fakeOutboxRepo{}, searcher, nil,
		appsvc.CacheConfig{Store: cache, TTL: time.Minute, Metrics: metrics})
	first, err := svc.Search(context.Background(), appsvc.SearchRequest{Query: "result", Author: "Budi", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Search(context.Background(), appsvc.SearchRequest{Query: "result", Author: "Budi", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if searcher.calls != 1 || metrics.misses == 0 || metrics.hits == 0 ||
		metrics.searches != 1 || first.Articles[0].ID != second.Articles[0].ID {
		t.Fatalf("search calls=%d first=%+v second=%+v", searcher.calls, first, second)
	}
}

func TestSearchPreservesNonPITErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"timeout", context.DeadlineExceeded},
		{"cancellation", context.Canceled},
		{"network", errors.New("connection refused")},
		{"server", errors.New("elasticsearch: search: status 503")},
		{"malformed response", errors.New("elasticsearch: decode search response: unexpected EOF")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := appsvc.NewWithTransactor(
				&fakeTransactor{}, &fakeArticleRepo{}, &fakeAuthorRepo{}, &fakeOutboxRepo{},
				&fakePageSearcher{err: tt.err},
			)
			_, err := svc.Search(context.Background(), appsvc.SearchRequest{
				Query: "query", Author: "author", Limit: 10, Cursor: searchCursorForTest(t),
			})
			if errors.Is(err, domain.ErrInvalidCursor) {
				t.Fatalf("error incorrectly classified as invalid cursor: %v", err)
			}
			if !errors.Is(err, tt.err) {
				t.Fatalf("error = %v, want wrapped %v", err, tt.err)
			}
		})
	}
}

func TestSearchMapsOnlyExpiredPITToInvalidCursor(t *testing.T) {
	svc := appsvc.NewWithTransactor(
		&fakeTransactor{}, &fakeArticleRepo{}, &fakeAuthorRepo{}, &fakeOutboxRepo{},
		&fakePageSearcher{err: domain.ErrPITExpired},
	)
	_, err := svc.Search(context.Background(), appsvc.SearchRequest{
		Query: "query", Author: "author", Limit: 10, Cursor: searchCursorForTest(t),
	})
	if !errors.Is(err, domain.ErrInvalidCursor) {
		t.Fatalf("error = %v, want ErrInvalidCursor", err)
	}
}

// ── T2: author_id = 0 ─────────────────────────────────────────────────────────

func TestCreate_ZeroAuthorID(t *testing.T) {
	svc := newSvc(&fakeTransactor{}, &fakeArticleRepo{}, &fakeAuthorRepo{author: validAuthor()}, &fakeOutboxRepo{})
	_, err := svc.Create(context.Background(), appsvc.CreateRequest{AuthorID: 0, Title: "t", Body: "b"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

// ── T3: blank title ───────────────────────────────────────────────────────────

func TestCreate_BlankTitle(t *testing.T) {
	svc := newSvc(&fakeTransactor{}, &fakeArticleRepo{}, &fakeAuthorRepo{author: validAuthor()}, &fakeOutboxRepo{})
	_, err := svc.Create(context.Background(), appsvc.CreateRequest{AuthorID: 1, Title: "   ", Body: "b"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

// ── T4: blank body ────────────────────────────────────────────────────────────

func TestCreate_BlankBody(t *testing.T) {
	svc := newSvc(&fakeTransactor{}, &fakeArticleRepo{}, &fakeAuthorRepo{author: validAuthor()}, &fakeOutboxRepo{})
	_, err := svc.Create(context.Background(), appsvc.CreateRequest{AuthorID: 1, Title: "title", Body: ""})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

// ── T5: author not found ──────────────────────────────────────────────────────

func TestCreate_AuthorNotFound(t *testing.T) {
	svc := newSvc(&fakeTransactor{}, &fakeArticleRepo{}, &fakeAuthorRepo{err: domain.ErrAuthorNotFound}, &fakeOutboxRepo{})
	_, err := svc.Create(context.Background(), appsvc.CreateRequest{AuthorID: 99, Title: "t", Body: "b"})
	if !errors.Is(err, domain.ErrAuthorNotFound) {
		t.Errorf("expected ErrAuthorNotFound, got %v", err)
	}
}

// ── T6: outbox insert failure returns error ───────────────────────────────────

func TestCreate_OutboxInsertFailure(t *testing.T) {
	svc := newSvc(&fakeTransactor{}, &fakeArticleRepo{},
		&fakeAuthorRepo{author: validAuthor()},
		&fakeOutboxRepo{err: errors.New("db error")})
	_, err := svc.Create(context.Background(), appsvc.CreateRequest{AuthorID: 1, Title: "t", Body: "b"})
	if err == nil {
		t.Fatal("expected error when outbox insert fails, got nil")
	}
}

// ── T7: transactor failure ────────────────────────────────────────────────────

func TestCreate_TransactorFailure(t *testing.T) {
	svc := newSvc(&fakeTransactor{err: errors.New("connection lost")},
		&fakeArticleRepo{}, &fakeAuthorRepo{author: validAuthor()}, &fakeOutboxRepo{})
	_, err := svc.Create(context.Background(), appsvc.CreateRequest{AuthorID: 1, Title: "t", Body: "b"})
	if err == nil {
		t.Fatal("expected error when transactor fails, got nil")
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestList_LimitZero(t *testing.T) {
	svc := newSvc(&fakeTransactor{}, &fakeArticleRepo{}, &fakeAuthorRepo{}, &fakeOutboxRepo{})
	_, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 0})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestList_LimitTooHigh(t *testing.T) {
	svc := newSvc(&fakeTransactor{}, &fakeArticleRepo{}, &fakeAuthorRepo{}, &fakeOutboxRepo{})
	_, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 101})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestList_InvalidCursor(t *testing.T) {
	svc := newSvc(&fakeTransactor{}, &fakeArticleRepo{}, &fakeAuthorRepo{}, &fakeOutboxRepo{})
	_, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 20, Cursor: "%%%"})
	if !errors.Is(err, domain.ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor, got %v", err)
	}
}

func TestList_Empty(t *testing.T) {
	repo := &fakeArticleRepo{list: []*domain.Article{}}
	svc := newSvc(&fakeTransactor{}, repo, &fakeAuthorRepo{}, &fakeOutboxRepo{})
	result, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Articles) != 0 {
		t.Errorf("len = %d, want 0", len(result.Articles))
	}
	if result.HasMore || result.NextCursor != nil {
		t.Error("empty page must not have more results")
	}
}

func TestList_HasMoreAndCursor(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0).UTC()
	t1 := t0.Add(-time.Second)
	t2 := t0.Add(-2 * time.Second)
	repo := &fakeArticleRepo{list: []*domain.Article{
		{ID: 3, Title: "newest", CreatedAt: t0},
		{ID: 2, Title: "middle", CreatedAt: t1},
		{ID: 1, Title: "oldest", CreatedAt: t2},
	}}
	svc := newSvc(&fakeTransactor{}, repo, &fakeAuthorRepo{}, &fakeOutboxRepo{})

	result, err := svc.List(context.Background(), appsvc.ListRequest{Author: " Budi ", Limit: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.HasMore {
		t.Error("expected has_more")
	}
	if result.NextCursor == nil {
		t.Fatal("expected next_cursor")
	}
	if len(result.Articles) != 2 {
		t.Fatalf("len = %d, want 2", len(result.Articles))
	}
	if result.Articles[0].ID != 3 || result.Articles[1].ID != 2 {
		t.Errorf("unexpected page ids: %d, %d", result.Articles[0].ID, result.Articles[1].ID)
	}
	decoded, err := domain.DecodeCursor(*result.NextCursor)
	if err != nil {
		t.Fatalf("decode next_cursor: %v", err)
	}
	if decoded.ID != 2 || decoded.CreatedAt != t1.UnixNano() || decoded.AuthorName != "budi" {
		t.Errorf("cursor = %+v, want id=2 created_at=%d", decoded, t1.UnixNano())
	}
}

func TestList_NormalizesAuthorFilter(t *testing.T) {
	repo := &fakeArticleRepo{list: []*domain.Article{}}
	svc := newSvc(&fakeTransactor{}, repo, &fakeAuthorRepo{}, &fakeOutboxRepo{})

	_, err := svc.List(context.Background(), appsvc.ListRequest{Author: "  B\u00dcDI  ", Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastList.AuthorName != "b\u00fcdi" {
		t.Errorf("author filter = %q, want %q", repo.lastList.AuthorName, "b\u00fcdi")
	}
}

func TestList_RejectsCursorFromDifferentAuthorFilter(t *testing.T) {
	token, err := domain.EncodeCursor(domain.Cursor{
		CreatedAt:  1_700_000_000_000_000_000,
		ID:         42,
		AuthorName: "budi",
	})
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}

	svc := newSvc(&fakeTransactor{}, &fakeArticleRepo{}, &fakeAuthorRepo{}, &fakeOutboxRepo{})
	_, err = svc.List(context.Background(), appsvc.ListRequest{Author: "Siti", Limit: 20, Cursor: token})
	if !errors.Is(err, domain.ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor, got %v", err)
	}
}
