package httputil

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

// ServeFile serves the file at path like echo's c.File, with Content-Type
// from its extension and Last-Modified conditional GETs, except that only a
// missing file returns notFound. c.File turns every open failure, including
// a permission error, into echo's generic 404; here any other failure is a
// server fault.
func ServeFile(c echo.Context, path string, notFound error) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return notFound
		}
		return errors.WithStack(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return errors.WithStack(err)
	}
	if info.IsDir() {
		return notFound
	}
	http.ServeContent(c.Response(), c.Request(), filepath.Base(path), info.ModTime(), f)
	return nil
}
