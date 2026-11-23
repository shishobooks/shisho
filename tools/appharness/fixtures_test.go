package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixtureCacheMakesFixturesOncePerKey(t *testing.T) {
	t.Parallel()
	cache := t.TempDir()
	made := 0
	makeFixtures := func(dir string) error {
		made++
		return os.WriteFile(filepath.Join(dir, "book.epub"), []byte("book"), 0o600)
	}

	first, err := fillFixtureCache(cache, "key-a", makeFixtures)
	require.NoError(t, err)
	second, err := fillFixtureCache(cache, "key-a", makeFixtures)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Equal(t, 1, made)
	assert.FileExists(t, filepath.Join(first, "book.epub"))

	// A new key makes them again, beside the first key's, which another
	// worktree may be using.
	third, err := fillFixtureCache(cache, "key-b", makeFixtures)
	require.NoError(t, err)
	assert.Equal(t, 2, made)
	assert.FileExists(t, filepath.Join(third, "book.epub"))
	assert.DirExists(t, first)
}

func TestFixtureCacheDropsSetsLongUnused(t *testing.T) {
	t.Parallel()
	cache := t.TempDir()
	makeFixtures := func(string) error { return nil }
	old, err := fillFixtureCache(cache, "key-old", makeFixtures)
	require.NoError(t, err)
	stale := filepath.Join(cache, ".making-killed")
	require.NoError(t, os.Mkdir(stale, 0o755))
	long := time.Now().Add(-unusedFixturesKept - time.Hour)
	require.NoError(t, os.Chtimes(old, long, long))
	require.NoError(t, os.Chtimes(stale, long, long))

	current, err := fillFixtureCache(cache, "key-new", makeFixtures)
	require.NoError(t, err)
	assert.DirExists(t, current)
	assert.NoDirExists(t, old)
	assert.NoDirExists(t, stale)

	// Using a set keeps it, however old it was.
	require.NoError(t, os.Chtimes(current, long, long))
	_, err = fillFixtureCache(cache, "key-new", makeFixtures)
	require.NoError(t, err)
	_, err = fillFixtureCache(cache, "key-other", makeFixtures)
	require.NoError(t, err)
	assert.DirExists(t, current)
}

// The fixtures are made by internal/testgen and the packages of this module
// it imports, so the cache key has to cover each of them.
func TestFixtureSourcesCoverTheGenerators(t *testing.T) {
	t.Parallel()
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "-f", "{{if .Module}}{{if .Module.Main}}{{.Dir}}{{end}}{{end}}", "../../internal/testgen").Output()
	require.NoError(t, err)
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	for _, dir := range strings.Fields(string(out)) {
		rel, err := filepath.Rel(root, dir)
		require.NoError(t, err)
		covered := false
		for _, source := range fixtureSources {
			if rel == source || strings.HasPrefix(rel, source+"/") {
				covered = true
			}
		}
		assert.True(t, covered, "fixtureSources leaves out %s, which internal/testgen uses", rel)
	}
}

func TestFixtureCacheKeepsNothingWhenMakingFails(t *testing.T) {
	t.Parallel()
	cache := t.TempDir()
	_, err := fillFixtureCache(cache, "key-a", func(string) error { return assert.AnError })
	require.Error(t, err)
	entries, err := os.ReadDir(cache)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestFixtureKeyFollowsTheDefinitionsAndGenerators(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(path, content string) {
		full := filepath.Join(root, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	}
	for _, source := range fixtureSources {
		if filepath.Ext(source) != "" {
			write(source, source)
		} else {
			write(source+"/source.go", source)
		}
	}
	write("tools/appharness/ios.go", "ios")
	write("internal/testgen/testdata/cover.png", "png")

	key := func() string {
		k, err := fixtureKey(root)
		require.NoError(t, err)
		return k
	}
	base := key()
	assert.Equal(t, base, key())

	write("tools/appharness/ios.go", "ios changed")
	assert.Equal(t, base, key(), "the rest of the harness doesn't change the fixtures")

	write("tools/appharness/fixtures.go", "fixtures changed")
	definitions := key()
	assert.NotEqual(t, base, definitions)

	write("internal/testgen/testdata/cover.png", "png changed")
	assert.NotEqual(t, definitions, key())
}
