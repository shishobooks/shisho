package plugins

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
)

// AllowedDownloadHosts lists the allowed host prefixes for plugin download URLs.
// Tests can override this to allow test servers.
var AllowedDownloadHosts = []string{"https://github.com/"}

// Installer failures a caller must tell apart from server faults. Each wraps
// the detail, so the error's message keeps it. installerError in
// handler_install.go renders them.
var (
	// ErrInvalidDownloadURL marks a download URL outside AllowedDownloadHosts.
	ErrInvalidDownloadURL = errors.New("invalid download URL")
	// ErrChecksumMismatch marks a download whose SHA256 is not the expected one.
	ErrChecksumMismatch = errors.New("SHA256 mismatch")
	// ErrInvalidPackage marks a download that is not a valid plugin package:
	// not a ZIP file, or without a valid manifest.json.
	ErrInvalidPackage = errors.New("invalid plugin package")
	// ErrDownloadFailed marks a download host that could not be reached or
	// answered with a status other than 200.
	ErrDownloadFailed = errors.New("failed to download plugin")
)

// invalidDownloadURL returns ErrInvalidDownloadURL naming the allowed hosts.
func invalidDownloadURL() error {
	return errors.WithStack(fmt.Errorf("%w: only URLs starting with %v are allowed", ErrInvalidDownloadURL, AllowedDownloadHosts))
}

// invalidPackage returns ErrInvalidPackage with detail and the cause.
func invalidPackage(detail string, err error) error {
	if err == nil {
		return errors.WithStack(fmt.Errorf("%w: %s", ErrInvalidPackage, detail))
	}
	return errors.WithStack(fmt.Errorf("%w: %s: %w", ErrInvalidPackage, detail, err))
}

// readPackageManifest reads and parses manifest.json from an extracted
// plugin directory. A missing or invalid manifest is ErrInvalidPackage; a
// failed read is a server fault.
func readPackageManifest(dir string) (*Manifest, error) {
	manifestData, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if os.IsNotExist(err) {
		return nil, invalidPackage("manifest.json is missing", nil)
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to read manifest.json from extracted plugin")
	}
	manifest, err := ParseManifest(manifestData)
	if err != nil {
		return nil, invalidPackage("invalid manifest", err)
	}
	return manifest, nil
}

// Installer handles downloading and extracting plugins.
type Installer struct {
	pluginDir string // Base directory for installed plugins
}

// NewInstaller creates a new Installer.
func NewInstaller(pluginDir string) *Installer {
	return &Installer{pluginDir: pluginDir}
}

// Directories inside each scope directory that hold packages in transit:
// {pluginDir}/{scope}/.staging and {pluginDir}/{scope}/.trash. Keeping them
// next to the installed plugins means moving a directory in or out of place
// is a rename even when a scope directory is its own mount. Plugin ids
// cannot start with a dot (see validPathSegment), so neither can collide
// with a plugin.
const (
	// stagingDirName holds packages that are extracted and checked but not
	// yet installed.
	stagingDirName = ".staging"
	// trashDirName holds replaced plugin directories, as
	// .trash/<random>/<id>, until the transition that replaced them commits.
	trashDirName = ".trash"
)

// stagedPackage is a downloaded, verified, and extracted plugin package that
// is not installed yet.
type stagedPackage struct {
	dir      string
	manifest *Manifest
}

// remove deletes the staged files. It is a no-op once the package has been
// moved into place.
func (p *stagedPackage) remove() {
	if err := os.RemoveAll(p.dir); err != nil {
		logger.New().Warn("failed to remove a staged plugin package", logger.Data{"path": p.dir, "error": err.Error()})
	}
}

// stagePackage downloads a plugin ZIP, verifies its SHA256, and extracts it
// into its own directory under {pluginDir}/{scope}/.staging. It requires a
// valid manifest whose id is pluginID, so a package never lands under an id
// it does not declare. Nothing outside the staging directory is touched;
// the caller installs the package with swapIn or removes it.
func (inst *Installer) stagePackage(ctx context.Context, scope, pluginID, downloadURL, expectedSHA256 string) (*stagedPackage, error) {
	if !isAllowedDownloadURL(downloadURL) {
		return nil, invalidDownloadURL()
	}

	tmpFile, err := inst.downloadToTemp(ctx, downloadURL)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile)

	if err := inst.verifySHA256(tmpFile, expectedSHA256); err != nil {
		return nil, err
	}

	stagingRoot := filepath.Join(inst.pluginDir, scope, stagingDirName)
	if err := os.MkdirAll(stagingRoot, 0755); err != nil {
		return nil, errors.Wrap(err, "failed to create the plugin staging directory")
	}
	dir, err := os.MkdirTemp(stagingRoot, "package-")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a plugin staging directory")
	}
	pkg := &stagedPackage{dir: dir}
	// MkdirTemp creates the directory private; installed plugins have
	// always been world-readable.
	if err := os.Chmod(dir, 0755); err != nil {
		pkg.remove()
		return nil, errors.Wrap(err, "failed to set plugin staging directory permissions")
	}

	if err := inst.extractZip(tmpFile, dir); err != nil {
		pkg.remove()
		return nil, err
	}

	manifest, err := readPackageManifest(dir)
	if err != nil {
		pkg.remove()
		return nil, err
	}
	if manifest.ID != pluginID {
		pkg.remove()
		return nil, invalidPackage(fmt.Sprintf("manifest id %q does not match plugin id %q", manifest.ID, pluginID), nil)
	}
	pkg.manifest = manifest
	return pkg, nil
}

// PluginDir returns the base directory for installed plugins.
func (inst *Installer) PluginDir() string {
	return inst.pluginDir
}

// dirSwap is a live plugin directory replaced by a staged one. Until commit
// or rollback, the replaced directory waits under the scope's .trash.
type dirSwap struct {
	live string
	// trashParent is the .trash entry holding the replaced directory, and
	// trash the directory itself. Both are empty when nothing was replaced.
	trashParent string
	trash       string
}

// swapIn moves the staged directory into place as scope/id. A directory
// already there is moved to .trash/<random>/<id> first, and put back if the
// move fails. A crash between the two renames leaves the old version only
// in .trash, which sweepTransitDirs restores at startup.
func swapIn(pluginDir, scope, id, staged string) (*dirSwap, error) {
	live := filepath.Join(pluginDir, scope, id)
	if err := os.MkdirAll(filepath.Dir(live), 0755); err != nil {
		return nil, errors.Wrap(err, "failed to create the plugin scope directory")
	}
	swap := &dirSwap{live: live}

	if _, err := os.Lstat(live); err == nil {
		trashRoot := filepath.Join(pluginDir, scope, trashDirName)
		if err := os.MkdirAll(trashRoot, 0755); err != nil {
			return nil, errors.Wrap(err, "failed to create the plugin trash directory")
		}
		parent, err := os.MkdirTemp(trashRoot, "replaced-")
		if err != nil {
			return nil, errors.Wrap(err, "failed to create a plugin trash directory")
		}
		trash := filepath.Join(parent, id)
		if err := os.Rename(live, trash); err != nil {
			_ = os.RemoveAll(parent)
			return nil, errors.Wrap(err, "failed to move the installed plugin aside")
		}
		swap.trashParent, swap.trash = parent, trash
	} else if !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "failed to check the installed plugin directory")
	}

	if err := os.Rename(staged, live); err != nil {
		err = errors.Wrap(err, "failed to move the new plugin version into place")
		if swap.trash != "" {
			if restoreErr := os.Rename(swap.trash, live); restoreErr != nil {
				return nil, errors.Wrapf(err, "and failed to restore the installed version from %s: %v", swap.trash, restoreErr)
			}
			_ = os.RemoveAll(swap.trashParent)
		}
		return nil, err
	}
	return swap, nil
}

// commit deletes the replaced directory.
func (s *dirSwap) commit() {
	if s.trashParent == "" {
		return
	}
	if err := os.RemoveAll(s.trashParent); err != nil {
		logger.New().Warn("failed to remove a replaced plugin directory", logger.Data{"path": s.trashParent, "error": err.Error()})
	}
}

// rollback removes the new directory and puts the replaced one back. If it
// fails, the replaced directory stays in .trash and the error says where.
func (s *dirSwap) rollback() error {
	if err := os.RemoveAll(s.live); err != nil {
		return errors.Wrapf(err, "failed to remove %s while rolling back a plugin swap", s.live)
	}
	if s.trash == "" {
		return nil
	}
	if err := os.Rename(s.trash, s.live); err != nil {
		return errors.Wrapf(err, "failed to restore %s from %s while rolling back a plugin swap", s.live, s.trash)
	}
	_ = os.RemoveAll(s.trashParent)
	return nil
}

// withRollback returns err, adding a rollback failure to it if swap cannot
// be undone.
func withRollback(err error, swap *dirSwap) error {
	if rollbackErr := swap.rollback(); rollbackErr != nil {
		return errors.Wrapf(err, "rollback also failed: %v", rollbackErr)
	}
	return err
}

// sweepTransitDirs cleans up after transitions a crash interrupted. It runs
// at startup, before any transition can be in flight. In every scope it
// first moves back each replaced directory in .trash whose plugin has no
// live directory (the crash came between swapIn's two renames), then
// removes .staging and .trash. A .trash it could not restore from is kept.
func sweepTransitDirs(pluginDir string) {
	log := logger.New()
	scopes, err := os.ReadDir(pluginDir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warn("failed to read the plugin directory", logger.Data{"path": pluginDir, "error": err.Error()})
		}
		return
	}
	for _, scope := range scopes {
		if !scope.IsDir() || !validPathSegment(scope.Name()) {
			continue
		}
		scopeDir := filepath.Join(pluginDir, scope.Name())
		trashRoot := filepath.Join(scopeDir, trashDirName)
		keepTrash := false
		parents, _ := os.ReadDir(trashRoot)
		for _, parent := range parents {
			replaced, _ := os.ReadDir(filepath.Join(trashRoot, parent.Name()))
			for _, entry := range replaced {
				live := filepath.Join(scopeDir, entry.Name())
				if _, err := os.Lstat(live); !os.IsNotExist(err) {
					continue
				}
				old := filepath.Join(trashRoot, parent.Name(), entry.Name())
				if err := os.Rename(old, live); err != nil {
					keepTrash = true
					log.Error("failed to restore a plugin directory left in trash", logger.Data{"path": live, "trash": old, "error": err.Error()})
					continue
				}
				log.Warn("restored a plugin directory an interrupted update left in trash", logger.Data{"path": live})
			}
		}
		dirs := []string{filepath.Join(scopeDir, stagingDirName)}
		if !keepTrash {
			dirs = append(dirs, trashRoot)
		}
		for _, dir := range dirs {
			if err := os.RemoveAll(dir); err != nil {
				log.Warn("failed to remove leftover plugin directory", logger.Data{"path": dir, "error": err.Error()})
			}
		}
	}
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
		return "", errors.WithStack(fmt.Errorf("%w: %w", ErrDownloadFailed, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.WithStack(fmt.Errorf("%w: HTTP %d", ErrDownloadFailed, resp.StatusCode))
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
		return errors.WithStack(fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expected, actual))
	}

	return nil
}

// extractZip extracts a ZIP file to the destination directory.
func (inst *Installer) extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return invalidPackage("not a ZIP file", err)
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

		// Keep the owner's read and write bits, so an entry stored without
		// permission bits does not leave a file the server cannot read.
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode().Perm()|0o600)
		if err != nil {
			return errors.Wrapf(err, "failed to create file %s", name)
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return invalidPackage("unreadable ZIP entry "+name, err)
		}

		// Limit extraction to 100MB per file to prevent decompression bombs.
		// A failure reading the entry (bad compressed data, a CRC mismatch)
		// is a corrupt package; a failure writing it is a server fault.
		const maxFileSize = 100 * 1024 * 1024
		entry := &readErrorRecorder{r: io.LimitReader(rc, maxFileSize)}
		_, err = io.Copy(outFile, entry)
		rc.Close()
		outFile.Close()
		if entry.err != nil {
			return invalidPackage("corrupt ZIP entry "+name, entry.err)
		}
		if err != nil {
			return errors.Wrapf(err, "failed to extract %s", name)
		}
	}

	return nil
}

// readErrorRecorder remembers the first read error other than io.EOF, so a
// copy can tell a failed read from a failed write.
type readErrorRecorder struct {
	r   io.Reader
	err error
}

func (r *readErrorRecorder) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) && r.err == nil {
		r.err = err
	}
	return n, err
}

// DownloadPluginImage downloads an image from the given URL and saves it as
// icon.png in dir, a staged package's directory, so the icon is installed
// with the rest of the package. Errors are non-fatal and logged by the
// caller.
func (inst *Installer) DownloadPluginImage(ctx context.Context, dir, imageURL string) error {
	if imageURL == "" {
		return nil
	}

	if !isAllowedDownloadURL(imageURL) {
		return errors.Errorf("image URL not from allowed host: %s", imageURL)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return errors.Wrap(err, "failed to create image download request")
	}

	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to download plugin image")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed to download plugin image: HTTP %d", resp.StatusCode)
	}

	destPath := filepath.Join(dir, "icon.png")

	// Limit image download to 5MB
	const maxImageSize = 5 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize))
	if err != nil {
		return errors.Wrap(err, "failed to read plugin image")
	}

	if err := os.WriteFile(destPath, data, 0600); err != nil {
		return errors.Wrap(err, "failed to save plugin image")
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
