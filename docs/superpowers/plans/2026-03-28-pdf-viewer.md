# PDF Viewer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an in-app PDF viewer that renders pages as server-side JPEG images via pdfium WASM, displayed through a shared reader component extracted from the existing CBZ reader.

**Architecture:** Backend renders PDF pages to JPEG via go-pdfium WASM (same library used for cover extraction), caches them on disk like CBZ pages, and serves them through the existing `/files/:id/page/:pageNum` endpoint. Frontend extracts a shared `PageReader` component from `CBZReader.tsx`, then builds a thin `PDFReader` wrapper on top. A `FileReader` dispatcher at the route level selects the right wrapper based on file type.

**Tech Stack:** Go (go-pdfium WASM, pdfcpu), React (TypeScript), Echo web framework, SQLite via Bun ORM

**Spec:** `docs/superpowers/specs/2026-03-28-pdf-viewer-design.md`

---

### Task 1: Add PDF Render Config Fields

**Files:**
- Modify: `pkg/config/config.go`
- Modify: `shisho.example.yaml`
- Modify: `app/components/pages/AdminSettings.tsx`

- [ ] **Step 1: Add config fields to Config struct**

In `pkg/config/config.go`, add two fields after the `DownloadCacheMaxSizeGB` field (in the Cache settings section):

```go
// PDF viewer rendering settings
PDFRenderDPI     int `koanf:"pdf_render_dpi" json:"pdf_render_dpi"`
PDFRenderQuality int `koanf:"pdf_render_quality" json:"pdf_render_quality"`
```

- [ ] **Step 2: Add defaults**

In the `defaults()` function in `pkg/config/config.go`, add after `DownloadCacheMaxSizeGB: 5,`:

```go
PDFRenderDPI:     200,
PDFRenderQuality: 85,
```

- [ ] **Step 3: Update shisho.example.yaml**

In `shisho.example.yaml`, update the Cache Settings section. First update the `cache_dir` description to mention PDF pages, then add the new fields:

Replace the existing `cache_dir` comment block:
```yaml
# Directory for caching generated files
# This includes:
#   - {cache_dir}/downloads/  Generated download files with embedded metadata
#   - {cache_dir}/cbz/        Extracted CBZ page images for the comic viewer
# Env: CACHE_DIR
# Default: /config/cache
cache_dir: /config/cache
```

With:
```yaml
# Directory for caching generated files
# This includes:
#   - {cache_dir}/downloads/  Generated download files with embedded metadata
#   - {cache_dir}/cbz/        Extracted CBZ page images for the comic viewer
#   - {cache_dir}/pdf/        Rendered PDF page images for the PDF viewer
# Env: CACHE_DIR
# Default: /config/cache
cache_dir: /config/cache
```

Then add after the `download_cache_max_size_gb` entry:

```yaml

# DPI for rendering PDF pages in the viewer (higher = sharper but larger images)
# Range: 72-600. At 200 DPI, a typical letter-size page renders to ~1700x2200 pixels.
# Env: PDF_RENDER_DPI
# Default: 200
pdf_render_dpi: 200

# JPEG quality for rendered PDF pages (higher = better quality but larger files)
# Range: 1-100
# Env: PDF_RENDER_QUALITY
# Default: 85
pdf_render_quality: 85
```

- [ ] **Step 4: Display config in AdminSettings**

In `app/components/pages/AdminSettings.tsx`, find the `ConfigRow` for "Download Cache Max Size" (around line 188-192). After it, add:

```tsx
<ConfigRow
  description="Resolution for rendering PDF pages in the viewer"
  label="PDF Render DPI"
  value={`${config.pdf_render_dpi} DPI`}
/>
<ConfigRow
  description="JPEG quality for rendered PDF pages"
  label="PDF Render Quality"
  value={`${config.pdf_render_quality}`}
/>
```

- [ ] **Step 5: Run `make tygo` to regenerate TypeScript types**

Run: `make tygo`

This regenerates the TypeScript types from Go structs so the frontend picks up the new config fields. "Nothing to be done" is normal if types are already up-to-date.

- [ ] **Step 6: Run checks**

Run: `make check:quiet`
Expected: All checks pass.

- [ ] **Step 7: Commit**

```bash
git add pkg/config/config.go shisho.example.yaml app/components/pages/AdminSettings.tsx
git commit -m "[Backend] Add pdf_render_dpi and pdf_render_quality config fields"
```

---

### Task 2: Export Pdfium Instance from `pkg/pdf`

**Files:**
- Modify: `pkg/pdf/cover.go`

The pdfium WASM pool is lazily initialized in `cover.go` as unexported variables. The new `pdfpages` package needs access to it. Export a function to get an instance from the shared pool.

- [ ] **Step 1: Add exported functions to cover.go**

Add these exported functions at the end of `pkg/pdf/cover.go` (before the closing of the file):

```go
// EnsurePdfiumPoolInit initializes the go-pdfium WASM pool if not already done.
// Safe to call multiple times; initialization happens at most once.
func EnsurePdfiumPoolInit() error {
	pdfiumOnce.Do(initPdfiumPool)
	return pdfiumErr
}

// PdfiumInstance returns a pdfium instance from the shared WASM pool.
// The caller must call instance.Close() when done to return it to the pool.
func PdfiumInstance(timeout time.Duration) (pdfium.Pdfium, error) {
	if err := EnsurePdfiumPoolInit(); err != nil {
		return nil, err
	}
	return pdfiumPool.GetInstance(timeout)
}
```

- [ ] **Step 2: Update renderPageCover to use the new function**

In `pkg/pdf/cover.go`, replace the beginning of `renderPageCover`:

```go
func renderPageCover(path string) ([]byte, string, error) {
	pdfiumOnce.Do(initPdfiumPool)
	if pdfiumErr != nil {
		return nil, "", pdfiumErr
	}

	instance, err := pdfiumPool.GetInstance(30 * time.Second)
	if err != nil {
		return nil, "", err
	}
	defer instance.Close()
```

With:

```go
func renderPageCover(path string) ([]byte, string, error) {
	instance, err := PdfiumInstance(30 * time.Second)
	if err != nil {
		return nil, "", err
	}
	defer instance.Close()
```

- [ ] **Step 3: Run existing PDF tests**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-2e21b5e1 && go test ./pkg/pdf/ -v -count=1`
Expected: All existing tests pass (the refactor is behavioral no-op).

- [ ] **Step 4: Commit**

```bash
git add pkg/pdf/cover.go
git commit -m "[Backend] Export pdfium pool for shared use by pdfpages"
```

---

### Task 3: Create `pkg/pdfpages` Cache (TDD)

**Files:**
- Create: `pkg/pdfpages/cache.go`
- Create: `pkg/pdfpages/cache_test.go`

- [ ] **Step 1: Write the failing test file**

Create `pkg/pdfpages/cache_test.go`:

```go
package pdfpages

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testPDFPath string

func TestMain(m *testing.M) {
	// Create a temp dir for test fixtures
	dir, err := os.MkdirTemp("", "pdfpages-test-*")
	if err != nil {
		panic("failed to create temp dir: " + err.Error())
	}

	// Create a minimal 3-page PDF for testing
	testPDFPath = filepath.Join(dir, "test.pdf")
	if err := writeTestPDF(testPDFPath, 3); err != nil {
		panic("failed to create test PDF: " + err.Error())
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// writeTestPDF writes a minimal valid PDF with the given page count.
func writeTestPDF(outPath string, pageCount int) error {
	var b strings.Builder
	var offsets []int
	objNum := 1

	b.WriteString("%PDF-1.4\n")

	// Obj 1: Catalog
	offsets = append(offsets, b.Len())
	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n", objNum))
	objNum++

	// Obj 2: Pages
	offsets = append(offsets, b.Len())
	firstPageObj := objNum + 1
	kidsParts := make([]string, pageCount)
	for i := 0; i < pageCount; i++ {
		kidsParts[i] = fmt.Sprintf("%d 0 R", firstPageObj+i)
	}
	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 612 792] >>\nendobj\n",
		objNum, strings.Join(kidsParts, " "), pageCount))
	objNum++

	// Page objects
	for i := 0; i < pageCount; i++ {
		offsets = append(offsets, b.Len())
		b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent 2 0 R >>\nendobj\n", objNum))
		objNum++
	}

	// Xref
	xrefOffset := b.Len()
	b.WriteString("xref\n")
	b.WriteString(fmt.Sprintf("0 %d\n", objNum))
	b.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		b.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}

	// Trailer
	b.WriteString("trailer\n")
	b.WriteString(fmt.Sprintf("<< /Size %d /Root 1 0 R >>\n", objNum))
	b.WriteString("startxref\n")
	b.WriteString(fmt.Sprintf("%d\n", xrefOffset))
	b.WriteString("%%EOF\n")

	return os.WriteFile(outPath, []byte(b.String()), 0644)
}

func TestGetPage_RendersAndCaches(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, 150, 85)

	// First call: render and cache
	cachedPath, mimeType, err := cache.GetPage(testPDFPath, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", mimeType)
	assert.FileExists(t, cachedPath)

	// Verify the cached file is a valid JPEG
	data, err := os.ReadFile(cachedPath)
	require.NoError(t, err)
	_, err = jpeg.Decode(bytes.NewReader(data))
	require.NoError(t, err)

	// Second call: should return cached path (same result)
	cachedPath2, mimeType2, err := cache.GetPage(testPDFPath, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, cachedPath, cachedPath2)
	assert.Equal(t, "image/jpeg", mimeType2)
}

func TestGetPage_DifferentPages(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, 150, 85)

	path0, _, err := cache.GetPage(testPDFPath, 2, 0)
	require.NoError(t, err)

	path1, _, err := cache.GetPage(testPDFPath, 2, 1)
	require.NoError(t, err)

	path2, _, err := cache.GetPage(testPDFPath, 2, 2)
	require.NoError(t, err)

	// All paths should be different files
	assert.NotEqual(t, path0, path1)
	assert.NotEqual(t, path1, path2)
}

func TestGetPage_InvalidPageNumber(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, 150, 85)

	// Page number too high
	_, _, err := cache.GetPage(testPDFPath, 3, 99)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")

	// Negative page number
	_, _, err = cache.GetPage(testPDFPath, 3, -1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestGetPage_InvalidPDF(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, 150, 85)

	invalidPath := filepath.Join(t.TempDir(), "invalid.pdf")
	require.NoError(t, os.WriteFile(invalidPath, []byte("not a pdf"), 0644))

	_, _, err := cache.GetPage(invalidPath, 4, 0)
	assert.Error(t, err)
}

func TestGetPage_NonexistentFile(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, 150, 85)

	_, _, err := cache.GetPage("/nonexistent/file.pdf", 5, 0)
	assert.Error(t, err)
}

func TestInvalidate(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, 150, 85)

	// Render a page to create cache entry
	cachedPath, _, err := cache.GetPage(testPDFPath, 6, 0)
	require.NoError(t, err)
	assert.FileExists(t, cachedPath)

	// Invalidate
	err = cache.Invalidate(6)
	require.NoError(t, err)

	// Cached file should be gone
	_, err = os.Stat(cachedPath)
	assert.True(t, os.IsNotExist(err))
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-2e21b5e1 && go test ./pkg/pdfpages/ -v -count=1`
Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Implement the cache**

Create `pkg/pdfpages/cache.go`:

```go
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-2e21b5e1 && go test ./pkg/pdfpages/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/pdfpages/cache.go pkg/pdfpages/cache_test.go
git commit -m "[Backend] Add pdfpages cache for server-side PDF page rendering"
```

---

### Task 4: Add PDF Outline Extraction (TDD)

**Files:**
- Create: `pkg/pdf/outline.go`
- Create: `pkg/pdf/outline_test.go`
- Modify: `pkg/pdf/pdf.go` (integrate into Parse)

- [ ] **Step 1: Write the failing test**

Create `pkg/pdf/outline_test.go`:

```go
package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractOutline_WithBookmarks(t *testing.T) {
	t.Parallel()

	pdfPath := filepath.Join(t.TempDir(), "with-outline.pdf")
	require.NoError(t, writeRawPDFWithOutline(pdfPath, 5, []outlineFixture{
		{title: "Chapter 1", pageIndex: 0},
		{title: "Chapter 2", pageIndex: 2},
		{title: "Chapter 3", pageIndex: 4},
	}))

	entries, err := ExtractOutline(pdfPath)
	require.NoError(t, err)

	require.Len(t, entries, 3)
	assert.Equal(t, "Chapter 1", entries[0].Title)
	assert.Equal(t, 0, entries[0].StartPage)
	assert.Equal(t, "Chapter 2", entries[1].Title)
	assert.Equal(t, 2, entries[1].StartPage)
	assert.Equal(t, "Chapter 3", entries[2].Title)
	assert.Equal(t, 4, entries[2].StartPage)
}

func TestExtractOutline_NoBookmarks(t *testing.T) {
	t.Parallel()

	// Use the standard no-metadata test PDF (no outline)
	path := filepath.Join(testdataDir, "no-metadata.pdf")
	entries, err := ExtractOutline(path)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestExtractOutline_InvalidPDF(t *testing.T) {
	t.Parallel()

	path := filepath.Join(testdataDir, "invalid.pdf")
	_, err := ExtractOutline(path)
	assert.Error(t, err)
}

func TestParse_IncludesChaptersFromOutline(t *testing.T) {
	t.Parallel()

	pdfPath := filepath.Join(t.TempDir(), "with-chapters.pdf")
	require.NoError(t, writeRawPDFWithOutline(pdfPath, 3, []outlineFixture{
		{title: "Introduction", pageIndex: 0},
		{title: "Main Content", pageIndex: 1},
	}))

	meta, err := Parse(pdfPath)
	require.NoError(t, err)

	require.Len(t, meta.Chapters, 2)
	assert.Equal(t, "Introduction", meta.Chapters[0].Title)
	require.NotNil(t, meta.Chapters[0].StartPage)
	assert.Equal(t, 0, *meta.Chapters[0].StartPage)
	assert.Equal(t, "Main Content", meta.Chapters[1].Title)
	require.NotNil(t, meta.Chapters[1].StartPage)
	assert.Equal(t, 1, *meta.Chapters[1].StartPage)
}

// outlineFixture describes a bookmark entry for test PDF generation.
type outlineFixture struct {
	title     string
	pageIndex int // 0-indexed
}

// writeRawPDFWithOutline creates a minimal PDF with an outline (bookmark) tree.
// Each bookmark uses an explicit /Dest [pageRef /Fit] to point to a page.
func writeRawPDFWithOutline(outPath string, pageCount int, bookmarks []outlineFixture) error {
	return writeCleanPDFWithOutline(outPath, pageCount, bookmarks)
}

// writeCleanPDFWithOutline builds a PDF with bookmarks in a single pass.
func writeCleanPDFWithOutline(outPath string, pageCount int, bookmarks []outlineFixture) error {
	var b strings.Builder
	var offsets []int
	objNum := 1

	b.WriteString("%PDF-1.4\n")

	// Pre-compute object numbers
	catalogObj := objNum // 1
	objNum++
	pagesObj := objNum // 2
	objNum++

	// Page objects: 3 .. 3+pageCount-1
	firstPageObj := objNum
	pageObjNums := make([]int, pageCount)
	for i := 0; i < pageCount; i++ {
		pageObjNums[i] = objNum
		objNum++
	}

	outlinesObj := objNum // after pages
	objNum++

	// Bookmark objects
	bookmarkObjNums := make([]int, len(bookmarks))
	for i := range bookmarks {
		bookmarkObjNums[i] = objNum
		objNum++
	}

	// Write Catalog (obj 1)
	offsets = append(offsets, b.Len())
	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Catalog /Pages %d 0 R /Outlines %d 0 R >>\nendobj\n",
		catalogObj, pagesObj, outlinesObj))

	// Write Pages (obj 2)
	offsets = append(offsets, b.Len())
	kidsParts := make([]string, pageCount)
	for i := 0; i < pageCount; i++ {
		kidsParts[i] = fmt.Sprintf("%d 0 R", pageObjNums[i])
	}
	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 612 792] >>\nendobj\n",
		pagesObj, strings.Join(kidsParts, " "), pageCount))

	// Write Page objects
	for i := 0; i < pageCount; i++ {
		offsets = append(offsets, b.Len())
		b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent %d 0 R >>\nendobj\n",
			pageObjNums[i], pagesObj))
	}

	// Write Outlines root
	offsets = append(offsets, b.Len())
	if len(bookmarks) > 0 {
		b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>\nendobj\n",
			outlinesObj, bookmarkObjNums[0], bookmarkObjNums[len(bookmarkObjNums)-1], len(bookmarks)))
	} else {
		b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Outlines /Count 0 >>\nendobj\n", outlinesObj))
	}

	// Write Bookmark objects
	for i, bm := range bookmarks {
		offsets = append(offsets, b.Len())
		pageRef := pageObjNums[bm.pageIndex]

		var parts []string
		parts = append(parts, fmt.Sprintf("/Title (%s)", bm.title))
		parts = append(parts, fmt.Sprintf("/Parent %d 0 R", outlinesObj))
		parts = append(parts, fmt.Sprintf("/Dest [%d 0 R /Fit]", pageRef))

		if i > 0 {
			parts = append(parts, fmt.Sprintf("/Prev %d 0 R", bookmarkObjNums[i-1]))
		}
		if i < len(bookmarks)-1 {
			parts = append(parts, fmt.Sprintf("/Next %d 0 R", bookmarkObjNums[i+1]))
		}

		b.WriteString(fmt.Sprintf("%d 0 obj\n<< %s >>\nendobj\n", bookmarkObjNums[i], strings.Join(parts, " ")))
	}

	// Xref table
	xrefOffset := b.Len()
	b.WriteString("xref\n")
	b.WriteString(fmt.Sprintf("0 %d\n", objNum))
	b.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		b.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}

	// Trailer
	b.WriteString("trailer\n")
	b.WriteString(fmt.Sprintf("<< /Size %d /Root %d 0 R >>\n", objNum, catalogObj))
	b.WriteString("startxref\n")
	b.WriteString(fmt.Sprintf("%d\n", xrefOffset))
	b.WriteString("%%EOF\n")

	return os.WriteFile(outPath, []byte(b.String()), 0644)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-2e21b5e1 && go test ./pkg/pdf/ -run TestExtractOutline -v -count=1`
Expected: FAIL — `ExtractOutline` is not defined.

- [ ] **Step 3: Implement ExtractOutline**

Create `pkg/pdf/outline.go`:

```go
package pdf

import (
	"time"

	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
	"github.com/pkg/errors"
)

// OutlineEntry represents a single bookmark from a PDF's outline tree.
type OutlineEntry struct {
	Title     string
	StartPage int // 0-indexed page number
}

// ExtractOutline extracts the bookmark/outline tree from a PDF and returns
// a flat list of entries with their target page numbers.
// Returns an empty slice (not an error) if the PDF has no bookmarks.
func ExtractOutline(path string) ([]OutlineEntry, error) {
	instance, err := PdfiumInstance(30 * time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get pdfium instance")
	}
	defer instance.Close()

	doc, err := instance.OpenDocument(&requests.OpenDocument{
		FilePath: &path,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to open PDF")
	}
	defer func() {
		_, _ = instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{
			Document: doc.Document,
		})
	}()

	bookmarksResp, err := instance.GetBookmarks(&requests.GetBookmarks{
		Document: doc.Document,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get bookmarks")
	}

	if len(bookmarksResp.Bookmarks) == 0 {
		return nil, nil
	}

	var entries []OutlineEntry
	flattenBookmarks(bookmarksResp.Bookmarks, &entries)
	return entries, nil
}

// flattenBookmarks recursively walks the bookmark tree and appends entries
// with valid page destinations to the result slice.
func flattenBookmarks(bookmarks []responses.GetBookmarksBookmark, result *[]OutlineEntry) {
	for _, bm := range bookmarks {
		if bm.DestInfo != nil {
			*result = append(*result, OutlineEntry{
				Title:     bm.Title,
				StartPage: bm.DestInfo.PageIndex,
			})
		}
		if len(bm.Children) > 0 {
			flattenBookmarks(bm.Children, result)
		}
	}
}
```

- [ ] **Step 4: Run the outline tests**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-2e21b5e1 && go test ./pkg/pdf/ -run TestExtractOutline -v -count=1`
Expected: Tests pass.

- [ ] **Step 5: Integrate outline extraction into Parse**

In `pkg/pdf/pdf.go`, find where `Parse` builds the return value (the `return &mediafile.ParsedMetadata{...}` block). Add outline extraction just before the return statement.

Add this code before the `return &mediafile.ParsedMetadata{` line:

```go
	// Extract outline (bookmarks) as chapters. Best-effort — don't fail Parse.
	var chapters []mediafile.ParsedChapter
	outlineEntries, outlineErr := ExtractOutline(path)
	if outlineErr == nil {
		for _, entry := range outlineEntries {
			startPage := entry.StartPage
			chapters = append(chapters, mediafile.ParsedChapter{
				Title:     entry.Title,
				StartPage: &startPage,
			})
		}
	}
```

Then add `Chapters: chapters,` to the returned `ParsedMetadata` struct literal, after the existing fields.

- [ ] **Step 6: Run the Parse integration test**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-2e21b5e1 && go test ./pkg/pdf/ -run TestParse_IncludesChaptersFromOutline -v -count=1`
Expected: PASS.

- [ ] **Step 7: Run all PDF tests**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-2e21b5e1 && go test ./pkg/pdf/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 8: Commit**

```bash
git add pkg/pdf/outline.go pkg/pdf/outline_test.go pkg/pdf/pdf.go
git commit -m "[Backend] Extract PDF outline/bookmarks as chapters during parse"
```

---

### Task 5: Extend `getPage` Handler for PDF

**Files:**
- Modify: `pkg/books/handlers.go`
- Modify: `pkg/books/routes.go`

- [ ] **Step 1: Add pdfpages.Cache to handler struct**

In `pkg/books/handlers.go`, find the `handler` struct definition. Add a new field after `pageCache`:

```go
pdfPageCache *pdfpages.Cache
```

Add the import at the top of the file:

```go
"github.com/shishobooks/shisho/pkg/pdfpages"
```

- [ ] **Step 2: Initialize pdfpages.Cache in routes.go**

In `pkg/books/routes.go`, find where `pageCache` is created (`pageCache := cbzpages.NewCache(cfg.CacheDir)`). Add after it:

```go
pdfPageCache := pdfpages.NewCache(cfg.CacheDir, cfg.PDFRenderDPI, cfg.PDFRenderQuality)
```

Then add the import:

```go
"github.com/shishobooks/shisho/pkg/pdfpages"
```

And in the `h := &handler{...}` struct literal, add after `pageCache: pageCache,`:

```go
pdfPageCache: pdfPageCache,
```

- [ ] **Step 3: Extend getPage handler**

In `pkg/books/handlers.go`, find the `getPage` method. Replace the file type check and cache lookup section:

```go
	// Only CBZ files have pages
	if file.FileType != models.FileTypeCBZ {
		return errcodes.ValidationError("Only CBZ files have pages")
	}
```

With:

```go
	// Only CBZ and PDF files have pages
	if file.FileType != models.FileTypeCBZ && file.FileType != models.FileTypePDF {
		return errcodes.ValidationError("Only CBZ and PDF files have pages")
	}
```

Then replace the cache call:

```go
	// Get or extract the page
	cachedPath, mimeType, err := h.pageCache.GetPage(file.Filepath, file.ID, pageNum)
```

With:

```go
	// Get or render the page from the appropriate cache
	var cachedPath, mimeType string
	switch file.FileType {
	case models.FileTypeCBZ:
		cachedPath, mimeType, err = h.pageCache.GetPage(file.Filepath, file.ID, pageNum)
	case models.FileTypePDF:
		cachedPath, mimeType, err = h.pdfPageCache.GetPage(file.Filepath, file.ID, pageNum)
	}
```

- [ ] **Step 4: Run checks**

Run: `make check:quiet`
Expected: All checks pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/books/handlers.go pkg/books/routes.go
git commit -m "[Backend] Extend getPage handler to serve rendered PDF pages"
```

---

### Task 6: Extract Shared PageReader Component

**Files:**
- Create: `app/components/pages/PageReader.tsx`
- Modify: `app/components/pages/CBZReader.tsx`

This is the largest frontend task. We extract all the shared reading UI from CBZReader into PageReader, then make CBZReader a thin wrapper.

- [ ] **Step 1: Create PageReader.tsx**

Create `app/components/pages/PageReader.tsx`:

```tsx
import { ArrowLeft, ChevronLeft, ChevronRight, Settings } from "lucide-react";
import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";

import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Slider } from "@/components/ui/slider";
import { useFileChapters } from "@/hooks/queries/chapters";
import {
  useUpdateViewerSettings,
  useViewerSettings,
} from "@/hooks/queries/settings";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { Chapter } from "@/types";

// Flatten chapters for progress bar (CBZ/PDF chapters don't nest)
const flattenChapters = (chapters: Chapter[]): Chapter[] => {
  const result: Chapter[] = [];
  for (const ch of chapters) {
    if (ch.start_page != null) {
      result.push(ch);
    }
    if (ch.children) {
      result.push(...flattenChapters(ch.children.filter(Boolean) as Chapter[]));
    }
  }
  return result;
};

interface PageReaderProps {
  fileId: number;
  bookId: number;
  libraryId: string;
  totalPages: number;
  getPageUrl: (pageNum: number) => string;
  title?: string;
}

export default function PageReader({
  fileId,
  bookId,
  libraryId,
  totalPages,
  getPageUrl,
  title,
}: PageReaderProps) {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  // Parse page from URL, default to 0
  const urlPage = parseInt(searchParams.get("page") || "0", 10);
  const [currentPage, setCurrentPage] = useState(isNaN(urlPage) ? 0 : urlPage);

  usePageTitle(title ? `Reading: ${title}` : "Reader");

  // Fetch chapters
  const { data: chapters = [] } = useFileChapters(fileId);
  const flatChapters = useMemo(() => flattenChapters(chapters), [chapters]);

  // Fetch and update viewer settings
  const { data: settings, isLoading: settingsLoading } = useViewerSettings();
  const updateSettings = useUpdateViewerSettings();
  const preloadCount = settings?.preload_count ?? 3;
  const fitMode = settings?.fit_mode ?? "fit-height";
  const settingsReady = !settingsLoading && settings != null;

  // Sync URL with current page
  useEffect(() => {
    const urlPage = parseInt(searchParams.get("page") || "0", 10);
    if (urlPage !== currentPage) {
      setSearchParams({ page: currentPage.toString() }, { replace: true });
    }
  }, [currentPage, searchParams, setSearchParams]);

  // Navigate to page
  const goToPage = useCallback(
    (page: number) => {
      if (page < 0) return;
      if (page >= totalPages) {
        // Navigate back to book detail
        navigate(`/libraries/${libraryId}/books/${bookId}`);
        return;
      }
      setCurrentPage(page);
    },
    [totalPages, navigate, libraryId, bookId],
  );

  // Keyboard navigation
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "ArrowRight" || e.key === "d" || e.key === "D") {
        goToPage(currentPage + 1);
      } else if (e.key === "ArrowLeft" || e.key === "a" || e.key === "A") {
        goToPage(currentPage - 1);
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [currentPage, goToPage]);

  // Preload pages
  const preloadedPages = useMemo(() => {
    const pages: number[] = [];
    for (
      let i = Math.max(0, currentPage - preloadCount);
      i <= Math.min(totalPages - 1, currentPage + preloadCount);
      i++
    ) {
      pages.push(i);
    }
    return pages;
  }, [currentPage, preloadCount, totalPages]);

  // Find current chapter
  const currentChapter = useMemo(() => {
    const filtered = flatChapters.filter(
      (ch) => ch.start_page != null && ch.start_page <= currentPage,
    );
    return filtered[filtered.length - 1];
  }, [flatChapters, currentPage]);

  // Progress percentage
  const progressPercent =
    totalPages > 1 ? (currentPage / (totalPages - 1)) * 100 : 0;

  // Handle progress bar click
  const handleProgressClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const percent = x / rect.width;
    const targetPage = Math.round(percent * (totalPages - 1));
    goToPage(Math.max(0, Math.min(targetPage, totalPages - 1)));
  };

  return (
    <div className="fixed inset-0 bg-background flex flex-col">
      {/* Header */}
      <header className="flex items-center justify-between px-4 py-2 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <Link
          className="flex items-center gap-2 text-muted-foreground hover:text-foreground"
          to={`/libraries/${libraryId}/books/${bookId}`}
        >
          <ArrowLeft className="h-4 w-4" />
          <span className="text-sm">Back</span>
        </Link>

        <div className="flex items-center gap-2">
          {/* Chapter dropdown */}
          {flatChapters.length > 0 && (
            <select
              className="text-sm bg-transparent border rounded px-2 py-1"
              onChange={(e) => {
                const ch = flatChapters.find(
                  (c) => c.id === Number(e.target.value),
                );
                if (ch?.start_page != null) {
                  goToPage(ch.start_page);
                }
              }}
              value={currentChapter?.id ?? ""}
            >
              {flatChapters.map((ch) => (
                <option key={ch.id} value={ch.id}>
                  {ch.title}
                </option>
              ))}
            </select>
          )}

          {/* Settings */}
          <Popover>
            <PopoverTrigger asChild>
              <Button size="icon" variant="ghost">
                <Settings className="h-4 w-4" />
              </Button>
            </PopoverTrigger>
            <PopoverContent align="end" className="w-64">
              <div className="space-y-4">
                <div>
                  <label className="text-sm font-medium">
                    Preload Count: {preloadCount}
                  </label>
                  <Slider
                    className="mt-2"
                    disabled={!settingsReady}
                    max={10}
                    min={1}
                    onValueChange={([value]) => {
                      updateSettings.mutate({
                        preload_count: value,
                        fit_mode: fitMode,
                      });
                    }}
                    step={1}
                    value={[preloadCount]}
                  />
                </div>
                <div>
                  <label className="text-sm font-medium">Fit Mode</label>
                  <div className="flex gap-2 mt-2">
                    <Button
                      disabled={!settingsReady}
                      onClick={() =>
                        updateSettings.mutate({
                          preload_count: preloadCount,
                          fit_mode: "fit-height",
                        })
                      }
                      size="sm"
                      variant={fitMode === "fit-height" ? "default" : "outline"}
                    >
                      Fit Height
                    </Button>
                    <Button
                      disabled={!settingsReady}
                      onClick={() =>
                        updateSettings.mutate({
                          preload_count: preloadCount,
                          fit_mode: "original",
                        })
                      }
                      size="sm"
                      variant={fitMode === "original" ? "default" : "outline"}
                    >
                      Original
                    </Button>
                  </div>
                </div>
              </div>
            </PopoverContent>
          </Popover>
        </div>
      </header>

      {/* Page Display */}
      <main
        className={`flex-1 flex items-center justify-center bg-black relative ${
          fitMode === "original" ? "overflow-auto" : "overflow-hidden"
        }`}
      >
        {/* Tap zones for mobile navigation */}
        <button
          aria-label="Previous page"
          className="absolute left-0 top-0 w-1/3 h-full z-10 cursor-pointer opacity-0"
          disabled={currentPage === 0}
          onClick={() => goToPage(currentPage - 1)}
          type="button"
        />
        <button
          aria-label="Next page"
          className="absolute right-0 top-0 w-1/3 h-full z-10 cursor-pointer opacity-0"
          onClick={() => goToPage(currentPage + 1)}
          type="button"
        />

        <img
          alt={`Page ${currentPage + 1}`}
          className={
            fitMode === "fit-height"
              ? "max-h-full w-auto object-contain"
              : "" // original: no constraints, natural size
          }
          src={getPageUrl(currentPage)}
        />
        {/* Preloaded images (hidden) */}
        {preloadedPages
          .filter((p) => p !== currentPage)
          .map((p) => (
            <link as="image" href={getPageUrl(p)} key={p} rel="prefetch" />
          ))}
      </main>

      {/* Controls */}
      <footer className="border-t bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        {/* Progress Bar */}
        <div className="px-4 pt-3">
          <div
            className="relative h-1.5 bg-muted rounded-full cursor-pointer"
            onClick={handleProgressClick}
          >
            <div
              className="absolute inset-y-0 left-0 bg-primary rounded-full"
              style={{ width: `${progressPercent}%` }}
            />
            {/* Chapter markers */}
            {flatChapters.map((ch) => {
              if (ch.start_page == null || totalPages <= 1) return null;
              const pos = (ch.start_page / (totalPages - 1)) * 100;
              return (
                <div
                  className="absolute top-1/2 -translate-y-1/2 w-0.5 h-2.5 bg-muted-foreground/50"
                  key={ch.id}
                  style={{ left: `${pos}%` }}
                  title={ch.title}
                />
              );
            })}
          </div>
          {currentChapter && (
            <div className="text-xs text-muted-foreground mt-1">
              {currentChapter.title}
            </div>
          )}
        </div>

        {/* Navigation buttons */}
        <div className="flex items-center justify-between px-4 py-2">
          <Button
            disabled={currentPage === 0}
            onClick={() => goToPage(currentPage - 1)}
            size="icon"
            variant="ghost"
          >
            <ChevronLeft className="h-5 w-5" />
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {currentPage + 1} of {totalPages}
          </span>
          <Button
            onClick={() => goToPage(currentPage + 1)}
            size="icon"
            variant="ghost"
          >
            <ChevronRight className="h-5 w-5" />
          </Button>
        </div>
      </footer>
    </div>
  );
}
```

- [ ] **Step 2: Refactor CBZReader to thin wrapper**

Replace the entire contents of `app/components/pages/CBZReader.tsx` with:

```tsx
import PageReader from "@/components/pages/PageReader";
import type { File } from "@/types";

interface CBZReaderProps {
  file: File;
  libraryId: string;
  bookTitle?: string;
}

export default function CBZReader({
  file,
  libraryId,
  bookTitle,
}: CBZReaderProps) {
  return (
    <PageReader
      bookId={file.book_id}
      fileId={file.id}
      getPageUrl={(page) => `/api/books/files/${file.id}/page/${page}`}
      libraryId={libraryId}
      title={bookTitle}
      totalPages={file.page_count || 0}
    />
  );
}
```

- [ ] **Step 3: Verify the build passes**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-2e21b5e1 && pnpm lint:types`
Expected: No type errors.

- [ ] **Step 4: Commit**

```bash
git add app/components/pages/PageReader.tsx app/components/pages/CBZReader.tsx
git commit -m "[Frontend] Extract shared PageReader component from CBZReader"
```

---

### Task 7: Add FileReader Dispatcher, PDFReader, and Update Routing

**Files:**
- Create: `app/components/pages/PDFReader.tsx`
- Create: `app/components/pages/FileReader.tsx`
- Modify: `app/router.tsx`
- Modify: `app/components/pages/BookDetail.tsx`

- [ ] **Step 1: Create PDFReader.tsx**

Create `app/components/pages/PDFReader.tsx`:

```tsx
import PageReader from "@/components/pages/PageReader";
import type { File } from "@/types";

interface PDFReaderProps {
  file: File;
  libraryId: string;
  bookTitle?: string;
}

export default function PDFReader({
  file,
  libraryId,
  bookTitle,
}: PDFReaderProps) {
  return (
    <PageReader
      bookId={file.book_id}
      fileId={file.id}
      getPageUrl={(page) => `/api/books/files/${file.id}/page/${page}`}
      libraryId={libraryId}
      title={bookTitle}
      totalPages={file.page_count || 0}
    />
  );
}
```

- [ ] **Step 2: Create FileReader.tsx**

Create `app/components/pages/FileReader.tsx`:

```tsx
import { useParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import CBZReader from "@/components/pages/CBZReader";
import PDFReader from "@/components/pages/PDFReader";
import { useBook } from "@/hooks/queries/books";
import { FileTypeCBZ, FileTypePDF } from "@/types";

export default function FileReader() {
  const { libraryId, bookId, fileId } = useParams<{
    libraryId: string;
    bookId: string;
    fileId: string;
  }>();

  const { data: book, isLoading } = useBook(bookId);
  const file = book?.files?.find((f) => f.id === Number(fileId));

  if (isLoading || !file) {
    return (
      <div className="fixed inset-0 bg-background flex items-center justify-center">
        <LoadingSpinner />
      </div>
    );
  }

  switch (file.file_type) {
    case FileTypeCBZ:
      return (
        <CBZReader
          bookTitle={book?.title}
          file={file}
          libraryId={libraryId!}
        />
      );
    case FileTypePDF:
      return (
        <PDFReader
          bookTitle={book?.title}
          file={file}
          libraryId={libraryId!}
        />
      );
    default:
      return (
        <div className="fixed inset-0 bg-background flex items-center justify-center">
          <p className="text-muted-foreground">
            Reading is not supported for this file type.
          </p>
        </div>
      );
  }
}
```

- [ ] **Step 3: Update router.tsx**

In `app/router.tsx`, replace the CBZReader import:

```tsx
import CBZReader from "@/components/pages/CBZReader";
```

With:

```tsx
import FileReader from "@/components/pages/FileReader";
```

Then find the read route:

```tsx
{
  path: "libraries/:libraryId/books/:bookId/files/:fileId/read",
  element: (
    <ProtectedRoute checkLibraryAccess>
      <CBZReader />
    </ProtectedRoute>
  ),
},
```

Replace `<CBZReader />` with `<FileReader />`:

```tsx
{
  path: "libraries/:libraryId/books/:bookId/files/:fileId/read",
  element: (
    <ProtectedRoute checkLibraryAccess>
      <FileReader />
    </ProtectedRoute>
  ),
},
```

- [ ] **Step 4: Update BookDetail.tsx — first Read button (desktop)**

In `app/components/pages/BookDetail.tsx`, find the first read button (around line 374-375):

```tsx
            {/* Read button - CBZ only */}
            {file.file_type === FileTypeCBZ && (
```

Replace with:

```tsx
            {/* Read button - CBZ and PDF */}
            {(file.file_type === FileTypeCBZ ||
              file.file_type === FileTypePDF) && (
```

Add `FileTypePDF` to the imports. Find the existing import that includes `FileTypeCBZ`:

```tsx
import { ..., FileTypeCBZ, ... } from "@/types";
```

Add `FileTypePDF` to that same import statement.

- [ ] **Step 5: Update BookDetail.tsx — second Read button (mobile)**

Find the second read button (around line 537-538):

```tsx
          {/* Read button - CBZ only */}
          {file.file_type === FileTypeCBZ && (
```

Replace with:

```tsx
          {/* Read button - CBZ and PDF */}
          {(file.file_type === FileTypeCBZ ||
            file.file_type === FileTypePDF) && (
```

- [ ] **Step 6: Verify the build and lint pass**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-2e21b5e1 && pnpm lint`
Expected: No errors.

- [ ] **Step 7: Commit**

```bash
git add app/components/pages/PDFReader.tsx app/components/pages/FileReader.tsx app/router.tsx app/components/pages/BookDetail.tsx
git commit -m "[Frontend] Add PDF reader with FileReader dispatcher and Read button for PDFs"
```

---

### Task 8: Documentation Updates

**Files:**
- Modify: `website/docs/configuration.md`
- Modify: `website/docs/supported-formats.md`

- [ ] **Step 1: Update configuration.md — Cache section**

In `website/docs/configuration.md`, find the Cache section table (around line 69-74). Replace the table with:

```markdown
### Cache

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `cache_dir` | `CACHE_DIR` | `/config/cache` | Directory for caching generated files (downloads, extracted CBZ pages, and rendered PDF pages) |
| `download_cache_max_size_gb` | `DOWNLOAD_CACHE_MAX_SIZE_GB` | `5` | Maximum size of the download cache in GB. Older files are removed automatically (LRU) when the limit is exceeded |
| `pdf_render_dpi` | `PDF_RENDER_DPI` | `200` | DPI for rendering PDF pages in the viewer. Higher values produce sharper images but use more disk space. Range: 72–600 |
| `pdf_render_quality` | `PDF_RENDER_QUALITY` | `85` | JPEG quality for rendered PDF pages. Higher values produce better quality but larger files. Range: 1–100 |
```

- [ ] **Step 2: Update supported-formats.md — PDF entry**

In `website/docs/supported-formats.md`, find the PDF line:

```markdown
- **PDF** — Full [metadata extraction](./metadata#pdf) including title, authors, description, cover art, and page count
```

Replace with:

```markdown
- **PDF** — Full [metadata extraction](./metadata#pdf) including title, authors, description, cover art, page count, and chapter extraction from PDF bookmarks. Includes an in-app viewer with page-by-page reading
```

- [ ] **Step 3: Verify docs build**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-2e21b5e1/website && pnpm build`
Expected: Build succeeds with no errors.

- [ ] **Step 4: Commit**

```bash
git add website/docs/configuration.md website/docs/supported-formats.md
git commit -m "[Docs] Document PDF viewer config options and update supported formats"
```

---

### Task 9: Final Verification

- [ ] **Step 1: Run full check suite**

Run: `make check:quiet`
Expected: All checks pass (Go tests, Go lint, JS lint, JS types).

- [ ] **Step 2: Manual verification checklist**

With `make start` running and a PDF file in a library:

1. Navigate to a book with a PDF file
2. Verify the "Read" button (BookOpen icon) appears on the PDF file row
3. Click it — should open the PDF viewer
4. Verify pages render as images
5. Test keyboard navigation (Arrow keys, A/D)
6. Test tap zones (click left/right thirds)
7. Test progress bar click-to-seek
8. Test settings (preload count, fit mode)
9. If the PDF has bookmarks, verify chapter dropdown appears
10. Verify "Back" link returns to book detail
11. Verify CBZ reading still works identically

- [ ] **Step 3: Update pkg/pdf/CLAUDE.md**

Add a section about outline extraction to `pkg/pdf/CLAUDE.md`:

```markdown
## Outline Extraction

PDF bookmarks (outlines) are extracted during `Parse()` and returned as `ParsedChapter` entries with `StartPage` values. This uses go-pdfium's `GetBookmarks` API.

### Key Types

- `OutlineEntry` — lightweight struct with `Title` and `StartPage` (0-indexed), defined in `pkg/pdf/outline.go`
- `ExtractOutline(path)` — standalone function, also called from `Parse()` as best-effort

### Integration

Outline extraction is integrated into `Parse()` so chapters flow through the standard scanner pipeline without any scanner changes. PDFs without bookmarks produce no chapters.

### Related Files

- `pkg/pdf/outline.go` — ExtractOutline implementation
- `pkg/pdf/outline_test.go` — Tests with raw PDF fixtures containing bookmarks
```

- [ ] **Step 4: Commit documentation update**

```bash
git add pkg/pdf/CLAUDE.md
git commit -m "[Docs] Update PDF CLAUDE.md with outline extraction documentation"
```
