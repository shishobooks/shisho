package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPortFileIsWrittenOnlyWhenTheDevLauncherAsks(t *testing.T) {
	t.Parallel()

	// An E2E API running in the same worktree must not overwrite the dev
	// API's port file, so the file is only written when its path is given.
	require.NoError(t, writePortFile("", 3689))

	// A fresh worktree may not have tmp/ yet.
	path := filepath.Join(t.TempDir(), "tmp", "api.port")
	require.NoError(t, writePortFile(path, 3690))
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "3690", string(contents))

	removePortFile(path)
	assert.NoFileExists(t, path)
	removePortFile("")
}
