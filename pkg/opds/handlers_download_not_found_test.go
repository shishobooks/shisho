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
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A file whose path is missing from disk returns a 404 whose message says
// "File not found." on both OPDS download routes, for GET and HEAD alike.
func TestDownloadHandlers_MissingFileOnDisk_ReturnsFileNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		kepub  bool
	}{
		{"download", http.MethodGet, false},
		{"download HEAD", http.MethodHead, false},
		{"download kepub", http.MethodGet, true},
		{"download kepub HEAD", http.MethodHead, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			ctx := context.Background()

			lib := &models.Library{
				Name:                     "Lib",
				CoverAspectRatio:         "book",
				DownloadFormatPreference: models.DownloadFormatOriginal,
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

			h := &handler{bookService: books.NewService(db, appsettings.NewService(db))}
			user := &models.User{
				ID:            1,
				Username:      "alice",
				IsActive:      true,
				LibraryAccess: []*models.UserLibraryAccess{{LibraryID: &lib.ID}},
			}

			e := echo.New()
			req := httptest.NewRequest(tt.method, "/", nil)
			c := e.NewContext(req, httptest.NewRecorder())
			c.SetParamNames("id")
			c.SetParamValues(strconv.Itoa(file.ID))
			auth.SetUser(c, user)

			if tt.kepub {
				err = h.downloadKepub(c)
			} else {
				err = h.download(c)
			}

			var codeErr *errcodes.Error
			require.ErrorAs(t, err, &codeErr)
			assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode)
			assert.Equal(t, "not_found", codeErr.Code)
			assert.Equal(t, "File not found.", codeErr.Message)
		})
	}
}
