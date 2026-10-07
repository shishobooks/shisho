package books

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errcodesBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func requireErrcodesNotFound(t *testing.T, rr *httptest.ResponseRecorder, message string) {
	t.Helper()
	require.Equal(t, http.StatusNotFound, rr.Code)
	var body errcodesBody
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "not_found", body.Error.Code)
	assert.Equal(t, message, body.Error.Message)
}

// A page request for a CBZ or PDF whose source is gone from disk must return
// the same "File not found." 404 as the download and stream routes, whether or
// not the page was rendered into the page cache before the file disappeared.
func TestGetPage_MissingSourceOnDisk_Returns404(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fileType    string
		warmedCache bool
	}{
		{"cbz", models.FileTypeCBZ, false},
		{"cbz with cached page", models.FileTypeCBZ, true},
		{"pdf", models.FileTypePDF, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			library, book := setupTestLibraryAndBook(t, db)
			filePath := filepath.Join(t.TempDir(), "book."+tt.fileType)
			if tt.fileType == models.FileTypeCBZ {
				createTestCBZWithPages(t, filePath, 5)
			} else {
				require.NoError(t, os.WriteFile(filePath, []byte("%PDF-1.4 fake"), 0o644))
			}
			file := setupTestFile(t, db, book, tt.fileType, filePath)
			user := setupTestUser(t, db, library.ID, true)
			e := setupTestServer(t, db)
			url := "/books/files/" + strconv.Itoa(file.ID) + "/page/0"

			if tt.warmedCache {
				rr := executeRequestWithUser(t, e, httptest.NewRequest(http.MethodGet, url, nil), user)
				require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			}

			require.NoError(t, os.Remove(filePath))

			rr := executeRequestWithUser(t, e, httptest.NewRequest(http.MethodGet, url, nil), user)
			requireErrcodesNotFound(t, rr, "File not found.")
		})
	}
}

// The page endpoint is authenticated, so shared caches must not store it.
// "private" still lets the browser cache the page for a year.
func TestGetPage_SetsPrivateCacheControl(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	library, book := setupTestLibraryAndBook(t, db)
	filePath := filepath.Join(t.TempDir(), "book.cbz")
	createTestCBZWithPages(t, filePath, 5)
	file := setupTestFile(t, db, book, models.FileTypeCBZ, filePath)
	user := setupTestUser(t, db, library.ID, true)
	e := setupTestServer(t, db)

	req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/page/1?r="+cbzpages.CBZPageKey, nil)
	rr := executeRequestWithUser(t, e, req, user)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, "private, max-age=31536000, immutable", rr.Header().Get("Cache-Control"))
	assert.NotEmpty(t, rr.Body.Bytes())
}

// A file cover missing from disk must return the errcodes "Cover not found."
// 404 like the book and series cover routes, not Echo's generic "Not Found".
func TestFileCover_MissingCoverOnDisk_ReturnsErrcodesNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		clearFilename bool
	}{
		{"stored cover filename", false},
		{"fallback cover filename", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			ctx := context.Background()
			fileID := seedBookWithFileCover(ctx, t, db)

			var file models.File
			require.NoError(t, db.NewSelect().Model(&file).Where("id = ?", fileID).Scan(ctx))
			require.NoError(t, os.Remove(filepath.Join(filepath.Dir(file.Filepath), *file.CoverImageFilename)))
			if tt.clearFilename {
				_, err := db.NewUpdate().Model((*models.File)(nil)).
					Set("cover_image_filename = NULL").
					Where("id = ?", fileID).
					Exec(ctx)
				require.NoError(t, err)
			}

			user := setupTestUser(t, db, file.LibraryID, true)
			e := setupTestServer(t, db)
			req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(fileID)+"/cover", nil)
			rr := executeRequestWithUser(t, e, req, user)

			requireErrcodesNotFound(t, rr, "Cover not found.")
		})
	}
}

// PDF pages depend on the server's render settings, which the page URL
// carries as r. A browser tab opened before a restart with new settings still
// asks for the old key; the page it gets is rendered at the new settings, so
// it must not be cached immutably under the old URL.
func TestGetPage_PDFCachesOnlyUnderCurrentRenderKey(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	library, book := setupTestLibraryAndBook(t, db)
	filePath := testgen.GeneratePDF(t, t.TempDir(), "book.pdf", testgen.PDFOptions{PageCount: 2})
	file := setupTestFile(t, db, book, models.FileTypePDF, filePath)
	user := setupTestUser(t, db, library.ID, true)
	e := setupTestServer(t, db)
	base := "/books/files/" + strconv.Itoa(file.ID) + "/page/0?v=1"

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"current render key", "&r=200-85", "private, max-age=31536000, immutable"},
		{"stale render key", "&r=100-85", "private, no-store"},
		{"no render key", "", "private, no-store"},
	}
	for _, tt := range tests {
		rr := executeRequestWithUser(t, e, httptest.NewRequest(http.MethodGet, base+tt.query, nil), user)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, tt.want, rr.Header().Get("Cache-Control"), tt.name)
	}
}

// CBZ page URLs carry cbzpages.CBZPageKey as r, so a change to which image a
// page number names reaches browsers holding immutable copies of the old
// pages. A tab loaded before such a change asks without the current key and
// must not cache the page it gets under that URL.
func TestGetPage_CBZCachesOnlyUnderCurrentPageKey(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	library, book := setupTestLibraryAndBook(t, db)
	filePath := testgen.GenerateCBZ(t, t.TempDir(), "book.cbz", testgen.CBZOptions{PageCount: 2})
	file := setupTestFile(t, db, book, models.FileTypeCBZ, filePath)
	user := setupTestUser(t, db, library.ID, true)
	e := setupTestServer(t, db)
	base := "/books/files/" + strconv.Itoa(file.ID) + "/page/0?v=1"

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"current page key", "&r=" + cbzpages.CBZPageKey, "private, max-age=31536000, immutable"},
		{"stale page key", "&r=1", "private, no-store"},
		{"no page key", "", "private, no-store"},
	}
	for _, tt := range tests {
		rr := executeRequestWithUser(t, e, httptest.NewRequest(http.MethodGet, base+tt.query, nil), user)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, tt.want, rr.Header().Get("Cache-Control"), tt.name)
	}
}
