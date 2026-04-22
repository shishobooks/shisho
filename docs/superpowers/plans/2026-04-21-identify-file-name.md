# Identify File Name Sync + Scan/Identify Consistency — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a book is identified, the target main file's `Name` tracks the identified title, and the scan and identify paths stop diverging on volume normalization, description HTML stripping, and whitespace trimming.

**Architecture:** Four targeted edits. Changes 1 and 4 extend `persistMetadata` in `pkg/plugins/handler.go` to mirror the identified title onto the main file's `Name`/`NameSource` and to trim Publisher/Imprint/URL. Changes 2 and 3 narrow `scan_unified.go`'s title volume normalization to file/filepath sources and add HTML stripping to the metadata description branch.

**Tech Stack:** Go (Echo, Bun, testify). Follow the project's TDD Red→Green→Refactor rule: write the test, confirm it fails for the right reason, implement, confirm it passes.

**Reference:** Spec at `docs/superpowers/specs/2026-04-21-identify-file-name-design.md`.

---

## File Structure

**Modified:**
- `pkg/plugins/handler.go` — `persistMetadata`: mirror title → main file `Name` (Change 1); trim Publisher/Imprint/URL (Change 4).
- `pkg/worker/scan_unified.go` — gate volume normalization on source priority (Change 2); strip HTML in description metadata branch (Change 3).
- `pkg/plugins/handler_apply_metadata_test.go` — new tests for Changes 1 and 4.
- `pkg/worker/scan_unified_test.go` — new tests for Changes 2 and 3.

**Not modified:**
- `pkg/worker/scan.go`'s `generateCBZFileName` — kept as-is for titleless CBZ fallback.
- `OrganizeBookFiles` in `pkg/books/service.go` — already early-returns when `OrganizeFileStructure` is disabled, which is the correct gate for "update DB name regardless, rename on disk only when enabled."

---

## Task 1: Identify mirrors title onto main file's Name (Change 1)

**Files:**
- Test: `pkg/plugins/handler_apply_metadata_test.go`
- Modify: `pkg/plugins/handler.go:1688-1696`

- [ ] **Step 1.1: Understand existing test helpers**

Read `pkg/plugins/handler_apply_metadata_test.go:17-105`. Note `stubBookStoreForApply`, `newApplyTestHandler`, `newApplyEchoContext`, `newApplyTestBook`. These stubs provide a minimal `bookStore` whose `UpdateFile` is a no-op — assertions must be made against the in-memory `*models.File` the handler mutates before flushing, not against a re-fetched file. The stubs' `RetrieveBook` always returns the same `*models.Book` pointer, so the book's `Files` slice is the place to assert file-level state.

- [ ] **Step 1.2: Add a helper for building a book with a main file**

`newApplyTestBook` only sets Title/Filepath. We need the book to have a file attached so `applyMetadata` can pick it as the target. Add a helper to `handler_apply_metadata_test.go` near the other helpers (after `newApplyTestBook`):

```go
// newApplyTestBookWithFile builds a book with a single attached main file.
// The returned file pointer is the same one persistMetadata will mutate, so
// tests can assert on its fields directly after applyMetadata returns.
func newApplyTestBookWithFile(t *testing.T, title string, fileType string) (*models.Book, *models.File) {
	t.Helper()
	book := newApplyTestBook(t, title)
	file := &models.File{
		ID:        1,
		BookID:    book.ID,
		LibraryID: book.LibraryID,
		Filepath:  filepath.Join(book.Filepath, "main"+"."+fileType),
		FileType:  fileType,
		FileRole:  models.FileRoleMain,
	}
	book.Files = []*models.File{file}
	return book, file
}
```

Add `"path/filepath"` to the imports if it isn't there already (check line 3-15; if absent, add it alongside the others).

- [ ] **Step 1.3: Write the failing test for main-file Name mirror**

Append to `pkg/plugins/handler_apply_metadata_test.go`:

```go
func TestApplyMetadata_UpdatesMainFileName_WhenTitleChanges(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Old Title", models.FileTypeEPUB)
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{"title": "New Title"})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	require.NotNil(t, file.Name, "main file Name should be set")
	assert.Equal(t, "New Title", *file.Name)
	require.NotNil(t, file.NameSource, "main file NameSource should be set")
	assert.Equal(t, "plugin:test/enricher", *file.NameSource)
}
```

- [ ] **Step 1.4: Run the test, confirm it fails**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/plugins/ -run TestApplyMetadata_UpdatesMainFileName_WhenTitleChanges -v
```
Expected: FAIL with `main file Name should be set` (pointer is nil).

- [ ] **Step 1.5: Implement Change 1 in `persistMetadata`**

Open `pkg/plugins/handler.go`. Find the Title block at lines 1688-1696:

```go
// Title
title := strings.TrimSpace(md.Title)
if title != "" {
	book.Title = title
	book.TitleSource = pluginSource
	book.SortTitle = sortname.ForTitle(title)
	book.SortTitleSource = pluginSource
	columns = append(columns, "title", "title_source", "sort_title", "sort_title_source")
}
```

Replace with:

```go
// Title
title := strings.TrimSpace(md.Title)
if title != "" {
	book.Title = title
	book.TitleSource = pluginSource
	book.SortTitle = sortname.ForTitle(title)
	book.SortTitleSource = pluginSource
	columns = append(columns, "title", "title_source", "sort_title", "sort_title_source")

	// Mirror the identified title onto the target main file's Name so
	// file organization and downloads reflect it. Supplements keep their
	// own filename-based label.
	if targetFile != nil && targetFile.FileRole == models.FileRoleMain {
		titleCopy := title
		targetFile.Name = &titleCopy
		targetFile.NameSource = &pluginSource
		fileColumns = append(fileColumns, "name", "name_source")
	}
}
```

**Why `titleCopy`:** `title` is a loop-free local, but using `&title` would tie the stored pointer to the loop-variable identity for the rest of the function. Taking an explicit copy before `&` makes the pointer's lifetime obvious and decouples it from any later reassignment. Existing scan code uses the same pattern.

Note: `fileColumns` is declared further down at line 1831 (`var fileColumns []string`). Since this new block runs before that declaration, move the `var fileColumns []string` declaration up so it's visible here. Grep for `var fileColumns []string` in this file — it should appear exactly once. Move it to just below the Title block (before the Subtitle block) so both the new code and the existing file-level code can append to it.

Actual change: at the current location (~line 1831), delete the `var fileColumns []string` line. Just before line 1688 (the Title comment), add:

```go
// Accumulate file-level column updates so Title/Narrator/Publisher/etc.
// can all contribute, then flush once at the end.
var fileColumns []string
```

- [ ] **Step 1.6: Run the test, confirm it passes**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/plugins/ -run TestApplyMetadata_UpdatesMainFileName_WhenTitleChanges -v
```
Expected: PASS.

- [ ] **Step 1.7: Run the full plugins test suite**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/plugins/ -v
```
Expected: all tests pass. If `fileColumns` move broke an ordering assumption, investigate.

- [ ] **Step 1.8: Write the supplement-guard test**

Append to `handler_apply_metadata_test.go`:

```go
func TestApplyMetadata_DoesNotUpdateSupplementFileName(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Old Title", models.FileTypePDF)
	file.FileRole = models.FileRoleSupplement
	originalName := "Supplement.pdf"
	file.Name = &originalName

	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{"title": "New Title"})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	require.NotNil(t, file.Name)
	assert.Equal(t, "Supplement.pdf", *file.Name, "supplement Name must not be overwritten with book title")
}
```

- [ ] **Step 1.9: Run the supplement test and confirm it passes**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/plugins/ -run TestApplyMetadata_DoesNotUpdateSupplementFileName -v
```
Expected: PASS (because the guard prevents the overwrite).

- [ ] **Step 1.10: Write the volume-preservation test**

Append:

```go
func TestApplyMetadata_PreservesVolumeNotation_CBZ(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Old Title", models.FileTypeCBZ)
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{"title": "Naruto v1"})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	assert.Equal(t, "Naruto v1", book.Title, "book.Title must not be volume-normalized on identify")
	require.NotNil(t, file.Name)
	assert.Equal(t, "Naruto v1", *file.Name, "file.Name must mirror the verbatim title")
}
```

- [ ] **Step 1.11: Run the volume test and confirm it passes**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/plugins/ -run TestApplyMetadata_PreservesVolumeNotation_CBZ -v
```
Expected: PASS.

- [ ] **Step 1.12: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && git add pkg/plugins/handler.go pkg/plugins/handler_apply_metadata_test.go && git commit -m "$(cat <<'EOF'
[Backend] Mirror identified title onto main file Name in applyMetadata

When a user applies identified metadata to a book, the target main file's
Name and NameSource now track the new title so OrganizeBookFiles renames
the file on disk (when enabled) and downloads reflect the identified title.
Supplements keep their own filename-based label via a FileRole guard.
EOF
)"
```

---

## Task 2: Identify trims Publisher, Imprint, and URL (Change 4)

**Files:**
- Test: `pkg/plugins/handler_apply_metadata_test.go`
- Modify: `pkg/plugins/handler.go:1859-1888`

- [ ] **Step 2.1: Write the failing test**

The existing stub `stubBookStoreForApply` doesn't track `UpdateFile` columns, and `publisherFinder`/`imprintFinder` aren't wired in `newApplyTestHandler`. We need to extend the handler wiring and stubs to assert on trimmed values.

Append helper stubs to `handler_apply_metadata_test.go`:

```go
// stubPublisherFinder records the name FindOrCreatePublisher was called with.
type stubPublisherFinder struct {
	lastName string
}

func (s *stubPublisherFinder) FindOrCreatePublisher(_ context.Context, name string, _ int) (*models.Publisher, error) {
	s.lastName = name
	return &models.Publisher{ID: 1, Name: name}, nil
}

// stubImprintFinder records the name FindOrCreateImprint was called with.
type stubImprintFinder struct {
	lastName string
}

func (s *stubImprintFinder) FindOrCreateImprint(_ context.Context, name string, _ int) (*models.Imprint, error) {
	s.lastName = name
	return &models.Imprint{ID: 1, Name: name}, nil
}
```

Extend `newApplyTestHandler` to accept optional finders:

```go
// newApplyTestHandlerWithFinders wires publisher/imprint finders so tests can
// assert on the exact names persistMetadata passed to FindOrCreate*.
func newApplyTestHandlerWithFinders(store *stubBookStoreForApply, pub *stubPublisherFinder, imp *stubImprintFinder) *handler {
	h := newApplyTestHandler(store)
	h.enrich.publisherFinder = pub
	h.enrich.imprintFinder = imp
	return h
}
```

Append the test:

```go
func TestApplyMetadata_TrimsPublisherImprintURL(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Book", models.FileTypeEPUB)
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	pub := &stubPublisherFinder{}
	imp := &stubImprintFinder{}
	h := newApplyTestHandlerWithFinders(store, pub, imp)
	c := newApplyEchoContext(t, map[string]any{
		"publisher": "  Some Publisher  ",
		"imprint":   "  Penguin Classics  ",
		"url":       "  https://example.com  ",
	})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	assert.Equal(t, "Some Publisher", pub.lastName, "publisher name must be trimmed before FindOrCreate")
	assert.Equal(t, "Penguin Classics", imp.lastName, "imprint name must be trimmed before FindOrCreate")
	require.NotNil(t, file.URL)
	assert.Equal(t, "https://example.com", *file.URL, "file URL must be trimmed")
}
```

- [ ] **Step 2.2: Run the test and confirm it fails**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/plugins/ -run TestApplyMetadata_TrimsPublisherImprintURL -v
```
Expected: FAIL — `pub.lastName` is `"  Some Publisher  "`, not trimmed.

- [ ] **Step 2.3: Implement the trims in `persistMetadata`**

In `pkg/plugins/handler.go`, find the Publisher block starting at ~line 1859 and replace:

```go
// Publisher (file-level, applied to target file)
if md.Publisher != "" && targetFile != nil && h.enrich.publisherFinder != nil {
	publisher, pErr := h.enrich.publisherFinder.FindOrCreatePublisher(ctx, md.Publisher, book.LibraryID)
	if pErr != nil {
		log.Warn("failed to find/create publisher", logger.Data{"name": md.Publisher, "error": pErr.Error()})
	} else {
		targetFile.PublisherID = &publisher.ID
		targetFile.PublisherSource = &pluginSource
		fileColumns = append(fileColumns, "publisher_id", "publisher_source")
	}
}
```

With:

```go
// Publisher (file-level, applied to target file)
publisherName := strings.TrimSpace(md.Publisher)
if publisherName != "" && targetFile != nil && h.enrich.publisherFinder != nil {
	publisher, pErr := h.enrich.publisherFinder.FindOrCreatePublisher(ctx, publisherName, book.LibraryID)
	if pErr != nil {
		log.Warn("failed to find/create publisher", logger.Data{"name": publisherName, "error": pErr.Error()})
	} else {
		targetFile.PublisherID = &publisher.ID
		targetFile.PublisherSource = &pluginSource
		fileColumns = append(fileColumns, "publisher_id", "publisher_source")
	}
}
```

Find the Imprint block at ~line 1872 and replace:

```go
// Imprint (file-level, applied to target file)
if md.Imprint != "" && targetFile != nil && h.enrich.imprintFinder != nil {
	imprint, iErr := h.enrich.imprintFinder.FindOrCreateImprint(ctx, md.Imprint, book.LibraryID)
	if iErr != nil {
		log.Warn("failed to find/create imprint", logger.Data{"name": md.Imprint, "error": iErr.Error()})
	} else {
		targetFile.ImprintID = &imprint.ID
		targetFile.ImprintSource = &pluginSource
		fileColumns = append(fileColumns, "imprint_id", "imprint_source")
	}
}
```

With:

```go
// Imprint (file-level, applied to target file)
imprintName := strings.TrimSpace(md.Imprint)
if imprintName != "" && targetFile != nil && h.enrich.imprintFinder != nil {
	imprint, iErr := h.enrich.imprintFinder.FindOrCreateImprint(ctx, imprintName, book.LibraryID)
	if iErr != nil {
		log.Warn("failed to find/create imprint", logger.Data{"name": imprintName, "error": iErr.Error()})
	} else {
		targetFile.ImprintID = &imprint.ID
		targetFile.ImprintSource = &pluginSource
		fileColumns = append(fileColumns, "imprint_id", "imprint_source")
	}
}
```

Find the URL block at ~line 1883 and replace:

```go
// URL (file-level, applied to target file)
if md.URL != "" && targetFile != nil {
	targetFile.URL = &md.URL
	targetFile.URLSource = &pluginSource
	fileColumns = append(fileColumns, "url", "url_source")
}
```

With:

```go
// URL (file-level, applied to target file)
url := strings.TrimSpace(md.URL)
if url != "" && targetFile != nil {
	targetFile.URL = &url
	targetFile.URLSource = &pluginSource
	fileColumns = append(fileColumns, "url", "url_source")
}
```

- [ ] **Step 2.4: Run the test and confirm it passes**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/plugins/ -run TestApplyMetadata_TrimsPublisherImprintURL -v
```
Expected: PASS.

- [ ] **Step 2.5: Run the full plugins package**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/plugins/ -v
```
Expected: all tests pass.

- [ ] **Step 2.6: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && git add pkg/plugins/handler.go pkg/plugins/handler_apply_metadata_test.go && git commit -m "$(cat <<'EOF'
[Backend] Trim Publisher, Imprint, URL whitespace in applyMetadata

Matches the scan path, which already trims Publisher/Imprint via TrimSpace.
URL is trimmed here so plugins that accidentally emit trailing whitespace
don't store dirty values.
EOF
)"
```

---

## Task 3: Scan gates volume normalization on source priority (Change 2)

**Files:**
- Test: `pkg/worker/scan_unified_test.go`
- Modify: `pkg/worker/scan_unified.go:809-813`

- [ ] **Step 3.1: Verify you understand the existing code path**

Read `pkg/worker/scan_unified.go:800-830` for the book-title update block. The `title` local is computed from `metadata.Title`, then the normalization runs unconditionally, then `shouldUpdateScalar` decides whether to overwrite `book.Title`. We're gating the normalization step.

Also read `pkg/models/data-source.go:32-60` — `GetDataSourcePriority` handles the `plugin:scope/id` prefix case.

- [ ] **Step 3.2: Write the failing test for plugin-source title preservation**

Append to `pkg/worker/scan_unified_test.go`:

```go
func TestScanFileCore_Title_PluginSource_NotVolumeNormalized(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Placeholder",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Placeholder",
		AuthorSource: models.DataSourceFilepath,
	}
	require.NoError(t, tc.bookService.CreateBook(tc.ctx, book))

	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "naruto.cbz"),
		FileType:      models.FileTypeCBZ,
		FilesizeBytes: 1000,
	}
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, file))

	metadata := &mediafile.ParsedMetadata{
		Title:      "Naruto v1",
		DataSource: models.PluginDataSource("test", "enricher"),
	}

	_, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true, nil, nil)
	require.NoError(t, err)

	reloaded, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	assert.Equal(t, "Naruto v1", reloaded.Title, "plugin-sourced title must not be volume-normalized")
}
```

Check the top of `scan_unified_test.go` for imports — confirm `books`, `models`, `mediafile`, `testgen` are already imported (they are based on existing tests). No new imports expected.

- [ ] **Step 3.3: Run the test and confirm it fails for the right reason**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/worker/ -run TestScanFileCore_Title_PluginSource_NotVolumeNormalized -v
```
Expected: FAIL with `expected "Naruto v1", got "Naruto v001"` (or similar — the v1 got normalized to v001).

- [ ] **Step 3.4: Implement the source gate**

Open `pkg/worker/scan_unified.go`. Find lines 809-813:

```go
title := strings.TrimSpace(metadata.Title)
// Normalize volume indicators (e.g., "#007" -> "v7") for CBZ files
if normalizedTitle, hasVolume := fileutils.NormalizeVolumeInTitle(title, file.FileType); hasVolume {
	title = normalizedTitle
}
titleSource := metadata.SourceForField("title")
```

Replace with:

```go
title := strings.TrimSpace(metadata.Title)
titleSource := metadata.SourceForField("title")
// Normalize volume indicators (e.g., "#007" -> "v7") for CBZ files only when
// the title came from the file itself or its path. Plugin/sidecar/manual
// titles are user-curated and must not be rewritten
// (e.g., "Naruto v1" must not become "Naruto v001").
if models.GetDataSourcePriority(titleSource) >= models.DataSourceFileMetadataPriority {
	if normalizedTitle, hasVolume := fileutils.NormalizeVolumeInTitle(title, file.FileType); hasVolume {
		title = normalizedTitle
	}
}
```

- [ ] **Step 3.5: Run the test and confirm it passes**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/worker/ -run TestScanFileCore_Title_PluginSource_NotVolumeNormalized -v
```
Expected: PASS.

- [ ] **Step 3.6: Add a regression test confirming file_metadata still normalizes**

Append to `scan_unified_test.go`:

```go
func TestScanFileCore_Title_FileMetadataSource_StillVolumeNormalized(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Placeholder",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Placeholder",
		AuthorSource: models.DataSourceFilepath,
	}
	require.NoError(t, tc.bookService.CreateBook(tc.ctx, book))

	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "comic.cbz"),
		FileType:      models.FileTypeCBZ,
		FilesizeBytes: 1000,
	}
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, file))

	// File-embedded metadata with raw volume notation.
	metadata := &mediafile.ParsedMetadata{
		Title:      "Some Title #7",
		DataSource: models.DataSourceCBZMetadata,
	}

	_, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true, nil, nil)
	require.NoError(t, err)

	reloaded, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	assert.Equal(t, "Some Title v007", reloaded.Title, "file_metadata-sourced volume notation must still be normalized")
}
```

- [ ] **Step 3.7: Run the file_metadata test and confirm it passes**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/worker/ -run TestScanFileCore_Title_FileMetadataSource_StillVolumeNormalized -v
```
Expected: PASS.

- [ ] **Step 3.8: Run the full worker test suite**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/worker/ -timeout 300s
```
Expected: all tests pass. If any existing test asserted normalization on a non-file source, update its expected value — the test was encoding the old, now-broken behavior.

- [ ] **Step 3.9: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && git add pkg/worker/scan_unified.go pkg/worker/scan_unified_test.go && git commit -m "$(cat <<'EOF'
[Backend] Only normalize CBZ title volume notation for file/filepath sources

Plugin, sidecar, and manual titles are user-curated and shouldn't be
rewritten. A plugin returning "Naruto v1" now stays "Naruto v1" instead
of becoming "Naruto v001". Filepath titles continue to be normalized via
applyFilepathFallbacks at their derivation site.
EOF
)"
```

---

## Task 4: Scan strips HTML from description metadata (Change 3)

**Files:**
- Test: `pkg/worker/scan_unified_test.go`
- Modify: `pkg/worker/scan_unified.go:886-888`

- [ ] **Step 4.1: Write the failing test**

Append to `scan_unified_test.go`:

```go
func TestScanFileCore_Description_StripsHTMLFromMetadata(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Book",
		AuthorSource: models.DataSourceFilepath,
	}
	require.NoError(t, tc.bookService.CreateBook(tc.ctx, book))

	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "book.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, file))

	metadata := &mediafile.ParsedMetadata{
		Title:       "Book",
		Description: "<p>Hello <b>world</b></p>",
		DataSource:  models.PluginDataSource("test", "enricher"),
	}

	_, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true, nil, nil)
	require.NoError(t, err)

	reloaded, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	require.NotNil(t, reloaded.Description)
	assert.Equal(t, "Hello world", *reloaded.Description, "description HTML must be stripped from scan metadata")
}
```

- [ ] **Step 4.2: Run the test and confirm it fails**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/worker/ -run TestScanFileCore_Description_StripsHTMLFromMetadata -v
```
Expected: FAIL — description is stored as `"<p>Hello <b>world</b></p>"`.

- [ ] **Step 4.3: Implement the strip**

Open `pkg/worker/scan_unified.go`. Find line 887:

```go
// Description (from metadata)
description := strings.TrimSpace(metadata.Description)
```

Replace with:

```go
// Description (from metadata) — strip HTML so enricher-provided markup
// doesn't leak into the stored description. Matches the sidecar branch
// below and the identify apply path.
description := htmlutil.StripTags(strings.TrimSpace(metadata.Description))
```

Check imports at the top of the file — `htmlutil` should already be imported (the sidecar branch at line 907 uses it). Confirm:

```bash
grep -n htmlutil pkg/worker/scan_unified.go | head
```

If not present, add the import. The package path is `github.com/shishobooks/shisho/pkg/htmlutil` (match whatever the sidecar line uses).

- [ ] **Step 4.4: Run the test and confirm it passes**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/worker/ -run TestScanFileCore_Description_StripsHTMLFromMetadata -v
```
Expected: PASS.

- [ ] **Step 4.5: Run the full worker test suite**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && go test ./pkg/worker/ -timeout 300s
```
Expected: all tests pass.

- [ ] **Step 4.6: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && git add pkg/worker/scan_unified.go pkg/worker/scan_unified_test.go && git commit -m "$(cat <<'EOF'
[Backend] Strip HTML from description in scan metadata branch

Matches the sidecar branch and the identify apply path. Plugin enrichers
can return HTML descriptions; without this, the HTML leaked into
book.Description only on the scan path.
EOF
)"
```

---

## Task 5: Verify and update docs

**Files:**
- Check: `website/docs/metadata.md`, any identify-related docs
- Run: `mise tygo`, `mise check:quiet`

- [ ] **Step 5.1: Run tygo for type generation**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && mise tygo
```
Expected: completes successfully. "skipping, outputs are up-to-date" is normal and not an error. No code changed Go types that flow into TS in this plan, so no `.ts` changes expected — but run it to be sure.

- [ ] **Step 5.2: Run the full project check**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && mise check:quiet
```
Expected: all checks pass with a pass/fail summary.

- [ ] **Step 5.3: Skim docs for pages that describe the changed behavior**

Grep the docs for anything that could be stale:

```bash
grep -rn "normaliz\|volume\|identify\|file.Name\|file name" website/docs/ | head -30
```

Look for pages that describe:
- How book titles are normalized (volume notation).
- The identify workflow and what it updates.

Target pages per spec:
- `website/docs/metadata.md` — if it describes title normalization, update to note it applies only to file-embedded and filepath sources.
- Any page describing identify — mention that applying now renames the main file (when OrganizeFileStructure is enabled).

If a page needs an update, edit it in-place. If no page documents these behaviors, that's fine — no new docs required by this change.

- [ ] **Step 5.4: Commit any doc changes**

If step 5.3 produced edits:

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && git add website/docs/ && git commit -m "$(cat <<'EOF'
[Docs] Note source-gated title normalization and identify file rename
EOF
)"
```

If no doc changes were needed, skip this step.

- [ ] **Step 5.5: Final sanity check — run mise check:quiet once more**

```bash
cd /Users/robinjoseph/.worktrees/shisho/identify-file-name && mise check:quiet
```
Expected: all checks pass. This is the last verification before the branch is ready to ship.

---

## Done

All four changes are in; both identify and scan paths trim consistently, normalize only file/filepath titles, strip HTML from descriptions, and identify's target main file tracks the new title. The branch is ready for review.
