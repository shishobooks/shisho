package books

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A request the file's current state cannot honor is a 422 with the
// invalid_state code, distinct from a rejected payload value.
func TestFileStateChecksAreInvalidState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileType string
		role     string
		method   string
		path     string
		body     string
		message  string
	}{
		{
			name: "review state on a supplement", fileType: "txt", role: models.FileRoleSupplement,
			method: http.MethodPatch, path: "/review", body: `{"override":"reviewed"}`,
			message: "Cannot set review state on a supplement file",
		},
		{
			name: "cover page on a file without pages", fileType: models.FileTypeEPUB, role: models.FileRoleMain,
			method: http.MethodPut, path: "/cover-page", body: `{"page":0}`,
			message: "This file does not support page-based covers",
		},
		{
			name: "page of a file without pages", fileType: models.FileTypeEPUB, role: models.FileRoleMain,
			method: http.MethodGet, path: "/page/0",
			message: "Only CBZ and PDF files have pages",
		},
		{
			name: "KePub download of an audiobook", fileType: models.FileTypeM4B, role: models.FileRoleMain,
			method: http.MethodGet, path: "/download/kepub",
			message: "KePub conversion is not supported for m4b files",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			library, book := setupTestLibraryAndBook(t, db)
			path := filepath.Join(book.Filepath, "book."+tt.fileType)
			require.NoError(t, os.WriteFile(path, []byte("content"), 0o644))
			file := setupTestFile(t, db, book, tt.fileType, path)
			if tt.role != models.FileRoleMain {
				file.FileRole = tt.role
				_, err := db.NewUpdate().Model(file).Column("file_role").WherePK().Exec(t.Context())
				require.NoError(t, err)
			}
			user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, "/books/files/"+strconv.Itoa(file.ID)+tt.path, body)
			if tt.body != "" {
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			}
			rr := executeRequestWithUser(t, setupTestServer(t, db), req, user)

			assertErrorResponse(t, rr, http.StatusUnprocessableEntity, "invalid_state", tt.message)
		})
	}

	t.Run("cover upload on a page-based file", func(t *testing.T) {
		t.Parallel()
		db := testdb.New(t)
		library, book := setupTestLibraryAndBook(t, db)
		path := filepath.Join(book.Filepath, "book.cbz")
		createTestCBZWithPages(t, path, 1)
		file := setupTestFile(t, db, book, models.FileTypeCBZ, path)
		user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

		req := uploadCoverRequest(t, file.ID, "image/png", "cover.png", makeTestPNGBytes(t, 4, 4))
		rr := executeRequestWithUser(t, setupTestServer(t, db), req, user)

		assertErrorResponse(t, rr, http.StatusUnprocessableEntity, "invalid_state", "Cover upload is not supported for this file type.")
	})
}

// A generated download whose generation fails is a server fault, not a 422
// that blames the request. The source here is not a valid EPUB, so the
// generator fails reading it.
func TestGeneratedDownload_GenerationFailureIsServerError(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/download", "/download/kepub"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			library, book := setupTestLibraryAndBook(t, db)
			file := setupTestFile(t, db, book, models.FileTypeEPUB, createTestEPUBFile(t))
			user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

			req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+path, nil)
			rr := executeRequestWithUser(t, setupTestServer(t, db), req, user)

			assertErrorResponse(t, rr, http.StatusInternalServerError, "internal_server_error", "Internal Server Error")
		})
	}
}

// Setting a cover page whose extracted image cannot be written is a server
// fault. A nonempty directory where the cover file goes blocks the write.
func TestUpdateFileCoverPage_WriteFailureIsServerError(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	library, book := setupTestLibraryAndBook(t, db)
	path := filepath.Join(book.Filepath, "book.cbz")
	createTestCBZWithPages(t, path, 2)
	file := setupTestFile(t, db, book, models.FileTypeCBZ, path)
	pageCount := 2
	file.PageCount = &pageCount
	_, err := db.NewUpdate().Model(file).Column("page_count").WherePK().Exec(context.Background())
	require.NoError(t, err)
	obstruction := filepath.Join(book.Filepath, "book.cbz.cover.jpg")
	require.NoError(t, os.Mkdir(obstruction, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(obstruction, "keep"), []byte("x"), 0o600))
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	req := httptest.NewRequest(http.MethodPut, "/books/files/"+strconv.Itoa(file.ID)+"/cover-page", strings.NewReader(`{"page":1}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rr := executeRequestWithUser(t, setupTestServer(t, db), req, user)

	assertErrorResponse(t, rr, http.StatusInternalServerError, "internal_server_error", "Internal Server Error")
}

// failingReader returns part of a multipart body and then err, as a
// connection that drops mid-upload, or a failed spill of the upload to a
// temporary file, would.
type failingReader struct {
	r   io.Reader
	err error
}

func (f *failingReader) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if errors.Is(err, io.EOF) {
		return n, f.err
	}
	return n, err
}

// partialCoverBody returns the start of a multipart body with a cover part
// and no closing boundary, and its content type.
func partialCoverBody(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("cover", "cover.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("partial"))
	require.NoError(t, err)
	return &buf, w.FormDataContentType()
}

// A cover upload with no cover part is a 422, one whose multipart body is
// malformed or cut short is the binder's 400 malformed_payload, and one that
// fails on the server's filesystem is a 500.
func TestUploadFileCover_FormFileErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    func(t *testing.T) (io.Reader, string)
		status  int
		code    string
		message string
	}{
		{"no cover part", func(t *testing.T) (io.Reader, string) {
			var buf bytes.Buffer
			w := multipart.NewWriter(&buf)
			require.NoError(t, w.WriteField("other", "value"))
			require.NoError(t, w.Close())
			return &buf, w.FormDataContentType()
		}, http.StatusUnprocessableEntity, "validation_error", "Cover image is required"},
		{"not a multipart body", func(*testing.T) (io.Reader, string) {
			return strings.NewReader(`{}`), echo.MIMEApplicationJSON
		}, http.StatusUnprocessableEntity, "validation_error", "Cover image is required"},
		{"body cut short", func(t *testing.T) (io.Reader, string) {
			return partialCoverBody(t)
		}, http.StatusBadRequest, "malformed_payload", "Malformed Payload"},
		{"connection dropped", func(t *testing.T) (io.Reader, string) {
			body, contentType := partialCoverBody(t)
			return &failingReader{r: body, err: errors.New("connection reset")}, contentType
		}, http.StatusBadRequest, "malformed_payload", "Malformed Payload"},
		{"filesystem failure", func(t *testing.T) (io.Reader, string) {
			body, contentType := partialCoverBody(t)
			return &failingReader{r: body, err: &fs.PathError{Op: "write", Path: "/tmp/multipart-1", Err: syscall.ENOSPC}}, contentType
		}, http.StatusInternalServerError, "internal_server_error", "Internal Server Error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, e, user, file, _, _ := newUploadCoverFixture(t)
			body, contentType := tt.body(t)
			req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID)+"/cover", body)
			req.Header.Set(echo.HeaderContentType, contentType)
			rr := executeRequestWithUser(t, e, req, user)

			assertErrorResponse(t, rr, tt.status, tt.code, tt.message)
		})
	}
}

// A Range header the stream handler does not support, such as a suffix range
// or a malformed one, is ignored and the whole file served with 200, as RFC
// 9110 allows, rather than failing the request.
func TestStreamFile_UnsupportedRangeServesWholeFile(t *testing.T) {
	t.Parallel()

	for _, rangeHeader := range []string{"bytes=-500", "bytes=abc", "items=0-10"} {
		t.Run(rangeHeader, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			library, book := setupTestLibraryAndBook(t, db)
			m4bPath := createTestM4BFile(t, 5000)
			file := setupTestFile(t, db, book, models.FileTypeM4B, m4bPath)
			user := setupTestUser(t, db, library.ID, true)

			req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/stream", nil)
			req.Header.Set("Range", rangeHeader)
			rr := executeRequestWithUser(t, setupTestServer(t, db), req, user)

			require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())
			assert.Equal(t, 5000, rr.Body.Len())
			assert.Empty(t, rr.Header().Get("Content-Range"))
		})
	}
}
