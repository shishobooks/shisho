package books

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadOriginalFile_SetsCacheControlNoStore(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, book := setupTestLibraryAndBook(t, db)
	epubPath := createTestEPUBFile(t)
	file := setupTestFile(t, db, book, "epub", epubPath)
	user := setupTestUser(t, db, library.ID, true)

	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/download/original", nil)
	rr := executeRequestWithUser(t, e, req, user)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "private, no-store", rr.Header().Get("Cache-Control"))
}

func TestStreamFile_SetsCacheControlNoStore(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, book := setupTestLibraryAndBook(t, db)
	m4bPath := createTestM4BFile(t, 1000)
	file := setupTestFile(t, db, book, "m4b", m4bPath)
	user := setupTestUser(t, db, library.ID, true)

	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/stream", nil)
	rr := executeRequestWithUser(t, e, req, user)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "private, no-store", rr.Header().Get("Cache-Control"))
}

func TestStreamFile_RangeRequest_SetsCacheControlNoStore(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, book := setupTestLibraryAndBook(t, db)
	m4bPath := createTestM4BFile(t, 1000)
	file := setupTestFile(t, db, book, "m4b", m4bPath)
	user := setupTestUser(t, db, library.ID, true)

	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/stream", nil)
	req.Header.Set("Range", "bytes=0-99")
	rr := executeRequestWithUser(t, e, req, user)

	require.Equal(t, http.StatusPartialContent, rr.Code)
	assert.Equal(t, "private, no-store", rr.Header().Get("Cache-Control"))
}
