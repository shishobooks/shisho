package pdfpages

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/klippa-app/go-pdfium/requests"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/pdf"
)

// Cache manages rendered PDF page images.
type Cache struct {
	dir     string
	dpi     int
	quality int
}

// NewCache creates a new Cache with the given base directory and render settings.
func NewCache(dir string, dpi int, quality int) *Cache {
	return &Cache{dir: dir, dpi: dpi, quality: quality}
}

// GetPage returns the path to a cached page image, rendering if necessary.
// pageNum is 0-indexed.
func (c *Cache) GetPage(pdfPath string, fileID int, pageNum int) (cachedPath string, mimeType string, err error) {
	if pageNum < 0 {
		return "", "", errors.Errorf("page %d out of range", pageNum)
	}

	// Check if page is already cached
	expected := c.pagePath(fileID, pageNum)
	if _, err := os.Stat(expected); err == nil {
		return expected, "image/jpeg", nil
	}

	// Render the page
	return c.renderPage(pdfPath, fileID, pageNum)
}

// renderPage renders a single PDF page and caches the result as JPEG.
// Thread safety: concurrent calls are serialized by the pdfium pool (MaxTotal: 1).
func (c *Cache) renderPage(pdfPath string, fileID int, pageNum int) (cachedPath string, mimeType string, err error) {
	instance, err := pdf.PdfiumInstance(30 * time.Second)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get pdfium instance")
	}
	defer instance.Close()

	doc, err := instance.OpenDocument(&requests.OpenDocument{
		FilePath: &pdfPath,
	})
	if err != nil {
		return "", "", errors.Wrap(err, "failed to open PDF")
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
		return "", "", errors.Wrap(err, "failed to get page count")
	}
	if pageNum >= pageCountResp.PageCount {
		return "", "", errors.Errorf("page %d out of range (0-%d)", pageNum, pageCountResp.PageCount-1)
	}

	// Render the page
	render, err := instance.RenderPageInDPI(&requests.RenderPageInDPI{
		DPI: c.dpi,
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: doc.Document,
				Index:    pageNum,
			},
		},
	})
	if err != nil {
		return "", "", errors.Wrap(err, "failed to render page")
	}
	defer render.Cleanup()

	// Encode to JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, render.Result.Image, &jpeg.Options{Quality: c.quality}); err != nil {
		return "", "", errors.Wrap(err, "failed to encode JPEG")
	}

	// Write to cache
	cacheDir := c.pageDir(fileID)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", "", errors.WithStack(err)
	}

	outPath := c.pagePath(fileID, pageNum)
	if err := os.WriteFile(outPath, buf.Bytes(), 0644); err != nil {
		return "", "", errors.WithStack(err)
	}

	return outPath, "image/jpeg", nil
}

// Invalidate removes all cached pages for a file.
func (c *Cache) Invalidate(fileID int) error {
	return os.RemoveAll(c.pageDir(fileID))
}

// pageDir returns the cache directory for a file's rendered pages.
func (c *Cache) pageDir(fileID int) string {
	return filepath.Join(c.dir, "pdf", strconv.Itoa(fileID))
}

// pagePath returns the expected cache path for a specific page.
func (c *Cache) pagePath(fileID int, pageNum int) string {
	return filepath.Join(c.pageDir(fileID), fmt.Sprintf("page_%d.jpg", pageNum))
}
