package http

import (
	"database/sql"
	"log/slog"

	"github.com/gin-gonic/gin"

	appsvc "github.com/kumparan/article-service/internal/application/article"
	"github.com/kumparan/article-service/internal/platform/metrics"
)

// NewRouter constructs and returns the application router.
func NewRouter(db *sql.DB, articleSvc *appsvc.Service, observability ...ObservabilityConfig) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	if len(observability) > 0 {
		r.Use(RequestObservability(observability[0].Logger, observability[0].Metrics))
	}

	r.GET("/health", HealthHandler())
	r.GET("/ready", ReadyHandler(db))

	articles := NewArticleHandler(articleSvc)
	authors := NewAuthorHandler(articleSvc)
	r.POST("/authors", authors.Create)
	r.GET("/authors", authors.List)
	r.POST("/articles", articles.Create)
	r.PATCH("/articles/:id", articles.Update)
	r.GET("/articles", articles.List)

	return r
}

type ObservabilityConfig struct {
	Logger  *slog.Logger
	Metrics *metrics.Registry
}
