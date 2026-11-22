package downloadcache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeAccessed(t *testing.T, dir string, at time.Time) {
	t.Helper()
	require.NoError(t, WriteMetadata(dir, &CacheMetadata{
		FileID:            1,
		DeliveredFileType: "epub",
		SizeBytes:         5,
		LastAccessedAt:    at,
	}))
}

func lastAccessed(t *testing.T, dir string) time.Time {
	t.Helper()
	meta, err := ReadMetadata(dir, 1, "epub")
	require.NoError(t, err)
	require.NotNil(t, meta)
	return meta.LastAccessedAt
}

// The web reader reads a book a part at a time, each part a cache hit, so a
// chapter of photos is dozens of hits at once.
func TestUpdateLastAccessed_LeavesARecentAccess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	recent := time.Now().Add(-10 * time.Second)
	writeAccessed(t, dir, recent)

	require.NoError(t, UpdateLastAccessed(dir, 1, "epub"))

	assert.WithinDuration(t, recent, lastAccessed(t, dir), time.Millisecond)
}

func TestUpdateLastAccessed_RecordsAnOlderAccess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeAccessed(t, dir, time.Now().Add(-2*time.Minute))

	require.NoError(t, UpdateLastAccessed(dir, 1, "epub"))

	assert.WithinDuration(t, time.Now(), lastAccessed(t, dir), 5*time.Second)
}
