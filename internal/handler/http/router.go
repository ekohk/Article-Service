package http

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	appsvc "github.com/kumparan/article-service/internal/application/article"
)

// NewRouter constructs and returns the application router.
func NewRouter(db *sql.DB, articleSvc *appsvc.Service) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", HealthHandler())
	r.GET("/ready", ReadyHandler(db))

	articles := NewArticleHandler(articleSvc)
	r.POST("/articles", articles.Create)

	return r
}
