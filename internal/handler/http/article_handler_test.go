package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	appsvc "github.com/kumparan/article-service/internal/application/article"
	domain "github.com/kumparan/article-service/internal/domain/article"
	httphandler "github.com/kumparan/article-service/internal/handler/http"
)

// ── fake service satisfying ArticleCreator ────────────────────────────────────

type fakeCreator struct {
	result *domain.Article
	err    error
}

func (f *fakeCreator) Create(_ context.Context, _ appsvc.CreateRequest) (*domain.Article, error) {
	return f.result, f.err
}

// testRouter wires a router with a fake creator for POST /articles.
func testRouter(creator httphandler.ArticleCreator) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	h := httphandler.NewArticleHandler(creator)
	r.POST("/articles", h.Create)
	return r
}

func postJSON(r *gin.Engine, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/articles", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func fixedTime() time.Time {
	t, _ := time.Parse(time.RFC3339, "2026-11-03T10:00:00Z")
	return t
}

// ── handler tests (no DB) ─────────────────────────────────────────────────────

// Valid request → 201 with data envelope.
func TestCreateHandler_ValidRequest_Returns201(t *testing.T) {
	ts := fixedTime()
	creator := &fakeCreator{result: &domain.Article{
		ID: 1, AuthorID: 1, Title: "Hello", Body: "World",
		Version: 1, CreatedAt: ts, UpdatedAt: ts,
	}}
	w := postJSON(testRouter(creator), `{"author_id":1,"title":"Hello","body":"World"}`)
	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body: %s", w.Code, w.Body)
	}
	var resp map[string]map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["data"] == nil {
		t.Error("response missing data envelope")
	}
}

// Malformed JSON → 400 VALIDATION_ERROR.
func TestCreateHandler_MalformedJSON_Returns400(t *testing.T) {
	w := postJSON(testRouter(&fakeCreator{}), `{bad json`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "VALIDATION_ERROR")
}

// Author not found → 404 AUTHOR_NOT_FOUND.
func TestCreateHandler_AuthorNotFound_Returns404(t *testing.T) {
	w := postJSON(testRouter(&fakeCreator{err: domain.ErrAuthorNotFound}),
		`{"author_id":99,"title":"t","body":"b"}`)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "AUTHOR_NOT_FOUND")
}

// Validation error (blank title) → 400 VALIDATION_ERROR.
func TestCreateHandler_ValidationError_Returns400(t *testing.T) {
	w := postJSON(testRouter(&fakeCreator{err: domain.ErrInvalidInput}),
		`{"author_id":1,"title":"","body":"b"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "VALIDATION_ERROR")
}

// Unexpected error → 500 INTERNAL_ERROR, no leak.
func TestCreateHandler_InternalError_Returns500(t *testing.T) {
	w := postJSON(testRouter(&fakeCreator{err: errors.New("database exploded")}),
		`{"author_id":1,"title":"t","body":"b"}`)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "INTERNAL_ERROR")
	if strings.Contains(w.Body.String(), "database exploded") {
		t.Error("response body leaks internal error detail")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func assertErrorCode(t *testing.T, body []byte, want string) {
	t.Helper()
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if resp.Error.Code != want {
		t.Errorf("error.code = %q, want %q; body: %s", resp.Error.Code, want, body)
	}
}
