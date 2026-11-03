package http

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// healthStatus is the response body for /health and /ready.
type healthStatus struct {
	Status string `json:"status"`
}

// HealthHandler serves GET /health (liveness probe).
// It returns 200 as long as the process is running — no dependency checks.
func HealthHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, healthStatus{Status: "ok"})
	}
}

// ReadyHandler serves GET /ready (readiness probe).
// It returns 200 only when PostgreSQL is reachable.
// It returns 503 when PostgreSQL is unavailable, without leaking connection details.
func ReadyHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			// Log the real error server-side (via the request logger middleware);
			// return a safe response body to the client.
			_ = c.Error(err) // attaches error for middleware logging
			c.JSON(http.StatusServiceUnavailable, healthStatus{Status: "unavailable"})
			return
		}

		c.JSON(http.StatusOK, healthStatus{Status: "ok"})
	}
}
