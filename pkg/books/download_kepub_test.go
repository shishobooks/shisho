package books

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A device or Share recipient given the original of a bare .kepub gets it
// named .kepub.epub, the only name Kobo's reader and KOReader take as a
// KePub. Download Original keeps the file's own extension.
func TestResolveFallbackDownload_BareKePubIsNamedKePubEPUB(t *testing.T) {
	t.Parallel()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	file := &models.File{
		FileType: models.FileTypeKePub,
		FileRole: models.FileRoleSupplement,
		Filepath: "/lib/Book/Book - Author.kepub",
	}

	download, err := ResolveFallbackDownload(c, nil, &models.Book{}, file)
	require.NoError(t, err)
	assert.Equal(t, "Book - Author.kepub.epub", download.Filename)
	assert.Equal(t, "/lib/Book/Book - Author.kepub", download.Path)

	assert.Equal(t, "Book - Author.kepub", OriginalDownload(&models.Book{}, file).Filename)
}

// A main KePub whose generation fails falls back to the original, named
// .kepub.epub whichever name it has on disk.
func TestResolveFallbackDownload_DamagedKePubFallsBackAsKePubEPUB(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{
		"Book.kepub":      "Book.kepub.epub",
		"Book.kepub.epub": "Book.kepub.epub",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := testgen.WriteFile(t, dir, name, []byte("not a zip, so generation fails"))
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
			file := &models.File{ID: 1, FileType: models.FileTypeKePub, FileRole: models.FileRoleMain, Filepath: path}

			cache := downloadcache.NewCache(t.TempDir(), 1<<30)
			download, err := ResolveFallbackDownload(c, cache, &models.Book{Title: "Book"}, file)
			require.NoError(t, err)
			assert.Equal(t, path, download.Path, "the original is served")
			assert.Equal(t, want, download.Filename)
		})
	}
}
