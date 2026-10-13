package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/cache"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdfpages"
	"github.com/stretchr/testify/require"
)

func TestCoverThumbnails_BookFileAndSeriesShareTheManagedCache(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
	file := new(models.File)
	require.NoError(t, f.db.NewSelect().Model(file).Where("id = ?", seeded.fileID).Scan(f.ctx))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 600, 900))))
	require.NoError(t, os.WriteFile(covers.FileCoverPath(file), buf.Bytes(), 0644))
	series := &models.Series{LibraryID: f.lib.ID, Name: "Series", NameSource: models.DataSourceManual, SortName: "Series", SortNameSource: models.DataSourceManual}
	f.insert(series)
	f.insert(&models.BookSeries{BookID: seeded.bookID, SeriesID: series.ID, SortOrder: 1})
	var first string
	for _, path := range []string{
		fmt.Sprintf("/api/books/%d/cover", seeded.bookID),
		fmt.Sprintf("/api/books/files/%d/cover", seeded.fileID),
		fmt.Sprintf("/api/series/%d/cover", series.ID),
	} {
		body := f.request(http.MethodGet, path+"?size=256&aspect=book&r="+covers.ThumbnailRenderKey, "", http.StatusOK)
		img, format, err := image.Decode(bytes.NewReader([]byte(body)))
		require.NoError(t, err)
		require.Equal(t, "jpeg", format)
		require.Equal(t, image.Rect(0, 0, 171, 256), img.Bounds())
		if first == "" {
			first = body
		} else {
			require.Equal(t, first, body)
		}
	}
	var entries []cache.Info
	require.NoError(t, json.Unmarshal([]byte(f.request(http.MethodGet, "/api/cache", "", http.StatusOK)), &entries))
	var thumbs *cache.Info
	for i := range entries {
		if entries[i].ID == "cover_thumbnails" {
			thumbs = &entries[i]
		}
	}
	require.NotNil(t, thumbs)
	require.Equal(t, 1, thumbs.FileCount)
	require.Positive(t, thumbs.SizeBytes)
	f.request(http.MethodPost, "/api/cache/cover_thumbnails/clear", "", http.StatusOK)
	require.FileExists(t, covers.FileCoverPath(file), "clearing thumbnails must preserve the source cover")
}

// defaultCacheSettings is GET /api/settings/cache before anything is saved.
const defaultCacheSettings = `{
	"cover_thumbnails": {"max_size_gb": 1, "unlimited": false},
	"downloads": {"max_size_gb": 5, "unlimited": false},
	"cbz_pages": {"max_size_gb": 5, "unlimited": true},
	"pdf_pages": {"max_size_gb": 5, "unlimited": true}
}`

func TestCacheSettings_DefaultsAndPersistedLimits(t *testing.T) {
	t.Parallel()
	f := newSharingSettingsFixture(t, false)
	get := f.do(t, f.admin, http.MethodGet, "/api/settings/cache", "")
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())
	require.JSONEq(t, defaultCacheSettings, get.Body.String())

	put := f.do(t, f.admin, http.MethodPut, "/api/settings/cache", `{"cbz_pages":{"max_size_gb":2.5,"unlimited":false}}`)
	require.Equal(t, http.StatusOK, put.Code, put.Body.String())
	put = f.do(t, f.admin, http.MethodPut, "/api/settings/cache", `{"downloads":{"max_size_gb":3,"unlimited":true}}`)
	require.Equal(t, http.StatusOK, put.Code, put.Body.String())
	saved := `{
		"cover_thumbnails": {"max_size_gb": 1, "unlimited": false},
		"downloads": {"max_size_gb": 3, "unlimited": true},
		"cbz_pages": {"max_size_gb": 2.5, "unlimited": false},
		"pdf_pages": {"max_size_gb": 5, "unlimited": true}
	}`
	require.JSONEq(t, saved, put.Body.String(), "saving one limit leaves the others as they were")

	restarted, err := New(newPermissionTestConfig(t), f.db, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	f.handler = restarted.Handler
	get = f.do(t, f.admin, http.MethodGet, "/api/settings/cache", "")
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())
	require.JSONEq(t, saved, get.Body.String())
}

func TestCacheSettings_DownloadsLimitAppliesNowAndAfterRestart(t *testing.T) {
	t.Parallel()
	f := newSharingSettingsFixture(t, false)
	cfg := newPermissionTestConfig(t)
	start := func() *downloadcache.Cache {
		// Built unlimited, as cmd/api/main.go does, so only the server can
		// set its limit.
		dl := downloadcache.NewCache(t.TempDir(), math.MaxInt64)
		srv, err := New(cfg, f.db, nil, nil, nil, nil, dl, nil, nil, nil)
		require.NoError(t, err)
		f.handler = srv.Handler
		return dl
	}
	dl := start()
	require.Equal(t, int64(5<<30), dl.MaxSize(), "the default limit applies at startup")

	rec := f.do(t, f.admin, http.MethodPut, "/api/settings/cache", `{"downloads":{"max_size_gb":2,"unlimited":false}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, int64(2<<30), dl.MaxSize(), "a saved limit applies immediately")
	require.Equal(t, int64(2<<30), start().MaxSize(), "a saved limit applies after restart")

	rec = f.do(t, f.admin, http.MethodPut, "/api/settings/cache", `{"downloads":{"max_size_gb":2,"unlimited":true}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, int64(math.MaxInt64), start().MaxSize())
}

func TestCacheSettings_PageCacheLimitAppliesNowAndAfterRestart(t *testing.T) {
	t.Parallel()
	f := newSharingSettingsFixture(t, false)
	cfg := newPermissionTestConfig(t)
	writeOld := func(path string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte("page"), 0644))
		past := time.Now().Add(-2 * time.Hour)
		require.NoError(t, os.Chtimes(path, past, past))
	}
	start := func() {
		srv, err := New(cfg, f.db, nil, nil, nil, nil, downloadcache.NewCache(t.TempDir(), 1<<30), cbzpages.NewCache(cfg.CacheDir), pdfpages.NewCache(cfg.CacheDir, cfg.PDFRenderDPI, cfg.PDFRenderQuality), nil)
		require.NoError(t, err)
		f.handler = srv.Handler
	}
	count := func(id string) int {
		rec := f.do(t, f.admin, http.MethodGet, "/api/cache", "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var entries []cache.Info
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &entries))
		for _, entry := range entries {
			if entry.ID == id {
				return entry.FileCount
			}
		}
		t.Fatalf("cache %s missing", id)
		return -1
	}
	start()
	writeOld(filepath.Join(cfg.CacheDir, "cbz", "1", "page_0_v2.jpg"))
	writeOld(filepath.Join(cfg.CacheDir, "pdf", "1", "150-85", "page_0.jpg"))
	require.Equal(t, 1, count("cbz_pages"))

	rec := f.do(t, f.admin, http.MethodPut, "/api/settings/cache", `{"cbz_pages":{"max_size_gb":0,"unlimited":false}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Zero(t, count("cbz_pages"), "lowering the limit evicts existing pages immediately")
	require.Equal(t, 1, count("pdf_pages"), "each cache has its own limit")

	rec = f.do(t, f.admin, http.MethodPut, "/api/settings/cache", `{"pdf_pages":{"max_size_gb":0,"unlimited":false}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Zero(t, count("pdf_pages"))

	writeOld(filepath.Join(cfg.CacheDir, "cbz", "2", "page_0_v2.jpg"))
	start()
	require.Zero(t, count("cbz_pages"), "a saved page cache limit applies at startup")
}

func TestCacheSettings_CoverThumbnailZeroEvictsAndSurvivesRestart(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
	file := new(models.File)
	require.NoError(t, f.db.NewSelect().Model(file).Where("id = ?", seeded.fileID).Scan(f.ctx))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 600, 900))))
	require.NoError(t, os.WriteFile(covers.FileCoverPath(file), buf.Bytes(), 0644))
	coverPath := fmt.Sprintf("/api/books/%d/cover?size=256&aspect=book&r=%s", seeded.bookID, covers.ThumbnailRenderKey)
	f.request(http.MethodGet, coverPath, "", http.StatusOK)
	count := func() int {
		var entries []cache.Info
		require.NoError(t, json.Unmarshal([]byte(f.request(http.MethodGet, "/api/cache", "", http.StatusOK)), &entries))
		for _, entry := range entries {
			if entry.ID == "cover_thumbnails" {
				return entry.FileCount
			}
		}
		t.Fatal("thumbnail cache missing")
		return -1
	}
	require.Equal(t, 1, count())
	f.request(http.MethodPut, "/api/settings/cache", `{"cover_thumbnails":{"max_size_gb":0,"unlimited":false}}`, http.StatusOK)
	require.Zero(t, count(), "lowering the limit evicts existing thumbnails immediately")
	server, err := New(newPermissionTestConfig(t), f.db, nil, nil, nil, nil, downloadcache.NewCache(t.TempDir(), 1<<30), nil, nil, nil)
	require.NoError(t, err)
	f.handler = server.Handler
	f.request(http.MethodGet, coverPath, "", http.StatusOK)
	require.Zero(t, count(), "a zero limit must still apply after restart")
	require.FileExists(t, covers.FileCoverPath(file))
}

func TestCacheSettings_PermissionsValidationAndDemoMode(t *testing.T) {
	t.Parallel()
	f := newSharingSettingsFixture(t, false)
	rec := f.do(t, f.configReader, http.MethodGet, "/api/settings/cache", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	for _, user := range []*models.User{f.configReader, f.viewer} {
		rec = f.do(t, user, http.MethodPut, "/api/settings/cache", `{"cover_thumbnails":{"max_size_gb":0,"unlimited":false}}`)
		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	}
	rec = f.do(t, f.viewer, http.MethodGet, "/api/settings/cache", "")
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	for _, body := range []string{
		`{"cover_thumbnails":{"max_size_gb":-1,"unlimited":false}}`,
		`{"pdf_pages":{"max_size_gb":1025,"unlimited":true}}`,
		// A missing size must not read as 0 and empty the cache.
		`{"downloads":{"unlimited":false}}`,
		`{"cover_thumbnails":{}}`,
		`{"cbz_pages":{"max_size_gb":2}}`,
	} {
		rec = f.do(t, f.admin, http.MethodPut, "/api/settings/cache", body)
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	}
	rec = f.do(t, f.admin, http.MethodGet, "/api/settings/cache", "")
	require.JSONEq(t, defaultCacheSettings, rec.Body.String())
	demo := newSharingSettingsFixture(t, true)
	rec = demo.do(t, demo.admin, http.MethodPut, "/api/settings/cache", `{"downloads":{"max_size_gb":2,"unlimited":false}}`)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	rec = demo.do(t, demo.admin, http.MethodGet, "/api/settings/cache", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
