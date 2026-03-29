# PDF Cover Page Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow users to select which page of a PDF to use as the cover image, using the same page picker UI that CBZ files use.

**Architecture:** Remove the CBZ-only restriction from the cover page handler, route to the correct page cache by file type, and use `cover_page != null` as the universal discriminator for the page-based cover workflow in the frontend.

**Tech Stack:** Go (Echo handlers, pdfpages cache), React/TypeScript (PagePicker component, FileEditDialog)

---

### Task 1: Rename CBZ Page Components to Generic Names

Rename the three CBZ-specific page components to generic names and update all imports.

**Files:**
- Rename: `app/components/files/CBZPagePicker.tsx` → `app/components/files/PagePicker.tsx`
- Rename: `app/components/files/CBZPagePreview.tsx` → `app/components/files/PagePreview.tsx`
- Rename: `app/components/files/CBZPageThumbnail.tsx` → `app/components/files/PageThumbnail.tsx`
- Modify: `app/components/library/FileEditDialog.tsx` (import path)
- Modify: `app/components/library/FileEditDialog.test.tsx` (mock path)
- Modify: `app/components/files/ChapterRow.tsx` (import paths)

- [ ] **Step 1: Rename `CBZPagePicker.tsx` → `PagePicker.tsx`**

Rename the file and update internal names:

```bash
mv app/components/files/CBZPagePicker.tsx app/components/files/PagePicker.tsx
```

In `app/components/files/PagePicker.tsx`:
- Rename interface `CBZPagePickerProps` → `PagePickerProps`
- Rename component `CBZPagePicker` → `PagePicker`
- Update JSDoc comment: "Dialog for selecting a page from a file." (remove "CBZ")
- Update default export

- [ ] **Step 2: Rename `CBZPagePreview.tsx` → `PagePreview.tsx`**

```bash
mv app/components/files/CBZPagePreview.tsx app/components/files/PagePreview.tsx
```

In `app/components/files/PagePreview.tsx`:
- Rename interface `CBZPagePreviewProps` → `PagePreviewProps`
- Rename component `CBZPagePreview` → `PagePreview`
- Update JSDoc comments (remove "CBZ" references)
- Update default export

- [ ] **Step 3: Rename `CBZPageThumbnail.tsx` → `PageThumbnail.tsx`**

```bash
mv app/components/files/CBZPageThumbnail.tsx app/components/files/PageThumbnail.tsx
```

In `app/components/files/PageThumbnail.tsx`:
- Rename interface `CBZPageThumbnailProps` → `PageThumbnailProps`
- Rename component `CBZPageThumbnail` → `PageThumbnail`
- Update JSDoc comments (remove "CBZ" references)
- Update default export

- [ ] **Step 4: Update all imports**

In `app/components/library/FileEditDialog.tsx` (line 15):
```tsx
// Old
import CBZPagePicker from "@/components/files/CBZPagePicker";
// New
import PagePicker from "@/components/files/PagePicker";
```

Also update the JSX usage (line 787):
```tsx
// Old
<CBZPagePicker
// New
<PagePicker
```

In `app/components/library/FileEditDialog.test.tsx` (line 58):
```tsx
// Old
vi.mock("@/components/files/CBZPagePicker", () => ({
// New
vi.mock("@/components/files/PagePicker", () => ({
```

In `app/components/files/ChapterRow.tsx` (lines 1-2):
```tsx
// Old
import CBZPagePicker from "./CBZPagePicker";
import CBZPagePreview from "./CBZPagePreview";
// New
import PagePicker from "./PagePicker";
import PagePreview from "./PagePreview";
```

Also update all JSX usages of `CBZPagePicker` → `PagePicker` and `CBZPagePreview` → `PagePreview` in ChapterRow.tsx (lines 448, 511, 633).

- [ ] **Step 5: Run lint to verify no broken imports**

Run: `pnpm lint:types`
Expected: PASS (no type errors)

- [ ] **Step 6: Commit**

```bash
git add -A app/components/files/PagePicker.tsx app/components/files/PagePreview.tsx \
  app/components/files/PageThumbnail.tsx app/components/library/FileEditDialog.tsx \
  app/components/library/FileEditDialog.test.tsx app/components/files/ChapterRow.tsx
git commit -m "[Frontend] Rename CBZ page components to generic PagePicker/PagePreview/PageThumbnail"
```

---

### Task 2: Update Backend Handler to Support PDF

Remove the CBZ-only file type restriction and route to the correct page cache based on file type.

**Files:**
- Modify: `pkg/books/handlers_cover_page.go:19-68`

- [ ] **Step 1: Write the failing test — PDF cover page selection**

In `pkg/books/handlers_cover_page_test.go`, add a new test after the existing "sets cover page and extracts cover image" test. This test creates a PDF file (using a real PDF rendered via pdfpages), sets cover page 0, and verifies the cover is extracted.

However, PDF page rendering requires the pdfium WASM runtime which is heavy for unit tests. Instead, the test should verify the **handler logic** — that PDF files are accepted and routed to the pdfPageCache. We can test with a mock-like approach: create a PDF file record and use the pdfPageCache. Since we can't easily create a real PDF in tests, we'll test the validation path instead.

Add this test to `pkg/books/handlers_cover_page_test.go` inside `TestUpdateFileCoverPage`:

```go
t.Run("returns 400 for file without page count", func(t *testing.T) {
    // This test already exists — skip
})

t.Run("accepts PDF file type", func(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    ctx := context.Background()
    e := echo.New()
    bookService := NewService(db)

    h := &handler{
        bookService: bookService,
        // Note: no pageCache or pdfPageCache — we test that validation passes
        // and the handler reaches the page extraction step (which will fail
        // because pdfPageCache is nil, but that proves the file type check passed)
    }

    // Create test library
    library := &models.Library{
        Name:                     "Test Library",
        CoverAspectRatio:         "book",
        DownloadFormatPreference: models.DownloadFormatOriginal,
    }
    _, err := db.NewInsert().Model(library).Exec(ctx)
    require.NoError(t, err)

    bookDir := filepath.Join(t.TempDir(), "Test Book")
    err = os.MkdirAll(bookDir, 0755)
    require.NoError(t, err)

    book := &models.Book{
        LibraryID:       library.ID,
        Title:           "Test Book",
        TitleSource:     models.DataSourceFilepath,
        SortTitle:       "Test Book",
        SortTitleSource: models.DataSourceFilepath,
        AuthorSource:    models.DataSourceFilepath,
        Filepath:        bookDir,
    }
    _, err = db.NewInsert().Model(book).Exec(ctx)
    require.NoError(t, err)

    // Create PDF file with page count
    pageCount := 10
    file := &models.File{
        LibraryID:     library.ID,
        BookID:        book.ID,
        FileType:      models.FileTypePDF,
        FileRole:      models.FileRoleMain,
        Filepath:      filepath.Join(bookDir, "test.pdf"),
        FilesizeBytes: 1000,
        PageCount:     &pageCount,
    }
    _, err = db.NewInsert().Model(file).Exec(ctx)
    require.NoError(t, err)

    payload := map[string]int{"page": 0}
    body, _ := json.Marshal(payload)
    req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
    req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)
    c.SetParamNames("id")
    c.SetParamValues(strconv.Itoa(file.ID))

    err = h.updateFileCoverPage(c)
    // The handler should pass validation (not return "file type not supported")
    // but will fail at page extraction since pdfPageCache is nil.
    // We verify it's NOT a validation error about file type.
    require.Error(t, err)
    assert.NotContains(t, err.Error(), "does not support page-based covers")
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/robinjoseph/.worktrees/shisho/pdf-cover && go test ./pkg/books/ -run TestUpdateFileCoverPage/accepts_PDF_file_type -v`
Expected: FAIL — the handler returns "Cover page selection is only available for CBZ files"

- [ ] **Step 3: Update the handler to accept any file with pages**

In `pkg/books/handlers_cover_page.go`, make these changes:

Replace lines 55-58 (the CBZ-only validation):
```go
// Old
// Validate file type is CBZ
if file.FileType != models.FileTypeCBZ {
    return errcodes.ValidationError("Cover page selection is only available for CBZ files")
}
```

With:
```go
// New — validate file has pages (CBZ, PDF, or any future page-based format)
if file.PageCount == nil {
    return errcodes.ValidationError("This file does not support page-based covers")
}
```

Remove the existing `PageCount == nil` check from line 61 since it's now handled above. The bounds check becomes:
```go
// Validate page is within bounds
if payload.Page < 0 || payload.Page >= *file.PageCount {
    return errcodes.ValidationError("Page number is out of bounds")
}
```

Replace line 66 (the page extraction) which currently only uses `h.pageCache`:
```go
// Old
cachedPath, mimeType, err := h.pageCache.GetPage(file.Filepath, file.ID, payload.Page)
if err != nil {
    log.Error("failed to extract page from CBZ", logger.Data{"error": err.Error(), "page": payload.Page})
    return errcodes.ValidationError("Failed to extract page from CBZ file")
}
```

With file-type-aware cache routing:
```go
// New
var cachedPath, mimeType string
switch file.FileType {
case models.FileTypeCBZ:
    cachedPath, mimeType, err = h.pageCache.GetPage(file.Filepath, file.ID, payload.Page)
case models.FileTypePDF:
    cachedPath, mimeType, err = h.pdfPageCache.GetPage(file.Filepath, file.ID, payload.Page)
default:
    return errcodes.ValidationError("This file does not support page-based covers")
}
if err != nil {
    log.Error("failed to extract cover page", logger.Data{"error": err.Error(), "page": payload.Page, "file_type": file.FileType})
    return errcodes.ValidationError("Failed to extract page from file")
}
```

Update the log message on line 112:
```go
// Old
log.Info("set CBZ cover page", logger.Data{
// New
log.Info("set cover page", logger.Data{
```

Update the comment on line 19:
```go
// Old
// updateFileCoverPagePayload is the request body for setting a CBZ cover page.
// New
// updateFileCoverPagePayload is the request body for setting a cover page.
```

Update the comment on line 24:
```go
// Old
// updateFileCoverPage handles PUT /files/:id/cover-page
// Sets the cover page for a CBZ file and extracts it as an external cover image.
// New
// updateFileCoverPage handles PUT /files/:id/cover-page
// Sets the cover page for a page-based file (CBZ, PDF) and extracts it as an external cover image.
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/robinjoseph/.worktrees/shisho/pdf-cover && go test ./pkg/books/ -run TestUpdateFileCoverPage/accepts_PDF_file_type -v`
Expected: PASS

- [ ] **Step 5: Update the "returns 400 for non-CBZ file" test**

The existing test at line 299 creates an EPUB file (which has no page count) and expects rejection. Update the test name and assertion to match the new error message.

In `pkg/books/handlers_cover_page_test.go`, rename the test:
```go
// Old
t.Run("returns 400 for non-CBZ file", func(t *testing.T) {
// New
t.Run("returns 400 for file without pages", func(t *testing.T) {
```

The test body already creates an EPUB file without `PageCount`, so it will correctly hit the new `PageCount == nil` validation. No other changes needed — the test uses `require.Error(t, err)` which is sufficient.

- [ ] **Step 6: Run all cover page tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/pdf-cover && go test ./pkg/books/ -run TestUpdateFileCoverPage -v`
Expected: All tests PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/books/handlers_cover_page.go pkg/books/handlers_cover_page_test.go
git commit -m "[Backend] Support PDF cover page selection in updateFileCoverPage handler"
```

---

### Task 3: Update Frontend to Use `cover_page` as Discriminator

Replace all `file.file_type === FileTypeCBZ` checks in the cover section of FileEditDialog with `file.cover_page != null`.

**Files:**
- Modify: `app/components/library/FileEditDialog.tsx:650-796`

- [ ] **Step 1: Identify all CBZ file type checks in the cover section**

In `app/components/library/FileEditDialog.tsx`, the cover section (lines 650-796) has these checks to update:

1. Line 663: `file.file_type !== FileTypeCBZ` — controls showing upload-style cover preview
2. Line 689: `file.file_type === FileTypeCBZ` — controls showing page-style cover preview
3. Line 715: `file.file_type === FileTypeCBZ` — controls page number badge
4. Line 756: `file.file_type === FileTypeCBZ` — controls "Select page" button
5. Line 773: `file.file_type !== FileTypeCBZ` / line 774: `file.file_type === FileTypeCBZ` — unsaved indicator logic
6. Line 786: `file.file_type === FileTypeCBZ` — page picker dialog rendering

- [ ] **Step 2: Create a helper variable and update all checks**

At the start of the cover section rendering (or near the existing state variables), the component already has access to `file.cover_page`. Use it directly in conditionals.

Replace each check:

Line 663 — cover thumbnail for non-page files:
```tsx
// Old
{file.file_type !== FileTypeCBZ && (
// New
{file.cover_page == null && (
```

Line 689 — cover thumbnail for page-based files:
```tsx
// Old
{file.file_type === FileTypeCBZ && (
// New
{file.cover_page != null && (
```

Line 715-716 — page number badge:
```tsx
// Old
{file.file_type === FileTypeCBZ &&
  (pendingCoverPage ?? file.cover_page) != null && (
// New (cover_page is already non-null from outer condition, but pendingCoverPage may differ)
{file.cover_page != null &&
  (pendingCoverPage ?? file.cover_page) != null && (
```

Line 756 — "Select page" button:
```tsx
// Old
{file.file_type === FileTypeCBZ && (
// New
{file.cover_page != null && (
```

Lines 773-774 — unsaved indicator:
```tsx
// Old
{((file.file_type !== FileTypeCBZ && pendingCoverFile) ||
  (file.file_type === FileTypeCBZ &&
    pendingCoverPage !== null &&
    pendingCoverPage !== file.cover_page)) && (
// New
{((file.cover_page == null && pendingCoverFile) ||
  (file.cover_page != null &&
    pendingCoverPage !== null &&
    pendingCoverPage !== file.cover_page)) && (
```

Line 786 — page picker dialog:
```tsx
// Old
{file.file_type === FileTypeCBZ && file.page_count != null && (
  <CBZPagePicker
// New
{file.cover_page != null && file.page_count != null && (
  <PagePicker
```

- [ ] **Step 3: Remove unused `FileTypeCBZ` import if no longer needed**

Check if `FileTypeCBZ` is still used elsewhere in the file (e.g., line 569 for `canBeMainFile`). If it's still used, keep the import. If not, remove it.

Line 569 still uses `FileTypeCBZ`:
```tsx
const canBeMainFile = [FileTypeCBZ, FileTypeEPUB, FileTypeM4B].includes(
```

So keep the import. No change needed here.

- [ ] **Step 4: Run lint and unit tests**

Run: `pnpm lint:types && pnpm exec vitest run app/components/library/FileEditDialog.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add app/components/library/FileEditDialog.tsx
git commit -m "[Frontend] Use cover_page as discriminator for page-based cover workflow"
```

---

### Task 4: Run Full Checks

- [ ] **Step 1: Run mise check:quiet**

Run: `mise check:quiet`
Expected: All checks pass

- [ ] **Step 2: Commit any fixes if needed**

If any lint or test issues arise, fix and commit.
