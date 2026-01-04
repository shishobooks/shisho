package fileutils

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	_ "image/gif" // Register GIF decoder for image normalization.

	_ "golang.org/x/image/webp" // Register WebP decoder for image normalization.
)

// OrganizeFileResult contains the results of organizing a file.
type OrganizeFileResult struct {
	OriginalPath  string
	NewPath       string
	FolderCreated bool
	Moved         bool
	CoversMoved   int
	CoversError   error
}

// OrganizeRootLevelFile creates a folder and moves a root-level file into it.
func OrganizeRootLevelFile(originalPath string, opts OrganizedNameOptions) (*OrganizeFileResult, error) {
	result := &OrganizeFileResult{
		OriginalPath: originalPath,
	}

	// Get the directory containing the original file
	baseDir := filepath.Dir(originalPath)

	// Generate the organized folder name
	folderName := GenerateOrganizedFolderName(opts)
	targetFolder := filepath.Join(baseDir, folderName)

	// Generate the organized filename
	filename := GenerateOrganizedFileName(opts, originalPath)
	targetPath := filepath.Join(targetFolder, filename)

	result.NewPath = targetPath

	// Check if target folder already exists
	if _, err := os.Stat(targetFolder); os.IsNotExist(err) {
		// Create the target folder
		err := os.MkdirAll(targetFolder, 0755)
		if err != nil {
			return result, errors.WithStack(err)
		}
		result.FolderCreated = true
	}

	// Check if a file already exists at the target path
	if _, err := os.Stat(targetPath); err == nil {
		// File already exists, generate a unique name
		targetPath = generateUniqueFilepath(targetPath)
		result.NewPath = targetPath
	}

	// Move the file
	err := moveFile(originalPath, targetPath)
	if err != nil {
		// If we created the folder and the move failed, try to clean up
		if result.FolderCreated {
			os.RemoveAll(targetFolder)
		}
		return result, errors.WithStack(err)
	}

	// Move associated cover images
	coversMoved, err := moveAssociatedCovers(originalPath, targetPath)
	result.CoversMoved = coversMoved
	if err != nil {
		result.CoversError = err
		// Don't fail the entire operation - the main file has been moved successfully
	}

	result.Moved = true
	return result, nil
}

// RenameOrganizedFile renames an already organized file with new metadata.
func RenameOrganizedFile(currentPath string, opts OrganizedNameOptions) (string, error) {
	// Get the directory containing the current file
	currentDir := filepath.Dir(currentPath)

	// Generate new filename
	newFilename := GenerateOrganizedFileName(opts, currentPath)
	newPath := filepath.Join(currentDir, newFilename)

	// If the path is the same, no need to rename
	if currentPath == newPath {
		return currentPath, nil
	}

	// Check if a file already exists at the target path
	if _, err := os.Stat(newPath); err == nil {
		// File already exists, generate a unique name
		newPath = generateUniqueFilepath(newPath)
	}

	// Rename the file
	err := os.Rename(currentPath, newPath)
	if err != nil {
		return currentPath, errors.WithStack(err)
	}

	return newPath, nil
}

// RenameOrganizedFolder renames a folder containing organized files.
func RenameOrganizedFolder(currentFolderPath string, opts OrganizedNameOptions) (string, error) {
	// Get the parent directory
	parentDir := filepath.Dir(currentFolderPath)

	// Generate new folder name
	newFolderName := GenerateOrganizedFolderName(opts)
	newFolderPath := filepath.Join(parentDir, newFolderName)

	// If the path is the same, no need to rename
	if currentFolderPath == newFolderPath {
		return currentFolderPath, nil
	}

	// Check if a folder already exists at the target path
	if _, err := os.Stat(newFolderPath); err == nil {
		// Folder already exists, generate a unique name
		newFolderPath = generateUniqueDirpath(newFolderPath)
	}

	// Rename the folder
	err := os.Rename(currentFolderPath, newFolderPath)
	if err != nil {
		return currentFolderPath, errors.WithStack(err)
	}

	return newFolderPath, nil
}

// moveFile safely moves a file from source to destination.
func moveFile(src, dst string) error {
	// Try a simple rename first (fastest, works if src and dst are on same filesystem)
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// If rename failed, do a copy + delete
	err = copyFile(src, dst)
	if err != nil {
		return errors.WithStack(err)
	}

	// Remove the source file only after successful copy
	err = os.Remove(src)
	if err != nil {
		// If we can't remove the source, try to clean up the destination
		os.Remove(dst)
		return errors.WithStack(err)
	}

	return nil
}

// copyFile copies a file from source to destination.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return errors.WithStack(err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return errors.WithStack(err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return errors.WithStack(err)
	}

	// Copy file permissions
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return errors.WithStack(err)
	}

	err = destFile.Chmod(sourceInfo.Mode())
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// generateUniqueFilepath creates a unique filepath by appending a number if needed.
func generateUniqueFilepath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := filepath.Base(path)
	nameWithoutExt := base[:len(base)-len(ext)]

	for i := 1; i < 1000; i++ {
		newName := fmt.Sprintf("%s (%d)%s", nameWithoutExt, i, ext)
		newPath := filepath.Join(dir, newName)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
	}

	// Fallback - this should rarely happen
	return path
}

// generateUniqueDirpath creates a unique directory path by appending a number if needed.
func generateUniqueDirpath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	parent := filepath.Dir(path)
	name := filepath.Base(path)

	for i := 1; i < 1000; i++ {
		newName := fmt.Sprintf("%s (%d)", name, i)
		newPath := filepath.Join(parent, newName)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
	}

	// Fallback - this should rarely happen
	return path
}

// moveAssociatedCovers finds and moves cover images and sidecar files associated with a file.
func moveAssociatedCovers(originalFilePath, newFilePath string) (int, error) {
	originalDir := filepath.Dir(originalFilePath)
	newDir := filepath.Dir(newFilePath)

	// Get the full filename (including extension) for cover naming
	originalFilename := filepath.Base(originalFilePath)
	newFilename := filepath.Base(newFilePath)

	// Common image extensions for covers
	imageExtensions := []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp"}

	coversMoved := 0

	// Look for individual covers: {filename}.cover.{ext}
	// e.g., mybook.epub.cover.jpg for mybook.epub
	for _, ext := range imageExtensions {
		originalCoverName := originalFilename + ".cover" + ext
		originalCoverPath := filepath.Join(originalDir, originalCoverName)

		// Check if this cover exists
		if _, err := os.Stat(originalCoverPath); err == nil {
			// Generate the new cover name
			newCoverName := newFilename + ".cover" + ext
			newCoverPath := filepath.Join(newDir, newCoverName)

			// Move the cover
			err := moveFile(originalCoverPath, newCoverPath)
			if err != nil {
				return coversMoved, errors.WithStack(err)
			}
			coversMoved++
		}
	}

	// Move file sidecar: {filename}.metadata.json
	originalFileSidecar := originalFilePath + ".metadata.json"
	if _, err := os.Stat(originalFileSidecar); err == nil {
		newFileSidecar := newFilePath + ".metadata.json"
		if err := moveFile(originalFileSidecar, newFileSidecar); err != nil {
			return coversMoved, errors.WithStack(err)
		}
	}

	// Move book sidecar for root-level files: {basename}.metadata.json
	// Book sidecars use the filename without extension
	originalBaseName := getBaseNameWithoutExt(originalFilePath)
	newBaseName := getBaseNameWithoutExt(newFilePath)
	originalBookSidecar := filepath.Join(originalDir, originalBaseName+".metadata.json")
	if _, err := os.Stat(originalBookSidecar); err == nil {
		newBookSidecar := filepath.Join(newDir, newBaseName+".metadata.json")
		if err := moveFile(originalBookSidecar, newBookSidecar); err != nil {
			return coversMoved, errors.WithStack(err)
		}
	}

	// Also move canonical covers if they exist in the same directory
	// Only move them if this is the only file in the directory
	files, err := os.ReadDir(originalDir)
	if err != nil {
		return coversMoved, errors.WithStack(err)
	}

	// Count non-cover files in the directory
	fileCount := 0
	for _, file := range files {
		if !file.IsDir() {
			name := file.Name()
			// Skip if it's a cover file
			if strings.Contains(name, ".cover.") ||
				strings.HasPrefix(name, "cover.") ||
				strings.HasPrefix(name, "audiobook_cover.") {
				continue
			}
			fileCount++
		}
	}

	// Only move canonical covers if this was the only non-cover file in the directory
	if fileCount <= 1 {
		canonicalCovers := []string{"cover", "audiobook_cover"}
		for _, coverBase := range canonicalCovers {
			for _, ext := range imageExtensions {
				coverName := coverBase + ext
				originalCoverPath := filepath.Join(originalDir, coverName)

				if _, err := os.Stat(originalCoverPath); err == nil {
					newCoverPath := filepath.Join(newDir, coverName)
					err := moveFile(originalCoverPath, newCoverPath)
					if err != nil {
						return coversMoved, errors.WithStack(err)
					}
					coversMoved++
				}
			}
		}
	}

	return coversMoved, nil
}

// getBaseNameWithoutExt returns the filename without its directory and extension.
func getBaseNameWithoutExt(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// CoverImageExtensions contains all supported image extensions for cover files.
var CoverImageExtensions = []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp"}

// CoverExistsWithBaseName checks if any cover file exists with the given base name,
// regardless of image extension. This allows users to provide custom covers (e.g., cover.png)
// that won't be overwritten even if the book would extract a different format (e.g., cover.jpg).
//
// Parameters:
//   - dir: the directory to check
//   - baseName: the cover base name without extension (e.g., "cover", "book_cover", "audiobook_cover")
//
// Returns the path to the existing cover file if found, or empty string if no cover exists.
func CoverExistsWithBaseName(dir, baseName string) string {
	for _, ext := range CoverImageExtensions {
		coverPath := filepath.Join(dir, baseName+ext)
		if _, err := os.Stat(coverPath); err == nil {
			return coverPath
		}
	}
	return ""
}

// NormalizeImage decodes and re-encodes an image to strip problematic metadata
// (like gAMA chunks without sRGB in PNG) that cause color rendering issues in browsers.
// Returns the normalized image data and the new MIME type.
// If the input is a JPEG, it stays as JPEG to preserve quality. Otherwise, it becomes PNG.
func NormalizeImage(data []byte, mimeType string) ([]byte, string, error) {
	// Decode the image
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// If we can't decode, return original data
		return data, mimeType, nil
	}

	var buf bytes.Buffer

	// Preserve JPEG format to avoid quality loss, otherwise use PNG
	if mimeType == "image/jpeg" || mimeType == "image/jpg" {
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
			return data, mimeType, nil
		}
		return buf.Bytes(), "image/jpeg", nil
	}

	// Re-encode as PNG (universal, lossless)
	if err := png.Encode(&buf, img); err != nil {
		// If we can't encode, return original data
		return data, mimeType, nil
	}

	return buf.Bytes(), "image/png", nil
}
