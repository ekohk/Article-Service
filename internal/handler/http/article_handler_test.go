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

// ── fake service satisfying ArticleService ────────────────────────────────────

type fakeCreator struct {
	result *domain.Article
	list   *appsvc.ListResult
	err    error
	last   appsvc.ListRequest
}

func (f *fakeCreator) Create(_ context.Context, _ appsvc.CreateRequest) (*domain.Article, error) {
	return f.result, f.err
}

func (f *fakeCreator) List(_ context.Context, req appsvc.ListRequest) (*appsvc.ListResult, error) {
	f.last = req
	if f.list != nil {
		return f.list, f.err
	}
	return &appsvc.ListResult{Articles: []*domain.Article{}, Limit: 20}, f.err
}

// testRouter wires a router with a fake service for article endpoints.
func testRouter(svc httphandler.ArticleService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	h := httphandler.NewArticleHandler(svc)
	r.POST("/articles", h.Create)
	r.GET("/articles", h.List)
	return r
}

func getArticles(r *gin.Engine, rawQuery string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/articles"+rawQuery, nil)
	r.ServeHTTP(w, req)
	return w
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

func TestListHandler_Empty_Returns200(t *testing.T) {
	w := getArticles(testRouter(&fakeCreator{
		list: &appsvc.ListResult{Articles: []*domain.Article{}, Limit: 20},
	}), "")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", w.Code, w.Body)
	}
	var resp struct {
		Data       []articleJSON  `json:"data"`
		Pagination paginationJSON `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data == nil {
		t.Fatal("data must be [] not null")
	}
	if len(resp.Data) != 0 {
		t.Errorf("len(data) = %d, want 0", len(resp.Data))
	}
	if resp.Pagination.HasMore || resp.Pagination.NextCursor != nil {
		t.Errorf("unexpected pagination: %+v", resp.Pagination)
	}
	if resp.Pagination.Limit != 20 {
		t.Errorf("limit = %d, want 20", resp.Pagination.Limit)
	}
}

func TestListHandler_Items_Returns200(t *testing.T) {
	ts := fixedTime()
	cursor := "next-token"
	w := getArticles(testRouter(&fakeCreator{
		list: &appsvc.ListResult{
			Articles: []*domain.Article{
				{ID: 2, AuthorID: 1, Title: "New", Body: "b", Version: 1, CreatedAt: ts, UpdatedAt: ts},
			},
			Limit:      20,
			HasMore:    true,
			NextCursor: &cursor,
		},
	}), "")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", w.Code, w.Body)
	}
	var resp struct {
		Data       []articleJSON  `json:"data"`
		Pagination paginationJSON `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].ID != 2 {
		t.Errorf("data = %+v", resp.Data)
	}
	if !resp.Pagination.HasMore || resp.Pagination.NextCursor == nil || *resp.Pagination.NextCursor != cursor {
		t.Errorf("pagination = %+v", resp.Pagination)
	}
}

func TestListHandler_ForwardsAuthor(t *testing.T) {
	creator := &fakeCreator{}
	w := getArticles(testRouter(creator), "?author=%20Budi%20&limit=10")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", w.Code, w.Body)
	}
	if creator.last.Author != " Budi " || creator.last.Limit != 10 {
		t.Errorf("request = %+v, want author %q and limit 10", creator.last, " Budi ")
	}
}

func TestListHandler_InvalidCursor_Returns400(t *testing.T) {
	w := getArticles(testRouter(&fakeCreator{err: domain.ErrInvalidCursor}), "?cursor=nope")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_CURSOR")
}

func TestListHandler_InvalidLimit_Returns400(t *testing.T) {
	w := getArticles(testRouter(&fakeCreator{err: domain.ErrInvalidInput}), "?limit=0")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "VALIDATION_ERROR")
}

func TestListHandler_NonIntegerLimit_Returns400(t *testing.T) {
	w := getArticles(testRouter(&fakeCreator{}), "?limit=abc")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "VALIDATION_ERROR")
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

type articleJSON struct {
	ID int64 `json:"id"`
}

type paginationJSON struct {
	Limit      int     `json:"limit"`
	NextCursor *string `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}
