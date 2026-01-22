# CBZ Cover Page Selection Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow users to select any page from a CBZ file as the cover image via the FileEditDialog

**Architecture:** New PUT endpoint extracts the selected page as an external cover file and updates the File model's cover metadata. The CBZPagePicker component is reused with a customizable title for cover selection.

**Tech Stack:** Go/Echo backend, React/TypeScript frontend, TanStack Query

---

## Task 1: Add CoverPage field to FileSidecar

**Files:**
- Modify: `pkg/sidecar/types.go:23-33`
- Modify: `pkg/sidecar/sidecar.go:172-226`

**Step 1: Add CoverPage field to FileSidecar struct**

In `pkg/sidecar/types.go`, add `CoverPage` field to `FileSidecar`:

```go
type FileSidecar struct {
	Version     int                  `json:"version"`
	Narrators   []NarratorMetadata   `json:"narrators,omitempty"`
	URL         *string              `json:"url,omitempty"`
	Publisher   *string              `json:"publisher,omitempty"`
	Imprint     *string              `json:"imprint,omitempty"`
	ReleaseDate *string              `json:"release_date,omitempty"` // ISO 8601 date string (YYYY-MM-DD)
	Identifiers []IdentifierMetadata `json:"identifiers,omitempty"`
	Name        *string              `json:"name,omitempty"`
	Chapters    []ChapterMetadata    `json:"chapters,omitempty"`
	CoverPage   *int                 `json:"cover_page,omitempty"` // 0-indexed page number for CBZ cover
}
```

**Step 2: Update FileSidecarFromModel to include CoverPage**

In `pkg/sidecar/sidecar.go`, update `FileSidecarFromModel` to copy the `CoverPage` field:

```go
// FileSidecarFromModel creates a FileSidecar from a File model.
func FileSidecarFromModel(file *models.File) *FileSidecar {
	s := &FileSidecar{
		Version:   CurrentVersion,
		URL:       file.URL,
		Publisher: nil,
		Imprint:   nil,
		Name:      file.Name,
		CoverPage: file.CoverPage,
	}
	// ... rest of function unchanged
```

**Step 3: Run tests to verify no regressions**

Run: `make test`
Expected: All tests pass

**Step 4: Commit**

```bash
git add pkg/sidecar/types.go pkg/sidecar/sidecar.go
git commit -m "$(cat <<'EOF'
[Sidecar] Add CoverPage field to FileSidecar

Add CoverPage field to persist cover page selection to sidecar metadata.
This enables the cover page choice to survive library rescans.
EOF
)"
```

---

## Task 2: Create UpdateFileCoverPage handler

**Files:**
- Create: `pkg/books/handlers_cover_page.go`
- Modify: `pkg/books/routes.go:61`

**Step 1: Write the failing test**

Create test file `pkg/books/handlers_cover_page_test.go`:

```go
package books

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateFileCoverPage(t *testing.T) {
	t.Run("sets cover page and extracts cover image", func(t *testing.T) {
		db := testhelpers.NewTestDB(t)
		cfg := &config.Config{CacheDir: t.TempDir()}
		e := echo.New()
		bookService := NewService(db)
		pageCache := cbzpages.NewCache(cfg.CacheDir)

		h := &handler{
			bookService: bookService,
			pageCache:   pageCache,
		}

		// Create test library, book, and CBZ file
		library := testhelpers.CreateLibrary(t, db, "Test Library", t.TempDir())
		bookDir := filepath.Join(library.Filepath, "Test Book")
		testhelpers.CreateDirectory(t, bookDir)
		cbzPath := filepath.Join(bookDir, "test.cbz")
		testhelpers.CreateTestCBZ(t, cbzPath, 5) // 5 pages

		// Create book and file in database
		book := testhelpers.CreateBook(t, db, library.ID, bookDir, "Test Book")
		file := testhelpers.CreateFile(t, db, book.ID, library.ID, cbzPath, models.FileTypeCBZ)
		pageCount := 5
		file.PageCount = &pageCount
		_, err := db.NewUpdate().Model(file).Column("page_count").WherePK().Exec(testhelpers.Ctx(t))
		require.NoError(t, err)

		// Make request to set cover page to page 2 (0-indexed)
		payload := map[string]int{"page": 2}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(testhelpers.Itoa(file.ID))

		err = h.updateFileCoverPage(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify file was updated
		var updatedFile models.File
		err = db.NewSelect().Model(&updatedFile).Where("id = ?", file.ID).Scan(testhelpers.Ctx(t))
		require.NoError(t, err)

		require.NotNil(t, updatedFile.CoverPage)
		assert.Equal(t, 2, *updatedFile.CoverPage)
		assert.NotNil(t, updatedFile.CoverMimeType)
		assert.NotNil(t, updatedFile.CoverSource)
		assert.Equal(t, models.DataSourceManual, *updatedFile.CoverSource)
		assert.NotNil(t, updatedFile.CoverImagePath)
	})

	t.Run("returns 400 for invalid page number", func(t *testing.T) {
		db := testhelpers.NewTestDB(t)
		cfg := &config.Config{CacheDir: t.TempDir()}
		e := echo.New()
		bookService := NewService(db)
		pageCache := cbzpages.NewCache(cfg.CacheDir)

		h := &handler{
			bookService: bookService,
			pageCache:   pageCache,
		}

		// Create test library, book, and CBZ file
		library := testhelpers.CreateLibrary(t, db, "Test Library", t.TempDir())
		bookDir := filepath.Join(library.Filepath, "Test Book")
		testhelpers.CreateDirectory(t, bookDir)
		cbzPath := filepath.Join(bookDir, "test.cbz")
		testhelpers.CreateTestCBZ(t, cbzPath, 5) // 5 pages

		book := testhelpers.CreateBook(t, db, library.ID, bookDir, "Test Book")
		file := testhelpers.CreateFile(t, db, book.ID, library.ID, cbzPath, models.FileTypeCBZ)
		pageCount := 5
		file.PageCount = &pageCount
		_, err := db.NewUpdate().Model(file).Column("page_count").WherePK().Exec(testhelpers.Ctx(t))
		require.NoError(t, err)

		// Request page 10 which is out of bounds
		payload := map[string]int{"page": 10}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(testhelpers.Itoa(file.ID))

		err = h.updateFileCoverPage(c)
		require.Error(t, err)
	})

	t.Run("returns 400 for non-CBZ file", func(t *testing.T) {
		db := testhelpers.NewTestDB(t)
		cfg := &config.Config{CacheDir: t.TempDir()}
		e := echo.New()
		bookService := NewService(db)
		pageCache := cbzpages.NewCache(cfg.CacheDir)

		h := &handler{
			bookService: bookService,
			pageCache:   pageCache,
		}

		// Create test library, book, and EPUB file
		library := testhelpers.CreateLibrary(t, db, "Test Library", t.TempDir())
		bookDir := filepath.Join(library.Filepath, "Test Book")
		testhelpers.CreateDirectory(t, bookDir)
		epubPath := filepath.Join(bookDir, "test.epub")
		testhelpers.CreateFile(t, epubPath, []byte("fake epub"))

		book := testhelpers.CreateBook(t, db, library.ID, bookDir, "Test Book")
		file := testhelpers.CreateFile(t, db, book.ID, library.ID, epubPath, models.FileTypeEPUB)

		payload := map[string]int{"page": 0}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(testhelpers.Itoa(file.ID))

		err := h.updateFileCoverPage(c)
		require.Error(t, err)
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/books/... -run TestUpdateFileCoverPage`
Expected: FAIL - handler not defined

**Step 3: Create handler file**

Create `pkg/books/handlers_cover_page.go`:

```go
package books

import (
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/logger"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
)

// updateFileCoverPagePayload is the request body for setting a CBZ cover page.
type updateFileCoverPagePayload struct {
	Page int `json:"page"` // 0-indexed page number
}

// updateFileCoverPage handles PUT /files/:id/cover-page
// Sets the cover page for a CBZ file and extracts it as an external cover image.
func (h *handler) updateFileCoverPage(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	var payload updateFileCoverPagePayload
	if err := c.Bind(&payload); err != nil {
		return errcodes.ValidationError("Invalid request body")
	}

	// Fetch the file with book relation
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Validate file type is CBZ
	if file.FileType != models.FileTypeCBZ {
		return errcodes.ValidationError("Cover page selection is only available for CBZ files")
	}

	// Validate page is within bounds
	if file.PageCount == nil || payload.Page < 0 || payload.Page >= *file.PageCount {
		return errcodes.ValidationError("Page number is out of bounds")
	}

	// Extract the page image using the page cache
	cachedPath, mimeType, err := h.pageCache.GetPage(file.Filepath, file.ID, payload.Page)
	if err != nil {
		log.Error("failed to extract page from CBZ", logger.Data{"error": err.Error(), "page": payload.Page})
		return errcodes.InternalError("Failed to extract page from CBZ file")
	}

	// Determine cover directory (same logic as fileCover and uploadFileCover)
	isRootLevelBook := false
	if info, err := os.Stat(file.Book.Filepath); err == nil && !info.IsDir() {
		isRootLevelBook = true
	}
	var coverDir string
	if isRootLevelBook {
		coverDir = filepath.Dir(file.Book.Filepath)
	} else {
		coverDir = file.Book.Filepath
	}

	// Generate the cover filename: {filename}.cover.{ext}
	filename := filepath.Base(file.Filepath)
	coverBaseName := filename + ".cover"

	// Get extension from mime type
	ext := getExtensionFromMimeType(mimeType)
	if ext == "" {
		ext = filepath.Ext(cachedPath)
	}

	// Delete any existing cover with this base name (regardless of extension)
	for _, existingExt := range fileutils.CoverImageExtensions {
		existingPath := filepath.Join(coverDir, coverBaseName+existingExt)
		if _, err := os.Stat(existingPath); err == nil {
			if err := os.Remove(existingPath); err != nil {
				log.Warn("failed to remove existing cover", logger.Data{"path": existingPath, "error": err.Error()})
			}
		}
	}

	// Copy the extracted page to the cover location
	coverFilePath := filepath.Join(coverDir, coverBaseName+ext)
	if err := copyFile(cachedPath, coverFilePath); err != nil {
		log.Error("failed to copy cover image", logger.Data{"error": err.Error()})
		return errcodes.InternalError("Failed to save cover image")
	}

	log.Info("set CBZ cover page", logger.Data{
		"file_id":    file.ID,
		"page":       payload.Page,
		"cover_path": coverFilePath,
		"mime_type":  mimeType,
	})

	// Update file's cover metadata
	coverFilename := coverBaseName + ext
	file.CoverPage = &payload.Page
	file.CoverMimeType = &mimeType
	file.CoverSource = strPtr(models.DataSourceManual)
	file.CoverImagePath = &coverFilename

	if err := h.bookService.UpdateFile(ctx, file, UpdateFileOptions{
		Columns: []string{"cover_page", "cover_mime_type", "cover_source", "cover_image_path"},
	}); err != nil {
		return errors.WithStack(err)
	}

	// Write sidecar to persist the cover page choice
	if err := sidecar.WriteFileSidecarFromModel(file); err != nil {
		log.Warn("failed to write file sidecar", logger.Data{"error": err.Error()})
	}

	return c.JSON(200, file)
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return errors.WithStack(err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return errors.WithStack(err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return errors.WithStack(err)
	}

	return nil
}
```

**Step 4: Register the route**

In `pkg/books/routes.go`, add after line 61 (after the `POST /files/:id/cover` route):

```go
g.PUT("/files/:id/cover-page", h.updateFileCoverPage, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
```

**Step 5: Run test to verify it passes**

Run: `go test -v ./pkg/books/... -run TestUpdateFileCoverPage`
Expected: PASS

**Step 6: Run full test suite**

Run: `make test`
Expected: All tests pass

**Step 7: Commit**

```bash
git add pkg/books/handlers_cover_page.go pkg/books/handlers_cover_page_test.go pkg/books/routes.go
git commit -m "$(cat <<'EOF'
[API] Add PUT /files/:id/cover-page endpoint for CBZ cover selection

Extract the selected page from CBZ and save as external cover file.
Updates cover_page, cover_mime_type, cover_source, cover_image_path.
Writes sidecar to persist the choice across rescans.
EOF
)"
```

---

## Task 3: Add title prop to CBZPagePicker

**Files:**
- Modify: `app/components/files/CBZPagePicker.tsx`

**Step 1: Add title prop to CBZPagePicker**

In `app/components/files/CBZPagePicker.tsx`, update the props interface and component:

```tsx
export interface CBZPagePickerProps {
  fileId: number;
  pageCount: number;
  currentPage: number | null;
  onSelect: (page: number) => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title?: string; // Optional custom title, defaults to "Select Page"
}

/**
 * Dialog for selecting a page from a CBZ file.
 * Shows a grid of page thumbnails with lazy loading of pages in batches of 10.
 */
const CBZPagePicker = ({
  fileId,
  pageCount,
  currentPage,
  onSelect,
  open,
  onOpenChange,
  title = "Select Page",
}: CBZPagePickerProps) => {
```

And update the DialogTitle:

```tsx
<DialogHeader className="pr-8">
  <DialogTitle>{title}</DialogTitle>
</DialogHeader>
```

**Step 2: Run lint and type checks**

Run: `yarn lint`
Expected: No errors

**Step 3: Commit**

```bash
git add app/components/files/CBZPagePicker.tsx
git commit -m "$(cat <<'EOF'
[UI] Add title prop to CBZPagePicker component

Allows customizing the dialog title for different use cases like
cover page selection vs reading position.
EOF
)"
```

---

## Task 4: Add useSetFileCoverPage mutation hook

**Files:**
- Modify: `app/hooks/queries/books.ts`

**Step 1: Add the mutation hook**

In `app/hooks/queries/books.ts`, add after `useUploadFileCover`:

```ts
interface SetFileCoverPageVariables {
  id: number;
  page: number;
}

export const useSetFileCoverPage = () => {
  const queryClient = useQueryClient();

  return useMutation<File, ShishoAPIError, SetFileCoverPageVariables>({
    mutationFn: ({ id, page }) => {
      return API.request("PUT", `/books/files/${id}/cover-page`, { page }, null);
    },
    onSuccess: () => {
      // Invalidate book queries to refresh file/cover data
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
    },
  });
};
```

**Step 2: Run lint and type checks**

Run: `yarn lint`
Expected: No errors

**Step 3: Commit**

```bash
git add app/hooks/queries/books.ts
git commit -m "$(cat <<'EOF'
[Frontend] Add useSetFileCoverPage mutation hook

Calls PUT /books/files/:id/cover-page to set the cover page for CBZ files.
Invalidates book queries to refresh cover thumbnails.
EOF
)"
```

---

## Task 5: Add CBZ cover selection UI to FileEditDialog

**Files:**
- Modify: `app/components/library/FileEditDialog.tsx`

**Step 1: Add imports and state**

In `app/components/library/FileEditDialog.tsx`, add imports:

```tsx
import CBZPagePicker from "@/components/files/CBZPagePicker";
import { useSetFileCoverPage } from "@/hooks/queries/books";
```

And add state for the page picker dialog inside the component:

```tsx
const [coverPagePickerOpen, setCoverPagePickerOpen] = useState(false);
const setCoverPageMutation = useSetFileCoverPage();
```

**Step 2: Add handler for cover page selection**

Add handler function:

```tsx
const handleCoverPageSelect = (page: number) => {
  setCoverPageMutation.mutate(
    { id: file.id, page },
    {
      onSuccess: () => {
        // Update the cache buster to refresh the cover image
        setCoverCacheBuster(Date.now());
      },
    },
  );
};
```

**Step 3: Add CBZ cover section UI**

Replace the existing cover section (around lines 493-539) that excludes CBZ files with a unified approach. After the `{!isSupplement && fileRole !== FileRoleSupplement && (` block, update to include CBZ:

```tsx
{/* Cover section - different UI for CBZ vs other file types */}
{!isSupplement && fileRole !== FileRoleSupplement && (
  <>
    {file.file_type === FileTypeCBZ ? (
      /* CBZ Cover Page Selection */
      <div className="space-y-2">
        <Label>Cover Image</Label>
        <div className="flex items-start gap-4">
          <div className="w-32">
            {file.cover_mime_type || file.cover_image_path ? (
              <img
                alt="File cover"
                className="w-full h-auto rounded border border-border"
                src={`/api/books/files/${file.id}/cover?t=${coverCacheBuster}`}
              />
            ) : (
              <CoverPlaceholder
                className="rounded border border-dashed border-border aspect-[2/3]"
                variant="book"
              />
            )}
          </div>
          <div className="flex flex-col gap-2">
            {file.cover_page != null && (
              <span className="text-sm text-muted-foreground">
                Page {file.cover_page + 1}
              </span>
            )}
            <Button
              disabled={setCoverPageMutation.isPending}
              onClick={() => setCoverPagePickerOpen(true)}
              size="sm"
              type="button"
              variant="outline"
            >
              {setCoverPageMutation.isPending ? (
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
              ) : (
                <ImageIcon className="h-4 w-4 mr-2" />
              )}
              Select from pages
            </Button>
          </div>
        </div>
        {file.page_count != null && (
          <CBZPagePicker
            currentPage={file.cover_page}
            fileId={file.id}
            onOpenChange={setCoverPagePickerOpen}
            onSelect={handleCoverPageSelect}
            open={coverPagePickerOpen}
            pageCount={file.page_count}
            title="Select Cover Page"
          />
        )}
      </div>
    ) : (
      /* Non-CBZ Cover Upload */
      <div className="space-y-2">
        {/* ... existing cover upload UI ... */}
      </div>
    )}
  </>
)}
```

**Step 4: Add ImageIcon import**

Add to the lucide-react imports:

```tsx
import { ImageIcon } from "lucide-react";
```

**Step 5: Run lint and type checks**

Run: `yarn lint`
Expected: No errors

**Step 6: Test manually**

Run: `make start`
- Open a CBZ file in the FileEditDialog
- Verify cover thumbnail shows
- Verify "Page N" label shows current cover page
- Click "Select from pages"
- Verify CBZPagePicker opens with title "Select Cover Page"
- Select a different page
- Verify cover updates with new selection

**Step 7: Commit**

```bash
git add app/components/library/FileEditDialog.tsx
git commit -m "$(cat <<'EOF'
[UI] Add CBZ cover page selection to FileEditDialog

Shows current cover with page number and "Select from pages" button.
Uses CBZPagePicker for page selection and calls the new cover-page API.
EOF
)"
```

---

## Task 6: Add CoverPage to KePub CBZ generation metadata

**Files:**
- Modify: `pkg/kepub/cbz.go:26-39`
- Modify: `pkg/filegen/kepub_cbz.go:44-104`

**Step 1: Add CoverPage field to CBZMetadata**

In `pkg/kepub/cbz.go`, add to CBZMetadata struct:

```go
type CBZMetadata struct {
	Title       string
	Name        *string // Name takes precedence over Title when non-empty
	Subtitle    *string
	Description *string
	Authors     []CBZAuthor
	Series      []CBZSeries
	Genres      []string
	Tags        []string
	URL         *string
	Publisher   *string
	Imprint     *string
	ReleaseDate *time.Time
	Chapters    []CBZChapter
	CoverPage   *int // 0-indexed page number for cover (nil = first page)
}
```

**Step 2: Update buildCBZMetadata to include CoverPage**

In `pkg/filegen/kepub_cbz.go`, add to `buildCBZMetadata`:

```go
// Set cover page if available
if file != nil && file.CoverPage != nil {
	metadata.CoverPage = file.CoverPage
}
```

**Step 3: Update KePub generation to use CoverPage**

In `pkg/kepub/cbz.go`, update the OPF generation to use the cover page index. Find the line that sets `properties="cover-image"` on the first image (around line 472 and 491-492) and update to use the metadata cover page:

Update the cover meta tag generation (around line 472):

```go
// Determine cover page index
coverPageIdx := 0
if metadata != nil && metadata.CoverPage != nil {
	coverPageIdx = *metadata.CoverPage
}

buf.WriteString(fmt.Sprintf(`    <meta name="cover" content="img%04d"/>
`, coverPageIdx+1))
```

Update the manifest item generation (around line 490-493):

```go
// Image item
buf.WriteString(fmt.Sprintf(`    <item id="%s" href="images/%s" media-type="%s"`, imgID, page.filename, page.mediaType))
if i == coverPageIdx {
	buf.WriteString(` properties="cover-image"`)
}
buf.WriteString(`/>
`)
```

Note: You'll need to move `coverPageIdx` to be calculated before the manifest loop so it's available in the loop scope.

**Step 4: Write test for cover page in KePub generation**

Add test to `pkg/kepub/cbz_test.go`:

```go
func TestConvertCBZWithMetadata_CoverPage(t *testing.T) {
	// Create test CBZ with 3 pages
	srcPath := filepath.Join(t.TempDir(), "test.cbz")
	createTestCBZ(t, srcPath, 3)

	destPath := filepath.Join(t.TempDir(), "output.kepub.epub")

	coverPage := 2 // Third page (0-indexed)
	metadata := &CBZMetadata{
		Title:     "Test Comic",
		CoverPage: &coverPage,
	}

	c := NewConverter()
	err := c.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
	require.NoError(t, err)

	// Verify the OPF has cover-image on the correct page
	destZip, err := zip.OpenReader(destPath)
	require.NoError(t, err)
	defer destZip.Close()

	var opfContent []byte
	for _, f := range destZip.File {
		if strings.HasSuffix(f.Name, "content.opf") {
			r, err := f.Open()
			require.NoError(t, err)
			opfContent, err = io.ReadAll(r)
			r.Close()
			require.NoError(t, err)
			break
		}
	}

	require.NotEmpty(t, opfContent)

	// Check that cover meta tag references the correct image
	assert.Contains(t, string(opfContent), `<meta name="cover" content="img0003"/>`)

	// Check that cover-image property is on the third image
	assert.Contains(t, string(opfContent), `id="img0003"`)
	assert.Contains(t, string(opfContent), `properties="cover-image"`)

	// Ensure the first image does NOT have cover-image property
	// (it should appear before the cover-image one)
	idx1 := strings.Index(string(opfContent), `id="img0001"`)
	idxCover := strings.Index(string(opfContent), `cover-image`)
	assert.True(t, idx1 < idxCover, "img0001 should appear before cover-image property")
}
```

**Step 5: Run test to verify it passes**

Run: `go test -v ./pkg/kepub/... -run TestConvertCBZWithMetadata_CoverPage`
Expected: PASS

**Step 6: Run full test suite**

Run: `make test`
Expected: All tests pass

**Step 7: Commit**

```bash
git add pkg/kepub/cbz.go pkg/kepub/cbz_test.go pkg/filegen/kepub_cbz.go
git commit -m "$(cat <<'EOF'
[KePub] Use CoverPage when converting CBZ to KePub

Marks the specified page as cover-image in the EPUB manifest.
Defaults to first page if not specified.
EOF
)"
```

---

## Task 7: Run full validation

**Step 1: Run all checks**

Run: `make check`
Expected: All checks pass (tests, Go lint, JS lint)

**Step 2: Test end-to-end manually**

Run: `make start`

1. Navigate to a book with a CBZ file
2. Open the FileEditDialog for the CBZ file
3. Verify current cover shows with page number
4. Click "Select from pages"
5. Select a different page (e.g., page 3)
6. Verify:
   - Cover thumbnail updates immediately
   - Page number label updates to "Page 3"
   - Closing and reopening dialog shows the new cover
7. Download the CBZ file (should have ComicInfo.xml with FrontCover on page 3)
8. Download as KePub (should have cover-image on page 3)

**Step 3: Final commit if any cleanup needed**

If no changes needed, skip this step.

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Add CoverPage to FileSidecar | `pkg/sidecar/types.go`, `pkg/sidecar/sidecar.go` |
| 2 | Create PUT /files/:id/cover-page handler | `pkg/books/handlers_cover_page.go`, `pkg/books/routes.go` |
| 3 | Add title prop to CBZPagePicker | `app/components/files/CBZPagePicker.tsx` |
| 4 | Add useSetFileCoverPage mutation | `app/hooks/queries/books.ts` |
| 5 | Add CBZ cover UI to FileEditDialog | `app/components/library/FileEditDialog.tsx` |
| 6 | Add CoverPage to KePub generation | `pkg/kepub/cbz.go`, `pkg/filegen/kepub_cbz.go` |
| 7 | Full validation | N/A |
