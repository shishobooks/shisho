// Package testutils provides test-only API endpoints.
// These routes are only registered when SHISHO_TEST_MODE=true.
package testutils

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers test-only routes.
// These endpoints should ONLY be registered in test environments.
// fileRoot is where POST /test/books writes files for withFileOnDisk. It must
// belong to this server alone, because DELETE /test/ereader removes it.
func RegisterRoutes(e *echo.Group, db *bun.DB, manager *plugins.Manager, installer *plugins.Installer, fileRoot string) {
	h := &handler{db: db, manager: manager, installer: installer, fileRoot: fileRoot}

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
	test.POST("/plugins", h.seedPlugin)
	test.DELETE("/plugins", h.deleteAllPlugins)
}
