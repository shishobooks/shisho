package worker

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const identifyCoverEnricherManifest = `{
  "manifestVersion": 1,
  "id": "cover-enricher",
  "name": "Cover Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "fileTypes": ["cbz"],
      "fields": ["cover"]
    }
  }
}`

func identifyTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 30, B: 30, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// newIdentifyCoverScanFixture scans a CBZ with JPEG pages, then swaps its
// scanned cover for a PNG so a page apply installs at a different extension
// than the cover it replaces.
func newIdentifyCoverScanFixture(t *testing.T) (*identifyScanFixture, string, []byte) {
	t.Helper()

	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)
	installTestPlugin(t, tc, pluginDir, "cover-enricher", identifyCoverEnricherManifest, `var plugin = {metadataEnricher: {search: function(ctx) {return {results: []};}}};`)
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))

	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "Comic")
	testgen.GenerateCBZ(t, bookDir, "book.cbz", testgen.CBZOptions{Title: "Comic", HasComicInfo: true, PageCount: 3, ImageFormat: "jpeg"})
	require.NoError(t, tc.runScan())

	f := &identifyScanFixture{tc: tc}
	_, file := f.retrieve(t)
	require.NotNil(t, file.CoverImageFilename, "precondition: the Scan extracted a cover")
	require.NoError(t, os.Remove(filepath.Join(bookDir, *file.CoverImageFilename)))

	prevPath := filepath.Join(bookDir, "book.cbz.cover.png")
	prevBytes := identifyTestPNG(t)
	require.NoError(t, os.WriteFile(prevPath, prevBytes, 0600))
	prevFilename, prevMime, prevSource := "book.cbz.cover.png", "image/png", models.DataSourceManual
	file.CoverImageFilename = &prevFilename
	file.CoverMimeType = &prevMime
	file.CoverSource = &prevSource
	require.NoError(t, tc.bookService.UpdateFile(tc.ctx, file, books.UpdateFileOptions{Columns: []string{"cover_image_filename", "cover_mime_type", "cover_source"}}))
	return f, prevPath, prevBytes
}

// A page-cover apply whose on-disk install fails must warn and leave the
// previous cover readable, with its metadata and provenance untouched.
func TestIdentifyApply_CoverPage_FailedInstallKeepsPreviousCover(t *testing.T) {
	t.Parallel()

	f, prevPath, prevBytes := newIdentifyCoverScanFixture(t)
	book, file := f.retrieve(t)

	// A nonempty directory at the destination makes the install fail
	// deterministically: it can be neither created over nor removed.
	obstruction := filepath.Join(filepath.Dir(prevPath), "book.cbz.cover.jpg")
	require.NoError(t, os.Mkdir(obstruction, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(obstruction, "keep"), []byte("x"), 0600))

	resp := postIdentifyApplyResponse(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
		BookID:      book.ID,
		FileID:      &file.ID,
		Fields:      map[string]any{"cover_page": 1},
		PluginScope: "test",
		PluginID:    "cover-enricher",
	})
	require.Len(t, resp.Warnings, 1)
	assert.Contains(t, resp.Warnings[0], "Cover was not applied")

	_, file = f.retrieve(t)
	assert.Equal(t, "book.cbz.cover.png", *file.CoverImageFilename)
	assert.Equal(t, "image/png", *file.CoverMimeType)
	assert.Equal(t, models.DataSourceManual, *file.CoverSource)
	got, err := os.ReadFile(prevPath)
	require.NoError(t, err, "the previous cover must still exist after a failed install")
	assert.Equal(t, prevBytes, got, "the previous cover bytes must be untouched")
}

func TestIdentifyApply_CoverPage_ReplacesPreviousCover(t *testing.T) {
	t.Parallel()

	f, prevPath, _ := newIdentifyCoverScanFixture(t)
	book, file := f.retrieve(t)

	resp := postIdentifyApplyResponse(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
		BookID:      book.ID,
		FileID:      &file.ID,
		Fields:      map[string]any{"cover_page": 1},
		PluginScope: "test",
		PluginID:    "cover-enricher",
	})
	assert.Empty(t, resp.Warnings)

	_, file = f.retrieve(t)
	assert.Equal(t, "book.cbz.cover.jpg", *file.CoverImageFilename)
	assert.Equal(t, "image/jpeg", *file.CoverMimeType)
	assert.Equal(t, models.PluginDataSource("test", "cover-enricher"), *file.CoverSource)
	require.NotNil(t, file.CoverPage)
	assert.Equal(t, 1, *file.CoverPage)
	_, statErr := os.Stat(prevPath)
	assert.True(t, os.IsNotExist(statErr), "the previous cover must be removed after a successful install")
	entries, err := filepath.Glob(filepath.Join(filepath.Dir(prevPath), "book.cbz.cover.*"))
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(filepath.Dir(prevPath), "book.cbz.cover.jpg")}, entries)
}

// Cover URLs are cached as immutable and busted by cover_cache_key, which is
// derived from the selected file's updated_at. Replacing the cover of the file
// that is already selected must therefore change the key; selecting a
// different file would change it regardless, so this pins the same file
// before and after through real persistence.
func TestIdentifyApply_CoverImage_RefreshesCacheKeyForSameFile(t *testing.T) {
	t.Parallel()

	// Encoded once up front: the handler runs on the server goroutine, where
	// a require failure could not stop the test.
	body := identifyTestJPEG(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	srvURL, err := url.Parse(srv.URL)
	require.NoError(t, err)

	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)
	manifest := fmt.Sprintf(`{
  "manifestVersion": 1,
  "id": "cover-url-enricher",
  "name": "Cover URL Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {"fileTypes": ["epub"], "fields": ["cover"]},
    "httpAccess": {"domains": [%q]}
  }
}`, srvURL.Host)
	installTestPlugin(t, tc, pluginDir, "cover-url-enricher", manifest, `var plugin = {metadataEnricher: {search: function(ctx) {return {results: []};}}};`)
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))

	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "Novel")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Novel", HasCover: true})
	require.NoError(t, tc.runScan())

	f := &identifyScanFixture{tc: tc}
	_, file := f.retrieve(t)
	require.NotNil(t, file.CoverImageFilename, "precondition: the Scan extracted a cover")

	// Push the file into the past so a same-second apply cannot mask a key
	// that failed to refresh.
	past := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	_, err = tc.db.ExecContext(tc.ctx, "UPDATE files SET updated_at = ? WHERE id = ?", past, file.ID)
	require.NoError(t, err)
	book, file := f.retrieve(t)
	staleKey := covers.CacheKey(book.Files, book.Library.CoverAspectRatio)
	require.Equal(t, fmt.Sprintf("%d-%d", file.ID, past.Unix()), staleKey)

	resp := postIdentifyApplyResponse(t, newIdentifyApplyServer(t, tc), plugins.PluginApplyPayload{
		BookID:      book.ID,
		FileID:      &file.ID,
		Fields:      map[string]any{"cover_url": srv.URL + "/cover.jpg"},
		PluginScope: "test",
		PluginID:    "cover-url-enricher",
	})
	assert.Empty(t, resp.Warnings)

	book, file = f.retrieve(t)
	assert.Equal(t, "book.epub.cover.jpg", *file.CoverImageFilename)
	assert.Equal(t, models.PluginDataSource("test", "cover-url-enricher"), *file.CoverSource)
	freshKey := covers.CacheKey(book.Files, book.Library.CoverAspectRatio)
	assert.NotEqual(t, staleKey, freshKey, "replacing the selected file's cover must change its cache key")
	assert.Equal(t, fmt.Sprintf("%d-%d", file.ID, file.UpdatedAt.Unix()), freshKey, "the key must still describe the same file")
	assert.Equal(t, freshKey, resp.CoverCacheKey, "the apply response must carry the refreshed key")
}

func identifyTestJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 30, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}
