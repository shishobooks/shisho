# Data Sources, Resource Deletes, and Merges

The priority list and series provenance are in "Data Source Priority System" in `pkg/AGENTS.md`. This doc covers how scans merge sources, what resource deletes must do to owners, and the merge checklist.

## Scan merge

During scans, enricher plugins override file-embedded metadata per field (enricher-first merge in `runMetadataEnrichers`, then `mergeEnrichedMetadata`).

**Identifier provenance is per entry.** `mergeEnrichedMetadata` unions identifiers by type (the earlier contributor keeps a shared type) and stamps each appended entry's `mediafile.ParsedIdentifier.Source` (`json:"-"`, never on the wire). `FieldDataSources["identifiers"]` stays with the first contributor, which is the highest priority because enrichers merge before the file-parser fallback; overwriting it per appended entry would mislabel a mixed collection with its lowest origin. Persistence writes each `file_identifiers.source` from the entry's `Source`, falling back to the field source.

- `shouldUpdateRelationship` compares values only, so the identifier block also calls `identifierAttributionStale` when values match: an ordinary scan rewrites only when the incoming aggregate strictly outranks the stored one (leaving manual and sidecar collections alone), and a forced refresh rewrites when any entry's origin differs.
- Intended: Identify accepting a proposal that omits an embedded identifier saves a plugin-sourced collection without it, and the next ordinary scan restores it because the union's aggregate has the same plugin priority (`TestScan_MixedIdentifierCollection_OrdinaryScanRestoresEmbeddedAfterIdentifyAccept`).

**Identifier edits reconcile through `identifiers.ReconcileSources(existing, incoming)`.** The Book edit handler and Identify build incoming `[]*models.FileIdentifier` with the source a new or replaced entry should get, and the helper keys both sides on `identifiers.Key` (type plus normalized value) so pre-normalization values still match. `pkg/books` imports `pkg/plugins`, so shared identifier logic lives in `pkg/identifiers`. Reject duplicate identifier types before any delete; `BulkCreateFileIdentifiers` dedupes by type and would silently drop a row.

**The scanner canonicalizes incoming series names.** `shouldUpdateParsedSeries` and `seriesSidecarMatches` compare names (Identify compares resolved IDs), because `FindOrCreateSeries` has side effects and cannot run before the priority gate. `canonicalAttachedSeriesName` in `scan_unified.go` first maps an incoming name (parsed, enricher, or sidecar) to the attached Series' current name when it matches that name or an Alias. Without it, renaming a Series and keeping the old name as an Alias reinserts the membership every scan, and without the Alias creates a duplicate Series.

**Series number groups are atomic.** `series_number`, `series_number_end`, and `series_number_unit` always come from one source: copy, merge, clear, validate, and overlay them together. `pkg/seriesnum` holds the only rules: `ValidateGroup` returns why a group with a start is rejected, `ValidGroup` reports whether a group has a start and passes, and `Group` returns a valid group unchanged or three nils. An end needs a finite start, both ends must be finite, and external ranges need end greater than start. Malformed external groups (hook results, sidecars) are discarded whole; Identify apply rejects them as validation errors (see `docs/agents/plugins/identify-apply.md`). The book update handler clears a unit without a start and collapses an end equal to the start before `ValidateGroup`; plugins and the scanner call `Group` or `ValidGroup`.

## Resource deletes

`DeletePublisher`, `DeleteGenre`, `DeleteTag`, `DeleteSeries`, and `DeletePerson` do three things a new delete path for an owner-referenced resource must copy. `deleteSeries` in `pkg/series/handlers.go` is the reference; `pkg/server/resource_delete_test.go` covers the wiring.

1. **Stamp `manual` on every affected owner's source** (`publisher_source`, `genre_source`, `tag_source`, `series_source`, `author_source`, `narrator_source`) inside the delete's transaction, before the join rows or foreign key go, whatever the prior source and even when other members remain. Otherwise the sidecar from the last scan re-creates the resource on the next scan (ADR 0006). `pkg/worker/collection_delete_scan_test.go` shows the scan-level test. Merges and orphan cleanup are exempt.
2. **Recompute Reviewed after commit.** The service returns affected Book IDs (Publisher: Books owning a File it published; Person: Books it authored plus Books owning a File it narrated; each once) and the handler passes them to `RecomputeReviewedForBooks`. These handlers take the `review.BookReviewRecomputer` interface (declared in `pkg/books/review`, which imports none of them) because `pkg/books` imports them. Series takes the full `*books.Service` because it also uses it for covers and listings. All of them, and `pkg/chapters`, need the shared `WithAppSettings` books service or the recompute silently does nothing.
3. **Reindex** per `search-fts.md`. Genre, Tag, and Publisher pass only their own id, since `books_fts` has no such columns.

**Single-file deletes.** `deleteFile` in `pkg/books/handlers.go` runs `books.CleanupOrphanedPeople` when other Files remain (the deleted File's Narrators may have narrated nothing else) and the full `CleanupOrphanedEntities` when the last File goes.

## Merges

Every resource merge (People, Series, Genres, Tags, Publishers) and every re-point mutation follows this checklist. References: `merge` in `pkg/people/handlers.go`, `MergePeople` in `pkg/people/service.go`, and `merge.CheckPreconditions`.

1. **Retrieve both sides first**, so a missing one is a 404 from the retrieve rather than a 500 from inside the transaction.
2. **Run `merge.CheckPreconditions`:** 403 without access to either side's Library (the source gets deleted), 422 on a self-merge, 422 across Libraries, 401 for a nil user.
3. **Keep a self-merge backstop in the service.** Each `Merge*` returns `merge.SelfMergeError` when target equals source, because a self-merge deletes the target and every link to it.
4. **Dedupe join rows before re-pointing.** Where the target already has the row the source would move (same Book for Genre, Tag, or Series; same Book and role for an Author; same File for a Narrator), drop the source's row or the re-point violates the unique index. Compare nullable columns with `IS`: `ux_authors_book_person_role` treats NULL roles as distinct, so `=` lists a generic Author twice. `MergeSeries` also copies the source's number group into a target row that has none. A column reference like `files.publisher_id` cannot collide.
5. **Trees must not gain a cycle.** `MergePublishers` moves a target that sits below the source to the source's parent (a root when the source has none, or when a pre-existing cycle would loop) before re-parenting the source's children. `descendantIDsSubquery` walks with `UNION` so corrupt circular data cannot loop it.
6. **Aliases go through `aliases.TransferAliasesOnMerge`.** It moves the source's aliases and adds the source's name as a target alias, skipping a name that is already the target's name or alias or another resource's alias.
7. **Reindex after commit** with the target and source in `Affected` (`search-fts.md`). This drops the deleted source's row; calling `Index*` on the target directly re-inserted a ghost row after a self-merge.
8. **Leave Reviewed and sources alone.** A merge does not change which resources a Book has.

**Renames onto another resource's name** differ by kind. Genres, Tags, and Publishers merge into the existing one (guarded by `existing.ID != id`). Series and People reject with a 422 telling the user to merge, so combining them is always explicit.

**The Books merge** (`mergeBooks` in `pkg/books/handlers.go`) is a file move: it rejects a source listed twice, skips the target when listed as a source, requires every source in the target's Library, and runs `CleanupOrphanedEntities` when it deletes a source, as `deleteBook` does. The Book edit handler stores a Person once per role when the same author name (any case) is sent twice.
