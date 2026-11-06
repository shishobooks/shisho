package downloadcache

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/fingerprint"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedHash struct {
	fileID        int
	deliveredType string
	hash          string
}

// hashRecorder stands in for the database, optionally failing.
type hashRecorder struct {
	mu   sync.Mutex
	got  []recordedHash
	fail bool
}

func (r *hashRecorder) record(_ context.Context, fileID int, deliveredType, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errors.New("database is unavailable")
	}
	r.got = append(r.got, recordedHash{fileID, deliveredType, hash})
	return nil
}

func (r *hashRecorder) recorded() []recordedHash {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedHash(nil), r.got...)
}

func documentHashFixture(t *testing.T) (*models.Book, *models.File) {
	t.Helper()
	dir := t.TempDir()
	srcPath := testgen.GenerateEPUB(t, dir, "source.epub", testgen.EPUBOptions{Title: "Original"})
	book := &models.Book{ID: 1, Title: "Original", Filepath: dir}
	file := &models.File{ID: 7, BookID: 1, Filepath: srcPath, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain}
	return book, file
}

func TestCache_RecordsDocumentHashAtPublish(t *testing.T) {
	t.Parallel()
	book, file := documentHashFixture(t)
	recorder := &hashRecorder{}
	cache := NewCache(t.TempDir(), 1<<30, WithDocumentHashRecorder(recorder.record))
	t.Cleanup(cache.Wait)

	path, _, err := cache.GetOrGenerate(context.Background(), book, file, models.FileTypeEPUB)
	require.NoError(t, err)

	want, err := fingerprint.ComputeKOReaderPartialMD5(path)
	require.NoError(t, err)
	assert.Equal(t, []recordedHash{{file.ID, models.FileTypeEPUB, want}}, recorder.recorded())

	// A cache hit serves the same bytes, already recorded.
	_, _, err = cache.GetOrGenerate(context.Background(), book, file, models.FileTypeEPUB)
	require.NoError(t, err)
	assert.Len(t, recorder.recorded(), 1)
}

func TestCache_EveryMetadataEditRecordsItsOwnHash(t *testing.T) {
	t.Parallel()
	book, file := documentHashFixture(t)
	recorder := &hashRecorder{}
	cache := NewCache(t.TempDir(), 1<<30, WithDocumentHashRecorder(recorder.record))
	t.Cleanup(cache.Wait)

	_, _, err := cache.GetOrGenerate(context.Background(), book, file, models.FileTypeEPUB)
	require.NoError(t, err)

	book.Title = "Edited"
	_, _, err = cache.GetOrGenerate(context.Background(), book, file, models.FileTypeEPUB)
	require.NoError(t, err)

	got := recorder.recorded()
	require.Len(t, got, 2)
	assert.NotEqual(t, got[0].hash, got[1].hash)
}

// A file already in the cache when Shisho learned to hash, or whose hash
// failed to record, gets recorded the next time it's served.
func TestCache_CacheHitRecordsAHashNotYetRecorded(t *testing.T) {
	t.Parallel()
	book, file := documentHashFixture(t)
	dir := t.TempDir()
	recorder := &hashRecorder{fail: true}
	cache := NewCache(dir, 1<<30, WithDocumentHashRecorder(recorder.record))
	t.Cleanup(cache.Wait)

	path, _, err := cache.GetOrGenerate(context.Background(), book, file, models.FileTypeEPUB)
	require.NoError(t, err, "a hash that fails to record doesn't fail the download")
	require.FileExists(t, path)
	meta, err := ReadMetadata(dir, file.ID, models.FileTypeEPUB)
	require.NoError(t, err)
	assert.Empty(t, meta.DocumentHash)

	recorder.mu.Lock()
	recorder.fail = false
	recorder.mu.Unlock()

	_, _, err = cache.GetOrGenerate(context.Background(), book, file, models.FileTypeEPUB)
	require.NoError(t, err)
	want, err := fingerprint.ComputeKOReaderPartialMD5(path)
	require.NoError(t, err)
	assert.Equal(t, []recordedHash{{file.ID, models.FileTypeEPUB, want}}, recorder.recorded())
	meta, err = ReadMetadata(dir, file.ID, models.FileTypeEPUB)
	require.NoError(t, err)
	assert.Equal(t, want, meta.DocumentHash)
}

func TestCache_OpenRecordsDocumentHash(t *testing.T) {
	t.Parallel()
	book, file := documentHashFixture(t)
	recorder := &hashRecorder{}
	dir := t.TempDir()
	cache := NewCache(dir, 1<<30, WithDocumentHashRecorder(recorder.record))
	t.Cleanup(cache.Wait)

	generated, err := cache.Open(context.Background(), book, file, models.FileTypeEPUB)
	require.NoError(t, err)
	if generated.Body != nil {
		_, err := io.Copy(io.Discard, generated.Body)
		require.NoError(t, err)
		require.NoError(t, generated.Body.Close())
	}
	cache.Wait()

	want, err := fingerprint.ComputeKOReaderPartialMD5(cachedFilename(dir, file.ID, models.FileTypeEPUB))
	require.NoError(t, err)
	assert.Equal(t, []recordedHash{{file.ID, models.FileTypeEPUB, want}}, recorder.recorded())
}
