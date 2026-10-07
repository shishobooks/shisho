# Search Index (FTS)

The FTS5 tables have no triggers (re-aggregating `GROUP_CONCAT` in every write transaction is too slow during scans and renames), so every write path reindexes explicitly, and CASCADE never cleans them. Rows are keyed by `rowid` equal to the entity id; write and delete them only through `pkg/search/service.go`.

## Keeping the index current: collect, mutate, defer reindex

A write path that changes an indexed name, title, or link, or deletes an indexed entity:

1. Calls `searchService.CollectAffected` with what it touches **before** mutating, because a delete or relink drops the links the expansion follows.
2. Immediately `defer`s `searchService.ReindexAffected`. The defer is required: a mutation can commit and then fail a later step, so the reindex must run on error paths. It never fails the request.
3. Appends ids it learns during the mutation (a created Book) to the collected `search.Affected`.

Transaction boundaries and source stamps stay as they are; the reindex runs after them. An entity created by `FindOrCreate*()` needs its own row: `Index*()` where it is attached, or its id in `Affected`.

## Orphan cleanup

After a Book or File delete, call `books.CleanupOrphanedEntities`, which deletes unreferenced entities and their FTS rows. A new orphanable entity adds its cleanup there.

## Adding a column

A new column that copies data across tables extends `expandAffected` in `pkg/search/affected.go` with the new edge, the table's one `INSERT ... SELECT` in `pkg/search/service.go`, and the fixture of `TestIndexMethods_MatchRebuildAllIndexes`.
