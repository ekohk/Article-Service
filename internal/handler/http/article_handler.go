package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	appsvc "github.com/kumparan/article-service/internal/application/article"
	domain "github.com/kumparan/article-service/internal/domain/article"
)

// ArticleService is the subset of the application service used by this handler.
// Declaring a narrow interface here keeps the handler testable without the full
// service graph.
type ArticleService interface {
	Create(ctx context.Context, req appsvc.CreateRequest) (*domain.Article, error)
	Update(ctx context.Context, req appsvc.UpdateRequest) (*domain.Article, error)
	List(ctx context.Context, req appsvc.ListRequest) (*appsvc.ListResult, error)
	Search(ctx context.Context, req appsvc.SearchRequest) (*appsvc.ListResult, error)
}

// ArticleHandler holds the dependencies for article HTTP handlers.
type ArticleHandler struct {
	svc ArticleService
}

// NewArticleHandler creates an ArticleHandler.
func NewArticleHandler(svc ArticleService) *ArticleHandler {
	return &ArticleHandler{svc: svc}
}

// createArticleRequest is the expected JSON body for POST /articles.
type createArticleRequest struct {
	AuthorID int64  `json:"author_id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

type updateArticleRequest struct {
	Title *string `json:"title"`
	Body  *string `json:"body"`
}

// articleResponse is the JSON representation returned to API consumers.
// Timestamps are formatted as RFC 3339 UTC.
type articleResponse struct {
	ID        int64  `json:"id"`
	AuthorID  int64  `json:"author_id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Version   int64  `json:"version"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Create handles POST /articles.
func (h *ArticleHandler) Create(c *gin.Context) {
	// Enforce request body limit per D-03 (2 MB).
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)

	var req createArticleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid or malformed request body")
		return
	}

	result, err := h.svc.Create(c.Request.Context(), appsvc.CreateRequest{
		AuthorID: req.AuthorID,
		Title:    req.Title,
		Body:     req.Body,
	})
	if err != nil {
		mapServiceError(c, err)
		return
	}

	respondCreated(c, toArticleResponse(result))
}

func (h *ArticleHandler) Update(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "id must be a positive integer")
		return
	}
	var req updateArticleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid or malformed request body")
		return
	}
	result, err := h.svc.Update(c.Request.Context(), appsvc.UpdateRequest{
		ID: id, Title: req.Title, Body: req.Body,
	})
	if err != nil {
		mapServiceError(c, err)
		return
	}
	respondOK(c, toArticleResponse(result))
}

// List handles GET /articles (PostgreSQL listing or Elasticsearch keyword search).
func (h *ArticleHandler) List(c *gin.Context) {
	limit := domain.DefaultLimit
	if raw, ok := c.GetQuery("limit"); ok {
		n, err := strconv.Atoi(raw)
		if err != nil {
			respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be an integer between 1 and 100")
			return
		}
		limit = n
	}

	var result *appsvc.ListResult
	var err error
	if strings.TrimSpace(c.Query("query")) != "" {
		result, err = h.svc.Search(c.Request.Context(), appsvc.SearchRequest{
			Query:  c.Query("query"),
			Author: c.Query("author"),
			Limit:  limit,
			Cursor: c.Query("cursor"),
		})
	} else {
		result, err = h.svc.List(c.Request.Context(), appsvc.ListRequest{
			Author: c.Query("author"),
			Limit:  limit,
			Cursor: c.Query("cursor"),
		})
	}
	if err != nil {
		mapServiceError(c, err)
		return
	}

	items := make([]articleResponse, 0, len(result.Articles))
	for _, a := range result.Articles {
		items = append(items, toArticleResponse(a))
	}

	respondList(c, items, paginationBody{
		Limit:      result.Limit,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	})
}

// mapServiceError translates domain/application errors to HTTP responses.
// Raw infrastructure errors are never sent to clients.
func mapServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, domain.ErrAuthorNotFound):
		respondError(c, http.StatusNotFound, "AUTHOR_NOT_FOUND", "author not found")
	case errors.Is(err, domain.ErrNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "resource not found")
	case errors.Is(err, domain.ErrInvalidCursor):
		respondError(c, http.StatusBadRequest, "INVALID_CURSOR", "invalid pagination cursor")
	default:
		_ = c.Error(err)
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
	}
}

func toArticleResponse(a *domain.Article) articleResponse {
	return articleResponse{
		ID:        a.ID,
		AuthorID:  a.AuthorID,
		Title:     a.Title,
		Body:      a.Body,
		Version:   a.Version,
		CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: a.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
