package koreaderplugin

import (
	"archive/zip"
	"bytes"
	"io"
	"io/fs"
	"regexp"
	"strconv"
	"testing"
	"testing/fstest"
	"time"

	pluginfiles "github.com/shishobooks/shisho/koreader-plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pluginFS(main string) fstest.MapFS {
	return fstest.MapFS{
		"shisho.koplugin/_meta.lua":       {Data: []byte("return {}\n")},
		"shisho.koplugin/main.lua":        {Data: []byte(main)},
		"shisho.koplugin/shisho/util.lua": {Data: []byte("return {}\n")},
	}
}

// unzip returns each file in a zip by its path.
func unzip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	files := map[string]string{}
	for _, f := range reader.File {
		rc, err := f.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		files[f.Name] = string(content)
	}
	return files
}

func TestPluginVersion(t *testing.T) {
	t.Parallel()
	for version, want := range map[string]string{
		"v0.7.0":                  "0.7.0",
		"0.7.0":                   "0.7.0",
		"v0.7.0-3-gabc1234":       "0.7.0-dev",
		"v0.7.0-3-gabc1234-dirty": "0.7.0-dev",
		"v1.2.3-rc.1":             "1.2.3-rc.1",
		"v1.2.3-rc.1-2-gabc1234":  "1.2.3-dev",
		"v1.2.3-dev":              "1.2.3-dev",
		"dev":                     "0.0.0-dev",
		"":                        "0.0.0-dev",
		"abc1234":                 "0.0.0-dev",
	} {
		assert.Equal(t, want, PluginVersion(version), version)
	}
}

func TestBundle_ZipHoldsThePluginAndItsBuild(t *testing.T) {
	t.Parallel()
	bundle, err := NewBundle(pluginFS("return 1\n"), "v0.7.0")
	require.NoError(t, err)

	files := unzip(t, bundle.Zip())
	assert.Equal(t, "return 1\n", files["shisho.koplugin/main.lua"])
	assert.Equal(t, "return {}\n", files["shisho.koplugin/shisho/util.lua"])
	assert.Contains(t, files, "shisho.koplugin/_meta.lua")
	assert.NotContains(t, files, "shisho.koplugin/pairing.lua", "only a Device page download is paired")

	build := files["shisho.koplugin/build.lua"]
	assert.Contains(t, build, `version = "0.7.0"`)
	assert.Contains(t, build, `hash = "`+bundle.Hash+`"`)
	assert.Equal(t, "0.7.0", bundle.Version)
	assert.Len(t, bundle.Hash, 64)
}

func TestBundle_LeavesOutHiddenFiles(t *testing.T) {
	t.Parallel()
	plain, err := NewBundle(pluginFS("return 1\n"), "v0.7.0")
	require.NoError(t, err)
	files := pluginFS("return 1\n")
	files["shisho.koplugin/.DS_Store"] = &fstest.MapFile{Data: []byte("x")}
	files["shisho.koplugin/shisho/.main.lua.swp"] = &fstest.MapFile{Data: []byte("x")}
	withHidden, err := NewBundle(files, "v0.7.0")
	require.NoError(t, err)

	assert.Equal(t, plain.Hash, withHidden.Hash)
	assert.NotContains(t, unzip(t, withHidden.Zip()), "shisho.koplugin/.DS_Store")
}

func TestBundle_HashFollowsTheFiles(t *testing.T) {
	t.Parallel()
	a, err := NewBundle(pluginFS("return 1\n"), "v0.7.0")
	require.NoError(t, err)
	same, err := NewBundle(pluginFS("return 1\n"), "v0.8.0")
	require.NoError(t, err)
	changed, err := NewBundle(pluginFS("return 2\n"), "v0.7.0")
	require.NoError(t, err)

	assert.Equal(t, a.Hash, same.Hash, "the version is not part of the files")
	assert.NotEqual(t, a.Hash, changed.Hash)
}

func TestBundle_PairedZipCarriesTheServerAndCode(t *testing.T) {
	t.Parallel()
	bundle, err := NewBundle(pluginFS("return 1\n"), "v0.7.0")
	require.NoError(t, err)
	expires := time.Unix(1800000000, 0)

	data, err := bundle.PairedZip(`https://books.example.com`, "abc123", expires)
	require.NoError(t, err)

	files := unzip(t, data)
	assert.Equal(t, "return 1\n", files["shisho.koplugin/main.lua"])
	assert.Equal(t, unzip(t, bundle.Zip())["shisho.koplugin/build.lua"], files["shisho.koplugin/build.lua"])
	pairing := files["shisho.koplugin/pairing.lua"]
	assert.Contains(t, pairing, `server_url = "https://books.example.com"`)
	assert.Contains(t, pairing, `short_code = "abc123"`)
	assert.Contains(t, pairing, `expires_at = 1800000000`)
}

func TestBundle_PairedZipEscapesTheURL(t *testing.T) {
	t.Parallel()
	bundle, err := NewBundle(pluginFS("return 1\n"), "v0.7.0")
	require.NoError(t, err)

	data, err := bundle.PairedZip("http://host\"\n]]--", "abc123", time.Unix(1, 0))
	require.NoError(t, err)

	assert.Contains(t, unzip(t, data)["shisho.koplugin/pairing.lua"], `server_url = "http://host\"\n]]--"`)
}

func TestBundle_EmbeddedPluginBuilds(t *testing.T) {
	t.Parallel()
	bundle, err := DefaultBundle()
	require.NoError(t, err)

	files := unzip(t, bundle.Zip())
	assert.Contains(t, files, "shisho.koplugin/_meta.lua")
	assert.Contains(t, files, "shisho.koplugin/main.lua")
}

func TestProtocol_MatchesThePlugins(t *testing.T) {
	t.Parallel()
	source, err := fs.ReadFile(pluginfiles.FS, pluginfiles.Dir+"/shisho/protocol.lua")
	require.NoError(t, err)

	match := regexp.MustCompile(`version = (\d+)`).FindSubmatch(source)
	require.NotNil(t, match, "shisho/protocol.lua names no version")
	assert.Equal(t, strconv.Itoa(Protocol), string(match[1]), "the plugin this server serves speaks Protocol")
	assert.GreaterOrEqual(t, Protocol, ProtocolMin)
	assert.LessOrEqual(t, Protocol, ProtocolMax)
}
