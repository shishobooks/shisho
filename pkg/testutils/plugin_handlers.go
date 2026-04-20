package testutils

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

// buildFixtureZip produces a deterministic zip of the fixture plugin files and
// returns the bytes plus their SHA256. The zip stores entries with a fixed
// modtime so repeat calls produce identical bytes.
func buildFixtureZip() ([]byte, string, error) {
	files := fixtureFiles()

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range names {
		fh := &zip.FileHeader{Name: name, Method: zip.Deflate}
		fh.Modified = fixedTime
		w, err := zw.CreateHeader(fh)
		if err != nil {
			return nil, "", errors.Wrapf(err, "create zip entry %s", name)
		}
		if _, err := w.Write(files[name]); err != nil {
			return nil, "", errors.Wrapf(err, "write zip entry %s", name)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, "", errors.Wrap(err, "close zip")
	}

	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:]), nil
}

// fixtureZip serves the fixture plugin as a zip.
// GET /test/plugins/fixture.zip
func (h *handler) fixtureZip(c echo.Context) error {
	data, _, err := buildFixtureZip()
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(c.Blob(http.StatusOK, "application/zip", data))
}

type fixtureInfoResponse struct {
	Scope       string `json:"scope"`
	ID          string `json:"id"`
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
}

// fixtureInfo returns metadata about the fixture plugin.
// GET /test/plugins/fixture-info
func (h *handler) fixtureInfo(c echo.Context) error {
	_, sum, err := buildFixtureZip()
	if err != nil {
		return errors.WithStack(err)
	}
	scheme := "http"
	if c.Request().TLS != nil {
		scheme = "https"
	}
	downloadURL := scheme + "://" + c.Request().Host + "/test/plugins/fixture.zip"
	return c.JSON(http.StatusOK, fixtureInfoResponse{
		Scope:       fixtureScope,
		ID:          fixtureID,
		Version:     fixtureVersion,
		DownloadURL: downloadURL,
		SHA256:      sum,
	})
}
