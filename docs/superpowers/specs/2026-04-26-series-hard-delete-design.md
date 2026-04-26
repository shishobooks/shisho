# Series Hard-Delete — Design

## Problem

`models.Series` is the only model in the codebase using Bun's `soft_delete` tag. The infrastructure built around it — a `deleted_at` column, a partial unique index `WHERE deleted_at IS NULL`, hand-written `s.deleted_at IS NULL` filters in FTS rebuild SQL, a `RestoreSeries` function, manual `book_series` cleanup in `DeleteSeries` — exists for one model and leaks complexity across the search, books, and series packages.

The soft-delete behavior is also write-only: `RestoreSeries` has zero callers (no HTTP handler, no frontend, no CLI, no test), and the only `WhereAllWithDeleted()` call sits inside `RestoreSeries` itself. Once a series is "soft-deleted", every read path — listing, search, FTS rebuild, Bun relation walks — filters it out, with no path back. We're paying for a recovery mechanism nobody can invoke.

A precedent for ripping out unused soft-delete plumbing already exists: `20260423000000_drop_libraries_deleted_at.go` removed the equivalent column from `libraries` two days ago.

## Goals

- Convert `models.Series` from soft-delete to hard-delete.
- Remove the `deleted_at` column, the partial unique index, the `WhereAllWithDeleted()` call, the `s.deleted_at IS NULL` filters in FTS rebuild SQL, and the unused `RestoreSeries` function.
- Recreate the unique index on `(name COLLATE NOCASE, library_id)` without the partial WHERE clause so it enforces uniqueness over the whole table.
- Hard-delete any rows currently in the soft-deleted state as part of the migration. CASCADE on `book_series.series_id` (set in `20260406100000_add_fk_cascades.go`) cleans their join rows; a defensive purge handles any orphaned `series_fts` rows.
- Pin the new behavior with a regression test that fails on the pre-change tree.
- Fix a pre-existing latent bug exposed by smoke testing: `CleanupOrphanedSeries` deletes orphan rows from `series` but never purges the corresponding `series_fts` rows, leaving the search index pointing at non-existent series. The fix changes `CleanupOrphanedSeries` to return the deleted IDs (atomically via `RETURNING id`) and updates all five callers (4 in `pkg/books/handlers.go`, 1 in `pkg/worker/worker.go`) to pair the call with `searchService.DeleteFromSeriesIndex` — same shape as the existing `people` cleanup pattern.
- No user-visible behavior change for the soft-delete removal itself. Soft-deleted series are already filtered everywhere; making them actually gone has no observable effect on the UI, OPDS, eReader, Kobo sync, plugins, or sidecars. The `CleanupOrphanedSeries` fix corrects an observable bug (stale search results pointing at deleted series).

## Non-Goals

- Touching the `users.is_active` "deactivation" toggle in `pkg/users/service.go`. That's a different mechanism (a boolean flag, not a `deleted_at` timestamp), it's actively used, and removing it would require user-facing UX changes.
- Deduplicating the two copies of `CleanupOrphanedSeries` (`pkg/series/service.go` and `pkg/books/service.go`). The duplication exists to avoid an import cycle; resolving it is a separate refactor.
- Frontend `GlobalSearch` cache invalidation on entity mutations was added in this same branch (in scope after smoke testing surfaced "deleted series still in search until refresh"). Series, person, and book mutation hooks now invalidate `[GlobalSearch]` so search results refresh without a manual reload.
- The analogous backend "FTS not cleaned up" bug exists for genres and tags. Pulled into scope alongside series after the smoke test exposed the pattern: `CleanupOrphanedGenres` and `CleanupOrphanedTags` now also return deleted IDs, and all callers in `pkg/books/handlers.go` and `pkg/worker/worker.go` pair the call with the corresponding `searchService.DeleteFromGenreIndex` / `DeleteFromTagIndex`.
- Adding any new restore mechanism. There was no working one before; we're not building one.
- Frontend, OPDS, eReader, plugin SDK, sidecar, or `website/docs/` changes. Verified during exploration that none of these reference soft-delete.

## Architecture

### Migration

New file: `pkg/migrations/20260426000000_drop_series_deleted_at.go`. Modeled on `20260423000000_drop_libraries_deleted_at.go`.

Up direction, in order:

1. `DELETE FROM series WHERE deleted_at IS NOT NULL` — hard-deletes all currently-soft-deleted rows. CASCADE on `book_series.series_id` removes their join rows automatically. Done before the schema change so the unconditional unique index in step 5 can't collide on a name shared between a soft-deleted and a live row in the same library.
2. `DELETE FROM series_fts WHERE series_id NOT IN (SELECT id FROM series)` — defensive purge. Handlers already call `DeleteFromSeriesIndex` on soft-delete today, so this should be a no-op in practice; included so the migration is self-healing if any orphans crept in.
3. `DROP INDEX ux_series_name_library_id` — the partial index references `deleted_at` and would block step 4.
4. `ALTER TABLE series DROP COLUMN deleted_at` — SQLite ≥ 3.35 (March 2021) supports this; the project is well above that version.
5. `CREATE UNIQUE INDEX ux_series_name_library_id ON series (name COLLATE NOCASE, library_id)` — recreate without the partial WHERE clause.

FK enforcement (`PRAGMA foreign_keys=ON`) stays on throughout. That's what makes step 1 cascade to `book_series` cleanly.

Down direction is lossy (does not restore deleted rows), matching the precedent set by `20260423000000_drop_libraries_deleted_at.go`:

1. `DROP INDEX ux_series_name_library_id`
2. `ALTER TABLE series ADD COLUMN deleted_at TIMESTAMPTZ`
3. `CREATE UNIQUE INDEX ux_series_name_library_id ON series (name COLLATE NOCASE, library_id) WHERE deleted_at IS NULL`

### Code changes

**`pkg/models/series.go:21`** — delete the `DeletedAt` field carrying the `bun:",soft_delete"` tag:

```go
DeletedAt *time.Time `bun:",soft_delete" json:"-"`
```

This single tag removal flips every `NewDelete().Model((*models.Series)(nil))` in the codebase from a soft-delete UPDATE to an actual SQL DELETE. No call site needs to change behavior; Bun translates the same builder call to different SQL based on the tag.

**`pkg/series/service.go`:**

- `DeleteSeries` (lines 232–266): the inner `tx.NewDelete().Model((*models.BookSeries)(nil))` is now redundant — `book_series.series_id` already CASCADEs. Simplify to: SELECT distinct affected book IDs, then DELETE the series; CASCADE removes the join rows. Drop the transaction wrapper (one SELECT + one DELETE doesn't need it). Update the doc comment from "soft-deletes a series and hard-deletes its book_series join rows" to "deletes a series; book_series join rows cascade".
- `RestoreSeries` (lines 268–281): delete the entire function. This removes the only `WhereAllWithDeleted()` call in the codebase.
- `MergeSeries` (lines 283–319): unchanged behavior. Update the comment from "moves all books, soft-deletes source" to "moves all books, deletes source".
- `CleanupOrphanedSeries` (lines 321–332): unchanged behavior. Update the comment from "soft-deletes series with no books" to "deletes series with no books".

**`pkg/books/service.go:1363`** — same `CleanupOrphanedSeries` comment fix on the duplicate copy.

**`pkg/search/service.go`:**

- Line 578: drop `AND s.deleted_at IS NULL` from the books-FTS rebuild subquery's join.
- Line 596: drop `WHERE s.deleted_at IS NULL` from the series-FTS rebuild query.

### Tests

Existing tests in `pkg/series/service_test.go` continue to pass without modification. They assert outside-observable hard-delete semantics (book_series rows go away, search index gets reindexed) which are unchanged by the move from soft to hard delete.

New regression test: `TestDeleteSeries_HardDeletesRowAndFTS` in `pkg/series/service_test.go`. Asserts:

1. After `DeleteSeries(id)`, `SELECT count(*) FROM series WHERE id = ?` returns 0. (Proves a hard delete; would fail on the pre-change tree because soft-delete leaves the row in place.)
2. After the handler-level call (which invokes `DeleteFromSeriesIndex`), `SELECT count(*) FROM series_fts WHERE series_id = ?` returns 0.
3. `book_series` rows for the deleted series are gone (already covered by `TestDeleteSeries_RemovesBookSeriesAndFlipsReviewed`; re-asserting here for completeness in the new pinning test).

Follows the Red-Green-Refactor rule from `CLAUDE.md`: write the test first, run on a tree that still has the soft-delete tag (and the migration applied) to confirm assertion 1 fails, then drop the tag + dead code and confirm green.

### CleanupOrphanedSeries — signature change + caller updates

`CleanupOrphanedSeries` has 5 callers (4 in `pkg/books/handlers.go` and 1 in `pkg/worker/worker.go`) and is still useful after this change — deleting a book CASCADEs to remove `book_series` rows but does not remove the series itself, so `CleanupOrphanedSeries` clears empty series after their last book leaves.

However, smoke testing surfaced a pre-existing bug: `CleanupOrphanedSeries` only deletes from the `series` table, leaving stale rows in `series_fts`. The series search path (`searchSeriesInternal` in `pkg/search/service.go`) queries `series_fts` directly with no JOIN to `series`, so deleted-via-cleanup series remain in search results forever. This was masked under soft-delete (the listings filtered on `deleted_at`, but search still showed them), and is more visible under hard-delete (clicking a stale search result 404s rather than showing a hidden "deleted" page).

Fix:

- Change both copies of `CleanupOrphanedSeries` (`pkg/series/service.go` and `pkg/books/service.go`) to return `([]int, error)` — the IDs of deleted series — using `NewDelete().Returning("id").Scan(ctx, &ids)`. SQLite's `RETURNING` clause makes this atomic, so the IDs returned are exactly the rows the same statement deleted (no race between a SELECT and a DELETE).
- Update all 5 callers to iterate the returned IDs and call `searchService.DeleteFromSeriesIndex` for each. This mirrors the existing `people` cleanup pattern at `pkg/books/handlers.go:680-689`.
- The worker's `cleanupOrphanedEntities` previously logged a count from `result.RowsAffected()`; switch to `len(deletedIDs)`.

A new test `TestCleanupOrphanedSeries_ReturnsDeletedIDs` in `pkg/series/service_test.go` pins the signature and behavior.

## Verification

Local, in order:

1. `mise db:rollback && mise db:migrate` — exercises the up migration.
2. `mise db:rollback` — exercises the down migration (restores the column + partial index).
3. `mise db:migrate` — re-up.
4. **Red:** run `TestDeleteSeries_HardDeletesRowAndFTS` against a tree with the migration applied but the model tag still present. Assertion 1 must fail.
5. **Green:** drop the model tag + dead code, re-run the test, confirm pass.
6. `mise lint test` — Go-only edit, per the targeted-subset guidance in `CLAUDE.md`.
7. Final pre-PR: `mise check:quiet`.

Manual smoke (under five minutes):

- `mise start`, log in as `robin:password123`.
- Delete a series from the series list. Confirm it disappears from the list and from global search.
- Merge two series. Confirm the source disappears and the target keeps all books.

## Rollout

- Single PR on the existing `task/soft-delete` branch.
- Migration runs at application startup; no replicas, no concurrent writers, bounded by however many soft-deleted series exist (cheap).
- No feature flag required — soft-deleted series were already filtered everywhere, so the behavior change is invisible to users.
- PR description should call out the lossy-down migration explicitly so a reviewer doesn't assume the down direction is non-destructive.
- No follow-up cleanup PR — this is a one-shot ship with no flag to remove later.
