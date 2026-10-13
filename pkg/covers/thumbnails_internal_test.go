package covers

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/require"
)

// Ordinary covers resize one per CPU until their estimated memory fills the
// budget, so a host with many CPUs and no container CPU limit cannot decode
// enough covers at once to exceed a container's memory limit.
func TestThumbnailDecodeWeight_BoundsConcurrencyByCPUAndMemory(t *testing.T) {
	t.Parallel()
	atOnce := func(bytes, procs int64) int64 {
		return thumbnailDecodeBudget / thumbnailDecodeWeight(bytes, procs)
	}
	retail := thumbnailDecodeBytes(image.Config{ColorModel: color.YCbCrModel, Width: 1400, Height: 2100}, 4, ThumbnailSize512, ThumbnailAspectBook)
	require.Equal(t, int64(1), atOnce(retail, 1))
	require.Equal(t, int64(4), atOnce(retail, 4))
	require.Less(t, atOnce(retail, 64), int64(64), "memory, not CPUs, bounds a large host")
	require.Greater(t, atOnce(retail, 64), int64(4))
	largest := thumbnailDecodeBytes(image.Config{ColorModel: color.YCbCrModel, Width: 5600, Height: 5700}, 16, ThumbnailSize2048, "")
	require.Equal(t, int64(1), atOnce(largest, 64), "the largest covers resize alone")
}

// The estimate must cover what a resize really allocates, including the
// resampler's scratch buffer and formats whose decoders buffer more than the
// image, or the budget does not bound memory. Not parallel: it reads
// process-wide allocation counters.
func TestThumbnailDecodeBytes_CoversActualAllocations(t *testing.T) {
	paths, err := filepath.Glob("testdata/*")
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	cache := NewThumbnailCache(t.TempDir(), 1<<20)
	for _, path := range paths {
		f, err := os.Open(path)
		require.NoError(t, err)
		cfg, format, err := image.DecodeConfig(f)
		require.NoError(t, err)
		bpp := decodeBytesPerPixel(f, format, cfg.ColorModel)
		require.NoError(t, f.Close())
		for _, tc := range []struct {
			size   int
			aspect string
		}{{ThumbnailSize128, ThumbnailAspectBook}, {ThumbnailSize512, ThumbnailAspectBook}, {ThumbnailSize2048, ""}} {
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			_, err := cache.resize(context.Background(), path, tc.size, tc.aspect)
			runtime.ReadMemStats(&after)
			require.NoError(t, err)
			allocated := int64(after.TotalAlloc - before.TotalAlloc)
			require.LessOrEqual(t, allocated, thumbnailDecodeBytes(cfg, bpp, tc.size, tc.aspect), "%s at %d %q", path, tc.size, tc.aspect)
		}
	}
}

func TestDecodeBytesPerPixel_ReadsLayoutFromHeaders(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]int64{
		"baseline-420.jpg":           4,
		"progressive-444.jpg":        16,
		"progressive-cmyk.jpg":       26,
		"progressive-gray.jpg":       6,
		"progressive-rgb.jpg":        20,
		"interlaced-16bit-alpha.png": 20,
		"lossy-alpha.webp":           8,
	} {
		f, err := os.Open(filepath.Join("testdata", name))
		require.NoError(t, err)
		cfg, format, err := image.DecodeConfig(f)
		require.NoError(t, err)
		require.Equal(t, want, decodeBytesPerPixel(f, format, cfg.ColorModel), name)
		require.NoError(t, f.Close())
	}
}

func TestThumbnailCache_GenerationWaitsForDecodeBudget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	name := "cover.png"
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewGray(image.Rect(0, 0, 60, 90))))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0644))
	file := &models.File{ID: 1, FileType: models.FileTypeEPUB, Filepath: filepath.Join(dir, "book.epub"), CoverImageFilename: &name}
	cache := NewThumbnailCache(t.TempDir(), 1<<20)

	require.NoError(t, cache.decodes.Acquire(context.Background(), thumbnailDecodeBudget))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := cache.Get(ctx, file, 128)
	require.ErrorIs(t, err, context.DeadlineExceeded, "a full budget must hold generation back")

	cache.decodes.Release(thumbnailDecodeBudget)
	_, err = cache.Get(context.Background(), file, 128)
	require.NoError(t, err)
}
