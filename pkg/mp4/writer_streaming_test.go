package mp4_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/mp4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestM4BGenerator_MemoryDoesNotScaleWithMdat(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "dest.m4b")

	const mdatSize = int64(64 << 20)
	testgen.SkipIfNoFFmpeg(t)
	srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
		Title:     "Source Title",
		Duration:  1,
		Faststart: true,
	})
	testgen.ExpandLastMdatSparse(t, srcPath, mdatSize)

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	generator := &filegen.M4BGenerator{}
	book := &models.Book{Title: "Generated Title"}
	file := &models.File{FileType: models.FileTypeM4B, Filepath: srcPath}
	require.NoError(t, generator.Generate(context.Background(), srcPath, destPath, book, file))

	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc
	assert.Less(t, allocated, uint64(8<<20),
		"rewriting a sparse %d-byte mdat allocated %d bytes", mdatSize, allocated)

	srcInfo, err := os.Stat(srcPath)
	require.NoError(t, err)
	destInfo, err := os.Stat(destPath)
	require.NoError(t, err)
	sizeDelta := destInfo.Size() - srcInfo.Size()
	if sizeDelta < 0 {
		sizeDelta = -sizeDelta
	}
	assert.Less(t, sizeDelta, int64(1<<20), "metadata rewrite changed the media-sized portion")
	assertNoRewriteTemps(t, destPath)
}

func TestWriteToFileContext_CancellationRemovesPartialOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source.m4b")
	destPath := filepath.Join(dir, "dest.m4b")
	writeSparseM4B(t, srcPath, 64<<20)
	require.NoError(t, os.WriteFile(destPath, []byte("existing destination"), 0600))

	ctx := newCancelAfterTempProgressContext(destPath, 64<<10)
	err := mp4.WriteToFileContext(ctx, srcPath, destPath, &mp4.Metadata{})
	require.ErrorIs(t, err, context.Canceled)

	contents, readErr := os.ReadFile(destPath)
	require.NoError(t, readErr)
	assert.Equal(t, []byte("existing destination"), contents)
	assertNoRewriteTemps(t, destPath)
}

func TestWriteToFile_RejectsOversizedMoovWithoutAllocatingIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source.m4b")
	destPath := filepath.Join(dir, "dest.m4b")

	const moovSize = int64(257 << 20)
	file, err := os.OpenFile(srcPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	require.NoError(t, err)
	ftyp := testBox("ftyp", []byte("M4B "))
	_, err = file.Write(ftyp)
	require.NoError(t, err)
	moovHeader := make([]byte, 8)
	binary.BigEndian.PutUint32(moovHeader[:4], uint32(moovSize))
	copy(moovHeader[4:], "moov")
	_, err = file.Write(moovHeader)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(int64(len(ftyp))+moovSize))
	require.NoError(t, file.Close())

	err = mp4.WriteToFile(srcPath, destPath, &mp4.Metadata{})
	require.ErrorContains(t, err, "moov box exceeds")
	assert.NoFileExists(t, destPath)
	assertNoRewriteTemps(t, destPath)
}

func TestWriteToFile_PreservesLargeSizeBoxesAndOrdering(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source.m4b")
	destPath := filepath.Join(dir, "dest.m4b")

	source := append(testBox("ftyp", []byte("M4B ")), largeTestBox("skip", []byte("preserve me"))...)
	source = append(source, testBox("mdat", []byte("audio payload"))...)
	source = append(source, testBox("moov", testBox("free", []byte{0}))...)
	source = append(source, 0xAA, 0xBB, 0xCC)
	require.NoError(t, os.WriteFile(srcPath, source, 0600))

	require.NoError(t, mp4.WriteToFile(srcPath, destPath, &mp4.Metadata{}))
	destination, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, source, destination)
}

func TestWriteToFile_RejectsExcessiveTopLevelBoxCount(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source.m4b")
	destPath := filepath.Join(dir, "dest.m4b")

	freeBox := testBox("free", nil)
	source := bytes.Repeat(freeBox, 100_001)
	source = append(source, testBox("moov", testBox("free", []byte{0}))...)
	require.NoError(t, os.WriteFile(srcPath, source, 0600))

	err := mp4.WriteToFile(srcPath, destPath, &mp4.Metadata{})
	require.ErrorContains(t, err, "top-level box count exceeds")
	assert.NoFileExists(t, destPath)
}

func TestWrite_PreservesSourceFileMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "source.m4b")
	source := append(testBox("ftyp", []byte("M4B ")), testBox("moov", testBox("free", []byte{0}))...)
	source = append(source, testBox("mdat", []byte("audio payload"))...)
	require.NoError(t, os.WriteFile(path, source, 0640))

	require.NoError(t, mp4.Write(path, &mp4.Metadata{}, mp4.WriteOptions{}))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0640), info.Mode().Perm())
}

func TestWrite_PreservesSymlinkAndUpdatesTarget(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "target.m4b")
	linkPath := filepath.Join(dir, "link.m4b")
	source := append(testBox("ftyp", []byte("M4B ")), testBox("moov", testBox("free", []byte{0}))...)
	source = append(source, testBox("mdat", []byte("audio payload"))...)
	require.NoError(t, os.WriteFile(targetPath, source, 0600))
	require.NoError(t, os.Symlink(targetPath, linkPath))

	require.NoError(t, mp4.Write(linkPath, &mp4.Metadata{}, mp4.WriteOptions{}))
	linkInfo, err := os.Lstat(linkPath)
	require.NoError(t, err)
	assert.NotZero(t, linkInfo.Mode()&os.ModeSymlink)
	target, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	assert.Equal(t, source, target)
}

type cancelAfterTempProgressContext struct {
	context.Context
	pattern  string
	minSize  int64
	done     chan struct{}
	canceled bool
}

func newCancelAfterTempProgressContext(destPath string, minSize int64) *cancelAfterTempProgressContext {
	pattern := filepath.Join(filepath.Dir(destPath), "."+filepath.Base(destPath)+".tmp-*")
	return &cancelAfterTempProgressContext{
		Context: context.Background(),
		pattern: pattern,
		minSize: minSize,
		done:    make(chan struct{}),
	}
}

func (c *cancelAfterTempProgressContext) Done() <-chan struct{} { return c.done }

func (c *cancelAfterTempProgressContext) Err() error {
	if c.canceled {
		return context.Canceled
	}
	matches, _ := filepath.Glob(c.pattern)
	for _, match := range matches {
		if info, err := os.Stat(match); err == nil && info.Size() >= c.minSize {
			c.canceled = true
			close(c.done)
			return context.Canceled
		}
	}
	return nil
}

func assertNoRewriteTemps(t *testing.T, destPath string) {
	t.Helper()
	pattern := filepath.Join(filepath.Dir(destPath), "."+filepath.Base(destPath)+".tmp-*")
	matches, err := filepath.Glob(pattern)
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func writeSparseM4B(t *testing.T, path string, mdatSize int64) {
	t.Helper()

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	ftyp := testBox("ftyp", []byte("M4B "))
	moov := testBox("moov", testBox("free", []byte{0}))
	_, err = file.Write(append(ftyp, moov...))
	require.NoError(t, err)

	mdatHeader := make([]byte, 8)
	require.LessOrEqual(t, mdatSize, int64(^uint32(0)))
	binary.BigEndian.PutUint32(mdatHeader[:4], uint32(mdatSize))
	copy(mdatHeader[4:], "mdat")
	_, err = file.Write(mdatHeader)
	require.NoError(t, err)

	mdatStart := int64(len(ftyp) + len(moov))
	require.NoError(t, file.Truncate(mdatStart+mdatSize))
	require.NoError(t, file.Close())
}

func testBox(boxType string, content []byte) []byte {
	box := make([]byte, 8+len(content))
	binary.BigEndian.PutUint32(box[:4], uint32(len(box)))
	copy(box[4:8], boxType)
	copy(box[8:], content)
	return box
}

func largeTestBox(boxType string, content []byte) []byte {
	box := make([]byte, 16+len(content))
	binary.BigEndian.PutUint32(box[:4], 1)
	copy(box[4:8], boxType)
	binary.BigEndian.PutUint64(box[8:16], uint64(len(box)))
	copy(box[16:], content)
	return box
}
