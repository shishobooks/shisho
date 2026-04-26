package books

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shishobooks/shisho/pkg/libraries"
)

func TestBookCover_SetsCacheControlPrivateNoCache(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()
	e := echo.New()
	bookService := NewService(db)
	libraryService := libraries.NewService(db)
	h := &handler{bookService: bookService, libraryService: libraryService}

	fileID := seedBookWithFileCover(ctx, t, db)

	// Look up the book ID via the seeded file.
	file, err := bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &fileID})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(file.BookID))

	require.NoError(t, h.bookCover(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
	assert.NotEmpty(t, rec.Header().Get("ETag"))
	// Last-Modified is intentionally NOT emitted: the served file's identity
	// can change (hybrid book + aspect-ratio change, file removed) without any
	// change to the new cover's mtime, so mtime-based revalidation could
	// return stale 304s. ETag bakes the file ID into the validator.
	assert.Empty(t, rec.Header().Get("Last-Modified"))
	assert.NotEmpty(t, rec.Body.Bytes())
}

func TestBookCover_Returns304WhenIfNoneMatchMatches(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()
	e := echo.New()
	bookService := NewService(db)
	libraryService := libraries.NewService(db)
	h := &handler{bookService: bookService, libraryService: libraryService}

	fileID := seedBookWithFileCover(ctx, t, db)
	file, err := bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &fileID})
	require.NoError(t, err)

	// First GET to capture the ETag.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetParamNames("id")
	c1.SetParamValues(strconv.Itoa(file.BookID))
	require.NoError(t, h.bookCover(c1))
	etag := rec1.Header().Get("ETag")
	require.NotEmpty(t, etag)

	// Revalidate with If-None-Match.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("id")
	c2.SetParamValues(strconv.Itoa(file.BookID))
	require.NoError(t, h.bookCover(c2))

	assert.Equal(t, http.StatusNotModified, rec2.Code)
	assert.Empty(t, rec2.Body.Bytes())
	assert.Equal(t, etag, rec2.Header().Get("ETag"))
	assert.Equal(t, "private, no-cache", rec2.Header().Get("Cache-Control"))
}
