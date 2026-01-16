# File Name Field Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `name` field to files that displays prominently in the UI as the primary identifier, defaults to title from file metadata, and can be manually overridden.

**Architecture:** The name field follows the existing pattern of nullable fields with source tracking. It's populated during scanning from file metadata (Title), can be overridden via sidecar or manual edit, and affects file generation output. The fingerprint includes name for cache invalidation.

**Tech Stack:** Go (Echo, Bun ORM), React/TypeScript, SQLite

---

## Task 1: Database Migration

**Files:**
- Modify: `pkg/migrations/20250321211048_create_initial_tables.go`

**Step 1: Add columns to files table**

In `pkg/migrations/20250321211048_create_initial_tables.go`, find the files table CREATE statement (around line 141) and add two new columns after `cover_page`:

```go
// Add after line ~167 (after cover_page INTEGER,)
name TEXT,
name_source TEXT,
```

The full context - add these lines in the files table definition:
```go
cover_page INTEGER,
name TEXT,
name_source TEXT,
```

**Step 2: Run migration to verify**

```bash
make db:rollback && make db:migrate
```

Expected: Migration runs successfully, no errors.

**Step 3: Commit**

```bash
git add pkg/migrations/20250321211048_create_initial_tables.go
git commit -m "[Database] Add name and name_source columns to files table"
```

---

## Task 2: Model Changes

**Files:**
- Modify: `pkg/models/file.go`

**Step 1: Add fields to File struct**

In `pkg/models/file.go`, add the Name and NameSource fields to the File struct. Add them after the existing `CoverPage` field (around line 35):

```go
Name       *string `json:"name"`
NameSource *string `json:"name_source" tstype:"DataSource"`
```

**Step 2: Generate TypeScript types**

```bash
make tygo
```

Expected: Types regenerated (may say "Nothing to be done" if already up-to-date, which is fine).

**Step 3: Run tests to verify no regressions**

```bash
make test
```

Expected: All tests pass.

**Step 4: Commit**

```bash
git add pkg/models/file.go
git commit -m "[Model] Add Name and NameSource fields to File struct"
```

---

## Task 3: Sidecar Types

**Files:**
- Modify: `pkg/sidecar/types.go`

**Step 1: Add Name to FileSidecar struct**

In `pkg/sidecar/types.go`, add the Name field to the `FileSidecar` struct (around line 23-31):

```go
type FileSidecar struct {
	Version     int                  `json:"version"`
	Narrators   []NarratorMetadata   `json:"narrators,omitempty"`
	URL         *string              `json:"url,omitempty"`
	Publisher   *string              `json:"publisher,omitempty"`
	Imprint     *string              `json:"imprint,omitempty"`
	ReleaseDate *string              `json:"release_date,omitempty"`
	Identifiers []IdentifierMetadata `json:"identifiers,omitempty"`
	Name        *string              `json:"name,omitempty"`
}
```

**Step 2: Run tests**

```bash
make test
```

Expected: All tests pass.

**Step 3: Commit**

```bash
git add pkg/sidecar/types.go
git commit -m "[Sidecar] Add Name field to FileSidecar struct"
```

---

## Task 4: Sidecar Conversion

**Files:**
- Modify: `pkg/sidecar/sidecar.go`
- Test: `pkg/sidecar/sidecar_test.go`

**Step 1: Write failing test for FileSidecarFromModel with Name**

In `pkg/sidecar/sidecar_test.go`, add a test case that verifies Name is included in the sidecar:

```go
func TestFileSidecarFromModel_Name(t *testing.T) {
	name := "Custom File Name"
	file := &models.File{
		Name: &name,
	}

	sidecar := FileSidecarFromModel(file)

	assert.NotNil(t, sidecar.Name)
	assert.Equal(t, "Custom File Name", *sidecar.Name)
}

func TestFileSidecarFromModel_NilName(t *testing.T) {
	file := &models.File{
		Name: nil,
	}

	sidecar := FileSidecarFromModel(file)

	assert.Nil(t, sidecar.Name)
}
```

**Step 2: Run test to verify it fails**

```bash
go test -v ./pkg/sidecar/... -run TestFileSidecarFromModel_Name
```

Expected: FAIL - Name is not being set in the sidecar.

**Step 3: Update FileSidecarFromModel to include Name**

In `pkg/sidecar/sidecar.go`, in the `FileSidecarFromModel` function, add Name to the returned struct initialization:

```go
func FileSidecarFromModel(file *models.File) *FileSidecar {
	s := &FileSidecar{
		Version:   CurrentVersion,
		URL:       file.URL,
		Publisher: nil,
		Imprint:   nil,
		Name:      file.Name,  // Add this line
	}
	// ... rest of function
```

**Step 4: Run test to verify it passes**

```bash
go test -v ./pkg/sidecar/... -run TestFileSidecarFromModel_Name
```

Expected: PASS

**Step 5: Run all tests**

```bash
make test
```

Expected: All tests pass.

**Step 6: Commit**

```bash
git add pkg/sidecar/sidecar.go pkg/sidecar/sidecar_test.go
git commit -m "[Sidecar] Include Name field in FileSidecarFromModel conversion"
```

---

## Task 5: Download Fingerprint

**Files:**
- Modify: `pkg/downloadcache/fingerprint.go`
- Test: `pkg/downloadcache/fingerprint_test.go`

**Step 1: Write failing test for fingerprint with Name**

In `pkg/downloadcache/fingerprint_test.go`, add a test that verifies Name affects the fingerprint:

```go
func TestComputeFingerprint_IncludesFileName(t *testing.T) {
	name := "Custom Edition Name"
	book := &models.Book{
		Title: "Test Book",
	}
	file := &models.File{
		Name: &name,
	}

	fp, err := ComputeFingerprint(book, file)

	assert.NoError(t, err)
	assert.NotNil(t, fp.Name)
	assert.Equal(t, "Custom Edition Name", *fp.Name)
}

func TestComputeFingerprint_DifferentNamesProduceDifferentHashes(t *testing.T) {
	book := &models.Book{Title: "Test Book"}

	name1 := "Edition A"
	file1 := &models.File{Name: &name1}
	fp1, _ := ComputeFingerprint(book, file1)

	name2 := "Edition B"
	file2 := &models.File{Name: &name2}
	fp2, _ := ComputeFingerprint(book, file2)

	hash1, _ := fp1.Hash()
	hash2, _ := fp2.Hash()

	assert.NotEqual(t, hash1, hash2, "Different names should produce different hashes")
}
```

**Step 2: Run test to verify it fails**

```bash
go test -v ./pkg/downloadcache/... -run TestComputeFingerprint_IncludesFileName
```

Expected: FAIL - Name field doesn't exist on Fingerprint.

**Step 3: Add Name to Fingerprint struct**

In `pkg/downloadcache/fingerprint.go`, add Name field to the Fingerprint struct:

```go
type Fingerprint struct {
	Title       string                  `json:"title"`
	Subtitle    *string                 `json:"subtitle,omitempty"`
	Description *string                 `json:"description,omitempty"`
	Authors     []FingerprintAuthor     `json:"authors"`
	Narrators   []FingerprintNarrator   `json:"narrators"`
	Series      []FingerprintSeries     `json:"series"`
	Genres      []string                `json:"genres"`
	Tags        []string                `json:"tags"`
	Identifiers []FingerprintIdentifier `json:"identifiers,omitempty"`
	URL         *string                 `json:"url,omitempty"`
	Publisher   *string                 `json:"publisher,omitempty"`
	Imprint     *string                 `json:"imprint,omitempty"`
	ReleaseDate *time.Time              `json:"release_date,omitempty"`
	Cover       *FingerprintCover       `json:"cover,omitempty"`
	Format      string                  `json:"format,omitempty"`
	Name        *string                 `json:"name,omitempty"`  // Add this line
}
```

**Step 4: Update ComputeFingerprint to include Name**

In the `ComputeFingerprint` function, add Name from the file:

```go
// Add file-level metadata
if file != nil {
	fp.URL = file.URL
	fp.ReleaseDate = file.ReleaseDate
	fp.Name = file.Name  // Add this line
	// ... rest of existing code
}
```

**Step 5: Run test to verify it passes**

```bash
go test -v ./pkg/downloadcache/... -run TestComputeFingerprint_IncludesFileName
```

Expected: PASS

**Step 6: Run all tests**

```bash
make test
```

Expected: All tests pass.

**Step 7: Commit**

```bash
git add pkg/downloadcache/fingerprint.go pkg/downloadcache/fingerprint_test.go
git commit -m "[Cache] Add Name field to download fingerprint for cache invalidation"
```

---

## Task 6: Scanner - Populate Name from Metadata

**Files:**
- Modify: `pkg/worker/scan.go`

**Step 1: Add Name variables in scanning logic**

In `pkg/worker/scan.go`, find the section where file metadata variables are initialized (around line 400-420, where `fileURL`, `fileURLSource`, etc. are declared) and add:

```go
var fileName *string
fileNameSource := models.DataSourceFilepath
```

**Step 2: Populate Name from parsed metadata**

After the metadata is parsed (where Title is extracted), add logic to populate fileName:

```go
// After extracting title from metadata (around line 450-480)
if metadata != nil {
	// ... existing title extraction ...

	// Populate file name from metadata title
	if trimmedTitle := strings.TrimSpace(metadata.Title); trimmedTitle != "" {
		fileName = &trimmedTitle
		fileNameSource = metadata.DataSource
	}
}
```

**Step 3: Apply sidecar override for Name**

In the sidecar application section (around line 1151-1211), add:

```go
// Apply file sidecar data for name (higher priority than file metadata)
if fileSidecarData != nil && fileSidecarData.Name != nil && *fileSidecarData.Name != "" {
	if models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[fileNameSource] {
		jobLog.Info("applying file sidecar data for name", "name", *fileSidecarData.Name)
		fileName = fileSidecarData.Name
		fileNameSource = models.DataSourceSidecar
	}
}
```

**Step 4: Set Name on new file creation**

In the file creation section (around line 1120-1130), add Name fields:

```go
file := &models.File{
	// ... existing fields ...
	Name:       fileName,
	NameSource: &fileNameSource,
}
```

**Step 5: Add Name update logic for existing files**

In the existing file update section (around line 1256-1353), add:

```go
// Update name if source priority is higher
if shouldUpdateScalar(fileName, existingFile.Name, fileNameSource, getSource(existingFile.NameSource)) {
	jobLog.Info("updating name", "from", safeDeref(existingFile.Name), "to", safeDeref(fileName))
	existingFile.Name = fileName
	existingFile.NameSource = &fileNameSource
	fileUpdated = true
	fileUpdateColumns = append(fileUpdateColumns, "name", "name_source")
}
```

**Step 6: Run tests**

```bash
make test
```

Expected: All tests pass.

**Step 7: Commit**

```bash
git add pkg/worker/scan.go
git commit -m "[Scanner] Populate Name and NameSource from file metadata during scanning"
```

---

## Task 7: File Generators - EPUB

**Files:**
- Modify: `pkg/filegen/epub.go`
- Test: `pkg/filegen/epub_test.go`

**Step 1: Write failing test**

In `pkg/filegen/epub_test.go`, add a test that verifies Name is written to the EPUB:

```go
func TestEPUBGenerator_UsesFileNameForTitle(t *testing.T) {
	// Setup test files
	srcPath := "testdata/sample.epub"
	destPath := t.TempDir() + "/output.epub"

	name := "Custom Edition Title"
	book := &models.Book{Title: "Original Title"}
	file := &models.File{Name: &name}

	generator := &EPUBGenerator{}
	err := generator.Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// Parse the output and verify title
	metadata, err := epub.Parse(destPath)
	require.NoError(t, err)
	assert.Equal(t, "Custom Edition Title", metadata.Title)
}
```

**Step 2: Run test to verify it fails**

```bash
go test -v ./pkg/filegen/... -run TestEPUBGenerator_UsesFileNameForTitle
```

Expected: FAIL - Title is set to book.Title, not file.Name.

**Step 3: Update EPUB generator to use file.Name**

In `pkg/filegen/epub.go`, modify the metadata building to prefer file.Name over book.Title:

```go
// In the Generate or buildMetadata function, update title logic:
title := book.Title
if file != nil && file.Name != nil && *file.Name != "" {
	title = *file.Name
}
```

**Step 4: Run test to verify it passes**

```bash
go test -v ./pkg/filegen/... -run TestEPUBGenerator_UsesFileNameForTitle
```

Expected: PASS

**Step 5: Commit**

```bash
git add pkg/filegen/epub.go pkg/filegen/epub_test.go
git commit -m "[Generator] Use file.Name for EPUB title when available"
```

---

## Task 8: File Generators - CBZ

**Files:**
- Modify: `pkg/filegen/cbz.go`
- Test: `pkg/filegen/cbz_test.go`

**Step 1: Write failing test**

```go
func TestCBZGenerator_UsesFileNameForTitle(t *testing.T) {
	srcPath := "testdata/sample.cbz"
	destPath := t.TempDir() + "/output.cbz"

	name := "Custom Comic Title"
	book := &models.Book{Title: "Original Title"}
	file := &models.File{Name: &name}

	generator := &CBZGenerator{}
	err := generator.Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// Parse and verify
	metadata, err := cbz.Parse(destPath)
	require.NoError(t, err)
	assert.Equal(t, "Custom Comic Title", metadata.Title)
}
```

**Step 2: Run test to verify it fails**

```bash
go test -v ./pkg/filegen/... -run TestCBZGenerator_UsesFileNameForTitle
```

Expected: FAIL

**Step 3: Update CBZ generator**

Similar pattern to EPUB - prefer file.Name when available.

**Step 4: Run test to verify it passes**

```bash
go test -v ./pkg/filegen/... -run TestCBZGenerator_UsesFileNameForTitle
```

Expected: PASS

**Step 5: Commit**

```bash
git add pkg/filegen/cbz.go pkg/filegen/cbz_test.go
git commit -m "[Generator] Use file.Name for CBZ title when available"
```

---

## Task 9: File Generators - M4B

**Files:**
- Modify: `pkg/filegen/m4b.go`
- Test: `pkg/filegen/m4b_test.go`

**Step 1: Write failing test**

```go
func TestM4BGenerator_UsesFileNameForTitle(t *testing.T) {
	srcPath := "testdata/sample.m4b"
	destPath := t.TempDir() + "/output.m4b"

	name := "Custom Audiobook Title"
	book := &models.Book{Title: "Original Title"}
	file := &models.File{Name: &name}

	generator := &M4BGenerator{}
	err := generator.Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// Parse and verify
	metadata, err := mp4.Parse(destPath)
	require.NoError(t, err)
	assert.Equal(t, "Custom Audiobook Title", metadata.Title)
}
```

**Step 2: Run test to verify it fails**

```bash
go test -v ./pkg/filegen/... -run TestM4BGenerator_UsesFileNameForTitle
```

Expected: FAIL

**Step 3: Update M4B generator**

In `pkg/filegen/m4b.go`, in the `buildMetadata` function, update title logic:

```go
func (g *M4BGenerator) buildMetadata(book *models.Book, file *models.File, src *mp4.Metadata) *mp4.Metadata {
	title := book.Title
	if file != nil && file.Name != nil && *file.Name != "" {
		title = *file.Name
	}

	meta := &mp4.Metadata{
		Title: title,
		// ... rest of function
	}
```

**Step 4: Run test to verify it passes**

```bash
go test -v ./pkg/filegen/... -run TestM4BGenerator_UsesFileNameForTitle
```

Expected: PASS

**Step 5: Commit**

```bash
git add pkg/filegen/m4b.go pkg/filegen/m4b_test.go
git commit -m "[Generator] Use file.Name for M4B title when available"
```

---

## Task 10: File Generators - KePub

**Files:**
- Modify: `pkg/kepub/cbz.go`
- Test: `pkg/kepub/cbz_test.go`

**Step 1: Write failing test**

Test that when converting CBZ to KePub, the file.Name is used in the generated EPUB.

**Step 2: Run test to verify it fails**

```bash
go test -v ./pkg/kepub/... -run TestKePubCBZ_UsesFileName
```

Expected: FAIL

**Step 3: Update KePub CBZ converter**

Ensure the CBZMetadata struct passed to the converter includes the Name field and it's used for the EPUB title.

**Step 4: Run test to verify it passes**

```bash
go test -v ./pkg/kepub/... -run TestKePubCBZ_UsesFileName
```

Expected: PASS

**Step 5: Commit**

```bash
git add pkg/kepub/cbz.go pkg/kepub/cbz_test.go
git commit -m "[Generator] Use file.Name for KePub conversion title when available"
```

---

## Task 11: Frontend - Display Name in BookDetail

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Update file display to show Name prominently**

In `BookDetail.tsx`, find where files are rendered. Update to show:
1. Name prominently (or filename fallback if Name is null)
2. Filename in muted text below

```tsx
// Helper to extract filename from filepath
const getFilename = (filepath: string) => {
  return filepath.split('/').pop() || filepath;
};

// In the file row rendering:
<div className="flex flex-col">
  <span className="font-medium truncate" title={file.name || getFilename(file.filepath)}>
    {file.name || getFilename(file.filepath)}
  </span>
  <span className="text-xs text-muted-foreground truncate" title={file.filepath}>
    {getFilename(file.filepath)}
  </span>
</div>
```

**Step 2: Verify in browser**

```bash
make start
```

Navigate to a book detail page and verify:
- Name displays prominently (or filename if no name)
- Filename shows below in muted text
- Truncation works on long names
- Hover shows full value

**Step 3: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "[UI] Display file Name prominently with filename below"
```

---

## Task 12: Frontend - File Edit Dialog Name Field

**Files:**
- Modify: `app/components/library/FileEditDialog.tsx`

**Step 1: Add Name input field**

In the FileEditDialog, add a Name input field:

```tsx
<div className="space-y-2">
  <Label htmlFor="name">Name</Label>
  <Input
    id="name"
    value={formData.name || ''}
    onChange={(e) => setFormData(prev => ({ ...prev, name: e.target.value || null }))}
    placeholder="Auto-derived from file metadata"
  />
  <p className="text-xs text-muted-foreground">
    Display name for this file. Leave empty to use the title from file metadata.
  </p>
</div>
```

**Step 2: Update form submission**

Ensure the form submission includes the name field and sets name_source to "manual" when changed.

**Step 3: Verify in browser**

```bash
make start
```

Test:
- Open file edit dialog
- Name field shows current value (or empty)
- Can edit and save
- Clearing the field removes the override

**Step 4: Commit**

```bash
git add app/components/library/FileEditDialog.tsx
git commit -m "[UI] Add Name field to file edit dialog"
```

---

## Task 13: Update Design Documentation

**Files:**
- Modify: `docs/plans/2026-01-15-file-ui-design.md`

**Step 1: Update design doc to reflect new layout**

Add documentation about the Name field display pattern:
- Primary row shows Name (or filename fallback)
- Filename shown below in muted text
- Truncation behavior
- Edit capability

**Step 2: Commit**

```bash
git add docs/plans/2026-01-15-file-ui-design.md
git commit -m "[Docs] Update file UI design with Name field layout"
```

---

## Task 14: Final Verification

**Step 1: Run all checks**

```bash
make check
```

Expected: All linting, type checking, and tests pass.

**Step 2: Test end-to-end flow**

1. Reset database: `make db:rollback && make db:migrate`
2. Start app: `make start`
3. Add a book with files
4. Verify Name is populated from metadata
5. Edit file Name manually
6. Verify manual Name persists through rescan
7. Clear Name and verify fallback to metadata
8. Download file and verify title in metadata

**Step 3: Final commit**

```bash
git add -A
git commit -m "[Feature] Complete file Name field implementation"
```

---

## Summary

This implementation adds a `name` field to files following existing patterns:

| Task | Component | Description |
|------|-----------|-------------|
| 1 | Database | Add name, name_source columns |
| 2 | Model | Add Name, NameSource fields |
| 3 | Sidecar Types | Add Name to FileSidecar |
| 4 | Sidecar Conversion | Include Name in FileSidecarFromModel |
| 5 | Fingerprint | Add Name for cache invalidation |
| 6 | Scanner | Populate Name from metadata with source tracking |
| 7-10 | Generators | Use file.Name for output title |
| 11-12 | Frontend | Display Name and allow editing |
| 13 | Docs | Update design documentation |
| 14 | Verification | End-to-end testing |
