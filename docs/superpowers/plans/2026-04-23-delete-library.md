# Delete Library Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a user-facing "Delete library" action to the library settings page, gated by `libraries:write`, that hard-deletes the library and all DB-resident content (rows + FTS) without touching files on disk.

**Architecture:** New `DELETE /api/libraries/:id` endpoint. A service method runs job cancellation → FTS purge → `DELETE FROM libraries` inside a single transaction, relying on existing `ON DELETE CASCADE` FKs for child tables. The existing `OnLibraryChanged` callback is invoked after commit to refresh the filesystem monitor. The unused `deleted_at` soft-delete plumbing on libraries is removed in the same change.

**Tech Stack:** Go 1.x, Echo, Bun ORM, SQLite FTS5; React 19, TanStack Query, TailwindCSS, Vitest; Docusaurus.

**Spec:** `docs/superpowers/specs/2026-04-23-delete-library-design.md`

**Before you start each task:**
- Read the spec section referenced in the task.
- Read the root `CLAUDE.md`, `pkg/CLAUDE.md`, `app/CLAUDE.md`, and `website/CLAUDE.md` — they contain project conventions (JSON naming, test parallelism, doc casing, destructive-action UX) that are review failures if violated.
- Use Red-Green-Refactor: every test must be observed failing before its implementation is written.

**Commit messages** follow the project convention `[Category] Short description` — see root `CLAUDE.md`. This plan uses `[Backend]`, `[Fix]` (removing dead code), `[Frontend]`, and `[Docs]`.

---

## Task 1: Remove `deleted_at` soft-delete plumbing

**Files:**
- Create: `pkg/migrations/20260423000000_drop_libraries_deleted_at.go`
- Modify: `pkg/models/library.go:16-28`
- Modify: `pkg/libraries/service.go:18-25, 119-157`
- Modify: `pkg/libraries/handlers.go:1-223`
- Modify: `pkg/libraries/validators.go:11-24`

This task removes the existing unused soft-delete code. The column is currently only toggled by the update handler via `params.Deleted`; no UI reads it, no background code honors it beyond the one list filter. Removing it in the same change keeps the model, migrations, and UI coherent.

- [ ] **Step 1: Add migration to drop the `deleted_at` column**

Create `pkg/migrations/20260423000000_drop_libraries_deleted_at.go`:

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE libraries DROP COLUMN deleted_at`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE libraries ADD COLUMN deleted_at TIMESTAMPTZ`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

- [ ] **Step 2: Remove the `DeletedAt` field from the Library model**

Edit `pkg/models/library.go` — delete the line `DeletedAt *time.Time \`json:"deleted_at,omitempty"\`` (line 27). If `time` is no longer referenced in the file, remove the import. After editing, the struct should end with the `LibraryPaths` field.

- [ ] **Step 3: Remove `IncludeDeleted` from list options and the `deleted_at IS NULL` filter**

In `pkg/libraries/service.go`:

Replace the `ListLibrariesOptions` struct (currently at lines 18-25) with:

```go
type ListLibrariesOptions struct {
	Limit      *int
	Offset     *int
	LibraryIDs []int // If set, only return libraries with these IDs

	includeTotal bool
}
```

In `listLibrariesWithTotal` (currently lines 119-157), delete the block:

```go
	if !opts.IncludeDeleted {
		q = q.Where("deleted_at IS NULL")
	}
```

- [ ] **Step 4: Remove the soft-delete branch in the update handler**

In `pkg/libraries/handlers.go`:

Remove the block (lines 194-201):

```go
	if params.Deleted != nil && (*params.Deleted && library.DeletedAt == nil || !*params.Deleted && library.DeletedAt != nil) {
		if *params.Deleted {
			library.DeletedAt = pointerutil.Time(time.Now())
		} else {
			library.DeletedAt = nil
		}
		opts.Columns = append(opts.Columns, "deleted_at")
	}
```

In the same file, update the `onLibraryChanged` guard (currently line 218) — it currently fires for `opts.UpdateLibraryPaths || slices.Contains(opts.Columns, "deleted_at")`. Simplify to just `opts.UpdateLibraryPaths`:

```go
	if h.onLibraryChanged != nil && opts.UpdateLibraryPaths {
		h.onLibraryChanged()
	}
```

In the `list` handler (currently lines 109-143), remove `IncludeDeleted: params.Deleted` from the `opts` struct literal.

Remove the now-unused imports from `pkg/libraries/handlers.go`:
- `"slices"` (only used by the removed condition)
- `"time"` (only used by `time.Now()` in the removed block)
- `"github.com/robinjoseph08/golib/pointerutil"` (only used for `pointerutil.Time`)

Verify by running `goimports -w pkg/libraries/handlers.go` or by letting the test build fail and catching unused-import errors.

- [ ] **Step 5: Remove the `Deleted` field from update/list payloads**

In `pkg/libraries/validators.go`:

Remove the `Deleted` field from `ListLibrariesQuery`:

```go
type ListLibrariesQuery struct {
	Limit  int `query:"limit" json:"limit,omitempty" default:"10" validate:"min=1,max=100"`
	Offset int `query:"offset" json:"offset,omitempty" validate:"min=0"`
}
```

Remove the `Deleted` field from `UpdateLibraryPayload`:

```go
type UpdateLibraryPayload struct {
	Name                     *string  `json:"name,omitempty" validate:"omitempty,max=100"`
	OrganizeFileStructure    *bool    `json:"organize_file_structure,omitempty"`
	CoverAspectRatio         *string  `json:"cover_aspect_ratio,omitempty" validate:"omitempty,oneof=book audiobook book_fallback_audiobook audiobook_fallback_book"`
	DownloadFormatPreference *string  `json:"download_format_preference,omitempty" validate:"omitempty,oneof=original kepub ask"`
	LibraryPaths             []string `json:"library_paths,omitempty" validate:"omitempty,min=1,max=50,dive"`
}
```

- [ ] **Step 6: Build and verify no remaining references to removed symbols**

Run:

```bash
go build ./...
grep -rn "DeletedAt\|IncludeDeleted\|params\.Deleted" pkg/
```

Expected: `go build` succeeds. `grep` returns no matches inside `pkg/libraries/`, `pkg/models/library.go`, or any caller. (Matches inside unrelated models like `Series.DeletedAt` are fine — series still has soft-delete; this task only touches Library.)

- [ ] **Step 7: Run full Go test suite**

Run: `mise test`

Expected: all tests pass. Existing library tests (none yet) and callers that use `ListLibrariesOptions` compile and pass.

- [ ] **Step 8: Commit**

```bash
git add pkg/migrations/20260423000000_drop_libraries_deleted_at.go pkg/models/library.go pkg/libraries/service.go pkg/libraries/handlers.go pkg/libraries/validators.go
git commit -m "[Fix] Remove unused deleted_at soft-delete from libraries"
```

---

## Task 2: Write failing service test for DeleteLibrary — cascades + FTS purge + job cancellation

**Files:**
- Create: `pkg/libraries/service_test.go`

This task establishes the red side of TDD. One test file, multiple test cases, all failing because `DeleteLibrary` doesn't exist yet. The tests exercise the full contract: row removal, CASCADE coverage, FTS purge, active-job cancellation, not-found error, and transactional atomicity.

- [ ] **Step 1: Write the test file**

Create `pkg/libraries/service_test.go`:

```go
package libraries

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func newTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() { db.Close() })

	return db
}

// seedLibraryWithContent creates a library with one book, one file, one series,
// one person, one genre, one tag, and corresponding FTS entries. Returns the
// library ID and the seeded entity IDs for assertions.
type seededIDs struct {
	LibraryID int
	BookID    int
	FileID    int
	SeriesID  int
	PersonID  int
	GenreID   int
	TagID     int
}

func seedLibraryWithContent(t *testing.T, ctx context.Context, db *bun.DB, name string) seededIDs {
	t.Helper()

	library := &models.Library{
		Name:                     name,
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Returning("*").Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID: library.ID,
		Title:     "Seeded Book",
		Filepath:  "/tmp/seeded",
	}
	_, err = db.NewInsert().Model(book).Returning("*").Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/seeded/book.epub",
		FileType:  "epub",
	}
	_, err = db.NewInsert().Model(file).Returning("*").Exec(ctx)
	require.NoError(t, err)

	series := &models.Series{
		LibraryID: library.ID,
		Name:      "Seeded Series",
	}
	_, err = db.NewInsert().Model(series).Returning("*").Exec(ctx)
	require.NoError(t, err)

	person := &models.Person{
		LibraryID: library.ID,
		Name:      "Seeded Person",
		SortName:  "Person, Seeded",
	}
	_, err = db.NewInsert().Model(person).Returning("*").Exec(ctx)
	require.NoError(t, err)

	genre := &models.Genre{
		LibraryID: library.ID,
		Name:      "Seeded Genre",
	}
	_, err = db.NewInsert().Model(genre).Returning("*").Exec(ctx)
	require.NoError(t, err)

	tag := &models.Tag{
		LibraryID: library.ID,
		Name:      "Seeded Tag",
	}
	_, err = db.NewInsert().Model(tag).Returning("*").Exec(ctx)
	require.NoError(t, err)

	// Seed minimal FTS rows directly (Index* methods have relation loading
	// requirements that are overkill for this test's needs).
	_, err = db.ExecContext(ctx,
		`INSERT INTO books_fts (book_id, library_id, title, filepath, subtitle, authors, filenames, narrators, series_names)
		 VALUES (?, ?, ?, ?, '', '', '', '', '')`,
		book.ID, library.ID, book.Title, book.Filepath)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO series_fts (series_id, library_id, name, description, book_titles, book_authors)
		 VALUES (?, ?, ?, '', '', '')`,
		series.ID, library.ID, series.Name)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO persons_fts (person_id, library_id, name, sort_name) VALUES (?, ?, ?, ?)`,
		person.ID, library.ID, person.Name, person.SortName)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO genres_fts (genre_id, library_id, name) VALUES (?, ?, ?)`,
		genre.ID, library.ID, genre.Name)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO tags_fts (tag_id, library_id, name) VALUES (?, ?, ?)`,
		tag.ID, library.ID, tag.Name)
	require.NoError(t, err)

	return seededIDs{
		LibraryID: library.ID,
		BookID:    book.ID,
		FileID:    file.ID,
		SeriesID:  series.ID,
		PersonID:  person.ID,
		GenreID:   genre.ID,
		TagID:     tag.ID,
	}
}

func TestDeleteLibrary_RemovesRowAndCascades(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	seeded := seedLibraryWithContent(t, ctx, db, "Test Library")

	err := svc.DeleteLibrary(ctx, seeded.LibraryID)
	require.NoError(t, err)

	// Library row removed.
	libCount, err := db.NewSelect().Model((*models.Library)(nil)).Where("id = ?", seeded.LibraryID).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, libCount, "library row should be gone")

	// CASCADE removed children.
	for name, count := range map[string]func() (int, error){
		"books":   func() (int, error) { return db.NewSelect().Model((*models.Book)(nil)).Where("library_id = ?", seeded.LibraryID).Count(ctx) },
		"files":   func() (int, error) { return db.NewSelect().Model((*models.File)(nil)).Where("library_id = ?", seeded.LibraryID).Count(ctx) },
		"series":  func() (int, error) { return db.NewSelect().Model((*models.Series)(nil)).Where("library_id = ?", seeded.LibraryID).Count(ctx) },
		"persons": func() (int, error) { return db.NewSelect().Model((*models.Person)(nil)).Where("library_id = ?", seeded.LibraryID).Count(ctx) },
		"genres":  func() (int, error) { return db.NewSelect().Model((*models.Genre)(nil)).Where("library_id = ?", seeded.LibraryID).Count(ctx) },
		"tags":    func() (int, error) { return db.NewSelect().Model((*models.Tag)(nil)).Where("library_id = ?", seeded.LibraryID).Count(ctx) },
	} {
		n, err := count()
		require.NoError(t, err, name)
		assert.Zero(t, n, "%s rows should be cascaded", name)
	}
}

func TestDeleteLibrary_PurgesFTS(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	seeded := seedLibraryWithContent(t, ctx, db, "FTS Library")

	err := svc.DeleteLibrary(ctx, seeded.LibraryID)
	require.NoError(t, err)

	for _, table := range []string{"books_fts", "series_fts", "persons_fts", "genres_fts", "tags_fts"} {
		var count int
		err := db.NewSelect().TableExpr(table).ColumnExpr("COUNT(*)").Where("library_id = ?", seeded.LibraryID).Scan(ctx, &count)
		require.NoError(t, err, table)
		assert.Zero(t, count, "%s rows for deleted library should be purged", table)
	}
}

func TestDeleteLibrary_CancelsActiveJobs(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Target library.
	lib := &models.Library{Name: "Target", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	_, err := db.NewInsert().Model(lib).Returning("*").Exec(ctx)
	require.NoError(t, err)

	// Second library whose jobs should not be touched.
	other := &models.Library{Name: "Other", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	_, err = db.NewInsert().Model(other).Returning("*").Exec(ctx)
	require.NoError(t, err)

	insertJob := func(status string, libraryID *int) int {
		j := &models.Job{
			Type:      models.JobTypeScan,
			Status:    status,
			LibraryID: libraryID,
			Data:      "{}",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_, err := db.NewInsert().Model(j).Returning("*").Exec(ctx)
		require.NoError(t, err)
		return j.ID
	}

	pendingID := insertJob(models.JobStatusPending, &lib.ID)
	runningID := insertJob(models.JobStatusInProgress, &lib.ID)
	completedID := insertJob(models.JobStatusCompleted, &lib.ID)
	otherLibraryJobID := insertJob(models.JobStatusPending, &other.ID)
	globalJobID := insertJob(models.JobStatusPending, nil)

	err = svc.DeleteLibrary(ctx, lib.ID)
	require.NoError(t, err)

	loadStatus := func(id int) string {
		var j models.Job
		err := db.NewSelect().Model(&j).Where("id = ?", id).Scan(ctx)
		require.NoError(t, err)
		return j.Status
	}

	assert.Equal(t, models.JobStatusFailed, loadStatus(pendingID), "pending job for deleted library should be failed")
	assert.Equal(t, models.JobStatusFailed, loadStatus(runningID), "running job for deleted library should be failed")
	assert.Equal(t, models.JobStatusCompleted, loadStatus(completedID), "completed job must not change")
	assert.Equal(t, models.JobStatusPending, loadStatus(otherLibraryJobID), "other library's job must not change")
	assert.Equal(t, models.JobStatusPending, loadStatus(globalJobID), "global job (no library_id) must not change")
}

func TestDeleteLibrary_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	err := svc.DeleteLibrary(ctx, 99999)
	require.Error(t, err)

	var codeErr *errcodes.Error
	require.True(t, errors.As(err, &codeErr), "error must wrap errcodes.Error, got %T: %v", err, err)
	assert.Equal(t, "not_found", codeErr.Code)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/libraries/... -run TestDeleteLibrary -v`

Expected: compilation failure — `svc.DeleteLibrary undefined`. This is the Red step.

- [ ] **Step 3: Commit the failing tests**

```bash
git add pkg/libraries/service_test.go
git commit -m "[Backend] Add failing tests for DeleteLibrary service"
```

---

## Task 3: Implement `Service.DeleteLibrary`

**Files:**
- Modify: `pkg/libraries/service.go` — append a new method at the end.

Implements the service described in the spec's Architecture → Service section. Single transaction; the order is fixed because FTS purge needs primary IDs to still exist and job cancellation benefits from `library_id` still being populated for audit.

- [ ] **Step 1: Write the implementation**

Append to `pkg/libraries/service.go`:

```go
// DeleteLibrary hard-deletes a library and all of its DB-resident content.
// Files on disk are not touched. The operation runs in a single transaction:
//
//  1. Cancel any pending/in-progress jobs scoped to this library.
//  2. Purge FTS rows (books_fts, series_fts, persons_fts, genres_fts, tags_fts)
//     for this library. FTS purge must happen before the CASCADE so rows are
//     still resolvable.
//  3. Delete the library row; SQLite cascades the rest.
//
// Returns errcodes.NotFound if the library does not exist.
func (svc *Service) DeleteLibrary(ctx context.Context, id int) error {
	return errors.WithStack(svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Verify the library exists so we can return NotFound rather than
		// silently succeeding on a non-existent ID.
		exists, err := tx.NewSelect().Model((*models.Library)(nil)).Where("id = ?", id).Exists(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		if !exists {
			return errcodes.NotFound("Library")
		}

		// 1. Cancel active jobs. jobs.library_id is ON DELETE SET NULL, so rows
		//    survive the cascade; we still update them here so the audit trail
		//    shows they were cancelled as part of the delete.
		_, err = tx.NewUpdate().
			Model((*models.Job)(nil)).
			Set("status = ?", models.JobStatusFailed).
			Set("updated_at = ?", time.Now()).
			Where("library_id = ?", id).
			Where("status IN (?)", bun.In([]string{models.JobStatusPending, models.JobStatusInProgress})).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// 2. Purge FTS rows. Each FTS table carries library_id directly, so
		//    we can delete by that filter without first collecting child IDs.
		for _, table := range []string{"books_fts", "series_fts", "persons_fts", "genres_fts", "tags_fts"} {
			_, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE library_id = ?", id)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		// 3. Delete the library. ON DELETE CASCADE handles children.
		_, err = tx.NewDelete().Model((*models.Library)(nil)).Where("id = ?", id).Exec(ctx)
		return errors.WithStack(err)
	}))
}
```

Add the missing imports to the top of `pkg/libraries/service.go`. The file already imports `context`, `database/sql`, `time`, `errors`, `errcodes`, `models`, and `bun`. No new imports are required.

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./pkg/libraries/... -run TestDeleteLibrary -v`

Expected: all four tests pass (`TestDeleteLibrary_RemovesRowAndCascades`, `TestDeleteLibrary_PurgesFTS`, `TestDeleteLibrary_CancelsActiveJobs`, `TestDeleteLibrary_NotFound`).

- [ ] **Step 3: Commit**

```bash
git add pkg/libraries/service.go
git commit -m "[Backend] Add DeleteLibrary service method"
```

---

## Task 4: Write failing handler tests

**Files:**
- Create: `pkg/libraries/handlers_test.go`

Covers permission, library-access, not-found, and the happy path. Permission enforcement lives in middleware (not in the handler body), so the handler test mirrors the production wiring by registering the DELETE route on an Echo instance with the real auth middleware.

- [ ] **Step 1: Write the test file**

Create `pkg/libraries/handlers_test.go`:

```go
package libraries

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// newDeleteTestServer wires up the libraries DELETE route against a real DB
// and a stubbed auth middleware that injects the provided user into the
// request context. Returns the Echo instance plus a counter that increments
// each time onLibraryChanged fires.
func newDeleteTestServer(t *testing.T, db *bun.DB, user *models.User) (*echo.Echo, *int) {
	t.Helper()

	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle

	// Inject user context middleware that mirrors the real auth middleware.
	stubAuth := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if user != nil {
				c.Set("user", user)
				c.Set("user_id", user.ID)
			}
			return next(c)
		}
	}

	authMiddleware := auth.NewMiddleware(db, "test-secret")
	callbacks := 0
	g := e.Group("/libraries")
	g.Use(stubAuth)
	RegisterRoutesWithGroup(g, db, authMiddleware, RegisterRoutesOptions{
		OnLibraryChanged: func() { callbacks++ },
	})
	return e, &callbacks
}

func seedUser(t *testing.T, ctx context.Context, db *bun.DB, roleName string, allAccess bool) *models.User {
	t.Helper()

	role := &models.Role{}
	err := db.NewSelect().Model(role).Where("name = ?", roleName).Scan(ctx)
	require.NoError(t, err)

	u := &models.User{
		Username:         roleName + "-user",
		PasswordHash:     "unused",
		RoleID:           role.ID,
		AllLibraryAccess: allAccess,
		Role:             role,
	}
	_, err = db.NewInsert().Model(u).Returning("*").Exec(ctx)
	require.NoError(t, err)

	// Load permissions so user.HasPermission works inside middleware.
	err = db.NewSelect().Model(u).Relation("Role").Relation("Role.Permissions").Where("u.id = ?", u.ID).Scan(ctx)
	require.NoError(t, err)

	return u
}

func TestDeleteLibraryHandler_HappyPath(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	admin := seedUser(t, ctx, db, models.RoleAdmin, true)
	e, callbacks := newDeleteTestServer(t, db, admin)

	seeded := seedLibraryWithContent(t, ctx, db, "Doomed")

	req := httptest.NewRequest(http.MethodDelete, "/libraries/"+strconv.Itoa(seeded.LibraryID), nil)
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)

	count, err := db.NewSelect().Model((*models.Library)(nil)).Where("id = ?", seeded.LibraryID).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, count, "library should be gone")

	assert.Equal(t, 1, *callbacks, "onLibraryChanged should fire once on success")
}

func TestDeleteLibraryHandler_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	admin := seedUser(t, ctx, db, models.RoleAdmin, true)
	e, _ := newDeleteTestServer(t, db, admin)

	req := httptest.NewRequest(http.MethodDelete, "/libraries/99999", nil)
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestDeleteLibraryHandler_RequiresWritePermission(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	viewer := seedUser(t, ctx, db, models.RoleViewer, true)
	e, callbacks := newDeleteTestServer(t, db, viewer)

	seeded := seedLibraryWithContent(t, ctx, db, "Protected")

	req := httptest.NewRequest(http.MethodDelete, "/libraries/"+strconv.Itoa(seeded.LibraryID), nil)
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)

	count, err := db.NewSelect().Model((*models.Library)(nil)).Where("id = ?", seeded.LibraryID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "library must survive")

	assert.Zero(t, *callbacks, "onLibraryChanged must not fire on 403")
}

func TestDeleteLibraryHandler_RequiresLibraryAccess(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	editor := seedUser(t, ctx, db, models.RoleEditor, false) // no library access
	e, _ := newDeleteTestServer(t, db, editor)

	seeded := seedLibraryWithContent(t, ctx, db, "NotYours")

	req := httptest.NewRequest(http.MethodDelete, "/libraries/"+strconv.Itoa(seeded.LibraryID), nil)
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/libraries/... -run TestDeleteLibraryHandler -v`

Expected: 404 on the happy path (no DELETE route registered yet). At least `TestDeleteLibraryHandler_HappyPath` should fail with HTTP 404.

Note: if `seedUser` or helper utility references don't compile, fix those first — the goal is failing tests, not compile errors.

- [ ] **Step 3: Commit the failing tests**

```bash
git add pkg/libraries/handlers_test.go
git commit -m "[Backend] Add failing tests for library delete handler"
```

---

## Task 5: Implement the delete handler and register the route

**Files:**
- Modify: `pkg/libraries/handlers.go` — add `delete` method.
- Modify: `pkg/libraries/routes.go` — register the DELETE route.

- [ ] **Step 1: Add the handler method**

Append to `pkg/libraries/handlers.go` (after the `update` method):

```go
func (h *handler) delete(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Library")
	}

	if err := h.libraryService.DeleteLibrary(ctx, id); err != nil {
		return errors.WithStack(err)
	}

	if h.onLibraryChanged != nil {
		h.onLibraryChanged()
	}

	return c.NoContent(http.StatusNoContent)
}
```

- [ ] **Step 2: Register the DELETE route**

In `pkg/libraries/routes.go`, append after the existing `POST /:id` registration (line 34):

```go
	g.DELETE("/:id", h.delete,
		authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite),
		authMiddleware.RequireLibraryAccess("id"))
```

The final `RegisterRoutesWithGroup` body should look like:

```go
	g.GET("", h.list)
	g.GET("/:id", h.retrieve, authMiddleware.RequireLibraryAccess("id"))
	g.POST("", h.create, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite))
	g.POST("/:id", h.update, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))
	g.DELETE("/:id", h.delete,
		authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite),
		authMiddleware.RequireLibraryAccess("id"))
```

- [ ] **Step 3: Run handler tests**

Run: `go test ./pkg/libraries/... -run TestDeleteLibraryHandler -v`

Expected: all four handler tests pass.

- [ ] **Step 4: Run the full Go test suite**

Run: `mise test`

Expected: all tests pass, including unrelated packages. If any test fails because it previously referenced `DeletedAt` / `IncludeDeleted`, fix the reference inline — the cleanup in Task 1 was supposed to cover this, but a caller may have been missed.

- [ ] **Step 5: Commit**

```bash
git add pkg/libraries/handlers.go pkg/libraries/routes.go
git commit -m "[Backend] Add DELETE /libraries/:id endpoint"
```

---

## Task 6: Regenerate frontend types

**Files:**
- Modify: `app/types/generated/*` (auto-generated; do not hand-edit)

Running tygo picks up the removal of `DeletedAt` from the Go model and the removal of `Deleted` from the update payload, so the TypeScript `Library` and `UpdateLibraryPayload` types stay in sync.

- [ ] **Step 1: Run tygo**

Run: `mise tygo`

Expected: either "skipping, outputs are up-to-date" (which is normal per root `CLAUDE.md` — Air may have run it already) or a brief regeneration. The `app/types/generated/` directory is gitignored, so there is nothing to commit from this step.

- [ ] **Step 2: Verify types compile**

Run: `pnpm lint:types`

Expected: no TypeScript errors.

If `pnpm lint:types` surfaces errors, they are almost certainly in handwritten files that referenced the removed `deleted_at` or `deleted` fields — fix those callsites before proceeding.

---

## Task 7: Add `deleteLibrary` to the API client and `useDeleteLibrary` hook

**Files:**
- Modify: `app/libraries/api.ts`
- Modify: `app/hooks/queries/libraries.ts`

- [ ] **Step 1: Add the API method**

Add the following method inside the `ShishoAPI` class in `app/libraries/api.ts` (beside the other domain-specific methods). If the class has no other library methods (the current file only has API-key helpers), place it after `clearKoboSync`:

```ts
  deleteLibrary(id: number): Promise<void> {
    return this.request("DELETE", `/libraries/${id}`);
  }
```

Note: the codebase pattern is to route other library calls through the generic `API.request(...)` directly from the hook file. Either approach works — if you prefer consistency with the existing `useLibrary` / `useUpdateLibrary` hooks, skip this step and call `API.request("DELETE", ...)` directly from the hook instead. Whichever approach is used must match the existing convention in `libraries.ts`.

Decision for this plan: **skip the API class method**, call `API.request` directly from the hook to match the existing hook style.

- [ ] **Step 2: Add the `useDeleteLibrary` hook**

In `app/hooks/queries/libraries.ts`, import the books query key at the top alongside the existing imports:

```ts
import { QueryKey as BooksQueryKey } from "./books";
```

(If the import already exists for another reason, reuse it.)

Append the new hook at the bottom of the file:

```ts
export const useDeleteLibrary = () => {
  const queryClient = useQueryClient();

  return useMutation<void, ShishoAPIError, { id: number }>({
    mutationFn: ({ id }) => {
      return API.request("DELETE", `/libraries/${id}`);
    },
    onSuccess: (_data, { id }) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLibraries] });
      queryClient.removeQueries({ queryKey: [QueryKey.RetrieveLibrary, String(id)] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};
```

Note the deliberate use of `removeQueries` (not `invalidateQueries`) for the single-library detail — the library is gone, so any cached `useLibrary` call should drop its data rather than refetch a now-404 endpoint.

- [ ] **Step 3: Verify the types compile**

Run: `pnpm lint:types`

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add app/hooks/queries/libraries.ts
git commit -m "[Frontend] Add useDeleteLibrary mutation hook"
```

---

## Task 8: Create `DeleteLibraryDialog` component and its tests

**Files:**
- Create: `app/components/library/DeleteLibraryDialog.tsx`
- Create: `app/components/library/DeleteLibraryDialog.test.tsx`

Dedicated component — not a variant of `DeleteConfirmationDialog` (which is book/file-specific). Type-to-confirm UX matches the destructive-action convention in the spec.

- [ ] **Step 1: Write the failing component tests**

Create `app/components/library/DeleteLibraryDialog.test.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import { DeleteLibraryDialog } from "./DeleteLibraryDialog";

const mockDelete = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/queries/libraries", () => ({
  useDeleteLibrary: () => ({
    mutateAsync: mockDelete,
    isPending: false,
  }),
}));

const renderDialog = (props: Partial<React.ComponentProps<typeof DeleteLibraryDialog>> = {}) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <DeleteLibraryDialog
          library={{ id: 1, name: "My Library" }}
          onOpenChange={onOpenChange}
          open={true}
          {...props}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { onOpenChange };
};

describe("DeleteLibraryDialog", () => {
  it("disables the Delete button until the user types the exact library name", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDialog();

    const deleteButton = screen.getByRole("button", { name: /^Delete$/ });
    expect(deleteButton).toBeDisabled();

    const input = screen.getByLabelText(/Type the library name to confirm/i);
    await user.type(input, "my library"); // wrong case
    expect(deleteButton).toBeDisabled();

    await user.clear(input);
    await user.type(input, "My Library");
    expect(deleteButton).toBeEnabled();
  });

  it("calls the delete mutation with the library id on confirm", async () => {
    mockDelete.mockResolvedValueOnce(undefined);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDialog();

    await user.type(screen.getByLabelText(/Type the library name to confirm/i), "My Library");
    await user.click(screen.getByRole("button", { name: /^Delete$/ }));

    expect(mockDelete).toHaveBeenCalledWith({ id: 1 });
  });

  it("surfaces the three caveats in the warning banner", () => {
    renderDialog();

    expect(screen.getByText(/irreversible/i)).toBeInTheDocument();
    expect(screen.getByText(/Files on disk will not be deleted/i)).toBeInTheDocument();
    expect(screen.getByText(/Sidecar and metadata files will not be cleaned up/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm vitest run app/components/library/DeleteLibraryDialog.test.tsx`

Expected: failure — module `./DeleteLibraryDialog` does not exist.

- [ ] **Step 3: Implement the component**

Create `app/components/library/DeleteLibraryDialog.tsx`:

```tsx
import { AlertTriangle, Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useDeleteLibrary } from "@/hooks/queries/libraries";

interface DeleteLibraryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  library: { id: number; name: string };
}

export function DeleteLibraryDialog({
  open,
  onOpenChange,
  library,
}: DeleteLibraryDialogProps) {
  const [typedName, setTypedName] = useState("");
  const deleteMutation = useDeleteLibrary();
  const navigate = useNavigate();

  // Reset the typed value whenever the dialog reopens.
  useEffect(() => {
    if (open) setTypedName("");
  }, [open]);

  const canDelete = typedName === library.name && !deleteMutation.isPending;

  const handleConfirm = async () => {
    try {
      await deleteMutation.mutateAsync({ id: library.id });
      toast.success(`Library "${library.name}" deleted.`);
      onOpenChange(false);
      navigate("/settings/libraries");
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Something went wrong.";
      toast.error(msg);
    }
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md overflow-x-hidden">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-destructive shrink-0" />
            Delete library
          </DialogTitle>
          <DialogDescription className="sr-only">
            Permanently delete this library from the database.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 min-w-0">
          <div className="bg-destructive/10 border border-destructive/20 rounded-md p-3 text-sm text-destructive break-words space-y-2">
            <p>This action is irreversible.</p>
            <p>Files on disk will not be deleted.</p>
            <p>
              Sidecar and metadata files will not be cleaned up. You&apos;ll
              need to remove them manually if desired.
            </p>
          </div>

          <p className="text-sm">
            Are you sure you want to delete{" "}
            <span className="font-medium">&ldquo;{library.name}&rdquo;</span>?
          </p>

          <div className="space-y-2">
            <Label htmlFor="delete-library-confirm">
              Type the library name to confirm
            </Label>
            <Input
              autoComplete="off"
              id="delete-library-confirm"
              onChange={(e) => setTypedName(e.target.value)}
              placeholder={library.name}
              value={typedName}
            />
          </div>
        </div>

        <DialogFooter>
          <Button
            disabled={deleteMutation.isPending}
            onClick={() => onOpenChange(false)}
            variant="outline"
          >
            Cancel
          </Button>
          <Button
            disabled={!canDelete}
            onClick={handleConfirm}
            variant="destructive"
          >
            {deleteMutation.isPending && (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            )}
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm vitest run app/components/library/DeleteLibraryDialog.test.tsx`

Expected: all three tests pass.

- [ ] **Step 5: Commit**

```bash
git add app/components/library/DeleteLibraryDialog.tsx app/components/library/DeleteLibraryDialog.test.tsx
git commit -m "[Frontend] Add DeleteLibraryDialog component"
```

---

## Task 9: Wire the Danger Zone into `LibrarySettings`

**Files:**
- Modify: `app/components/pages/LibrarySettings.tsx`

- [ ] **Step 1: Add the Danger Zone section**

In `app/components/pages/LibrarySettings.tsx`:

Add the imports at the top of the file:

```tsx
import { DeleteLibraryDialog } from "@/components/library/DeleteLibraryDialog";
import { useAuth } from "@/hooks/useAuth";
```

Add state for the dialog near the other `useState` declarations (around line 53):

```tsx
const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
```

Pull `hasPermission` from `useAuth`. Add near the top of the component body:

```tsx
const { hasPermission } = useAuth();
const canDeleteLibrary = hasPermission("libraries", "write");
```

At the end of the `return (...)` JSX, after the existing `<div className="max-w-2xl space-y-6 border border-border rounded-md p-6">` wrapper closes but before `<UnsavedChangesDialog ...>`, insert:

```tsx
{canDeleteLibrary && libraryQuery.data && (
  <section className="max-w-2xl mt-8 border border-destructive/40 rounded-md p-4 md:p-6">
    <h2 className="text-base md:text-lg font-semibold text-destructive mb-1">
      Danger Zone
    </h2>
    <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mt-2">
      <div className="min-w-0">
        <p className="font-medium">Delete this library</p>
        <p className="text-sm text-muted-foreground">
          Permanently removes this library and all of its books, files, and
          metadata from the database. Files on disk are not touched.
        </p>
      </div>
      <Button
        className="shrink-0"
        onClick={() => setDeleteDialogOpen(true)}
        size="sm"
        variant="destructive"
      >
        <Trash2 className="h-4 w-4 sm:mr-2" />
        <span className="hidden sm:inline">Delete library</span>
      </Button>
    </div>
  </section>
)}

{libraryQuery.data && (
  <DeleteLibraryDialog
    library={{ id: libraryQuery.data.id, name: libraryQuery.data.name }}
    onOpenChange={setDeleteDialogOpen}
    open={deleteDialogOpen}
  />
)}
```

`Trash2` is already imported in this file (it's used on per-path remove buttons). No new lucide imports needed.

- [ ] **Step 2: Manually verify in the dev server**

Start the dev server: `mise start`

In a browser:
1. Log in as an admin user, navigate to a library's settings page.
2. Confirm the Danger Zone appears at the bottom with a red border.
3. Click **Delete library**. Confirm the dialog opens and the Delete button is disabled.
4. Type a wrong value — button stays disabled. Type the exact library name — button enables.
5. Click **Delete**. Confirm success toast, redirect to `/settings/libraries`, deleted library is absent from the list.
6. Open `tmp/library/` on disk and confirm all original files are still there.
7. Log out, log in as a Viewer, navigate to a library's settings page. Confirm the Danger Zone does not appear.

Report back what you observed in each step.

- [ ] **Step 3: Run all JS checks**

Run: `mise lint:js`

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add app/components/pages/LibrarySettings.tsx
git commit -m "[Frontend] Add Danger Zone delete action to library settings"
```

---

## Task 10: Update documentation

**Files:**
- Create: `website/docs/libraries.md`
- Modify: `website/docs/getting-started.md:44`

The spec's Docs section specifies a new dedicated `libraries.md` page at `sidebar_position: 15` (between getting-started at 10 and directory-structure at 20).

- [ ] **Step 1: Create the new doc page**

Create `website/docs/libraries.md`:

```markdown
---
sidebar_position: 15
---

# Libraries

Shisho organizes your collection into **libraries**, each pointing at one or more directories on disk. This page covers how to create, configure, and delete a library.

## Creating a Library

Go to **Admin → Libraries → Add Library**. Choose a name, pick a cover aspect ratio (book, audiobook, or a fallback mode), and add one or more directory paths. Saving the library kicks off a scan of the configured paths automatically.

## Library Settings

Each library has its own settings page reachable from **Admin → Libraries → Settings** on the corresponding row. Settings cover:

- **Library name** and **paths** — rename or add/remove scanned directories.
- **Cover display aspect ratio** — how book and series covers render in gallery views.
- **Download format preference** — original / KePub / Ask-on-download for EPUB and CBZ files.
- **Organize file structure during scans** — when enabled, Shisho moves and renames files into a standardized layout. See [Directory Structure](./directory-structure.md) for the naming rules and triggering events.
- **Plugin order** — override the global plugin order for this library.

## Deleting a Library

At the bottom of the library settings page, users with `libraries:write` permission (Admin and Editor roles by default) see a **Danger Zone** section with a **Delete library** button.

Click the button, type the library name to confirm, and click **Delete**. The confirmation dialog surfaces these caveats:

- **The action is irreversible.** There is no undo.
- **Files on disk are not deleted.** Book files, audiobooks, comics, and PDFs remain exactly where they are.
- **Sidecar and metadata files are not cleaned up.** `.shisho.json` sidecars, `.cover.jpg` images, and other generated metadata remain on disk. Remove them manually if you want a truly clean slate.

### What is deleted

- The library row itself.
- All books, files, series, persons (authors and narrators), genres, tags, publishers, and imprints scoped to the library.
- All file identifiers and chapters for those files.
- Per-library plugin configuration, hook configuration, and field settings.
- Per-user library access grants and per-user library settings (sort spec, etc.).
- Full-text search entries for the above so searches no longer surface stale results.

### What happens to active jobs

Any pending or in-progress scan, hash, or plugin job scoped to the deleted library is cancelled (marked `failed`) as part of the deletion. Global jobs and jobs targeting other libraries are untouched.

See [Users and Permissions](./users-and-permissions.md) for how to grant or revoke `libraries:write`.
```

- [ ] **Step 2: Update the cross-link in getting-started.md**

In `website/docs/getting-started.md`, replace line 44:

Before:
```markdown
Access Shisho at `http://localhost:5173` and create a library pointing to `/media` (or wherever you mounted your books).
```

After:
```markdown
Access Shisho at `http://localhost:5173` and [create a library](./libraries.md) pointing to `/media` (or wherever you mounted your books).
```

- [ ] **Step 3: Verify the docs build**

Run: `mise docs`

Navigate to `http://localhost:3000/shisho/docs/libraries` (or whatever URL the dev server reports) and confirm the page renders, the sidebar shows "Libraries" between "Getting Started" and "Directory Structure", and the cross-link from Getting Started resolves.

Stop the dev server when done.

- [ ] **Step 4: Commit**

```bash
git add website/docs/libraries.md website/docs/getting-started.md
git commit -m "[Docs] Document library deletion and add Libraries page"
```

---

## Task 11: Final verification

- [ ] **Step 1: Run the full project check suite**

Run: `mise check:quiet`

Expected: the pass/fail summary reports all checks passing. This runs Go tests, Go lint, and all JS lint/tests in parallel. Per root `CLAUDE.md`, run this once — if it fails, use `mise check` for verbose output.

- [ ] **Step 2: Run the full race-detection suite**

Run: `mise test:race`

Expected: passes cleanly. Concurrent DeleteLibrary calls are unlikely in practice but the transactional implementation should be race-clean.

- [ ] **Step 3: End-to-end smoke test in the browser**

Not automated (this feature has no E2E coverage planned; the unit + handler + component tests cover the surface). Perform the manual steps listed in Task 9 Step 2 once more against the final committed code.

---

## Self-Review

**Spec coverage:**

| Spec section | Covered by |
|---|---|
| Delete library endpoint, `libraries:write`-gated | Tasks 4, 5 |
| CASCADE coverage (books, files, series, persons, etc.) | Task 2 (TestDeleteLibrary_RemovesRowAndCascades) |
| FTS purge (all five tables) | Tasks 2, 3 |
| Job cancellation | Tasks 2, 3 |
| Monitor refresh via OnLibraryChanged | Tasks 4, 5 |
| No files on disk deleted | Implicit (no filesystem code touched); manual verification in Task 9 |
| Remove `deleted_at` plumbing | Task 1 |
| Danger Zone UI gated by permission | Task 9 |
| Type-to-confirm dialog | Task 8 |
| Three caveats in dialog copy | Task 8 (test `surfaces the three caveats`) |
| Invalidate affected TanStack Query caches | Task 7 |
| Docs: new `libraries.md` page with delete section | Task 10 |
| Docs: sidebar position 15 | Task 10 Step 1 frontmatter |
| Docs: update getting-started cross-link | Task 10 Step 2 |

**Placeholder scan:** No `TBD` / `TODO` / "add appropriate X" language. Every code-bearing step contains literal code. Every test references functions or components that are defined in an earlier step in this plan.

**Type consistency check:** `DeleteLibraryDialog` is referred to by that exact name in Tasks 8, 9, and the self-review. The mutation hook is `useDeleteLibrary` in all three tasks (7, 8, 9). Service method is `DeleteLibrary` in all backend tasks (2, 3, 4, 5). The `RegisterRoutesOptions.OnLibraryChanged` callback name matches its usage in Task 4's test helper and the existing `pkg/server/server.go:152-154`.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-23-delete-library.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
