package article_test

import (
	"context"
	"database/sql"
	"errors"
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

func (f *fakeAuthorRepo) GetByID(_ context.Context, _ int64) (*domain.Author, error) {
	return f.author, f.err
}
func (f *fakeAuthorRepo) GetIDsByNormalizedName(_ context.Context, _ string) ([]int64, error) {
	return nil, nil
}

type fakeArticleRepo struct {
	err      error
	list     []*domain.Article
	lastList domain.ListParams
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
	return a, nil
}
func (f *fakeArticleRepo) GetByID(_ context.Context, _ int64) (*domain.Article, error) {
	return nil, nil
}
func (f *fakeArticleRepo) GetByIDs(_ context.Context, _ []int64) ([]*domain.Article, error) {
	return nil, nil
}
func (f *fakeArticleRepo) List(_ context.Context, p domain.ListParams) ([]*domain.Article, error) {
	f.lastList = p
	if f.err != nil {
		return nil, f.err
	}
	return f.list, nil
}

type fakeOutboxRepo struct {
	inserted bool
	err      error
}

func (f *fakeOutboxRepo) Insert(_ context.Context, _ *sql.Tx, _ *domain.OutboxEvent) error {
	if f.err != nil {
		return f.err
	}
	f.inserted = true
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
