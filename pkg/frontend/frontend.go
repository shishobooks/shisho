// Package frontend serves the embedded web application.
package frontend

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:dist
var embedded embed.FS

// Handler serves the frontend compiled into the binary.
func Handler() http.Handler {
	files, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}
	return NewHandler(files)
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
