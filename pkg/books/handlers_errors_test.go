package books

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errorBody struct {
	Error struct {
		Code       string `json:"code"`
		Message    string `json:"message"`
		StatusCode int    `json:"status_code"`
	} `json:"error"`
}

// assertErrorResponse requires the rendered errcodes body for status, code,
// and message.
func assertErrorResponse(t *testing.T, rr *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	require.Equal(t, status, rr.Code, "response body: %s", rr.Body.String())
	var body errorBody
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, code, body.Error.Code)
	assert.Equal(t, message, body.Error.Message)
	assert.Equal(t, status, body.Error.StatusCode)
}

// The file update validation errors go through errcodes, so their wire code
// is bad_request rather than a snake-cased copy of the message.
func TestUpdateFile_ValidationErrorsUseErrcodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileType string
		role     string
		body     string
		message  string
	}{
		{
			name:     "upgrading an unsupported supplement to main",
			fileType: "txt",
			role:     models.FileRoleSupplement,
			body:     `{"file_role":"main"}`,
			message:  "Cannot upgrade to main file: file type 'txt' is not supported as a main file.",
		},
		{
			name:     "an invalid language tag",
			fileType: models.FileTypeEPUB,
			role:     models.FileRoleMain,
			body:     `{"language":"!!"}`,
			message:  "Invalid language tag: !!",
		},
		{
			name:     "preferring a cover the file does not have",
			fileType: models.FileTypeEPUB,
			role:     models.FileRoleMain,
			body:     `{"is_preferred_cover":true}`,
			message:  "Cannot set preferred cover: file has no cover image.",
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

			req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rr := executeRequestWithUser(t, setupTestServer(t, db), req, user)

			assertErrorResponse(t, rr, http.StatusBadRequest, "bad_request", tt.message)
		})
	}
}

// A path ID that does not parse names no row, so it is a 404 for the
// resource, the same as an ID with no row.
func TestBookHandlers_NonNumericIDReturnsNotFound(t *testing.T) {
	t.Parallel()
	h := &handler{}

	tests := []struct {
		name     string
		method   string
		fn       echo.HandlerFunc
		resource string
	}{
		{"delete book", http.MethodDelete, h.deleteBook, "Book"},
		{"delete file", http.MethodDelete, h.deleteFile, "File"},
		{"set file review", http.MethodPatch, h.setFileReview, "File"},
		{"set book review", http.MethodPatch, h.setBookReview, "Book"},
		{"list library languages", http.MethodGet, h.listLibraryLanguages, "Library"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := echo.New().NewContext(httptest.NewRequest(tt.method, "/", nil), httptest.NewRecorder())
			c.SetParamNames("id")
			c.SetParamValues("abc")

			err := tt.fn(c)

			var ecErr *errcodes.Error
			require.ErrorAs(t, err, &ecErr, "want an errcodes error, got %T: %v", err, err)
			assert.Equal(t, http.StatusNotFound, ecErr.HTTPCode)
			assert.Equal(t, "not_found", ecErr.Code)
			assert.Equal(t, tt.resource+" not found.", ecErr.Message)
		})
	}
}
