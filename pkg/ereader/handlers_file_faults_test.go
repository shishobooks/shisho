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
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reach"
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
	device   *devices.Device
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
	device, err := devices.NewService(db).Create(ctx, user.ID, "Test Key", devices.DeviceTypeBrowser)
	require.NoError(t, err)
	lib := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
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
	bookService := books.NewService(db, appsettings.NewService(db))
	return &downloadFixture{
		t: t, db: db, lib: lib, book: book, dir: dir, cacheDir: cacheDir, device: device,
		h: &handler{bookService: bookService, downloadCache: cache, reachService: reach.NewService(db, bookService)},
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

// serve runs a download handler, the KePub one when kepub is set, and
// renders its error the way the server does, so the recorder holds what a
// client would receive.
func (f *downloadFixture) serve(file *models.File, kepub bool) *httptest.ResponseRecorder {
	f.t.Helper()
	deliveredType := ""
	if kepub {
		deliveredType = models.FileTypeKePub
	}
	return f.serveAs(file, deliveredType)
}

// serveAs runs the download handler for file delivered as deliveredType
// ("" for its own File Type) the way serve does.
func (f *downloadFixture) serveAs(file *models.File, deliveredType string) *httptest.ResponseRecorder {
	f.t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	withDevice(c, f.t, f.db, f.device)
	c.SetParamNames("credential", "fileId", "deliveredType")
	c.SetParamValues(f.device.Credential, strconv.Itoa(file.ID), deliveredType)
	var err error
	if deliveredType != "" {
		err = f.h.DownloadFileAs(c)
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
			ext := "-epub"
			if kepub {
				ext = "-kepub"
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

// The original is served when there is nothing to generate: a supplement or
// a main file whose type has no generator. Downloads are typed from the file
// type.
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
			want := "Original." + tt.fileType
			if tt.role == models.FileRoleSupplement {
				want = "Original - original." + tt.fileType
			}
			assert.Equal(t, `attachment; filename="`+want+`"`, rec.Header().Get("Content-Disposition"), "named after the book")
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
		f := newDownloadFixture(t, `Title "Quoted" Война`)
		rec := f.serve(f.addEPUB(), false)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, "application/epub+zip", rec.Header().Get("Content-Type"))
		assert.Contains(t, rec.Header().Get("Content-Disposition"), "; filename*=UTF-8''")
		assert.Contains(t, rec.Header().Get("Content-Disposition"), "%D0%92%D0%BE%D0%B9%D0%BD%D0%B0")
	})
	// MOBI and AZW3 downloads are generated with the book's metadata and
	// keep the type's media type and extension.
	for _, tt := range []struct {
		fileType    string
		kind        testgen.MOBIKind
		contentType string
	}{
		{models.FileTypeMOBI, testgen.MOBIKindMOBI6, "application/x-mobipocket-ebook"},
		{models.FileTypeAZW3, testgen.MOBIKindKF8, "application/vnd.amazon.mobi8-ebook"},
	} {
		t.Run(tt.fileType, func(t *testing.T) {
			t.Parallel()
			f := newDownloadFixture(t, "Kindle Book")
			file := f.addFile(tt.fileType, models.FileRoleMain, testgen.GenerateMOBI(t, f.dir, "book."+tt.fileType, testgen.MOBIOptions{Kind: tt.kind, Title: "Kindle Book"}))
			rec := f.serve(file, false)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Equal(t, tt.contentType, rec.Header().Get("Content-Type"))
			assert.Contains(t, rec.Header().Get("Content-Disposition"), "."+tt.fileType+`"`)
		})
	}
	t.Run("cbz", func(t *testing.T) {
		t.Parallel()
		f := newDownloadFixture(t, "Comic")
		file := f.addFile(models.FileTypeCBZ, models.FileRoleMain, testgen.GenerateCBZ(t, f.dir, "comic.cbz", testgen.CBZOptions{Title: "Comic"}))
		rec := f.serve(file, false)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, "application/vnd.comicbook+zip", rec.Header().Get("Content-Type"))
	})
}

// A download link names only a File Type the File can become, so any other
// is an invalid_state 422, and a supplement goes only as itself. A name no
// File Type has is a 404.
func TestDownloadHandlers_UnreachableFileType(t *testing.T) {
	t.Parallel()
	f := newDownloadFixture(t, "Unreachable")
	path := filepath.Join(f.dir, "audio.m4b")
	require.NoError(t, os.WriteFile(path, []byte("original bytes"), 0o644))
	audio := f.addFile(models.FileTypeM4B, models.FileRoleMain, path)
	pdfPath := filepath.Join(f.dir, "book.pdf")
	require.NoError(t, os.WriteFile(pdfPath, []byte("original bytes"), 0o644))
	pdf := f.addFile(models.FileTypePDF, models.FileRoleMain, pdfPath)
	supplementPath := filepath.Join(f.dir, "extras.cbz")
	require.NoError(t, os.WriteFile(supplementPath, []byte("original bytes"), 0o644))
	supplement := f.addFile(models.FileTypeCBZ, models.FileRoleSupplement, supplementPath)

	for _, tt := range []struct {
		file          *models.File
		deliveredType string
		status        int
	}{
		{audio, models.FileTypeKePub, http.StatusUnprocessableEntity},
		{pdf, models.FileTypeKePub, http.StatusUnprocessableEntity},
		{supplement, models.FileTypeEPUB, http.StatusUnprocessableEntity},
		{audio, "fb2", http.StatusNotFound},
	} {
		rec := f.serveAs(tt.file, tt.deliveredType)
		assert.Equal(t, tt.status, rec.Code, tt.deliveredType)
		assert.Empty(t, rec.Header().Get("Content-Disposition"), tt.deliveredType)
	}
}

// A damaged EPUB (here one that is not a zip) on a Kobo Device delivered as
// KePub is a 500 with no attachment: a failed Conversion never sends the
// Original File in its place. Delivered as EPUB, it still gets the Original
// File, since a failed generation of its own File Type only loses Shisho's
// metadata. A failure partway through a Conversion is covered with the other
// channels in pkg/server's streaming tests.
func TestDownloadHandlers_DamagedEPUB(t *testing.T) {
	t.Parallel()
	addDamaged := func(f *downloadFixture) *models.File {
		// An extension no mime table knows, so the Content-Type below can
		// only come from the file type.
		path := filepath.Join(f.dir, "broken.bin")
		require.NoError(t, os.WriteFile(path, []byte("not a zip"), 0o644))
		return f.addFile(models.FileTypeEPUB, models.FileRoleMain, path)
	}
	onKobo := func(t *testing.T) *downloadFixture {
		f := newDownloadFixture(t, "Broken")
		kobo, err := devices.NewService(f.db).Create(context.Background(), f.device.UserID, "Kobo", devices.DeviceTypeKobo)
		require.NoError(t, err)
		f.device = kobo
		return f
	}

	t.Run("as KePub", func(t *testing.T) {
		t.Parallel()
		f := onKobo(t)
		rec := f.serveAs(addDamaged(f), models.FileTypeKePub)
		assertServerFault(t, rec)
		assert.NotContains(t, rec.Body.String(), "not a zip")
	})

	t.Run("as EPUB", func(t *testing.T) {
		t.Parallel()
		f := onKobo(t)
		rec := f.serveAs(addDamaged(f), "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, "not a zip", rec.Body.String())
		assert.Equal(t, `attachment; filename="Broken.epub"`, rec.Header().Get("Content-Disposition"), "named after the book, with its File Type's extension")
		assert.Equal(t, "application/epub+zip", rec.Header().Get("Content-Type"))
	})
}
