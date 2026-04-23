# Manual Book Title Edit: Sync File Name Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a user manually edits a book's title, sync each main file's `Name` to the new title — but only when `file.Name` is unset or already matches the old title. Custom filenames survive.

**Architecture:** Extend the existing title-update block in the `update` handler at `pkg/books/handlers.go:238-250`. Before overwriting `book.Title`, capture the old title. After the title update, iterate `book.Files`, apply the match rule (nil/empty OR trim+casefold equals old title), and persist any changed files via the existing `bookService.UpdateFile(ctx, file, books.UpdateFileOptions{Columns: []string{"name", "name_source"}})` method at `pkg/books/service.go:686`.

**Tech Stack:** Go, Echo, Bun ORM, SQLite. Tests use `testify`, Echo's `httptest`, and the existing handler-test helpers (`setupTestDB`, `setupTestServer`, `setupTestFile`, `setupTestUser`, `executeRequestWithUser`) in `pkg/books/handlers_test.go`.

**Spec:** `docs/superpowers/specs/2026-04-23-manual-title-file-name-sync-design.md`

---

## Task 1: Red — Write the Happy-Path Test

**Files:**
- Test: `pkg/books/handlers_test.go` — append new test function at end of file.

- [ ] **Step 1: Write the failing test**

Append to `pkg/books/handlers_test.go`. The test creates a book with `Title = "Foo"` and one main file with `Name = "Foo"`, POSTs to `/books/:id` with `{"title": "Bar"}`, and asserts the file row in the DB has `Name = "Bar"` and `NameSource = "manual"`.

```go
func TestUpdateBook_Title_UpdatesMainFileName_WhenMatchesOldTitle(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	bookDir := t.TempDir()
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Foo",
		TitleSource:     models.DataSourceManual,
		SortTitle:       "Foo",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	epubPath := filepath.Join(bookDir, "test.epub")
	require.NoError(t, os.WriteFile(epubPath, []byte("epub content"), 0644))
	file := setupTestFile(t, db, book, models.FileTypeEPUB, epubPath)

	// Seed file.Name = old title ("Foo") so the sync rule triggers.
	name := "Foo"
	nameSource := models.DataSourceFilepath
	file.Name = &name
	file.NameSource = &nameSource
	_, err = db.NewUpdate().
		Model(file).
		Column("name", "name_source").
		WherePK().
		Exec(ctx)
	require.NoError(t, err)

	user := setupTestUser(t, db, library.ID, true)
	err = db.NewSelect().
		Model(user).
		Relation("Role").
		Relation("Role.Permissions").
		Where("u.id = ?", user.ID).
		Scan(ctx)
	require.NoError(t, err)

	e := setupTestServer(t, db)
	body := `{"title": "Bar"}`
	req := httptest.NewRequest(http.MethodPost, "/books/"+strconv.Itoa(book.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err = db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "Bar", *updated.Name)
	require.NotNil(t, updated.NameSource)
	assert.Equal(t, models.DataSourceManual, *updated.NameSource)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./pkg/books/ -run TestUpdateBook_Title_UpdatesMainFileName_WhenMatchesOldTitle -v
```

Expected: FAIL. `updated.Name` will still be `"Foo"` because the handler doesn't sync yet.

- [ ] **Step 3: Add the sync logic in the update handler**

Modify `pkg/books/handlers.go` in the title-update block that currently starts at line 238. Replace this block:

```go
	// Update title
	if params.Title != nil && *params.Title != book.Title {
		book.Title = *params.Title
		book.TitleSource = models.DataSourceManual
		opts.Columns = append(opts.Columns, "title", "title_source")
		shouldOrganizeFiles = true
		// Regenerate sort title when title changes (unless sort_title_source is manual)
		if book.SortTitleSource != models.DataSourceManual {
			book.SortTitle = sortname.ForTitle(*params.Title)
			book.SortTitleSource = models.DataSourceFilepath
			opts.Columns = append(opts.Columns, "sort_title", "sort_title_source")
		}
	}
```

with:

```go
	// Update title
	if params.Title != nil && *params.Title != book.Title {
		oldTitle := book.Title
		newTitle := *params.Title
		book.Title = newTitle
		book.TitleSource = models.DataSourceManual
		opts.Columns = append(opts.Columns, "title", "title_source")
		shouldOrganizeFiles = true
		// Regenerate sort title when title changes (unless sort_title_source is manual)
		if book.SortTitleSource != models.DataSourceManual {
			book.SortTitle = sortname.ForTitle(newTitle)
			book.SortTitleSource = models.DataSourceFilepath
			opts.Columns = append(opts.Columns, "sort_title", "sort_title_source")
		}
		// Sync file.Name on each main file whose current Name is unset or
		// matches the old title (trim + case-insensitive). Custom filenames
		// that deliberately differ from the book title are preserved.
		for _, f := range book.Files {
			if f.FileRole != models.FileRoleMain {
				continue
			}
			currentEmpty := f.Name == nil || *f.Name == ""
			currentMatches := false
			if f.Name != nil {
				currentMatches = strings.EqualFold(strings.TrimSpace(*f.Name), strings.TrimSpace(oldTitle))
			}
			if !currentEmpty && !currentMatches {
				continue
			}
			nameCopy := newTitle
			manualSource := models.DataSourceManual
			f.Name = &nameCopy
			f.NameSource = &manualSource
			if err := h.bookService.UpdateFile(ctx, f, UpdateFileOptions{Columns: []string{"name", "name_source"}}); err != nil {
				log.Warn("failed to update file name on title change", logger.Data{"file_id": f.ID, "error": err.Error()})
			}
		}
	}
```

Notes:
- `oldTitle` must be captured before the `book.Title = newTitle` assignment.
- `strings.EqualFold` already does a case-insensitive compare, so passing trimmed values without manually lowercasing is correct and idiomatic.
- `strings`, `log`, and `logger.Data` are already used elsewhere in `handlers.go`. Verify the imports at the top of the file already include `"strings"` — if somehow not, add it.

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./pkg/books/ -run TestUpdateBook_Title_UpdatesMainFileName_WhenMatchesOldTitle -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/books/handlers.go pkg/books/handlers_test.go
git commit -m "[Backend] Sync file.Name on manual book title edit when it matches old title"
```

---

## Task 2: Add the Remaining Behavior Tests

The implementation in Task 1 is written to cover every branch of the rule, so these tests document and lock in each case. Add them as separate test functions so failures point at the exact condition that regressed.

**Files:**
- Test: `pkg/books/handlers_test.go` — append each test at end of file.

- [ ] **Step 1: Add nil / empty / trim-and-casefold / preserve / supplement / multi-file / unchanged tests**

Append the following test functions after the one from Task 1. Each one follows the same shape as Task 1 (build library/book/file, POST, inspect DB), only changing the initial state and assertions.

```go
func TestUpdateBook_Title_UpdatesNilFileName_ToNewTitle(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	library, book, file := seedBookAndFile(t, db, "Foo", nil, nil, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Bar")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err := db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "Bar", *updated.Name)
	require.NotNil(t, updated.NameSource)
	assert.Equal(t, models.DataSourceManual, *updated.NameSource)
}

func TestUpdateBook_Title_UpdatesEmptyFileName_ToNewTitle(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	emptyName := ""
	filepathSource := models.DataSourceFilepath
	library, book, file := seedBookAndFile(t, db, "Foo", &emptyName, &filepathSource, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Bar")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err := db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "Bar", *updated.Name)
}

func TestUpdateBook_Title_MatchesWithTrimAndCasefold(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	fileName := "  foo bar  "
	filepathSource := models.DataSourceFilepath
	library, book, file := seedBookAndFile(t, db, "Foo Bar", &fileName, &filepathSource, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Baz")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err := db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "Baz", *updated.Name)
}

func TestUpdateBook_Title_PreservesCustomFileName_WhenDiffers(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	customName := "Baz"
	manualSource := models.DataSourceManual
	library, book, file := seedBookAndFile(t, db, "Foo", &customName, &manualSource, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Bar")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err := db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "Baz", *updated.Name, "custom file.Name that differs from old title must be preserved")
	require.NotNil(t, updated.NameSource)
	assert.Equal(t, models.DataSourceManual, *updated.NameSource)
}

func TestUpdateBook_Title_DoesNotTouchSupplementFileName(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	matchingName := "Foo"
	filepathSource := models.DataSourceFilepath
	library, book, supplement := seedBookAndFile(t, db, "Foo", &matchingName, &filepathSource, models.FileRoleSupplement)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Bar")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err := db.NewSelect().Model(&updated).Where("id = ?", supplement.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "Foo", *updated.Name, "supplement file name must not be synced from book title")
	require.NotNil(t, updated.NameSource)
	assert.Equal(t, models.DataSourceFilepath, *updated.NameSource)
}

func TestUpdateBook_Title_MultipleMainFiles_IndependentlyChecked(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	bookDir := t.TempDir()
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Foo",
		TitleSource:     models.DataSourceManual,
		SortTitle:       "Foo",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	matchingPath := filepath.Join(bookDir, "match.epub")
	require.NoError(t, os.WriteFile(matchingPath, []byte("x"), 0644))
	matchingFile := setupTestFile(t, db, book, models.FileTypeEPUB, matchingPath)
	matchingName := "Foo"
	filepathSource := models.DataSourceFilepath
	matchingFile.Name = &matchingName
	matchingFile.NameSource = &filepathSource
	_, err = db.NewUpdate().Model(matchingFile).Column("name", "name_source").WherePK().Exec(ctx)
	require.NoError(t, err)

	customPath := filepath.Join(bookDir, "custom.epub")
	require.NoError(t, os.WriteFile(customPath, []byte("x"), 0644))
	customFile := setupTestFile(t, db, book, models.FileTypeEPUB, customPath)
	customName := "Totally Different"
	manualSource := models.DataSourceManual
	customFile.Name = &customName
	customFile.NameSource = &manualSource
	_, err = db.NewUpdate().Model(customFile).Column("name", "name_source").WherePK().Exec(ctx)
	require.NoError(t, err)

	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Bar")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updatedMatching models.File
	err = db.NewSelect().Model(&updatedMatching).Where("id = ?", matchingFile.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updatedMatching.Name)
	assert.Equal(t, "Bar", *updatedMatching.Name)

	var updatedCustom models.File
	err = db.NewSelect().Model(&updatedCustom).Where("id = ?", customFile.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updatedCustom.Name)
	assert.Equal(t, "Totally Different", *updatedCustom.Name)
}

func TestUpdateBook_Title_Unchanged_DoesNotTouchFileName(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	// file.Name intentionally set to a differently-cased variant of the title
	// so if the sync ran it would normalize the value. We expect it NOT to run
	// because the title itself is unchanged.
	fileName := "foo"
	filepathSource := models.DataSourceFilepath
	library, book, file := seedBookAndFile(t, db, "Foo", &fileName, &filepathSource, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Foo")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err := db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "foo", *updated.Name, "no-op title change must not touch file.Name")
	require.NotNil(t, updated.NameSource)
	assert.Equal(t, models.DataSourceFilepath, *updated.NameSource,
		"no-op title change must not touch file.NameSource")
}
```

- [ ] **Step 2: Add the shared test helpers the tests above depend on**

Add these helpers near the other test helpers in `pkg/books/handlers_test.go` (somewhere after `setupTestServer`, before the first `Test*` function). They DRY up the bookkeeping shared across the new tests.

```go
// seedBookAndFile inserts a library, a book with the given title, and a file
// with the given role/name/name-source. Returns the library, the book, and
// the file. The book uses a directory-backed layout rooted at t.TempDir().
func seedBookAndFile(
	t *testing.T,
	db *bun.DB,
	bookTitle string,
	fileName *string,
	fileNameSource *string,
	role string,
) (*models.Library, *models.Book, *models.File) {
	t.Helper()
	ctx := context.Background()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	bookDir := t.TempDir()
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           bookTitle,
		TitleSource:     models.DataSourceManual,
		SortTitle:       bookTitle,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	filePath := filepath.Join(bookDir, "test.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("epub content"), 0644))
	file := setupTestFile(t, db, book, models.FileTypeEPUB, filePath)
	file.FileRole = role
	_, err = db.NewUpdate().Model(file).Column("file_role").WherePK().Exec(ctx)
	require.NoError(t, err)

	if fileName != nil || fileNameSource != nil {
		file.Name = fileName
		file.NameSource = fileNameSource
		_, err = db.NewUpdate().Model(file).Column("name", "name_source").WherePK().Exec(ctx)
		require.NoError(t, err)
	}

	return library, book, file
}

// loadUserWithRole reloads a user with Role and Role.Permissions so the
// RequirePermission middleware passes in tests.
func loadUserWithRole(t *testing.T, db *bun.DB, user *models.User) *models.User {
	t.Helper()
	ctx := context.Background()
	err := db.NewSelect().
		Model(user).
		Relation("Role").
		Relation("Role.Permissions").
		Where("u.id = ?", user.ID).
		Scan(ctx)
	require.NoError(t, err)
	return user
}

// newUpdateTitleRequest builds a POST /books/:id request with a JSON body
// containing only a title change.
func newUpdateTitleRequest(bookID int, newTitle string) *http.Request {
	body := `{"title": ` + strconv.Quote(newTitle) + `}`
	req := httptest.NewRequest(http.MethodPost, "/books/"+strconv.Itoa(bookID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}
```

- [ ] **Step 3: Refactor Task 1's test to use the new helpers (optional DRY pass)**

If time permits, rewrite the Task 1 test to use `seedBookAndFile`, `loadUserWithRole`, and `newUpdateTitleRequest` so all the new tests share the same shape. This is optional — functionally the test is already correct.

- [ ] **Step 4: Run the full new-test suite**

Run:
```bash
go test ./pkg/books/ -run 'TestUpdateBook_Title_' -v
```

Expected: all seven tests PASS.

- [ ] **Step 5: Run the full books package tests to catch regressions**

Run:
```bash
go test ./pkg/books/ -count=1
```

Expected: PASS. If anything else in the package broke, the implementation from Task 1 has an unintended side effect — stop and investigate rather than papering over.

- [ ] **Step 6: Commit**

```bash
git add pkg/books/handlers_test.go
git commit -m "[Test] Cover file.Name sync edge cases on manual book title edit"
```

---

## Task 3: Update Docs

**Files:**
- Modify: `website/docs/directory-structure.md:30-32`

- [ ] **Step 1: Extend the Organize Files section to mention manual edits**

Replace lines 30–32 in `website/docs/directory-structure.md`:

```markdown
Shisho includes an optional "Organize Files" feature in library settings that can automatically organize your books into a consistent directory structure. When enabled, Shisho will move and rename files based on metadata — during scans, and also when you identify a book and apply a plugin search result (the target file is renamed to match the identified title).

If you prefer to manage your own file organization, you can leave this disabled and Shisho will work with whatever structure you have. With Organize Files disabled, identify still updates the book's title and the target file's stored name, but no files are moved or renamed on disk.
```

with:

```markdown
Shisho includes an optional "Organize Files" feature in library settings that can automatically organize your books into a consistent directory structure. When enabled, Shisho will move and rename files based on metadata — during scans, when you identify a book and apply a plugin search result (the target file is renamed to match the identified title), and when you manually edit a book's title (each main file whose stored name still matches the old title is renamed too; custom filenames that differ from the book title are preserved).

If you prefer to manage your own file organization, you can leave this disabled and Shisho will work with whatever structure you have. With Organize Files disabled, these actions still update the book's title and the corresponding files' stored names in the database, but no files are moved or renamed on disk.
```

- [ ] **Step 2: Spot-check the rendered doc (optional)**

If `mise docs` is already running, eyeball the Directory Structure page. If not, skip — the content is plain Markdown with no Docusaurus-specific features.

- [ ] **Step 3: Commit**

```bash
git add website/docs/directory-structure.md
git commit -m "[Docs] Note manual title edits also sync file.Name"
```

---

## Task 4: Final Verification

- [ ] **Step 1: Run `mise check:quiet`**

Run:
```bash
mise check:quiet
```

Expected: PASS. This runs Go tests, Go lint, JS lint, and JS tests.

If anything fails: read the failure, fix the root cause, re-run. Do not loop multiple times — one invocation gives a clear pass/fail summary.

- [ ] **Step 2: Sanity-check manually (optional)**

If `mise start` is convenient, start the app, create a library with a book whose `file.Name` matches its title, edit the title in the UI, and verify in the Files panel or via the DB that the file's name updates. Then repeat with a book whose `file.Name` has been manually set to something different, and verify the file's name does NOT change. Write down the result in the PR description.

---

## Self-Review (done while writing this plan)

**Spec coverage:**
- Rule (nil/empty OR trim+casefold match) → Task 1 Step 3 implementation and Task 2 Steps 1–2 edge-case tests.
- Supplements skipped → `TestUpdateBook_Title_DoesNotTouchSupplementFileName` (Task 2).
- `NameSource = DataSourceManual` on updated files → Task 1 happy-path test asserts it; Task 2 `_PreservesCustomFileName_WhenDiffers` asserts the opposite case.
- Ordering (sweep runs before `bookService.UpdateBook`) → Task 1 Step 3 places the loop inside the title block, which runs before `h.bookService.UpdateBook(ctx, book, opts)` at line 531.
- Failure behavior (log and continue) → Task 1 Step 3 uses `log.Warn` on per-file failure.
- Docs → Task 3.
- Multiple main files, independently checked → Task 2 `_MultipleMainFiles_IndependentlyChecked`.

**Placeholder scan:** No TODOs, no "similar to", no unwritten code. The explanatory note inside Task 1 Step 3 about `EqualFold` is collapsed to a single final code block so the engineer isn't asked to synthesize two options.

**Type consistency:** `UpdateFileOptions{Columns: []string{"name", "name_source"}}` matches the existing signature in `pkg/books/service.go:686`. `models.FileRoleMain` / `models.FileRoleSupplement` and `models.DataSourceManual` / `models.DataSourceFilepath` are constants already used throughout the handler and tests.
