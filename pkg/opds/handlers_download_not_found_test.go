package opds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reach"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A file whose path is missing from disk returns a 404 whose message says
// "File not found.", delivered as itself or as a KePub, for GET and HEAD
// alike.
func TestDownloadHandlers_MissingFileOnDisk_ReturnsFileNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		ext    string
	}{
		{"download", http.MethodGet, ""},
		{"download HEAD", http.MethodHead, ""},
		{"download kepub", http.MethodGet, ".kepub.epub"},
		{"download kepub HEAD", http.MethodHead, ".kepub.epub"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			ctx := context.Background()

			lib := &models.Library{
				Name:             "Lib",
				CoverAspectRatio: "book",
			}
			_, err := db.NewInsert().Model(lib).Exec(ctx)
			require.NoError(t, err)

			dir := t.TempDir()
			book := &models.Book{
				LibraryID:       lib.ID,
				Title:           "Missing",
				TitleSource:     models.DataSourceFilepath,
				SortTitle:       "Missing",
				SortTitleSource: models.DataSourceFilepath,
				AuthorSource:    models.DataSourceFilepath,
				Filepath:        dir,
			}
			_, err = db.NewInsert().Model(book).Exec(ctx)
			require.NoError(t, err)

			file := &models.File{
				LibraryID:     lib.ID,
				BookID:        book.ID,
				Filepath:      filepath.Join(dir, "missing.epub"),
				FileType:      models.FileTypeEPUB,
				FileRole:      models.FileRoleMain,
				FilesizeBytes: 4,
			}
			_, err = db.NewInsert().Model(file).Exec(ctx)
			require.NoError(t, err)

			bookService := books.NewService(db, appsettings.NewService(db))
			h := &handler{bookService: bookService, reachService: reach.NewService(db, bookService)}
			user := &models.User{
				ID:            1,
				Username:      "alice",
				IsActive:      true,
				LibraryAccess: []*models.UserLibraryAccess{{LibraryID: &lib.ID}},
			}

			e := echo.New()
			req := httptest.NewRequest(tt.method, "/", nil)
			c := e.NewContext(req, httptest.NewRecorder())
			c.SetParamNames("file")
			c.SetParamValues(strconv.Itoa(file.ID) + tt.ext)
			auth.SetUser(c, user)

			err = h.download(c)

			var codeErr *errcodes.Error
			require.ErrorAs(t, err, &codeErr)
			assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode)
			assert.Equal(t, "not_found", codeErr.Code)
			assert.Equal(t, "File not found.", codeErr.Message)
		})
	}
}
