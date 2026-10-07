package kobo

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
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

type koboDownloadFixture struct {
	t        *testing.T
	db       *bun.DB
	h        *handler
	book     *models.Book
	dir      string
	cacheDir string
}

func newKoboDownloadFixture(t *testing.T, title string) *koboDownloadFixture {
	t.Helper()
	db := newSyncPointTestDB(t)
	ctx := context.Background()
	lib := &models.Library{Name: "Lib", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
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
	return &koboDownloadFixture{
		t: t, db: db, book: book, dir: dir, cacheDir: cacheDir,
		h: &handler{service: NewService(db), bookService: books.NewService(db, appsettings.NewService(db)), downloadCache: cache},
	}
}

func (f *koboDownloadFixture) addFile(fileType, path string) *models.File {
	f.t.Helper()
	file := &models.File{LibraryID: f.book.LibraryID, BookID: f.book.ID, FileType: fileType, FileRole: models.FileRoleMain, Filepath: path, FilesizeBytes: 1}
	_, err := f.db.NewInsert().Model(file).Exec(context.Background())
	require.NoError(f.t, err)
	return file
}

func (f *koboDownloadFixture) addEPUB() *models.File {
	f.t.Helper()
	return f.addFile(models.FileTypeEPUB, testgen.GenerateEPUB(f.t, f.dir, "book.epub", testgen.EPUBOptions{Title: "Book"}))
}

func (f *koboDownloadFixture) serve(file *models.File) *httptest.ResponseRecorder {
	f.t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	withKeyOwner(c)
	c.SetParamNames("bookId")
	c.SetParamValues(ShishoID(file.ID))
	if err := f.h.handleDownload(c); err != nil {
		errcodes.NewHandler().Handle(err, c)
	}
	return rec
}

func lockKoboFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
}

func assertKoboServerFault(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Empty(t, rec.Header().Get("Content-Disposition"))
}

// A file that exists but cannot be opened is a 500 on the Kobo download,
// from the KePub cache or with an unreadable source, which must not fall
// back to serving the original.
func TestHandleDownload_UnreadableFileIsServerError(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict root")
	}

	t.Run("from the cache", func(t *testing.T) {
		t.Parallel()
		f := newKoboDownloadFixture(t, "Cached")
		file := f.addEPUB()
		require.Equal(t, http.StatusOK, f.serve(file).Code)
		lockKoboFile(t, filepath.Join(f.cacheDir, strconv.Itoa(file.ID)+".kepub.epub"))
		assertKoboServerFault(t, f.serve(file))
	})
	t.Run("with an unreadable source", func(t *testing.T) {
		t.Parallel()
		f := newKoboDownloadFixture(t, "Locked")
		file := f.addEPUB()
		lockKoboFile(t, file.Filepath)
		assertKoboServerFault(t, f.serve(file))
	})
}

// The Kobo download is named with the Kobo-safe KePub filename, not the raw
// title. The Kobo scope holds only EPUB and CBZ files, which KePub always
// converts, so there is no original fallback to name.
func TestHandleDownload_Filename(t *testing.T) {
	t.Parallel()
	f := newKoboDownloadFixture(t, `Title: "Quoted" Ünïcode`)
	file := f.addEPUB()
	rec := f.serve(file)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	want := downloadcache.FormatKepubDownloadFilename(f.book, file)
	assert.Equal(t, "Title Quoted ncode.kepub.epub", want)
	assert.Equal(t, `attachment; filename="`+want+`"`, rec.Header().Get("Content-Disposition"))
	assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
	assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
}
