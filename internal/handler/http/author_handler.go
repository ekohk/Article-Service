package http

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	domain "github.com/kumparan/article-service/internal/domain/article"
)

type AuthorService interface {
	CreateAuthor(context.Context, string) (*domain.Author, error)
	ListAuthors(context.Context) ([]*domain.Author, error)
}

type AuthorHandler struct {
	svc AuthorService
}

func NewAuthorHandler(svc AuthorService) *AuthorHandler {
	return &AuthorHandler{svc: svc}
}

type createAuthorRequest struct {
	Name string `json:"name"`
}

type authorResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func (h *AuthorHandler) Create(c *gin.Context) {
	var req createAuthorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid or malformed request body")
		return
	}
	author, err := h.svc.CreateAuthor(c.Request.Context(), req.Name)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	respondCreated(c, authorResponse{
		ID: author.ID, Name: author.Name, CreatedAt: author.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *AuthorHandler) List(c *gin.Context) {
	authors, err := h.svc.ListAuthors(c.Request.Context())
	if err != nil {
		mapServiceError(c, err)
		return
	}
	items := make([]authorResponse, 0, len(authors))
	for _, author := range authors {
		items = append(items, toAuthorResponse(author))
	}
	respondOK(c, items)
}

func toAuthorResponse(author *domain.Author) authorResponse {
	return authorResponse{
		ID: author.ID, Name: author.Name,
		CreatedAt: author.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}
