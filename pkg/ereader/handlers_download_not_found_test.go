package ereader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A file whose path is missing from disk returns a 404 whose message says
// "File not found." on both eReader file routes, for GET and HEAD alike.
func TestDownloadFileHandlers_MissingFileOnDisk_ReturnsFileNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		kepub  bool
	}{
		{"file", http.MethodGet, false},
		{"file HEAD", http.MethodHead, false},
		{"file kepub", http.MethodGet, true},
		{"file kepub HEAD", http.MethodHead, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := newTestDB(t)
			ctx := context.Background()

			var roleID int
			require.NoError(t, db.QueryRow("SELECT id FROM roles WHERE name = 'admin'").Scan(&roleID))
			user := &models.User{Username: "missing_file_user", PasswordHash: "hash", RoleID: roleID, IsActive: true}
			_, err := db.NewInsert().Model(user).Exec(ctx)
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, "INSERT INTO user_library_access (user_id, library_id) VALUES (?, NULL)", user.ID)
			require.NoError(t, err)
			apiKey, err := apikeys.NewService(db).Create(ctx, user.ID, "Test Key")
			require.NoError(t, err)

			lib := &models.Library{
				Name:                     "Lib",
				CoverAspectRatio:         "book",
				DownloadFormatPreference: models.DownloadFormatOriginal,
			}
			_, err = db.NewInsert().Model(lib).Exec(ctx)
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

			h := &handler{bookService: books.NewService(db)}

			e := echo.New()
			req := httptest.NewRequest(tt.method, "/", nil)
			req = req.WithContext(keyContext(ctx, t, db, apiKey))
			c := e.NewContext(req, httptest.NewRecorder())
			c.SetParamNames("apiKey", "fileId")
			c.SetParamValues(apiKey.Key, strconv.Itoa(file.ID))

			if tt.kepub {
				err = h.DownloadFileKepub(c)
			} else {
				err = h.DownloadFile(c)
			}

			var codeErr *errcodes.Error
			require.ErrorAs(t, err, &codeErr)
			assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode)
			assert.Equal(t, "not_found", codeErr.Code)
			assert.Equal(t, "File not found.", codeErr.Message)
		})
	}
}
