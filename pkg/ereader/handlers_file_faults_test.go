package ereader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// downloadFixture is one book in one library, served by the eReader download
// handlers with a download cache the test can reach into.
type downloadFixture struct {
	t        *testing.T
	db       *bun.DB
	h        *handler
	lib      *models.Library
	book     *models.Book
	dir      string
	cacheDir string
	apiKey   *apikeys.APIKey
}

func newDownloadFixture(t *testing.T, title string) *downloadFixture {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()
	var roleID int
	require.NoError(t, db.QueryRow("SELECT id FROM roles WHERE name = 'admin'").Scan(&roleID))
	user := &models.User{Username: "reader", PasswordHash: "hash", RoleID: roleID, IsActive: true}
	_, err := db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO user_library_access (user_id, library_id) VALUES (?, NULL)", user.ID)
	require.NoError(t, err)
	apiKey, err := apikeys.NewService(db).Create(ctx, user.ID, "Test Key")
	require.NoError(t, err)
	lib := &models.Library{Name: "Lib", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	_, err = db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)
	dir := t.TempDir()
	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           title,
		TitleSource:     models.DataSourceManual,
		SortTitle:       title,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        dir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	cacheDir := t.TempDir()
	cache := downloadcache.NewCache(cacheDir, 1<<30)
	t.Cleanup(cache.Wait)
	return &downloadFixture{
		t: t, db: db, lib: lib, book: book, dir: dir, cacheDir: cacheDir, apiKey: apiKey,
		h: &handler{bookService: books.NewService(db), downloadCache: cache},
	}
}

func (f *downloadFixture) addFile(fileType, role, path string) *models.File {
	f.t.Helper()
	file := &models.File{LibraryID: f.lib.ID, BookID: f.book.ID, FileType: fileType, FileRole: role, Filepath: path, FilesizeBytes: 1}
	_, err := f.db.NewInsert().Model(file).Exec(context.Background())
	require.NoError(f.t, err)
	return file
}

func (f *downloadFixture) addEPUB() *models.File {
	f.t.Helper()
	return f.addFile(models.FileTypeEPUB, models.FileRoleMain, testgen.GenerateEPUB(f.t, f.dir, "book.epub", testgen.EPUBOptions{Title: "Book"}))
}

// serve runs a download handler and renders its error the way the server
// does, so the recorder holds what a client would receive.
func (f *downloadFixture) serve(file *models.File, kepub bool) *httptest.ResponseRecorder {
	f.t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	withKey(c, f.t, f.db, f.apiKey)
	c.SetParamNames("apiKey", "fileId")
	c.SetParamValues(f.apiKey.Key, strconv.Itoa(file.ID))
	var err error
	if kepub {
		err = f.h.DownloadFileKepub(c)
	} else {
		err = f.h.DownloadFile(c)
	}
	if err != nil {
		errcodes.NewHandler().Handle(err, c)
	}
	return rec
}

func lockFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
}

func assertServerFault(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Empty(t, rec.Header().Get("Content-Disposition"))
}

// A file that exists but cannot be opened is a 500 on both eReader download
// routes. An unreadable source makes generation fail, and that is a server
// fault, not a reason to fall back to serving the original.
func TestDownloadHandlers_UnreadableFileIsServerError(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict root")
	}

	for _, kepub := range []bool{false, true} {
		name := "download"
		if kepub {
			name = "kepub"
		}
		t.Run(name+" from the cache", func(t *testing.T) {
			t.Parallel()
			f := newDownloadFixture(t, "Cached")
			file := f.addEPUB()
			require.Equal(t, http.StatusOK, f.serve(file, kepub).Code)
			ext := ".epub"
			if kepub {
				ext = ".kepub.epub"
			}
			lockFile(t, filepath.Join(f.cacheDir, strconv.Itoa(file.ID)+ext))
			assertServerFault(t, f.serve(file, kepub))
		})
		t.Run(name+" with an unreadable source", func(t *testing.T) {
			t.Parallel()
			f := newDownloadFixture(t, "Locked")
			file := f.addEPUB()
			lockFile(t, file.Filepath)
			assertServerFault(t, f.serve(file, kepub))
		})
	}
}

// The original is served when there is nothing to generate: a supplement, a
// main file whose type has no generator, and a KePub request for a type
// KePub cannot convert. Downloads are typed from the file type.
func TestDownloadHandlers_ServesOriginalWhenNothingToGenerate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fileType    string
		role        string
		kepub       bool
		contentType string
	}{
		{"supplement", "txt", models.FileRoleSupplement, false, "text/plain; charset=utf-8"},
		{"plugin format", "fb2", models.FileRoleMain, false, ""},
		{"m4b as kepub", models.FileTypeM4B, models.FileRoleMain, true, "audio/mp4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newDownloadFixture(t, "Original")
			path := filepath.Join(f.dir, "original."+tt.fileType)
			require.NoError(t, os.WriteFile(path, []byte("original bytes"), 0o644))
			file := f.addFile(tt.fileType, tt.role, path)

			rec := f.serve(file, tt.kepub)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Equal(t, "original bytes", rec.Body.String())
			assert.Equal(t, `attachment; filename="original.`+tt.fileType+`"`, rec.Header().Get("Content-Disposition"))
			assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
			if tt.contentType != "" {
				assert.Equal(t, tt.contentType, rec.Header().Get("Content-Type"))
			}
		})
	}
}

// Generated downloads carry the file type's media type, whatever the host's
// mime table says, and an escaped filename with the UTF-8 filename* form.
func TestDownloadHandlers_GeneratedHeaders(t *testing.T) {
	t.Parallel()

	t.Run("epub", func(t *testing.T) {
		t.Parallel()
		f := newDownloadFixture(t, `Title "Quoted" Ünïcode`)
		rec := f.serve(f.addEPUB(), false)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, "application/epub+zip", rec.Header().Get("Content-Type"))
		assert.Contains(t, rec.Header().Get("Content-Disposition"), "; filename*=UTF-8''")
		assert.Contains(t, rec.Header().Get("Content-Disposition"), "%C3%9Cn%C3%AFcode")
	})
	t.Run("cbz", func(t *testing.T) {
		t.Parallel()
		f := newDownloadFixture(t, "Comic")
		file := f.addFile(models.FileTypeCBZ, models.FileRoleMain, testgen.GenerateCBZ(t, f.dir, "comic.cbz", testgen.CBZOptions{Title: "Comic"}))
		rec := f.serve(file, false)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, "application/vnd.comicbook+zip", rec.Header().Get("Content-Type"))
	})
}
