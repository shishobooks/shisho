# Cache Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a new admin page at `/settings/cache` that shows the size and file count of each server cache (downloads, CBZ pages, PDF pages) and lets users with `config:write` clear any of them individually via a confirm dialog.

**Architecture:** Each of the three existing cache types (`pkg/downloadcache`, `pkg/cbzpages`, `pkg/pdfpages`) gains a `SizeBytes()` method returning `(int64, int, error)` and a `Clear()` method. A new `pkg/cache` package exposes `GET /api/cache` and `POST /api/cache/:id/clear`, dispatching to the right cache by id via a `switch`. The frontend gets a new `/settings/cache` route with three cards and an `AlertDialog` confirm flow.

**Tech Stack:** Go (Echo, Bun, testify), TypeScript/React (Tanstack Query, shadcn/radix AlertDialog, lucide icons), Playwright (not used here), mise tasks.

**Prerequisites for all agents:** Check the project's root `CLAUDE.md` and any relevant subdirectory `CLAUDE.md` files for rules that apply to your work. These contain critical project conventions, gotchas, and requirements (e.g. docs update requirements, testing conventions, naming rules). Violations are review failures.

---

## File Structure

**Go (backend):**
- Modify: `pkg/downloadcache/cache.go` — add `SizeBytes()` and `Clear()` methods.
- Modify: `pkg/downloadcache/cache_test.go` — new tests.
- Modify: `pkg/cbzpages/cache.go` — add `SizeBytes()` and `Clear()` methods and a `rootDir()` helper.
- Modify: `pkg/cbzpages/cache_test.go` — new tests (if file doesn't exist, create it).
- Modify: `pkg/pdfpages/cache.go` — add `SizeBytes()` and `Clear()` methods and a `rootDir()` helper.
- Modify: `pkg/pdfpages/cache_test.go` — new tests.
- Create: `pkg/cache/handlers.go` — HTTP handlers.
- Create: `pkg/cache/routes.go` — route registration.
- Create: `pkg/cache/handlers_test.go` — handler tests.
- Modify: `cmd/api/main.go` — construct `cbzCache` and `pdfCache` alongside `dlCache`, pass into `server.New`.
- Modify: `pkg/server/server.go` — accept and wire the two new caches; register the new route group; remove the now-duplicate local instantiation.

**TypeScript (frontend):**
- Modify: `app/router.tsx` — register `/settings/cache` route.
- Modify: `app/components/pages/useAdminNavItems.ts` — add Cache nav item.
- Create: `app/components/pages/AdminCache.tsx` — the new page.
- Create: `app/hooks/queries/cache.ts` — React Query hooks.

**Docs:**
- Modify: `website/docs/configuration.md` — cross-link to the new page from the Storage section.
- Create: `website/docs/cache-management.md` — describes each cache and the admin UI.
- Modify: `website/sidebars.ts` (or wherever docs sidebar is defined) — add entry for the new page.

---

## Conventions (read before starting)

- **Always run `mise tygo` after modifying any Go type that's exposed to the frontend.** The `app/types/generated/` directory is gitignored — don't try to `git add` it.
- **JSON fields are snake_case**, including Go struct tags.
- **Tests must use `t.Parallel()`** unless they share global state.
- **Follow TDD (Red → Green → Refactor):** write the failing test, run it to confirm it fails, write the minimal fix, confirm it passes.
- **Commits:** `[Backend] ...`, `[Frontend] ...`, `[Docs] ...`, `[Test] ...` as appropriate.
- **Final verification:** `mise check:quiet` must pass before the last commit.

---

## Task 1: Add `SizeBytes` and `Clear` to `downloadcache.Cache`

**Files:**
- Modify: `pkg/downloadcache/cache.go`
- Modify: `pkg/downloadcache/cache_test.go`

The download cache root is the directory passed into `NewCache` (e.g. `/config/cache/downloads`). It contains individual cached files (one file + `.meta.json` per entry) plus a `bulk/` subdirectory with bulk-zip outputs. `SizeBytes` must sum both. `Clear` must remove all children of the root but preserve the root directory itself.

- [ ] **Step 1: Write failing tests**

Add to `pkg/downloadcache/cache_test.go`:

```go
func TestCache_SizeBytes_Empty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := NewCache(dir, 1<<30)

	bytes, count, err := c.SizeBytes()
	require.NoError(t, err)
	assert.Equal(t, int64(0), bytes)
	assert.Equal(t, 0, count)
}

func TestCache_SizeBytes_MissingRoot(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	c := NewCache(dir, 1<<30)

	bytes, count, err := c.SizeBytes()
	require.NoError(t, err)
	assert.Equal(t, int64(0), bytes)
	assert.Equal(t, 0, count)
}

func TestCache_SizeBytes_CountsIndividualAndBulk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := NewCache(dir, 1<<30)

	// Individual cached file: data + .meta.json
	require.NoError(t, os.WriteFile(filepath.Join(dir, "1.epub"), []byte("hello"), 0644))
	meta := CacheMetadata{FileID: 1, SizeBytes: 5, LastAccessedAt: time.Now()}
	metaBytes, err := json.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "1.epub.meta.json"), metaBytes, 0644))

	// Bulk zip
	bulkDir := filepath.Join(dir, "bulk")
	require.NoError(t, os.MkdirAll(bulkDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bulkDir, "abc.zip"), []byte("zipdata!!"), 0644))

	bytes, count, err := c.SizeBytes()
	require.NoError(t, err)
	// 5 bytes from metadata-recorded individual file + 9 bytes from bulk zip
	assert.Equal(t, int64(14), bytes)
	assert.Equal(t, 2, count)
}

func TestCache_Clear_RemovesAllChildrenKeepsRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := NewCache(dir, 1<<30)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "1.epub"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "1.epub.meta.json"), []byte("{}"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "bulk"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bulk", "abc.zip"), []byte("z"), 0644))

	require.NoError(t, c.Clear())

	// Root still exists
	_, err := os.Stat(dir)
	require.NoError(t, err)

	// Contents are gone
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestCache_Clear_IdempotentOnMissingRoot(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	c := NewCache(dir, 1<<30)

	require.NoError(t, c.Clear())
}
```

Add imports as needed (`encoding/json`, `os`, `path/filepath`, `time`, `github.com/stretchr/testify/require`, `github.com/stretchr/testify/assert`).

- [ ] **Step 2: Run tests, confirm they fail**

```bash
go test ./pkg/downloadcache/ -run 'TestCache_SizeBytes|TestCache_Clear' -v
```

Expected: FAIL (undefined `SizeBytes` / `Clear` methods).

- [ ] **Step 3: Implement `SizeBytes` and `Clear`**

Add to `pkg/downloadcache/cache.go` (near the bottom, after `MaxSize()`):

```go
// SizeBytes returns the total size in bytes and file count of the cache.
// Includes individual cached files (tracked via metadata) and bulk zip files.
// A missing root directory is treated as empty.
func (c *Cache) SizeBytes() (int64, int, error) {
	var totalBytes int64
	var totalCount int

	entries, err := ListCacheEntries(c.dir)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to list cache entries")
	}
	for _, e := range entries {
		totalBytes += e.SizeBytes
		totalCount++
	}

	bulkEntries, bulkBytes, err := listBulkZipEntries(c.dir)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to list bulk entries")
	}
	totalBytes += bulkBytes
	totalCount += len(bulkEntries)

	return totalBytes, totalCount, nil
}

// Clear removes all cached content while preserving the root directory itself.
// Safe to call when the root does not exist.
func (c *Cache) Clear() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "failed to read cache directory")
	}
	for _, e := range entries {
		path := filepath.Join(c.dir, e.Name())
		if err := os.RemoveAll(path); err != nil {
			return errors.Wrapf(err, "failed to remove %s", path)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests, confirm they pass**

```bash
go test ./pkg/downloadcache/ -run 'TestCache_SizeBytes|TestCache_Clear' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/downloadcache/cache.go pkg/downloadcache/cache_test.go
git commit -m "[Backend] Add SizeBytes and Clear to download cache"
```

---

## Task 2: Add `SizeBytes` and `Clear` to `cbzpages.Cache`

**Files:**
- Modify: `pkg/cbzpages/cache.go`
- Create or modify: `pkg/cbzpages/cache_test.go`

The CBZ page cache stores files under `{c.dir}/cbz/{fileID}/page_*.{ext}`. `SizeBytes` and `Clear` operate on `{c.dir}/cbz` only, not the shared parent `{c.dir}`.

- [ ] **Step 1: Check whether `pkg/cbzpages/cache_test.go` exists**

```bash
ls pkg/cbzpages/
```

If the test file does not exist, create it with a package declaration:

```go
package cbzpages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

- [ ] **Step 2: Write failing tests**

Append to `pkg/cbzpages/cache_test.go`:

```go
func TestCache_SizeBytes_Empty(t *testing.T) {
	t.Parallel()
	c := NewCache(t.TempDir())

	bytes, count, err := c.SizeBytes()
	require.NoError(t, err)
	assert.Equal(t, int64(0), bytes)
	assert.Equal(t, 0, count)
}

func TestCache_SizeBytes_CountsNestedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := NewCache(dir)

	// Simulate cached pages under {dir}/cbz/{fileID}/
	pageDir := filepath.Join(dir, "cbz", "42")
	require.NoError(t, os.MkdirAll(pageDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pageDir, "page_0.jpg"), []byte("abc"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pageDir, "page_1.jpg"), []byte("defgh"), 0644))

	bytes, count, err := c.SizeBytes()
	require.NoError(t, err)
	assert.Equal(t, int64(8), bytes)
	assert.Equal(t, 2, count)
}

func TestCache_Clear_RemovesCbzSubtree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := NewCache(dir)

	pageDir := filepath.Join(dir, "cbz", "42")
	require.NoError(t, os.MkdirAll(pageDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pageDir, "page_0.jpg"), []byte("x"), 0644))

	// An unrelated sibling should not be affected
	siblingDir := filepath.Join(dir, "pdf", "99")
	require.NoError(t, os.MkdirAll(siblingDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(siblingDir, "page_0.jpg"), []byte("y"), 0644))

	require.NoError(t, c.Clear())

	// cbz subtree gone
	_, err := os.Stat(filepath.Join(dir, "cbz"))
	assert.True(t, os.IsNotExist(err))

	// pdf sibling untouched
	_, err = os.Stat(filepath.Join(siblingDir, "page_0.jpg"))
	require.NoError(t, err)
}

func TestCache_Clear_IdempotentWhenMissing(t *testing.T) {
	t.Parallel()
	c := NewCache(t.TempDir())

	require.NoError(t, c.Clear())
}
```

- [ ] **Step 3: Run tests, confirm they fail**

```bash
go test ./pkg/cbzpages/ -run 'TestCache_SizeBytes|TestCache_Clear' -v
```

Expected: FAIL.

- [ ] **Step 4: Implement the methods**

Add to `pkg/cbzpages/cache.go` (after `Invalidate`):

```go
// rootDir returns the directory this cache owns.
func (c *Cache) rootDir() string {
	return filepath.Join(c.dir, "cbz")
}

// SizeBytes returns the total bytes and file count under the cache root.
// A missing root is treated as empty.
func (c *Cache) SizeBytes() (int64, int, error) {
	var totalBytes int64
	var totalCount int

	root := c.rootDir()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, errors.Wrap(err, "failed to walk cache")
	}
	return totalBytes, totalCount, nil
}

// Clear removes the cache root directory entirely. Safe when missing.
func (c *Cache) Clear() error {
	if err := os.RemoveAll(c.rootDir()); err != nil {
		return errors.Wrap(err, "failed to clear cache")
	}
	return nil
}
```

Note: `github.com/pkg/errors` is already imported in the package. If not, add it.

- [ ] **Step 5: Run tests, confirm they pass**

```bash
go test ./pkg/cbzpages/ -run 'TestCache_SizeBytes|TestCache_Clear' -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/cbzpages/cache.go pkg/cbzpages/cache_test.go
git commit -m "[Backend] Add SizeBytes and Clear to CBZ page cache"
```

---

## Task 3: Add `SizeBytes` and `Clear` to `pdfpages.Cache`

**Files:**
- Modify: `pkg/pdfpages/cache.go`
- Modify: `pkg/pdfpages/cache_test.go`

Same structure as CBZ but rooted at `{c.dir}/pdf`.

- [ ] **Step 1: Write failing tests**

Append to `pkg/pdfpages/cache_test.go`:

```go
func TestCache_SizeBytes_EmptyCacheDir(t *testing.T) {
	t.Parallel()
	c := NewCache(t.TempDir(), 150, 85)

	bytes, count, err := c.SizeBytes()
	require.NoError(t, err)
	assert.Equal(t, int64(0), bytes)
	assert.Equal(t, 0, count)
}

func TestCache_SizeBytes_CountsRenderedPages(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := NewCache(dir, 150, 85)

	pageDir := filepath.Join(dir, "pdf", "42")
	require.NoError(t, os.MkdirAll(pageDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pageDir, "page_0.jpg"), []byte("abc"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pageDir, "page_1.jpg"), []byte("defgh"), 0644))

	bytes, count, err := c.SizeBytes()
	require.NoError(t, err)
	assert.Equal(t, int64(8), bytes)
	assert.Equal(t, 2, count)
}

func TestCache_Clear_RemovesPdfSubtree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := NewCache(dir, 150, 85)

	pageDir := filepath.Join(dir, "pdf", "42")
	require.NoError(t, os.MkdirAll(pageDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pageDir, "page_0.jpg"), []byte("x"), 0644))

	siblingDir := filepath.Join(dir, "cbz", "99")
	require.NoError(t, os.MkdirAll(siblingDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(siblingDir, "page_0.jpg"), []byte("y"), 0644))

	require.NoError(t, c.Clear())

	_, err := os.Stat(filepath.Join(dir, "pdf"))
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(filepath.Join(siblingDir, "page_0.jpg"))
	require.NoError(t, err)
}

func TestCache_Clear_IdempotentWhenMissing(t *testing.T) {
	t.Parallel()
	c := NewCache(t.TempDir(), 150, 85)

	require.NoError(t, c.Clear())
}
```

Add imports (`os`, `path/filepath`, `github.com/stretchr/testify/require`, `github.com/stretchr/testify/assert`) if they are not already present.

- [ ] **Step 2: Run tests, confirm they fail**

```bash
go test ./pkg/pdfpages/ -run 'TestCache_SizeBytes|TestCache_Clear' -v
```

Expected: FAIL.

- [ ] **Step 3: Implement the methods**

Add to `pkg/pdfpages/cache.go` (after `Invalidate`):

```go
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
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, errors.Wrap(err, "failed to walk cache")
	}
	return totalBytes, totalCount, nil
}

// Clear removes the cache root directory entirely. Safe when missing.
func (c *Cache) Clear() error {
	if err := os.RemoveAll(c.rootDir()); err != nil {
		return errors.Wrap(err, "failed to clear cache")
	}
	return nil
}
```

Ensure `github.com/pkg/errors` is in the imports (it already is in `pdfpages/cache.go`).

- [ ] **Step 4: Run tests, confirm they pass**

```bash
go test ./pkg/pdfpages/ -run 'TestCache_SizeBytes|TestCache_Clear' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/pdfpages/cache.go pkg/pdfpages/cache_test.go
git commit -m "[Backend] Add SizeBytes and Clear to PDF page cache"
```

---

## Task 4: Create `pkg/cache` package with handlers and route registration

**Files:**
- Create: `pkg/cache/handlers.go`
- Create: `pkg/cache/routes.go`
- Create: `pkg/cache/handlers_test.go`

The handler exposes three caches by stable id and dispatches via `switch`. IDs: `"downloads"`, `"cbz_pages"`, `"pdf_pages"`. Handler depends only on a narrow interface to keep tests simple.

- [ ] **Step 1: Write failing handler tests**

Create `pkg/cache/handlers_test.go`:

```go
package cache

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCache struct {
	bytes       int64
	count       int
	sizeErr     error
	clearErr    error
	clearCalled int
}

func (f *fakeCache) SizeBytes() (int64, int, error) {
	return f.bytes, f.count, f.sizeErr
}

func (f *fakeCache) Clear() error {
	f.clearCalled++
	return f.clearErr
}

func newTestHandler() (*Handler, *fakeCache, *fakeCache, *fakeCache) {
	dl := &fakeCache{bytes: 100, count: 2}
	cbz := &fakeCache{bytes: 50, count: 5}
	pdf := &fakeCache{bytes: 25, count: 1}
	h := NewHandler(dl, cbz, pdf)
	return h, dl, cbz, pdf
}

func TestList_ReturnsAllThreeCaches(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestHandler()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/cache", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.list(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Caches, 3)

	ids := []string{resp.Caches[0].ID, resp.Caches[1].ID, resp.Caches[2].ID}
	assert.Contains(t, ids, "downloads")
	assert.Contains(t, ids, "cbz_pages")
	assert.Contains(t, ids, "pdf_pages")

	for _, ci := range resp.Caches {
		switch ci.ID {
		case "downloads":
			assert.Equal(t, int64(100), ci.SizeBytes)
			assert.Equal(t, 2, ci.FileCount)
		case "cbz_pages":
			assert.Equal(t, int64(50), ci.SizeBytes)
			assert.Equal(t, 5, ci.FileCount)
		case "pdf_pages":
			assert.Equal(t, int64(25), ci.SizeBytes)
			assert.Equal(t, 1, ci.FileCount)
		}
	}
}

func TestClear_DispatchesByID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id        string
		expectDL  int
		expectCBZ int
		expectPDF int
	}{
		{"downloads", 1, 0, 0},
		{"cbz_pages", 0, 1, 0},
		{"pdf_pages", 0, 0, 1},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			h, dl, cbz, pdf := newTestHandler()

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/cache/"+tc.id+"/clear", strings.NewReader(""))
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues(tc.id)

			require.NoError(t, h.clear(c))
			assert.Equal(t, http.StatusOK, rec.Code)

			assert.Equal(t, tc.expectDL, dl.clearCalled)
			assert.Equal(t, tc.expectCBZ, cbz.clearCalled)
			assert.Equal(t, tc.expectPDF, pdf.clearCalled)

			var resp ClearResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.NotEmpty(t, resp)
		})
	}
}

func TestClear_UnknownIDReturns404(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestHandler()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/cache/unknown/clear", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("unknown")

	err := h.clear(c)
	require.Error(t, err)
	he, ok := err.(*echo.HTTPError)
	require.True(t, ok, "expected echo.HTTPError, got %T", err)
	assert.Equal(t, http.StatusNotFound, he.Code)
}

func TestClear_ReportsPreClearSize(t *testing.T) {
	t.Parallel()
	h, dl, _, _ := newTestHandler()
	dl.bytes = 500
	dl.count = 7

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/cache/downloads/clear", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("downloads")

	require.NoError(t, h.clear(c))

	var resp ClearResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, int64(500), resp.ClearedBytes)
	assert.Equal(t, 7, resp.ClearedFiles)
}
```

- [ ] **Step 2: Run tests, confirm they fail**

```bash
go test ./pkg/cache/ -v
```

Expected: FAIL (package does not compile — `Handler`, `NewHandler`, `ListResponse`, `ClearResponse`, `.list`, `.clear` are undefined).

- [ ] **Step 3: Implement `pkg/cache/handlers.go`**

```go
package cache

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

// Provider is the minimal interface a cache must implement to be managed.
type Provider interface {
	SizeBytes() (int64, int, error)
	Clear() error
}

// Handler exposes HTTP endpoints for cache management.
type Handler struct {
	downloads Provider
	cbzPages  Provider
	pdfPages  Provider
}

// NewHandler returns a new cache management handler.
func NewHandler(downloads, cbzPages, pdfPages Provider) *Handler {
	return &Handler{
		downloads: downloads,
		cbzPages:  cbzPages,
		pdfPages:  pdfPages,
	}
}

// CacheInfo describes a single cache in the list response.
type CacheInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SizeBytes   int64  `json:"size_bytes"`
	FileCount   int    `json:"file_count"`
}

// ListResponse is returned from GET /cache.
type ListResponse struct {
	Caches []CacheInfo `json:"caches"`
}

// ClearResponse is returned from POST /cache/:id/clear.
type ClearResponse struct {
	ClearedBytes int64 `json:"cleared_bytes"`
	ClearedFiles int   `json:"cleared_files"`
}

type cacheEntry struct {
	id          string
	name        string
	description string
	provider    Provider
}

func (h *Handler) entries() []cacheEntry {
	return []cacheEntry{
		{
			id:          "downloads",
			name:        "Downloads",
			description: "Generated format conversions (e.g. kepub), plugin-generated files, and bulk-download zips.",
			provider:    h.downloads,
		},
		{
			id:          "cbz_pages",
			name:        "CBZ Pages",
			description: "Page images extracted from CBZ files for the in-app reader.",
			provider:    h.cbzPages,
		},
		{
			id:          "pdf_pages",
			name:        "PDF Pages",
			description: "JPEGs rendered from PDF pages for the in-app reader.",
			provider:    h.pdfPages,
		},
	}
}

func (h *Handler) list(c echo.Context) error {
	entries := h.entries()
	resp := ListResponse{Caches: make([]CacheInfo, 0, len(entries))}
	for _, e := range entries {
		bytes, count, err := e.provider.SizeBytes()
		if err != nil {
			return errors.Wrapf(err, "failed to compute size for %s", e.id)
		}
		resp.Caches = append(resp.Caches, CacheInfo{
			ID:          e.id,
			Name:        e.name,
			Description: e.description,
			SizeBytes:   bytes,
			FileCount:   count,
		})
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) clear(c echo.Context) error {
	id := c.Param("id")

	entries := h.entries()
	var entry *cacheEntry
	for i := range entries {
		if entries[i].id == id {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		return errcodes.NotFound("cache")
	}

	bytes, count, err := entry.provider.SizeBytes()
	if err != nil {
		return errors.Wrapf(err, "failed to compute size for %s before clear", id)
	}
	if err := entry.provider.Clear(); err != nil {
		return errors.Wrapf(err, "failed to clear %s", id)
	}

	return c.JSON(http.StatusOK, ClearResponse{
		ClearedBytes: bytes,
		ClearedFiles: count,
	})
}
```

Check that `errcodes.NotFound` returns a `*echo.HTTPError` with `http.StatusNotFound`. If it returns a different type, adjust the test's type assertion to match, but prefer fixing the test to use the project's actual error helper rather than bypassing it.

Verify with:

```bash
grep -n "func NotFound" pkg/errcodes/
```

If it does not produce an `*echo.HTTPError`, rewrite the handler `return` to `return echo.NewHTTPError(http.StatusNotFound, "cache not found")` and remove the `errcodes` import.

- [ ] **Step 4: Implement `pkg/cache/routes.go`**

```go
package cache

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
)

// RegisterRoutes registers cache management routes on the given echo instance.
// GET /cache requires config:read; POST /cache/:id/clear requires config:write.
func RegisterRoutes(e *echo.Echo, h *Handler, authMiddleware *auth.Middleware) {
	g := e.Group("/cache")
	g.Use(authMiddleware.Authenticate)

	g.GET("", h.list, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationRead))
	g.POST("/:id/clear", h.clear, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationWrite))
}
```

- [ ] **Step 5: Run handler tests, confirm they pass**

```bash
go test ./pkg/cache/ -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/cache/handlers.go pkg/cache/routes.go pkg/cache/handlers_test.go
git commit -m "[Backend] Add cache management handlers and routes"
```

---

## Task 5: Wire cache handlers into the server

**Files:**
- Modify: `cmd/api/main.go`
- Modify: `pkg/server/server.go`

Today `cbzCache` and `pdfCache` are constructed inside `registerProtectedRoutes` as local vars. Move their construction to `main.go` so they share lifecycle with `dlCache`, then pass them into `server.New` and through to route registration.

- [ ] **Step 1: Read the current wiring**

```bash
grep -n "cbzpages\|pdfpages\|dlCache" cmd/api/main.go pkg/server/server.go
```

Read the surrounding context so you know exactly where to insert.

- [ ] **Step 2: Update `cmd/api/main.go`**

Locate:

```go
dlCache := downloadcache.NewCache(filepath.Join(cfg.CacheDir, "downloads"), cfg.DownloadCacheMaxSizeBytes())

wrkr := worker.New(cfg, db, pluginManager, broker, dlCache)

srv, err := server.New(cfg, db, wrkr, pluginManager, broker, dlCache, logBuffer)
```

Change to:

```go
dlCache := downloadcache.NewCache(filepath.Join(cfg.CacheDir, "downloads"), cfg.DownloadCacheMaxSizeBytes())
cbzCache := cbzpages.NewCache(cfg.CacheDir)
pdfCache := pdfpages.NewCache(cfg.CacheDir, cfg.PDFRenderDPI, cfg.PDFRenderQuality)

wrkr := worker.New(cfg, db, pluginManager, broker, dlCache)

srv, err := server.New(cfg, db, wrkr, pluginManager, broker, dlCache, cbzCache, pdfCache, logBuffer)
```

Add imports to `cmd/api/main.go`:

```go
"github.com/shishobooks/shisho/pkg/cbzpages"
"github.com/shishobooks/shisho/pkg/pdfpages"
```

- [ ] **Step 3: Update `pkg/server/server.go` `New` signature and callers**

Change:

```go
func New(cfg *config.Config, db *bun.DB, w *worker.Worker, pm *plugins.Manager, broker *events.Broker, dlCache *downloadcache.Cache, logBuffer *logs.RingBuffer) (*http.Server, error) {
```

to:

```go
func New(cfg *config.Config, db *bun.DB, w *worker.Worker, pm *plugins.Manager, broker *events.Broker, dlCache *downloadcache.Cache, cbzCache *cbzpages.Cache, pdfCache *pdfpages.Cache, logBuffer *logs.RingBuffer) (*http.Server, error) {
```

Inside `New`, thread `cbzCache` and `pdfCache` into `registerProtectedRoutes`. Change the signature of `registerProtectedRoutes`:

```go
func registerProtectedRoutes(e *echo.Echo, db *bun.DB, cfg *config.Config, authMiddleware *auth.Middleware, w *worker.Worker, pm *plugins.Manager, broker *events.Broker, dlCache *downloadcache.Cache, cbzCache *cbzpages.Cache, pdfCache *pdfpages.Cache) {
```

Update the call site inside `New` to pass the new arguments.

Inside `registerProtectedRoutes`, find:

```go
cbzCache := cbzpages.NewCache(cfg.CacheDir)
pdfCache := pdfpages.NewCache(cfg.CacheDir, cfg.PDFRenderDPI, cfg.PDFRenderQuality)
```

Delete these two lines (the caches now come in as parameters). Leave everything else using `cbzCache` / `pdfCache` untouched.

- [ ] **Step 4: Register the cache routes in `New`**

At the end of `New`, after the existing route registrations and before `return`, add:

```go
cacheHandler := cache.NewHandler(dlCache, cbzCache, pdfCache)
cache.RegisterRoutes(e, cacheHandler, authMiddleware)
```

Add import to `pkg/server/server.go`:

```go
"github.com/shishobooks/shisho/pkg/cache"
```

If there's already a `cache` import path collision, alias this one to `cachepkg "github.com/shishobooks/shisho/pkg/cache"` and use `cachepkg.` instead.

- [ ] **Step 5: Verify the whole project builds and tests pass**

```bash
go build ./...
go test ./pkg/cache/ ./pkg/downloadcache/ ./pkg/cbzpages/ ./pkg/pdfpages/ ./pkg/server/ -v
```

Expected: builds; tests PASS. If any server tests rely on the old `New` signature, update them to pass `nil` or test doubles for the new parameters.

- [ ] **Step 6: Commit**

```bash
git add cmd/api/main.go pkg/server/server.go
git commit -m "[Backend] Wire cache management handlers into server"
```

---

## Task 6: Regenerate TypeScript types and add frontend query hooks

**Files:**
- Generated: `app/types/generated/*` (via `mise tygo`)
- Create: `app/hooks/queries/cache.ts`

- [ ] **Step 1: Run tygo**

```bash
mise tygo
```

Expected output: either a summary of generated files, or "skipping, outputs are up-to-date". Either is fine — the second just means types haven't changed yet, which is OK since our types come from `pkg/cache` that may or may not be covered by `tygo.yaml`.

- [ ] **Step 2: Check whether `pkg/cache` is covered by tygo**

```bash
grep -n "pkg/cache\|cache:" tygo.yaml
```

If `pkg/cache` is not listed in `tygo.yaml`, add an entry modeled after other packages, for example:

```yaml
  - path: "github.com/shishobooks/shisho/pkg/cache"
    output_path: "app/types/generated/cache.ts"
    indent: "  "
```

Then rerun `mise tygo`. Confirm `app/types/generated/cache.ts` exists (note: it is gitignored — do not try to `git add` it).

- [ ] **Step 3: Check how other query hooks are organized**

```bash
ls app/hooks/queries/
```

Read one of the existing simple hooks (e.g. `config.ts`) to copy its style — query keys, base URL, React Query options.

- [ ] **Step 4: Create `app/hooks/queries/cache.ts`**

```ts
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import type {
  CacheInfo,
  ClearResponse,
  ListResponse,
} from "@/types/generated/cache";

const CACHE_QUERY_KEY = ["cache"] as const;

async function fetchCaches(): Promise<CacheInfo[]> {
  const res = await fetch("/api/cache");
  if (!res.ok) {
    throw new Error(`Failed to load caches: ${res.status}`);
  }
  const data = (await res.json()) as ListResponse;
  return data.caches;
}

async function clearCache(id: string): Promise<ClearResponse> {
  const res = await fetch(`/api/cache/${encodeURIComponent(id)}/clear`, {
    method: "POST",
  });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(body || `Failed to clear cache: ${res.status}`);
  }
  return (await res.json()) as ClearResponse;
}

export const useCaches = () =>
  useQuery({
    queryKey: CACHE_QUERY_KEY,
    queryFn: fetchCaches,
    placeholderData: keepPreviousData,
  });

export const useClearCache = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: clearCache,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: CACHE_QUERY_KEY });
    },
  });
};
```

If the existing `queries/config.ts` uses a different HTTP helper (e.g. an `api` or `fetchJson` wrapper), use that helper instead of raw `fetch` for consistency. Inspect it before writing this file:

```bash
cat app/hooks/queries/config.ts
```

- [ ] **Step 5: Typecheck**

```bash
pnpm lint:types
```

Expected: passes. If it fails because `ListResponse`/`ClearResponse`/`CacheInfo` are not exported from the generated file, open `app/types/generated/cache.ts` and confirm the exact names — tygo uses the Go struct names verbatim — then adjust the imports.

- [ ] **Step 6: Commit**

```bash
git add app/hooks/queries/cache.ts tygo.yaml
git commit -m "[Frontend] Add cache query hooks"
```

(Include `tygo.yaml` only if it was edited in Step 2.)

---

## Task 7: Add sidebar nav item and register the route

**Files:**
- Modify: `app/components/pages/useAdminNavItems.ts`
- Modify: `app/router.tsx`

- [ ] **Step 1: Add the Cache nav item**

In `app/components/pages/useAdminNavItems.ts`, add `HardDrive` to the lucide-react import list and insert a new entry between `Jobs` and `Plugins` (or wherever feels most appropriate in the existing order — place it just before `Logs` since both are admin-observability items):

```ts
import {
  Briefcase,
  Cog,
  HardDrive,
  Library,
  Puzzle,
  ScrollText,
  Users,
  type LucideIcon,
} from "lucide-react";
```

Insert in the returned array, right before the `/settings/logs` entry:

```ts
{
  to: "/settings/cache",
  Icon: HardDrive,
  label: "Cache",
  isActive: location.pathname.startsWith("/settings/cache"),
  show: canViewConfig,
},
```

- [ ] **Step 2: Register the route**

In `app/router.tsx`:

1. Add the import near the other admin page imports:

```ts
import AdminCache from "@/components/pages/AdminCache";
```

2. Inside the `children` array of the `settings` route, add a new entry modeled after `/settings/plugins`:

```ts
{
  path: "cache",
  element: (
    <ProtectedRoute
      requiredPermissions={[{ resource: "config", operation: "read" }]}
    >
      <AdminCache />
    </ProtectedRoute>
  ),
},
```

Inspect the exact shape of other entries first — the `ProtectedRoute` API varies by project:

```bash
grep -n "ProtectedRoute" app/router.tsx | head -10
```

Match the surrounding entries' prop names exactly (e.g. some codebases use `requiredPermission={{ resource, operation }}` singular). Copy the nearest sibling for `/settings/plugins` that already requires `config:read`.

- [ ] **Step 3: Typecheck and lint**

```bash
pnpm lint:types
pnpm lint:eslint
```

Both should pass. (`AdminCache` does not exist yet — the typecheck will fail here. That's expected; we'll create it in the next task.)

Because of the forward reference, **do not commit yet**. Move on to Task 8 and commit together.

---

## Task 8: Build the `AdminCache` page

**Files:**
- Create: `app/components/pages/AdminCache.tsx`

- [ ] **Step 1: Inspect the existing AlertDialog and Button primitives**

```bash
ls app/components/ui/
grep -n "AlertDialog" app/components/ui/*.tsx | head -5
```

Read one existing consumer of `AlertDialog` (e.g. via `grep -rln "AlertDialog" app/components/pages/`) to copy its structure.

- [ ] **Step 2: Inspect existing byte/size formatting helper (if any)**

```bash
grep -rn "formatBytes\|formatSize\|humanSize" app/
```

If a helper exists, reuse it. Otherwise, inline a small `formatBytes` inside the new page (simple binary units: B, KB, MB, GB).

- [ ] **Step 3: Create `app/components/pages/AdminCache.tsx`**

Use the existing page conventions (see `AdminSettings.tsx` for layout). Skeleton:

```tsx
import { useState } from "react";
import { toast } from "sonner"; // Or whatever toast library this project uses

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { useCaches, useClearCache } from "@/hooks/queries/cache";
import { useAuth } from "@/hooks/useAuth";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { CacheInfo } from "@/types/generated/cache";

const formatBytes = (bytes: number): string => {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(
    sizes.length - 1,
    Math.floor(Math.log(bytes) / Math.log(k)),
  );
  const value = bytes / Math.pow(k, i);
  return `${value >= 10 ? value.toFixed(0) : value.toFixed(1)} ${sizes[i]}`;
};

const CacheCard = ({
  cache,
  canClear,
  onClearRequested,
  isClearing,
}: {
  cache: CacheInfo;
  canClear: boolean;
  onClearRequested: () => void;
  isClearing: boolean;
}) => (
  <div className="border border-border rounded-md p-4 md:p-6">
    <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-3">
      <div>
        <h2 className="text-base md:text-lg font-semibold">{cache.name}</h2>
        <p className="text-xs md:text-sm text-muted-foreground mt-1">
          {cache.description}
        </p>
        <div className="mt-3 text-sm text-muted-foreground">
          <span className="font-mono">{formatBytes(cache.size_bytes)}</span>
          <span className="mx-2">&middot;</span>
          <span>
            {cache.file_count} {cache.file_count === 1 ? "file" : "files"}
          </span>
        </div>
      </div>
      {canClear && (
        <Button
          variant="outline"
          onClick={onClearRequested}
          disabled={isClearing || cache.file_count === 0}
        >
          {isClearing ? "Clearing..." : "Clear"}
        </Button>
      )}
    </div>
  </div>
);

const AdminCache = () => {
  usePageTitle("Cache");
  const { hasPermission } = useAuth();
  const canClear = hasPermission("config", "write");

  const { data: caches, isLoading, error } = useCaches();
  const clearMutation = useClearCache();

  const [pending, setPending] = useState<CacheInfo | null>(null);

  if (isLoading) return <LoadingSpinner />;

  if (error) {
    return (
      <div className="text-center">
        <h1 className="text-2xl font-semibold mb-4">Error loading caches</h1>
        <p className="text-muted-foreground">{error.message}</p>
      </div>
    );
  }

  if (!caches) return null;

  const handleConfirmClear = async () => {
    if (!pending) return;
    const target = pending;
    setPending(null);
    try {
      const result = await clearMutation.mutateAsync(target.id);
      toast.success(
        `Cleared ${formatBytes(result.cleared_bytes)} (${result.cleared_files} files) from ${target.name}`,
      );
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to clear cache",
      );
    }
  };

  return (
    <div>
      <div className="mb-6 md:mb-8">
        <h1 className="text-xl md:text-2xl font-semibold mb-1 md:mb-2">
          Cache
        </h1>
        <p className="text-sm md:text-base text-muted-foreground">
          Inspect and clear server caches. Content will be regenerated on next
          access.
        </p>
      </div>

      <div className="grid gap-6">
        {caches.map((cache) => (
          <CacheCard
            key={cache.id}
            cache={cache}
            canClear={canClear}
            isClearing={
              clearMutation.isPending && clearMutation.variables === cache.id
            }
            onClearRequested={() => setPending(cache)}
          />
        ))}
      </div>

      <AlertDialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) setPending(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Clear {pending?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              This will delete {pending?.file_count} files (
              {pending ? formatBytes(pending.size_bytes) : ""}). Content will be
              regenerated on next access.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={handleConfirmClear}>
              Clear
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
};

export default AdminCache;
```

Before trusting this, verify each import path against the repo:

```bash
grep -rn "sonner\|useToast\|react-hot-toast" app/ | head -5
grep -n "hasPermission" app/hooks/useAuth.ts
```

If the project uses a different toast library or a different `hasPermission` signature (e.g. `hasPermission({ resource, operation })`), adjust the code to match. Copy the style from `AdminSettings.tsx` and an existing page with a confirm dialog (e.g. a delete confirm in `PersonList.tsx`).

- [ ] **Step 4: Typecheck and lint**

```bash
pnpm lint:types
pnpm lint:eslint
pnpm lint:prettier
```

Expected: all pass.

- [ ] **Step 5: Manual verification**

Start the dev server (the user likely has `mise start` running; if not, start it). Visit `http://localhost:8080/settings/cache` (or wherever the frontend is served). Verify:

1. All three cards render with non-error sizes.
2. Clicking "Clear" opens the confirm dialog with the correct size and file count.
3. Confirming clears the cache, the toast appears, and the card refreshes to 0 B / 0 files.
4. The relevant on-disk directories under `tmp/cache` (or the configured `cache_dir`) are empty after clearing.
5. A user without `config:write` (Editor/Viewer) does not see the Clear button.

If you cannot run the UI yourself, state that explicitly rather than claiming success.

- [ ] **Step 6: Commit (combined with Task 7 changes)**

```bash
git add app/components/pages/AdminCache.tsx app/components/pages/useAdminNavItems.ts app/router.tsx
git commit -m "[Frontend] Add cache management page at /settings/cache"
```

---

## Task 9: Documentation

**Files:**
- Modify: `website/docs/configuration.md`
- Create: `website/docs/cache-management.md`
- Possibly modify: `website/sidebars.ts` or similar sidebar config

- [ ] **Step 1: Find the docs sidebar definition**

```bash
ls website/
find website -name "sidebar*" -o -name "sidebars*" | head
```

Open the sidebar config to see how pages are ordered. Plan to insert `cache-management` near `configuration`.

- [ ] **Step 2: Create `website/docs/cache-management.md`**

```markdown
---
title: Cache Management
---

Shisho maintains three on-disk caches to speed up common operations. All three live under the directory set by the [`cache_dir`](./configuration.md#cache_dir) config option (default `/config/cache`).

| Cache | What it stores | When it matters |
|-------|----------------|-----------------|
| **Downloads** | Generated format conversions (e.g. kepub), files produced by plugins, and bulk-download zips. | Speeds up repeat downloads; plugin-generated conversions are often expensive to recompute. The size is capped by [`download_cache_max_size_gb`](./configuration.md#download_cache_max_size_gb) and auto-evicted LRU-style. |
| **CBZ Pages** | Page images extracted from CBZ files for the in-app reader. | Avoids re-extracting pages every time a CBZ is opened. |
| **PDF Pages** | JPEGs rendered from PDF pages for the in-app reader. | Avoids re-rendering pages; expensive on large or image-heavy PDFs. |

## Viewing cache usage

Admins can view the current size and file count of each cache at **Settings → Cache** (`/settings/cache`). The page requires the `config:read` permission.

## Clearing caches

Each cache has its own **Clear** button. Clearing is safe — content is regenerated on next access — but can temporarily slow down affected operations (downloads, the PDF reader, etc.) while the cache rebuilds.

Clearing requires the `config:write` permission (admin-only by default). A confirmation dialog shows the number of files and total size that will be deleted before the action is performed.

## When to clear

- **Downloads**: reclaim disk space if the cap has been raised and is now too large, or after removing a plugin whose generated files should not be reused.
- **CBZ Pages / PDF Pages**: force the reader to re-extract or re-render after changing a config option that affects output (e.g. [`pdf_render_dpi`](./configuration.md#pdf_render_dpi) or [`pdf_render_quality`](./configuration.md#pdf_render_quality)).

See also: [Configuration](./configuration.md).
```

- [ ] **Step 3: Cross-link from `website/docs/configuration.md`**

Find the Storage / `cache_dir` section and add a sentence linking to the new page, for example:

```markdown
See [Cache Management](./cache-management.md) for how to inspect and clear these caches from the admin UI.
```

- [ ] **Step 4: Add the new page to the docs sidebar**

Edit the sidebar config found in Step 1 to include `cache-management` immediately after `configuration`. The exact syntax depends on whether the project uses autogenerated sidebars or explicit entries. If it uses autogenerated, the file will appear automatically — skip this step.

- [ ] **Step 5: Build docs to verify**

```bash
cd website && pnpm build && cd ..
```

Expected: clean build, no broken links reported.

- [ ] **Step 6: Commit**

```bash
git add website/docs/cache-management.md website/docs/configuration.md
# Also add website/sidebars.* if modified.
git commit -m "[Docs] Document cache management page"
```

---

## Task 10: Full verification and final polish

- [ ] **Step 1: Run the full check suite**

```bash
mise check:quiet
```

Expected: pass. If anything fails, fix in place (`go test`, `pnpm lint:types`, `pnpm lint:eslint`, `pnpm lint:prettier`, `golangci-lint`), then rerun.

- [ ] **Step 2: Manual smoke in dev server**

Start `mise start` if not already running. Confirm:

- `/settings/cache` renders three cards.
- Clear flow works end to end.
- On-disk directories behave as expected (see Task 8 Step 5).

- [ ] **Step 3: Final commit if any fixup was needed**

```bash
git add -u
git diff --staged  # sanity check
git commit -m "[Backend] Address review findings for cache management"
```

(Skip if no changes.)

---

## Spec Coverage Check

| Spec requirement | Task |
|------------------|------|
| Three caches listed: downloads, cbz_pages, pdf_pages | Task 4 |
| `SizeBytes` and `Clear` per cache | Tasks 1, 2, 3 |
| `GET /api/cache` gated by `config:read` | Task 4 + Task 5 |
| `POST /api/cache/:id/clear` gated by `config:write` | Task 4 + Task 5 |
| Unknown id → 404 | Task 4 |
| Sidebar item visible to `config:read` | Task 7 |
| `/settings/cache` page with three cards and sizes | Task 8 |
| Clear button disabled without `config:write` | Task 8 |
| AlertDialog confirm flow | Task 8 |
| Toast on success/failure, refetch on success | Tasks 6, 8 |
| Types via tygo, not hand-written | Task 6 |
| Snake_case JSON payloads | Task 4 |
| `t.Parallel()` on all new tests | Tasks 1–4 |
| Docs updated: configuration cross-link + new page | Task 9 |
| `mise check:quiet` passes | Task 10 |
