//go:build unix

package downloadcache

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/shishobooks/shisho/pkg/filegen/filegentest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A cache directory that cannot be written does not break downloads: each
// one streams the Generated File straight to the client, builds its own
// copy, and logs a warning. Not parallel: it captures the process-wide log.
func TestCache_ReadOnlyCacheStillStreams(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes to read-only directories")
	}
	warnings := captureWarnings(t)
	gen := filegentest.NewPausing("first,", "second")
	cache, book, file := pausingCache(t, gen)
	require.NoError(t, os.Chmod(cache.Dir(), 0o555))
	t.Cleanup(func() { _ = os.Chmod(cache.Dir(), 0o755) })
	ctx := context.Background()

	head, err := cache.Head(ctx, book, file, file.FileType)
	require.NoError(t, err)
	assert.Equal(t, int64(len(gen.Output())), head.Size)

	a, err := cache.Open(ctx, book, file, file.FileType)
	require.NoError(t, err)
	require.NotNil(t, a.Body)
	defer a.Body.Close()
	assert.Equal(t, int64(len(gen.Output())), a.Size)
	assert.Equal(t, "first,", readN(t, a.Body, len(gen.First)))
	b, err := cache.Open(ctx, book, file, file.FileType)
	require.NoError(t, err)
	defer b.Body.Close()
	assert.Equal(t, "first,", readN(t, b.Body, len(gen.First)))

	gen.Release()
	restA, err := io.ReadAll(a.Body)
	require.NoError(t, err)
	restB, err := io.ReadAll(b.Body)
	require.NoError(t, err)
	assert.Equal(t, "second", string(restA))
	assert.Equal(t, "second", string(restB))
	cache.Wait()

	assert.Equal(t, 2, gen.Writes(), "without a cache each download builds its own copy")
	entries, err := os.ReadDir(cache.Dir())
	require.NoError(t, err)
	assert.Empty(t, entries)
	lines := warnings()
	require.NotEmpty(t, lines)
	assert.Contains(t, lines[0].Message, "cache")
	assert.EqualValues(t, file.ID, lines[0].Data["file_id"])
}
