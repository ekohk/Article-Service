// Package http contains HTTP handlers, response helpers, and the router.
// Handlers parse transport-level input and delegate to application services.
// Business logic must not live in this package.
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// envelope is the top-level wrapper for all success responses.
type envelope struct {
	Data any `json:"data,omitempty"`
}

// errorBody is the error response format documented in TDD Section 10.5.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, envelope{Data: data})
}

func respondCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, envelope{Data: data})
}

func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, errorBody{Error: errorDetail{Code: code, Message: message}})
}
