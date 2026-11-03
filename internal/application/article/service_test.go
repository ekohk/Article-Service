package article_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

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
	err error
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
func (f *fakeArticleRepo) List(_ context.Context, _ domain.ListParams) ([]*domain.Article, error) {
	return nil, nil
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
