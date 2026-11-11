package main

import (
	"archive/zip"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The release workflow runs this after publishing images and npm, so a
// failure there would leave a release half done. CI runs this test first.
func TestWrite_ReleaseZipIsStampedButUnpaired(t *testing.T) {
	t.Parallel()
	out := filepath.Join(t.TempDir(), "zip", "shisho.koplugin.zip")

	require.NoError(t, write("v0.8.0", out))

	reader, err := zip.OpenReader(out)
	require.NoError(t, err)
	defer reader.Close()
	files := map[string]string{}
	for _, f := range reader.File {
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		rc.Close()
		files[f.Name] = string(data)
	}
	assert.Contains(t, files, "shisho.koplugin/main.lua")
	assert.Contains(t, files["shisho.koplugin/build.lua"], `version = "0.8.0"`)
	assert.NotContains(t, files, "shisho.koplugin/pairing.lua")
}
