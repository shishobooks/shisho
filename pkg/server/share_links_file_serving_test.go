package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skipIfRootUser(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict root")
	}
}

// lockPath removes every permission bit from path, so it still stats but
// cannot be opened, and restores them when the test ends.
func lockPath(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
}

// assertFileServerFault checks the whole failed response: a 500 with the JSON
// error body and none of the headers the served file would carry.
func assertFileServerFault(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.NotContains(t, rec.Header().Get("Cache-Control"), "immutable")
	assert.Empty(t, rec.Header().Get("Content-Disposition"))
}

func (f *shareLinksFixture) shareGet(token string, path string) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.do(nil, http.MethodGet, "/api/share/"+token+path, "")
}

func shareFilePath(file *models.File, suffix string) string {
	return fmt.Sprintf("/files/%d%s", file.ID, suffix)
}

// A shared file that exists but cannot be opened is a 500 on the share
// routes, whether it would be served from the download cache or as the
// original, and a failed download is not counted.
func TestShareLinks_UnreadableFileIsServerError(t *testing.T) {
	t.Parallel()
	skipIfRootUser(t)

	t.Run("file cover", func(t *testing.T) {
		t.Parallel()
		f := newShareLinksFixture(t)
		f.setSharing(true, false)
		link := f.mustCreate(f.admin, f.bookA, `{}`)
		lockPath(t, filepath.Join(f.bookA.Filepath, *f.epubA.CoverImageFilename))

		assertFileServerFault(t, f.shareGet(link.Token, shareFilePath(f.epubA, "/cover")))
	})

	t.Run("download from the cache", func(t *testing.T) {
		t.Parallel()
		f := newShareLinksFixture(t)
		f.setSharing(true, false)
		link := f.mustCreate(f.admin, f.bookA, `{}`)
		require.Equal(t, http.StatusOK, f.shareGet(link.Token, shareFilePath(f.epubA, "/download")).Code)
		lockPath(t, filepath.Join(f.cacheDir, strconv.Itoa(f.epubA.ID)+".epub"))

		assertFileServerFault(t, f.shareGet(link.Token, shareFilePath(f.epubA, "/download")))
		assert.Equal(t, 1, f.listed(f.bookA)[link.ID].DownloadCount)
	})

	// An unreadable source makes generation fail. That is not a file type
	// with nothing to generate, so it must not fall back to the original.
	t.Run("download with an unreadable source", func(t *testing.T) {
		t.Parallel()
		f := newShareLinksFixture(t)
		f.setSharing(true, false)
		link := f.mustCreate(f.admin, f.bookA, `{}`)
		lockPath(t, f.epubA.Filepath)

		assertFileServerFault(t, f.shareGet(link.Token, shareFilePath(f.epubA, "/download")))
		assert.Equal(t, 0, f.listed(f.bookA)[link.ID].DownloadCount)
	})

	t.Run("supplement", func(t *testing.T) {
		t.Parallel()
		f := newShareLinksFixture(t)
		f.setSharing(true, false)
		link := f.mustCreate(f.admin, f.bookA, `{}`)
		lockPath(t, f.suppA.Filepath)

		assertFileServerFault(t, f.shareGet(link.Token, shareFilePath(f.suppA, "/download")))
		assert.Equal(t, 0, f.listed(f.bookA)[link.ID].DownloadCount)
	})
}

// Share downloads are typed from the file type and named with the escaped
// ASCII filename plus the UTF-8 filename* form.
func TestShareLinks_DownloadHeaders(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)
	supp := f.insertSupplement(context.Background(), f.bookA, `Notes "Quoted" Ünïcode.pdf`)
	link := f.mustCreate(f.admin, f.bookA, `{}`)

	rec := f.shareGet(link.Token, shareFilePath(f.epubA, "/download"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "application/epub+zip", rec.Header().Get("Content-Type"))
	assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))

	rec = f.shareGet(link.Token, shareFilePath(supp, "/download"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "application/pdf", rec.Header().Get("Content-Type"))
	assert.Equal(t,
		`attachment; filename="Notes \"Quoted\" ncode.pdf"; filename*=UTF-8''Notes%20%22Quoted%22%20%C3%9Cn%C3%AFcode.pdf`,
		rec.Header().Get("Content-Disposition"))
}
