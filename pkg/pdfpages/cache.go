package pdfpages

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"

	"github.com/klippa-app/go-pdfium/requests"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/lrufiles"
	"github.com/shishobooks/shisho/pkg/pdf"
)

// Cache manages rendered PDF page images.
type Cache struct {
	dir     string
	dpi     int
	quality int
	lru     *lrufiles.Index
}

// NewCache creates a new Cache with the given base directory and render settings.
func NewCache(dir string, dpi int, quality int) *Cache {
	return &Cache{dir: dir, dpi: dpi, quality: quality, lru: lrufiles.New(filepath.Join(dir, "pdf"))}
}

// RenderKey returns the key of this cache's render settings.
func (c *Cache) RenderKey() string {
	return RenderKey(c.dpi, c.quality)
}

// RenderKey identifies the render settings, as "{dpi}-{quality}". Rendered
// pages are stored under it, and the frontend adds it to PDF page URLs, so a
// restart with a new pdf_render_dpi or pdf_render_quality renders new pages
// instead of serving the old ones from disk or from the browser cache.
func RenderKey(dpi int, quality int) string {
	return fmt.Sprintf("%d-%d", dpi, quality)
}

// ErrPageOutOfRange is returned by GetPage for a page the PDF does not have.
var ErrPageOutOfRange = errors.New("page out of range")

// GetPage returns the path to a cached page image, rendering if necessary.
// pageNum is 0-indexed.
func (c *Cache) GetPage(pdfPath string, fileID int, pageNum int) (cachedPath string, mimeType string, err error) {
	if pageNum < 0 {
		return "", "", errors.Wrapf(ErrPageOutOfRange, "page %d", pageNum)
	}

	// Check if page is already cached
	expected := c.pagePath(fileID, pageNum)
	if c.lru.Used(expected) {
		return expected, "image/jpeg", nil
	}

	path, err := c.renderPage(pdfPath, pageNum, expected, 0)
	if err != nil {
		return "", "", err
	}
	return path, "image/jpeg", nil
}

// GetThumbnailPage returns the path to a cached rendering of the page that
// fits within size pixels on its longest side, rendering it if necessary.
// It renders straight at that size, a fraction of the work of a full page at
// the configured DPI, for page thumbnails. pageNum is 0-indexed.
func (c *Cache) GetThumbnailPage(pdfPath string, fileID int, pageNum int, size int) (string, error) {
	if pageNum < 0 {
		return "", errors.Wrapf(ErrPageOutOfRange, "page %d", pageNum)
	}
	expected := c.thumbnailPath(fileID, pageNum, size)
	if c.lru.Used(expected) {
		return expected, nil
	}
	return c.renderPage(pdfPath, pageNum, expected, size)
}

// renderPage renders a single PDF page to outPath as JPEG: at the configured
// DPI, or within size pixels on its longest side when size is not 0.
// Thread safety: concurrent calls are serialized by the pdfium pool (MaxTotal: 1).
func (c *Cache) renderPage(pdfPath string, pageNum int, outPath string, size int) (string, error) {
	instance, err := pdf.PdfiumInstance(pdf.InteractivePdfiumTimeout)
	if err != nil {
		return "", errors.Wrap(err, "failed to get pdfium instance")
	}
	defer instance.Close()

	doc, err := instance.OpenDocument(&requests.OpenDocument{
		FilePath: &pdfPath,
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to open PDF")
	}
	defer func() {
		_, _ = instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{
			Document: doc.Document,
		})
	}()

	// Validate page bounds
	pageCountResp, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{
		Document: doc.Document,
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to get page count")
	}
	if pageNum >= pageCountResp.PageCount {
		return "", errors.Wrapf(ErrPageOutOfRange, "page %d (0-%d)", pageNum, pageCountResp.PageCount-1)
	}

	page := requests.Page{
		ByIndex: &requests.PageByIndex{
			Document: doc.Document,
			Index:    pageNum,
		},
	}
	var rendered image.Image
	if size > 0 {
		// Width and Height bound the image; PDFium keeps the page's shape.
		render, err := instance.RenderPageInPixels(&requests.RenderPageInPixels{
			Page:   page,
			Width:  size,
			Height: size,
		})
		if err != nil {
			return "", errors.Wrap(err, "failed to render page")
		}
		defer render.Cleanup()
		rendered = render.Result.RenderedImage
	} else {
		render, err := instance.RenderPageInDPI(&requests.RenderPageInDPI{
			DPI:  c.dpi,
			Page: page,
		})
		if err != nil {
			return "", errors.Wrap(err, "failed to render page")
		}
		defer render.Cleanup()
		rendered = render.Result.RenderedImage
	}

	// Encode to JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, rendered, &jpeg.Options{Quality: c.quality}); err != nil {
		return "", errors.Wrap(err, "failed to encode JPEG")
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return "", errors.WithStack(err)
	}

	// Write atomically so a concurrent request's existence check never
	// serves a partially written page.
	if err := fileutils.WriteFileAtomic(outPath, buf.Bytes(), 0644); err != nil { //nolint:gosec // Cache files need to be readable by the HTTP server
		return "", err
	}
	c.lru.Added(outPath)

	return outPath, nil
}

// Invalidate removes all cached pages for a file, at every render setting.
// The cache does not notice a file replaced on disk, so whatever replaces a
// file must call this (the scan does). Like Clear it is a plain RemoveAll: a
// render that opened the old file before the swap can write one stale page
// back afterwards. The window is one page render, so it is accepted rather
// than locked.
func (c *Cache) Invalidate(fileID int) error {
	if err := os.RemoveAll(c.fileDir(fileID)); err != nil {
		return errors.WithStack(err)
	}
	c.lru.Forget(c.fileDir(fileID))
	return nil
}

// SetMaxBytes changes the disk budget, lrufiles.Unlimited for none, and
// evicts the least recently used pages down to it now. Pages rendered at
// earlier render settings count toward it and are the first to go.
func (c *Cache) SetMaxBytes(maxBytes int64) error {
	return c.lru.SetMaxBytes(maxBytes)
}

// rootDir returns the directory this cache owns.
func (c *Cache) rootDir() string {
	return filepath.Join(c.dir, "pdf")
}

// SizeBytes returns the total bytes and file count under the cache root.
// A missing root is treated as empty.
func (c *Cache) SizeBytes() (int64, int, error) {
	var totalBytes int64
	var totalCount int

	root := c.rootDir()
	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		totalBytes += info.Size()
		totalCount++
		return nil
	})
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to walk cache")
	}
	return totalBytes, totalCount, nil
}

// Clear removes the cache root directory entirely. Safe when missing.
//
// A concurrent GetPage call may race the removal and fail with ENOENT as its
// MkdirAll/WriteFile sequence hits the deleted tree; the next attempt recreates
// the directory and succeeds. There is no mutex here: the pdfium pool
// serializes renders, and the race is accepted rather than locked, so
// callers must not assume Clear is invisible to concurrent readers.
func (c *Cache) Clear() error {
	if err := os.RemoveAll(c.rootDir()); err != nil {
		return errors.Wrap(err, "failed to clear cache")
	}
	c.lru.Reset()
	return nil
}

// fileDir returns the cache directory for everything rendered from a file,
// at any render settings. Invalidate removes it.
func (c *Cache) fileDir(fileID int) string {
	return filepath.Join(c.dir, "pdf", strconv.Itoa(fileID))
}

// pageDir returns the cache directory for a file's pages at the current
// render settings.
func (c *Cache) pageDir(fileID int) string {
	return filepath.Join(c.fileDir(fileID), c.RenderKey())
}

// pagePath returns the expected cache path for a specific page.
func (c *Cache) pagePath(fileID int, pageNum int) string {
	return filepath.Join(c.pageDir(fileID), fmt.Sprintf("page_%d.jpg", pageNum))
}

// thumbnailPath returns the cache path for a page rendered within size
// pixels, under the file's directory so Invalidate removes it too. It is
// keyed by the JPEG quality, the one render setting a thumbnail shares with
// a full page.
func (c *Cache) thumbnailPath(fileID int, pageNum int, size int) string {
	return filepath.Join(c.fileDir(fileID), fmt.Sprintf("thumb-%d-%d", size, c.quality), fmt.Sprintf("page_%d.jpg", pageNum))
}
