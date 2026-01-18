package cbzpages

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// maxImageSize is the maximum size for a single page image (100 MB).
// This prevents decompression bombs from consuming excessive memory.
const maxImageSize = 100 * 1024 * 1024

// Cache manages extracted CBZ page images.
type Cache struct {
	dir string
}

// NewCache creates a new Cache with the given directory.
func NewCache(dir string) *Cache {
	return &Cache{dir: dir}
}

// GetPage returns the path to a cached page image, extracting if necessary.
// pageNum is 0-indexed.
func (c *Cache) GetPage(cbzPath string, fileID int, pageNum int) (cachedPath string, mimeType string, err error) {
	// Check if page is already cached
	cacheDir := c.pageDir(fileID)
	pattern := filepath.Join(cacheDir, fmt.Sprintf("page_%d.*", pageNum))
	matches, _ := filepath.Glob(pattern)
	if len(matches) > 0 {
		return matches[0], mimeTypeFromPath(matches[0]), nil
	}

	// Extract the page from the CBZ
	return c.extractPage(cbzPath, fileID, pageNum)
}

// extractPage extracts a single page from a CBZ file and caches it.
func (c *Cache) extractPage(cbzPath string, fileID int, pageNum int) (cachedPath string, mimeType string, err error) {
	f, err := os.Open(cbzPath)
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	defer f.Close()

	stats, err := f.Stat()
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	zipReader, err := zip.NewReader(f, stats.Size())
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	// Get sorted image files
	imageFiles := getSortedImageFiles(zipReader)
	if pageNum < 0 || pageNum >= len(imageFiles) {
		return "", "", errors.Errorf("page %d out of range (0-%d)", pageNum, len(imageFiles)-1)
	}

	targetFile := imageFiles[pageNum]

	// Create cache directory
	cacheDir := c.pageDir(fileID)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", "", errors.WithStack(err)
	}

	// Extract the page
	ext := strings.ToLower(filepath.Ext(targetFile.Name))
	cachedPath = filepath.Join(cacheDir, fmt.Sprintf("page_%d%s", pageNum, ext))

	r, err := targetFile.Open()
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	defer r.Close()

	outFile, err := os.Create(cachedPath)
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	defer outFile.Close()

	// Use LimitReader to prevent decompression bombs
	_, err = io.Copy(outFile, io.LimitReader(r, maxImageSize))
	if err != nil {
		os.Remove(cachedPath)
		return "", "", errors.WithStack(err)
	}

	return cachedPath, mimeTypeFromPath(cachedPath), nil
}

// pageDir returns the cache directory for a file's pages.
func (c *Cache) pageDir(fileID int) string {
	return filepath.Join(c.dir, "cbz", strconv.Itoa(fileID))
}

// Invalidate removes all cached pages for a file.
func (c *Cache) Invalidate(fileID int) error {
	return os.RemoveAll(c.pageDir(fileID))
}

// getSortedImageFiles returns a sorted list of image files from a zip reader.
func getSortedImageFiles(zipReader *zip.Reader) []*zip.File {
	var imageFiles []*zip.File
	for _, file := range zipReader.File {
		ext := strings.ToLower(filepath.Ext(file.Name))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp" {
			imageFiles = append(imageFiles, file)
		}
	}

	sort.Slice(imageFiles, func(i, j int) bool {
		return imageFiles[i].Name < imageFiles[j].Name
	})

	return imageFiles
}

// mimeTypeFromPath returns the MIME type based on file extension.
func mimeTypeFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
