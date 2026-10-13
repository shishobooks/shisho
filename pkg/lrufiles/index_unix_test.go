//go:build unix

package lrufiles

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndex_UnreadableDirectoryDoesNotStopEviction(t *testing.T) {
	t.Parallel()
	idx, clk, root := newTestIndex(t)
	old := filepath.Join(root, "1", "old")
	writeFile(t, old, 100)
	past := clk.now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(old, past, past))
	locked := filepath.Join(root, "2")
	require.NoError(t, os.Mkdir(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	require.NoError(t, idx.SetMaxBytes(0))
	assert.NoFileExists(t, old, "files the walk can read are still evicted")
}
