package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"testing"

	"github.com/shishobooks/shisho/pkg/cache"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/models"
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
		body := f.request(http.MethodGet, path+"?size=256&aspect=book&r=1", "", http.StatusOK)
		img, err := png.Decode(bytes.NewReader([]byte(body)))
		require.NoError(t, err)
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

func TestCoverThumbnailSettings_DefaultAndPersistedLimit(t *testing.T) {
	t.Parallel()
	f := newSharingSettingsFixture(t, false)
	get := f.do(t, f.admin, http.MethodGet, "/api/settings/cache", "")
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())
	require.JSONEq(t, `{"cover_thumbnail_max_size_gb":1}`, get.Body.String())
	put := f.do(t, f.admin, http.MethodPut, "/api/settings/cache", `{"cover_thumbnail_max_size_gb":2.5}`)
	require.Equal(t, http.StatusOK, put.Code, put.Body.String())
	require.JSONEq(t, `{"cover_thumbnail_max_size_gb":2.5}`, put.Body.String())
	restarted, err := New(newPermissionTestConfig(t), f.db, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	f.handler = restarted.Handler
	get = f.do(t, f.admin, http.MethodGet, "/api/settings/cache", "")
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())
	require.JSONEq(t, `{"cover_thumbnail_max_size_gb":2.5}`, get.Body.String())
}

func TestCoverThumbnailSettings_ZeroEvictsAndSurvivesRestart(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
	file := new(models.File)
	require.NoError(t, f.db.NewSelect().Model(file).Where("id = ?", seeded.fileID).Scan(f.ctx))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 600, 900))))
	require.NoError(t, os.WriteFile(covers.FileCoverPath(file), buf.Bytes(), 0644))
	coverPath := fmt.Sprintf("/api/books/%d/cover?size=256&aspect=book&r=1", seeded.bookID)
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
	f.request(http.MethodPut, "/api/settings/cache", `{"cover_thumbnail_max_size_gb":0}`, http.StatusOK)
	require.Zero(t, count(), "lowering the limit evicts existing thumbnails immediately")
	server, err := New(newPermissionTestConfig(t), f.db, nil, nil, nil, nil, downloadcache.NewCache(t.TempDir(), 1<<30), nil, nil, nil)
	require.NoError(t, err)
	f.handler = server.Handler
	f.request(http.MethodGet, coverPath, "", http.StatusOK)
	require.Zero(t, count(), "a zero limit must still apply after restart")
	require.FileExists(t, covers.FileCoverPath(file))
}

func TestCoverThumbnailSettings_PermissionsValidationAndDemoMode(t *testing.T) {
	t.Parallel()
	f := newSharingSettingsFixture(t, false)
	rec := f.do(t, f.configReader, http.MethodGet, "/api/settings/cache", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	for _, user := range []*models.User{f.configReader, f.viewer} {
		rec = f.do(t, user, http.MethodPut, "/api/settings/cache", `{"cover_thumbnail_max_size_gb":0}`)
		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	}
	rec = f.do(t, f.viewer, http.MethodGet, "/api/settings/cache", "")
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	for _, body := range []string{`{"cover_thumbnail_max_size_gb":-1}`, `{"cover_thumbnail_max_size_gb":1025}`} {
		rec = f.do(t, f.admin, http.MethodPut, "/api/settings/cache", body)
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	}
	rec = f.do(t, f.admin, http.MethodGet, "/api/settings/cache", "")
	require.JSONEq(t, `{"cover_thumbnail_max_size_gb":1}`, rec.Body.String())
	demo := newSharingSettingsFixture(t, true)
	rec = demo.do(t, demo.admin, http.MethodPut, "/api/settings/cache", `{"cover_thumbnail_max_size_gb":2}`)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	rec = demo.do(t, demo.admin, http.MethodGet, "/api/settings/cache", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
