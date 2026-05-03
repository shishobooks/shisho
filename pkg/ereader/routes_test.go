package ereader

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestRegisterRoutes_DownloadAcceptsHEAD(t *testing.T) {
	t.Parallel()

	db := setupEReaderDB(t)

	e := echo.New()
	RegisterRoutes(e, db, nil)

	methods := map[string]map[string]bool{}
	for _, r := range e.Routes() {
		if methods[r.Path] == nil {
			methods[r.Path] = map[string]bool{}
		}
		methods[r.Path][r.Method] = true
	}

	for _, path := range []string{
		"/ereader/key/:apiKey/download/:bookId",
		"/ereader/key/:apiKey/file/:fileId",
		"/ereader/key/:apiKey/file/:fileId/kepub",
	} {
		assert.True(t, methods[path][http.MethodGet], "GET %s should be registered", path)
		assert.True(t, methods[path][http.MethodHead], "HEAD %s should be registered", path)
	}
}
