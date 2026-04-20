// Package testutils provides test-only API endpoints.
// These routes are only registered when ENVIRONMENT=test.
package testutils

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers test-only routes.
// These endpoints should ONLY be registered in test environments.
func RegisterRoutes(e *echo.Echo, db *bun.DB, manager *plugins.Manager, installer *plugins.Installer) {
	h := &handler{db: db, manager: manager, installer: installer}

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

	// Plugin fixture endpoints — serve the fixture plugin as a zip so the
	// real install flow can download it from localhost during E2E tests.
	test.GET("/plugins/fixture.zip", h.fixtureZip)
	test.GET("/plugins/fixture-info", h.fixtureInfo)
}
