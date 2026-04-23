# Cover image cache control — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace URL-based cache busting (`?t=...`) on cover images with HTTP-level conditional requests (`Cache-Control: private, no-cache` + `Last-Modified` → `If-Modified-Since` → `304`), and drop the frontend cache-buster prop cascade.

**Architecture:** Backend cover endpoints emit `Cache-Control: private, no-cache` and rely on Echo's `c.File()` (which wraps `http.ServeContent`) for automatic `Last-Modified` handling and `304 Not Modified` responses. The Kobo handler, which re-encodes on every request, adds manual `If-Modified-Since` short-circuiting so revalidated requests skip the resize work. Frontend drops every `?t=...` query param from cover URLs, removes `cacheBuster` props, and adds `key={bookQuery.dataUpdatedAt}` on `<img>` tags in pages that can mutate covers (so React remount drives HTTP revalidation).

**Tech Stack:** Go + Echo + `net/http` (backend), React + TanStack Query + Vitest (frontend), Caddy (prod reverse proxy, no changes).

**Reference spec:** `docs/superpowers/specs/2026-04-23-cover-cache-control-design.md`

---

## Before starting

Before touching any file, run the existing check suite to establish a green baseline:

```bash
mise check:quiet
```

Expected: all checks pass. If anything fails, investigate before starting — the plan assumes a clean starting state.

---

## Task 1: Add Cache-Control to `fileCover` (backend)

**Goal:** `GET /api/books/files/:id/cover` returns `Cache-Control: private, no-cache`. `Last-Modified` is automatic via `c.File()`. Conditional GETs return `304`.

**Files:**
- Modify: `pkg/books/handlers.go` — `fileCover` function at line 1269
- Create: `pkg/books/handlers_file_cover_cache_test.go`

---

- [ ] **Step 1.1: Write the failing tests**

Create `pkg/books/handlers_file_cover_cache_test.go`:

```go
package books

import (
	"context"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/shishobooks/shisho/pkg/models"
)

// seedBookWithFileCover creates a library, book, and file with a real cover
// image on disk. Returns the file ID and the on-disk cover path.
func seedBookWithFileCover(t *testing.T, ctx context.Context, db *bun.DB) (fileID int, coverPath string) {
	t.Helper()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	bookDir := filepath.Join(t.TempDir(), "Test Book")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

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

	filePath := filepath.Join(bookDir, "test.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0o644))

	coverFilename := "test.epub.cover.jpg"
	coverPath = filepath.Join(bookDir, coverFilename)
	coverFile, err := os.Create(coverPath)
	require.NoError(t, err)
	require.NoError(t, jpeg.Encode(coverFile, image.NewRGBA(image.Rect(0, 0, 10, 10)), nil))
	require.NoError(t, coverFile.Close())

	mimeType := "image/jpeg"
	file := &models.File{
		LibraryID:          library.ID,
		BookID:             book.ID,
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		Filepath:           filePath,
		FilesizeBytes:      1000,
		CoverImageFilename: &coverFilename,
		CoverMimeType:      &mimeType,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	return file.ID, coverPath
}

func TestFileCover_SetsCacheControlPrivateNoCache(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()
	e := echo.New()
	h := &handler{bookService: NewService(db)}

	fileID, _ := seedBookWithFileCover(t, ctx, db)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(fileID))

	require.NoError(t, h.fileCover(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
	assert.NotEmpty(t, rec.Header().Get("Last-Modified"))
	assert.NotEmpty(t, rec.Body.Bytes())
}

func TestFileCover_Returns304WhenIfModifiedSinceMatches(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()
	e := echo.New()
	h := &handler{bookService: NewService(db)}

	fileID, _ := seedBookWithFileCover(t, ctx, db)

	// First GET to capture Last-Modified.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetParamNames("id")
	c1.SetParamValues(strconv.Itoa(fileID))
	require.NoError(t, h.fileCover(c1))
	lastModified := rec1.Header().Get("Last-Modified")
	require.NotEmpty(t, lastModified)

	// Second GET with If-Modified-Since.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("If-Modified-Since", lastModified)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("id")
	c2.SetParamValues(strconv.Itoa(fileID))
	require.NoError(t, h.fileCover(c2))

	assert.Equal(t, http.StatusNotModified, rec2.Code)
	assert.Empty(t, rec2.Body.Bytes())
}
```

- [ ] **Step 1.2: Run tests and confirm they fail**

```bash
go test ./pkg/books/ -run TestFileCover_ -v
```

Expected: both tests FAIL. `TestFileCover_SetsCacheControlPrivateNoCache` fails because `Cache-Control` is empty. `TestFileCover_Returns304WhenIfModifiedSinceMatches` may already pass (Echo's `c.File()` handles the conditional GET on its own, even without setting `Cache-Control`), but one failing test is enough to move on.

- [ ] **Step 1.3: Implement — add Cache-Control header**

In `pkg/books/handlers.go`, modify the `fileCover` function. Find the block (around line 1296) that ends with:

```go
	coverPath := fileutils.ResolveCoverPath(file.Book.Filepath, coverFilename)

	return errors.WithStack(c.File(coverPath))
}
```

Change to:

```go
	coverPath := fileutils.ResolveCoverPath(file.Book.Filepath, coverFilename)

	c.Response().Header().Set("Cache-Control", "private, no-cache")
	return errors.WithStack(c.File(coverPath))
}
```

- [ ] **Step 1.4: Run tests and confirm they pass**

```bash
go test ./pkg/books/ -run TestFileCover_ -v
```

Expected: both tests PASS.

- [ ] **Step 1.5: Commit**

```bash
git add pkg/books/handlers.go pkg/books/handlers_file_cover_cache_test.go
git commit -m "[Backend] Set Cache-Control: private, no-cache on file cover endpoint"
```

---

## Task 2: Add Cache-Control to `bookCover` (backend)

**Goal:** Same behavior on `GET /api/books/:id/cover`.

**Files:**
- Modify: `pkg/books/handlers.go` — `bookCover` function at line 1466
- Create: `pkg/books/handlers_book_cover_cache_test.go`

---

- [ ] **Step 2.1: Write the failing tests**

Create `pkg/books/handlers_book_cover_cache_test.go`:

```go
package books

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
)

func TestBookCover_SetsCacheControlPrivateNoCache(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()
	e := echo.New()
	bookService := NewService(db)
	libraryService := libraries.NewService(db)
	h := &handler{bookService: bookService, libraryService: libraryService}

	fileID, _ := seedBookWithFileCover(t, ctx, db)

	// Look up the book ID via the seeded file.
	file, err := bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &fileID})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(file.BookID))

	require.NoError(t, h.bookCover(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
	assert.NotEmpty(t, rec.Header().Get("Last-Modified"))
	assert.NotEmpty(t, rec.Body.Bytes())
}

func TestBookCover_Returns304WhenIfModifiedSinceMatches(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()
	e := echo.New()
	bookService := NewService(db)
	libraryService := libraries.NewService(db)
	h := &handler{bookService: bookService, libraryService: libraryService}

	fileID, _ := seedBookWithFileCover(t, ctx, db)
	file, err := bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &fileID})
	require.NoError(t, err)

	// First GET.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetParamNames("id")
	c1.SetParamValues(strconv.Itoa(file.BookID))
	require.NoError(t, h.bookCover(c1))
	lastModified := rec1.Header().Get("Last-Modified")
	require.NotEmpty(t, lastModified)

	// Second GET with If-Modified-Since.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("If-Modified-Since", lastModified)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("id")
	c2.SetParamValues(strconv.Itoa(file.BookID))
	require.NoError(t, h.bookCover(c2))

	assert.Equal(t, http.StatusNotModified, rec2.Code)
	assert.Empty(t, rec2.Body.Bytes())
}
```

If `libraries.NewService` has a different signature in this codebase, match what `pkg/books/handlers_cover_page_test.go` does for handler setup (look for how it wires `libraryService`).

- [ ] **Step 2.2: Run and confirm failure**

```bash
go test ./pkg/books/ -run TestBookCover_ -v
```

Expected: `TestBookCover_SetsCacheControlPrivateNoCache` FAILS (no Cache-Control set).

- [ ] **Step 2.3: Implement**

In `pkg/books/handlers.go`, modify `bookCover` (line 1466). The function ends at line 1502 with:

```go
	coverPath := fileutils.ResolveCoverPath(book.Filepath, *coverFile.CoverImageFilename)
	return errors.WithStack(c.File(coverPath))
}
```

Change to:

```go
	coverPath := fileutils.ResolveCoverPath(book.Filepath, *coverFile.CoverImageFilename)
	c.Response().Header().Set("Cache-Control", "private, no-cache")
	return errors.WithStack(c.File(coverPath))
}
```

- [ ] **Step 2.4: Run tests and confirm pass**

```bash
go test ./pkg/books/ -run TestBookCover_ -v
```

Expected: PASS.

- [ ] **Step 2.5: Commit**

```bash
git add pkg/books/handlers.go pkg/books/handlers_book_cover_cache_test.go
git commit -m "[Backend] Set Cache-Control: private, no-cache on book cover endpoint"
```

---

## Task 3: Change Cache-Control on `seriesCover` (backend)

**Goal:** Replace the current `public, max-age=86400` with `private, no-cache` on `GET /api/series/:id/cover`.

**Files:**
- Modify: `pkg/series/handlers.go` — header set at line 276 inside `seriesCover` (func at line 231)
- Create: `pkg/series/handlers_cover_cache_test.go`

---

- [ ] **Step 3.1: Write the failing tests**

Create `pkg/series/handlers_cover_cache_test.go`. Pattern the test DB / seed helpers on `pkg/books/handlers_file_cover_cache_test.go` — create a library, a series, a book linked to the series via `book_series`, a file on disk under the book's directory, and a cover file next to it. The handler under test is `h.seriesCover(c)`.

Assert:
- First GET: status 200, `Cache-Control: private, no-cache`, `Last-Modified` non-empty, body non-empty.
- Second GET with `If-Modified-Since: <first Last-Modified>`: status 304, body empty.

If the `pkg/series` package doesn't already have a DB-backed handler test, mirror the setup in `pkg/books/handlers_cover_page_test.go` (which sets up an Echo context + a real SQLite DB via `setupTestDB(t)`). The `series` package uses `book_series` as the join table — insert a `models.BookSeries` row with `book_id`, `series_id`, and `series_number` to associate the book with the series.

- [ ] **Step 3.2: Run and confirm failure**

```bash
go test ./pkg/series/ -run TestSeriesCover_ -v
```

Expected: FAIL on `Cache-Control` assertion (currently `public, max-age=86400`).

- [ ] **Step 3.3: Implement**

In `pkg/series/handlers.go`, change the header set at line 276:

Before:

```go
	// Set appropriate headers
	c.Response().Header().Set("Cache-Control", "public, max-age=86400")

	return errors.WithStack(c.File(coverImagePath))
```

After:

```go
	// Set appropriate headers
	c.Response().Header().Set("Cache-Control", "private, no-cache")

	return errors.WithStack(c.File(coverImagePath))
```

- [ ] **Step 3.4: Run and confirm pass**

```bash
go test ./pkg/series/ -run TestSeriesCover_ -v
```

- [ ] **Step 3.5: Commit**

```bash
git add pkg/series/handlers.go pkg/series/handlers_cover_cache_test.go
git commit -m "[Backend] Switch series cover endpoint to Cache-Control: private, no-cache"
```

---

## Task 4: Add Cache-Control to eReader `Cover` (backend)

**Goal:** Same treatment for `pkg/ereader/handlers.go:692`.

**Files:**
- Modify: `pkg/ereader/handlers.go` — the `Cover` function ends at line 740 with `return errors.WithStack(c.File(coverPath))`
- Create or extend: `pkg/ereader/handlers_cover_cache_test.go`

---

- [ ] **Step 4.1: Write the failing tests**

Create `pkg/ereader/handlers_cover_cache_test.go`. Follow the same pattern as Tasks 1–3. The handler is `h.Cover`, and the route param is `bookId`. The eReader handler uses API key auth (`apiKey := GetAPIKeyFromContext(ctx)`) — review `pkg/ereader/middleware_test.go` or the existing `handlers_test.go` in that package to see how tests set up the API key context. Tests must:

1. Assert 200 with `Cache-Control: private, no-cache` + `Last-Modified`.
2. Assert 304 on `If-Modified-Since` match.

- [ ] **Step 4.2: Run and confirm failure**

```bash
go test ./pkg/ereader/ -run TestCover_ -v
```

- [ ] **Step 4.3: Implement**

In `pkg/ereader/handlers.go`, modify the `Cover` function (starts line 692). The final return is at line 740:

Before:

```go
	coverPath := fileutils.ResolveCoverPath(book.Filepath, *coverFile.CoverImageFilename)
	return errors.WithStack(c.File(coverPath))
}
```

After:

```go
	coverPath := fileutils.ResolveCoverPath(book.Filepath, *coverFile.CoverImageFilename)
	c.Response().Header().Set("Cache-Control", "private, no-cache")
	return errors.WithStack(c.File(coverPath))
}
```

- [ ] **Step 4.4: Run and confirm pass**

```bash
go test ./pkg/ereader/ -run TestCover_ -v
```

- [ ] **Step 4.5: Commit**

```bash
git add pkg/ereader/handlers.go pkg/ereader/handlers_cover_cache_test.go
git commit -m "[Backend] Set Cache-Control: private, no-cache on eReader cover endpoint"
```

---

## Task 5: Kobo `handleCover` — manual If-Modified-Since short-circuit (backend)

**Goal:** Kobo's resized-cover handler sets `Cache-Control: private, no-cache` and `Last-Modified` from the source cover's mtime. When the client sends `If-Modified-Since` with a timestamp ≥ the source mtime, the handler returns `304 Not Modified` without doing any image decode/resize/encode work. The no-resize branch (`c.File(coverPath)` when width or height is 0) gets automatic handling from Echo — no change needed there.

**Files:**
- Modify: `pkg/kobo/handlers.go` — `handleCover` function at line 225
- Create: `pkg/kobo/handlers_cover_cache_test.go`

---

- [ ] **Step 5.1: Write the failing tests**

Create `pkg/kobo/handlers_cover_cache_test.go`. Three tests:

1. `TestHandleCover_SetsCacheControlPrivateNoCache` — GET the resized endpoint (e.g. `/v1/books/<id>/thumbnail/100/150/`). Assert 200, `Cache-Control: private, no-cache`, `Last-Modified` non-empty, non-empty JPEG body.
2. `TestHandleCover_Returns304WhenIfModifiedSinceMatches` — Second GET with `If-Modified-Since` from the first. Assert 304, empty body.
3. `TestHandleCover_304SkipsResizeWork` — verify the resize does not execute when 304 is returned. Simplest approach: in the test, replace `coverPath` with a source cover file that would cause `image.Decode` to error (e.g. write garbage bytes). On the 200 path this would fail; on the 304 path it must return 304 before touching the file contents.

   Concretely:
   - Seed a book with `cover_image_filename` pointing at a valid JPEG.
   - First GET to get the `Last-Modified`.
   - Overwrite the cover file on disk with invalid bytes (non-JPEG) but preserve the mtime via `os.Chtimes`.
   - Second GET with `If-Modified-Since: <Last-Modified>`. Assert 304. If the handler tried to decode/resize, it would return an error — the 304 proves the decode path was skipped.

Mirror the test setup pattern used in existing `pkg/kobo/handlers_test.go`.

- [ ] **Step 5.2: Run and confirm failure**

```bash
go test ./pkg/kobo/ -run TestHandleCover_ -v
```

Expected: all three FAIL. Current handler writes `public, max-age=86400`, has no `Last-Modified`, and always runs the resize pipeline.

- [ ] **Step 5.3: Implement**

In `pkg/kobo/handlers.go`, modify `handleCover` (starts line 225). Locate the existing resize block starting at line 250 (`// Parse requested dimensions`) and ending at line 287 (the `jpeg.Encode` return).

Add a block immediately before the existing `widthStr := c.Param("w")` line that:
1. Stats the cover file to get mtime.
2. Sets `Cache-Control: private, no-cache` and `Last-Modified`.
3. Checks `If-Modified-Since` and short-circuits with 304 before any image work.

Ensure `net/http` and `time` are already imported (they are — file already uses `http`).

Before:

```go
	coverPath := fileutils.ResolveCoverPath(book.Filepath, *file.CoverImageFilename)

	// Parse requested dimensions
	widthStr := c.Param("w")
	heightStr := c.Param("h")
	width, _ := strconv.Atoi(widthStr)
	height, _ := strconv.Atoi(heightStr)

	if width == 0 || height == 0 {
		// Serve original if dimensions not specified
		return c.File(coverPath)
	}

	// Open and resize the image
	imgFile, err := os.Open(coverPath)
	if err != nil {
		return errcodes.NotFound("Cover")
	}
	defer imgFile.Close()
	// ... (decode, resize, encode) ...

	c.Response().Header().Set("Content-Type", "image/jpeg")
	c.Response().Header().Set("Cache-Control", "public, max-age=86400")
	c.Response().WriteHeader(http.StatusOK)
	return jpeg.Encode(c.Response().Writer, dstImg, &jpeg.Options{Quality: 80})
```

After:

```go
	coverPath := fileutils.ResolveCoverPath(book.Filepath, *file.CoverImageFilename)

	// Stat source cover for Last-Modified + conditional GET short-circuit.
	// This runs before the resize so revalidated requests skip the expensive
	// decode/resize/encode work.
	coverStat, err := os.Stat(coverPath)
	if err != nil {
		return errcodes.NotFound("Cover")
	}
	modTime := coverStat.ModTime().UTC().Truncate(time.Second)
	c.Response().Header().Set("Cache-Control", "private, no-cache")
	c.Response().Header().Set("Last-Modified", modTime.Format(http.TimeFormat))
	if ims := c.Request().Header.Get("If-Modified-Since"); ims != "" {
		if t, parseErr := http.ParseTime(ims); parseErr == nil && !modTime.After(t) {
			c.Response().WriteHeader(http.StatusNotModified)
			return nil
		}
	}

	// Parse requested dimensions
	widthStr := c.Param("w")
	heightStr := c.Param("h")
	width, _ := strconv.Atoi(widthStr)
	height, _ := strconv.Atoi(heightStr)

	if width == 0 || height == 0 {
		// Serve original if dimensions not specified. c.File handles
		// Last-Modified/If-Modified-Since automatically for this branch.
		return c.File(coverPath)
	}

	// Open and resize the image
	imgFile, err := os.Open(coverPath)
	if err != nil {
		return errcodes.NotFound("Cover")
	}
	defer imgFile.Close()
	// ... (existing decode, resize, encode — unchanged) ...

	c.Response().Header().Set("Content-Type", "image/jpeg")
	c.Response().WriteHeader(http.StatusOK)
	return jpeg.Encode(c.Response().Writer, dstImg, &jpeg.Options{Quality: 80})
```

Key edits:
- Add the stat + conditional GET block right after `coverPath := ...`.
- Remove the old `c.Response().Header().Set("Cache-Control", "public, max-age=86400")` line immediately above `c.Response().WriteHeader(http.StatusOK)`.
- Ensure `"time"` is in the import block; add it if missing.

Note: when `width == 0 || height == 0`, `c.File(coverPath)` handles Last-Modified on its own, but we've already set our own `Cache-Control` and `Last-Modified` above. Both are harmless; Echo's `c.File` won't overwrite a `Cache-Control` already on the response.

- [ ] **Step 5.4: Run and confirm pass**

```bash
go test ./pkg/kobo/ -run TestHandleCover_ -v
```

- [ ] **Step 5.5: Commit**

```bash
git add pkg/kobo/handlers.go pkg/kobo/handlers_cover_cache_test.go
git commit -m "[Backend] Add If-Modified-Since short-circuit to Kobo cover handler"
```

---

## Task 6: Update `CoverGalleryTabs.test.tsx` for stable URLs (frontend)

**Goal:** TDD red — update existing tests to assert on stable URLs and a key-driven remount pattern. They will fail until Task 7 is done.

**Files:**
- Modify: `app/components/library/CoverGalleryTabs.test.tsx`

---

- [ ] **Step 6.1: Replace the test body**

Current test at lines 24-58 asserts `?t=111` and `?t=222` URLs and exercises an error-then-cacheBuster-bump remount. The new behavior: URL is stable, remount is driven by a `cacheKey` prop that changes.

Replace the file's test contents (keep imports and `makeFile` helper) with:

```tsx
describe("CoverGalleryTabs", () => {
  it("renders cover image with stable URL (no cache-buster query)", () => {
    const files = [
      makeFile({ id: 1, file_type: "epub", filepath: "/library/a.epub" }),
      makeFile({ id: 2, file_type: "m4b", filepath: "/library/b.m4b" }),
    ];

    const { container } = render(<CoverGalleryTabs files={files} />);

    const img = container.querySelector("img");
    expect(img).not.toBeNull();
    expect(img?.getAttribute("src")).toBe("/api/books/files/1/cover");
  });

  it("re-mounts cover image when cacheKey changes", () => {
    const files = [
      makeFile({ id: 1, file_type: "epub", filepath: "/library/a.epub" }),
      makeFile({ id: 2, file_type: "m4b", filepath: "/library/b.m4b" }),
    ];

    const { container, rerender } = render(
      <CoverGalleryTabs cacheKey={111} files={files} />,
    );
    const firstImg = container.querySelector("img");
    expect(firstImg).not.toBeNull();
    expect(firstImg?.getAttribute("src")).toBe("/api/books/files/1/cover");

    // Simulate an error first so the img is unmounted, then a cacheKey bump
    // should cause it to re-mount (simulating a "retry" after the cover was
    // fixed on disk — equivalent to the old cacheBuster-driven retry flow).
    fireEvent.error(firstImg!);
    expect(container.querySelector("img")).toBeNull();

    rerender(<CoverGalleryTabs cacheKey={222} files={files} />);
    const secondImg = container.querySelector("img");
    expect(secondImg).not.toBeNull();
    expect(secondImg?.getAttribute("src")).toBe("/api/books/files/1/cover");
  });
});
```

- [ ] **Step 6.2: Run and confirm failure**

```bash
pnpm test -- app/components/library/CoverGalleryTabs.test.tsx
```

Expected: FAIL — `cacheKey` prop doesn't exist yet; tests assert `src` exactly `/api/books/files/1/cover` but current code produces `/api/books/files/1/cover` only when `cacheBuster` is undefined (actually passes the first test), and the rerender path still requires new prop.

- [ ] **Step 6.3: Commit (tests only, still red)**

```bash
git add app/components/library/CoverGalleryTabs.test.tsx
git commit -m "[Test] Update CoverGalleryTabs tests for stable cover URLs"
```

---

## Task 7: Update `CoverGalleryTabs.tsx` — drop cacheBuster, add cacheKey

**Goal:** Component uses a stable URL and accepts a `cacheKey` prop used as the `<img key>` for remounting. Makes Task 6's tests pass.

**Files:**
- Modify: `app/components/library/CoverGalleryTabs.tsx`

---

- [ ] **Step 7.1: Rewrite the component's props and URL**

Change the props interface (line 14-18) from:

```tsx
interface CoverGalleryTabsProps {
  files: File[];
  className?: string;
  cacheBuster?: number;
}
```

to:

```tsx
interface CoverGalleryTabsProps {
  files: File[];
  className?: string;
  /**
   * Forces the <img> to remount (and thus re-request) when this value changes.
   * Typically set to a React Query `dataUpdatedAt` so cover updates triggered
   * by mutations on this page flow through HTTP revalidation.
   */
  cacheKey?: number;
}
```

Change the destructuring at lines 59-63:

```tsx
function CoverGalleryTabs({
  files,
  className,
  cacheKey,
}: CoverGalleryTabsProps) {
```

Replace the `coverUrl` construction at lines 83-87:

```tsx
  const coverUrl = selectedFile
    ? `/api/books/files/${selectedFile.id}/cover`
    : null;
```

On the `<img>` at line 146, add a `key` prop:

```tsx
        {hasCover && coverUrl && (
          <img
            alt={`${selectedFile?.name || "File"} Cover`}
            className={cn(
              "absolute inset-0 w-full h-full object-cover",
              !coverLoaded && "opacity-0",
            )}
            key={`${selectedFile?.id}-${cacheKey ?? 0}`}
            onError={() => setCoverError(true)}
            onLoad={handleCoverLoad}
            src={coverUrl}
          />
        )}
```

Note the key combines `selectedFile.id` (tab switch drives remount) and `cacheKey` (external bump drives remount). The existing `useEffect` at line 106-108 that resets `coverError` on `coverUrl` change can stay, but update its dependency from `[coverUrl]` to `[coverUrl, cacheKey]` so error state also resets when the key bumps:

```tsx
  useEffect(() => {
    setCoverError(false);
  }, [coverUrl, cacheKey]);
```

- [ ] **Step 7.2: Run tests and confirm pass**

```bash
pnpm test -- app/components/library/CoverGalleryTabs.test.tsx
```

Expected: PASS.

- [ ] **Step 7.3: Commit**

```bash
git add app/components/library/CoverGalleryTabs.tsx
git commit -m "[Frontend] Stable cover URL + cacheKey remount prop on CoverGalleryTabs"
```

---

## Task 8: Update `FileCoverThumbnail.tsx` — drop cacheBuster, add cacheKey

**Goal:** Same transform as Task 7 for the thumbnail component.

**Files:**
- Modify: `app/components/library/FileCoverThumbnail.tsx`

---

- [ ] **Step 8.1: Rewrite the component**

Replace the whole file contents with:

```tsx
import { useState } from "react";

import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { cn } from "@/libraries/utils";
import type { File } from "@/types";

interface FileCoverThumbnailProps {
  file: File;
  className?: string;
  onClick?: () => void;
  /**
   * Forces the <img> to remount (and thus re-request) when this value changes.
   * Typically a React Query `dataUpdatedAt` from a mutation-aware query.
   */
  cacheKey?: number;
}

/**
 * Small cover thumbnail for a file.
 * Shows the file's cover image or a placeholder based on file type.
 * Aspect ratio: 2:3 for EPUB/CBZ, square for M4B.
 */
function FileCoverThumbnail({
  file,
  className,
  onClick,
  cacheKey,
}: FileCoverThumbnailProps) {
  const [imageLoaded, setImageLoaded] = useState(false);
  const [imageError, setImageError] = useState(false);

  const isAudiobook = file.file_type === "m4b";
  const aspectClass = isAudiobook ? "aspect-square" : "aspect-[2/3]";
  const placeholderVariant = isAudiobook ? "audiobook" : "book";

  const hasCover = file.cover_image_filename && !imageError;
  const coverUrl = `/api/books/files/${file.id}/cover`;

  return (
    <div
      className={cn(
        "relative overflow-hidden rounded border border-border shrink-0 cursor-pointer",
        "transition-all duration-200 hover:scale-105 hover:shadow-md",
        aspectClass,
        className,
      )}
      onClick={onClick}
    >
      {(!imageLoaded || !hasCover) && (
        <CoverPlaceholder
          className="absolute inset-0"
          variant={placeholderVariant}
        />
      )}

      {hasCover && (
        <img
          alt=""
          className={cn(
            "absolute inset-0 w-full h-full object-cover",
            !imageLoaded && "opacity-0",
          )}
          key={`${file.id}-${cacheKey ?? 0}`}
          onError={() => setImageError(true)}
          onLoad={() => setImageLoaded(true)}
          src={coverUrl}
        />
      )}
    </div>
  );
}

export default FileCoverThumbnail;
```

- [ ] **Step 8.2: Check type errors in callers**

```bash
pnpm lint:types
```

Expected: errors wherever `FileCoverThumbnail` is called with `cacheBuster={...}` — those are fixed in the next tasks. If any caller passes a `cacheBuster` prop, that's a type error now.

- [ ] **Step 8.3: Commit**

```bash
git add app/components/library/FileCoverThumbnail.tsx
git commit -m "[Frontend] Stable cover URL + cacheKey remount prop on FileCoverThumbnail"
```

(The commit may temporarily break type-checking in files that pass `cacheBuster`. Those are fixed in the next tasks. This is acceptable as a mid-sequence state; all callers are fixed before the final commit.)

---

## Task 9: Update `FileEditDialog.tsx` — stable URLs + key on inline imgs + rename prop

**Goal:** The two inline `<img>` tags at lines 753 and 780 drop their `?t=` query params and get a `key` for remount. Any `FileCoverThumbnail`/`CoverGalleryTabs` invocations inside this file pass `cacheKey` instead of `cacheBuster`.

**Files:**
- Modify: `app/components/library/FileEditDialog.tsx`

---

- [ ] **Step 9.1: Find the caching-related passes and update them**

In `FileEditDialog.tsx`:

1. Read the file to locate `coverCacheBuster` (or whatever local variable drives the current `?t=` calls). Search for `?t=` — you should find two hits at lines 753 and 780.

2. For each of the two inline `<img>` tags, change:

```tsx
<img
  ...
  src={`/api/books/files/${file.id}/cover?t=${coverCacheBuster}`}
/>
```

to:

```tsx
<img
  ...
  key={`${file.id}-${coverCacheBuster}`}
  src={`/api/books/files/${file.id}/cover`}
/>
```

3. If `coverCacheBuster` is a prop passed into `FileEditDialog`, rename it to `cacheKey` throughout the file — prop interface, destructuring, and uses. If it's a local variable derived from a query's `dataUpdatedAt`, leave the variable name alone but ensure it is used only as a `key`, never in the URL.

4. If this file renders `<FileCoverThumbnail cacheBuster={...}>` or `<CoverGalleryTabs cacheBuster={...}>`, rename those prop names to `cacheKey`.

- [ ] **Step 9.2: Verify no `?t=` remains in this file**

```bash
grep -n '?t=' app/components/library/FileEditDialog.tsx
```

Expected: no output.

- [ ] **Step 9.3: Type-check**

```bash
pnpm lint:types
```

Expected: no errors reported from `FileEditDialog.tsx`. If there are errors in other files (e.g., `BookDetail.tsx` passing `cacheBuster` to this dialog), those are fixed in Task 10.

- [ ] **Step 9.4: Commit**

```bash
git add app/components/library/FileEditDialog.tsx
git commit -m "[Frontend] Stable cover URLs + key-driven remount in FileEditDialog"
```

---

## Task 10: Update `BookDetail.tsx` — drop coverCacheBuster from URLs, keep as key source

**Goal:** `BookDetail` uses stable cover URLs. The existing `coverCacheBuster` constant (`bookQuery.dataUpdatedAt`) is repurposed as a React `key` on the main cover `<img>` and is passed down as `cacheKey` to `CoverGalleryTabs` and `FileCoverThumbnail`.

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

---

- [ ] **Step 10.1: Drop the cache-buster from the main cover URL**

Find the line defining the main cover URL (currently at line 811-813):

```tsx
const coverCacheBuster = bookQuery.dataUpdatedAt;
...
  ? `/api/books/${bookQuery.data.id}/cover?t=${coverCacheBuster}`
```

Replace the URL (line 813) with:

```tsx
  ? `/api/books/${bookQuery.data.id}/cover`
```

Leave the `coverCacheBuster` constant alone — it's still used as a `key` source.

- [ ] **Step 10.2: Add `key` to the main cover `<img>`**

Find the `<img>` element that uses the above `src` (search for the line that renders the main cover image around line 815+). Add:

```tsx
key={coverCacheBuster}
```

- [ ] **Step 10.3: Rename prop passes from `cacheBuster` to `cacheKey`**

There are four existing call sites at lines 238, 1073, 1355, 1406 passing `cacheBuster={coverCacheBuster}` to child components. Change each to:

```tsx
cacheKey={coverCacheBuster}
```

- [ ] **Step 10.4: Verify**

```bash
grep -n '?t=' app/components/pages/BookDetail.tsx
grep -n 'cacheBuster' app/components/pages/BookDetail.tsx
```

Expected: no output from either.

- [ ] **Step 10.5: Type-check and run tests**

```bash
pnpm lint:types
pnpm test -- app/components
```

Expected: no type errors, all component tests pass.

- [ ] **Step 10.6: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "[Frontend] Stable cover URL + cacheKey plumbing in BookDetail"
```

---

## Task 11: Update `GlobalSearch.tsx` — drop cacheBuster prop on `SearchResultCover`

**Goal:** `SearchResultCover` uses stable URL. No `cacheBuster` prop. No key needed (search dropdown doesn't mutate covers; new searches remount naturally).

**Files:**
- Modify: `app/components/library/GlobalSearch.tsx`

---

- [ ] **Step 11.1: Update the component**

In `GlobalSearch.tsx`, change `SearchResultCoverProps` (lines 47-53) to remove `cacheBuster`:

```tsx
interface SearchResultCoverProps {
  type: "book" | "series";
  id: number;
  thumbnailClasses: string;
  variant: "book" | "audiobook";
}
```

Update `SearchResultCover` destructuring (lines 55-61):

```tsx
const SearchResultCover = ({
  type,
  id,
  thumbnailClasses,
  variant,
}: SearchResultCoverProps) => {
```

Update `coverUrl` (line 62):

```tsx
const coverUrl = `/api/${type === "book" ? "books" : "series"}/${id}/cover`;
```

Update the two call sites at lines 333 and 377 to remove the `cacheBuster={searchQuery.dataUpdatedAt}` prop.

- [ ] **Step 11.2: Verify**

```bash
grep -n '?t=\|cacheBuster' app/components/library/GlobalSearch.tsx
```

Expected: no output.

- [ ] **Step 11.3: Type-check**

```bash
pnpm lint:types
```

- [ ] **Step 11.4: Commit**

```bash
git add app/components/library/GlobalSearch.tsx
git commit -m "[Frontend] Stable cover URLs in GlobalSearch; drop cacheBuster prop"
```

---

## Task 12: Update `BookItem.tsx` — drop `?t=`

**Files:**
- Modify: `app/components/library/BookItem.tsx`

---

- [ ] **Step 12.1: Change the URL**

Find line 154:

```tsx
const coverUrl = `/api/books/${book.id}/cover?t=${new Date(book.updated_at).getTime()}`;
```

Change to:

```tsx
const coverUrl = `/api/books/${book.id}/cover`;
```

No key needed — BookItem is a list-grid item that doesn't mutate covers from its own view.

- [ ] **Step 12.2: Verify and commit**

```bash
grep -n '?t=' app/components/library/BookItem.tsx
```

Expected: no output.

```bash
git add app/components/library/BookItem.tsx
git commit -m "[Frontend] Stable cover URL in BookItem"
```

---

## Task 13: Update `SeriesList.tsx` — drop `?t=`

**Files:**
- Modify: `app/components/pages/SeriesList.tsx`

---

- [ ] **Step 13.1: Change the URL**

Find line 53:

```tsx
const coverUrl = `/api/series/${seriesItem.id}/cover?t=${new Date(seriesItem.updated_at).getTime()}`;
```

Change to:

```tsx
const coverUrl = `/api/series/${seriesItem.id}/cover`;
```

- [ ] **Step 13.2: Verify and commit**

```bash
grep -n '?t=' app/components/pages/SeriesList.tsx
git add app/components/pages/SeriesList.tsx
git commit -m "[Frontend] Stable cover URL in SeriesList"
```

---

## Task 14: Update `IdentifyReviewForm.tsx` — drop `?t=`

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx`

---

- [ ] **Step 14.1: Change the URL**

Find line 616-618:

```tsx
const currentCoverUrl = file?.cover_image_filename
  ? `/api/books/files/${file.id}/cover?t=${new Date(file.updated_at).getTime()}`
  : undefined;
```

Change to:

```tsx
const currentCoverUrl = file?.cover_image_filename
  ? `/api/books/files/${file.id}/cover`
  : undefined;
```

No key needed — the form is displayed during an identify flow; once accepted, the user navigates away.

- [ ] **Step 14.2: Verify and commit**

```bash
grep -n '?t=' app/components/library/IdentifyReviewForm.tsx
git add app/components/library/IdentifyReviewForm.tsx
git commit -m "[Frontend] Stable cover URL in IdentifyReviewForm"
```

---

## Task 15: Update root `CLAUDE.md` — Critical Gotchas

**Files:**
- Modify: `CLAUDE.md` (lines 86-92)

---

- [ ] **Step 15.1: Replace the section**

Current text (lines 86-92):

```markdown
### Frontend

**Cover images need cache busting** — All cover image URLs must include a `?t=` parameter to ensure updated covers display without caching issues:
```tsx
const coverUrl = `/api/books/${id}/cover?t=${query.dataUpdatedAt}`;
```
```

Replace with:

```markdown
### Frontend

**Cover images use HTTP revalidation, not URL cache-busting** — Cover endpoints set `Cache-Control: private, no-cache` and rely on `Last-Modified` for revalidation. Never append `?t=...` query params to cover URLs. For pages that can trigger cover mutations, add `key={query.dataUpdatedAt}` to the `<img>` so React remounts it and the browser refetches:

```tsx
<img key={bookQuery.dataUpdatedAt} src={`/api/books/${id}/cover`} />
```
```

- [ ] **Step 15.2: Commit**

```bash
git add CLAUDE.md
git commit -m "[Docs] Update root CLAUDE.md for cover HTTP revalidation"
```

---

## Task 16: Update `app/CLAUDE.md` — Cover Image Freshness section

**Files:**
- Modify: `app/CLAUDE.md` (lines 296-340 — the "Cover Image Cache Busting" section)

---

- [ ] **Step 16.1: Replace the section**

Delete the existing section from line 296 to line 340 (the section starting `## Cover Image Cache Busting` through the end of the "Checklist for New Cover Components" list). Replace with:

```markdown
## Cover Image Freshness

Cover endpoints set `Cache-Control: private, no-cache` and emit `Last-Modified` from the cover file's mtime. The browser stores the image but revalidates with the server on every `<img>` mount via a conditional `GET` carrying `If-Modified-Since`. The server returns `304 Not Modified` (no body) when the cover is unchanged, or `200 OK` with new bytes when it has changed.

### Rules

- **Do not append `?t=...`** or any other cache-busting query parameter to cover URLs. Use stable URLs:
  ```tsx
  const coverUrl = `/api/books/${book.id}/cover`;
  ```
- **For pages that can trigger a cover mutation** (e.g., BookDetail, FileEditDialog), add `key={query.dataUpdatedAt}` to the `<img>` tag. A query invalidation bumps `dataUpdatedAt`, the key changes, React remounts the element, and the browser makes a fresh HTTP request that revalidates:
  ```tsx
  <img
    key={bookQuery.dataUpdatedAt}
    src={`/api/books/${book.id}/cover`}
  />
  ```
- **For pages that only display covers** (lists, grids, search results), no `key` is needed. Navigation naturally remounts the `<img>`, which triggers revalidation.
- **For child components that render covers inside editing surfaces**, pass a `cacheKey` prop (typed as `number`) down and use it as part of the `<img key>`. Parents should pass `bookQuery.dataUpdatedAt`.

### Endpoints

| Endpoint | Description |
|----------|-------------|
| `/api/books/{id}/cover` | Book cover (selected from files) |
| `/api/books/files/{id}/cover` | Specific file's cover |
| `/api/series/{id}/cover` | Series cover (from first book) |

### Checklist for new cover components

- [ ] Cover URL does NOT include `?t=` query param
- [ ] For pages that mutate covers, `<img>` has `key={query.dataUpdatedAt}` (or is driven by a `cacheKey` prop) so remount triggers revalidation
- [ ] Cover-mutating mutations invalidate the query whose `dataUpdatedAt` drives the key (default: `RetrieveBook`, already invalidated by `useUploadFileCover` and `useSetFileCoverPage`)
```

- [ ] **Step 16.2: Commit**

```bash
git add app/CLAUDE.md
git commit -m "[Docs] Rewrite cover freshness section in app/CLAUDE.md"
```

---

## Task 17: Full verification pass

**Goal:** Confirm everything compiles, lints, tests green, and covers behave correctly end-to-end.

---

- [ ] **Step 17.1: Run full check suite**

```bash
mise check:quiet
```

Expected: all checks pass. If anything fails, fix the underlying issue before proceeding.

- [ ] **Step 17.2: Grep for any remaining cache-buster references**

```bash
grep -rn '?t=' app --include='*.tsx' --include='*.ts' | grep -v node_modules | grep -iv '\.test\.' | grep -i cover
grep -rn 'cacheBuster' app --include='*.tsx' --include='*.ts' | grep -v node_modules
```

Expected: no results from either command (test files may still reference the old pattern if any lingered — investigate if so).

- [ ] **Step 17.3: Manual browser verification**

Start the dev environment:

```bash
mise start
```

In the browser:
1. Open a book detail page. Open DevTools → Network tab.
2. Observe the initial cover request: `200 OK`, response headers include `Cache-Control: private, no-cache` and `Last-Modified`.
3. Reload the page. Observe the cover request: `304 Not Modified` with a small response body (few hundred bytes).
4. Upload a new cover from the book detail page. Observe the cover visibly update on the page without a full reload.
5. Navigate to the series list page that contains this book's series. Observe the series cover shows the new book cover (if the book drives the series cover).
6. Navigate to the series detail page. Observe the book's cover in the books-in-series list is the new one.
7. Open global search, type the book title, observe the thumbnail in the dropdown is the new cover.

- [ ] **Step 17.4: Verify Caddy prod-mode behavior (optional but recommended)**

If feasible, build and run against Caddy locally:

```bash
mise build
# Then run the built binary and point Caddy at it per project docs.
```

Repeat Step 17.3 items 1-3 through Caddy. Confirm `Cache-Control`, `Last-Modified`, and `304` pass through untouched. Caddy adds no additional caching; these headers are transparent.

- [ ] **Step 17.5: Final commit (if any fixups were needed)**

If steps 17.1-17.3 uncovered issues and you made fixup commits, that's fine. If the tree is clean at the end, nothing more to commit.

---

## Self-review

**Spec coverage check:**

- Backend `Cache-Control` on `bookCover` ✓ (Task 2)
- Backend `Cache-Control` on `fileCover` ✓ (Task 1)
- Backend `Cache-Control` on `seriesCover` ✓ (Task 3)
- Backend `Cache-Control` on eReader `Cover` ✓ (Task 4)
- Kobo manual 304 handling ✓ (Task 5)
- Kobo test that 304 skips resize work ✓ (Task 5 step 5.1 test #3)
- `Last-Modified` via `c.File()` — implicit, no action needed ✓
- No `ETag` — covered by not adding one ✓
- No backend scan-path / `book.updated_at` bumping — explicitly out of scope, no tasks for it ✓
- Frontend stable URLs (9 call sites) ✓ (Tasks 7-14)
- Remove `cacheBuster` prop cascade ✓ (Tasks 7, 8, 9, 10, 11)
- `key={dataUpdatedAt}` on pages that mutate covers ✓ (Tasks 7, 8, 9, 10)
- No `SeriesBooks` invalidation added — confirmed unnecessary ✓
- `coverCache` utility kept as-is ✓ (no task touches it)
- Caddy no-op ✓ (Task 17 step 17.4 verifies)
- Dev mode no-op ✓ (Task 17 step 17.3 verifies)
- Test coverage: 2 tests per endpoint backend + 1 Kobo extra + updated CoverGalleryTabs tests ✓
- `CLAUDE.md` root update ✓ (Task 15)
- `app/CLAUDE.md` rewrite ✓ (Task 16)
- `CHANGELOG.md` — no retroactive edits; new entry lands with release flow ✓

**Placeholder scan:** No `TBD`, `TODO`, `FIXME`, or `"fill in later"` in any task. All test bodies and code snippets are concrete.

**Type consistency:** The new prop name `cacheKey` is used consistently across `CoverGalleryTabs`, `FileCoverThumbnail`, and their callers (`BookDetail`, `FileEditDialog`). The `<img key>` pattern is `${id}-${cacheKey ?? 0}` across both components. `GlobalSearch` doesn't need the prop and the prop removal is clean.

No spec requirement without a corresponding task.
