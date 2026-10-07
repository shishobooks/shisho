# Data Sources, Resource Deletes, and Merges

The priority order and series provenance are under "Data sources" in `pkg/AGENTS.md`.

## Writing metadata

- **During scans, enricher plugins override file-embedded metadata per field**, then the file parser fills the gaps (`runMetadataEnrichers`). Identifier provenance is per entry: each `file_identifiers` row carries its own source.
- **Identifier edits reconcile through `identifiers.ReconcileSources`**, which keys both sides on type plus normalized value so pre-normalization values still match. Shared identifier logic lives in `pkg/identifiers`, because `pkg/books` imports `pkg/plugins`. Reject duplicate identifier types before any delete: `BulkCreateFileIdentifiers` dedupes by type and would silently drop a row.
- **Series number groups are atomic.** `series_number`, `series_number_end`, and `series_number_unit` always come from one source: copy, merge, clear, validate, and overlay them together, using only the rules in `pkg/seriesnum`. Malformed external groups (hook results, sidecars) are discarded whole.

## Resource deletes

A new delete path for an owner-referenced resource (one a Book or File points at) does what the existing ones do. `deleteSeries` in `pkg/series/handlers.go` is the reference; add the new path to `pkg/server/resource_delete_test.go`.

1. **Stamp `manual` on every affected owner's `*_source`** inside the delete's transaction, before the join rows or foreign key go, whatever the prior source and even when other members remain. Otherwise the sidecar from the last scan re-creates the resource on the next scan (ADR 0006). Merges and orphan cleanup are exempt.
2. **Recompute Reviewed after commit** for every affected Book, through the shared books service. A package that `pkg/books` imports takes the `review.BookReviewRecomputer` interface instead.
3. **Reindex** per `search-fts.md`.

## Merges

Every resource merge and every re-point mutation follows this checklist. References: `merge` in `pkg/people/handlers.go`, `MergePeople` in `pkg/people/service.go`, and `merge.CheckPreconditions`.

1. **Retrieve both sides first**, so a missing one is a 404 from the retrieve rather than a 500 from inside the transaction.
2. **Run `merge.CheckPreconditions`** for library access, self-merge, and cross-library rejection.
3. **Keep a self-merge backstop in the service** (`merge.SelfMergeError`), because a self-merge deletes the target and every link to it.
4. **Dedupe join rows before re-pointing.** Where the target already has the row the source would move, drop the source's row or the re-point violates the unique index. Compare nullable columns with `IS`, since a unique index treats NULLs as distinct and `=` misses them.
5. **Trees must not gain a cycle**, even from corrupt circular data.
6. **Aliases go through `aliases.TransferAliasesOnMerge`.**
7. **Reindex after commit** with the target and source in `search.Affected` (`search-fts.md`). Calling `Index*` on the target directly would re-insert a ghost row after a self-merge.
8. **Leave Reviewed and sources alone.** A merge does not change which resources a Book has.

**Renames onto another resource's name**: Genres, Tags, and Publishers merge into the existing one; Series and People reject with a 422 telling the user to merge, so combining them is always explicit.
