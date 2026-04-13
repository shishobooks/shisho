package fingerprint

import (
	"crypto/sha256"
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
