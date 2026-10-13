package lrufiles

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clock is a settable time source, so tests can age entries past the window
// that protects recently used files.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestIndex(t *testing.T) (*Index, *clock, string) {
	t.Helper()
	root := t.TempDir()
	idx := New(root)
	clk := &clock{t: time.Now()}
	idx.now = clk.now
	return idx, clk, root
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, make([]byte, size), 0644))
}

// add writes a file and records it, the way a cache publishes one.
func add(t *testing.T, idx *Index, path string, size int) {
	t.Helper()
	writeFile(t, path, size)
	idx.Added(path)
}

func tracked(idx *Index, path string) bool {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	_, ok := idx.entries[path]
	return ok
}

func TestIndex_UnlimitedNeverEvicts(t *testing.T) {
	t.Parallel()
	idx, clk, root := newTestIndex(t)
	for i := range 5 {
		add(t, idx, filepath.Join(root, "1", string(rune('a'+i))), 100)
		clk.advance(time.Hour)
	}
	entries, err := os.ReadDir(filepath.Join(root, "1"))
	require.NoError(t, err)
	assert.Len(t, entries, 5)
}

func TestIndex_EvictsLeastRecentlyUsedAboveTheLimit(t *testing.T) {
	t.Parallel()
	idx, clk, root := newTestIndex(t)
	a, b, c := filepath.Join(root, "1", "a"), filepath.Join(root, "1", "b"), filepath.Join(root, "2", "c")
	require.NoError(t, idx.SetMaxBytes(250))
	add(t, idx, a, 100)
	clk.advance(time.Hour)
	add(t, idx, b, 100)
	clk.advance(time.Hour)
	idx.Used(a) // a is now more recent than b
	clk.advance(time.Hour)
	add(t, idx, c, 100)

	assert.FileExists(t, a)
	assert.NoFileExists(t, b, "the least recently used file is evicted first")
	assert.FileExists(t, c)
}

func TestIndex_LoweringTheLimitEvictsExistingFilesNow(t *testing.T) {
	t.Parallel()
	idx, clk, root := newTestIndex(t)
	// Files already on disk from an earlier run, oldest first by mtime.
	old, recent := filepath.Join(root, "1", "old"), filepath.Join(root, "1", "recent")
	writeFile(t, old, 100)
	writeFile(t, recent, 100)
	past := clk.now().Add(-3 * time.Hour)
	require.NoError(t, os.Chtimes(old, past, past))
	require.NoError(t, os.Chtimes(recent, past.Add(time.Hour), past.Add(time.Hour)))
	// Leftover temp files are not cache entries and are never counted.
	writeFile(t, filepath.Join(root, "1", ".extracting-page_0-123"), 1000)
	writeFile(t, filepath.Join(root, "1", "page_0.jpg.456.tmp"), 1000)

	require.NoError(t, idx.SetMaxBytes(150))

	assert.NoFileExists(t, old)
	assert.FileExists(t, recent)
}

func TestIndex_ZeroLimitKeepsRecentlyUsedFiles(t *testing.T) {
	t.Parallel()
	idx, clk, root := newTestIndex(t)
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	require.NoError(t, idx.SetMaxBytes(0))
	add(t, idx, a, 100)
	assert.FileExists(t, a, "a file just handed to a caller is not deleted before it is served")
	clk.advance(2 * time.Minute)
	add(t, idx, b, 100)
	assert.NoFileExists(t, a, "once out of the protection window it is evicted")
	assert.FileExists(t, b)
}

func TestIndex_ForgetAndResetDropRemovedFiles(t *testing.T) {
	t.Parallel()
	idx, clk, root := newTestIndex(t)
	require.NoError(t, idx.SetMaxBytes(1000))
	add(t, idx, filepath.Join(root, "2", "b"), 400)
	add(t, idx, filepath.Join(root, "1", "a"), 400)
	require.NoError(t, os.RemoveAll(filepath.Join(root, "1")))
	idx.Forget(filepath.Join(root, "1"))
	clk.advance(time.Hour)
	// Without Forget the removed 400 bytes would still count and evict b.
	add(t, idx, filepath.Join(root, "3", "c"), 400)
	assert.FileExists(t, filepath.Join(root, "2", "b"))

	require.NoError(t, os.RemoveAll(root))
	idx.Reset()
	clk.advance(time.Hour)
	add(t, idx, filepath.Join(root, "4", "d"), 900)
	assert.FileExists(t, filepath.Join(root, "4", "d"), "a reset index counts nothing from before the clear")
}

func TestIndex_UnlimitedNeverWalksButKeepsRecency(t *testing.T) {
	t.Parallel()
	idx, clk, root := newTestIndex(t)
	require.NoError(t, idx.SetMaxBytes(Unlimited))
	page := filepath.Join(root, "1", "a")
	add(t, idx, page, 100)
	past := clk.now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(page, past, past))

	assert.True(t, idx.Used(page))
	assert.False(t, idx.Used(filepath.Join(root, "1", "missing")), "a hit on a missing file reports a miss")
	assert.False(t, idx.loaded, "an unlimited cache has nothing to evict, so it never walks the directory")
	info, err := os.Stat(page)
	require.NoError(t, err)
	assert.WithinDuration(t, clk.now(), info.ModTime(), time.Second, "a hit still records recency on disk for a limit set later")
}

func TestIndex_UsedReportsAFileEvictedBeforeTheHitRecordedIt(t *testing.T) {
	t.Parallel()
	idx, clk, root := newTestIndex(t)
	a := filepath.Join(root, "a")
	require.NoError(t, idx.SetMaxBytes(150))
	add(t, idx, a, 100)
	clk.advance(time.Hour)
	add(t, idx, filepath.Join(root, "b"), 100)
	require.NoFileExists(t, a)
	assert.False(t, idx.Used(a), "the caller must regenerate a page evicted between its existence check and Used")
}

func TestIndex_AddedIgnoresAFileAlreadyRemoved(t *testing.T) {
	t.Parallel()
	idx, _, root := newTestIndex(t)
	require.NoError(t, idx.SetMaxBytes(1000))
	missing := filepath.Join(root, "gone")
	idx.Added(missing)
	assert.False(t, tracked(idx, missing), "a page cleared before it was recorded must not count toward the limit")
}

func TestIndex_ForgetKeepsAPageRewrittenDuringTheRemoval(t *testing.T) {
	t.Parallel()
	idx, _, root := newTestIndex(t)
	require.NoError(t, idx.SetMaxBytes(1000))
	dir := filepath.Join(root, "1")
	page := filepath.Join(dir, "a")
	add(t, idx, page, 100)
	// An extraction that started before the invalidation publishes after
	// RemoveAll but before Forget runs.
	require.NoError(t, os.RemoveAll(dir))
	add(t, idx, page, 100)
	idx.Forget(dir)
	assert.True(t, tracked(idx, page), "a page still on disk stays counted")
}

func TestIndex_ResetRescansPagesPublishedDuringTheClear(t *testing.T) {
	t.Parallel()
	idx, _, root := newTestIndex(t)
	require.NoError(t, idx.SetMaxBytes(1000))
	raced := filepath.Join(root, "1", "a")
	add(t, idx, raced, 100)
	idx.Reset() // the file survived the clear, as when it was renamed into place after RemoveAll
	add(t, idx, filepath.Join(root, "2", "b"), 100)
	assert.True(t, tracked(idx, raced), "a page still on disk after a clear is counted again")
}

func TestIndex_FutureMtimeDoesNotBlockEviction(t *testing.T) {
	t.Parallel()
	idx, clk, root := newTestIndex(t)
	page := filepath.Join(root, "a")
	writeFile(t, page, 100)
	future := clk.now().Add(time.Hour)
	require.NoError(t, os.Chtimes(page, future, future))
	require.NoError(t, idx.SetMaxBytes(0))
	clk.advance(2 * time.Minute)
	require.NoError(t, idx.SetMaxBytes(0))
	assert.NoFileExists(t, page, "a clock set back after a file was written must not protect it for the gap")
}

func TestIndex_UsedAfterResetCountsThePageOnceAndKeepsIt(t *testing.T) {
	t.Parallel()
	idx, clk, root := newTestIndex(t)
	require.NoError(t, idx.SetMaxBytes(1000))
	idx.Reset()
	page := filepath.Join(root, "1", "a")
	writeFile(t, page, 100)
	past := clk.now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(page, past, past))

	require.True(t, idx.Used(page))
	idx.mu.Lock()
	total, n := idx.total, idx.lru.Len()
	idx.mu.Unlock()
	assert.Equal(t, int64(100), total, "the walk the hit triggered already counted the page")
	assert.Equal(t, 1, n)
	add(t, idx, filepath.Join(root, "2", "b"), 850)
	assert.FileExists(t, page, "a page a reader just fetched survives the next eviction")
}
