package http

import (
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kumparan/article-service/internal/platform/metrics"
)

var requestSequence atomic.Uint64

func requestID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), requestSequence.Add(1))
}

func RequestObservability(log *slog.Logger, registry *metrics.Registry) gin.HandlerFunc {
	if log == nil {
		log = slog.Default()
	}
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = requestID()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		start := time.Now()

		c.Next()

		status := c.Writer.Status()
		if registry != nil {
			registry.IncRequest(status)
		}
		attrs := []any{
			"request_id", id,
			"method", c.Request.Method,
			"route", c.FullPath(),
			"status", status,
			"duration", time.Since(start).String(),
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, "error_category", "handler")
		}
		log.Info("http request", attrs...)
	}
}
