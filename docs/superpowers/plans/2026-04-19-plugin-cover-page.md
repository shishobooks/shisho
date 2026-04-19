# Plugin-Settable Cover Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let plugins set `coverPage` for CBZ/PDF files (under the existing `cover` capability) so enrichers can choose a non-first page as the cover without manual intervention.

**Architecture:** (1) Wire `coverPage` through the two JS→Go parsers that don't already handle it; `parseParsedMetadata` already does. (2) Extract the manual UI handler's page-to-cover-image logic into a shared `ExtractCoverPageToFile` function in `pkg/books`. (3) Inject a `PageExtractor` dependency into `plugins.EnrichDeps` and update `persistMetadata` to apply `md.CoverPage` for page-based files, silently ignoring `coverData`/`coverUrl` for those formats (strict file-type-gated precedence). Out-of-range pages are skipped with a warning.

**Tech Stack:** Go, Bun ORM, Goja, existing `pkg/cbzpages` and `pkg/pdfpages` caches for page rendering.

**Spec:** `docs/superpowers/specs/2026-04-19-plugin-cover-page-design.md`

---

## Background notes

- `ParsedMetadata.CoverPage *int` already exists (`pkg/mediafile/mediafile.go:50`).
- `coverPage?: number` already declared in SDK (`packages/plugin-sdk/metadata.d.ts:66`).
- **`parseParsedMetadata` already parses `coverPage`** (`pkg/plugins/hooks.go:552-557`) — no change needed.
- `models.File.CoverPage *int` already exists and `IsPageBasedFileType` returns true for CBZ and PDF (`pkg/models/file.go:86`).
- `files.cover_page` column already exists — no migration.
- The manual UI handler `updateFileCoverPage` (`pkg/books/handlers_cover_page.go:26-135`) is the reference implementation for applying a cover page.
- Existing persist-metadata test pattern lives in `pkg/plugins/handler_persist_metadata_test.go` — use the same stub-based setup.
- `EnrichDeps` is constructed once in `pkg/server/server.go:200` and passed to both `RegisterIdentifyRoutes` and `RegisterRoutesWithGroup`. Add a new field there.

---

## Task 1: Parse `coverPage` in `parseSearchResponse`

**Files:**
- Modify: `pkg/plugins/hooks.go:390-401` (add `coverPage` parsing inside the per-result loop)
- Test: `pkg/plugins/hooks_search_result_test.go` (existing file for `parseSearchResponse` tests)

- [ ] **Step 1: Read the existing test file to match style**

Run: `cat pkg/plugins/hooks_search_result_test.go | head -60` so your new test matches the existing test setup (VM creation, JS eval, `parseSearchResponse` call, `results[0]` assertions).

- [ ] **Step 2: Add a failing test for `coverPage` in search results**

Append to `pkg/plugins/hooks_search_result_test.go`:

```go
func TestParseSearchResponse_CoverPage(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	val, err := vm.RunString(`({ results: [{ title: "x", coverPage: 3 }] })`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "test", "plugin-id")

	require.Len(t, resp.Results, 1)
	require.NotNil(t, resp.Results[0].CoverPage)
	assert.Equal(t, 3, *resp.Results[0].CoverPage)
}

func TestParseSearchResponse_CoverPage_MissingOrNull(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	val, err := vm.RunString(`({ results: [{ title: "a" }, { title: "b", coverPage: null }] })`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "test", "plugin-id")

	require.Len(t, resp.Results, 2)
	assert.Nil(t, resp.Results[0].CoverPage)
	assert.Nil(t, resp.Results[1].CoverPage)
}
```

If `goja` is not already imported by the test file, add `"github.com/dop251/goja"` to the imports.

- [ ] **Step 3: Run tests and confirm they fail**

Run: `go test ./pkg/plugins/ -run TestParseSearchResponse_CoverPage -v`

Expected: FAIL. Likely `Expected: not <nil>` because `CoverPage` is never set.

- [ ] **Step 4: Add the parsing block to `parseSearchResponse`**

In `pkg/plugins/hooks.go`, immediately after the `confidence` block (near line 427), add:

```go
// coverPage -> *int
coverPageVal := itemObj.Get("coverPage")
if coverPageVal != nil && !goja.IsUndefined(coverPageVal) && !goja.IsNull(coverPageVal) {
	cp := int(coverPageVal.ToInteger())
	md.CoverPage = &cp
}
```

- [ ] **Step 5: Run the tests and confirm they pass**

Run: `go test ./pkg/plugins/ -run TestParseSearchResponse_CoverPage -v`

Expected: PASS.

- [ ] **Step 6: Run the full plugins test package to catch regressions**

Run: `go test ./pkg/plugins/ -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/plugins/hooks.go pkg/plugins/hooks_search_result_test.go
git commit -m "[Backend] Parse coverPage from metadata enricher search results"
```

---

## Task 2: Parse `cover_page` in `convertFieldsToMetadata`

**Files:**
- Modify: `pkg/plugins/handler.go:1929-1970` (the `convertFieldsToMetadata` function)
- Test: `pkg/plugins/handler_convert_test.go` (existing)

- [ ] **Step 1: Add a failing test**

Append to `pkg/plugins/handler_convert_test.go`:

```go
func TestConvertFieldsToMetadata_CoverPage(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"cover_page": float64(4), // JSON numbers decode to float64
	}

	md := convertFieldsToMetadata(fields)

	require.NotNil(t, md.CoverPage)
	assert.Equal(t, 4, *md.CoverPage)
}

func TestConvertFieldsToMetadata_CoverPage_Missing(t *testing.T) {
	t.Parallel()
	md := convertFieldsToMetadata(map[string]any{})
	assert.Nil(t, md.CoverPage)
}
```

- [ ] **Step 2: Run and confirm they fail**

Run: `go test ./pkg/plugins/ -run TestConvertFieldsToMetadata_CoverPage -v`

Expected: FAIL. `md.CoverPage` is nil.

- [ ] **Step 3: Add the parsing block**

In `pkg/plugins/handler.go`, inside `convertFieldsToMetadata`, after the `series_number` block (near line 1960), add:

```go
// Cover page (0-indexed page number for CBZ/PDF)
if v, ok := fields["cover_page"].(float64); ok {
	cp := int(v)
	md.CoverPage = &cp
}
```

- [ ] **Step 4: Run and confirm they pass**

Run: `go test ./pkg/plugins/ -run TestConvertFieldsToMetadata_CoverPage -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/plugins/handler.go pkg/plugins/handler_convert_test.go
git commit -m "[Backend] Parse cover_page in manual identify apply payload"
```

---

## Task 3: Extract shared `ExtractCoverPageToFile` helper

This is a pure refactor: move the page extraction + cover file write logic from `updateFileCoverPage` into a new exported function so `persistMetadata` can reuse it. Existing tests in `handlers_cover_page_test.go` validate behavior is unchanged.

**Files:**
- Create: `pkg/books/coverpage_extract.go`
- Modify: `pkg/books/handlers_cover_page.go` (swap in the helper)
- Test: no new test — existing `handlers_cover_page_test.go` is the regression suite

- [ ] **Step 1: Create the helper file**

Create `pkg/books/coverpage_extract.go`:

```go
package books

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdfpages"
)

// ExtractCoverPageToFile renders `page` from the given page-based file (CBZ or
// PDF) via the appropriate page cache and writes the rendered image as the
// cover file alongside the book. Returns the cover filename (not path) and
// MIME type. Any existing cover image with the same base name is removed first
// regardless of extension.
//
// Callers are responsible for updating the file's CoverPage, CoverImageFilename,
// CoverMimeType, and CoverSource fields on the model and persisting them.
func ExtractCoverPageToFile(
	file *models.File,
	bookFilepath string,
	page int,
	cbzCache *cbzpages.Cache,
	pdfCache *pdfpages.Cache,
	log logger.Logger,
) (filename string, mimeType string, err error) {
	var cachedPath string
	switch file.FileType {
	case models.FileTypeCBZ:
		cachedPath, mimeType, err = cbzCache.GetPage(file.Filepath, file.ID, page)
	case models.FileTypePDF:
		cachedPath, mimeType, err = pdfCache.GetPage(file.Filepath, file.ID, page)
	default:
		return "", "", errors.Errorf("file type %q does not support page-based covers", file.FileType)
	}
	if err != nil {
		return "", "", errors.Wrap(err, "failed to extract cover page")
	}

	coverDir := fileutils.ResolveCoverDir(bookFilepath)
	coverBaseName := filepath.Base(file.Filepath) + ".cover"

	ext := getExtensionFromMimeType(mimeType)
	if ext == "" {
		ext = filepath.Ext(cachedPath)
	}

	// Delete any existing cover with this base name (regardless of extension).
	for _, existingExt := range fileutils.CoverImageExtensions {
		existingPath := filepath.Join(coverDir, coverBaseName+existingExt)
		if _, err := os.Stat(existingPath); err == nil {
			if err := os.Remove(existingPath); err != nil {
				log.Warn("failed to remove existing cover", logger.Data{"path": existingPath, "error": err.Error()})
			}
		}
	}

	coverFilename := coverBaseName + ext
	coverFilepath := filepath.Join(coverDir, coverFilename)
	if err := copyFile(cachedPath, coverFilepath); err != nil {
		return "", "", errors.Wrap(err, "failed to save cover image")
	}

	return coverFilename, mimeType, nil
}
```

Note: `copyFile` and `getExtensionFromMimeType` are already package-private helpers in `pkg/books/handlers_cover_page.go` and `handlers.go` — the new file references them directly.

- [ ] **Step 2: Replace the inline logic in `updateFileCoverPage`**

In `pkg/books/handlers_cover_page.go`, replace lines 65-107 (from `// Extract the page image using the appropriate page cache` through the `copyFile` call) with:

```go
	coverFilename, mimeType, err := ExtractCoverPageToFile(
		file,
		file.Book.Filepath,
		payload.Page,
		h.pageCache,
		h.pdfPageCache,
		log,
	)
	if err != nil {
		log.Error("failed to extract cover page", logger.Data{"error": err.Error(), "page": payload.Page, "file_type": file.FileType})
		return errcodes.ValidationError("Failed to extract page from file")
	}

	log.Info("set cover page", logger.Data{
		"file_id":   file.ID,
		"page":      payload.Page,
		"cover":     coverFilename,
		"mime_type": mimeType,
	})
```

Then update the field-setting block (existing lines 117-121) to use `coverFilename` directly instead of reconstructing it:

```go
	file.CoverPage = &payload.Page
	file.CoverMimeType = &mimeType
	file.CoverSource = strPtr(models.DataSourceManual)
	file.CoverImageFilename = &coverFilename
```

Remove now-unused imports if any (`io`, `net/http` are still used for `copyFile` / response). Run `goimports -w pkg/books/handlers_cover_page.go`.

- [ ] **Step 3: Run the existing cover-page handler tests**

Run: `go test ./pkg/books/ -run TestUpdateFileCoverPage -v -count=1`

Expected: PASS (all existing tests — this is a pure refactor).

- [ ] **Step 4: Run the broader books test suite**

Run: `go test ./pkg/books/ -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/books/coverpage_extract.go pkg/books/handlers_cover_page.go
git commit -m "[Backend] Extract shared ExtractCoverPageToFile helper"
```

---

## Task 4: Wire `PageExtractor` dependency through `EnrichDeps`

**Files:**
- Modify: `pkg/plugins/handler.go` (add `pageExtractor` interface + field)
- Modify: `pkg/plugins/routes.go` (add `PageExtractor` field to public `EnrichDeps`, copy into private `enrichDeps`)
- Create: `pkg/books/plugin_page_extractor.go` (wrapper type implementing the interface)
- Modify: `pkg/server/server.go` (construct and wire the extractor)

- [ ] **Step 1: Define the interface in the plugins package**

In `pkg/plugins/handler.go`, add this interface definition near the other dep interfaces (around line 107, after `searchIndexer`):

```go
// pageExtractor renders a page from a page-based file (CBZ/PDF) and writes
// it as that file's cover image. Returns the cover filename (not a full path)
// and the MIME type of the extracted image.
type pageExtractor interface {
	ExtractCoverPage(file *models.File, bookFilepath string, page int) (filename, mimeType string, err error)
}
```

And add the field to `enrichDeps` (`pkg/plugins/handler.go:39`):

```go
type enrichDeps struct {
	bookStore       bookStore
	relStore        relationStore
	identStore      identifierStore
	personFinder    personFinder
	genreFinder     genreFinder
	tagFinder       tagFinder
	publisherFinder publisherFinder
	imprintFinder   imprintFinder
	searchIndexer   searchIndexer
	pageExtractor   pageExtractor
}
```

- [ ] **Step 2: Add the public field on `EnrichDeps` and copy it in both registration sites**

In `pkg/plugins/routes.go`:

Add to `EnrichDeps` (line 12):

```go
type EnrichDeps struct {
	BookStore       bookStore
	RelStore        relationStore
	IdentStore      identifierStore
	PersonFinder    personFinder
	GenreFinder     genreFinder
	TagFinder       tagFinder
	PublisherFinder publisherFinder
	ImprintFinder   imprintFinder
	SearchIndexer   searchIndexer
	PageExtractor   pageExtractor
}
```

In both `RegisterRoutesWithGroup` (line 28-39) and `RegisterIdentifyRoutes` (line 70-81), add `pageExtractor: ed.PageExtractor,` at the end of the `enrichDeps{...}` literal.

- [ ] **Step 3: Create the wrapper type in the books package**

Create `pkg/books/plugin_page_extractor.go`:

```go
package books

import (
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdfpages"
)

// PluginPageExtractor adapts ExtractCoverPageToFile to the plugins package's
// pageExtractor interface so plugin-provided cover_page values can be applied
// without the plugins package needing to depend on books or the page caches
// directly.
type PluginPageExtractor struct {
	cbzCache *cbzpages.Cache
	pdfCache *pdfpages.Cache
	log      logger.Logger
}

// NewPluginPageExtractor constructs a page extractor backed by its own
// cbzpages and pdfpages caches. Multiple instances pointing at the same
// cache directory are safe; the page cache writes are idempotent.
func NewPluginPageExtractor(cbzCache *cbzpages.Cache, pdfCache *pdfpages.Cache) *PluginPageExtractor {
	return &PluginPageExtractor{
		cbzCache: cbzCache,
		pdfCache: pdfCache,
		log:      logger.New(),
	}
}

// ExtractCoverPage satisfies the plugins.pageExtractor interface.
func (p *PluginPageExtractor) ExtractCoverPage(file *models.File, bookFilepath string, page int) (string, string, error) {
	return ExtractCoverPageToFile(file, bookFilepath, page, p.cbzCache, p.pdfCache, p.log)
}
```

The codebase uses `logger.New()` (no arguments) — see `pkg/plugins/hooks.go:776` for an example.

- [ ] **Step 4: Wire it in `pkg/server/server.go`**

In `pkg/server/server.go`, before `enrichDeps := &plugins.EnrichDeps{...}` (around line 200), construct the page caches and the extractor. The caches need to be created here so the server owns their lifetime (the books handler already creates its own inside `RegisterRoutesWithGroup`; a second instance here pointing at the same `cfg.CacheDir` is safe). Add:

```go
	cbzCache := cbzpages.NewCache(cfg.CacheDir)
	pdfCache := pdfpages.NewCache(cfg.CacheDir, cfg.PDFRenderDPI, cfg.PDFRenderQuality)
	pageExtractor := books.NewPluginPageExtractor(cbzCache, pdfCache)
```

Then add `PageExtractor: pageExtractor,` to the `plugins.EnrichDeps{...}` literal (after `SearchIndexer`).

Add the imports `"github.com/shishobooks/shisho/pkg/cbzpages"` and `"github.com/shishobooks/shisho/pkg/pdfpages"` at the top of `server.go` if not already present.

- [ ] **Step 5: Build the whole module to catch wiring errors**

Run: `go build ./...`

Expected: no output (clean build). If there are errors about unresolved references or unused imports, resolve them.

- [ ] **Step 6: Run plugins tests to confirm the new field doesn't break existing tests**

Run: `go test ./pkg/plugins/ -count=1`

Expected: PASS (existing tests should be unaffected — they construct `enrichDeps` without the new field, which defaults to nil).

- [ ] **Step 7: Commit**

```bash
git add pkg/plugins/handler.go pkg/plugins/routes.go pkg/books/plugin_page_extractor.go pkg/server/server.go
git commit -m "[Backend] Wire PageExtractor dependency through plugin EnrichDeps"
```

---

## Task 5: Apply `coverPage` in `persistMetadata` — CBZ happy path

**Files:**
- Modify: `pkg/plugins/handler.go:1870-1896` (replace the current cover-data block with a file-type-gated branch)
- Test: `pkg/plugins/handler_persist_metadata_test.go` (add new test cases + stub extractor)

- [ ] **Step 1: Add a stub `PageExtractor` for tests**

Append to `pkg/plugins/handler_persist_metadata_test.go` (top level, near the existing `stubBookStoreForPersist`):

```go
// stubPageExtractor records calls and returns a fixed (filename, mimeType).
// Set `wantErr` to simulate a failed extraction.
type stubPageExtractor struct {
	calls    []stubPageExtractorCall
	filename string
	mimeType string
	wantErr  error
}

type stubPageExtractorCall struct {
	FileID       int
	BookFilepath string
	Page         int
}

func (s *stubPageExtractor) ExtractCoverPage(file *models.File, bookFilepath string, page int) (string, string, error) {
	s.calls = append(s.calls, stubPageExtractorCall{FileID: file.ID, BookFilepath: bookFilepath, Page: page})
	if s.wantErr != nil {
		return "", "", s.wantErr
	}
	return s.filename, s.mimeType, nil
}
```

- [ ] **Step 2: Write a failing test for CBZ `coverPage` happy path**

Append to `pkg/plugins/handler_persist_metadata_test.go`:

```go
func TestPersistMetadata_CoverPage_CBZ_HappyPath(t *testing.T) {
	t.Parallel()

	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "comic.cbz")
	require.NoError(t, os.WriteFile(filePath, []byte("fake cbz"), 0600))

	pageCount := 10
	file := &models.File{
		ID:        1,
		BookID:    1,
		Filepath:  filePath,
		FileType:  models.FileTypeCBZ,
		PageCount: &pageCount,
	}
	book := &models.Book{
		ID:        1,
		LibraryID: 1,
		Filepath:  libraryDir,
		Files:     []*models.File{file},
	}

	extractor := &stubPageExtractor{filename: "comic.cbz.cover.jpg", mimeType: "image/jpeg"}

	h := &handler{
		enrich: &enrichDeps{
			bookStore:     &stubBookStoreForPersist{book: book},
			pageExtractor: extractor,
		},
	}

	page := 3
	md := &mediafile.ParsedMetadata{CoverPage: &page}

	err := h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	require.Len(t, extractor.calls, 1)
	assert.Equal(t, 1, extractor.calls[0].FileID)
	assert.Equal(t, 3, extractor.calls[0].Page)

	require.NotNil(t, file.CoverPage)
	assert.Equal(t, 3, *file.CoverPage)
	require.NotNil(t, file.CoverImageFilename)
	assert.Equal(t, "comic.cbz.cover.jpg", *file.CoverImageFilename)
	require.NotNil(t, file.CoverMimeType)
	assert.Equal(t, "image/jpeg", *file.CoverMimeType)
	require.NotNil(t, file.CoverSource)
	assert.Equal(t, models.PluginDataSource("test", "plugin-id"), *file.CoverSource)
}
```

- [ ] **Step 3: Run and confirm it fails**

Run: `go test ./pkg/plugins/ -run TestPersistMetadata_CoverPage_CBZ_HappyPath -v`

Expected: FAIL. `CoverPage` and `CoverImageFilename` will still be nil — `persistMetadata` doesn't handle `md.CoverPage` yet.

- [ ] **Step 4: Update `persistMetadata` to handle `coverPage` for page-based files**

In `pkg/plugins/handler.go`, replace the current block at lines 1870-1896 (`// Apply cover data ...` through the closing brace) with a file-type-gated branch:

```go
	// Apply cover data. Precedence is strict: page-based files (CBZ, PDF)
	// only accept coverPage; other formats only accept coverData / coverUrl.
	if targetFile != nil {
		if models.IsPageBasedFileType(targetFile.FileType) {
			// Page-based: apply coverPage, silently ignore coverData/coverUrl.
			if md.CoverPage != nil {
				page := *md.CoverPage
				switch {
				case page < 0:
					log.Warn("plugin-provided coverPage is negative, skipping", logger.Data{"file_id": targetFile.ID, "cover_page": page})
				case targetFile.PageCount == nil:
					log.Warn("plugin-provided coverPage skipped: page count unknown", logger.Data{"file_id": targetFile.ID, "cover_page": page})
				case page >= *targetFile.PageCount:
					log.Warn("plugin-provided coverPage is out of range, skipping", logger.Data{"file_id": targetFile.ID, "cover_page": page, "page_count": *targetFile.PageCount})
				case h.enrich.pageExtractor == nil:
					log.Warn("plugin-provided coverPage skipped: no page extractor configured", logger.Data{"file_id": targetFile.ID})
				default:
					coverFilename, mimeType, extractErr := h.enrich.pageExtractor.ExtractCoverPage(targetFile, book.Filepath, page)
					if extractErr != nil {
						log.Warn("failed to extract plugin-provided cover page", logger.Data{"file_id": targetFile.ID, "cover_page": page, "error": extractErr.Error()})
					} else {
						targetFile.CoverPage = &page
						targetFile.CoverImageFilename = &coverFilename
						targetFile.CoverMimeType = &mimeType
						source := models.PluginDataSource(pluginScope, pluginID)
						targetFile.CoverSource = &source
						fileColumns = append(fileColumns, "cover_page", "cover_image_filename", "cover_mime_type", "cover_source")
					}
				}
			}
		} else {
			// Non-page-based: existing coverData write path.
			if len(md.CoverData) > 0 {
				coverDir := fileutils.ResolveCoverDirForWrite(book.Filepath, targetFile.Filepath)
				coverBaseName := filepath.Base(targetFile.Filepath) + ".cover"

				normalizedData, normalizedMime, _ := fileutils.NormalizeImage(md.CoverData, md.CoverMimeType)
				coverExt := ".png"
				if normalizedMime == md.CoverMimeType {
					coverExt = md.CoverExtension()
				}

				coverFilename := coverBaseName + coverExt
				coverFilepath := filepath.Join(coverDir, coverFilename)

				if err := os.WriteFile(coverFilepath, normalizedData, 0600); err != nil {
					log.Warn("failed to write cover file", logger.Data{"error": err.Error()})
				} else {
					targetFile.CoverImageFilename = &coverFilename
					fileColumns = append(fileColumns, "cover_image_filename")
				}
			}
		}
	}
```

Confirm imports still include `os`, `path/filepath`, `fileutils`, and `logger` (they should already be there).

`models.PluginDataSource(scope, id)` is defined at `pkg/models/data-source.go:46` and returns a string like `"plugin:scope/id"`.

- [ ] **Step 5: Run the new test to confirm it passes**

Run: `go test ./pkg/plugins/ -run TestPersistMetadata_CoverPage_CBZ_HappyPath -v`

Expected: PASS.

- [ ] **Step 6: Run the full `handler_persist_metadata_test.go` to confirm no regression**

Run: `go test ./pkg/plugins/ -run TestPersistMetadata -v -count=1`

Expected: PASS (including the existing `TestPersistMetadata_CoverWrite_RootLevelFile_SyntheticBookPath`).

- [ ] **Step 7: Commit**

```bash
git add pkg/plugins/handler.go pkg/plugins/handler_persist_metadata_test.go
git commit -m "[Backend] Apply plugin-provided coverPage to CBZ/PDF files"
```

---

## Task 6: Add remaining persist tests (PDF, bounds, precedence)

All of these should pass against the implementation from Task 5 — they exercise different paths through the new branch.

**Files:**
- Test: `pkg/plugins/handler_persist_metadata_test.go`

- [ ] **Step 1: Add PDF happy path test**

Append to `pkg/plugins/handler_persist_metadata_test.go`:

```go
func TestPersistMetadata_CoverPage_PDF_HappyPath(t *testing.T) {
	t.Parallel()

	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "book.pdf")
	require.NoError(t, os.WriteFile(filePath, []byte("fake pdf"), 0600))

	pageCount := 100
	file := &models.File{
		ID: 2, BookID: 2, Filepath: filePath, FileType: models.FileTypePDF, PageCount: &pageCount,
	}
	book := &models.Book{ID: 2, LibraryID: 1, Filepath: libraryDir, Files: []*models.File{file}}

	extractor := &stubPageExtractor{filename: "book.pdf.cover.jpg", mimeType: "image/jpeg"}
	h := &handler{enrich: &enrichDeps{bookStore: &stubBookStoreForPersist{book: book}, pageExtractor: extractor}}

	page := 7
	md := &mediafile.ParsedMetadata{CoverPage: &page}

	err := h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	require.Len(t, extractor.calls, 1)
	assert.Equal(t, 7, extractor.calls[0].Page)
	require.NotNil(t, file.CoverPage)
	assert.Equal(t, 7, *file.CoverPage)
}
```

- [ ] **Step 2: Add out-of-bounds tests**

```go
func TestPersistMetadata_CoverPage_OutOfBounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		pageCount *int
		page      int
	}{
		{"negative", intPtr(10), -1},
		{"page equals count", intPtr(5), 5},
		{"page above count", intPtr(5), 99},
		{"page count unknown", nil, 3},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			libraryDir := t.TempDir()
			filePath := filepath.Join(libraryDir, "comic.cbz")
			require.NoError(t, os.WriteFile(filePath, []byte("fake cbz"), 0600))

			file := &models.File{
				ID: 1, BookID: 1, Filepath: filePath, FileType: models.FileTypeCBZ, PageCount: tc.pageCount,
			}
			book := &models.Book{ID: 1, LibraryID: 1, Filepath: libraryDir, Files: []*models.File{file}}
			extractor := &stubPageExtractor{filename: "x", mimeType: "image/jpeg"}
			h := &handler{enrich: &enrichDeps{bookStore: &stubBookStoreForPersist{book: book}, pageExtractor: extractor}}

			md := &mediafile.ParsedMetadata{CoverPage: &tc.page}

			err := h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
			require.NoError(t, err)

			assert.Empty(t, extractor.calls, "extractor should not be called for invalid page")
			assert.Nil(t, file.CoverPage, "file.CoverPage should remain unchanged")
			assert.Nil(t, file.CoverImageFilename, "file.CoverImageFilename should remain unchanged")
		})
	}
}

func intPtr(v int) *int { return &v }
```

If `intPtr` is already defined in the test package, drop the helper definition. (Check with `grep -n "func intPtr" pkg/plugins`.)

- [ ] **Step 3: Add precedence tests**

```go
// Plugin returns both coverPage and coverData for a CBZ file — coverPage wins,
// coverData is silently ignored.
func TestPersistMetadata_CoverPage_CBZ_BeatsCoverData(t *testing.T) {
	t.Parallel()

	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "comic.cbz")
	require.NoError(t, os.WriteFile(filePath, []byte("fake cbz"), 0600))

	pageCount := 10
	file := &models.File{
		ID: 1, BookID: 1, Filepath: filePath, FileType: models.FileTypeCBZ, PageCount: &pageCount,
	}
	book := &models.Book{ID: 1, LibraryID: 1, Filepath: libraryDir, Files: []*models.File{file}}
	extractor := &stubPageExtractor{filename: "comic.cbz.cover.jpg", mimeType: "image/jpeg"}
	h := &handler{enrich: &enrichDeps{bookStore: &stubBookStoreForPersist{book: book}, pageExtractor: extractor}}

	page := 2
	md := &mediafile.ParsedMetadata{
		CoverPage:     &page,
		CoverData:     makePersistTestJPEG(400, 600),
		CoverMimeType: "image/jpeg",
	}

	err := h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	// coverPage path taken
	require.Len(t, extractor.calls, 1)
	require.NotNil(t, file.CoverPage)
	assert.Equal(t, 2, *file.CoverPage)

	// coverData file must NOT have been written alongside the file
	_, err = os.Stat(filepath.Join(libraryDir, "comic.cbz.cover.png"))
	assert.True(t, os.IsNotExist(err), "coverData should not have been written for a page-based file")
}

// Plugin returns coverPage for a non-page-based format (EPUB) — coverPage is
// silently ignored, coverData (if provided) is applied.
func TestPersistMetadata_CoverPage_EPUB_Ignored(t *testing.T) {
	t.Parallel()

	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "book.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0600))

	file := &models.File{
		ID: 1, BookID: 1, Filepath: filePath, FileType: models.FileTypeEPUB,
	}
	book := &models.Book{ID: 1, LibraryID: 1, Filepath: libraryDir, Files: []*models.File{file}}
	extractor := &stubPageExtractor{filename: "x", mimeType: "image/jpeg"}
	h := &handler{enrich: &enrichDeps{bookStore: &stubBookStoreForPersist{book: book}, pageExtractor: extractor}}

	page := 3
	md := &mediafile.ParsedMetadata{
		CoverPage:     &page,
		CoverData:     makePersistTestJPEG(400, 600),
		CoverMimeType: "image/jpeg",
	}

	err := h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	// Extractor must not be called for non-page-based files
	assert.Empty(t, extractor.calls)
	// file.CoverPage must remain unchanged
	assert.Nil(t, file.CoverPage)
	// coverData write path ran — CoverImageFilename is set
	require.NotNil(t, file.CoverImageFilename)
	assert.Equal(t, "book.epub.cover.jpg", *file.CoverImageFilename)
}
```

- [ ] **Step 4: Run all new tests**

Run: `go test ./pkg/plugins/ -run TestPersistMetadata_CoverPage -v -count=1`

Expected: PASS for all cases.

- [ ] **Step 5: Run the whole plugins + books packages**

Run: `go test ./pkg/plugins/ ./pkg/books/ -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/plugins/handler_persist_metadata_test.go
git commit -m "[Backend] Test cover_page persist: PDF, bounds, precedence"
```

---

## Task 7: Update docs

**Files:**
- Modify: `website/docs/plugins/development.md`
- Modify: `pkg/plugins/CLAUDE.md`

- [ ] **Step 1: Read the current plugin dev docs cover section**

Run: `grep -n "coverPage\|coverData\|coverUrl" website/docs/plugins/development.md` and read the surrounding section to find the right spot.

- [ ] **Step 2: Add the cover_page rules to `website/docs/plugins/development.md`**

Under the `cover` field group description (wherever `coverData`, `coverMimeType`, `coverPage`, `coverUrl` are documented), add a subsection:

```markdown
### Cover page selection (CBZ and PDF only)

For page-based file formats — CBZ and PDF — the cover is derived from a page
of the file itself, not from an arbitrary image. Plugins can tell Shisho which
page to use by returning `coverPage` (a 0-indexed page number).

**Precedence (strict, file-type-gated):**

- **CBZ/PDF:** Only `coverPage` is applied. `coverData` and `coverUrl` are
  silently ignored for these formats. A plugin that returns only `coverData`
  for a CBZ/PDF will not change the cover.
- **EPUB/M4B and other formats:** Only `coverData` / `coverUrl` are applied.
  `coverPage` is silently ignored.

**Bounds:** If `coverPage` is negative, greater than or equal to the file's
page count, or the page count is unknown, the value is skipped with a warning
in the server logs — Shisho will not fall through to `coverData`.

**Cover source:** When `coverPage` is applied, the rendered page is saved as
the file's cover image on disk, and `cover_source` is set to
`plugin:<scope>/<id>`. This mirrors the behavior of the manual page-picker UI
(which uses `manual` as the source).

Example search result from a metadata enricher:

```javascript
return {
  results: [{
    title: "Saga #1",
    authors: [{ name: "Brian K. Vaughan", role: "writer" }],
    coverPage: 3, // The cover image is on page 3 of this CBZ, not page 0.
    confidence: 0.95
  }]
};
```
```

- [ ] **Step 3: Add a note to `pkg/plugins/CLAUDE.md`**

Find the fileParser example near line 150 (the block that mentions `coverPage: 0`) and add a one-liner beneath that field:

```markdown
      coverPage: 0,                           // CBZ/PDF only; silently ignored for other formats, skipped with warning when out of range
```

Also find the "Logical field groupings" section (already says `cover` controls `coverData`, `coverMimeType`, `coverPage`, `coverUrl`) and append after that list item:

```markdown
- **`coverPage` precedence:** For CBZ/PDF, only `coverPage` is applied (`coverData`/`coverUrl` ignored). For other formats, only `coverData`/`coverUrl` are applied (`coverPage` ignored). Out-of-range pages are skipped with a warning.
```

- [ ] **Step 4: Commit**

```bash
git add website/docs/plugins/development.md pkg/plugins/CLAUDE.md
git commit -m "[Docs] Document plugin-settable cover_page for CBZ/PDF"
```

---

## Task 8: Final validation

- [ ] **Step 1: Run the project-wide check**

Run: `mise check:quiet`

Expected: one-line pass summary. If any step fails, address the specific issue and re-run.

- [ ] **Step 2: Verify `mise tygo` isn't flagging new type drift**

Run: `mise tygo`

Expected: "skipping, outputs are up-to-date" — no Go type changes were made that affect the generated TypeScript. If output is generated, inspect whether the changes are expected; if not, something unrelated changed.

- [ ] **Step 3: Push to remote (only if the user asks you to)**

Stop here. Do not push, rebase, or open a PR unless the user explicitly requests it.
