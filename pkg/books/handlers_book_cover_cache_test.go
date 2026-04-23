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

	fileID, _ := seedBookWithFileCover(ctx, t, db)

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
	assert.NotEmpty(t, rec.Header().Get("Last-Modified"))
	assert.NotEmpty(t, rec.Body.Bytes())
}

func TestBookCover_Returns304WhenIfModifiedSinceMatches(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()
	e := echo.New()
	bookService := NewService(db)
	libraryService := libraries.NewService(db)
	h := &handler{bookService: bookService, libraryService: libraryService}

	fileID, _ := seedBookWithFileCover(ctx, t, db)
	file, err := bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &fileID})
	require.NoError(t, err)

	// First GET.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetParamNames("id")
	c1.SetParamValues(strconv.Itoa(file.BookID))
	require.NoError(t, h.bookCover(c1))
	lastModified := rec1.Header().Get("Last-Modified")
	require.NotEmpty(t, lastModified)

	// Second GET with If-Modified-Since.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("If-Modified-Since", lastModified)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("id")
	c2.SetParamValues(strconv.Itoa(file.BookID))
	require.NoError(t, h.bookCover(c2))

	assert.Equal(t, http.StatusNotModified, rec2.Code)
	assert.Empty(t, rec2.Body.Bytes())
}
