# Supplementary Files Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add support for non-primary supplementary files (PDFs, text files, etc.) that are visible on book pages and downloadable.

**Architecture:** Supplements use the existing `files` table with a new `file_role` column. Discovery happens during scanning - directory-based books include all non-main files, root-level books use basename matching. The download handler delegates supplements to the original file endpoint (no processing). Frontend shows supplements in a separate section with simplified UI.

**Tech Stack:** Go (Echo, Bun ORM), React 19, TypeScript, SQLite, TailwindCSS

---

## Task 1: Add file_role Column to Database

**Files:**
- Modify: `pkg/migrations/20250321211048_create_initial_tables.go:141-166`
- Modify: `pkg/models/file.go:16-49`

**Step 1: Add file_role column to migration**

In `pkg/migrations/20250321211048_create_initial_tables.go`, add `file_role` column to files table at line 149 (after `file_type`):

```go
file_role TEXT NOT NULL DEFAULT 'main',
```

**Step 2: Add FileRole constants to models**

In `pkg/models/file.go`, add after line 14 (after FileTypeM4B constant):

```go
const (
	//tygo:emit export type FileRole = typeof FileRoleMain | typeof FileRoleSupplement;
	FileRoleMain       = "main"
	FileRoleSupplement = "supplement"
)
```

**Step 3: Add FileRole field to File struct**

In `pkg/models/file.go`, add after line 26 (after `FileType` field):

```go
FileRole          string            `bun:",nullzero,default:'main'" json:"file_role" tstype:"FileRole"`
```

**Step 4: Run migrations and regenerate types**

Run: `make db:migrate && make tygo`
Expected: Migration applies successfully, types regenerated

**Step 5: Commit**

```bash
git add pkg/migrations/20250321211048_create_initial_tables.go pkg/models/file.go
git commit -m "$(cat <<'EOF'
[Database] Add file_role column for supplementary files

Adds file_role column to files table with 'main' and 'supplement' values.
Main files are primary content (epub, m4b, cbz), supplements are auxiliary
files like PDFs bundled with audiobooks.
EOF
)"
```

---

## Task 2: Add Supplement Exclude Patterns to Config

**Files:**
- Modify: `pkg/config/config.go:20-68`
- Modify: `shisho.example.yaml:92`

**Step 1: Add config field**

In `pkg/config/config.go`, add after line 42 (after `DownloadCacheMaxSizeGB`):

```go
// Supplement discovery settings
SupplementExcludePatterns []string `koanf:"supplement_exclude_patterns" json:"supplement_exclude_patterns"`
```

**Step 2: Add default value**

In `pkg/config/config.go`, add in `defaults()` function after line 66 (after `DownloadCacheMaxSizeGB: 5`):

```go
SupplementExcludePatterns: []string{".*", ".DS_Store", "Thumbs.db", "desktop.ini"},
```

**Step 3: Add to NewForTest**

In `pkg/config/config.go`, add in `NewForTest()` function after line 132 (after `DownloadCacheMaxSizeGB`):

```go
cfg.SupplementExcludePatterns = []string{".*", ".DS_Store", "Thumbs.db", "desktop.ini"}
```

**Step 4: Add example config**

In `shisho.example.yaml`, add after line 91 (after download_cache_max_size_gb section):

```yaml

# =============================================================================
# SUPPLEMENT DISCOVERY SETTINGS
# =============================================================================

# Patterns to exclude from supplement discovery (glob patterns)
# These files will not be picked up as supplements even if they're in book directories
# Env: SUPPLEMENT_EXCLUDE_PATTERNS (comma-separated)
# Default: [".*", ".DS_Store", "Thumbs.db", "desktop.ini"]
supplement_exclude_patterns:
  - ".*"           # hidden files (dotfiles)
  - ".DS_Store"
  - "Thumbs.db"
  - "desktop.ini"
```

**Step 5: Commit**

```bash
git add pkg/config/config.go shisho.example.yaml
git commit -m "$(cat <<'EOF'
[Config] Add supplement_exclude_patterns for supplement discovery

Adds configurable patterns to exclude from supplement file discovery.
Defaults exclude hidden files and common system files.
EOF
)"
```

---

## Task 3: Write Supplement Discovery Unit Tests

**Files:**
- Create: `pkg/worker/supplement_test.go`

**Step 1: Create test file with directory-based tests**

Create `pkg/worker/supplement_test.go`:

```go
package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessScanJob_SupplementsInDirectory(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with main file + supplements
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateM4B(t, bookDir, "book.m4b", testgen.M4BOptions{
		Title:    "My Book",
		HasCover: true,
	})

	// Create supplement files
	supplementPDF := filepath.Join(bookDir, "companion.pdf")
	require.NoError(t, os.WriteFile(supplementPDF, []byte("PDF content"), 0644))

	supplementTXT := filepath.Join(bookDir, "notes.txt")
	require.NoError(t, os.WriteFile(supplementTXT, []byte("Notes content"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	// Verify book was created
	books := tc.listBooks()
	require.Len(t, books, 1)

	// Verify files: 1 main + 2 supplements
	files := tc.listFiles()
	require.Len(t, files, 3)

	mainFiles := 0
	supplementFiles := 0
	for _, f := range files {
		if f.FileRole == models.FileRoleMain {
			mainFiles++
			assert.Equal(t, "m4b", f.FileType)
		} else if f.FileRole == models.FileRoleSupplement {
			supplementFiles++
			assert.Contains(t, []string{"pdf", "txt"}, f.FileType)
		}
	}
	assert.Equal(t, 1, mainFiles)
	assert.Equal(t, 2, supplementFiles)
}

func TestProcessScanJob_SupplementsExcludeHiddenFiles(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{})

	// Create hidden file (should be excluded)
	hiddenFile := filepath.Join(bookDir, ".hidden")
	require.NoError(t, os.WriteFile(hiddenFile, []byte("hidden"), 0644))

	// Create .DS_Store (should be excluded)
	dsStore := filepath.Join(bookDir, ".DS_Store")
	require.NoError(t, os.WriteFile(dsStore, []byte("dsstore"), 0644))

	// Create normal supplement (should be included)
	supplement := filepath.Join(bookDir, "guide.pdf")
	require.NoError(t, os.WriteFile(supplement, []byte("guide"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	files := tc.listFiles()
	// Should have 2: main epub + guide.pdf supplement
	// Hidden files and .DS_Store should be excluded
	require.Len(t, files, 2)

	for _, f := range files {
		assert.NotContains(t, f.Filepath, ".hidden")
		assert.NotContains(t, f.Filepath, ".DS_Store")
	}
}

func TestProcessScanJob_SupplementsExcludeShishoFiles(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{})

	// Create shisho special files (should be excluded)
	coverFile := filepath.Join(bookDir, "book.cover.jpg")
	require.NoError(t, os.WriteFile(coverFile, []byte("cover"), 0644))

	metadataFile := filepath.Join(bookDir, "book.metadata.json")
	require.NoError(t, os.WriteFile(metadataFile, []byte("{}"), 0644))

	// Create normal supplement
	supplement := filepath.Join(bookDir, "appendix.pdf")
	require.NoError(t, os.WriteFile(supplement, []byte("appendix"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	files := tc.listFiles()
	// Should have 2: main epub + appendix.pdf
	require.Len(t, files, 2)

	for _, f := range files {
		assert.NotContains(t, f.Filepath, ".cover.")
		assert.NotContains(t, f.Filepath, ".metadata.json")
	}
}

func TestProcessScanJob_SupplementsInSubdirectory(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateM4B(t, bookDir, "book.m4b", testgen.M4BOptions{})

	// Create subdirectory with supplements
	subDir := testgen.CreateSubDir(t, bookDir, "extras")
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "bonus.pdf"), []byte("bonus"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "artwork.jpg"), []byte("art"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	files := tc.listFiles()
	// Should have 3: main m4b + 2 supplements in subdirectory
	require.Len(t, files, 3)

	supplementCount := 0
	for _, f := range files {
		if f.FileRole == models.FileRoleSupplement {
			supplementCount++
		}
	}
	assert.Equal(t, 2, supplementCount)
}

func TestProcessScanJob_RootLevelSupplements(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create root-level main file
	testgen.GenerateM4B(t, libraryPath, "My Book.m4b", testgen.M4BOptions{})

	// Create supplement with matching basename
	supplement := filepath.Join(libraryPath, "My Book.pdf")
	require.NoError(t, os.WriteFile(supplement, []byte("supplement"), 0644))

	// Create unrelated file (different basename - should NOT be picked up)
	unrelated := filepath.Join(libraryPath, "Other Book.pdf")
	require.NoError(t, os.WriteFile(unrelated, []byte("other"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	// Should have 2 books: "My Book" and "Other Book" (standalone supplement becomes own book? or just ignored?)
	// Based on design doc: root-level supplements with matching basename are grouped
	books := tc.listBooks()
	require.Len(t, books, 1, "Only My Book should exist, Other Book.pdf doesn't have a main file")

	files := tc.listFiles()
	// My Book.m4b (main) + My Book.pdf (supplement)
	require.Len(t, files, 2)

	mainCount := 0
	suppCount := 0
	for _, f := range files {
		if f.FileRole == models.FileRoleMain {
			mainCount++
		} else {
			suppCount++
		}
	}
	assert.Equal(t, 1, mainCount)
	assert.Equal(t, 1, suppCount)
}
```

**Step 2: Run tests to verify they fail**

Run: `make test`
Expected: Tests fail (supplement discovery not yet implemented)

**Step 3: Commit test file**

```bash
git add pkg/worker/supplement_test.go
git commit -m "$(cat <<'EOF'
[Test] Add supplement discovery tests

TDD tests for supplement file discovery during scanning.
Tests directory-based supplements, hidden file exclusion,
shisho file exclusion, subdirectory supplements, and root-level supplements.
EOF
)"
```

---

## Task 4: Implement Supplement Discovery Helper

**Files:**
- Modify: `pkg/worker/scan.go`

**Step 1: Add isMainFileExtension helper**

In `pkg/worker/scan.go`, add after line 40 (after filepathNarratorRE):

```go
// isMainFileExtension returns true if the extension is a main file type
func isMainFileExtension(ext string) bool {
	ext = strings.ToLower(ext)
	_, ok := extensionsToScan[ext]
	return ok
}

// isShishoSpecialFile returns true if the filename is a shisho-specific file
func isShishoSpecialFile(filename string) bool {
	lower := strings.ToLower(filename)
	// Exclude cover files: *.cover.* pattern
	if strings.Contains(lower, ".cover.") {
		return true
	}
	// Exclude metadata files: *.metadata.json
	if strings.HasSuffix(lower, ".metadata.json") {
		return true
	}
	return false
}

// matchesExcludePattern checks if filename matches any exclude pattern
func matchesExcludePattern(filename string, patterns []string) bool {
	for _, pattern := range patterns {
		// Simple prefix match for patterns starting with "."
		if strings.HasPrefix(pattern, ".") && strings.HasPrefix(filename, pattern) {
			return true
		}
		// Exact match
		if filename == pattern {
			return true
		}
		// Glob match for more complex patterns
		if matched, _ := filepath.Match(pattern, filename); matched {
			return true
		}
	}
	return false
}

// discoverSupplements finds supplement files for a book
func discoverSupplements(bookDir string, excludePatterns []string) ([]string, error) {
	var supplements []string

	err := filepath.WalkDir(bookDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		filename := filepath.Base(path)
		ext := filepath.Ext(path)

		// Skip main file types
		if isMainFileExtension(ext) {
			return nil
		}

		// Skip shisho special files
		if isShishoSpecialFile(filename) {
			return nil
		}

		// Skip files matching exclude patterns
		if matchesExcludePattern(filename, excludePatterns) {
			return nil
		}

		supplements = append(supplements, path)
		return nil
	})

	return supplements, err
}

// discoverRootLevelSupplements finds supplements for root-level books by basename matching
func discoverRootLevelSupplements(mainFilePath string, libraryPath string, excludePatterns []string) ([]string, error) {
	var supplements []string

	// Get basename without extension
	mainFilename := filepath.Base(mainFilePath)
	mainBasename := strings.TrimSuffix(mainFilename, filepath.Ext(mainFilename))

	// List files in the same directory
	entries, err := os.ReadDir(libraryPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		ext := filepath.Ext(filename)
		basename := strings.TrimSuffix(filename, ext)

		// Skip if it's the main file itself
		if filename == mainFilename {
			continue
		}

		// Skip main file types
		if isMainFileExtension(ext) {
			continue
		}

		// Skip shisho special files
		if isShishoSpecialFile(filename) {
			continue
		}

		// Skip excluded patterns
		if matchesExcludePattern(filename, excludePatterns) {
			continue
		}

		// Match if basename is same or starts with main basename
		// "MyBook.pdf" matches "MyBook.m4b"
		// "MyBook - Guide.txt" matches "MyBook.m4b"
		if basename == mainBasename || strings.HasPrefix(basename, mainBasename) {
			supplements = append(supplements, filepath.Join(libraryPath, filename))
		}
	}

	return supplements, nil
}
```

**Step 2: Run tests**

Run: `make test`
Expected: Tests still fail (not integrated into scan yet)

**Step 3: Commit helper functions**

```bash
git add pkg/worker/scan.go
git commit -m "$(cat <<'EOF'
[Scanner] Add supplement discovery helper functions

Helper functions for discovering supplement files during scanning.
Handles both directory-based books (recursive walk) and root-level
books (basename matching). Excludes main file types, shisho files,
and configurable patterns.
EOF
)"
```

---

## Task 5: Integrate Supplement Discovery into Scanner

**Files:**
- Modify: `pkg/worker/scan.go`
- Modify: `pkg/books/service.go`

**Step 1: Add CreateFile method to service**

In `pkg/books/service.go`, add after `CreateBook` function (around line 100):

```go
// CreateFile creates a new file in the database.
func (svc *Service) CreateFile(ctx context.Context, file *models.File) error {
	now := time.Now()
	if file.CreatedAt.IsZero() {
		file.CreatedAt = now
	}
	file.UpdatedAt = file.CreatedAt

	_, err := svc.db.NewInsert().
		Model(file).
		Returning("*").
		Exec(ctx)
	return errors.WithStack(err)
}
```

**Step 2: Update scanFile to create supplements**

In `pkg/worker/scan.go`, find the end of `scanFile` function (after file creation for main file, around line 750-800). Add supplement discovery after main file is created:

Find the section that creates the file entry and returns (search for the final file creation). After the main file is successfully created/associated with book, add:

```go
// Discover and create supplement files
if !isRootLevelFile {
	// Directory-based book: scan directory for supplements
	supplements, err := discoverSupplements(bookPath, w.cfg.SupplementExcludePatterns)
	if err != nil {
		jobLog.Warn("failed to discover supplements", logger.Data{"error": err.Error()})
	} else {
		for _, suppPath := range supplements {
			// Check if supplement already exists
			existingSupp, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
				Filepath:  &suppPath,
				LibraryID: &libraryID,
			})
			if err != nil && !errors.Is(err, errcodes.NotFound("File")) {
				jobLog.Warn("error checking supplement", logger.Data{"path": suppPath, "error": err.Error()})
				continue
			}
			if existingSupp != nil {
				continue // Already exists
			}

			// Get file info
			suppStat, err := os.Stat(suppPath)
			if err != nil {
				jobLog.Warn("can't stat supplement", logger.Data{"path": suppPath, "error": err.Error()})
				continue
			}

			suppExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(suppPath)), ".")
			suppFile := &models.File{
				LibraryID:     libraryID,
				BookID:        book.ID,
				Filepath:      suppPath,
				FileType:      suppExt,
				FileRole:      models.FileRoleSupplement,
				FilesizeBytes: suppStat.Size(),
			}

			if err := w.bookService.CreateFile(ctx, suppFile); err != nil {
				jobLog.Warn("failed to create supplement", logger.Data{"path": suppPath, "error": err.Error()})
				continue
			}
			jobLog.Info("created supplement file", logger.Data{"path": suppPath, "file_id": suppFile.ID})
		}
	}
} else {
	// Root-level book: find supplements by basename matching
	for _, libraryPath := range library.LibraryPaths {
		if filepath.Dir(path) == libraryPath.Filepath {
			supplements, err := discoverRootLevelSupplements(path, libraryPath.Filepath, w.cfg.SupplementExcludePatterns)
			if err != nil {
				jobLog.Warn("failed to discover root supplements", logger.Data{"error": err.Error()})
				break
			}
			for _, suppPath := range supplements {
				existingSupp, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
					Filepath:  &suppPath,
					LibraryID: &libraryID,
				})
				if err != nil && !errors.Is(err, errcodes.NotFound("File")) {
					continue
				}
				if existingSupp != nil {
					continue
				}

				suppStat, err := os.Stat(suppPath)
				if err != nil {
					continue
				}

				suppExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(suppPath)), ".")
				suppFile := &models.File{
					LibraryID:     libraryID,
					BookID:        book.ID,
					Filepath:      suppPath,
					FileType:      suppExt,
					FileRole:      models.FileRoleSupplement,
					FilesizeBytes: suppStat.Size(),
				}

				if err := w.bookService.CreateFile(ctx, suppFile); err != nil {
					continue
				}
				jobLog.Info("created root-level supplement", logger.Data{"path": suppPath, "file_id": suppFile.ID})
			}
			break
		}
	}
}
```

Note: The exact placement depends on finding where `book` variable has its final ID after creation. Look for where the main file is created and add this after.

**Step 3: Run tests**

Run: `make test`
Expected: Supplement discovery tests pass

**Step 4: Commit**

```bash
git add pkg/worker/scan.go pkg/books/service.go
git commit -m "$(cat <<'EOF'
[Scanner] Integrate supplement discovery into scan job

During scanning, discovers and creates supplement files:
- Directory-based books: recursively scans for non-main files
- Root-level books: finds supplements by basename matching
Excludes hidden files, system files, and shisho special files.
EOF
)"
```

---

## Task 6: Modify Download Handler for Supplements

**Files:**
- Modify: `pkg/books/handlers.go:1179-1247`

**Step 1: Add supplement check to downloadFile**

In `pkg/books/handlers.go`, modify `downloadFile` handler. Add after line 1193 (after file retrieval and before library access check):

```go
// Supplements download as-is, no processing
if file.FileRole == models.FileRoleSupplement {
	return h.downloadOriginalFile(c)
}
```

**Step 2: Run handler tests**

Run: `make test`
Expected: Tests pass

**Step 3: Commit**

```bash
git add pkg/books/handlers.go
git commit -m "$(cat <<'EOF'
[API] Delegate supplement downloads to original file handler

Supplements don't need metadata processing, so the download handler
delegates to downloadOriginalFile for supplement files.
EOF
)"
```

---

## Task 7: Add file_role to UpdateFilePayload

**Files:**
- Modify: `pkg/books/validators.go:44-51`
- Modify: `pkg/books/handlers.go:569-810`

**Step 1: Add FileRole to UpdateFilePayload**

In `pkg/books/validators.go`, modify `UpdateFilePayload` struct at line 44:

```go
// UpdateFilePayload is the payload for updating a file's metadata.
type UpdateFilePayload struct {
	FileRole    *string              `json:"file_role,omitempty" validate:"omitempty,oneof=main supplement"`
	Narrators   []string             `json:"narrators,omitempty" validate:"omitempty,dive,max=200"`
	URL         *string              `json:"url,omitempty" validate:"omitempty,max=500,url"`
	Publisher   *string              `json:"publisher,omitempty" validate:"omitempty,max=200"`
	Imprint     *string              `json:"imprint,omitempty" validate:"omitempty,max=200"`
	ReleaseDate *string              `json:"release_date,omitempty" validate:"omitempty"` // ISO 8601 date string
	Identifiers *[]IdentifierPayload `json:"identifiers,omitempty" validate:"omitempty,dive"`
}
```

**Step 2: Handle FileRole in updateFile handler**

In `pkg/books/handlers.go`, in `updateFile` function, add handling for file_role changes. Add after line 616 (after `opts := UpdateFileOptions{}`):

```go
// Handle file role change
if params.FileRole != nil && *params.FileRole != file.FileRole {
	oldRole := file.FileRole
	newRole := *params.FileRole

	// When downgrading from main to supplement, clear all main-file-only metadata
	if oldRole == models.FileRoleMain && newRole == models.FileRoleSupplement {
		// Clear cover fields
		file.CoverImagePath = nil
		file.CoverMimeType = nil
		file.CoverSource = nil
		file.CoverPage = nil
		opts.Columns = append(opts.Columns, "cover_image_path", "cover_mime_type", "cover_source", "cover_page")

		// Clear audiobook fields
		file.AudiobookDurationSeconds = nil
		file.AudiobookBitrateBps = nil
		opts.Columns = append(opts.Columns, "audiobook_duration_seconds", "audiobook_bitrate_bps")

		// Clear publisher/imprint
		file.PublisherID = nil
		file.PublisherSource = nil
		file.ImprintID = nil
		file.ImprintSource = nil
		opts.Columns = append(opts.Columns, "publisher_id", "publisher_source", "imprint_id", "imprint_source")

		// Clear release date
		file.ReleaseDate = nil
		file.ReleaseDateSource = nil
		opts.Columns = append(opts.Columns, "release_date", "release_date_source")

		// Clear URL
		file.URL = nil
		file.URLSource = nil
		opts.Columns = append(opts.Columns, "url", "url_source")

		// Clear narrator source (narrators will be handled separately)
		file.NarratorSource = nil
		opts.Columns = append(opts.Columns, "narrator_source")

		// Clear identifier source
		file.IdentifierSource = nil
		opts.Columns = append(opts.Columns, "identifier_source")

		// Delete narrators for this file
		_, err := h.bookService.DeleteNarratorsForFile(ctx, file.ID)
		if err != nil {
			log.Warn("failed to delete narrators on downgrade", logger.Data{"error": err.Error()})
		}

		// Delete identifiers for this file
		_, err = h.bookService.DeleteIdentifiersForFile(ctx, file.ID)
		if err != nil {
			log.Warn("failed to delete identifiers on downgrade", logger.Data{"error": err.Error()})
		}
	}

	file.FileRole = newRole
	opts.Columns = append(opts.Columns, "file_role")
}
```

**Step 3: Add DeleteNarratorsForFile method to service**

In `pkg/books/service.go`, add:

```go
// DeleteNarratorsForFile deletes all narrators for a file.
func (svc *Service) DeleteNarratorsForFile(ctx context.Context, fileID int) (int, error) {
	result, err := svc.db.NewDelete().
		Model((*models.Narrator)(nil)).
		Where("file_id = ?", fileID).
		Exec(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// DeleteIdentifiersForFile deletes all identifiers for a file.
func (svc *Service) DeleteIdentifiersForFile(ctx context.Context, fileID int) (int, error) {
	result, err := svc.db.NewDelete().
		Model((*models.FileIdentifier)(nil)).
		Where("file_id = ?", fileID).
		Exec(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
```

**Step 4: Run tests**

Run: `make test`
Expected: Tests pass

**Step 5: Commit**

```bash
git add pkg/books/validators.go pkg/books/handlers.go pkg/books/service.go
git commit -m "$(cat <<'EOF'
[API] Support file_role changes in file update endpoint

Allows changing file_role between 'main' and 'supplement'.
When downgrading to supplement, clears all main-file-only metadata
including cover, audiobook fields, publisher, imprint, narrators,
and identifiers.
EOF
)"
```

---

## Task 8: Add Supplements Section to BookDetail Frontend

**Files:**
- Modify: `app/components/pages/BookDetail.tsx:458-625`

**Step 1: Add helper to filter files by role**

In `app/components/pages/BookDetail.tsx`, add after line 247 (after `const book = bookQuery.data;`):

```tsx
// Separate main files and supplements
const mainFiles = book.files?.filter((f) => f.file_role !== "supplement") ?? [];
const supplements = book.files?.filter((f) => f.file_role === "supplement") ?? [];
```

**Step 2: Update Files section to use mainFiles**

Change line 461 (in the Files section):

```tsx
<h3 className="font-semibold mb-3">
  Files ({mainFiles.length})
</h3>
```

And change line 464 to iterate over mainFiles instead of book.files:

```tsx
{mainFiles.map((file) => (
```

**Step 3: Add Supplements section after Files section**

After line 624 (after the closing `</div>` of the Files section), add:

```tsx
{/* Supplements */}
{supplements.length > 0 && (
  <>
    <Separator />
    <div>
      <h3 className="font-semibold mb-3">
        Supplements ({supplements.length})
      </h3>
      <div className="space-y-2">
        {supplements.map((file) => (
          <div
            className="border-l-4 border-l-muted-foreground/30 pl-4 py-2"
            key={file.id}
          >
            <div className="flex items-center justify-between gap-4">
              <div className="flex items-center gap-2 min-w-0 flex-1">
                <Badge
                  className="uppercase text-xs"
                  variant="outline"
                >
                  {file.file_type}
                </Badge>
                <span className="text-sm truncate">
                  {file.filepath.split("/").pop()}
                </span>
              </div>
              <div className="flex items-center gap-3 text-xs text-muted-foreground flex-shrink-0">
                <span>{formatFileSize(file.filesize_bytes)}</span>
                <Button
                  onClick={() => handleDownloadOriginal(file.id)}
                  size="sm"
                  title="Download"
                  variant="ghost"
                >
                  <Download className="h-3 w-3" />
                </Button>
                <Button
                  onClick={() => setEditingFile(file)}
                  size="sm"
                  title="Edit"
                  variant="ghost"
                >
                  <Edit className="h-3 w-3" />
                </Button>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  </>
)}
```

**Step 4: Verify types are generated**

Run: `make tygo`
Expected: Types include `file_role` field

**Step 5: Run lint**

Run: `yarn lint`
Expected: No errors

**Step 6: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[UI] Add supplements section to book detail page

Displays supplement files in a separate section below main files.
Supplements show filename, file type badge, size, and download button.
Downloads supplements directly (no processing).
EOF
)"
```

---

## Task 9: Update FileEditDialog for Role-Based Forms

**Files:**
- Modify: `app/components/library/FileEditDialog.tsx`

**Step 1: Add file_role state and imports**

In `app/components/library/FileEditDialog.tsx`, add to imports (line 39):

```tsx
import { FileTypeCBZ, FileRoleMain, FileRoleSupplement, type File } from "@/types";
```

Add state after line 112 (after releaseDate state):

```tsx
const [fileRole, setFileRole] = useState(file.file_role ?? FileRoleMain);
const [showDowngradeConfirm, setShowDowngradeConfirm] = useState(false);
```

**Step 2: Reset file_role in useEffect**

Add to the useEffect at line 147 (inside the `if (open)` block):

```tsx
setFileRole(file.file_role ?? FileRoleMain);
setShowDowngradeConfirm(false);
```

**Step 3: Add file_role change handling to handleSubmit**

In `handleSubmit` function, add before `Object.keys(payload).length` check (around line 312):

```tsx
// Check if file role changed
if (fileRole !== (file.file_role ?? FileRoleMain)) {
  // If downgrading to supplement, require confirmation
  if (fileRole === FileRoleSupplement && !showDowngradeConfirm) {
    setShowDowngradeConfirm(true);
    return;
  }
  payload.file_role = fileRole;
}
```

Update the payload type to include file_role:

```tsx
const payload: {
  file_role?: string;
  narrators?: string[];
  url?: string;
  publisher?: string;
  imprint?: string;
  release_date?: string;
  identifiers?: Array<{ type: string; value: string }>;
} = {};
```

**Step 4: Add isSupplement check**

After the state declarations, add:

```tsx
const isSupplement = file.file_role === FileRoleSupplement;
```

**Step 5: Add Role Toggle UI**

In the dialog content (around line 334), add as the first form section:

```tsx
{/* File Role */}
<div className="space-y-2">
  <Label>File Role</Label>
  <Select
    onValueChange={setFileRole}
    value={fileRole}
  >
    <SelectTrigger>
      <SelectValue />
    </SelectTrigger>
    <SelectContent>
      <SelectItem value={FileRoleMain}>Main File</SelectItem>
      <SelectItem value={FileRoleSupplement}>Supplement</SelectItem>
    </SelectContent>
  </Select>
  {showDowngradeConfirm && (
    <p className="text-sm text-destructive">
      Changing to supplement will clear all metadata (narrators, identifiers, publisher, etc.). Click Save again to confirm.
    </p>
  )}
</div>
```

**Step 6: Conditionally render main-file-only sections**

Wrap the following sections with `{!isSupplement && fileRole !== FileRoleSupplement && (` ... `)}`:
- Cover Upload section (lines 351-392)
- Narrators section (lines 394-438)
- URL section (lines 441-451)
- Publisher section (lines 453-551)
- Imprint section (lines 553-647)
- Release Date section (lines 649-658)
- Identifiers section (lines 660-724)

**Step 7: Run lint and fix**

Run: `yarn lint`
Expected: No errors (or fix any issues)

**Step 8: Commit**

```bash
git add app/components/library/FileEditDialog.tsx
git commit -m "$(cat <<'EOF'
[UI] Add role toggle to file edit dialog

FileEditDialog now shows:
- For supplements: only role toggle (to upgrade to main)
- For main files: full form + role toggle (with downgrade confirmation)
Downgrading to supplement shows warning and requires double-save to confirm.
EOF
)"
```

---

## Task 10: Run Full Test Suite and Verify

**Step 1: Run backend tests**

Run: `make test`
Expected: All tests pass

**Step 2: Run linters**

Run: `make check`
Expected: All checks pass

**Step 3: Manual verification**

1. Start dev server: `make start`
2. Create a test book directory with main file + supplements
3. Run a library scan
4. Verify supplements appear in book detail
5. Test downloading a supplement
6. Test changing file role from main to supplement

**Step 4: Final commit if any fixes needed**

```bash
git add -A
git commit -m "[Fix] Address any issues found in testing"
```

---

## Task 11: Create Final Summary Commit

**Step 1: Review all changes**

Run: `git log --oneline -10`

**Step 2: Update design doc if needed**

If implementation differed from design, update `docs/plans/2026-01-14-supplementary-files-design.md`

**Step 3: Done!**

The supplementary files feature is complete. Main functionality:
- Database: `file_role` column distinguishes main files from supplements
- Config: `supplement_exclude_patterns` controls what files are excluded
- Scanner: Discovers supplements in book directories and for root-level books
- API: Download handler delegates supplements to original file endpoint
- Frontend: Book detail shows supplements section, FileEditDialog supports role changes
