# Search Index (FTS)

SQLite FTS5 tables `books_fts`, `series_fts`, `persons_fts`, `genres_fts`, `tags_fts`, and `publishers_fts`, maintained by `pkg/search/service.go`. They have no triggers (re-aggregating `GROUP_CONCAT` in every write transaction is too slow during scans and person renames), so every write path reindexes explicitly, and CASCADE never cleans them.

## Rows are keyed by `rowid` = entity id

Every insert sets `rowid` explicitly (`INSERT OR REPLACE INTO books_fts (rowid, book_id, ...)`, or `SELECT b.id AS rowid, b.id, ...` in bulk), and every per-entity delete filters `WHERE rowid = ?` through `deleteFTSRow`. The id columns (`book_id`, `series_id`, ...) are `UNINDEXED`: search queries read them, but filtering on them scans the table. An insert that leaves `rowid` unset breaks later deletes. `OR REPLACE` lets two writers indexing one entity converge.

## Keeping the index current: collect, mutate, defer reindex

```go
affected := h.searchService.CollectAffected(ctx, search.Affected{BookIDs: []int{id}})
defer h.searchService.ReindexAffected(ctx, affected)
// mutate; append ids learned along the way
affected.BookIDs = append(affected.BookIDs, newBook.ID)
```

- In `search.Affected`, name what the mutation touches; the service expands: a Person to the Books it authors or narrates, a Series to its Books, every Book to the Series holding it. Genres, Tags, and Publishers reindex only their own rows.
- `CollectAffected` expands **before** the mutation, because a delete or relink drops the links (CASCADE removes a deleted Book's `book_series` rows). `ReindexAffected` expands again afterwards, rewrites each row once, and deletes rows of entities that no longer exist, so callers need no `DeleteFrom*Index` for ids they passed. Every resource delete handler collects the deleted id and defers `ReindexAffected`.
- **The `defer` is required**: a rename commits before `SyncAliases` can reject, and a plugin apply commits the title before a duplicate series fails it, so the reindex must run on error paths. It detaches from ctx cancellation, logs failures, and never fails the request. Both methods are nil-safe.
- Transaction boundaries and source stamps stay as they are; the reindex runs after them.
- Entities created by `FindOrCreate*()` need their own row: `Index*()` where they are attached (the book update handler, `indexBookRelations`, the plugin relationship helpers) or their ids in `Affected`.
- If a new column copies data across tables, extend `expandAffected` in `pkg/search/affected.go` with the new edge.

## Scans

- A full scan does not index as it goes (FilePath mode, `isResync=false`). `ProcessScanJob` runs `RebuildAllIndexes` when it finishes, and in a defer when it fails. A scan cancelled by shutdown skips the rebuild (it would race the deadline); at startup `fetchJobs` calls `rebuildSearchAfterIncompleteScan`, which rebuilds when the latest started scan is not `completed`. The rebuild is one transaction, and with a one-connection pool every other query waits for it.
- A resync (`scanFileCore`, `isResync=true`) collects and defers like a handler; `indexBookRelations` writes only newly attached People, Genres, Tags, and Publishers. A changed file that a full scan hands to `scanFileByID` skips both, because `withSearchRebuildPending` marks the ctx and the closing rebuild covers it.

## Orphan cleanup

After a Book or File delete, a scan, or a monitor batch, call `books.CleanupOrphanedEntities` (`pkg/books/orphans.go`), which deletes every unreferenced Series, Person, Genre, Tag, and Publisher and its FTS row. It is the only remaining `DeleteFrom*Index` caller besides the Book edit handler's targeted loops, because it learns ids from the delete itself. Deleting one File of a Book that keeps others runs `books.CleanupOrphanedPeople`. `CleanupOrphanedSeries` lives on `books.Service` because `pkg/series` imports `pkg/books`. A new orphanable entity adds its cleanup and FTS removal to `CleanupOrphanedEntities`.

## One SQL statement per table

`pkg/search/service.go` holds one `INSERT ... SELECT` per FTS table. `RebuildAllIndexes` runs it unfiltered; `ReindexBookByID`, `ReindexSeriesByID`, and the `Index*` methods (which read only the model's ID) append `WHERE <alias>.id = ?`. An edit and a scan therefore write identical rows regardless of loaded relations. `TestIndexMethods_MatchRebuildAllIndexes` diffs every path against the rebuild; extend its fixture when adding a column. The column sources are readable from those statements.

`pkg/testutils` seeds `books_fts` through `ReindexAffected`, so E2E search matches production.
