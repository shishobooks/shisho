# Reset to File Metadata Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make "Reset to file metadata" fully wipe book/file metadata before re-scanning, so fields not present in the source file are cleared rather than retained.

**Architecture:** Add a `Reset` flag to ScanOptions, a `resetBookFileState` helper that wipes metadata columns and relations, and an `applyFilepathFallbacks` helper extracted from `scanFileCreateNew`. The reset path is: parse file → apply filepath fallbacks → wipe DB state → delete on-disk cover → re-extract cover → run scanFileCore. No changes to `shouldUpdateScalar`/`shouldUpdateRelationship`.

**Tech Stack:** Go (Echo, Bun ORM, SQLite), React/TypeScript frontend, Docusaurus docs site.

**Spec:** `docs/superpowers/specs/2026-04-12-reset-to-file-metadata-design.md`

---

### Task 1: Add `Reset` field to ScanOptions and plumbing

**Files:**
- Modify: `pkg/worker/scan_unified.go:120-135` (worker ScanOptions)
- Modify: `pkg/books/handlers.go:40-45` (books ScanOptions)
- Modify: `pkg/books/validators.go:67-82` (resolveScanMode)
- Modify: `pkg/books/handlers.go:1727-1733` (resyncFile handler)
- Modify: `pkg/books/handlers.go:1780-1786` (resyncBook handler)
- Modify: `pkg/worker/scan_unified.go:3308-3316` (Scan adapter)
- Modify: `pkg/worker/scan_unified.go:566-573` (scanBook → scanFileByID delegation)

- [ ] **Step 1: Add `Reset` to worker ScanOptions**

In `pkg/worker/scan_unified.go`, add the field after `SkipPlugins`:

```go
type ScanOptions struct {
	// Entry points (mutually exclusive - exactly one must be set)
	FilePath string // Batch scan: discover/create by path
	FileID   int    // Single file resync: file already in DB
	BookID   int    // Book resync: scan all files in book

	// Context (required for FilePath mode)
	LibraryID int

	// Behavior
	ForceRefresh  bool // Bypass priority checks, overwrite all metadata
	SkipPlugins   bool // Skip enricher plugins, use only file-embedded metadata
	Reset         bool // Wipe all metadata before scanning (reset to file-only state)
	BookResetDone bool // Book-level wipe already done by scanBook (skip in scanFileByID)

	// Logging (optional, for batch scan job context)
	JobLog *joblogs.JobLogger
}
```

- [ ] **Step 2: Add `Reset` to books ScanOptions**

In `pkg/books/handlers.go`:

```go
type ScanOptions struct {
	FileID       int  // Single file resync: file already in DB
	BookID       int  // Book resync: scan all files in book
	ForceRefresh bool // Bypass priority checks, overwrite all metadata
	SkipPlugins  bool // Skip enricher plugins, use only file-embedded metadata
	Reset        bool // Wipe all metadata before scanning (reset to file-only state)
}
```

- [ ] **Step 3: Update `resolveScanMode` to return `reset`**

In `pkg/books/validators.go`:

```go
func (p ResyncPayload) resolveScanMode() (forceRefresh, skipPlugins, reset bool) {
	switch p.Mode {
	case "refresh":
		return true, false, false
	case "reset":
		return true, true, true
	case "scan", "":
		return p.Refresh, false, false
	default:
		return false, false, false
	}
}
```

- [ ] **Step 4: Update resyncFile handler**

In `pkg/books/handlers.go`, update the call site at line ~1728:

```go
	forceRefresh, skipPlugins, reset := params.resolveScanMode()
	result, err := h.scanner.Scan(ctx, ScanOptions{
		FileID:       id,
		ForceRefresh: forceRefresh,
		SkipPlugins:  skipPlugins,
		Reset:        reset,
	})
```

- [ ] **Step 5: Update resyncBook handler**

In `pkg/books/handlers.go`, update the call site at line ~1781:

```go
	forceRefresh, skipPlugins, reset := params.resolveScanMode()
	result, err := h.scanner.Scan(ctx, ScanOptions{
		BookID:       id,
		ForceRefresh: forceRefresh,
		SkipPlugins:  skipPlugins,
		Reset:        reset,
	})
```

- [ ] **Step 6: Update the Scan adapter**

In `pkg/worker/scan_unified.go`, update the `Scan` method at line ~3310:

```go
	internalOpts := ScanOptions{
		FileID:       opts.FileID,
		BookID:       opts.BookID,
		ForceRefresh: opts.ForceRefresh,
		SkipPlugins:  opts.SkipPlugins,
		Reset:        opts.Reset,
	}
```

- [ ] **Step 7: Thread `Reset` through scanBook → scanFileByID**

In `pkg/worker/scan_unified.go`, update the loop in `scanBook` at line ~568:

```go
		fileResult, err := w.scanFileByID(ctx, ScanOptions{
			FileID:        file.ID,
			ForceRefresh:  opts.ForceRefresh,
			SkipPlugins:   opts.SkipPlugins,
			Reset:         opts.Reset,
			BookResetDone: opts.Reset,
			JobLog:        opts.JobLog,
		}, cache)
```

- [ ] **Step 8: Verify compilation**

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book && go build ./...`
Expected: Compiles successfully.

- [ ] **Step 9: Commit**

```bash
git add pkg/worker/scan_unified.go pkg/books/handlers.go pkg/books/validators.go
git commit -m "[Backend] Add Reset field to ScanOptions and plumb through handlers"
```

---

### Task 2: Extract `applyFilepathFallbacks` helper

**Files:**
- Modify: `pkg/worker/scan_unified.go` (extract helper, refactor `scanFileCreateNew`)
- Test: `pkg/worker/scan_unified_test.go`

This refactors the filepath-fallback logic from `scanFileCreateNew` into a reusable helper so the reset path can also derive title/authors/narrators/series from the filepath when the file has no embedded metadata.

- [ ] **Step 1: Write the failing test**

Add to `pkg/worker/scan_unified_test.go`:

```go
func TestApplyFilepathFallbacks_PopulatesTitleFromFilepath(t *testing.T) {
	t.Parallel()

	metadata := &mediafile.ParsedMetadata{
		DataSource: models.DataSourceEPUBMetadata,
	}

	applyFilepathFallbacks(metadata, "/library/[Author Name] Book Title.epub", "/library/[Author Name] Book Title", "epub", true)

	assert.Equal(t, "Book Title", metadata.Title)
	assert.Equal(t, models.DataSourceFilepath, metadata.SourceForField("title"))
}

func TestApplyFilepathFallbacks_PreservesExistingTitle(t *testing.T) {
	t.Parallel()

	metadata := &mediafile.ParsedMetadata{
		Title:      "Embedded Title",
		DataSource: models.DataSourceEPUBMetadata,
	}

	applyFilepathFallbacks(metadata, "/library/[Author] Something.epub", "/library/[Author] Something", "epub", true)

	assert.Equal(t, "Embedded Title", metadata.Title)
}

func TestApplyFilepathFallbacks_PopulatesAuthorsFromFilepath(t *testing.T) {
	t.Parallel()

	metadata := &mediafile.ParsedMetadata{
		DataSource: models.DataSourceEPUBMetadata,
	}

	applyFilepathFallbacks(metadata, "/library/[Jane Doe] Book.epub", "/library/[Jane Doe] Book", "epub", true)

	require.Len(t, metadata.Authors, 1)
	assert.Equal(t, "Jane Doe", metadata.Authors[0].Name)
}

func TestApplyFilepathFallbacks_PreservesExistingAuthors(t *testing.T) {
	t.Parallel()

	metadata := &mediafile.ParsedMetadata{
		Authors:    []mediafile.ParsedAuthor{{Name: "Embedded Author"}},
		DataSource: models.DataSourceEPUBMetadata,
	}

	applyFilepathFallbacks(metadata, "/library/[Other Author] Book.epub", "/library/[Other Author] Book", "epub", true)

	require.Len(t, metadata.Authors, 1)
	assert.Equal(t, "Embedded Author", metadata.Authors[0].Name)
}

func TestApplyFilepathFallbacks_PopulatesNarratorsFromFilepath(t *testing.T) {
	t.Parallel()

	metadata := &mediafile.ParsedMetadata{
		DataSource: models.DataSourceM4BMetadata,
	}

	applyFilepathFallbacks(metadata, "/library/[Author] Title {Narrator Name}.m4b", "/library/[Author] Title", "m4b", true)

	require.Len(t, metadata.Narrators, 1)
	assert.Equal(t, "Narrator Name", metadata.Narrators[0].Name)
}

func TestApplyFilepathFallbacks_PopulatesSeriesFromTitle(t *testing.T) {
	t.Parallel()

	metadata := &mediafile.ParsedMetadata{
		DataSource: models.DataSourceCBZMetadata,
	}

	applyFilepathFallbacks(metadata, "/library/My Series v3.cbz", "/library/My Series v3", "cbz", true)

	assert.NotEmpty(t, metadata.Series)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book && go test ./pkg/worker/ -run "TestApplyFilepathFallbacks" -v`
Expected: FAIL — `applyFilepathFallbacks` is not defined.

- [ ] **Step 3: Implement `applyFilepathFallbacks`**

Add to `pkg/worker/scan_unified.go`, near the existing `deriveInitialTitle` function (around line 2470):

```go
// applyFilepathFallbacks populates empty metadata fields from the filepath.
// This fills in title, authors, narrators, and series using the same logic
// that scanFileCreateNew uses when creating a book for the first time.
// Fields already present in metadata are not overwritten.
func applyFilepathFallbacks(metadata *mediafile.ParsedMetadata, filePath, bookPath, fileType string, isRootLevelFile bool) {
	if metadata == nil {
		return
	}

	setSource := func(field, source string) {
		if metadata.FieldDataSources == nil {
			metadata.FieldDataSources = make(map[string]string)
		}
		metadata.FieldDataSources[field] = source
	}

	// Title fallback
	if strings.TrimSpace(metadata.Title) == "" {
		metadata.Title = deriveInitialTitle(filePath, isRootLevelFile, nil)
		setSource("title", models.DataSourceFilepath)
	}

	// Authors fallback
	if len(metadata.Authors) == 0 {
		filepathAuthors := extractAuthorsFromFilepath(bookPath, isRootLevelFile)
		for _, name := range filepathAuthors {
			metadata.Authors = append(metadata.Authors, mediafile.ParsedAuthor{Name: name})
		}
		if len(metadata.Authors) > 0 {
			setSource("authors", models.DataSourceFilepath)
		}
	}

	// Narrators fallback
	if len(metadata.Narrators) == 0 {
		filepathNarrators := extractNarratorsFromFilepath(filePath, bookPath, isRootLevelFile)
		for _, name := range filepathNarrators {
			metadata.Narrators = append(metadata.Narrators, mediafile.ParsedNarrator{Name: name})
		}
		if len(metadata.Narrators) > 0 {
			setSource("narrators", models.DataSourceFilepath)
		}
	}

	// Series fallback from title (e.g., "My Series v3" → series="My Series", number="3")
	if metadata.Series == "" {
		title := metadata.Title
		if seriesName, volumeNumber, ok := fileutils.ExtractSeriesFromTitle(title, fileType); ok {
			metadata.Series = seriesName
			metadata.SeriesNumber = volumeNumber
			setSource("series", models.DataSourceFilepath)
		}
	}
}
```

- [ ] **Step 4: Verify SourceForField works with FieldDataSources**

The implementation uses `metadata.FieldDataSources` map directly (there is no `SetFieldSource` method). The `SourceForField(field)` method on `ParsedMetadata` (`pkg/mediafile/mediafile.go:97`) checks `FieldDataSources[field]` first, then falls back to `DataSource`. Verify the local `setSource` helper in `applyFilepathFallbacks` correctly initializes the map.

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book && go vet ./pkg/worker/`
Expected: No errors.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book && go test ./pkg/worker/ -run "TestApplyFilepathFallbacks" -v`
Expected: All 6 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/worker/scan_unified.go pkg/worker/scan_unified_test.go
git commit -m "[Backend] Extract applyFilepathFallbacks helper for reset mode"
```

---

### Task 3: Implement `resetBookFileState` helper

**Files:**
- Modify: `pkg/worker/scan_unified.go` (add helper)
- Test: `pkg/worker/scan_unified_test.go`

- [ ] **Step 1: Write the failing test for book-level reset**

Add to `pkg/worker/scan_unified_test.go`:

```go
func TestResetBookFileState_ClearsBookMetadata(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	// Create a library and a book directory
	libDir := testgen.TempLibraryDir(t)
	bookDir := testgen.CreateSubDir(t, libDir, "Test Book")
	tc.createLibrary([]string{libDir})

	// Create an EPUB with embedded metadata
	epubPath := testgen.GenerateEPUB(t, bookDir, "test.epub", testgen.EPUBOptions{
		Title:   "Original Title",
		Authors: []string{"Original Author"},
		HasCover: true,
	})

	// Scan the file to populate DB
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath:  epubPath,
		LibraryID: 1,
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, result.Book)
	require.NotNil(t, result.File)

	bookID := result.Book.ID
	fileID := result.File.ID

	// Manually set book fields that won't come from file metadata
	subtitle := "A Subtitle"
	description := "A Description"
	manualSource := models.DataSourceManual
	result.Book.Subtitle = &subtitle
	result.Book.SubtitleSource = &manualSource
	result.Book.Description = &description
	result.Book.DescriptionSource = &manualSource
	err = tc.bookService.UpdateBook(tc.ctx, result.Book, books.UpdateBookOptions{
		Columns: []string{"subtitle", "subtitle_source", "description", "description_source"},
	})
	require.NoError(t, err)

	// Add a genre and tag to the book
	genre, err := tc.worker.genreService.FindOrCreateGenre(tc.ctx, "Fantasy", 1)
	require.NoError(t, err)
	err = tc.bookService.CreateBookGenre(tc.ctx, &models.BookGenre{BookID: bookID, GenreID: genre.ID})
	require.NoError(t, err)
	tag, err := tc.worker.tagService.FindOrCreateTag(tc.ctx, "favorite", 1)
	require.NoError(t, err)
	err = tc.bookService.CreateBookTag(tc.ctx, &models.BookTag{BookID: bookID, TagID: tag.ID})
	require.NoError(t, err)

	// Set manual file-level fields
	language := "en"
	languageSource := models.DataSourceManual
	abridged := true
	abridgedSource := models.DataSourceManual
	result.File.Language = &language
	result.File.LanguageSource = &languageSource
	result.File.Abridged = &abridged
	result.File.AbridgedSource = &abridgedSource
	err = tc.bookService.UpdateFile(tc.ctx, result.File, books.UpdateFileOptions{
		Columns: []string{"language", "language_source", "abridged", "abridged_source"},
	})
	require.NoError(t, err)

	// Reload book and file with relations
	book, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)
	file, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, fileID)
	require.NoError(t, err)

	// Verify preconditions
	assert.NotNil(t, book.Subtitle)
	assert.NotNil(t, book.Description)
	assert.NotEmpty(t, book.BookGenres)
	assert.NotEmpty(t, book.BookTags)
	assert.NotNil(t, file.Language)
	assert.NotNil(t, file.Abridged)

	// Call resetBookFileState (skipBookWipe=false for standalone call)
	err = tc.worker.resetBookFileState(tc.ctx, book, file, false)
	require.NoError(t, err)

	// Reload and verify book fields are cleared
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)
	assert.Nil(t, book.Subtitle, "subtitle should be cleared")
	assert.Nil(t, book.SubtitleSource, "subtitle source should be cleared")
	assert.Nil(t, book.Description, "description should be cleared")
	assert.Nil(t, book.DescriptionSource, "description source should be cleared")
	assert.Empty(t, book.BookGenres, "genres should be cleared")
	assert.Empty(t, book.BookTags, "tags should be cleared")
	assert.Empty(t, book.Authors, "authors should be cleared")
	assert.Empty(t, book.BookSeries, "series should be cleared")

	// Reload and verify file fields are cleared
	file, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, fileID)
	require.NoError(t, err)
	assert.Nil(t, file.Language, "language should be cleared")
	assert.Nil(t, file.LanguageSource, "language source should be cleared")
	assert.Nil(t, file.Abridged, "abridged should be cleared")
	assert.Nil(t, file.AbridgedSource, "abridged source should be cleared")
	assert.Nil(t, file.Name, "name should be cleared")
	assert.Nil(t, file.URL, "url should be cleared")
	assert.Nil(t, file.ReleaseDate, "release date should be cleared")
	assert.Nil(t, file.PublisherID, "publisher should be cleared")
	assert.Nil(t, file.ImprintID, "imprint should be cleared")
	assert.Empty(t, file.Narrators, "narrators should be cleared")
	assert.Empty(t, file.Identifiers, "identifiers should be cleared")
}

func TestResetBookFileState_PreservesIdentityFields(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libDir := testgen.TempLibraryDir(t)
	bookDir := testgen.CreateSubDir(t, libDir, "Test Book")
	tc.createLibrary([]string{libDir})

	epubPath := testgen.GenerateEPUB(t, bookDir, "test.epub", testgen.EPUBOptions{
		Title:    "My Title",
		HasCover: true,
	})

	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath:  epubPath,
		LibraryID: 1,
	}, nil)
	require.NoError(t, err)

	bookID := result.Book.ID
	fileID := result.File.ID
	bookFilepath := result.Book.Filepath
	fileFilepath := result.File.Filepath
	fileType := result.File.FileType
	fileRole := result.File.FileRole
	libraryID := result.File.LibraryID
	primaryFileID := result.Book.PrimaryFileID

	book, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)
	file, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, fileID)
	require.NoError(t, err)

	err = tc.worker.resetBookFileState(tc.ctx, book, file, false)
	require.NoError(t, err)

	// Reload and verify identity fields are preserved
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)
	assert.Equal(t, bookID, book.ID)
	assert.Equal(t, bookFilepath, book.Filepath)
	assert.Equal(t, libraryID, book.LibraryID)
	assert.Equal(t, primaryFileID, book.PrimaryFileID)

	file, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, fileID)
	require.NoError(t, err)
	assert.Equal(t, fileID, file.ID)
	assert.Equal(t, fileFilepath, file.Filepath)
	assert.Equal(t, fileType, file.FileType)
	assert.Equal(t, fileRole, file.FileRole)
	assert.Equal(t, libraryID, file.LibraryID)
	assert.Equal(t, bookID, file.BookID)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book && go test ./pkg/worker/ -run "TestResetBookFileState" -v`
Expected: FAIL — `resetBookFileState` is not defined.

- [ ] **Step 3: Implement `resetBookFileState`**

Add to `pkg/worker/scan_unified.go`:

```go
// resetBookFileState wipes all metadata from a book and file, preparing them
// for a fresh scan. Identity fields (IDs, filepath, library_id, file_type,
// file_role, primary_file_id) and intrinsic file properties (filesize, duration,
// bitrate, codec, page_count) are preserved. Cover files are deleted from disk.
// When skipBookWipe is true, only file-level state is reset (used by scanBook
// which handles the book-level wipe itself).
func (w *Worker) resetBookFileState(ctx context.Context, book *models.Book, file *models.File, skipBookWipe bool) error {
	if !skipBookWipe {
	// --- Book-level columns ---
	book.Subtitle = nil
	book.SubtitleSource = nil
	book.Description = nil
	book.DescriptionSource = nil
	book.AuthorSource = ""
	book.GenreSource = nil
	book.TagSource = nil

	bookColumns := []string{
		"subtitle", "subtitle_source",
		"description", "description_source",
		"author_source",
		"genre_source", "tag_source",
	}
	if err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{Columns: bookColumns}); err != nil {
		return errors.Wrap(err, "failed to clear book metadata")
	}

	// --- Book-level relations ---
	if err := w.bookService.DeleteAuthors(ctx, book.ID); err != nil {
		return errors.Wrap(err, "failed to delete book authors")
	}
	if err := w.bookService.DeleteBookSeries(ctx, book.ID); err != nil {
		return errors.Wrap(err, "failed to delete book series")
	}
	if err := w.bookService.DeleteBookGenres(ctx, book.ID); err != nil {
		return errors.Wrap(err, "failed to delete book genres")
	}
	if err := w.bookService.DeleteBookTags(ctx, book.ID); err != nil {
		return errors.Wrap(err, "failed to delete book tags")
	}
	} // end !skipBookWipe

	// --- File-level columns ---
	file.Name = nil
	file.NameSource = nil
	file.URL = nil
	file.URLSource = nil
	file.ReleaseDate = nil
	file.ReleaseDateSource = nil
	file.PublisherID = nil
	file.PublisherSource = nil
	file.ImprintID = nil
	file.ImprintSource = nil
	file.Language = nil
	file.LanguageSource = nil
	file.Abridged = nil
	file.AbridgedSource = nil
	file.ChapterSource = nil
	file.NarratorSource = nil
	file.IdentifierSource = nil

	fileColumns := []string{
		"name", "name_source",
		"url", "url_source",
		"release_date", "release_date_source",
		"publisher_id", "publisher_source",
		"imprint_id", "imprint_source",
		"language", "language_source",
		"abridged", "abridged_source",
		"chapter_source",
		"narrator_source", "identifier_source",
	}

	// Delete cover from disk before clearing cover columns
	if file.CoverImageFilename != nil && *file.CoverImageFilename != "" {
		coverPath := filepath.Join(filepath.Dir(file.Filepath), *file.CoverImageFilename)
		_ = os.Remove(coverPath)
	}

	file.CoverImageFilename = nil
	file.CoverMimeType = nil
	file.CoverSource = nil
	file.CoverPage = nil
	fileColumns = append(fileColumns,
		"cover_image_filename", "cover_mime_type", "cover_source", "cover_page",
	)

	if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{Columns: fileColumns}); err != nil {
		return errors.Wrap(err, "failed to clear file metadata")
	}

	// --- File-level relations ---
	if _, err := w.bookService.DeleteNarratorsForFile(ctx, file.ID); err != nil {
		return errors.Wrap(err, "failed to delete file narrators")
	}
	if _, err := w.bookService.DeleteIdentifiersForFile(ctx, file.ID); err != nil {
		return errors.Wrap(err, "failed to delete file identifiers")
	}
	if err := w.chapterService.DeleteChaptersForFile(ctx, file.ID); err != nil {
		return errors.Wrap(err, "failed to delete file chapters")
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book && go test ./pkg/worker/ -run "TestResetBookFileState" -v`
Expected: Both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/worker/scan_unified.go pkg/worker/scan_unified_test.go
git commit -m "[Backend] Add resetBookFileState helper to wipe metadata before reset scan"
```

---

### Task 4: Wire reset into `scanFileByID`

**Files:**
- Modify: `pkg/worker/scan_unified.go:280-495` (scanFileByID)
- Test: `pkg/worker/scan_unified_test.go`

- [ ] **Step 1: Write the failing integration test**

Add to `pkg/worker/scan_unified_test.go`:

```go
// TestScanFileByID_ResetMode_ClearsNonFileMetadata verifies that reset mode
// wipes metadata not present in the file (subtitle, genres, etc.) and
// re-populates from file-embedded metadata + filepath fallbacks.
func TestScanFileByID_ResetMode_ClearsNonFileMetadata(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libDir := testgen.TempLibraryDir(t)
	bookDir := testgen.CreateSubDir(t, libDir, "[Test Author] Reset Book")
	tc.createLibrary([]string{libDir})

	// Create an EPUB with only title and author embedded
	epubPath := testgen.GenerateEPUB(t, bookDir, "reset-test.epub", testgen.EPUBOptions{
		Title:   "Reset Book",
		Authors: []string{"Test Author"},
		HasCover: true,
	})

	// Initial scan
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath:  epubPath,
		LibraryID: 1,
	}, nil)
	require.NoError(t, err)
	bookID := result.Book.ID
	fileID := result.File.ID

	// Manually add metadata that the EPUB doesn't have
	subtitle := "Fake Subtitle"
	description := "Fake Description"
	language := "fr"
	abridged := true
	manualSource := models.DataSourceManual
	result.Book.Subtitle = &subtitle
	result.Book.SubtitleSource = &manualSource
	result.Book.Description = &description
	result.Book.DescriptionSource = &manualSource
	err = tc.bookService.UpdateBook(tc.ctx, result.Book, books.UpdateBookOptions{
		Columns: []string{"subtitle", "subtitle_source", "description", "description_source"},
	})
	require.NoError(t, err)

	result.File.Language = &language
	result.File.LanguageSource = &manualSource
	result.File.Abridged = &abridged
	result.File.AbridgedSource = &manualSource
	err = tc.bookService.UpdateFile(tc.ctx, result.File, books.UpdateFileOptions{
		Columns: []string{"language", "language_source", "abridged", "abridged_source"},
	})
	require.NoError(t, err)

	// Add a genre the EPUB doesn't have
	genre, err := tc.worker.genreService.FindOrCreateGenre(tc.ctx, "Romance", 1)
	require.NoError(t, err)
	err = tc.bookService.CreateBookGenre(tc.ctx, &models.BookGenre{BookID: bookID, GenreID: genre.ID})
	require.NoError(t, err)

	// Now reset
	resetResult, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FileID:       fileID,
		ForceRefresh: true,
		SkipPlugins:  true,
		Reset:        true,
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, resetResult.Book)

	// Reload book
	book, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)

	// Fields from the EPUB should be repopulated
	assert.Equal(t, "Reset Book", book.Title)
	assert.NotEmpty(t, book.Authors, "authors should be repopulated from EPUB")

	// Fields NOT in the EPUB should be cleared
	assert.Nil(t, book.Subtitle, "subtitle should be cleared")
	assert.Nil(t, book.Description, "description should be cleared")
	assert.Empty(t, book.BookGenres, "genres should be cleared")

	// File fields should be cleared
	file, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, fileID)
	require.NoError(t, err)
	assert.Nil(t, file.Language, "language should be cleared")
	assert.Nil(t, file.Abridged, "abridged should be cleared")

	// Cover should be re-extracted
	assert.NotNil(t, file.CoverImageFilename, "cover should be re-extracted")
}

// TestScanFileByID_ResetMode_FilepathFallbackTitle verifies that when the
// source file has no embedded title, reset mode falls back to the filepath.
func TestScanFileByID_ResetMode_FilepathFallbackTitle(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libDir := testgen.TempLibraryDir(t)
	bookDir := testgen.CreateSubDir(t, libDir, "Filepath Title Book")
	tc.createLibrary([]string{libDir})

	// Create a CBZ with no title in ComicInfo (just pages, no metadata)
	cbzPath := testgen.GenerateCBZ(t, bookDir, "my-comic.cbz", testgen.CBZOptions{
		PageCount:    3,
		HasComicInfo: false,
	})

	// Initial scan — title should come from directory name
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath:  cbzPath,
		LibraryID: 1,
	}, nil)
	require.NoError(t, err)
	bookID := result.Book.ID
	fileID := result.File.ID

	// Manually set a wrong title (simulating plugin misidentification)
	result.Book.Title = "Wrong Plugin Title"
	result.Book.TitleSource = models.DataSourcePlugin
	result.Book.SortTitle = "Wrong Plugin Title"
	result.Book.SortTitleSource = models.DataSourcePlugin
	err = tc.bookService.UpdateBook(tc.ctx, result.Book, books.UpdateBookOptions{
		Columns: []string{"title", "title_source", "sort_title", "sort_title_source"},
	})
	require.NoError(t, err)

	// Reset
	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{
		FileID:       fileID,
		ForceRefresh: true,
		SkipPlugins:  true,
		Reset:        true,
	}, nil)
	require.NoError(t, err)

	// Title should now be derived from directory name, not the wrong plugin title
	book, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)
	assert.Equal(t, "Filepath Title Book", book.Title)
	assert.NotEqual(t, "Wrong Plugin Title", book.Title)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book && go test ./pkg/worker/ -run "TestScanFileByID_ResetMode" -v`
Expected: FAIL — reset mode doesn't clear metadata yet.

- [ ] **Step 3: Wire reset into `scanFileByID`**

In `pkg/worker/scan_unified.go`, modify `scanFileByID`. After parsing metadata (line ~459) and retrieving the parent book (line ~462), before calling `runMetadataEnrichers`, add the reset logic:

```go
	// Get parent book for scanFileCore
	book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve parent book")
	}

	// Reset mode: wipe metadata and apply filepath fallbacks
	if opts.Reset {
		// Determine if this is a root-level file by checking whether the file's
		// parent directory is a library path (same logic as scanFileCreateNew line ~2086-2096).
		// For directory-based books, filepath.Dir(file.Filepath) == book.Filepath.
		// For root-level files, filepath.Dir(file.Filepath) is a library path, not book.Filepath.
		isRootLevelFile := filepath.Dir(file.Filepath) != book.Filepath

		// Apply filepath fallbacks so title/authors are populated even if file has none
		applyFilepathFallbacks(metadata, file.Filepath, book.Filepath, file.FileType, isRootLevelFile)

		// Wipe book and file metadata.
		// If BookResetDone is set (called from scanBook), skip the book-level wipe
		// because scanBook already did it once for all files.
		if err := w.resetBookFileState(ctx, book, file, opts.BookResetDone); err != nil {
			return nil, errors.Wrap(err, "failed to reset book/file state")
		}

		// Reload book and file after wipe (relations were deleted)
		book, err = w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
		if err != nil {
			return nil, errors.Wrap(err, "failed to reload book after reset")
		}
		file, err = w.bookService.RetrieveFileWithRelations(ctx, file.ID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to reload file after reset")
		}
	}

	// Run metadata enrichers after parsing
	if !opts.SkipPlugins {
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book && go test ./pkg/worker/ -run "TestScanFileByID_ResetMode" -v`
Expected: Both tests PASS.

- [ ] **Step 5: Run existing scan tests to check for regressions**

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book && go test ./pkg/worker/ -v -count=1 2>&1 | tail -20`
Expected: All existing tests still PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/worker/scan_unified.go pkg/worker/scan_unified_test.go
git commit -m "[Backend] Wire reset mode into scanFileByID with filepath fallbacks"
```

---

### Task 5: Wire reset into `scanBook` (book-level wipe once)

**Files:**
- Modify: `pkg/worker/scan_unified.go:501-601` (scanBook)
- Test: `pkg/worker/scan_unified_test.go`

For book-level reset, the book metadata should be wiped **once** before iterating files, not per-file (otherwise file 2's scan would wipe file 1's contributions).

- [ ] **Step 1: Write the failing test**

Add to `pkg/worker/scan_unified_test.go`:

```go
// TestScanBook_ResetMode_WipesBookOnce verifies that book-level reset
// wipes book metadata once and then re-populates from all files.
func TestScanBook_ResetMode_WipesBookOnce(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libDir := testgen.TempLibraryDir(t)
	bookDir := testgen.CreateSubDir(t, libDir, "[Author Name] Multi File Book")
	tc.createLibrary([]string{libDir})

	// Create two EPUBs in the same directory (same book)
	testgen.GenerateEPUB(t, bookDir, "file1.epub", testgen.EPUBOptions{
		Title:   "Multi File Book",
		Authors: []string{"Author Name"},
		HasCover: true,
	})
	testgen.GenerateEPUB(t, bookDir, "file2.epub", testgen.EPUBOptions{
		Title:   "Multi File Book",
		Authors: []string{"Author Name"},
		HasCover: true,
	})

	// Initial scan
	require.NoError(t, tc.runScan())

	booksList := tc.listBooks()
	require.Len(t, booksList, 1)
	bookID := booksList[0].ID

	// Add fake metadata
	subtitle := "Should Be Cleared"
	manualSource := models.DataSourceManual
	booksList[0].Subtitle = &subtitle
	booksList[0].SubtitleSource = &manualSource
	err := tc.bookService.UpdateBook(tc.ctx, booksList[0], books.UpdateBookOptions{
		Columns: []string{"subtitle", "subtitle_source"},
	})
	require.NoError(t, err)

	genre, err := tc.worker.genreService.FindOrCreateGenre(tc.ctx, "Thriller", 1)
	require.NoError(t, err)
	err = tc.bookService.CreateBookGenre(tc.ctx, &models.BookGenre{BookID: bookID, GenreID: genre.ID})
	require.NoError(t, err)

	// Reset the entire book
	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{
		BookID:       bookID,
		ForceRefresh: true,
		SkipPlugins:  true,
		Reset:        true,
	}, nil)
	require.NoError(t, err)

	// Verify book metadata is clean
	book, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)
	assert.Nil(t, book.Subtitle, "subtitle should be cleared")
	assert.Empty(t, book.BookGenres, "genres should be cleared")
	assert.Equal(t, "Multi File Book", book.Title, "title should be repopulated")
	assert.NotEmpty(t, book.Authors, "authors should be repopulated from files")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book && go test ./pkg/worker/ -run "TestScanBook_ResetMode" -v`
Expected: FAIL — scanBook doesn't handle reset yet.

- [ ] **Step 3: Add book-level reset to `scanBook`**

In `pkg/worker/scan_unified.go`, modify `scanBook`. After retrieving the book and checking for empty files (line ~523), add the book-level wipe before the file loop:

```go
	// Reset mode: wipe book-level metadata once before scanning files.
	// File-level resets happen per-file inside scanFileByID.
	// We wipe book metadata here so that per-file scans don't clobber each
	// other (file 2 wiping file 1's contributions).
	if opts.Reset {
		bookColumns := []string{
			"subtitle", "subtitle_source",
			"description", "description_source",
			"author_source",
			"genre_source", "tag_source",
		}
		book.Subtitle = nil
		book.SubtitleSource = nil
		book.Description = nil
		book.DescriptionSource = nil
		book.AuthorSource = ""
		book.GenreSource = nil
		book.TagSource = nil
		if err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{Columns: bookColumns}); err != nil {
			return nil, errors.Wrap(err, "failed to clear book metadata for reset")
		}
		if err := w.bookService.DeleteAuthors(ctx, book.ID); err != nil {
			return nil, errors.Wrap(err, "failed to delete book authors for reset")
		}
		if err := w.bookService.DeleteBookSeries(ctx, book.ID); err != nil {
			return nil, errors.Wrap(err, "failed to delete book series for reset")
		}
		if err := w.bookService.DeleteBookGenres(ctx, book.ID); err != nil {
			return nil, errors.Wrap(err, "failed to delete book genres for reset")
		}
		if err := w.bookService.DeleteBookTags(ctx, book.ID); err != nil {
			return nil, errors.Wrap(err, "failed to delete book tags for reset")
		}
	}

	// Initialize file results
	fileResults := make([]*ScanResult, 0, len(book.Files))
```

`scanBook` does the book-level wipe inline (same columns/relations as in `resetBookFileState`), then passes `BookResetDone: true` when delegating to `scanFileByID`, which causes `resetBookFileState` to skip the book-level wipe via `skipBookWipe=true`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book && go test ./pkg/worker/ -run "TestScanBook_ResetMode" -v`
Expected: PASS.

- [ ] **Step 5: Run all scan tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book && go test ./pkg/worker/ -v -count=1 2>&1 | tail -20`
Expected: All PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/worker/scan_unified.go pkg/worker/scan_unified_test.go
git commit -m "[Backend] Wire reset mode into scanBook with once-only book wipe"
```

---

### Task 6: Update frontend dialog description

**Files:**
- Modify: `app/components/library/RescanDialog.tsx:39-44`

- [ ] **Step 1: Update the reset mode description**

In `app/components/library/RescanDialog.tsx`, update the reset mode entry:

```tsx
  {
    value: "reset",
    label: "Reset to file metadata",
    description:
      "Clear all metadata and re-scan as if this were a brand new file, without plugins. Manual edits and enricher data will be removed.",
  },
```

- [ ] **Step 2: Verify frontend builds**

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book && pnpm build`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add app/components/library/RescanDialog.tsx
git commit -m "[Frontend] Update reset mode description to reflect full wipe behavior"
```

---

### Task 7: Update documentation

**Files:**
- Modify: `website/docs/metadata.md:152-156`

- [ ] **Step 1: Update the rescan section**

In `website/docs/metadata.md`, replace the reset bullet point at line 156:

```markdown
- **Reset to file metadata** — Clears all existing metadata (including manual edits) and re-scans the file from scratch, without running plugins. Fields not present in the source file are removed. The title and authors will fall back to the filepath if the file has no embedded values. Use this when plugin enrichment has misidentified a book and you want a clean slate.
```

- [ ] **Step 2: Verify docs build**

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book/website && pnpm build`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add website/docs/metadata.md
git commit -m "[Docs] Update reset-to-file-metadata description to explain full wipe behavior"
```

---

### Task 8: Run full validation

- [ ] **Step 1: Run `mise check:quiet`**

Run: `cd /Users/robinjoseph/.worktrees/shisho/reset-book && mise check:quiet`
Expected: All checks pass (Go tests, Go lint, JS lint).

- [ ] **Step 2: Fix any failures**

If any step fails, investigate and fix. Re-run until all pass.

- [ ] **Step 3: Final commit if needed**

Only commit if there were fixes needed from step 2.
