# Series Hard-Delete Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert `models.Series` from soft-delete to hard-delete by dropping the `deleted_at` column, the `bun:",soft_delete"` tag, the unused `RestoreSeries` function, and the surrounding hand-written `deleted_at IS NULL` filters.

**Architecture:** Single SQLite migration drops the column and rebuilds the unique index unconditional. Removing the `bun:",soft_delete"` tag from `models.Series` flips every existing `NewDelete()` call from a soft-delete UPDATE to a real DELETE — Bun's behavior is entirely controlled by that tag, so call sites do not change. Pre-existing soft-deleted rows are hard-deleted by the migration; CASCADE on `book_series.series_id` (set in `20260406100000_add_fk_cascades.go`) cleans their join rows. A regression test pins the new behavior.

**Tech Stack:** Go 1.23, Bun ORM, SQLite, mise for task running, testify for assertions.

**Spec:** `docs/superpowers/specs/2026-04-26-series-hard-delete-design.md`

---

## File Structure

**Created:**
- `pkg/migrations/20260426000000_drop_series_deleted_at.go` — migration that hard-deletes soft-deleted rows, drops the column, and rebuilds the unique index.

**Modified:**
- `pkg/models/series.go` — remove `DeletedAt` field (drops the soft-delete tag).
- `pkg/series/service.go` — simplify `DeleteSeries`, delete `RestoreSeries`, fix doc comments on `MergeSeries` and `CleanupOrphanedSeries`.
- `pkg/series/service_test.go` — add `TestDeleteSeries_HardDeletesRowAndFTS` regression test.
- `pkg/books/service.go` — fix doc comment on duplicate `CleanupOrphanedSeries`.
- `pkg/search/service.go` — drop `s.deleted_at IS NULL` filters from books-FTS and series-FTS rebuild SQL.

---

## Task 1: Add the migration

**Files:**
- Create: `pkg/migrations/20260426000000_drop_series_deleted_at.go`

- [ ] **Step 1: Create the migration file**

Create `pkg/migrations/20260426000000_drop_series_deleted_at.go` with this exact content:

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Hard-delete any soft-deleted series. CASCADE on book_series.series_id
		// (set in 20260406100000_add_fk_cascades.go) cleans up join rows.
		// Done before the schema change so the unconditional unique index in
		// the final step can't collide on a name shared between a soft-deleted
		// and a live row in the same library.
		if _, err := db.Exec(`DELETE FROM series WHERE deleted_at IS NOT NULL`); err != nil {
			return errors.WithStack(err)
		}
		// Defensive: purge any series_fts rows whose series_id is gone.
		// Handlers already call DeleteFromSeriesIndex on soft-delete today,
		// so this should be a no-op in practice.
		if _, err := db.Exec(`DELETE FROM series_fts WHERE series_id NOT IN (SELECT id FROM series)`); err != nil {
			return errors.WithStack(err)
		}
		// The partial unique index references deleted_at; drop before the column.
		if _, err := db.Exec(`DROP INDEX ux_series_name_library_id`); err != nil {
			return errors.WithStack(err)
		}
		if _, err := db.Exec(`ALTER TABLE series DROP COLUMN deleted_at`); err != nil {
			return errors.WithStack(err)
		}
		if _, err := db.Exec(`CREATE UNIQUE INDEX ux_series_name_library_id ON series (name COLLATE NOCASE, library_id)`); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		// Lossy: does not restore deleted rows. Matches the precedent set by
		// 20260423000000_drop_libraries_deleted_at.go.
		if _, err := db.Exec(`DROP INDEX ux_series_name_library_id`); err != nil {
			return errors.WithStack(err)
		}
		if _, err := db.Exec(`ALTER TABLE series ADD COLUMN deleted_at TIMESTAMPTZ`); err != nil {
			return errors.WithStack(err)
		}
		if _, err := db.Exec(`CREATE UNIQUE INDEX ux_series_name_library_id ON series (name COLLATE NOCASE, library_id) WHERE deleted_at IS NULL`); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
```

- [ ] **Step 2: Run the migration up**

Run: `mise db:migrate`

Expected: completes without error. The dev DB at `tmp/data.sqlite` now lacks the `deleted_at` column on `series`.

- [ ] **Step 3: Verify the down migration works**

Run: `mise db:rollback`

Expected: completes without error. The `deleted_at` column is back, partial unique index restored.

- [ ] **Step 4: Run the migration up again**

Run: `mise db:migrate`

Expected: completes without error. Leaves the DB in the up state for the rest of the work.

- [ ] **Step 5: Commit the migration on its own**

```bash
git add pkg/migrations/20260426000000_drop_series_deleted_at.go
git commit -m "[Backend] Add migration to drop series.deleted_at"
```

---

## Task 2: Write the failing regression test (Red)

**Files:**
- Modify: `pkg/series/service_test.go` — add new test function.

- [ ] **Step 1: Add the regression test**

Append this function at the end of `pkg/series/service_test.go`. It re-uses the `setupSeriesTestDB` helper in `pkg/series/handlers_cover_cache_test.go:29` (same package, no import change needed).

```go
// TestDeleteSeries_HardDeletesRowAndFTS pins the post-soft-delete behavior:
// after DeleteSeries the row is gone from `series` (not just flagged with
// deleted_at), the join rows are gone, and DeleteFromSeriesIndex purges the
// FTS row. Catches accidental reintroduction of the soft_delete tag.
func TestDeleteSeries_HardDeletesRowAndFTS(t *testing.T) {
	t.Parallel()

	db := setupSeriesTestDB(t)
	ctx := context.Background()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceManual,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	s := &models.Series{
		LibraryID:      library.ID,
		Name:           "Test Series",
		NameSource:     models.DataSourceManual,
		SortName:       "Test Series",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(s).Exec(ctx)
	require.NoError(t, err)

	bs := &models.BookSeries{BookID: book.ID, SeriesID: s.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(bs).Exec(ctx)
	require.NoError(t, err)

	// Index the series in FTS so we can assert it's purged after deletion.
	searchSvc := search.NewService(db)
	require.NoError(t, searchSvc.IndexSeries(ctx, s))

	// Sanity: FTS row exists before deletion. series_fts is a virtual FTS5
	// table, so use a raw query (Bun model queries don't work against it).
	var pre int
	require.NoError(t, db.NewRaw("SELECT count(*) FROM series_fts WHERE series_id = ?", s.ID).
		Scan(ctx, &pre))
	require.Equal(t, 1, pre, "series_fts should have a row before deletion")

	// Delete via the service. The service does NOT touch FTS — the handler
	// does. We invoke DeleteFromSeriesIndex explicitly to mirror the handler.
	svc := NewService(db)
	_, err = svc.DeleteSeries(ctx, s.ID)
	require.NoError(t, err)
	require.NoError(t, searchSvc.DeleteFromSeriesIndex(ctx, s.ID))

	// 1. The row is gone from `series` (not just flagged). This is the
	//    assertion that fails on the pre-change tree.
	count, err := db.NewSelect().Model((*models.Series)(nil)).
		Where("id = ?", s.ID).
		Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "series row must be hard-deleted, not soft-deleted")

	// 2. book_series rows for the deleted series are gone (CASCADE).
	bsCount, err := db.NewSelect().Model((*models.BookSeries)(nil)).
		Where("series_id = ?", s.ID).
		Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, bsCount, "book_series rows must be removed")

	// 3. FTS row is gone.
	var post int
	require.NoError(t, db.NewRaw("SELECT count(*) FROM series_fts WHERE series_id = ?", s.ID).
		Scan(ctx, &post))
	assert.Equal(t, 0, post, "series_fts row must be removed after DeleteFromSeriesIndex")
}
```

- [ ] **Step 2: Run the test to verify it FAILS**

Run: `go test ./pkg/series/ -run TestDeleteSeries_HardDeletesRowAndFTS -v`

Expected: FAIL on the assertion `series row must be hard-deleted, not soft-deleted` (count == 1, expected 0). This proves the test is wired correctly and the soft-delete tag is still in effect.

If the test passes here, something is wrong — stop and investigate before proceeding.

---

## Task 3: Drop the `soft_delete` tag from the model (Green)

**Files:**
- Modify: `pkg/models/series.go:21`

- [ ] **Step 1: Remove the `DeletedAt` field**

Open `pkg/models/series.go`. The struct around line 15-32 looks like this:

```go
type Series struct {
	bun.BaseModel `bun:"table:series,alias:s" tstype:"-"`

	ID                 int           `bun:",pk,nullzero" json:"id"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
	DeletedAt          *time.Time    `bun:",soft_delete" json:"-"`
	LibraryID          int           `bun:",nullzero" json:"library_id"`
	...
}
```

Delete the `DeletedAt` line entirely. The struct should now have `UpdatedAt` followed directly by `LibraryID`.

- [ ] **Step 2: Run the regression test to verify it PASSES**

Run: `go test ./pkg/series/ -run TestDeleteSeries_HardDeletesRowAndFTS -v`

Expected: PASS. The `NewDelete()` call inside `DeleteSeries` now translates to a real `DELETE` statement; the row is gone, CASCADE cleans the join, the explicit FTS delete removes the FTS row.

- [ ] **Step 3: Run all series-package tests**

Run: `go test ./pkg/series/ -v`

Expected: all pass, including `TestDeleteSeries_RemovesBookSeriesAndFlipsReviewed` (still passes — its assertions are outside-observable hard-delete behavior).

---

## Task 4: Simplify `DeleteSeries`

**Files:**
- Modify: `pkg/series/service.go:232-266`

The inner `tx.NewDelete().Model((*models.BookSeries)(nil))` is now redundant — `book_series.series_id` already CASCADEs (set in `20260406100000_add_fk_cascades.go:150`). Drop the redundant delete and the surrounding transaction.

- [ ] **Step 1: Replace the function body**

Find this function in `pkg/series/service.go`:

```go
// DeleteSeries soft-deletes a series and hard-deletes its book_series join
// rows so they don't masquerade as live series associations on the affected
// books. Returns the IDs of books that had a join row removed; callers should
// use these to recompute review state.
func (svc *Service) DeleteSeries(ctx context.Context, seriesID int) ([]int, error) {
	var affectedBookIDs []int
	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		err := tx.NewSelect().
			Model((*models.BookSeries)(nil)).
			ColumnExpr("DISTINCT bs.book_id").
			Where("bs.series_id = ?", seriesID).
			Scan(ctx, &affectedBookIDs)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = tx.NewDelete().
			Model((*models.BookSeries)(nil)).
			Where("series_id = ?", seriesID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = tx.NewDelete().
			Model((*models.Series)(nil)).
			Where("id = ?", seriesID).
			Exec(ctx)
		return errors.WithStack(err)
	})
	if err != nil {
		return nil, err
	}
	return affectedBookIDs, nil
}
```

Replace it with:

```go
// DeleteSeries deletes a series. CASCADE on book_series.series_id removes
// the join rows. Returns the IDs of books that had a join row removed;
// callers should use these to recompute review state.
func (svc *Service) DeleteSeries(ctx context.Context, seriesID int) ([]int, error) {
	var affectedBookIDs []int
	err := svc.db.NewSelect().
		Model((*models.BookSeries)(nil)).
		ColumnExpr("DISTINCT bs.book_id").
		Where("bs.series_id = ?", seriesID).
		Scan(ctx, &affectedBookIDs)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	_, err = svc.db.NewDelete().
		Model((*models.Series)(nil)).
		Where("id = ?", seriesID).
		Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return affectedBookIDs, nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./pkg/series/`

Expected: builds cleanly. If `database/sql` or `bun` becomes an unused import after the function shrinks, the compiler will tell you — drop the unused import. (`bun.Tx` was the only `bun` reference inside the function, but the file uses `bun.DB` elsewhere, so the import stays.)

- [ ] **Step 3: Run series-package tests**

Run: `go test ./pkg/series/ -v`

Expected: all pass, including `TestDeleteSeries_RemovesBookSeriesAndFlipsReviewed` (CASCADE replaces the explicit delete) and the new regression test.

---

## Task 5: Remove `RestoreSeries`

**Files:**
- Modify: `pkg/series/service.go:268-281`

`RestoreSeries` has zero callers and is the only consumer of `WhereAllWithDeleted()`. Delete it.

- [ ] **Step 1: Delete the function**

Find this function in `pkg/series/service.go`:

```go
// RestoreSeries restores a soft-deleted series.
func (svc *Service) RestoreSeries(ctx context.Context, seriesID int) error {
	_, err := svc.db.
		NewUpdate().
		Model((*models.Series)(nil)).
		Set("deleted_at = NULL").
		Where("id = ?", seriesID).
		WhereAllWithDeleted().
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
```

Delete it entirely (including the doc comment).

- [ ] **Step 2: Verify the package still builds and there are no callers anywhere**

Run: `go build ./...`

Expected: builds cleanly. If anything in another package was calling `RestoreSeries`, it would surface here. (Exploration confirmed there are none, but this catches any miss.)

- [ ] **Step 3: Run all backend tests**

Run: `go test ./pkg/series/ ./pkg/books/ ./pkg/search/ -v`

Expected: all pass.

---

## Task 6: Drop `deleted_at` filters from FTS rebuild SQL

**Files:**
- Modify: `pkg/search/service.go:578` and `:596`

`s.deleted_at IS NULL` in the rebuild SQL stops working after the column is dropped. Drop both filters.

- [ ] **Step 1: Edit line 578 (books-FTS rebuild)**

In `pkg/search/service.go`, find this line inside the books-FTS rebuild SQL:

```sql
COALESCE((SELECT GROUP_CONCAT(s.name, ' ') FROM book_series bs JOIN series s ON bs.series_id = s.id WHERE bs.book_id = b.id AND s.deleted_at IS NULL), '')
```

Change to:

```sql
COALESCE((SELECT GROUP_CONCAT(s.name, ' ') FROM book_series bs JOIN series s ON bs.series_id = s.id WHERE bs.book_id = b.id), '')
```

(Drop the trailing `AND s.deleted_at IS NULL`.)

- [ ] **Step 2: Edit lines 595-596 (series-FTS rebuild)**

A few lines below, find:

```sql
FROM series s
WHERE s.deleted_at IS NULL
```

Change to:

```sql
FROM series s
```

(Drop the `WHERE s.deleted_at IS NULL` line entirely.)

- [ ] **Step 3: Verify compilation**

Run: `go build ./pkg/search/`

Expected: builds cleanly.

- [ ] **Step 4: Run search-package tests**

Run: `go test ./pkg/search/ -v`

Expected: all pass. The rebuild path is exercised by tests that call `RebuildAllIndexes`; they will fail at runtime against the post-migration schema if the `s.deleted_at IS NULL` references aren't removed.

---

## Task 7: Update doc comments

**Files:**
- Modify: `pkg/series/service.go` — comments on `MergeSeries` and `CleanupOrphanedSeries`.
- Modify: `pkg/books/service.go:1363` — comment on duplicate `CleanupOrphanedSeries`.

Comments still say "soft-deletes" but the behavior is now hard delete. Per the user-feedback memory `feedback_update_comments_scope.md`, comments must reflect current scope.

- [ ] **Step 1: Update `MergeSeries` comment in `pkg/series/service.go:283-285`**

Find:

```go
// MergeSeries merges sourceSeries into targetSeries (moves all books,
// soft-deletes source). Returns the IDs of books whose join rows moved so
// the caller can recompute search indexes that bake in the series name.
```

Change "soft-deletes source" to "deletes source":

```go
// MergeSeries merges sourceSeries into targetSeries (moves all books,
// deletes source). Returns the IDs of books whose join rows moved so
// the caller can recompute search indexes that bake in the series name.
```

- [ ] **Step 2: Update inline comment in `MergeSeries`**

Inside the same function (around line 308), find:

```go
		// Soft-delete the source series
		_, err = tx.NewDelete().
```

Change to:

```go
		// Delete the source series
		_, err = tx.NewDelete().
```

- [ ] **Step 3: Update `CleanupOrphanedSeries` comment in `pkg/series/service.go:321`**

Find:

```go
// CleanupOrphanedSeries soft-deletes series with no books.
```

Change to:

```go
// CleanupOrphanedSeries deletes series with no books.
```

- [ ] **Step 4: Update duplicate `CleanupOrphanedSeries` comment in `pkg/books/service.go:1363`**

Find:

```go
// CleanupOrphanedSeries soft-deletes series with no books.
// This is duplicated from series service to avoid import cycles.
```

Change to:

```go
// CleanupOrphanedSeries deletes series with no books.
// This is duplicated from series service to avoid import cycles.
```

- [ ] **Step 5: Verify compilation**

Run: `go build ./...`

Expected: builds cleanly.

---

## Task 8: Run targeted lint and tests

- [ ] **Step 1: Run Go lint and tests**

Run: `mise lint test`

Expected: all pass. This is the targeted-subset run for a Go-only change per `CLAUDE.md`.

If anything fails, do not proceed — investigate the failure and re-run.

---

## Task 9: Commit the code changes together

Tasks 2–7 together form one logical change: "switch series to hard-delete".

- [ ] **Step 1: Stage and commit**

```bash
git add pkg/models/series.go pkg/series/service.go pkg/series/service_test.go pkg/books/service.go pkg/search/service.go
git commit -m "$(cat <<'EOF'
[Backend] Hard-delete series, drop soft-delete plumbing

models.Series was the only model using bun's soft_delete tag. RestoreSeries
had zero callers and every read path filtered deleted_at IS NULL, so the
recovery mechanism was unreachable. Drop the tag, simplify DeleteSeries
(book_series.series_id already CASCADEs since 20260406100000), remove the
unused RestoreSeries, and drop the s.deleted_at IS NULL filters from the
FTS rebuild SQL.

Adds TestDeleteSeries_HardDeletesRowAndFTS as a regression pin.
EOF
)"
```

- [ ] **Step 2: Verify the commit looks right**

Run: `git log -1 --stat`

Expected: shows the 5 files modified — `pkg/models/series.go`, `pkg/series/service.go`, `pkg/series/service_test.go`, `pkg/books/service.go`, `pkg/search/service.go`.

---

## Task 10: Manual smoke test

Per `CLAUDE.md`, UI/feature changes warrant a quick manual check even when tests pass.

- [ ] **Step 1: Start the dev server**

Run: `mise start`

Expected: API + Vite frontend come up. Watch the API logs to confirm migration `20260426000000_drop_series_deleted_at` runs once with no error on first startup against your dev DB.

- [ ] **Step 2: Log in and exercise series delete**

Open http://localhost:5173, log in with `robin / password123`.

- Navigate to a library that has at least one series. If none, edit a book and add a series first.
- Open the series detail page and delete the series via the UI.
- Confirm the series disappears from the series list.
- Open global search and search for the deleted series name. Confirm zero series hits.

Expected: series gone from list and search.

- [ ] **Step 3: Exercise series merge**

- Create two series in the same library (or pick two existing ones).
- Merge one into the other via the UI.
- Confirm the source disappears, the target keeps all books.

Expected: source gone, target intact.

- [ ] **Step 4: Stop the dev server**

Hit Ctrl-C in the `mise start` terminal.

---

## Task 11: Final pre-PR check

- [ ] **Step 1: Run the full validation suite**

Run: `mise check:quiet`

Expected: passes. This is the once-before-PR gate per `CLAUDE.md`. Concurrent runs from other worktrees serialize via `flock`, so just kick it off and wait.

- [ ] **Step 2: Push the branch**

Run: `git push -u origin task/soft-delete`

Expected: pushes cleanly.

- [ ] **Step 3: Open the PR**

```bash
gh pr create --title "[Backend] Hard-delete series, remove soft-delete plumbing" --body "$(cat <<'EOF'
## Summary

- `models.Series` was the only model in the codebase using Bun's `soft_delete` tag, with `RestoreSeries` having zero callers — the recovery mechanism was unreachable.
- Drop the `deleted_at` column, the partial unique index `WHERE deleted_at IS NULL`, the unused `RestoreSeries` function, and the hand-written `s.deleted_at IS NULL` filters in FTS rebuild SQL.
- Simplify `DeleteSeries`: `book_series.series_id` already CASCADEs (set in `20260406100000_add_fk_cascades.go`), so the explicit join-row delete and surrounding transaction are now redundant.
- Migration hard-deletes any rows currently in the soft-deleted state and rebuilds the unique index unconditional. **Down migration is lossy** — does not restore deleted rows. Same precedent as `20260423000000_drop_libraries_deleted_at.go` two days ago.

## Test plan

- [x] `mise db:rollback && mise db:migrate` round-trip
- [x] New regression test `TestDeleteSeries_HardDeletesRowAndFTS` fails on master, passes on this branch
- [x] `mise lint test`
- [x] `mise check:quiet`
- [x] Manual: delete a series via UI; confirm it's gone from the list and global search
- [x] Manual: merge two series; confirm source is gone, target intact

Spec: `docs/superpowers/specs/2026-04-26-series-hard-delete-design.md`
EOF
)"
```

Expected: PR opens. Return the URL to the user.

---

## Notes for the implementing engineer

- **Don't skip the Red step (Task 2 Step 2).** Watching the test fail before changing the implementation is the only proof that the test actually catches the bug. Per `CLAUDE.md`, this is a hard requirement.
- **Don't add `Restore*` back.** If a future task needs deleted-series recovery, it should be designed deliberately with a UI surface, not by reintroducing a soft-delete tag.
- **`CleanupOrphanedSeries` is duplicated on purpose.** `pkg/series/service.go:321` and `pkg/books/service.go:1363` both exist to break an import cycle. Don't try to deduplicate as part of this PR — it's noted as out-of-scope in the spec.
- **The `users.is_active` "deactivation" pattern is unrelated.** It's a different mechanism, actively used, and explicitly out of scope.
- **No frontend, OPDS, eReader, plugin SDK, sidecar, or website docs to touch.** Exploration confirmed none of these reference soft-delete on series. Don't go looking for changes there.
