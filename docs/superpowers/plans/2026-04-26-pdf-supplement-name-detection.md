# PDF Supplement Name Detection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Auto-classify newly-discovered PDFs whose basename matches a configurable list of common supplement names (e.g. `Supplement.pdf`) as `FileRoleSupplement` instead of `FileRoleMain`, when a sibling main file exists in the same directory tree.

**Architecture:** Add a `PDFSupplementFilenames []string` config field with a default list. In `pkg/worker/scan_unified.go::scanFileCreateNew`, before the file row is built, decide whether to set `FileRoleSupplement` based on (1) `fileType == "pdf"`, (2) basename match against the configured names, and (3) presence of a non-PDF main-eligible sibling on disk OR an existing book row at the same `bookPath`. The "first file in a directory" rule is preserved automatically: if no sibling exists, the PDF stays main. Add a stable-sort in `ProcessScanJob` to push supplement-named PDFs after non-supplement files (defensive optimization).

**Tech Stack:** Go (Echo, Bun, koanf for config), `mise` for task running, testify for tests.

**Spec:** `docs/superpowers/specs/2026-04-26-pdf-supplement-name-detection-design.md`

**Subagent reminder:** Check the project's root `CLAUDE.md` and `pkg/CLAUDE.md` for rules that apply to your work — these contain critical conventions (TDD red-green-refactor, docs update requirements, `t.Parallel()` requirement, naming rules, commit message format `[Category] description`). Violations are review failures.

---

## Task 1: Add `PDFSupplementFilenames` config field with default

**Files:**
- Modify: `pkg/config/config.go`

The `Config` struct lives in `pkg/config/config.go`. The supplement-discovery section is already there (search for `// Supplement discovery settings`). Default values are set in `defaults()` and re-set in `NewForTest()`.

- [ ] **Step 1: Add the field to the `Config` struct**

Open `pkg/config/config.go` and find this block (currently around line 62-63):

```go
	// Supplement discovery settings
	SupplementExcludePatterns []string `koanf:"supplement_exclude_patterns" json:"supplement_exclude_patterns"`
```

Replace it with:

```go
	// Supplement discovery settings
	SupplementExcludePatterns []string `koanf:"supplement_exclude_patterns" json:"supplement_exclude_patterns"`
	PDFSupplementFilenames    []string `koanf:"pdf_supplement_filenames" json:"pdf_supplement_filenames"`
```

- [ ] **Step 2: Set the default in `defaults()`**

Find the `SupplementExcludePatterns:` line inside `defaults()` (currently around line 110):

```go
		SupplementExcludePatterns:     []string{".*", ".DS_Store", "Thumbs.db", "desktop.ini"},
```

Add this line right after it (keep the alignment the file uses — gofmt will handle column width):

```go
		PDFSupplementFilenames: []string{
			"supplement", "supplemental", "bonus", "bonus material", "bonus content",
			"companion", "notes", "liner notes", "errata", "booklet", "digital booklet",
			"appendix", "map", "maps", "insert", "guide", "reference",
			"cheat sheet", "cheatsheet", "cribsheet", "pamphlet", "extras",
		},
```

- [ ] **Step 3: Mirror the default in `NewForTest()`**

Find the `cfg.SupplementExcludePatterns = ...` line inside `NewForTest()` (currently around line 184). It's there because tests don't load the YAML, so they need the defaults set explicitly. Add right after it:

```go
	cfg.PDFSupplementFilenames = []string{
		"supplement", "supplemental", "bonus", "bonus material", "bonus content",
		"companion", "notes", "liner notes", "errata", "booklet", "digital booklet",
		"appendix", "map", "maps", "insert", "guide", "reference",
		"cheat sheet", "cheatsheet", "cribsheet", "pamphlet", "extras",
	}
```

- [ ] **Step 4: Add the same default to the test worker config**

Open `pkg/worker/testhelpers_test.go`. Find the `cfg := &config.Config{...}` block in `newTestContext()` (currently around line 100-103):

```go
	cfg := &config.Config{
		WorkerProcesses:           1,
		SupplementExcludePatterns: []string{".*", ".DS_Store", "Thumbs.db", "desktop.ini"},
	}
```

Replace with:

```go
	cfg := &config.Config{
		WorkerProcesses:           1,
		SupplementExcludePatterns: []string{".*", ".DS_Store", "Thumbs.db", "desktop.ini"},
		PDFSupplementFilenames: []string{
			"supplement", "supplemental", "bonus", "bonus material", "bonus content",
			"companion", "notes", "liner notes", "errata", "booklet", "digital booklet",
			"appendix", "map", "maps", "insert", "guide", "reference",
			"cheat sheet", "cheatsheet", "cribsheet", "pamphlet", "extras",
		},
	}
```

- [ ] **Step 5: Verify the package builds**

Run: `mise lint`
Expected: clean. (`mise lint` runs `golangci-lint run`; this is a Go-only change.)

- [ ] **Step 6: Verify worker tests still compile and pass**

Run: `go test ./pkg/config/... ./pkg/worker/...`
Expected: all tests pass. (Existing tests don't reference the new field, so they should be unaffected.)

- [ ] **Step 7: Commit**

```bash
git add pkg/config/config.go pkg/worker/testhelpers_test.go
git commit -m "[Backend] Add pdf_supplement_filenames config field"
```

---

## Task 2: Add `looksLikePDFSupplement` helper with unit tests

**Files:**
- Modify: `pkg/worker/scan.go` (add helper)
- Test: `pkg/worker/scan_helpers_test.go` (add test)

The matcher is a pure function — no I/O, no DB, no plugin manager. Lives next to the existing `isMainFileExtension` and `isShishoSpecialFile` helpers in `pkg/worker/scan.go`.

- [ ] **Step 1: Write the failing test**

Open `pkg/worker/scan_helpers_test.go` and append:

```go
func TestLooksLikePDFSupplement(t *testing.T) {
	t.Parallel()

	defaultNames := []string{
		"supplement", "supplemental", "bonus", "bonus material", "bonus content",
		"companion", "notes", "liner notes", "errata", "booklet", "digital booklet",
		"appendix", "map", "maps", "insert", "guide", "reference",
		"cheat sheet", "cheatsheet", "cribsheet", "pamphlet", "extras",
	}

	tests := []struct {
		name     string
		filename string
		names    []string
		want     bool
	}{
		{name: "exact match lowercase", filename: "supplement.pdf", names: defaultNames, want: true},
		{name: "exact match uppercase ext", filename: "Supplement.PDF", names: defaultNames, want: true},
		{name: "all caps basename", filename: "BONUS MATERIAL.pdf", names: defaultNames, want: true},
		{name: "trims surrounding whitespace", filename: "  supplement  .pdf", names: defaultNames, want: true},
		{name: "multi-word entry matches", filename: "liner notes.pdf", names: defaultNames, want: true},
		{name: "non-pdf extension does not match", filename: "supplement.txt", names: defaultNames, want: false},
		{name: "substring does not match", filename: "my book - supplement.pdf", names: defaultNames, want: false},
		{name: "unrelated name does not match", filename: "Companion Guide.pdf", names: defaultNames, want: false},
		{name: "empty names list disables matching", filename: "supplement.pdf", names: []string{}, want: false},
		{name: "nil names list disables matching", filename: "supplement.pdf", names: nil, want: false},
		{name: "custom list overrides default", filename: "extra.pdf", names: []string{"extra"}, want: true},
		{name: "no extension does not match", filename: "supplement", names: defaultNames, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := looksLikePDFSupplement(tt.filename, tt.names)
			assert.Equal(t, tt.want, got)
		})
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./pkg/worker/ -run TestLooksLikePDFSupplement -v`
Expected: compile error — `undefined: looksLikePDFSupplement`. This is the "Red" step of TDD.

- [ ] **Step 3: Implement `looksLikePDFSupplement` in `pkg/worker/scan.go`**

Open `pkg/worker/scan.go`. Find the existing helper `isShishoSpecialFile` (currently around line 124) and add this function right after it:

```go
// looksLikePDFSupplement returns true if filename has a .pdf extension and its
// basename (without extension, trimmed, lowercased) is an exact case-insensitive
// match for any entry in names. Returns false when names is empty/nil.
func looksLikePDFSupplement(filename string, names []string) bool {
	if len(names) == 0 {
		return false
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".pdf" {
		return false
	}
	basename := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(filename, filepath.Ext(filename))))
	if basename == "" {
		return false
	}
	for _, name := range names {
		if strings.ToLower(strings.TrimSpace(name)) == basename {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the test and confirm it passes**

Run: `go test ./pkg/worker/ -run TestLooksLikePDFSupplement -v`
Expected: all 12 sub-tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/worker/scan.go pkg/worker/scan_helpers_test.go
git commit -m "[Backend] Add looksLikePDFSupplement helper for filename matching"
```

---

## Task 3: Add `hasNonPDFMainSibling` helper with unit tests

**Files:**
- Modify: `pkg/worker/scan.go` (add helper)
- Test: `pkg/worker/scan_helpers_test.go` (add test)

This walks a directory recursively looking for any non-PDF main-eligible file. Used to decide whether a candidate supplement PDF has a real sibling main file. The plugin-extension set comes from `w.pluginManager.RegisteredFileExtensions()` (extensions without leading dots) — pass it as a parameter so the helper stays pure.

- [ ] **Step 1: Write the failing test**

Append to `pkg/worker/scan_helpers_test.go`:

```go
func TestHasNonPDFMainSibling(t *testing.T) {
	t.Parallel()

	t.Run("epub sibling", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "supplement.pdf"))
		writeFile(t, filepath.Join(dir, "book.epub"))

		got, err := hasNonPDFMainSibling(dir, nil)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("cbz sibling", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "supplement.pdf"))
		writeFile(t, filepath.Join(dir, "comic.cbz"))

		got, err := hasNonPDFMainSibling(dir, nil)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("m4b sibling", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "supplement.pdf"))
		writeFile(t, filepath.Join(dir, "audio.m4b"))

		got, err := hasNonPDFMainSibling(dir, nil)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("pdf-only directory has no sibling", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "supplement.pdf"))
		writeFile(t, filepath.Join(dir, "another.pdf"))

		got, err := hasNonPDFMainSibling(dir, nil)
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("unrelated extension is not a sibling", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "supplement.pdf"))
		writeFile(t, filepath.Join(dir, "notes.txt"))

		got, err := hasNonPDFMainSibling(dir, nil)
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("plugin-registered extension is a sibling", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "supplement.pdf"))
		writeFile(t, filepath.Join(dir, "book.azw3"))

		// Plugin extensions are stored without leading dot, lowercase.
		pluginExts := map[string]struct{}{"azw3": {}}
		got, err := hasNonPDFMainSibling(dir, pluginExts)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("recursive sibling in subdirectory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "supplement.pdf"))
		sub := filepath.Join(dir, "extras")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		writeFile(t, filepath.Join(sub, "book.epub"))

		got, err := hasNonPDFMainSibling(dir, nil)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("missing directory returns error", func(t *testing.T) {
		t.Parallel()
		_, err := hasNonPDFMainSibling(filepath.Join(t.TempDir(), "does-not-exist"), nil)
		assert.Error(t, err)
	})
}

// writeFile creates an empty file at path, failing the test on error.
func writeFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte{}, 0o644))
}
```

You'll need to add new imports at the top of `scan_helpers_test.go`:

```go
import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

(Keep any existing imports the file already has — `mediafile` and `models` are already there. Add `os`, `path/filepath`, and `require`.)

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./pkg/worker/ -run TestHasNonPDFMainSibling -v`
Expected: compile error — `undefined: hasNonPDFMainSibling`.

- [ ] **Step 3: Implement `hasNonPDFMainSibling` in `pkg/worker/scan.go`**

Add this function right after `looksLikePDFSupplement`:

```go
// hasNonPDFMainSibling returns true if dir (recursive) contains at least one
// file with a non-PDF main-eligible extension. Main-eligible means EPUB / CBZ /
// M4B or any extension in pluginExts (which comes from
// pluginManager.RegisteredFileExtensions() — keys are extensions without the
// leading dot, lowercase). pluginExts may be nil.
func hasNonPDFMainSibling(dir string, pluginExts map[string]struct{}) (bool, error) {
	found := false
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
		if ext == "pdf" {
			return nil
		}
		switch ext {
		case models.FileTypeEPUB, models.FileTypeCBZ, models.FileTypeM4B:
			found = true
			return filepath.SkipAll
		}
		if pluginExts != nil {
			if _, ok := pluginExts[ext]; ok {
				found = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return found, nil
}
```

`filepath.SkipAll` (Go 1.20+) bails out of the walk as soon as we find a hit, avoiding wasted I/O on large book directories.

- [ ] **Step 4: Run the test and confirm it passes**

Run: `go test ./pkg/worker/ -run TestHasNonPDFMainSibling -v`
Expected: all sub-tests pass.

- [ ] **Step 5: Run linter**

Run: `mise lint`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/worker/scan.go pkg/worker/scan_helpers_test.go
git commit -m "[Backend] Add hasNonPDFMainSibling helper for sibling check"
```

---

## Task 4: Sort `filesToScan` to defer supplement-named PDFs

**Files:**
- Modify: `pkg/worker/scan.go` (`ProcessScanJob`)
- Test: `pkg/worker/scan_helpers_test.go` (small unit test on the sort helper)

Pull the sort logic into a tiny pure helper so we can test it without spinning up a worker.

- [ ] **Step 1: Write the failing test**

Append to `pkg/worker/scan_helpers_test.go`:

```go
func TestPartitionSupplementPDFsLast(t *testing.T) {
	t.Parallel()

	names := []string{"supplement", "bonus"}

	// Mixed input — supplement-named PDFs interleaved with other files.
	input := []string{
		"/lib/[Author] Book/supplement.pdf",
		"/lib/[Author] Book/book.epub",
		"/lib/Other/bonus.pdf",
		"/lib/Other/audio.m4b",
		"/lib/Other/notes.txt",
	}

	got := partitionSupplementPDFsLast(input, names)

	// Non-supplement files must come first, in their original order.
	expected := []string{
		"/lib/[Author] Book/book.epub",
		"/lib/Other/audio.m4b",
		"/lib/Other/notes.txt",
		"/lib/[Author] Book/supplement.pdf",
		"/lib/Other/bonus.pdf",
	}
	assert.Equal(t, expected, got)
}

func TestPartitionSupplementPDFsLast_StableForNoMatches(t *testing.T) {
	t.Parallel()

	input := []string{"a.epub", "b.epub", "c.cbz"}
	got := partitionSupplementPDFsLast(input, []string{"supplement"})
	assert.Equal(t, input, got)
}

func TestPartitionSupplementPDFsLast_EmptyNames(t *testing.T) {
	t.Parallel()

	input := []string{"supplement.pdf", "book.epub"}
	got := partitionSupplementPDFsLast(input, nil)
	assert.Equal(t, input, got)
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./pkg/worker/ -run TestPartitionSupplementPDFsLast -v`
Expected: compile error — `undefined: partitionSupplementPDFsLast`.

- [ ] **Step 3: Implement the partition helper**

Add to `pkg/worker/scan.go` right after `hasNonPDFMainSibling`:

```go
// partitionSupplementPDFsLast returns paths reordered so that PDFs whose
// basename matches the supplement name list appear after every other path.
// Order within each partition is preserved (stable). The input slice is not
// mutated. When names is empty/nil, the input is returned unchanged.
func partitionSupplementPDFsLast(paths []string, names []string) []string {
	if len(names) == 0 {
		return paths
	}
	out := make([]string, 0, len(paths))
	var deferred []string
	for _, p := range paths {
		if looksLikePDFSupplement(filepath.Base(p), names) {
			deferred = append(deferred, p)
			continue
		}
		out = append(out, p)
	}
	return append(out, deferred...)
}
```

- [ ] **Step 4: Run the test and confirm it passes**

Run: `go test ./pkg/worker/ -run TestPartitionSupplementPDFsLast -v`
Expected: all 3 sub-tests pass.

- [ ] **Step 5: Wire the partition into `ProcessScanJob`**

Open `pkg/worker/scan.go`. Find the section right after the `filepath.WalkDir` loop completes and before `// Run input converters on discovered files` (currently around line 366-368):

```go
		// Run input converters on discovered files
		if w.pluginManager != nil {
			convertedFiles := w.runInputConverters(ctx, filesToScan, jobLog, library.ID)
			filesToScan = append(filesToScan, convertedFiles...)
		}
```

Add the partition call right before that block:

```go
		// Defer supplement-named PDFs so non-supplement files in the same
		// directory get processed first by the parallel worker pool. This
		// makes the supplement classification ordering-independent in the
		// common case where a sibling main file exists, even though the
		// on-disk sibling check in scanFileCreateNew is the actual
		// correctness mechanism.
		filesToScan = partitionSupplementPDFsLast(filesToScan, w.config.PDFSupplementFilenames)

		// Run input converters on discovered files
		if w.pluginManager != nil {
			convertedFiles := w.runInputConverters(ctx, filesToScan, jobLog, library.ID)
			filesToScan = append(filesToScan, convertedFiles...)
		}
```

- [ ] **Step 6: Run the helper tests and the full worker test suite**

Run: `go test ./pkg/worker/ -run TestPartitionSupplementPDFsLast -v && go test ./pkg/worker/...`
Expected: all pass.

- [ ] **Step 7: Lint**

Run: `mise lint`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add pkg/worker/scan.go pkg/worker/scan_helpers_test.go
git commit -m "[Backend] Defer supplement-named PDFs in scan dispatch order"
```

---

## Task 5: Apply supplement classification in `scanFileCreateNew`

**Files:**
- Modify: `pkg/worker/scan_unified.go` (`scanFileCreateNew`)
- Test: `pkg/worker/supplement_test.go` (new integration tests)

Wire the rule into the file-creation path. This is the correctness mechanism: regardless of the dispatch order, a PDF that meets the rule becomes a supplement.

The rule (from the spec):
- `fileType == "pdf"`, AND
- `looksLikePDFSupplement(filepath.Base(path), w.config.PDFSupplementFilenames)`, AND
- one of:
  - `existingBook != nil` (a book row already exists at this path), OR
  - `hasNonPDFMainSibling(bookPath, pluginExts)` returns true (an EPUB/CBZ/M4B/plugin-extension file is on disk in the directory tree).
- AND `!isRootLevelFile` (root-level PDFs always stay main).

When the rule fires, skip cover extraction (supplements don't get a cover during scan; matches the existing supplement create path at `scan_unified.go` ~L2554).

- [ ] **Step 1: Write the first failing integration test (sibling-EPUB → supplement)**

Open `pkg/worker/supplement_test.go` and append:

```go
// TestProcessScanJob_PDFNamedSupplementBecomesSupplement covers the core
// scenario: a PDF named "Supplement.pdf" alongside a main EPUB in the same
// directory must be classified as a supplement.
func TestProcessScanJob_PDFNamedSupplementBecomesSupplement(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:   "My Book",
		Authors: []string{"Author"},
	})
	testgen.GeneratePDF(t, bookDir, "Supplement.pdf", testgen.PDFOptions{})

	require.NoError(t, tc.runScan())

	booksList := tc.listBooks()
	require.Len(t, booksList, 1, "EPUB and supplement PDF should belong to one book")

	files := tc.listFiles()
	require.Len(t, files, 2)

	var mainCount, suppCount int
	for _, f := range files {
		switch f.FileRole {
		case models.FileRoleMain:
			mainCount++
			assert.Equal(t, "epub", f.FileType, "main file should be the EPUB")
		case models.FileRoleSupplement:
			suppCount++
			assert.Equal(t, "pdf", f.FileType, "supplement should be the PDF")
			assert.Equal(t, "Supplement.pdf", filepath.Base(f.Filepath))
		}
	}
	assert.Equal(t, 1, mainCount)
	assert.Equal(t, 1, suppCount)
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./pkg/worker/ -run TestProcessScanJob_PDFNamedSupplementBecomesSupplement -v`
Expected: test fails — currently the PDF is classified as main, so we get 2 main files (or one main book per file). The exact failure mode may be `mainCount=2, suppCount=0` or `len(books)=2` depending on directory layout. The point is: it fails because the feature doesn't exist yet.

- [ ] **Step 3: Implement the classification in `scanFileCreateNew`**

Open `pkg/worker/scan_unified.go`. Find the section where `scanFileCreateNew` handles cover extraction and creates the file (currently around lines 2393-2434). The current code reads:

```go
	// Handle cover extraction. extractAndSaveCover also adopts a cover file
	// that already sits next to the source file on disk, so we always call
	// it — even when the parser returned no cover data for the source.
	var coverImagePath *string
	var coverMimeType *string
	var coverSource *string
	var coverPage *int

	coverFilename, extractedMimeType, wasPreExisting, err := w.extractAndSaveCover(ctx, path, bookPath, isRootLevelFile, metadata, opts.JobLog)
	if err != nil {
		logWarn("failed to extract cover", logger.Data{"error": err.Error()})
	} else if coverFilename != "" {
		coverImagePath = &coverFilename
		if extractedMimeType != "" {
			coverMimeType = &extractedMimeType
		}
		if wasPreExisting {
			existingCoverSource := models.DataSourceExistingCover
			coverSource = &existingCoverSource
		} else {
			cs := metadata.SourceForField("cover")
			coverSource = &cs
		}
	}
	if metadata != nil && metadata.CoverPage != nil {
		coverPage = metadata.CoverPage
	}

	// Create file record
	logInfo("creating file", logger.Data{"path": path, "filesize": size})
	file := &models.File{
		LibraryID:          opts.LibraryID,
		BookID:             book.ID,
		Filepath:           path,
		FileType:           fileType,
		FilesizeBytes:      size,
		FileModifiedAt:     &modTime,
		CoverImageFilename: coverImagePath,
		CoverMimeType:      coverMimeType,
		CoverSource:        coverSource,
		CoverPage:          coverPage,
	}
```

Replace it with:

```go
	// Decide whether this new file should be a supplement based on filename
	// pattern + on-disk siblings. Only applies to PDFs in directory-based
	// books — root-level PDFs always stay main (root-level supplement linking
	// uses basename-prefix matching, handled separately).
	classifyAsSupplement := false
	if fileType == models.FileTypePDF && !isRootLevelFile && looksLikePDFSupplement(filepath.Base(path), w.config.PDFSupplementFilenames) {
		// Reuse-existing-book check: if a book row already lives at this
		// bookPath, there's by definition another file (or there will be).
		if existingBook != nil {
			classifyAsSupplement = true
		} else {
			// On-disk sibling check: look for any non-PDF main-eligible file
			// in the book directory. This is what makes the rule
			// ordering-independent when both files arrive in the same scan.
			var pluginExts map[string]struct{}
			if w.pluginManager != nil {
				pluginExts = w.pluginManager.RegisteredFileExtensions()
			}
			hasSibling, sibErr := hasNonPDFMainSibling(bookPath, pluginExts)
			if sibErr != nil {
				logWarn("failed to check for sibling main file", logger.Data{"error": sibErr.Error(), "dir": bookPath})
			} else if hasSibling {
				classifyAsSupplement = true
			}
		}
	}

	// Handle cover extraction. extractAndSaveCover also adopts a cover file
	// that already sits next to the source file on disk, so we always call
	// it — even when the parser returned no cover data for the source.
	// Skipped for files we're about to create as supplements, matching the
	// supplement-discovery code path further down (supplements don't get
	// scanned-cover extraction).
	var coverImagePath *string
	var coverMimeType *string
	var coverSource *string
	var coverPage *int

	if !classifyAsSupplement {
		coverFilename, extractedMimeType, wasPreExisting, err := w.extractAndSaveCover(ctx, path, bookPath, isRootLevelFile, metadata, opts.JobLog)
		if err != nil {
			logWarn("failed to extract cover", logger.Data{"error": err.Error()})
		} else if coverFilename != "" {
			coverImagePath = &coverFilename
			if extractedMimeType != "" {
				coverMimeType = &extractedMimeType
			}
			if wasPreExisting {
				existingCoverSource := models.DataSourceExistingCover
				coverSource = &existingCoverSource
			} else {
				cs := metadata.SourceForField("cover")
				coverSource = &cs
			}
		}
		if metadata != nil && metadata.CoverPage != nil {
			coverPage = metadata.CoverPage
		}
	}

	// Create file record
	logInfo("creating file", logger.Data{"path": path, "filesize": size, "supplement": classifyAsSupplement})
	fileRole := models.FileRoleMain
	if classifyAsSupplement {
		fileRole = models.FileRoleSupplement
	}
	file := &models.File{
		LibraryID:          opts.LibraryID,
		BookID:             book.ID,
		Filepath:           path,
		FileType:           fileType,
		FileRole:           fileRole,
		FilesizeBytes:      size,
		FileModifiedAt:     &modTime,
		CoverImageFilename: coverImagePath,
		CoverMimeType:      coverMimeType,
		CoverSource:        coverSource,
		CoverPage:          coverPage,
	}
```

Note the additions:
- `classifyAsSupplement` decision block before cover extraction.
- Cover extraction guarded by `if !classifyAsSupplement { ... }`.
- `FileRole: fileRole` added to the `models.File{}` literal.
- `"supplement": classifyAsSupplement` added to the create-file log line.

- [ ] **Step 4: Run the test and confirm it now passes**

Run: `go test ./pkg/worker/ -run TestProcessScanJob_PDFNamedSupplementBecomesSupplement -v`
Expected: PASS.

- [ ] **Step 5: Add the rest of the integration tests**

Append to `pkg/worker/supplement_test.go`:

```go
// TestProcessScanJob_PDFAloneInDirStaysMain confirms the safety rule:
// a directory containing only a supplement-named PDF must still import as
// a main file so the book isn't silently dropped.
func TestProcessScanJob_PDFAloneInDirStaysMain(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] PDF Only Book")
	testgen.GeneratePDF(t, bookDir, "Supplement.pdf", testgen.PDFOptions{})

	require.NoError(t, tc.runScan())

	require.Len(t, tc.listBooks(), 1)

	files := tc.listFiles()
	require.Len(t, files, 1)
	assert.Equal(t, models.FileRoleMain, files[0].FileRole, "lone supplement-named PDF should be main")
	assert.Equal(t, "pdf", files[0].FileType)
}

// TestProcessScanJob_PDFCaseInsensitiveMatch confirms basename matching
// ignores case for both the filename and the configured names.
func TestProcessScanJob_PDFCaseInsensitiveMatch(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Casing Test")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Casing Test"})
	testgen.GeneratePDF(t, bookDir, "BONUS.pdf", testgen.PDFOptions{})

	require.NoError(t, tc.runScan())

	require.Len(t, tc.listBooks(), 1)
	files := tc.listFiles()
	require.Len(t, files, 2)

	for _, f := range files {
		if f.FileType == "pdf" {
			assert.Equal(t, models.FileRoleSupplement, f.FileRole)
		}
	}
}

// TestProcessScanJob_PDFSubstringDoesNotMatch confirms substring matches do
// NOT trigger supplement classification — only exact basename matches.
func TestProcessScanJob_PDFSubstringDoesNotMatch(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Substring Test")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Substring Test"})
	testgen.GeneratePDF(t, bookDir, "My Book - Supplement.pdf", testgen.PDFOptions{})

	require.NoError(t, tc.runScan())

	// PDF basename "My Book - Supplement" is NOT an exact match for any list
	// entry, so it stays main. Two main files end up on the same book (the
	// existing scan flow merges files with the same bookPath into one book).
	require.Len(t, tc.listBooks(), 1)
	files := tc.listFiles()
	require.Len(t, files, 2)
	for _, f := range files {
		assert.Equal(t, models.FileRoleMain, f.FileRole, "substring match should not classify as supplement")
	}
}

// TestProcessScanJob_PDFEmptyConfigDisablesFeature confirms setting
// PDFSupplementFilenames to an empty slice disables the auto-classification.
func TestProcessScanJob_PDFEmptyConfigDisablesFeature(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	tc.worker.config.PDFSupplementFilenames = nil

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Disabled Test")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Disabled Test"})
	testgen.GeneratePDF(t, bookDir, "Supplement.pdf", testgen.PDFOptions{})

	require.NoError(t, tc.runScan())

	files := tc.listFiles()
	require.Len(t, files, 2)
	for _, f := range files {
		assert.Equal(t, models.FileRoleMain, f.FileRole, "feature should be disabled when config is empty")
	}
}

// TestProcessScanJob_PDFExistingMainNotReclassified confirms an existing
// PDF main file whose name happens to match the supplement list is NOT
// reclassified on rescan. The rule only applies at file creation.
func TestProcessScanJob_PDFExistingMainNotReclassified(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// First scan: only the PDF exists, so it imports as main.
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Rescan Test")
	testgen.GeneratePDF(t, bookDir, "Supplement.pdf", testgen.PDFOptions{})
	require.NoError(t, tc.runScan())

	files := tc.listFiles()
	require.Len(t, files, 1)
	require.Equal(t, models.FileRoleMain, files[0].FileRole)
	mainID := files[0].ID

	// Add an EPUB sibling and rescan.
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Rescan Test"})
	require.NoError(t, tc.runScan())

	afterFiles := tc.listFiles()
	require.Len(t, afterFiles, 2)

	for _, f := range afterFiles {
		if f.ID == mainID {
			assert.Equal(t, models.FileRoleMain, f.FileRole, "existing main PDF must not be reclassified on rescan")
		}
	}
}
```

- [ ] **Step 6: Run the new test suite**

Run: `go test ./pkg/worker/ -run "TestProcessScanJob_PDF" -v`
Expected: all 6 tests pass.

- [ ] **Step 7: Run the full worker test suite to confirm nothing broke**

Run: `go test ./pkg/worker/...`
Expected: all pass. The pre-existing `TestProcessScanJob_ScannableSupplementNotRescannedAsMain` (which adds a PDF as a supplement directly via the DB) is the closest neighbor — it should keep passing.

- [ ] **Step 8: Lint**

Run: `mise lint`
Expected: clean.

- [ ] **Step 9: Commit**

```bash
git add pkg/worker/scan_unified.go pkg/worker/supplement_test.go
git commit -m "[Backend] Auto-classify supplement-named PDFs on scan"
```

---

## Task 6: Update example YAML

**Files:**
- Modify: `shisho.example.yaml`

`shisho.example.yaml` is part of the project's "complete reference of all configurable fields" promise (see root `CLAUDE.md`). Anything added to `Config` must show up here.

- [ ] **Step 1: Add the new field to the supplement section**

Open `shisho.example.yaml`. Find the `supplement_exclude_patterns:` block (currently around line 156-168). Append after it:

```yaml
# PDF basenames (case-insensitive, no extension) that should be auto-classified
# as supplements when discovered next to a main EPUB / CBZ / M4B file. A PDF
# alone in a directory always imports as a main file regardless of name.
# Set to [] to disable. Substring matches are NOT applied — exact basename only.
# Env: PDF_SUPPLEMENT_FILENAMES (comma-separated)
# Default: see list below
pdf_supplement_filenames:
  - "supplement"
  - "supplemental"
  - "bonus"
  - "bonus material"
  - "bonus content"
  - "companion"
  - "notes"
  - "liner notes"
  - "errata"
  - "booklet"
  - "digital booklet"
  - "appendix"
  - "map"
  - "maps"
  - "insert"
  - "guide"
  - "reference"
  - "cheat sheet"
  - "cheatsheet"
  - "cribsheet"
  - "pamphlet"
  - "extras"
```

- [ ] **Step 2: Commit**

```bash
git add shisho.example.yaml
git commit -m "[Backend] Document pdf_supplement_filenames in example config"
```

---

## Task 7: Update website docs

**Files:**
- Modify: `website/docs/configuration.md`
- Modify: `website/docs/supplement-files.md`

Per root `CLAUDE.md`: any user-facing change must update `website/docs/`. New config option goes in `configuration.md`; new supplement behavior goes in `supplement-files.md`.

- [ ] **Step 1: Document the config field in `website/docs/configuration.md`**

Open `website/docs/configuration.md` and find the section that documents `supplement_exclude_patterns` (search for `supplement_exclude_patterns`). Add a sibling entry for `pdf_supplement_filenames` immediately after it. Match the existing formatting (heading level, table structure, env-var notation) of `supplement_exclude_patterns` exactly — copy that section's pattern.

The content should cover:
- **Field name:** `pdf_supplement_filenames`
- **Env var:** `PDF_SUPPLEMENT_FILENAMES` (comma-separated)
- **Default:** the full default list (same 22 entries as in `pkg/config/config.go::defaults`)
- **Behavior:** PDF basenames (case-insensitive, no extension) that get classified as supplements on scan when a sibling EPUB/CBZ/M4B exists in the same directory. Exact match only — substrings don't match. A PDF alone in a directory always imports as main.
- **Disable:** set to `[]`.

- [ ] **Step 2: Document the supplement behavior in `website/docs/supplement-files.md`**

Open `website/docs/supplement-files.md`. Find the section "What Counts as a Supplement" and the existing exclude-patterns subsection. Add a new section titled "PDF Auto-Classification" between "Excluded Files" and the "Working with Supplements" section.

Content to include:
- A PDF whose basename exactly matches an entry in `pdf_supplement_filenames` is classified as a supplement when a sibling main file (EPUB / CBZ / M4B / plugin-registered file extension) exists in the same directory tree.
- A directory-only PDF (no sibling main file) imports as a main file regardless of name.
- The check runs only at file creation. PDFs already imported as main files are not retroactively reclassified.
- Cross-link to `configuration.md#pdf_supplement_filenames` for the configurable list.
- Show a small example tree like the existing examples in this doc:

```
[Author] My Book/
├── My Book.epub          ← main file
└── Supplement.pdf        ← classified as supplement (matches default list)
```

- [ ] **Step 3: Verify markdown renders**

Run: `cd website && pnpm build`
Expected: build completes without errors. (Docusaurus catches broken links and bad markdown.)

- [ ] **Step 4: Commit**

```bash
git add website/docs/configuration.md website/docs/supplement-files.md
git commit -m "[Docs] Document pdf_supplement_filenames behavior"
```

---

## Task 8: Update `pkg/CLAUDE.md` with the new gotcha

**Files:**
- Modify: `pkg/CLAUDE.md`

Per root `CLAUDE.md`: when the work introduces a new convention or gotcha, update the relevant `CLAUDE.md`. The new convention here: `scanFileCreateNew` skips cover extraction when classifying as supplement. Future agents touching this code path could break that quietly.

- [ ] **Step 1: Add a short note**

Open `pkg/CLAUDE.md`. Find the section beginning `### scanInternal and File Organization`. Add a new sibling section right after the existing "Scan Cache Must Include Supplements" section:

```markdown
### Auto-Classification of Supplement-Named PDFs

`scanFileCreateNew` (in `pkg/worker/scan_unified.go`) inspects new PDF files for the auto-supplement rule before creating the file row: when a PDF's basename matches `config.PDFSupplementFilenames` AND a sibling main file (EPUB / CBZ / M4B / plugin-registered extension) exists on disk OR a book row already exists at the same `bookPath`, it's created with `FileRole=Supplement` and **cover extraction is skipped** (matching the existing supplement-discovery path that creates supplements without covers).

When editing the cover-extraction block in `scanFileCreateNew`, preserve the `if !classifyAsSupplement { ... }` guard. Rescans don't re-run this rule — existing main-file PDFs whose names match the list keep their role.
```

- [ ] **Step 2: Commit**

```bash
git add pkg/CLAUDE.md
git commit -m "[Docs] Note auto-supplement PDF rule in pkg/CLAUDE.md"
```

---

## Task 9: Final verification

**Files:** none (verification only)

- [ ] **Step 1: Run the full check**

Run: `mise check:quiet`
Expected: clean. (This runs Go tests with `-race`, golangci-lint, Vitest, ESLint, Prettier, tsc, and chromium e2e.)

If any failure appears, fix it inline before considering the plan complete. Common issues:
- gofmt/goimports differences — run `mise lint` and apply suggestions.
- `tygo` regenerated types — none expected here (no Go struct that's tygo'd was changed). If tygo runs and reports "skipping, outputs are up-to-date", that's normal per root CLAUDE.md.

- [ ] **Step 2: Smoke-test in a running dev environment (manual)**

This step is for the reviewer running interactively, not for an autonomous agent. Skip if running unattended; the integration tests in Task 5 cover correctness.

If running interactively:
1. Start `mise start`.
2. Drop a `Book.epub` and `Supplement.pdf` into a watched library directory.
3. Trigger a scan.
4. Open the book detail page in the frontend; confirm the PDF appears under "Supplements" and the EPUB is the main file.

- [ ] **Step 3: Confirm everything is committed**

Run: `git status`
Expected: working tree clean.

Run: `git log --oneline master..HEAD`
Expected: a sequence of commits matching the per-task messages above.
