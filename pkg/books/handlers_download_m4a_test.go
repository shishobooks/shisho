package books

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/mp4"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An .m4a is an M4B (ADR 0018). Its Generated File is named .m4b, matching
// the audiobook marking Shisho writes into it, while Download unmodified
// file returns the .m4a as it is on disk.
func TestDownloadFile_M4AAudiobook(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	db := testdb.New(t)
	library, book := setupTestLibraryAndBook(t, db)
	path := testgen.GenerateM4B(t, book.Filepath, "book.m4a", testgen.M4BOptions{Title: "Title In File", Duration: 1})
	content := testgen.ReadFile(t, path)
	file := setupTestFile(t, db, book, models.FileTypeM4B, path)
	user := setupTestUser(t, db, library.ID, true)
	server := setupTestServer(t, db)

	t.Run("generated download is an .m4b", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/download", nil)
		rr := executeRequestWithUser(t, server, req, user)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, "audio/mp4", rr.Header().Get(echo.HeaderContentType))
		assert.Contains(t, rr.Header().Get(echo.HeaderContentDisposition), `filename="Test Book.m4b"`)
		meta, err := mp4.Parse(testgen.WriteFile(t, t.TempDir(), "out.m4b", rr.Body.Bytes()))
		require.NoError(t, err)
		assert.Equal(t, "Test Book", meta.Title, "the download carries Shisho's metadata")
	})

	t.Run("Download unmodified file returns the .m4a", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/download/original", nil)
		rr := executeRequestWithUser(t, server, req, user)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, "audio/mp4", rr.Header().Get(echo.HeaderContentType))
		assert.Contains(t, rr.Header().Get(echo.HeaderContentDisposition), `filename="Test Book.m4a"`)
		assert.True(t, bytes.Equal(content, rr.Body.Bytes()), "the file is served untouched")
	})
}
