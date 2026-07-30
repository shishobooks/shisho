package books

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadFile_GeneratesM4BForGetAndHead(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			db := setupTestDB(t)
			library, book := setupTestLibraryAndBook(t, db)
			srcPath := testgen.GenerateM4B(t, t.TempDir(), "source.m4b", testgen.M4BOptions{
				Title:     "Source Title",
				Duration:  1,
				Faststart: true,
			})
			testgen.ExpandLastMdatSparse(t, srcPath, 64<<20)
			file := setupTestFile(t, db, book, models.FileTypeM4B, srcPath)
			user := setupTestUser(t, db, library.ID, true)
			server := setupTestServer(t, db)

			req := httptest.NewRequest(method, "/books/files/"+strconv.Itoa(file.ID)+"/download", nil)
			response := newDiscardResponseWriter()
			(&userContextHandler{echo: server, user: user}).ServeHTTP(response, req)

			require.Equal(t, http.StatusOK, response.status)
			assert.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
			assert.NotEmpty(t, response.Header().Get("Content-Disposition"))
			if method == http.MethodHead {
				assert.Zero(t, response.bytesWritten)
				assert.NotEmpty(t, response.Header().Get("Content-Length"))
			} else {
				assert.Positive(t, response.bytesWritten)
			}
		})
	}
}

type discardResponseWriter struct {
	header       http.Header
	status       int
	bytesWritten int64
}

func newDiscardResponseWriter() *discardResponseWriter {
	return &discardResponseWriter{header: make(http.Header)}
}

func (w *discardResponseWriter) Header() http.Header {
	return w.header
}

func (w *discardResponseWriter) WriteHeader(status int) {
	w.status = status
}

func (w *discardResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.bytesWritten += int64(len(data))
	return len(data), nil
}
