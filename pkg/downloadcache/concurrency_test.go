package downloadcache

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readZipEntry returns the contents of name inside the zip at path. It fails
// the test if the zip is truncated or the entry is missing.
func readZipEntry(t *testing.T, path, name string) string {
	t.Helper()
	r, err := zip.OpenReader(path)
	require.NoError(t, err, "cached file %s is not a complete zip", path)
	defer r.Close()
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		defer rc.Close()
		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		return string(data)
	}
	t.Fatalf("entry %s not found in %s", name, path)
	return ""
}

func TestCache_GetOrGenerate_ConcurrentColdReads(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0755))

	srcPath := filepath.Join(tmpDir, "source.epub")
	createTestEPUB(t, srcPath, "Original Title", []string{"Original Author"})

	cache := NewCache(cacheDir, 1<<30)
	t.Cleanup(cache.Wait)

	book := &models.Book{Title: "Shared Title"}
	file := &models.File{ID: 7, FileType: models.FileTypeEPUB, Filepath: srcPath}

	const readers = 16
	start := make(chan struct{})
	paths := make([]string, readers)
	errs := make([]error, readers)
	var wg sync.WaitGroup
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			paths[i], _, errs[i] = cache.GetOrGenerate(context.Background(), book, file)
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 0; i < readers; i++ {
		require.NoError(t, errs[i], "reader %d", i)
		assert.Contains(t, readZipEntry(t, paths[i], "content.opf"), "Shared Title")
	}

	meta, err := ReadMetadata(cacheDir, file.ID)
	require.NoError(t, err)
	require.NotNil(t, meta)

	// A later read is a cache hit on the same generation.
	path, _, err := cache.GetOrGenerate(context.Background(), book, file)
	require.NoError(t, err)
	assert.Equal(t, paths[0], path)
	after, err := ReadMetadata(cacheDir, file.ID)
	require.NoError(t, err)
	assert.True(t, meta.GeneratedAt.Equal(after.GeneratedAt))

	cache.Wait() // background cleanup holds a .cleanup.lock file while it runs
	assertNoStagingLeftovers(t, cacheDir)
}

func TestCache_GetOrGenerate_FailureLeavesNoPartialOutput(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0755))

	// Not a zip, so the EPUB generator fails after the cache has started.
	srcPath := filepath.Join(tmpDir, "broken.epub")
	require.NoError(t, os.WriteFile(srcPath, []byte("not a zip"), 0600))

	cache := NewCache(cacheDir, 1<<30)
	t.Cleanup(cache.Wait)

	file := &models.File{ID: 3, FileType: models.FileTypeEPUB, Filepath: srcPath}
	_, _, err := cache.GetOrGenerate(context.Background(), &models.Book{Title: "x"}, file)
	require.Error(t, err)

	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// assertNoStagingLeftovers fails if anything other than published cache
// entries remains in dir.
func assertNoStagingLeftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		name := e.Name()
		assert.False(t, e.IsDir(), "unexpected directory %s", name)
		assert.False(t, strings.HasSuffix(name, ".tmp"), "unexpected temp file %s", name)
		assert.False(t, strings.HasPrefix(name, "."), "unexpected hidden file %s", name)
	}
}

// blockingOps returns cacheOps backed by an in-memory metadata slot whose
// generate step signals started, then blocks until proceed is closed.
func blockingOps(dir string, fileID int, started chan<- struct{}, proceed <-chan struct{}, calls *int32) cacheOps {
	var mu sync.Mutex
	published := false
	dest := cachedFilename(dir, fileID, "bin")
	return cacheOps{
		lookup: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			if published {
				return dest, nil
			}
			return "", nil
		},
		touch: func() {},
		generate: func(stagedPath string) error {
			atomic.AddInt32(calls, 1)
			f, err := os.Create(stagedPath)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := f.WriteString("partial"); err != nil {
				return err
			}
			started <- struct{}{}
			<-proceed
			_, err = f.WriteString("+complete")
			return err
		},
		writeMeta: func(*CacheMetadata) error {
			mu.Lock()
			defer mu.Unlock()
			published = true
			return nil
		},
		meta: CacheMetadata{FileID: fileID},
	}
}

func TestCache_getOrGenerate_WaitersReuseSingleGeneration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cache := NewCache(dir, 1<<30)
	t.Cleanup(cache.Wait)

	started := make(chan struct{}, 4)
	proceed := make(chan struct{})
	var calls int32
	ops := blockingOps(dir, 1, started, proceed, &calls)
	dest := cachedFilename(dir, 1, "bin")

	type result struct {
		path string
		err  error
	}
	results := make(chan result, 3)
	run := func() {
		p, err := cache.getOrGenerate(context.Background(), dest, ops)
		results <- result{p, err}
	}

	go run()
	<-started // the first caller is mid-write

	// The published path must not exist while generation is in flight.
	_, err := os.Stat(dest)
	assert.True(t, os.IsNotExist(err), "partial output was visible at the published path")

	go run()
	go run()
	// Both waiters must be queued on the lock before the leader publishes, so
	// they exercise the recheck under the lock rather than the fast-path hit.
	require.Eventually(t, func() bool { return cache.generating.refs(dest) == 3 }, 5*time.Second, time.Millisecond)
	close(proceed)

	for i := 0; i < 3; i++ {
		r := <-results
		require.NoError(t, r.err)
		data, err := os.ReadFile(r.path)
		require.NoError(t, err)
		assert.Equal(t, "partial+complete", string(data))
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
	cache.Wait()
	assertNoStagingLeftovers(t, dir)
}

func TestCache_getOrGenerate_CanceledWaiterReturnsPromptly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cache := NewCache(dir, 1<<30)
	t.Cleanup(cache.Wait)

	started := make(chan struct{}, 4)
	proceed := make(chan struct{})
	var calls int32
	ops := blockingOps(dir, 1, started, proceed, &calls)
	dest := cachedFilename(dir, 1, "bin")

	done := make(chan error, 1)
	go func() {
		_, err := cache.getOrGenerate(context.Background(), dest, ops)
		done <- err
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := cache.getOrGenerate(ctx, dest, ops)
	require.ErrorIs(t, err, context.Canceled)

	close(proceed)
	require.NoError(t, <-done)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestCache_getOrGenerate_UnrelatedFilesDoNotWait(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cache := NewCache(dir, 1<<30)
	t.Cleanup(cache.Wait)

	started := make(chan struct{}, 4)
	proceed := make(chan struct{})
	var calls int32
	blocked := blockingOps(dir, 1, started, proceed, &calls)

	done := make(chan error, 1)
	go func() {
		_, err := cache.getOrGenerate(context.Background(), cachedFilename(dir, 1, "bin"), blocked)
		done <- err
	}()
	<-started

	// File 2 generates to completion while file 1 is still blocked.
	otherStarted := make(chan struct{}, 1)
	otherProceed := make(chan struct{})
	close(otherProceed)
	path, err := cache.getOrGenerate(context.Background(), cachedFilename(dir, 2, "bin"), blockingOps(dir, 2, otherStarted, otherProceed, &calls))
	require.NoError(t, err)
	assert.Equal(t, cachedFilename(dir, 2, "bin"), path)

	close(proceed)
	require.NoError(t, <-done)
}

func TestCache_getOrGenerate_GenerateErrorCleansStaging(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cache := NewCache(dir, 1<<30)
	t.Cleanup(cache.Wait)

	dest := cachedFilename(dir, 1, "bin")
	boom := errors.New("boom")
	_, err := cache.getOrGenerate(context.Background(), dest, cacheOps{
		lookup: func() (string, error) { return "", nil },
		touch:  func() {},
		generate: func(stagedPath string) error {
			require.NoError(t, os.WriteFile(stagedPath, []byte("half"), 0600))
			return boom
		},
		writeMeta: func(*CacheMetadata) error { return nil },
	})
	require.ErrorIs(t, err, boom)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// refs reports how many callers hold or wait for key. Test helper.
func (l *keyedLock) refs(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if slot, ok := l.slots[key]; ok {
		return slot.refs
	}
	return 0
}
