package plugins

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
	"github.com/pkg/errors"
)

var (
	errZipSlip       = errors.New("zip slip detected: entry resolves outside destination")
	errEntryNotFound = errors.New("entry not found in archive")
)

// injectArchiveNamespace sets up shisho.archive with ZIP manipulation functions.
// All functions use the Runtime's fsCtx for path validation.
func injectArchiveNamespace(vm *goja.Runtime, shishoObj *goja.Object, rt *Runtime) error {
	archiveObj := vm.NewObject()
	if err := shishoObj.Set("archive", archiveObj); err != nil {
		return fmt.Errorf("failed to set shisho.archive: %w", err)
	}

	archiveObj.Set("extractZip", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		ctx := rt.fsCtx
		if ctx == nil {
			panic(vm.ToValue("shisho.archive.extractZip: no filesystem context available"))
		}
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("shisho.archive.extractZip: archivePath and destDir arguments are required"))
		}
		archivePath := call.Argument(0).String()
		destDir := call.Argument(1).String()

		if !ctx.isReadAllowed(archivePath) {
			panic(vm.ToValue("shisho.archive.extractZip: read access denied for path: " + archivePath))
		}
		if !ctx.isWriteAllowed(destDir) {
			panic(vm.ToValue("shisho.archive.extractZip: write access denied for path: " + destDir))
		}

		if err := doExtractZip(archivePath, destDir); err != nil {
			panic(vm.ToValue("shisho.archive.extractZip: " + err.Error()))
		}

		return goja.Undefined()
	})

	archiveObj.Set("createZip", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		ctx := rt.fsCtx
		if ctx == nil {
			panic(vm.ToValue("shisho.archive.createZip: no filesystem context available"))
		}
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("shisho.archive.createZip: srcDir and destPath arguments are required"))
		}
		srcDir := call.Argument(0).String()
		destPath := call.Argument(1).String()

		if !ctx.isReadAllowed(srcDir) {
			panic(vm.ToValue("shisho.archive.createZip: read access denied for path: " + srcDir))
		}
		if !ctx.isWriteAllowed(destPath) {
			panic(vm.ToValue("shisho.archive.createZip: write access denied for path: " + destPath))
		}

		if err := doCreateZip(srcDir, destPath); err != nil {
			panic(vm.ToValue("shisho.archive.createZip: " + err.Error()))
		}

		return goja.Undefined()
	})

	archiveObj.Set("readZipEntry", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		ctx := rt.fsCtx
		if ctx == nil {
			panic(vm.ToValue("shisho.archive.readZipEntry: no filesystem context available"))
		}
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("shisho.archive.readZipEntry: archivePath and entryPath arguments are required"))
		}
		archivePath := call.Argument(0).String()
		entryPath := call.Argument(1).String()

		if !ctx.isReadAllowed(archivePath) {
			panic(vm.ToValue("shisho.archive.readZipEntry: read access denied for path: " + archivePath))
		}

		data, err := doReadZipEntry(archivePath, entryPath)
		if err != nil {
			panic(vm.ToValue("shisho.archive.readZipEntry: " + err.Error()))
		}

		return vm.ToValue(vm.NewArrayBuffer(data))
	})

	archiveObj.Set("listZipEntries", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		ctx := rt.fsCtx
		if ctx == nil {
			panic(vm.ToValue("shisho.archive.listZipEntries: no filesystem context available"))
		}
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.archive.listZipEntries: archivePath argument is required"))
		}
		archivePath := call.Argument(0).String()

		if !ctx.isReadAllowed(archivePath) {
			panic(vm.ToValue("shisho.archive.listZipEntries: read access denied for path: " + archivePath))
		}

		entries, err := doListZipEntries(archivePath)
		if err != nil {
			panic(vm.ToValue("shisho.archive.listZipEntries: " + err.Error()))
		}

		result := make([]interface{}, len(entries))
		for i, e := range entries {
			result[i] = e
		}
		return vm.ToValue(result)
	})

	return nil
}

// doExtractZip extracts all files from a ZIP archive to the destination directory.
// It prevents zip slip attacks by validating that extracted paths stay within destDir.
func doExtractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	absDestDir, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("failed to resolve dest dir: %w", err)
	}

	for _, f := range r.File {
		// Construct the full path and validate against zip slip
		target := filepath.Join(absDestDir, f.Name) //nolint:gosec
		absTarget, err := filepath.Abs(target)
		if err != nil {
			return fmt.Errorf("failed to resolve path for entry %q: %w", f.Name, err)
		}

		// Zip slip protection: ensure the resolved path is within destDir
		if !isPathWithin(absTarget, absDestDir) {
			return errors.Wrapf(errZipSlip, "entry %q", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(absTarget, 0755); err != nil {
				return fmt.Errorf("failed to create directory %q: %w", f.Name, err)
			}
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(absTarget), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory for %q: %w", f.Name, err)
		}

		if err := extractZipFile(f, absTarget); err != nil {
			return err
		}
	}

	return nil
}

// maxZipEntrySize is the maximum size of a single zip entry (256 MB).
const maxZipEntrySize = 256 << 20

// extractZipFile extracts a single file from a zip archive to the target path.
func extractZipFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("failed to open entry %q: %w", f.Name, err)
	}
	defer rc.Close()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return fmt.Errorf("failed to create file %q: %w", f.Name, err)
	}
	defer out.Close()

	// Limit read size to prevent decompression bombs
	limited := io.LimitReader(rc, maxZipEntrySize)
	if _, err := io.Copy(out, limited); err != nil {
		return fmt.Errorf("failed to write entry %q: %w", f.Name, err)
	}

	return nil
}

// doCreateZip creates a ZIP archive from all files in srcDir.
func doCreateZip(srcDir, destPath string) error {
	absSrcDir, err := filepath.Abs(srcDir)
	if err != nil {
		return fmt.Errorf("failed to resolve src dir: %w", err)
	}

	outFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer outFile.Close()

	w := zip.NewWriter(outFile)
	defer w.Close()

	err = filepath.Walk(absSrcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Compute relative path for the zip entry
		relPath, err := filepath.Rel(absSrcDir, path)
		if err != nil {
			return fmt.Errorf("failed to compute relative path: %w", err)
		}

		// Skip the root directory itself
		if relPath == "." {
			return nil
		}

		// Use forward slashes in zip entries
		zipPath := strings.ReplaceAll(relPath, string(filepath.Separator), "/")

		if info.IsDir() {
			// Add directory entry (trailing slash)
			_, err := w.Create(zipPath + "/")
			return err
		}

		// Add file entry
		fw, err := w.Create(zipPath)
		if err != nil {
			return fmt.Errorf("failed to create zip entry %q: %w", zipPath, err)
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file %q: %w", path, err)
		}
		defer f.Close()

		if _, err := io.Copy(fw, f); err != nil {
			return fmt.Errorf("failed to write file %q to zip: %w", path, err)
		}

		return nil
	})

	return err
}

// doReadZipEntry reads a specific entry from a ZIP archive and returns its contents.
func doReadZipEntry(archivePath, entryPath string) ([]byte, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name == entryPath {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open entry %q: %w", entryPath, err)
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("failed to read entry %q: %w", entryPath, err)
			}
			return data, nil
		}
	}

	return nil, errors.Wrapf(errEntryNotFound, "entry %q", entryPath)
}

// doListZipEntries returns the names of all entries in a ZIP archive.
func doListZipEntries(archivePath string) ([]string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	entries := make([]string, len(r.File))
	for i, f := range r.File {
		entries[i] = f.Name
	}

	return entries, nil
}
