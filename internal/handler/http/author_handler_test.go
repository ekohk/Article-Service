package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	domain "github.com/kumparan/article-service/internal/domain/article"
	httphandler "github.com/kumparan/article-service/internal/handler/http"
)

type fakeAuthorService struct {
	author *domain.Author
	err    error
}

func (f fakeAuthorService) CreateAuthor(context.Context, string) (*domain.Author, error) {
	return f.author, f.err
}

func (f fakeAuthorService) ListAuthors(context.Context) ([]*domain.Author, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []*domain.Author{f.author}, nil
}

func TestCreateAuthorHandler_ReturnsCreatedAuthor(t *testing.T) {
	handler := httphandler.NewAuthorHandler(fakeAuthorService{
		author: &domain.Author{ID: 7, Name: "Andi Pratama", CreatedAt: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)},
	})
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/authors", strings.NewReader(`{"name":"Andi Pratama"}`))
	req.Header.Set("Content-Type", "application/json")
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(r)
	ctx.Request = req
	handler.Create(ctx)

	if r.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", r.Code, r.Body.String())
	}
	if !strings.Contains(r.Body.String(), `"id":7`) || !strings.Contains(r.Body.String(), `"name":"Andi Pratama"`) {
		t.Fatalf("response does not contain author data: %s", r.Body.String())
	}
}
