# Enricher Cover Resolution Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make enricher covers actually get saved to disk, gated by a resolution check (only apply if higher resolution than current cover), with page-based formats (CBZ/PDF) always blocking external covers.

**Architecture:** Post-enrichment cover upgrade approach — keep existing flow, add an `upgradeEnricherCover` step after enrichers run in both scan paths. Guards change from `CoverPage != nil` to file type checks via a centralized helper. Resolution comparison uses `image.DecodeConfig` (header-only, no full decode).

**Tech Stack:** Go (backend), React/TypeScript (frontend), SQLite (database)

---

### Task 1: Add `IsPageBasedFileType` Helper (Backend)

**Files:**
- Modify: `pkg/models/file.go`

- [ ] **Step 1: Add the helper function**

Add after the `CoverExtension()` method at the end of `pkg/models/file.go`:

```go
// IsPageBasedFileType returns true for file types that derive covers from page
// content (CBZ, PDF). These formats should never have their covers replaced by
// external sources (plugins, uploads).
func IsPageBasedFileType(fileType string) bool {
	return fileType == FileTypeCBZ || fileType == FileTypePDF
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/robinjoseph/.worktrees/shisho/auto-cover && go build ./pkg/models/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add pkg/models/file.go
git commit -m "[Backend] Add IsPageBasedFileType helper for cover protection guards"
```

---

### Task 2: Add `isPageBasedFileType` Helper (Frontend)

**Files:**
- Modify: `app/libraries/utils.ts`

- [ ] **Step 1: Add the frontend helper**

Add to the end of `app/libraries/utils.ts`:

```ts
/**
 * Returns true for file types that derive covers from page content (CBZ, PDF).
 * These formats should never have their covers replaced by external sources.
 */
export const isPageBasedFileType = (fileType: string | undefined): boolean =>
  fileType === "cbz" || fileType === "pdf";
```

Note: The parameter accepts `string | undefined` because `file?.file_type` can be undefined when the file object is optional.

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/robinjoseph/.worktrees/shisho/auto-cover && pnpm lint:types`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add app/libraries/utils.ts
git commit -m "[Frontend] Add isPageBasedFileType helper for cover protection guards"
```

---

### Task 3: Add Image Resolution Helpers

**Files:**
- Modify: `pkg/fileutils/operations.go`
- Create: `pkg/fileutils/resolution_test.go`

- [ ] **Step 1: Write the failing tests**

Create `pkg/fileutils/resolution_test.go`:

```go
package fileutils

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createTestJPEG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	return buf.Bytes()
}

func createTestPNG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func TestImageResolution(t *testing.T) {
	t.Parallel()

	t.Run("returns width * height for JPEG", func(t *testing.T) {
		t.Parallel()
		data := createTestJPEG(800, 1200)
		assert.Equal(t, 800*1200, ImageResolution(data))
	})

	t.Run("returns width * height for PNG", func(t *testing.T) {
		t.Parallel()
		data := createTestPNG(640, 480)
		assert.Equal(t, 640*480, ImageResolution(data))
	})

	t.Run("returns 0 for invalid data", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 0, ImageResolution([]byte("not an image")))
	})

	t.Run("returns 0 for empty data", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 0, ImageResolution(nil))
	})
}

func TestImageFileResolution(t *testing.T) {
	t.Parallel()

	t.Run("returns resolution for JPEG file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.jpg")
		os.WriteFile(path, createTestJPEG(1024, 768), 0644)
		assert.Equal(t, 1024*768, ImageFileResolution(path))
	})

	t.Run("returns resolution for PNG file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.png")
		os.WriteFile(path, createTestPNG(500, 700), 0644)
		assert.Equal(t, 500*700, ImageFileResolution(path))
	})

	t.Run("returns 0 for nonexistent file", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 0, ImageFileResolution("/nonexistent/path.jpg"))
	})

	t.Run("returns 0 for non-image file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		os.WriteFile(path, []byte("hello"), 0644)
		assert.Equal(t, 0, ImageFileResolution(path))
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/auto-cover && go test ./pkg/fileutils/ -run "TestImageResolution|TestImageFileResolution" -v`
Expected: FAIL — `ImageResolution` and `ImageFileResolution` are not defined

- [ ] **Step 3: Implement the functions**

Add to the end of `pkg/fileutils/operations.go` (before the closing of the file):

```go
// ImageResolution returns the total pixel count (width * height) of an image
// by reading only the image header (no full decode). Returns 0 if the image
// cannot be decoded.
func ImageResolution(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	return cfg.Width * cfg.Height
}

// ImageFileResolution returns the total pixel count of an image file on disk.
// Returns 0 if the file cannot be read or decoded.
func ImageFileResolution(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0
	}
	return cfg.Width * cfg.Height
}
```

No new imports needed — `bytes`, `image`, and `os` are already imported in `operations.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/auto-cover && go test ./pkg/fileutils/ -run "TestImageResolution|TestImageFileResolution" -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/fileutils/operations.go pkg/fileutils/resolution_test.go
git commit -m "[Backend] Add ImageResolution and ImageFileResolution helpers"
```

---

### Task 4: PDF Parser — Always Set CoverPage

**Files:**
- Modify: `pkg/pdf/pdf.go`
- Modify: `pkg/pdf/CLAUDE.md`

- [ ] **Step 1: Write a test verifying CoverPage is always set**

Check if there's an existing test for the failed-extraction case. The PDF test file creates fixtures in `TestMain`. We need to verify that even when cover extraction fails, `CoverPage` is still set to 0.

Read the existing PDF test to understand the pattern, then add a test assertion. For now, let's first make the code change since the existing tests should still pass, and we can verify the behavior by checking the returned metadata.

- [ ] **Step 2: Update the PDF parser**

In `pkg/pdf/pdf.go`, change lines 87-98 from:

```go
	// Extract cover image (best-effort: don't fail Parse if cover extraction fails).
	// PDF covers are always derived from page 0, so set CoverPage to signal this
	// is a page-based cover that should not be overwritten.
	var coverData []byte
	var coverMime string
	var coverPage *int
	if cd, cm, err := extractCover(path); err == nil {
		coverData = cd
		coverMime = cm
		page0 := 0
		coverPage = &page0
	}
```

To:

```go
	// Extract cover image (best-effort: don't fail Parse if cover extraction fails).
	// PDF is a page-based format — always set CoverPage to 0 regardless of whether
	// cover extraction succeeds. This ensures the frontend shows the page picker
	// and the file type guard blocks external covers consistently.
	page0 := 0
	coverPage := &page0
	var coverData []byte
	var coverMime string
	if cd, cm, err := extractCover(path); err == nil {
		coverData = cd
		coverMime = cm
	}
```

- [ ] **Step 3: Run existing PDF tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/auto-cover && go test ./pkg/pdf/ -v`
Expected: All PASS

- [ ] **Step 4: Update `pkg/pdf/CLAUDE.md`**

In `pkg/pdf/CLAUDE.md`, find the "Cover Extraction" section and add a note after the "### Tier 2" subsection:

```markdown
### CoverPage Always Set

`CoverPage` is always set to `0` for PDF files, even when cover extraction fails. This ensures:
- The frontend shows the page picker (not the upload button)
- File type guards consistently block external covers for PDFs
- The data is consistent: PDFs are page-based formats

Do NOT conditionally set `CoverPage` based on extraction success. The file type (`models.IsPageBasedFileType`) is the authoritative guard for cover protection, and `CoverPage` serves as page selection data.
```

- [ ] **Step 5: Commit**

```bash
git add pkg/pdf/pdf.go pkg/pdf/CLAUDE.md
git commit -m "[Backend] PDF parser: always set CoverPage to 0 regardless of extraction success"
```

---

### Task 5: Change Backend Guards to File Type

**Files:**
- Modify: `pkg/books/handlers.go`
- Modify: `pkg/worker/scan_unified.go`
- Modify: `pkg/worker/scan_enricher_test.go`

- [ ] **Step 1: Update cover upload guard in handlers.go**

In `pkg/books/handlers.go`, change lines 1248-1252 from:

```go
	// Files with cover_page derive their cover from page content (CBZ, PDF) and
	// cannot have it replaced by upload.
	if file.CoverPage != nil {
		return errcodes.ValidationError("Cover upload is not supported for this file type.")
	}
```

To:

```go
	// Page-based formats (CBZ, PDF) derive their cover from page content and
	// cannot have it replaced by upload.
	if models.IsPageBasedFileType(file.FileType) {
		return errcodes.ValidationError("Cover upload is not supported for this file type.")
	}
```

- [ ] **Step 2: Update enricher cover guard in scan_unified.go**

In `pkg/worker/scan_unified.go`, change lines 2768-2775 from:

```go
	// Files with CoverPage derive covers from page content (CBZ, PDF) and must
	// not have them replaced by enricher-downloaded images.
	if metadata.CoverPage != nil {
		enrichedMeta.CoverData = metadata.CoverData
		enrichedMeta.CoverMimeType = metadata.CoverMimeType
		enrichedMeta.CoverPage = metadata.CoverPage
		enrichedMeta.FieldDataSources["cover"] = metadata.SourceForField("cover")
	}
```

To:

```go
	// Page-based formats (CBZ, PDF) derive covers from page content and must
	// not have them replaced by enricher-downloaded images.
	if models.IsPageBasedFileType(file.FileType) {
		enrichedMeta.CoverData = metadata.CoverData
		enrichedMeta.CoverMimeType = metadata.CoverMimeType
		enrichedMeta.CoverPage = metadata.CoverPage
		enrichedMeta.FieldDataSources["cover"] = metadata.SourceForField("cover")
	}
```

- [ ] **Step 3: Update enricher tests**

In `pkg/worker/scan_enricher_test.go`, update the two `CoverPage != nil` checks.

Change the test at line ~469 (inside `TestCoverPageProtection_CoverPageSet_EnricherCoverReverted`). Replace:

```go
	// 3. Apply CoverPage protection (as runMetadataEnrichers does)
	if fileMetadata.CoverPage != nil {
```

With:

```go
	// 3. Apply page-based file type protection (as runMetadataEnrichers does)
	// In production, this checks models.IsPageBasedFileType(file.FileType).
	// For this test, we simulate by checking CoverPage since we're testing the
	// CBZ case where CoverPage is always set.
	if fileMetadata.CoverPage != nil {
```

Change the test at line ~513 (inside `TestCoverPageProtection_NoCoverPage_EnricherCoverPreserved`). Replace:

```go
	// CoverPage protection should NOT trigger (no CoverPage)
	if fileMetadata.CoverPage != nil {
```

With:

```go
	// Page-based file type protection should NOT trigger (EPUB is not page-based)
	// In production, this checks models.IsPageBasedFileType(file.FileType).
	// For this test, the EPUB case naturally has no CoverPage, same result.
	if fileMetadata.CoverPage != nil {
```

Note: These tests simulate the enricher merge manually (without calling `runMetadataEnrichers`), so they use `CoverPage != nil` as a proxy for the file type check. The test comments clarify this. The actual production code uses `models.IsPageBasedFileType`.

- [ ] **Step 4: Run tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/auto-cover && go test ./pkg/worker/ -run "TestCoverPage" -v && go test ./pkg/books/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/books/handlers.go pkg/worker/scan_unified.go pkg/worker/scan_enricher_test.go
git commit -m "[Backend] Change cover protection guards from CoverPage to file type checks"
```

---

### Task 6: Change Frontend Guards to File Type

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx`
- Modify: `app/components/library/FileEditDialog.tsx`

- [ ] **Step 1: Update IdentifyReviewForm**

In `app/components/library/IdentifyReviewForm.tsx`, add the import at the top (with other imports from `@/libraries/utils`):

```ts
import { isPageBasedFileType } from "@/libraries/utils";
```

Then change line 523-525 from:

```tsx
  // Cover state — files with cover_page (CBZ, PDF) derive covers from page
  // content and shouldn't be overwritten by plugin images.
  const coverEditable = file?.cover_page == null;
```

To:

```tsx
  // Cover state — page-based formats (CBZ, PDF) derive covers from page
  // content and shouldn't be overwritten by plugin images.
  const coverEditable = !isPageBasedFileType(file?.file_type);
```

- [ ] **Step 2: Update FileEditDialog — add import**

In `app/components/library/FileEditDialog.tsx`, add the import:

```ts
import { isPageBasedFileType } from "@/libraries/utils";
```

Note: `cn` is likely already imported from this file. If so, combine the imports:

```ts
import { cn, isPageBasedFileType } from "@/libraries/utils";
```

- [ ] **Step 3: Update FileEditDialog — add derived constant**

Early in the component (near line 106 where other state is declared), add a derived constant:

```tsx
const isPageBased = isPageBasedFileType(file.file_type);
```

- [ ] **Step 4: Update FileEditDialog — replace guard checks**

Replace all **guard-style** `file.cover_page == null` and `file.cover_page != null` checks with `!isPageBased` and `isPageBased` respectively. These are the lines that control which UI to show (upload vs page picker):

| Line | Current | New |
|------|---------|-----|
| ~663 | `{file.cover_page == null && (` | `{!isPageBased && (` |
| ~689 | `{file.cover_page != null && (` | `{isPageBased && (` |
| ~715 | `{file.cover_page != null &&` | `{isPageBased &&` |
| ~726 | `{file.cover_page == null && (` | `{!isPageBased && (` |
| ~756 | `{file.cover_page != null && (` | `{isPageBased && (` |
| ~773 | `{((file.cover_page == null && pendingCoverFile) ||` | `{((!isPageBased && pendingCoverFile) ||` |
| ~774 | `(file.cover_page != null &&` | `(isPageBased &&` |
| ~786 | `{file.cover_page != null && file.page_count != null && (` | `{isPageBased && file.page_count != null && (` |

**DO NOT change** lines that use `file.cover_page` as **data** (value display, comparison):
- Line ~692: `pendingCoverPage !== file.cover_page` — comparing values, keep as-is
- Line ~718: `(pendingCoverPage ?? file.cover_page)! + 1` — displaying page number, keep as-is
- Line ~776: `pendingCoverPage !== file.cover_page` — comparing values, keep as-is
- Line ~788: `currentPage={pendingCoverPage ?? file.cover_page ?? null}` — page picker value, keep as-is

- [ ] **Step 5: Run type checks and linting**

Run: `cd /Users/robinjoseph/.worktrees/shisho/auto-cover && pnpm lint:types && pnpm lint:eslint`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add app/components/library/IdentifyReviewForm.tsx app/components/library/FileEditDialog.tsx
git commit -m "[Frontend] Change cover protection guards from cover_page to file type checks"
```

---

### Task 7: Implement `upgradeEnricherCover`

**Files:**
- Modify: `pkg/worker/scan_unified.go`

- [ ] **Step 1: Write the `upgradeEnricherCover` method**

Add the following method to `pkg/worker/scan_unified.go`, after the `extractAndSaveCover` method (around line 2529):

```go
// upgradeEnricherCover checks if an enricher provided a cover image that is
// higher resolution than the file's current cover. If so, it saves the enricher
// cover to disk and updates the file record.
//
// This is called after runMetadataEnrichers in both scan paths (new file creation
// and rescan). It only applies covers from plugin sources, respects the cover
// field setting (already enforced by filterMetadataFields), and never replaces
// covers for page-based formats (CBZ, PDF).
//
// bookFilepath is the parent book's filepath. The cover directory is determined
// automatically: if bookFilepath is a directory, covers are saved there; otherwise
// (root-level files where the book path may not exist as a directory), covers are
// saved in the file's parent directory.
func (w *Worker) upgradeEnricherCover(
	ctx context.Context,
	metadata *mediafile.ParsedMetadata,
	file *models.File,
	bookFilepath string,
	jobLog *joblogs.JobLogger,
) {
	log := logger.FromContext(ctx)

	logInfo := func(msg string, data logger.Data) {
		log.Info(msg, data)
		if jobLog != nil {
			jobLog.Info(msg, data)
		}
	}

	logWarn := func(msg string, data logger.Data) {
		log.Warn(msg, data)
		if jobLog != nil {
			jobLog.Warn(msg, data)
		}
	}

	// 1. Skip if no cover data in enriched metadata
	if metadata == nil || len(metadata.CoverData) == 0 {
		return
	}

	// 2. Skip if the cover source is not from a plugin
	coverSource := metadata.SourceForField("cover")
	if !strings.HasPrefix(coverSource, models.DataSourcePluginPrefix) {
		return
	}

	// 3. Skip for page-based file types — they derive covers from page content
	if models.IsPageBasedFileType(file.FileType) {
		return
	}

	// 4. Determine the cover directory (same logic as recoverMissingCover)
	coverDir := bookFilepath
	if info, err := os.Stat(bookFilepath); err != nil || !info.IsDir() {
		coverDir = filepath.Dir(file.Filepath)
	}

	coverBaseName := filepath.Base(file.Filepath) + ".cover"
	existingCoverPath := fileutils.CoverExistsWithBaseName(coverDir, coverBaseName)

	currentResolution := 0
	if existingCoverPath != "" {
		currentResolution = fileutils.ImageFileResolution(existingCoverPath)
	}

	// 5. Resolution gate — enricher cover must be strictly larger
	enricherResolution := fileutils.ImageResolution(metadata.CoverData)
	if enricherResolution == 0 {
		logWarn("enricher cover could not be decoded, skipping", logger.Data{
			"file_id": file.ID,
			"source":  coverSource,
		})
		return
	}
	if enricherResolution <= currentResolution {
		logInfo("enricher cover not larger than current cover, skipping", logger.Data{
			"file_id":             file.ID,
			"enricher_resolution": enricherResolution,
			"current_resolution":  currentResolution,
			"source":              coverSource,
		})
		return
	}

	// 6. Save enricher cover — normalize and write to disk
	normalizedData, normalizedMime, _ := fileutils.NormalizeImage(metadata.CoverData, metadata.CoverMimeType)
	coverExt := ".png"
	if normalizedMime == metadata.CoverMimeType {
		coverExt = metadata.CoverExtension()
	}

	coverFilename := coverBaseName + coverExt
	coverFilepath := filepath.Join(coverDir, coverFilename)

	// Remove any existing cover file with a different extension
	if existingCoverPath != "" && existingCoverPath != coverFilepath {
		os.Remove(existingCoverPath)
	}

	coverFile, err := os.Create(coverFilepath)
	if err != nil {
		logWarn("failed to save enricher cover", logger.Data{
			"error": err.Error(),
			"path":  coverFilepath,
		})
		return
	}
	defer coverFile.Close()

	if _, err := io.Copy(coverFile, bytes.NewReader(normalizedData)); err != nil {
		logWarn("failed to write enricher cover data", logger.Data{
			"error": err.Error(),
			"path":  coverFilepath,
		})
		return
	}

	logInfo("upgraded cover from enricher (higher resolution)", logger.Data{
		"file_id":             file.ID,
		"enricher_resolution": enricherResolution,
		"current_resolution":  currentResolution,
		"source":              coverSource,
		"path":                coverFilepath,
	})

	// 7. Update file record
	file.CoverImageFilename = &coverFilename
	file.CoverMimeType = &normalizedMime
	file.CoverSource = &coverSource
	if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{
		Columns: []string{"cover_image_filename", "cover_mime_type", "cover_source"},
	}); err != nil {
		logWarn("failed to update file cover after enricher upgrade", logger.Data{
			"error":   err.Error(),
			"file_id": file.ID,
		})
	}
}
```

Make sure these imports are present at the top of `scan_unified.go` (most already are — verify `io` and `bytes` are included):
- `bytes`
- `io`
- `strings`

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/robinjoseph/.worktrees/shisho/auto-cover && go build ./pkg/worker/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add pkg/worker/scan_unified.go
git commit -m "[Backend] Add upgradeEnricherCover method for resolution-gated cover upgrades"
```

---

### Task 8: Integrate `upgradeEnricherCover` into Scan Paths

**Files:**
- Modify: `pkg/worker/scan_unified.go`

- [ ] **Step 1: Integrate into `scanFileCreateNew`**

In `pkg/worker/scan_unified.go`, find the enricher call in `scanFileCreateNew` (around line 2222):

```go
	// Run metadata enrichers after parsing
	metadata = w.runMetadataEnrichers(ctx, metadata, file, book, opts.LibraryID, opts.JobLog)
```

Add immediately after it:

```go
	// Apply enricher cover if it's higher resolution than the current cover
	w.upgradeEnricherCover(ctx, metadata, file, bookPath, opts.JobLog)
```

- [ ] **Step 2: Integrate into `scanFileByID`**

In `pkg/worker/scan_unified.go`, find the enricher call in `scanFileByID` (around line 456):

```go
	// Run metadata enrichers after parsing
	metadata = w.runMetadataEnrichers(ctx, metadata, file, book, file.LibraryID, opts.JobLog)
```

Add immediately after it:

```go
	// Apply enricher cover if it's higher resolution than the current cover
	w.upgradeEnricherCover(ctx, metadata, file, book.Filepath, opts.JobLog)
```

Note: The function determines the cover directory automatically from the book filepath — if it's a directory, covers go there; otherwise it falls back to the file's parent directory (handles root-level files correctly).

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/robinjoseph/.worktrees/shisho/auto-cover && go build ./pkg/worker/`
Expected: No errors

- [ ] **Step 4: Run all worker tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/auto-cover && go test ./pkg/worker/ -v -count=1`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/worker/scan_unified.go
git commit -m "[Backend] Integrate upgradeEnricherCover into both scan paths"
```

---

### Task 9: Add Tests for `upgradeEnricherCover`

**Files:**
- Create: `pkg/worker/scan_cover_upgrade_test.go`

- [ ] **Step 1: Write tests for the upgrade logic**

Create `pkg/worker/scan_cover_upgrade_test.go`. These are unit tests for the resolution comparison and file type guard logic. Since `upgradeEnricherCover` is a method on `Worker` and requires `bookService`, we test the core decision logic by testing the helpers and the integration through the existing enricher test patterns.

```go
package worker

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
)

func makeJPEG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	return buf.Bytes()
}

func makePNG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// TestEnricherCoverResolutionGate tests the resolution comparison logic
// that decides whether an enricher cover should replace the current cover.
func TestEnricherCoverResolutionGate(t *testing.T) {
	t.Parallel()

	t.Run("enricher cover larger than current is accepted", func(t *testing.T) {
		t.Parallel()
		current := makeJPEG(200, 300)   // 60,000 pixels
		enricher := makeJPEG(800, 1200) // 960,000 pixels

		currentRes := fileutils.ImageResolution(current)
		enricherRes := fileutils.ImageResolution(enricher)

		assert.Greater(t, enricherRes, currentRes)
	})

	t.Run("enricher cover same size as current is rejected", func(t *testing.T) {
		t.Parallel()
		current := makeJPEG(800, 1200)
		enricher := makeJPEG(800, 1200)

		currentRes := fileutils.ImageResolution(current)
		enricherRes := fileutils.ImageResolution(enricher)

		// enricherResolution <= currentResolution → skip
		assert.False(t, enricherRes > currentRes)
	})

	t.Run("enricher cover smaller than current is rejected", func(t *testing.T) {
		t.Parallel()
		current := makeJPEG(800, 1200)
		enricher := makeJPEG(200, 300)

		currentRes := fileutils.ImageResolution(current)
		enricherRes := fileutils.ImageResolution(enricher)

		assert.False(t, enricherRes > currentRes)
	})

	t.Run("no current cover — enricher always accepted", func(t *testing.T) {
		t.Parallel()
		enricher := makeJPEG(400, 600)

		currentRes := 0 // no cover on disk
		enricherRes := fileutils.ImageResolution(enricher)

		assert.Greater(t, enricherRes, currentRes)
	})

	t.Run("undecodable enricher cover is rejected", func(t *testing.T) {
		t.Parallel()
		enricherRes := fileutils.ImageResolution([]byte("not an image"))
		assert.Equal(t, 0, enricherRes)
	})
}

// TestEnricherCoverPageBasedGuard tests that page-based file types block enricher covers.
func TestEnricherCoverPageBasedGuard(t *testing.T) {
	t.Parallel()

	t.Run("CBZ blocks enricher covers", func(t *testing.T) {
		t.Parallel()
		assert.True(t, models.IsPageBasedFileType(models.FileTypeCBZ))
	})

	t.Run("PDF blocks enricher covers", func(t *testing.T) {
		t.Parallel()
		assert.True(t, models.IsPageBasedFileType(models.FileTypePDF))
	})

	t.Run("EPUB allows enricher covers", func(t *testing.T) {
		t.Parallel()
		assert.False(t, models.IsPageBasedFileType(models.FileTypeEPUB))
	})

	t.Run("M4B allows enricher covers", func(t *testing.T) {
		t.Parallel()
		assert.False(t, models.IsPageBasedFileType(models.FileTypeM4B))
	})
}

// TestEnricherCoverPluginSourceCheck tests that only plugin-sourced covers trigger upgrades.
func TestEnricherCoverPluginSourceCheck(t *testing.T) {
	t.Parallel()

	t.Run("plugin source is detected", func(t *testing.T) {
		t.Parallel()
		md := &mediafile.ParsedMetadata{
			CoverData:     makeJPEG(800, 1200),
			CoverMimeType: "image/jpeg",
			FieldDataSources: map[string]string{
				"cover": models.PluginDataSource("test", "enricher"),
			},
		}
		source := md.SourceForField("cover")
		assert.True(t, len(source) > len(models.DataSourcePluginPrefix))
		assert.Equal(t, "plugin:", source[:7])
	})

	t.Run("file metadata source is not a plugin", func(t *testing.T) {
		t.Parallel()
		md := &mediafile.ParsedMetadata{
			CoverData:     makeJPEG(800, 1200),
			CoverMimeType: "image/jpeg",
			DataSource:    models.DataSourceEPUBMetadata,
		}
		source := md.SourceForField("cover")
		assert.NotEqual(t, "plugin:", source[:7])
	})
}
```

- [ ] **Step 2: Run the tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/auto-cover && go test ./pkg/worker/ -run "TestEnricherCover" -v`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add pkg/worker/scan_cover_upgrade_test.go
git commit -m "[Test] Add tests for enricher cover resolution gate and file type guard"
```

---

### Task 10: Update Website Documentation

**Files:**
- Modify: `website/docs/plugins/development.md`

- [ ] **Step 1: Update the Cover Images section**

In `website/docs/plugins/development.md`, find the "#### Cover Images" section (around line 414). Replace the existing content with an expanded version. Change:

```markdown
#### Cover Images

Set `coverUrl` on your search results — the server handles downloading and domain validation automatically. The URL's domain must be in your manifest's `httpAccess.domains` list.

```javascript
return {
  results: [{
    title: "Book Title",
    coverUrl: "https://covers.example.com/book.jpg"
  }]
};
```

For advanced use cases (file parsers extracting embedded covers, or enrichers that generate/composite images), you can set `coverData` as an `ArrayBuffer` instead. If both are set, `coverData` takes precedence.
```

To:

```markdown
#### Cover Images

Set `coverUrl` on your search results — the server handles downloading and domain validation automatically. The URL's domain must be in your manifest's `httpAccess.domains` list.

```javascript
return {
  results: [{
    title: "Book Title",
    coverUrl: "https://covers.example.com/book.jpg"
  }]
};
```

For advanced use cases (file parsers extracting embedded covers, or enrichers that generate/composite images), you can set `coverData` as an `ArrayBuffer` instead. If both are set, `coverData` takes precedence.

**Cover enrichment rules:**

During automatic scans, enricher-provided covers are subject to additional checks:

- **Resolution gate:** An enricher cover is only applied if its total resolution (width × height) is strictly greater than the file's current cover. If the file already has a cover of equal or greater resolution, the enricher cover is skipped. This prevents low-resolution external images from replacing high-quality embedded covers.
- **Page-based formats:** CBZ and PDF files derive covers from their page content. Plugin covers are never applied to these formats, even if the plugin declares the `cover` field.
- **Field settings:** The `cover` field must be enabled in the plugin's per-library field settings for cover enrichment to take effect. If disabled, all cover data from the plugin is silently stripped.
```

- [ ] **Step 2: Update the Enrichment Behavior section**

In the same file, find the "#### Enrichment Behavior" section (around line 476). After the paragraph that starts "When a user identifies a book using the interactive review screen", add a new paragraph:

```markdown
During automatic scans, enrichment also respects a cover resolution gate — enricher covers must have a higher total resolution than the existing cover to be applied. See [Cover Images](#cover-images) above for details.
```

- [ ] **Step 3: Verify the docs build**

Run: `cd /Users/robinjoseph/.worktrees/shisho/auto-cover && mise docs &` then check for build errors. If `mise docs` is not available in the worktree, just verify the markdown is valid.

- [ ] **Step 4: Commit**

```bash
git add website/docs/plugins/development.md
git commit -m "[Docs] Document enricher cover resolution gate and page-based format protection"
```

---

### Task 11: Run Full Validation

**Files:** None (validation only)

- [ ] **Step 1: Run all checks**

Run: `cd /Users/robinjoseph/.worktrees/shisho/auto-cover && mise check:quiet`
Expected: All checks pass (Go tests, Go lint, JS lint, JS tests)

- [ ] **Step 2: Fix any issues**

If any check fails, fix the issue and re-run. Common issues:
- Missing imports in Go files
- Linting issues (unused variables, formatting)
- TypeScript type errors from the frontend changes

- [ ] **Step 3: Final commit if fixes were needed**

Only commit if changes were made in step 2.
