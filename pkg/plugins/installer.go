package plugins

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// AllowedDownloadHosts lists the allowed host prefixes for plugin download URLs.
// Tests can override this to allow test servers.
var AllowedDownloadHosts = []string{"https://github.com/"}

// Installer handles downloading and extracting plugins.
type Installer struct {
	pluginDir string // Base directory for installed plugins
}

// NewInstaller creates a new Installer.
func NewInstaller(pluginDir string) *Installer {
	return &Installer{pluginDir: pluginDir}
}

// InstallPlugin downloads a plugin ZIP, verifies its SHA256, and extracts it.
// Returns the parsed manifest from the extracted plugin.
func (inst *Installer) InstallPlugin(ctx context.Context, scope, pluginID, downloadURL, expectedSHA256 string) (*Manifest, error) {
	if !isAllowedDownloadURL(downloadURL) {
		return nil, errors.Errorf("invalid download URL: only URLs starting with %v are allowed", AllowedDownloadHosts)
	}

	// Download ZIP to temp file
	tmpFile, err := inst.downloadToTemp(ctx, downloadURL)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile)

	// Verify SHA256
	if err := inst.verifySHA256(tmpFile, expectedSHA256); err != nil {
		return nil, err
	}

	// Extract to plugin directory
	destDir := filepath.Join(inst.pluginDir, scope, pluginID)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, errors.Wrap(err, "failed to create plugin directory")
	}

	if err := inst.extractZip(tmpFile, destDir); err != nil {
		// Clean up on failure
		os.RemoveAll(destDir)
		return nil, err
	}

	// Read and parse manifest.json from extracted directory
	manifestPath := filepath.Join(destDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		os.RemoveAll(destDir)
		return nil, errors.Wrap(err, "failed to read manifest.json from extracted plugin")
	}

	manifest, err := ParseManifest(manifestData)
	if err != nil {
		os.RemoveAll(destDir)
		return nil, errors.Wrap(err, "invalid manifest in downloaded plugin")
	}

	return manifest, nil
}

// PluginDir returns the base directory for installed plugins.
func (inst *Installer) PluginDir() string {
	return inst.pluginDir
}

// UninstallPlugin removes a plugin's files from disk.
func (inst *Installer) UninstallPlugin(scope, pluginID string) error {
	dir := filepath.Join(inst.pluginDir, scope, pluginID)
	return os.RemoveAll(dir)
}

// UpdatePlugin replaces an existing plugin with a new version.
func (inst *Installer) UpdatePlugin(ctx context.Context, scope, pluginID, downloadURL, expectedSHA256 string) (*Manifest, error) {
	if !isAllowedDownloadURL(downloadURL) {
		return nil, errors.Errorf("invalid download URL: only URLs starting with %v are allowed", AllowedDownloadHosts)
	}

	// Download ZIP to temp file
	tmpFile, err := inst.downloadToTemp(ctx, downloadURL)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile)

	// Verify SHA256
	if err := inst.verifySHA256(tmpFile, expectedSHA256); err != nil {
		return nil, err
	}

	// Extract to a temp directory first
	tmpDir, err := os.MkdirTemp("", "plugin-update-*")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp directory for update")
	}
	defer os.RemoveAll(tmpDir)

	if err := inst.extractZip(tmpFile, tmpDir); err != nil {
		return nil, err
	}

	// Verify manifest in new version before replacing
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read manifest.json from updated plugin")
	}

	manifest, err := ParseManifest(manifestData)
	if err != nil {
		return nil, errors.Wrap(err, "invalid manifest in updated plugin")
	}

	// Remove old plugin directory
	destDir := filepath.Join(inst.pluginDir, scope, pluginID)
	if err := os.RemoveAll(destDir); err != nil {
		return nil, errors.Wrap(err, "failed to remove old plugin directory")
	}

	// Move new files to plugin directory
	if err := os.MkdirAll(filepath.Dir(destDir), 0755); err != nil {
		return nil, errors.Wrap(err, "failed to create parent directory")
	}

	if err := os.Rename(tmpDir, destDir); err != nil {
		// Fallback: if rename fails (cross-device), copy files
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return nil, errors.Wrap(err, "failed to create plugin directory")
		}
		if err := inst.extractZip(tmpFile, destDir); err != nil {
			return nil, errors.Wrap(err, "failed to extract updated plugin")
		}
	}

	return manifest, nil
}

// downloadToTemp downloads a URL to a temporary file and returns the path.
func (inst *Installer) downloadToTemp(ctx context.Context, url string) (string, error) {
	client := &http.Client{Timeout: 120 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to create download request")
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "failed to download plugin")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("failed to download plugin: HTTP %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "plugin-download-*.zip")
	if err != nil {
		return "", errors.Wrap(err, "failed to create temp file")
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", errors.Wrap(err, "failed to write downloaded plugin to temp file")
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", errors.Wrap(err, "failed to close temp file")
	}

	return tmpFile.Name(), nil
}

// verifySHA256 computes the SHA256 of a file and compares it to the expected hash.
func (inst *Installer) verifySHA256(filePath, expected string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return errors.Wrap(err, "failed to open file for checksum verification")
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return errors.Wrap(err, "failed to compute SHA256")
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return errors.Errorf("SHA256 mismatch: expected %s, got %s", expected, actual)
	}

	return nil
}

// extractZip extracts a ZIP file to the destination directory.
func (inst *Installer) extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return errors.Wrap(err, "failed to open ZIP file")
	}
	defer r.Close()

	for _, f := range r.File {
		// Prevent zip slip attack
		name := filepath.Clean(f.Name)
		if strings.HasPrefix(name, "..") || strings.HasPrefix(name, "/") {
			continue
		}

		fpath := filepath.Join(destDir, name)

		// Ensure the file path is within the destination directory
		if !strings.HasPrefix(filepath.Clean(fpath), filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, 0755); err != nil {
				return errors.Wrapf(err, "failed to create directory %s", name)
			}
			continue
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return errors.Wrapf(err, "failed to create directory for %s", name)
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return errors.Wrapf(err, "failed to create file %s", name)
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return errors.Wrapf(err, "failed to open ZIP entry %s", name)
		}

		// Limit extraction to 100MB per file to prevent decompression bombs
		const maxFileSize = 100 * 1024 * 1024
		_, err = io.Copy(outFile, io.LimitReader(rc, maxFileSize))
		rc.Close()
		outFile.Close()
		if err != nil {
			return errors.Wrapf(err, "failed to extract %s", name)
		}
	}

	return nil
}

// isAllowedDownloadURL checks whether the URL matches any allowed download host prefix.
func isAllowedDownloadURL(url string) bool {
	for _, prefix := range AllowedDownloadHosts {
		if strings.HasPrefix(url, prefix) {
			return true
		}
	}
	return false
}
