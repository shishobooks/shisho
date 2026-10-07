package kobo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A file whose path is missing from disk returns a 404 whose message says
// "File not found." on the Kobo download route, for GET and HEAD alike.
func TestHandleDownload_MissingFileOnDisk_ReturnsFileNotFound(t *testing.T) {
	t.Parallel()

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			db := newSyncPointTestDB(t)
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

			h := &handler{
				service:       NewService(db),
				bookService:   books.NewService(db, appsettings.NewService(db)),
				downloadCache: downloadcache.NewCache(t.TempDir(), 1<<30),
			}

			e := echo.New()
			req := httptest.NewRequest(method, "/", nil)
			c := e.NewContext(req, httptest.NewRecorder())
			withKeyOwner(c)
			c.SetParamNames("bookId")
			c.SetParamValues(ShishoID(file.ID))

			err = h.handleDownload(c)

			var codeErr *errcodes.Error
			require.ErrorAs(t, err, &codeErr)
			assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode)
			assert.Equal(t, "not_found", codeErr.Code)
			assert.Equal(t, "File not found.", codeErr.Message)
		})
	}
}
