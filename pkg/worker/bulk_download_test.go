package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeBulkFingerprint(t *testing.T) {
	t.Parallel()

	t.Run("deterministic for same inputs", func(t *testing.T) {
		t.Parallel()
		hashes := []string{"abc123", "def456", "ghi789"}
		hash1 := ComputeBulkFingerprint([]int{1, 2, 3}, hashes)
		hash2 := ComputeBulkFingerprint([]int{1, 2, 3}, hashes)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("sorts file IDs for consistency", func(t *testing.T) {
		t.Parallel()
		hashes1 := []string{"abc123", "def456"}
		hashes2 := []string{"def456", "abc123"}
		hash1 := ComputeBulkFingerprint([]int{1, 2}, hashes1)
		hash2 := ComputeBulkFingerprint([]int{2, 1}, hashes2)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("different hashes produce different fingerprint", func(t *testing.T) {
		t.Parallel()
		hash1 := ComputeBulkFingerprint([]int{1, 2}, []string{"abc", "def"})
		hash2 := ComputeBulkFingerprint([]int{1, 2}, []string{"abc", "xyz"})
		assert.NotEqual(t, hash1, hash2)
	})
}

func TestDeduplicateFilenames(t *testing.T) {
	t.Parallel()

	t.Run("no duplicates", func(t *testing.T) {
		t.Parallel()
		names := map[int]string{1: "Book A.epub", 2: "Book B.epub"}
		result := DeduplicateFilenames(names)
		assert.Equal(t, "Book A.epub", result[1])
		assert.Equal(t, "Book B.epub", result[2])
	})

	t.Run("duplicates get numbered", func(t *testing.T) {
		t.Parallel()
		names := map[int]string{1: "Book.epub", 2: "Book.epub", 3: "Book.epub"}
		result := DeduplicateFilenames(names)
		values := []string{result[1], result[2], result[3]}
		require.Contains(t, values, "Book.epub")
		require.Contains(t, values, "Book (2).epub")
		require.Contains(t, values, "Book (3).epub")
	})
}
