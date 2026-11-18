// Package frontend serves the embedded web application.
package frontend

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
)

//go:embed all:dist
var embedded embed.FS

func embeddedFiles() fs.FS {
	files, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}
	return files
}

// Handler serves the frontend compiled into the binary.
func Handler() http.Handler {
	return NewHandler(embeddedFiles())
}

// buildMeta matches the tag the shisho-build-id plugin in vite.config.ts
// writes into index.html.
var buildMeta = regexp.MustCompile(`<meta name="shisho-build" content="([^"]+)"`)

// EmbeddedBuildID identifies the frontend build compiled into the binary.
func EmbeddedBuildID() string {
	return BuildID(embeddedFiles())
}

// BuildID identifies a frontend build, so a tab can tell it is running an
// older one than the server has. It is empty when there is no build, as in
// development, where Vite serves the app.
func BuildID(files fs.FS) string {
	data, err := fs.ReadFile(files, "index.html")
	if err != nil {
		return ""
	}
	match := buildMeta.FindSubmatch(data)
	if match == nil {
		return ""
	}
	return string(match[1])
}

// NewHandler serves files from a frontend build, falling back to index.html for
// SPA routes. The filesystem must be rooted at the build directory.
func NewHandler(files fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		data, err := fs.ReadFile(files, name)
		if err != nil && strings.HasPrefix(name, "assets/") {
			// Hashed build output never falls back to index.html: a tab opened
			// before an upgrade asks for chunks that are gone, and HTML in their
			// place surfaces as a MIME type error rather than a missing file.
			w.Header().Set("Cache-Control", "no-cache")
			http.NotFound(w, r)
			return
		}
		if err != nil || name == "placeholder.html" {
			name = "index.html"
			data, err = fs.ReadFile(files, name)
		}
		if err != nil {
			data, err = fs.ReadFile(files, "placeholder.html")
		}
		if err != nil {
			http.Error(w, "Frontend not built", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})
}
