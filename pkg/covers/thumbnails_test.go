package covers_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/require"
)

func coverFixture(t *testing.T, id int, shade color.NRGBA) *models.File {
	t.Helper()
	dir := t.TempDir()
	name := "cover.png"
	img := image.NewNRGBA(image.Rect(0, 0, 600, 900))
	for y := range 900 {
		for x := range 600 {
			img.SetNRGBA(x, y, shade)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0644))
	return &models.File{ID: id, FileType: models.FileTypeEPUB, Filepath: filepath.Join(dir, "book.epub"), CoverImageFilename: &name}
}

func TestServeBookCover_ThumbnailRequestAndConditionalGet(t *testing.T) {
	t.Parallel()
	cache := covers.NewThumbnailCache(t.TempDir(), 1<<20)
	file := coverFixture(t, 1, color.NRGBA{A: 255})
	e := echo.New()
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	e.GET("/cover", func(c echo.Context) error {
		return covers.ServeBookCover(c, []*models.File{file}, "book", covers.CacheControlImmutable, "Cover", cache)
	})
	request := func(query, etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/cover"+query, nil)
		req.Header.Set("If-None-Match", etag)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	first := request("?size=256&r=1", "")
	require.Equal(t, http.StatusOK, first.Code)
	img, err := png.Decode(bytes.NewReader(first.Body.Bytes()))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 171, 256), img.Bounds())
	require.Equal(t, covers.CacheControlImmutable, first.Header().Get("Cache-Control"))
	etag := first.Header().Get("ETag")
	require.NotEmpty(t, etag)
	require.Equal(t, http.StatusNotModified, request("?size=256&r=1", etag).Code)
	require.NotEqual(t, etag, request("?size=512&r=1", "").Header().Get("ETag"))
	original := request("", "")
	img, err = png.Decode(bytes.NewReader(original.Body.Bytes()))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 600, 900), img.Bounds())
	invalid := request("?size=300&r=1", "")
	require.Equal(t, http.StatusUnprocessableEntity, invalid.Code)
	require.Empty(t, invalid.Header().Get("Cache-Control"))
}

func TestServeFileCover_PreservesUnsupportedCoverFormats(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	name := "cover.svg"
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"><rect width="100" height="100" fill="red"/></svg>`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0644))
	file := &models.File{ID: 1, Filepath: filepath.Join(dir, "book.epub"), CoverImageFilename: &name}
	cache := covers.NewThumbnailCache(t.TempDir(), 1<<20)
	e := echo.New()
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	e.GET("/cover", func(c echo.Context) error {
		return covers.ServeFileCover(c, file, covers.CacheControlImmutable, "Cover", cache)
	})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/cover?size=256&r=1", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, data, rec.Body.Bytes())
	require.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
}

func TestServeFileCover_ThumbnailIgnoresUnknownRangeUnits(t *testing.T) {
	t.Parallel()
	file := coverFixture(t, 1, color.NRGBA{A: 255})
	cache := covers.NewThumbnailCache(t.TempDir(), 1<<20)
	e := echo.New()
	e.GET("/cover", func(c echo.Context) error {
		return covers.ServeFileCover(c, file, covers.CacheControlImmutable, "Cover", cache)
	})
	req := httptest.NewRequest(http.MethodGet, "/cover?size=256&r=1", nil)
	req.Header.Set("Range", "items=0-1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	_, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
	require.NoError(t, err)
}

func TestServeFileCover_ThumbnailFailuresRemainUncacheable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		missing bool
		status  int
	}{
		{"missing source", true, http.StatusNotFound},
		{"blocked cache destination", false, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file := coverFixture(t, 1, color.NRGBA{A: 255})
			dir := t.TempDir()
			if tc.missing {
				require.NoError(t, os.Remove(covers.FileCoverPath(file)))
			} else {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "cover-thumbnails"), []byte("blocked"), 0644))
			}
			cache := covers.NewThumbnailCache(dir, 1<<20)
			e := echo.New()
			e.HTTPErrorHandler = errcodes.NewHandler().Handle
			e.GET("/cover", func(c echo.Context) error {
				return covers.ServeFileCover(c, file, covers.CacheControlImmutable, "Cover", cache)
			})
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/cover?size=256&r=1", nil))
			require.Equal(t, tc.status, rec.Code)
			require.Empty(t, rec.Header().Get("Cache-Control"))
			require.Empty(t, rec.Header().Get("ETag"))
			require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
		})
	}
}

func TestServeFileCover_ThumbnailFitsTheCoverFrame(t *testing.T) {
	t.Parallel()
	file := coverFixture(t, 1, color.NRGBA{A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 1200, 900))))
	require.NoError(t, os.WriteFile(covers.FileCoverPath(file), buf.Bytes(), 0644))
	cache := covers.NewThumbnailCache(t.TempDir(), 1<<20)
	e := echo.New()
	e.GET("/cover", func(c echo.Context) error {
		return covers.ServeFileCover(c, file, covers.CacheControlImmutable, "Cover", cache)
	})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/cover?size=256&aspect=book&r=1", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 171, 256), img.Bounds())
}

func TestThumbnailCache_RemovesOldCoverVersions(t *testing.T) {
	t.Parallel()
	cache := covers.NewThumbnailCache(t.TempDir(), 1<<20)
	file := coverFixture(t, 1, color.NRGBA{R: 150, A: 255})
	first, err := cache.Get(context.Background(), file, 256)
	require.NoError(t, err)
	info, err := os.Stat(covers.FileCoverPath(file))
	require.NoError(t, err)
	require.NoError(t, os.Chtimes(covers.FileCoverPath(file), info.ModTime(), info.ModTime().Add(time.Nanosecond)))
	second, err := cache.Get(context.Background(), file, 256)
	require.NoError(t, err)
	require.NotEqual(t, first.Key, second.Key)
	_, count, err := cache.SizeBytes()
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestThumbnailCache_ReusesThumbnailAfterMetadataOnlyChange(t *testing.T) {
	t.Parallel()
	cache := covers.NewThumbnailCache(t.TempDir(), 1<<20)
	file := coverFixture(t, 1, color.NRGBA{A: 255})
	first, err := cache.Get(context.Background(), file, 256)
	require.NoError(t, err)
	file.UpdatedAt = time.Now()
	second, err := cache.Get(context.Background(), file, 256)
	require.NoError(t, err)
	require.Equal(t, first.Key, second.Key)
}

func TestThumbnailCache_EvictsLeastRecentlyUsedThumbnails(t *testing.T) {
	t.Parallel()
	files := []*models.File{
		coverFixture(t, 1, color.NRGBA{R: 150, A: 255}),
		coverFixture(t, 2, color.NRGBA{R: 150, A: 255}),
		coverFixture(t, 3, color.NRGBA{R: 150, A: 255}),
	}
	probe := covers.NewThumbnailCache(t.TempDir(), 1<<20)
	thumb, err := probe.Get(context.Background(), files[0], 256)
	require.NoError(t, err)
	dir := t.TempDir()
	cache := covers.NewThumbnailCache(dir, int64(2*len(thumb.Data)))
	first, err := cache.Get(context.Background(), files[0], 256)
	require.NoError(t, err)
	second, err := cache.Get(context.Background(), files[1], 256)
	require.NoError(t, err)
	_, err = cache.Get(context.Background(), files[0], 256)
	require.NoError(t, err)
	_, err = cache.Get(context.Background(), files[2], 256)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dir, "cover-thumbnails", first.Key+".png"))
	require.NoFileExists(t, filepath.Join(dir, "cover-thumbnails", second.Key+".png"))
	bytes, count, err := cache.SizeBytes()
	require.NoError(t, err)
	require.LessOrEqual(t, bytes, int64(2*len(thumb.Data)))
	require.Equal(t, 2, count)
}

func TestThumbnailCache_RejectsArbitrarySizes(t *testing.T) {
	t.Parallel()
	cache := covers.NewThumbnailCache(t.TempDir(), 1<<20)
	file := coverFixture(t, 1, color.NRGBA{A: 255})
	_, err := cache.Get(context.Background(), file, 300)
	require.ErrorIs(t, err, covers.ErrInvalidThumbnailSize)
	_, count, err := cache.SizeBytes()
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestThumbnailCache_ClearAllowsRegeneration(t *testing.T) {
	t.Parallel()
	cache := covers.NewThumbnailCache(t.TempDir(), 1<<20)
	file := coverFixture(t, 1, color.NRGBA{A: 255})
	first, err := cache.Get(context.Background(), file, 256)
	require.NoError(t, err)
	require.NoError(t, cache.Clear())
	_, count, err := cache.SizeBytes()
	require.NoError(t, err)
	require.Zero(t, count)
	second, err := cache.Get(context.Background(), file, 256)
	require.NoError(t, err)
	require.Equal(t, first, second)
}

// A context can pause a generation after it enters the resize queue, allowing
// the test to clear the cache while another request is waiting on that work.
type pausedThumbnailContext struct {
	context.Context
	entered atomic.Bool
	paused  chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *pausedThumbnailContext) Done() <-chan struct{} {
	c.entered.Store(true)
	return c.Context.Done()
}

func (c *pausedThumbnailContext) Err() error {
	if c.entered.Load() {
		c.once.Do(func() { close(c.paused); <-c.release })
	}
	return c.Context.Err()
}

type waitingThumbnailContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *waitingThumbnailContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestThumbnailCache_ClearDoesNotRepublishFromEarlierWaiters(t *testing.T) {
	t.Parallel()
	cache := covers.NewThumbnailCache(t.TempDir(), 1<<20)
	file := coverFixture(t, 1, color.NRGBA{A: 255})
	leader := &pausedThumbnailContext{Context: context.Background(), paused: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(leader.release) }) })
	results := make(chan error, 2)
	go func() { _, err := cache.Get(leader, file, 256); results <- err }()
	select {
	case <-leader.paused:
	case <-time.After(5 * time.Second):
		t.Fatal("generation did not start")
	}
	waiter := &waitingThumbnailContext{Context: context.Background(), waiting: make(chan struct{})}
	go func() { _, err := cache.Get(waiter, file, 256); results <- err }()
	select {
	case <-waiter.waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("duplicate request did not wait")
	}
	require.NoError(t, cache.Clear())
	release.Do(func() { close(leader.release) })
	for range 2 {
		require.NoError(t, <-results)
	}
	_, count, err := cache.SizeBytes()
	require.NoError(t, err)
	require.Zero(t, count, "requests started before Clear must not repopulate the cache")
	_, err = cache.Get(context.Background(), file, 256)
	require.NoError(t, err)
	_, count, err = cache.SizeBytes()
	require.NoError(t, err)
	require.Equal(t, 1, count, "later requests can generate a fresh thumbnail")
}

func TestThumbnailCache_DoesNotPublishCanceledGeneration(t *testing.T) {
	t.Parallel()
	cache := covers.NewThumbnailCache(t.TempDir(), 1<<20)
	file := coverFixture(t, 1, color.NRGBA{A: 255})
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err := cache.Get(ctx, file, 256)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	_, count, err := cache.SizeBytes()
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestThumbnailCache_GeneratesOnlyRequestedSizeAndReusesIt(t *testing.T) {
	t.Parallel()
	cache := covers.NewThumbnailCache(t.TempDir(), 1<<20)
	_, count, err := cache.SizeBytes()
	require.NoError(t, err)
	require.Zero(t, count)

	file := coverFixture(t, 1, color.NRGBA{R: 150, A: 255})
	first, err := cache.Get(context.Background(), file, 256)
	require.NoError(t, err)
	img, err := png.Decode(bytes.NewReader(first.Data))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 171, 256), img.Bounds())
	_, count, err = cache.SizeBytes()
	require.NoError(t, err)
	require.Equal(t, 1, count)

	second, err := cache.Get(context.Background(), file, 256)
	require.NoError(t, err)
	require.Equal(t, first, second)
	_, count, err = cache.SizeBytes()
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestThumbnailCache_ConcurrentRequestsAndRestartReuseOneCompleteFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cache := covers.NewThumbnailCache(dir, 1<<20)
	file := coverFixture(t, 1, color.NRGBA{R: 150, A: 255})
	results := make([]covers.Thumbnail, 20)
	errs := make([]error, 20)
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() { results[i], errs[i] = cache.Get(context.Background(), file, 256) })
	}
	wg.Wait()
	for i := range results {
		require.NoError(t, errs[i])
		require.Equal(t, results[0], results[i])
	}
	_, count, err := cache.SizeBytes()
	require.NoError(t, err)
	require.Equal(t, 1, count)
	path := filepath.Join(dir, "cover-thumbnails", results[0].Key+".png")
	before, err := os.Stat(path)
	require.NoError(t, err)
	restarted := covers.NewThumbnailCache(dir, 1<<20)
	thumb, err := restarted.Get(context.Background(), file, 256)
	require.NoError(t, err)
	require.Equal(t, results[0], thumb)
	after, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, before.ModTime(), after.ModTime(), "reuse must not regenerate the PNG")
}

func TestThumbnailCache_AveragesFineDetailRatherThanDroppingPixels(t *testing.T) {
	t.Parallel()
	file := coverFixture(t, 1, color.NRGBA{A: 255})
	img := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	for y := range 512 {
		for x := range 512 {
			shade := uint8(0)
			if x%2 == 0 {
				shade = 255
			}
			img.SetNRGBA(x, y, color.NRGBA{R: shade, G: shade, B: shade, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	require.NoError(t, os.WriteFile(covers.FileCoverPath(file), buf.Bytes(), 0644))
	cache := covers.NewThumbnailCache(t.TempDir(), 1<<20)
	thumb, err := cache.Get(context.Background(), file, 128)
	require.NoError(t, err)
	resized, err := png.Decode(bytes.NewReader(thumb.Data))
	require.NoError(t, err)
	shade := color.NRGBAModel.Convert(resized.At(64, 64)).(color.NRGBA)
	// Equal black and white stripes reduce to gray. Point sampling would
	// instead return black or white and break thin lettering into dots.
	require.InDelta(t, 127.5, shade.R, 2)
	require.InDelta(t, 127.5, shade.G, 2)
	require.InDelta(t, 127.5, shade.B, 2)
}
