package http_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	httphandler "github.com/kumparan/article-service/internal/handler/http"
	"github.com/kumparan/article-service/internal/platform/metrics"
)

func TestRequestObservabilityAddsIDAndRecordsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	registry := &metrics.Registry{}
	router := gin.New()
	router.Use(httphandler.RequestObservability(
		slog.New(slog.NewTextHandler(&logs, nil)), registry,
	))
	router.GET("/articles/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodGet, "/articles/7", nil)
	req.Header.Set("X-Request-ID", "request-test-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-ID") != "request-test-1" {
		t.Fatalf("request ID = %q", rec.Header().Get("X-Request-ID"))
	}
	snapshot := registry.Snapshot()
	if snapshot.Requests != 1 || snapshot.RequestErrors != 0 {
		t.Fatalf("metrics = %+v", snapshot)
	}
	for _, field := range []string{"request-test-1", "GET", "/articles/:id", "204", "duration"} {
		if !strings.Contains(logs.String(), field) {
			t.Errorf("log missing %q: %s", field, logs.String())
		}
	}
	if strings.Contains(logs.String(), "article body") {
		t.Error("request log contains article body")
	}
}

func TestRequestObservabilityCountsServerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := &metrics.Registry{}
	router := gin.New()
	router.Use(httphandler.RequestObservability(slog.Default(), registry))
	router.GET("/failure", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/failure", nil))
	if snapshot := registry.Snapshot(); snapshot.RequestErrors != 1 {
		t.Fatalf("request errors = %d", snapshot.RequestErrors)
	}
}
