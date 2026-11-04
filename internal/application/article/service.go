// Package article contains the application service that orchestrates use cases.
// It sits between HTTP handlers and domain/repository layers.
// It must not import Gin or any HTTP-specific types.
package article

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	domain "github.com/kumparan/article-service/internal/domain/article"
)

// Transactor executes fn inside a PostgreSQL transaction.
// On fn error the transaction is rolled back; on success it is committed.
// This interface lets unit tests bypass the real database entirely.
type Transactor interface {
	RunInTx(ctx context.Context, fn func(tx *sql.Tx) error) error
}

// sqlTransactor implements Transactor using a *sql.DB.
type sqlTransactor struct{ db *sql.DB }

func (s *sqlTransactor) RunInTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Service orchestrates article use cases.
type Service struct {
	transactor Transactor
	articles   domain.ArticleRepository
	authors    domain.AuthorRepository
	outbox     domain.OutboxRepository
}

// New creates an article Service backed by a real *sql.DB.
func New(
	db *sql.DB,
	articles domain.ArticleRepository,
	authors domain.AuthorRepository,
	outbox domain.OutboxRepository,
) *Service {
	return &Service{
		transactor: &sqlTransactor{db: db},
		articles:   articles,
		authors:    authors,
		outbox:     outbox,
	}
}

// NewWithTransactor creates a Service with an injectable Transactor (for tests).
func NewWithTransactor(
	t Transactor,
	articles domain.ArticleRepository,
	authors domain.AuthorRepository,
	outbox domain.OutboxRepository,
) *Service {
	return &Service{transactor: t, articles: articles, authors: authors, outbox: outbox}
}

// CreateRequest carries input for creating an article.
type CreateRequest struct {
	AuthorID int64
	Title    string
	Body     string
}

// Create validates input, verifies the author exists, inserts the article and
// its outbox event in a single transaction, and returns the persisted article.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*domain.Article, error) {
	// Domain validation (D-03 limits, business rules).
	if err := domain.ValidateCreate(req.AuthorID, req.Title, req.Body); err != nil {
		return nil, err
	}

	// Verify the author exists before opening a transaction.
	if _, err := s.authors.GetByID(ctx, req.AuthorID); err != nil {
		return nil, err
	}

	var saved *domain.Article

	err := s.transactor.RunInTx(ctx, func(tx *sql.Tx) error {
		var err error
		saved, err = s.articles.Insert(ctx, tx, &domain.Article{
			AuthorID: req.AuthorID,
			Title:    req.Title,
			Body:     req.Body,
		})
		if err != nil {
			return fmt.Errorf("insert article: %w", err)
		}

		payload, err := buildEventPayload(saved)
		if err != nil {
			return fmt.Errorf("build payload: %w", err)
		}

		return s.outbox.Insert(ctx, tx, &domain.OutboxEvent{
			AggregateType: "article",
			AggregateID:   saved.ID,
			EventType:     domain.EventTypeArticleCreated,
			Payload:       payload,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create article: %w", err)
	}

	return saved, nil
}

// ListRequest carries pagination input for GET /articles (PostgreSQL listing).
type ListRequest struct {
	Author string
	Limit  int
	Cursor string
}

// ListResult is a page of articles plus keyset pagination metadata.
type ListResult struct {
	Articles   []*domain.Article
	Limit      int
	NextCursor *string
	HasMore    bool
}

// List returns a newest-first page of articles from PostgreSQL.
// Keyword search and author filters are out of scope for IMPL-08.
func (s *Service) List(ctx context.Context, req ListRequest) (*ListResult, error) {
	if err := domain.ValidateLimit(req.Limit); err != nil {
		return nil, err
	}
	authorName := domain.NormalizeAuthorName(req.Author)
	cursor, err := domain.DecodeCursor(req.Cursor)
	if err != nil {
		return nil, err
	}
	if cursor != nil && cursor.AuthorName != authorName {
		return nil, fmt.Errorf("%w: cursor does not match author filter", domain.ErrInvalidCursor)
	}

	rows, err := s.articles.List(ctx, domain.ListParams{
		AuthorName: authorName,
		Limit:      req.Limit,
		Cursor:     cursor,
	})
	if err != nil {
		return nil, fmt.Errorf("list articles: %w", err)
	}

	hasMore := len(rows) > req.Limit
	if hasMore {
		rows = rows[:req.Limit]
	}
	if rows == nil {
		rows = []*domain.Article{}
	}

	result := &ListResult{
		Articles: rows,
		Limit:    req.Limit,
		HasMore:  hasMore,
	}
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		token, err := domain.EncodeCursor(domain.Cursor{
			CreatedAt:  last.CreatedAt.UTC().UnixNano(),
			ID:         last.ID,
			AuthorName: authorName,
		})
		if err != nil {
			return nil, err
		}
		result.NextCursor = &token
	}
	return result, nil
}

// eventPayload is the minimal outbox payload; worker loads article by ID (D-02).
type eventPayload struct {
	ArticleID  int64     `json:"article_id"`
	AuthorID   int64     `json:"author_id"`
	Version    int64     `json:"version"`
	OccurredAt time.Time `json:"occurred_at"`
}

func buildEventPayload(a *domain.Article) ([]byte, error) {
	return json.Marshal(eventPayload{
		ArticleID:  a.ID,
		AuthorID:   a.AuthorID,
		Version:    a.Version,
		OccurredAt: a.CreatedAt,
	})
}
