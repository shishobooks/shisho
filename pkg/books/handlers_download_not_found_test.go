package books

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Files missing from disk must produce a 404 whose message says "not found"
// exactly once. errcodes.NotFound appends " not found." to the resource name,
// so passing a phrase that already ends in "not found" doubles it.
func TestDownloadHandlers_MissingFileOnDisk_MessageSaysNotFoundOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		method   string
		fileType string
		path     string
		message  string
	}{
		{"download", http.MethodGet, "epub", "/download", "Source file not found."},
		{"download HEAD", http.MethodHead, "epub", "/download", "Source file not found."},
		{"download original", http.MethodGet, "epub", "/download/original", "File not found."},
		{"download kepub", http.MethodGet, "epub", "/download/kepub", "Source file not found."},
		{"download kepub HEAD", http.MethodHead, "epub", "/download/kepub", "Source file not found."},
		// Stream only accepts M4B; other types 404 before the disk check.
		{"stream", http.MethodGet, "m4b", "/stream", "File not found."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := setupTestDB(t)
			library, book := setupTestLibraryAndBook(t, db)
			var filePath string
			if tt.fileType == "m4b" {
				filePath = createTestM4BFile(t, 1000)
			} else {
				filePath = createTestEPUBFile(t)
			}
			file := setupTestFile(t, db, book, tt.fileType, filePath)
			user := setupTestUser(t, db, library.ID, true)
			require.NoError(t, os.Remove(filePath))

			e := setupTestServer(t, db)
			req := httptest.NewRequest(tt.method, "/books/files/"+strconv.Itoa(file.ID)+tt.path, nil)
			rr := executeRequestWithUser(t, e, req, user)

			require.Equal(t, http.StatusNotFound, rr.Code)
			var body struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
			assert.Equal(t, "not_found", body.Error.Code)
			assert.Equal(t, tt.message, body.Error.Message)
		})
	}
}
