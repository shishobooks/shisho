package testutils

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
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
// GET /test/plugins/fixture.zip.
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
// GET /test/plugins/fixture-info.
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

// seedPluginRequest is the request body for seeding an installed plugin.
type seedPluginRequest struct {
	Scope                  string  `json:"scope" validate:"required"`
	ID                     string  `json:"id" validate:"required"`
	Name                   string  `json:"name"`
	Version                string  `json:"version"`
	Status                 int     `json:"status"` // matches models.PluginStatus (0=active, -1=disabled, -2=malfunctioned, -3=notsupported)
	UpdateAvailableVersion *string `json:"update_available_version"`
	RepositoryScope        *string `json:"repository_scope"`
	RepositoryURL          *string `json:"repository_url"`
	SkipLoad               bool    `json:"skip_load"` // default false: call manager.LoadPlugin after seeding
}

// seedPlugin seeds an installed plugin for tests. Writes the fixture plugin
// files to disk under {pluginDir}/{scope}/{id}/ and inserts a plugins row with
// the caller-specified metadata. Optionally calls manager.LoadPlugin so the
// runtime is available.
// POST /test/plugins.
func (h *handler) seedPlugin(c echo.Context) error {
	ctx := c.Request().Context()

	var req seedPluginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}
	if req.Scope == "" || req.ID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "scope and id are required")
	}

	name := req.Name
	if name == "" {
		name = fixtureName
	}
	version := req.Version
	if version == "" {
		version = fixtureVersion
	}

	// Write fixture files to {pluginDir}/{scope}/{id}/.
	destDir := filepath.Join(h.installer.PluginDir(), req.Scope, req.ID)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return errors.Wrap(err, "create plugin dir")
	}
	for fname, data := range fixtureFiles() {
		if err := os.WriteFile(filepath.Join(destDir, fname), data, 0644); err != nil {
			return errors.Wrapf(err, "write %s", fname)
		}
	}

	// Insert DB row.
	plugin := &models.Plugin{
		Scope:                  req.Scope,
		ID:                     req.ID,
		Name:                   name,
		Version:                version,
		Status:                 models.PluginStatus(req.Status),
		AutoUpdate:             true,
		InstalledAt:            time.Now(),
		UpdateAvailableVersion: req.UpdateAvailableVersion,
		RepositoryScope:        req.RepositoryScope,
		RepositoryURL:          req.RepositoryURL,
	}
	if _, err := h.db.NewInsert().Model(plugin).Exec(ctx); err != nil {
		return errors.Wrap(err, "insert plugin row")
	}

	// Load the runtime unless told not to.
	if !req.SkipLoad && h.manager != nil && plugin.Status == models.PluginStatusActive {
		if err := h.manager.LoadPlugin(ctx, req.Scope, req.ID); err != nil {
			// Store load error on the row but don't fail the request —
			// the test may intentionally be seeding a broken state.
			msg := err.Error()
			plugin.LoadError = &msg
			_, _ = h.db.NewUpdate().Model(plugin).WherePK().Exec(ctx)
		}
	}

	return c.JSON(http.StatusCreated, plugin)
}
