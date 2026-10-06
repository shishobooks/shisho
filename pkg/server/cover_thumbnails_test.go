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
