package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdfpages"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The books page route renders through the page caches handed to New, not
// through caches it builds for itself. The injected CBZ cache points at a
// directory other than cfg.CacheDir, so a page landing there proves the
// route used it.
func TestGetPage_UsesPageCacheFromServerNew(t *testing.T) {
	t.Parallel()
	db := newPermissionTestDB(t)
	cfg := newPermissionTestConfig(t)
	injectedDir := t.TempDir()
	srv, err := New(cfg, db, worker.New(&config.Config{WorkerProcesses: 1}, db, nil, nil, nil), nil, nil,
		downloadcache.NewCache(t.TempDir(), 1<<30), cbzpages.NewCache(injectedDir), pdfpages.NewCache(t.TempDir(), 150, 85), nil)
	require.NoError(t, err)
	f := &resourceDeleteFixture{t: t, ctx: t.Context(), db: db, handler: srv.Handler, authSvc: auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration())}
	f.lib = &models.Library{Name: "Library", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	f.insert(f.lib)
	f.admin = insertPermissionTestUser(f.ctx, t, db, "admin", models.RoleAdmin, nil)

	bookDir := t.TempDir()
	cbzPath := filepath.Join(bookDir, "comic.cbz")
	writeTestCBZ(t, cbzPath)
	book := &models.Book{LibraryID: f.lib.ID, Title: "Comic", TitleSource: models.DataSourceManual, SortTitle: "Comic", SortTitleSource: models.DataSourceFilepath, Filepath: bookDir}
	f.insert(book)
	pageCount := 1
	file := &models.File{LibraryID: f.lib.ID, BookID: book.ID, FileType: models.FileTypeCBZ, FileRole: models.FileRoleMain, Filepath: cbzPath, FilesizeBytes: 1, PageCount: &pageCount}
	f.insert(file)

	f.request(http.MethodGet, fmt.Sprintf("/api/books/files/%d/page/0", file.ID), "", http.StatusOK)

	injected, err := filepath.Glob(filepath.Join(injectedDir, "cbz", strconv.Itoa(file.ID), "page_0.*"))
	require.NoError(t, err)
	assert.Len(t, injected, 1, "the page is cached in the injected cache's directory")
	var stray []string
	require.NoError(t, filepath.WalkDir(cfg.CacheDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			stray = append(stray, path)
		}
		return err
	}))
	assert.Empty(t, stray, "nothing is cached under cfg.CacheDir by a cache the route built itself")
}

// Updating a Book through the real routes recomputes its review state. This
// covers the wiring in server.go that hands the books routes the shared
// books service with app settings attached.
func TestUpdateBook_RecomputesReviewedThroughSharedBookService(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)

	f.request(http.MethodPost, fmt.Sprintf("/api/books/%d", seeded.bookID), `{"description":""}`, http.StatusOK)

	assert.False(t, f.reviewed(seeded.fileID), "the File leaves reviewed once the required description is cleared")
}

func writeTestCBZ(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.White)
	var pngBytes bytes.Buffer
	require.NoError(t, png.Encode(&pngBytes, img))

	out, err := os.Create(path)
	require.NoError(t, err)
	zw := zip.NewWriter(out)
	w, err := zw.Create("001.png")
	require.NoError(t, err)
	_, err = w.Write(pngBytes.Bytes())
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, out.Close())
}
