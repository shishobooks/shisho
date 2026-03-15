# First-Class PDF Support Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add PDF as a fourth built-in file type with full metadata extraction, cover generation, and metadata embedding on download.

**Architecture:** New `pkg/pdf/` package uses pdfcpu for metadata/image extraction and go-pdfium WASM for cover rendering fallback. A new `PDFGenerator` in `pkg/filegen/` writes metadata back via pdfcpu. Integration points across scanner, OPDS, eReader, and cover selection are updated to include PDF.

**Tech Stack:** pdfcpu (Apache-2.0, pure Go), go-pdfium (MIT, WASM via Wazero, pure Go)

**Spec:** `docs/superpowers/specs/2026-03-10-pdf-support-design.md`

---

## Chunk 1: Foundation — Constants, Dependencies, and Parser

### Task 1: Add PDF constants to models

**Files:**
- Modify: `pkg/models/file.go:9-14`
- Modify: `pkg/models/data-source.go:5-41`

- [ ] **Step 1: Add FileTypePDF constant**

In `pkg/models/file.go`, add `FileTypePDF` to the const block and update the tygo:emit comment:

```go
const (
	//tygo:emit export type FileType = typeof FileTypeCBZ | typeof FileTypeEPUB | typeof FileTypeM4B | typeof FileTypePDF;
	FileTypeCBZ  = "cbz"
	FileTypeEPUB = "epub"
	FileTypeM4B  = "m4b"
	FileTypePDF  = "pdf"
)
```

- [ ] **Step 2: Add DataSourcePDFMetadata constant**

In `pkg/models/data-source.go`, add the constant, update tygo:emit, and add to priority map:

```go
const (
	//tygo:emit export type DataSource = typeof DataSourceManual | typeof DataSourceSidecar | typeof DataSourcePlugin | typeof DataSourceFileMetadata | typeof DataSourceExistingCover | typeof DataSourceEPUBMetadata | typeof DataSourceCBZMetadata | typeof DataSourceM4BMetadata | typeof DataSourcePDFMetadata | typeof DataSourceFilepath | `plugin:${string}`;
	DataSourceManual        = "manual"
	DataSourceSidecar       = "sidecar"
	DataSourcePlugin        = "plugin"
	DataSourceFileMetadata  = "file_metadata"
	DataSourceExistingCover = "existing_cover"
	DataSourceEPUBMetadata  = "epub_metadata"
	DataSourceCBZMetadata   = "cbz_metadata"
	DataSourceM4BMetadata   = "m4b_metadata"
	DataSourcePDFMetadata   = "pdf_metadata"
	DataSourceFilepath      = "filepath"

	// ...rest unchanged
)

// Add to dataSourcePriority map:
var dataSourcePriority = map[string]int{
	// ...existing entries...
	DataSourcePDFMetadata:   DataSourceFileMetadataPriority,
	// ...rest unchanged
}
```

- [ ] **Step 3: Run `make tygo` and verify**

Run: `make tygo`
Expected: Types generated successfully (or "Nothing to be done for `tygo`")

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: Compiles without errors

- [ ] **Step 5: Commit**

```bash
git add pkg/models/file.go pkg/models/data-source.go
git commit -m "[Backend] Add PDF file type and data source constants"
```

### Task 2: Add Go dependencies

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add pdfcpu dependency**

Run: `go get github.com/pdfcpu/pdfcpu/pkg/api github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model`

- [ ] **Step 2: Add go-pdfium WASM dependency**

Run: `go get github.com/klippa-app/go-pdfium github.com/klippa-app/go-pdfium/webassembly`

- [ ] **Step 3: Tidy**

Run: `go mod tidy`

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: Compiles without errors

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "[Backend] Add pdfcpu and go-pdfium dependencies"
```

### Task 3: Create PDF parser with tests

**Files:**
- Create: `pkg/pdf/pdf.go`
- Create: `pkg/pdf/pdf_test.go`
- Create: `pkg/pdf/testdata/` (test fixtures)

This is the core parser. Follow the pattern of `pkg/epub/opf.go`, `pkg/cbz/cbz.go`, `pkg/mp4/mp4.go` — all expose a single `Parse(path string) (*mediafile.ParsedMetadata, error)` function.

- [ ] **Step 1: Create test fixtures**

Create small PDF test files in `pkg/pdf/testdata/`:
- `with-metadata.pdf` — has Title, Author, Subject, Keywords, CreationDate set in info dict
- `no-metadata.pdf` — valid PDF with empty/missing info dict
- `with-cover-image.pdf` — PDF with an embedded image on page 1
- `text-only.pdf` — PDF with only text on page 1 (no embedded images)

Use pdfcpu CLI or a Go test helper to generate these programmatically in a `TestMain` or setup function.

- [ ] **Step 2: Write failing test for metadata extraction**

```go
package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_WithMetadata(t *testing.T) {
	t.Parallel()

	metadata, err := Parse("testdata/with-metadata.pdf")
	require.NoError(t, err)

	assert.Equal(t, "Test PDF Title", metadata.Title)
	assert.Len(t, metadata.Authors, 1)
	assert.Equal(t, "Test Author", metadata.Authors[0].Name)
	assert.Equal(t, "", metadata.Authors[0].Role)
	assert.Equal(t, "A test PDF description", metadata.Description)
	assert.NotNil(t, metadata.ReleaseDate)
	assert.NotNil(t, metadata.PageCount)
	assert.Equal(t, "pdf_metadata", metadata.DataSource)
}

func TestParse_NoMetadata(t *testing.T) {
	t.Parallel()

	metadata, err := Parse("testdata/no-metadata.pdf")
	require.NoError(t, err)

	assert.Equal(t, "", metadata.Title)
	assert.Empty(t, metadata.Authors)
	assert.NotNil(t, metadata.PageCount)
}

func TestParse_MultipleAuthors(t *testing.T) {
	t.Parallel()

	// Test PDF with Author field "Author One, Author Two"
	metadata, err := Parse("testdata/multiple-authors.pdf")
	require.NoError(t, err)

	assert.Len(t, metadata.Authors, 2)
	assert.Equal(t, "Author One", metadata.Authors[0].Name)
	assert.Equal(t, "Author Two", metadata.Authors[1].Name)
}

func TestParse_Keywords(t *testing.T) {
	t.Parallel()

	// Test PDF with Keywords field "fiction, sci-fi, adventure"
	metadata, err := Parse("testdata/with-metadata.pdf")
	require.NoError(t, err)

	assert.Contains(t, metadata.Tags, "fiction")
}

func TestParse_InvalidPDF(t *testing.T) {
	t.Parallel()

	_, err := Parse("testdata/invalid.pdf")
	assert.Error(t, err)
}
```

Add an `invalid.pdf` test fixture — just a file with garbage bytes (e.g., `[]byte("not a pdf")`).

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./pkg/pdf/ -v -run TestParse`
Expected: FAIL — `Parse` function not defined

- [ ] **Step 4: Implement PDF parser**

Create `pkg/pdf/pdf.go`:

```go
package pdf

import (
	"regexp"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

var authorSplitRE = regexp.MustCompile(`\s*[,;&]\s*`)

// Parse extracts metadata from a PDF file.
func Parse(path string) (*mediafile.ParsedMetadata, error) {
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed

	ctx, err := api.ReadContextFile(path, conf)
	if err != nil {
		return nil, err
	}

	metadata := &mediafile.ParsedMetadata{
		DataSource: models.DataSourcePDFMetadata,
	}

	// Page count
	pageCount := ctx.PageCount
	metadata.PageCount = &pageCount

	// Info dict metadata
	if ctx.XRefTable != nil && ctx.XRefTable.Info != nil {
		info := ctx.XRefTable.Info
		// These fields need to be extracted from the info dict
		// pdfcpu provides them via the Properties/Info dict
	}

	// Extract info dict fields
	// (Implementation depends on pdfcpu's API for reading info dict —
	//  use api.InfoDict or ctx.XRefTable properties)

	// Title
	metadata.Title = extractInfoField(ctx, "Title")

	// Author(s)
	authorStr := extractInfoField(ctx, "Author")
	if authorStr != "" {
		parts := authorSplitRE.Split(authorStr, -1)
		for _, name := range parts {
			name = strings.TrimSpace(name)
			if name != "" {
				metadata.Authors = append(metadata.Authors, mediafile.ParsedAuthor{Name: name})
			}
		}
	}

	// Subject -> Description
	metadata.Description = extractInfoField(ctx, "Subject")

	// Keywords -> Tags
	keywordsStr := extractInfoField(ctx, "Keywords")
	if keywordsStr != "" {
		for _, kw := range regexp.MustCompile(`\s*[,;]\s*`).Split(keywordsStr, -1) {
			kw = strings.TrimSpace(kw)
			if kw != "" {
				metadata.Tags = append(metadata.Tags, kw)
			}
		}
	}

	// CreationDate -> ReleaseDate
	dateStr := extractInfoField(ctx, "CreationDate")
	if dateStr != "" {
		if t, err := parsePDFDate(dateStr); err == nil {
			metadata.ReleaseDate = &t
		}
	}

	// Cover extraction (two-tier)
	coverData, coverMime, err := extractCover(path, conf)
	if err == nil && len(coverData) > 0 {
		metadata.CoverData = coverData
		metadata.CoverMimeType = coverMime
	}

	return metadata, nil
}
```

**Important:** Before implementing, explore the pdfcpu API surface. The key function is likely `api.InfoDict(path, conf)` which returns `(map[string]string, error)`. Verify this by reading the pdfcpu source or running a quick spike. If the API differs, adapt the implementation accordingly. Do not spend time guessing — read the library code.

- [ ] **Step 5: Implement `extractInfoField` helper**

This reads a single field from the PDF info dictionary. Use `api.InfoDict()`:

```go
func extractInfoField(ctx *model.Context, field string) string {
	// Use pdfcpu's info dict API to read fields
	// Implementation depends on exact pdfcpu API surface
}
```

- [ ] **Step 6: Implement `parsePDFDate` helper**

PDF dates use the format `D:YYYYMMDDHHmmSSOHH'mm'`:

```go
func parsePDFDate(s string) (time.Time, error) {
	s = strings.TrimPrefix(s, "D:")
	// Try multiple layouts in order
	layouts := []string{
		"20060102150405-07'00'",
		"20060102150405Z",
		"20060102150405",
		"20060102",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse PDF date: %s", s)
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./pkg/pdf/ -v -run TestParse`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add pkg/pdf/
git commit -m "[Backend] Add PDF parser with metadata extraction"
```

### Task 4: Add cover extraction with tests

**Files:**
- Modify: `pkg/pdf/pdf.go`
- Create: `pkg/pdf/cover.go`
- Modify: `pkg/pdf/pdf_test.go`

- [ ] **Step 1: Write failing tests for cover extraction**

```go
func TestParse_CoverFromEmbeddedImage(t *testing.T) {
	t.Parallel()

	metadata, err := Parse("testdata/with-cover-image.pdf")
	require.NoError(t, err)

	assert.NotEmpty(t, metadata.CoverData)
	assert.Contains(t, []string{"image/jpeg", "image/png"}, metadata.CoverMimeType)
}

func TestParse_CoverFromRenderedPage(t *testing.T) {
	t.Parallel()

	metadata, err := Parse("testdata/text-only.pdf")
	require.NoError(t, err)

	// Fallback to go-pdfium rendering
	assert.NotEmpty(t, metadata.CoverData)
	assert.Equal(t, "image/jpeg", metadata.CoverMimeType)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/pdf/ -v -run TestParse_Cover`
Expected: FAIL

- [ ] **Step 3: Implement two-tier cover extraction**

Create `pkg/pdf/cover.go`:

```go
package pdf

import (
	"bytes"
	"image/jpeg"
	"sync"

	pdfcpuAPI "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
)

var (
	pdfiumPool     pdfium.Pool
	pdfiumInitOnce sync.Once
	pdfiumInitErr  error
)

// initPdfium lazily initializes the go-pdfium WASM runtime.
func initPdfium() (pdfium.Pool, error) {
	pdfiumInitOnce.Do(func() {
		pdfiumPool, pdfiumInitErr = webassembly.Init(webassembly.Config{
			MinIdle:  1,
			MaxIdle:  1,
			MaxTotal: 1,
		})
	})
	return pdfiumPool, pdfiumInitErr
}

// extractCover attempts to extract a cover image from a PDF.
// Tier 1: Extract embedded images from page 1 via pdfcpu.
// Tier 2: Render page 1 to JPEG via go-pdfium WASM.
func extractCover(path string, conf *model.Configuration) ([]byte, string, error) {
	// Tier 1: Try extracting embedded images from page 1
	data, mime, err := extractEmbeddedCover(path, conf)
	if err == nil && len(data) > 0 {
		return data, mime, nil
	}

	// Tier 2: Render page 1 via go-pdfium
	data, mime, err = renderCoverPage(path)
	if err != nil {
		return nil, "", err
	}
	return data, mime, nil
}

// extractEmbeddedCover extracts the largest embedded image from page 1.
func extractEmbeddedCover(path string, conf *model.Configuration) ([]byte, string, error) {
	// Use pdfcpu to extract images from page 1
	// pdfcpu's api.ExtractImagesRaw or similar
	// Pick the largest image by byte size or pixel dimensions
	// Return (imageData, mimeType, error)
	return nil, "", nil // placeholder
}

// renderCoverPage renders page 1 to a JPEG using go-pdfium WASM.
func renderCoverPage(path string) ([]byte, string, error) {
	pool, err := initPdfium()
	if err != nil {
		return nil, "", err
	}

	instance, err := pool.GetInstance(nil)
	if err != nil {
		return nil, "", err
	}
	defer instance.Close()

	// Open the PDF
	doc, err := instance.OpenDocument(&requests.OpenDocument{
		FilePath: &path,
	})
	if err != nil {
		return nil, "", err
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{
		Document: doc.Document,
	})

	// Render page 0 (first page) at ~150 DPI
	render, err := instance.RenderPageInDPI(&requests.RenderPageInDPI{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: doc.Document,
				Index:    0,
			},
		},
		DPI: 150,
	})
	if err != nil {
		return nil, "", err
	}

	// Encode to JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, render.Result.Image, &jpeg.Options{Quality: 85}); err != nil {
		return nil, "", err
	}

	return buf.Bytes(), "image/jpeg", nil
}
```

**Note:** The exact pdfcpu API for image extraction and go-pdfium API for rendering need to be confirmed against current library versions during implementation. The above is the intended structure — adapt method signatures to match the actual APIs.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/pdf/ -v -run TestParse_Cover`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/pdf/
git commit -m "[Backend] Add PDF cover extraction with pdfcpu and go-pdfium fallback"
```

## Chunk 2: Generator and Scanner Integration

### Task 5: Create PDF file generator with tests

**Files:**
- Create: `pkg/filegen/pdf.go`
- Create: `pkg/filegen/pdf_test.go`
- Modify: `pkg/filegen/generator.go:50-87`

- [ ] **Step 1: Write failing test for PDF generation**

```go
package filegen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdf"
)

func TestPDFGenerator_SupportedType(t *testing.T) {
	t.Parallel()
	g := &PDFGenerator{}
	assert.Equal(t, models.FileTypePDF, g.SupportedType())
}

func TestPDFGenerator_Generate(t *testing.T) {
	t.Parallel()

	// Use a test PDF fixture
	srcPath := "../pdf/testdata/with-metadata.pdf"
	destPath := filepath.Join(t.TempDir(), "output.pdf")

	book := &models.Book{
		Title: "Updated Title",
	}
	// Add authors to book.Authors if the model supports it
	file := &models.File{
		FileType: models.FileTypePDF,
	}

	g := &PDFGenerator{}
	err := g.Generate(t.Context(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// Verify dest file exists and is different from src
	destInfo, err := os.Stat(destPath)
	require.NoError(t, err)
	assert.Greater(t, destInfo.Size(), int64(0))

	// Verify original file is unchanged
	srcInfo, err := os.Stat(srcPath)
	require.NoError(t, err)
	assert.Greater(t, srcInfo.Size(), int64(0))

	// Re-parse the generated PDF and verify metadata was written
	metadata, err := pdf.Parse(destPath)
	require.NoError(t, err)
	assert.Equal(t, "Updated Title", metadata.Title)
}

func TestPDFGenerator_PreservesProducerCreator(t *testing.T) {
	t.Parallel()

	srcPath := "../pdf/testdata/with-metadata.pdf"
	destPath := filepath.Join(t.TempDir(), "output.pdf")

	book := &models.Book{Title: "New Title"}
	file := &models.File{FileType: models.FileTypePDF}

	g := &PDFGenerator{}
	err := g.Generate(t.Context(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// Read info dict from generated file and verify Producer/Creator are preserved
	// (Use pdfcpu API to read info dict fields directly)
	// The implementation should only set Title/Author/Subject/Keywords/CreationDate
	// and leave Producer/Creator untouched from the original
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/filegen/ -v -run TestPDFGenerator`
Expected: FAIL — `PDFGenerator` not defined

- [ ] **Step 3: Implement PDFGenerator**

Create `pkg/filegen/pdf.go`:

```go
package filegen

import (
	"context"
	"io"
	"os"
	"strings"

	pdfcpuAPI "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/shishobooks/shisho/pkg/models"
)

// PDFGenerator generates PDF files with modified metadata.
type PDFGenerator struct{}

// SupportedType returns the file type this generator handles.
func (g *PDFGenerator) SupportedType() string {
	return models.FileTypePDF
}

// Generate creates a modified PDF at destPath with updated metadata from the book/file models.
func (g *PDFGenerator) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error {
	if err := ctx.Err(); err != nil {
		return NewGenerationError(models.FileTypePDF, err, "context cancelled")
	}

	// Copy source to destination
	if err := copyFile(srcPath, destPath); err != nil {
		return NewGenerationError(models.FileTypePDF, err, "failed to copy source file")
	}

	if err := ctx.Err(); err != nil {
		return NewGenerationError(models.FileTypePDF, err, "context cancelled")
	}

	// Build properties map for info dict update
	properties := buildPDFProperties(book, file)

	// Update info dict in the destination file
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	if err := pdfcpuAPI.SetInfoDict(destPath, properties, conf); err != nil {
		return NewGenerationError(models.FileTypePDF, err, "failed to update PDF metadata")
	}

	return nil
}

// buildPDFProperties creates a properties map for pdfcpu info dict update.
func buildPDFProperties(book *models.Book, file *models.File) map[string]string {
	props := make(map[string]string)

	if book.Title != "" {
		props["Title"] = book.Title
	}

	// Collect author names
	if len(book.Authors) > 0 {
		names := make([]string, 0, len(book.Authors))
		for _, a := range book.Authors {
			if a.Person != nil {
				names = append(names, a.Person.Name)
			}
		}
		if len(names) > 0 {
			props["Author"] = strings.Join(names, ", ")
		}
	}

	if book.Description != nil && *book.Description != "" {
		props["Subject"] = *book.Description
	}

	// Tags -> Keywords
	if len(book.Tags) > 0 {
		tagNames := make([]string, 0, len(book.Tags))
		for _, bt := range book.Tags {
			if bt.Tag != nil {
				tagNames = append(tagNames, bt.Tag.Name)
			}
		}
		if len(tagNames) > 0 {
			props["Keywords"] = strings.Join(tagNames, ", ")
		}
	}

	// ReleaseDate -> CreationDate
	if file.ReleaseDate != nil {
		props["CreationDate"] = file.ReleaseDate.Format("D:20060102150405Z")
	}

	return props
}

// copyFile copies src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
```

**Note:** The exact pdfcpu API for setting info dict fields (`api.SetInfoDict` or similar) needs to be confirmed during implementation. Check pdfcpu's docs for the correct function name and signature.

- [ ] **Step 4: Register PDFGenerator in GetGenerator**

In `pkg/filegen/generator.go`, add PDF cases:

```go
// In GetGenerator (line ~52):
case models.FileTypePDF:
    return &PDFGenerator{}, nil

// In GetKepubGenerator (line ~67):
case models.FileTypePDF:
    return nil, ErrKepubNotSupported
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./pkg/filegen/ -v -run TestPDFGenerator`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/filegen/pdf.go pkg/filegen/pdf_test.go pkg/filegen/generator.go
git commit -m "[Backend] Add PDF file generator with metadata embedding"
```

### Task 6: Update existing supplement tests that use PDF

**Files:**
- Modify: `pkg/worker/supplement_test.go`
- Modify: `pkg/worker/scan_unified_test.go`

Once `.pdf` is added to `extensionsToScan`, existing tests that create `.pdf` files as supplements will break — those PDFs will now be discovered as main files. Update these tests to use `.txt` instead of `.pdf` for supplement file extensions.

- [ ] **Step 1: Update supplement_test.go**

In `pkg/worker/supplement_test.go`, find all test cases that create PDF supplement files (e.g., `companion.pdf`, `guide.pdf`, `appendix.pdf`, `bonus.pdf`, `My Book.pdf`) and change the file extension from `.pdf` to `.txt`. Also update any assertions that check for `FileType == "pdf"` to check for `"txt"` instead.

Tests to update:
- `TestProcessScanJob_SupplementsInDirectory`
- `TestProcessScanJob_SupplementsExcludeHiddenFiles`
- `TestProcessScanJob_SupplementsExcludeShishoFiles`
- `TestProcessScanJob_SupplementsInSubdirectory`
- `TestProcessScanJob_RootLevelSupplements`

- [ ] **Step 2: Update scan_unified_test.go**

In `pkg/worker/scan_unified_test.go`, find tests that create PDF files as supplements in the DB (search for `FileType: "pdf"` with `FileRole: models.FileRoleSupplement`). These tests manually set the file role in the DB so they may still work, but review each one — if the test also puts a `.pdf` file on disk and runs a scan, the scanner will now also discover it as a main file. Update the file extension to `.txt` if needed.

Key locations to check: lines ~377-380, ~3463, ~3534, ~3597, ~3626.

- [ ] **Step 3: Run updated tests**

Run: `go test ./pkg/worker/ -v -count=1 -run "Supplement|supplement"`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add pkg/worker/supplement_test.go pkg/worker/scan_unified_test.go
git commit -m "[Test] Update supplement tests to use .txt instead of .pdf"
```

### Task 7: Integrate PDF into scanner

**Files:**
- Modify: `pkg/worker/scan.go:27-31`
- Modify: `pkg/worker/scan_unified.go:2525-2531` (parseFileMetadata)
- Modify: `pkg/worker/scan_unified.go:3171-3179` (recoverMissingCover)
- Modify: `pkg/mediafile/mediafile.go:64` (PageCount comment)

- [ ] **Step 1: Add PDF to extensionsToScan**

In `pkg/worker/scan.go` line 27-31, add the `.pdf` entry:

```go
var extensionsToScan = map[string]map[string]struct{}{
	".epub": {"application/epub+zip": {}},
	".m4b":  {"audio/x-m4a": {}, "video/mp4": {}},
	".cbz":  {"application/zip": {}},
	".pdf":  {"application/pdf": {}},
}
```

This also automatically makes `isMainFileExtension` (line 112) return `true` for `.pdf`, which means `discoverSupplements` (line 167) will skip PDFs — they are now main files, not supplements.

- [ ] **Step 2: Add PDF case to parseFileMetadata**

In `pkg/worker/scan_unified.go`, add to the switch at line 2525:

```go
case models.FileTypePDF:
    metadata, err = pdf.Parse(path)
```

Add the import: `"github.com/shishobooks/shisho/pkg/pdf"`

- [ ] **Step 3: Add PDF case to recoverMissingCover**

In `pkg/worker/scan_unified.go`, add to the switch at line 3171:

```go
case models.FileTypePDF:
    metadata, parseErr = pdf.Parse(file.Filepath)
```

- [ ] **Step 4: Update PageCount comment**

In `pkg/mediafile/mediafile.go` line 64, change:

```go
// PageCount is the number of pages (CBZ files only)
```

to:

```go
// PageCount is the number of pages (CBZ and PDF files)
```

- [ ] **Step 5: Verify compilation**

Run: `go build ./...`
Expected: Compiles without errors

- [ ] **Step 6: Run all scanner tests**

Run: `go test ./pkg/worker/ -v -count=1`
Expected: All tests pass

- [ ] **Step 7: Commit**

```bash
git add pkg/worker/scan.go pkg/worker/scan_unified.go pkg/mediafile/mediafile.go
git commit -m "[Backend] Integrate PDF parser into scanner and cover recovery"
```

## Chunk 3: Integration Points — OPDS, Covers, eReader

### Task 8: Add PDF to OPDS

**Files:**
- Modify: `pkg/opds/feed.go:33-45` (MIME constants)
- Modify: `pkg/opds/feed.go:207-219` (FileTypeMimeType)
- Modify: `pkg/opds/handlers.go:83-87` (validateFileTypes)

- [ ] **Step 1: Add MimeTypePDF constant**

In `pkg/opds/feed.go` line 41, add:

```go
MimeTypeM4B         = "audio/mp4"
MimeTypePDF         = "application/pdf"
```

- [ ] **Step 2: Add PDF case to FileTypeMimeType**

In `pkg/opds/feed.go` line 209, add:

```go
case "pdf":
    return MimeTypePDF
```

- [ ] **Step 3: Add PDF to validateFileTypes**

In `pkg/opds/handlers.go` line 83-87, add:

```go
validTypes := map[string]bool{
    models.FileTypeEPUB: true,
    models.FileTypeCBZ:  true,
    models.FileTypeM4B:  true,
    models.FileTypePDF:  true,
}
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./pkg/opds/...`
Expected: Compiles without errors

- [ ] **Step 5: Commit**

```bash
git add pkg/opds/feed.go pkg/opds/handlers.go
git commit -m "[Backend] Add PDF support to OPDS feeds"
```

### Task 9: Add PDF to cover aspect ratio selection

**Files:**
- Modify: `pkg/books/handlers.go:1435-1440`
- Modify: `pkg/opds/service.go:793-798`
- Modify: `pkg/ereader/handlers.go:909-914`

All three `selectCoverFile` functions have identical switch statements. PDF should be categorized as a "book" file (same aspect ratio as EPUB/CBZ).

- [ ] **Step 1: Update books/handlers.go selectCoverFile**

In `pkg/books/handlers.go` line 1436, change:

```go
case models.FileTypeEPUB, models.FileTypeCBZ:
```

to:

```go
case models.FileTypeEPUB, models.FileTypeCBZ, models.FileTypePDF:
```

- [ ] **Step 2: Update opds/service.go selectCoverFile**

In `pkg/opds/service.go` line 794, change:

```go
case models.FileTypeEPUB, models.FileTypeCBZ:
```

to:

```go
case models.FileTypeEPUB, models.FileTypeCBZ, models.FileTypePDF:
```

- [ ] **Step 3: Update ereader/handlers.go selectCoverFile**

In `pkg/ereader/handlers.go` line 910, change:

```go
case models.FileTypeEPUB, models.FileTypeCBZ:
```

to:

```go
case models.FileTypeEPUB, models.FileTypeCBZ, models.FileTypePDF:
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: Compiles without errors

- [ ] **Step 5: Commit**

```bash
git add pkg/books/handlers.go pkg/opds/service.go pkg/ereader/handlers.go
git commit -m "[Backend] Add PDF to cover aspect ratio selection"
```

## Chunk 4: Documentation and Final Verification

### Task 10: Update documentation

**Files:**
- Modify: `website/docs/supported-formats.md`
- Modify: `website/docs/metadata.md`
- Create: `pkg/pdf/CLAUDE.md`
- Modify: `pkg/CLAUDE.md`

- [ ] **Step 1: Update supported-formats.md**

Replace the PDF line in `website/docs/supported-formats.md` line 12:

```md
- **PDF** — Full [metadata extraction](./metadata#pdf) including title, authors, description, keywords, cover art, and page count
```

- [ ] **Step 2: Add PDF section to metadata.md**

Add a `## PDF` section (following the pattern of existing format sections) documenting:
- Metadata fields extracted from info dict: Title, Author, Subject, Keywords, CreationDate
- How Author is split into multiple authors
- How Keywords map to tags
- How Subject maps to description
- Cover extraction strategy (embedded image → page render fallback)
- What metadata is written back on download

- [ ] **Step 3: Create pkg/pdf/CLAUDE.md**

Document:
- Package purpose and public API
- pdfcpu usage for info dict and image extraction
- go-pdfium WASM usage for cover rendering (lazy init, pool config)
- PDF date format parsing
- Author splitting convention
- Data source constant
- Test fixtures and how to create them
- Known limitations (no chapters, no XMP, no identifiers)

- [ ] **Step 4: Update pkg/CLAUDE.md**

In the "File Types" section, add:
```
  - PDF: `pkg/pdf/CLAUDE.md`
```

- [ ] **Step 5: Commit**

```bash
git add website/docs/supported-formats.md website/docs/metadata.md pkg/pdf/CLAUDE.md pkg/CLAUDE.md
git commit -m "[Docs] Add PDF format documentation"
```

### Task 11: Run full test suite and verify

- [ ] **Step 1: Generate TypeScript types**

Run: `make tygo`
Expected: Success (generates updated FileType and DataSource unions)

- [ ] **Step 2: Run full check**

Run: `make check`
Expected: All tests pass, no lint errors

If there are lint issues (e.g., unused imports, formatting), fix them and re-run.

- [ ] **Step 3: Verify with `make check && echo "SUCCESS"`**

Run: `make check && echo "SUCCESS"`
Expected: "SUCCESS" printed at the end

- [ ] **Step 4: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "[Fix] Address lint and test issues from PDF integration"
```
