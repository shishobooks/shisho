// Package koreaderplugin serves the KOReader plugin (ADR 0016): its zip,
// stamped with the server's version and a hash of the plugin's files, and
// the API the plugin signs in and checks for updates through.
package koreaderplugin

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	pluginfiles "github.com/shishobooks/shisho/koreader-plugin"
	"github.com/shishobooks/shisho/pkg/version"
)

const (
	// buildFile and pairingFile are the files a zip gains beyond the
	// plugin's source. The plugin reads both when present.
	buildFile   = "build.lua"
	pairingFile = "pairing.lua"
)

var (
	releaseVersion = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)(?:-(.*))?$`)
	// prerelease is a tagged prerelease, such as rc.1, as opposed to the
	// commits-since-tag and -dirty that git describe adds to a dev build.
	prerelease = regexp.MustCompile(`^[0-9A-Za-z.]+$`)
	describe   = regexp.MustCompile(`(^|-)(\d+-g[0-9a-f]+|dirty)(-|$)`)
)

// devRelease is the latest release tag a build with no version of its own
// was built from, which the dev server's build (mise build:air) sets with
// -ldflags "-X github.com/shishobooks/shisho/pkg/koreaderplugin.devRelease=v0.7.0".
// A server's own version stays "dev" there, because server plugins'
// minimum version checks pass for "dev".
var devRelease string

// PluginVersion is the plugin version a server of version v serves.
func PluginVersion(v string) string {
	return pluginVersion(v, devRelease)
}

// pluginVersion is the plugin version a server of version v serves, where
// built is the release tag a dev build was built from, when v names none. A
// release serves its own version, and a tagged prerelease its own, such as
// 0.8.0-rc.1, which the plugin orders before 0.8.0. Any other build carries
// the release it was built from with a -dev suffix, which the plugin orders
// after that release: from v, as git describe gives it, or from built. A
// build that knows no release, such as one from plain go build, counts as
// 0.0.0-dev.
func pluginVersion(v, built string) string {
	match := releaseVersion.FindStringSubmatch(v)
	switch {
	case match == nil:
		if base := releaseVersion.FindStringSubmatch(built); base != nil {
			return base[1] + "-dev"
		}
		return "0.0.0-dev"
	case match[2] == "":
		return match[1]
	case prerelease.MatchString(match[2]) && !describe.MatchString(match[2]) && match[2] != "dev":
		return match[1] + "-" + match[2]
	default:
		return match[1] + "-dev"
	}
}

// Bundle is the plugin's zip, built once from its source.
type Bundle struct {
	// Version is the plugin version the zip carries.
	Version string
	// Hash identifies the plugin's files, so two builds of one version can
	// be told apart.
	Hash string
	zip  []byte
}

// NewBundle zips the plugin directory in fsys and stamps it with the plugin
// version for serverVersion and a hash of its files.
func NewBundle(fsys fs.FS, serverVersion string) (*Bundle, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(fsys, pluginfiles.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Hidden files, such as .DS_Store or an editor's swap file, aren't
		// the plugin, and would change its hash.
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		files[path] = data
		return nil
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	hasher := sha256.New()
	for _, path := range paths {
		// Length-prefixed, so moving bytes between a path and its content
		// changes the hash.
		fmt.Fprintf(hasher, "%d:%s%d:", len(path), path, len(files[path]))
		hasher.Write(files[path])
	}
	bundle := &Bundle{Version: PluginVersion(serverVersion), Hash: hex.EncodeToString(hasher.Sum(nil))}

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, path := range paths {
		if err := addFile(w, path, files[path]); err != nil {
			return nil, err
		}
	}
	build := fmt.Sprintf("-- Stamped by the Shisho server that built this zip.\nreturn {\n    version = %s,\n    hash = %s,\n}\n",
		luaQuote(bundle.Version), luaQuote(bundle.Hash))
	if err := addFile(w, pluginfiles.Dir+"/"+buildFile, []byte(build)); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, errors.WithStack(err)
	}
	bundle.zip = buf.Bytes()
	return bundle, nil
}

// fixedModified dates every entry, so one plugin always zips to the same
// bytes.
var fixedModified = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func addFile(w *zip.Writer, path string, data []byte) error {
	out, err := w.CreateHeader(&zip.FileHeader{Name: path, Method: zip.Deflate, Modified: fixedModified})
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = out.Write(data)
	return errors.WithStack(err)
}

// Zip returns the zip with no server or short code, as a release attaches
// it and as the plugin updates itself from.
func (b *Bundle) Zip() []byte {
	return b.zip
}

// PairedZip returns the zip with serverURL and a Device short code that
// expires at expiresAt, so the plugin can sign itself in on first launch.
func (b *Bundle) PairedZip(serverURL, shortCode string, expiresAt time.Time) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(b.zip), int64(len(b.zip)))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range reader.File {
		if err := w.Copy(f); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	pairing := fmt.Sprintf("-- The server and Device short code this zip signs in with on first launch.\nreturn {\n    server_url = %s,\n    short_code = %s,\n    expires_at = %d,\n}\n",
		luaQuote(serverURL), luaQuote(shortCode), expiresAt.Unix())
	if err := addFile(w, pluginfiles.Dir+"/"+pairingFile, []byte(pairing)); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, errors.WithStack(err)
	}
	return buf.Bytes(), nil
}

// luaQuote returns s as a Lua string literal. UTF-8 passes through; quotes,
// backslashes, and control characters are escaped.
func luaQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\n':
			b.WriteString(`\n`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\%03d`, c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

var (
	defaultOnce   sync.Once
	defaultBundle *Bundle
	defaultErr    error
)

// DefaultBundle returns the embedded plugin's zip for this server, built on
// first use and kept for the life of the process. The dev server rebuilds
// its binary when the plugin's Lua changes, so it always serves the
// worktree's plugin.
func DefaultBundle() (*Bundle, error) {
	defaultOnce.Do(func() {
		defaultBundle, defaultErr = NewBundle(pluginfiles.FS, version.Version)
	})
	return defaultBundle, defaultErr
}
