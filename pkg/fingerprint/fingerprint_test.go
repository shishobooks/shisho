package fingerprint

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComputeSHA256_FixedContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")
	content := []byte("hello shisho")
	require.NoError(t, os.WriteFile(path, content, 0o644))

	expected := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(expected[:])

	got, err := ComputeSHA256(path)
	require.NoError(t, err)
	require.Equal(t, expectedHex, got)
}

func TestComputeSHA256_EmptyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "empty.bin")
	require.NoError(t, os.WriteFile(path, nil, 0o644))

	// sha256 of empty content
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	got, err := ComputeSHA256(path)
	require.NoError(t, err)
	require.Equal(t, expected, got)
}

func TestComputeSHA256_LargeFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "large.bin")

	f, err := os.Create(path)
	require.NoError(t, err)

	// Write 5 MB of pseudo-random data.
	r := rand.New(rand.NewSource(42))
	buf := make([]byte, 1024*1024)
	h := sha256.New()
	for i := 0; i < 5; i++ {
		_, _ = r.Read(buf)
		_, err := f.Write(buf)
		require.NoError(t, err)
		_, _ = h.Write(buf)
	}
	require.NoError(t, f.Close())
	expectedHex := hex.EncodeToString(h.Sum(nil))

	got, err := ComputeSHA256(path)
	require.NoError(t, err)
	require.Equal(t, expectedHex, got)
}

func TestComputeSHA256_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := ComputeSHA256(filepath.Join(t.TempDir(), "does-not-exist"))
	require.Error(t, err)
}

// Sanity: ComputeSHA256 must match crypto/sha256 over the same bytes.
func TestComputeSHA256_MatchesStdlib(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sanity.bin")
	content := []byte("the quick brown fox jumps over the lazy dog")
	require.NoError(t, os.WriteFile(path, content, 0o644))

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	h := sha256.New()
	_, err = io.Copy(h, f)
	require.NoError(t, err)
	expected := hex.EncodeToString(h.Sum(nil))

	got, err := ComputeSHA256(path)
	require.NoError(t, err)
	require.Equal(t, expected, got)
}

// patternBytes is the deterministic content of the files KOReader hashed for
// TestComputeKOReaderPartialMD5.
func patternBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte((i * 31) ^ (i >> 8) ^ (i >> 16))
	}
	return b
}

// The expected hashes came from KOReader v2026.07.1's util.partialMD5, run in
// the harness (tools/koreaderharness) on files with this content.
func TestComputeKOReaderPartialMD5(t *testing.T) {
	t.Parallel()

	tests := []struct {
		size int
		want string
	}{
		{0, "d41d8cd98f00b204e9800998ecf8427e"},
		{1, "93b885adfe0da089cdf634904fd59f71"},
		{500, "235f465285be252ba89ae8ce1f0fc130"},
		{1024, "6859b53fa0050dbf94edcb60daa649dc"},
		{1025, "84d70b3bf07d8eea51fc24efcd30775f"},
		{4096, "6ba4ac7e2262664f6524708a73f04bf1"},
		{5000, "c3707286ad860392e9e0644c7cccb621"},
		{70000, "2a73cacb40a62ce540a2e9481e06d0a1"},
		{3145728, "a6dd54713de0d0edc0a0972a66cc3d34"},
	}
	for _, tt := range tests {
		path := filepath.Join(t.TempDir(), "sample.bin")
		require.NoError(t, os.WriteFile(path, patternBytes(tt.size), 0o644))

		got, err := ComputeKOReaderPartialMD5(path)
		require.NoError(t, err)
		require.Equal(t, tt.want, got, "size %d", tt.size)
	}
}

// A sparse file reaches the last sample, at 1 GiB, without writing a gigabyte.
func TestComputeKOReaderPartialMD5_LastSample(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sparse.bin")
	f, err := os.Create(path)
	require.NoError(t, err)
	for _, offset := range []int64{262144, 1048576, 4194304, 16777216, 67108864, 268435456, 1073741824} {
		mark := make([]byte, 8)
		binary.BigEndian.PutUint64(mark, uint64(offset))
		_, err := f.WriteAt(mark, offset)
		require.NoError(t, err)
	}
	require.NoError(t, f.Truncate(1073741824+500))
	require.NoError(t, f.Close())

	got, err := ComputeKOReaderPartialMD5(path)
	require.NoError(t, err)
	require.Equal(t, "4d84f986dabdd871c2f1b6373a84cc74", got)
}

func TestComputeKOReaderPartialMD5_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := ComputeKOReaderPartialMD5(filepath.Join(t.TempDir(), "missing.bin"))
	require.Error(t, err)
}
