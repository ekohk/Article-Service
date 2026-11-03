package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	appsvc "github.com/kumparan/article-service/internal/application/article"
	domain "github.com/kumparan/article-service/internal/domain/article"
)

// ArticleCreator is the subset of the application service used by this handler.
// Declaring a narrow interface here keeps the handler testable without the full
// service graph.
type ArticleCreator interface {
	Create(ctx context.Context, req appsvc.CreateRequest) (*domain.Article, error)
}

// ArticleHandler holds the dependencies for article HTTP handlers.
type ArticleHandler struct {
	svc ArticleCreator
}

// NewArticleHandler creates an ArticleHandler.
func NewArticleHandler(svc ArticleCreator) *ArticleHandler {
	return &ArticleHandler{svc: svc}
}

// createArticleRequest is the expected JSON body for POST /articles.
type createArticleRequest struct {
	AuthorID int64  `json:"author_id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
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
