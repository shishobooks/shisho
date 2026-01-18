// Package testutils provides test-only API endpoints.
// These routes are only registered when ENVIRONMENT=test.
package testutils

import (
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers test-only routes.
// These endpoints should ONLY be registered in test environments.
func RegisterRoutes(e *echo.Echo, db *bun.DB) {
	h := &handler{db: db}

	test := e.Group("/test")
	test.POST("/users", h.createUser)
	test.DELETE("/users", h.deleteAllUsers)

	// eReader test data endpoints
	test.POST("/libraries", h.createLibrary)
	test.POST("/books", h.createBook)
	test.POST("/persons", h.createPerson)
	test.POST("/series", h.createSeries)
	test.POST("/api-keys", h.createAPIKey)
	test.DELETE("/ereader", h.deleteAllEReaderData)
}
