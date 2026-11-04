// Package article contains the application service that orchestrates use cases.
// It sits between HTTP handlers and domain/repository layers.
// It must not import Gin or any HTTP-specific types.
package article

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

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
	search     Searcher
	logger     *slog.Logger
	cache      Cache
	cacheTTL   time.Duration
	cacheGroup singleflight.Group
	metrics    Metrics
}

type Cache interface {
	Get(context.Context, string) ([]byte, bool, error)
	Set(context.Context, string, []byte, time.Duration) error
}

type Metrics interface {
	IncCacheHit()
	IncCacheMiss()
	ObserveSearch(time.Duration)
}

// Searcher returns article IDs in Elasticsearch relevance order.
type Searcher interface {
	Search(ctx context.Context, query, authorName string, limit int) ([]int64, error)
}

type pageSearcher interface {
	SearchPage(ctx context.Context, query, authorName, pitID string, searchAfter []json.RawMessage, limit int) (domain.SearchPage, error)
	ClosePIT(ctx context.Context, pitID string) error
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
		logger:     slog.Default(),
	}
}

// NewWithSearch creates a Service with an Elasticsearch-backed searcher.
func NewWithSearch(
	db *sql.DB,
	articles domain.ArticleRepository,
	authors domain.AuthorRepository,
	outbox domain.OutboxRepository,
	search Searcher,
	logger *slog.Logger,
	caches ...CacheConfig,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	service := &Service{
		transactor: &sqlTransactor{db: db},
		articles:   articles,
		authors:    authors,
		outbox:     outbox,
		search:     search,
		logger:     logger,
	}
	if len(caches) > 0 {
		service.cache = caches[0].Store
		service.cacheTTL = caches[0].TTL
		service.metrics = caches[0].Metrics
	}
	return service
}

type CacheConfig struct {
	Store   Cache
	TTL     time.Duration
	Metrics Metrics
}

// NewWithTransactor creates a Service with an injectable Transactor (for tests).
func NewWithTransactor(
	t Transactor,
	articles domain.ArticleRepository,
	authors domain.AuthorRepository,
	outbox domain.OutboxRepository,
	searchers ...Searcher,
) *Service {
	service := &Service{
		transactor: t,
		articles:   articles,
		authors:    authors,
		outbox:     outbox,
		logger:     slog.Default(),
	}
	if len(searchers) > 0 {
		service.search = searchers[0]
	}
	return service
}

// CreateRequest carries input for creating an article.
type CreateRequest struct {
	AuthorID int64
	Title    string
	Body     string
}

type UpdateRequest struct {
	ID    int64
	Title *string
	Body  *string
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

func (s *Service) Update(ctx context.Context, req UpdateRequest) (*domain.Article, error) {
	if req.ID <= 0 {
		return nil, fmt.Errorf("%w: article id must be positive", domain.ErrInvalidInput)
	}
	if err := domain.ValidateUpdate(req.Title, req.Body); err != nil {
		return nil, err
	}
	current, err := s.articles.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if req.Title != nil {
		current.Title = *req.Title
	}
	if req.Body != nil {
		current.Body = *req.Body
	}

	var updated *domain.Article
	err = s.transactor.RunInTx(ctx, func(tx *sql.Tx) error {
		updated, err = s.articles.Update(ctx, tx, current)
		if err != nil {
			return fmt.Errorf("update article: %w", err)
		}
		payload, err := buildEventPayload(updated)
		if err != nil {
			return fmt.Errorf("build update payload: %w", err)
		}
		return s.outbox.Insert(ctx, tx, &domain.OutboxEvent{
			AggregateType: "article",
			AggregateID:   updated.ID,
			EventType:     domain.EventTypeArticleUpdated,
			Payload:       payload,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("update article: %w", err)
	}
	return updated, nil
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
func cacheKey(query, author string, limit int, cursor string) string {
	return "articles:v1:q=" + url.QueryEscape(query) +
		"&author=" + url.QueryEscape(author) +
		"&limit=" + strconv.Itoa(limit) +
		"&cursor=" + url.QueryEscape(cursor)
}

func (s *Service) cached(ctx context.Context, key string) (*ListResult, bool) {
	if s.cache == nil {
		return nil, false
	}
	raw, found, err := s.cache.Get(ctx, key)
	if err != nil {
		s.logger.Warn("cache get failed; using source", "key", key, "error", err)
		return nil, false
	}
	if !found {
		if s.metrics != nil {
			s.metrics.IncCacheMiss()
		}
		return nil, false
	}
	var result ListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		s.logger.Warn("cache decode failed; using source", "key", key, "error", err)
		return nil, false
	}
	if s.metrics != nil {
		s.metrics.IncCacheHit()
	}
	return &result, true
}

func (s *Service) storeCached(ctx context.Context, key string, result *ListResult) {
	if s.cache == nil || s.cacheTTL <= 0 {
		return
	}
	raw, err := json.Marshal(result)
	if err != nil {
		s.logger.Warn("cache encode failed", "key", key, "error", err)
		return
	}
	if err := s.cache.Set(ctx, key, raw, s.cacheTTL); err != nil {
		s.logger.Warn("cache set failed", "key", key, "error", err)
	}
}

func (s *Service) List(ctx context.Context, req ListRequest) (*ListResult, error) {
	if err := domain.ValidateLimit(req.Limit); err != nil {
		return nil, err
	}

	authorName := domain.NormalizeAuthorName(req.Author)
	key := cacheKey("", authorName, req.Limit, req.Cursor)
	if result, ok := s.cached(ctx, key); ok {
		return result, nil
	}
	var result *ListResult
	var err error
	value, err, _ := s.cacheGroup.Do(key, func() (any, error) {
		if cached, ok := s.cached(ctx, key); ok {
			return cached, nil
		}
		result, err = s.listSource(ctx, req, authorName)
		if err != nil {
			return nil, err
		}
		s.storeCached(ctx, key, result)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*ListResult), nil
}

func (s *Service) listSource(ctx context.Context, req ListRequest, authorName string) (*ListResult, error) {
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

// SearchRequest carries keyword and optional normalized author filter input.
type SearchRequest struct {
	Query  string
	Author string
	Limit  int
	Cursor string
}

// Search returns current PostgreSQL article rows in Elasticsearch order.
// Search visibility is eventually consistent because Elasticsearch is a
// derived projection.
func (s *Service) Search(ctx context.Context, req SearchRequest) (*ListResult, error) {
	if err := domain.ValidateLimit(req.Limit); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Query) == "" {
		return s.List(ctx, ListRequest{Author: req.Author, Limit: req.Limit})
	}
	query := strings.TrimSpace(req.Query)
	author := domain.NormalizeAuthorName(req.Author)
	key := cacheKey(query, author, req.Limit, req.Cursor)
	if result, ok := s.cached(ctx, key); ok {
		return result, nil
	}
	value, err, _ := s.cacheGroup.Do(key, func() (any, error) {
		if cached, ok := s.cached(ctx, key); ok {
			return cached, nil
		}
		start := time.Now()
		result, err := s.searchSource(ctx, req, query, author)
		if s.metrics != nil {
			s.metrics.ObserveSearch(time.Since(start))
		}
		if err != nil {
			return nil, err
		}
		s.storeCached(ctx, key, result)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*ListResult), nil
}

func (s *Service) searchSource(ctx context.Context, req SearchRequest, query, author string) (*ListResult, error) {
	if s.search == nil {
		return nil, fmt.Errorf("search: Elasticsearch searcher is not configured")
	}

	var pitID string
	var after []json.RawMessage
	if req.Cursor != "" {
		cursor, err := decodeSearchCursor(req.Cursor, query, author)
		if err != nil {
			return nil, err
		}
		pitID, after = cursor.PITID, cursor.After
	}
	paged, supportsPaging := s.search.(pageSearcher)
	if !supportsPaging && req.Cursor != "" {
		return nil, fmt.Errorf("%w: search cursor unsupported", domain.ErrInvalidCursor)
	}
	var page domain.SearchPage
	var err error
	if supportsPaging {
		page, err = paged.SearchPage(ctx, query, author, pitID, after, req.Limit)
	} else {
		var ids []int64
		ids, err = s.search.Search(ctx, query, author, req.Limit)
		page.IDs = ids
	}
	if err != nil {
		if pitID != "" {
			_ = paged.ClosePIT(ctx, pitID)
			if errors.Is(err, domain.ErrPITExpired) {
				return nil, fmt.Errorf("%w: expired or unavailable search cursor", domain.ErrInvalidCursor)
			}
		}
		return nil, fmt.Errorf("search articles: %w", err)
	}
	if page.PITID == "" {
		page.PITID = pitID
	}
	if !page.HasMore {
		if supportsPaging {
			defer func() { _ = paged.ClosePIT(ctx, page.PITID) }()
		}
	}
	ids := page.IDs
	rows, err := s.articles.GetByIDs(ctx, ids)
	if err != nil {
		if supportsPaging {
			_ = paged.ClosePIT(ctx, page.PITID)
		}
		return nil, fmt.Errorf("hydrate search articles: %w", err)
	}

	byID := make(map[int64]*domain.Article, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	ordered := make([]*domain.Article, 0, len(rows))
	for _, id := range ids {
		if row, ok := byID[id]; ok {
			ordered = append(ordered, row)
		} else {
			s.logger.Warn("search hit missing from PostgreSQL", "article_id", id)
		}
	}
	result := &ListResult{Articles: ordered, Limit: req.Limit, HasMore: page.HasMore}
	if page.HasMore {
		token, err := encodeSearchCursor(page.PITID, page.LastSort, query, author)
		if err != nil {
			if supportsPaging {
				_ = paged.ClosePIT(ctx, page.PITID)
			}
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
