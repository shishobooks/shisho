# Identify apply and source attribution

Implementation rules for `POST /plugins/apply` (`handler_apply_metadata.go`, `handler_persist_metadata.go`, `handler_attribution.go`, `handler_relationships.go`, `handler_identifiers.go`, `handler_convert.go`). The policy, which state gets which source and why, is ADR 0006 (`docs/adr/0006-identify-source-attribution.md`); read it first. This page covers where that policy lives in code and the traps around it.

## Request and response

- **`sources` carries intent, never a source string.** `PluginApplyPayload.Sources` maps field keys (plus `file_name` via `SourcesKeyFileName`) to `SourceIntent`. Only a changed value has its intent mapped, through `applyAttribution.sourceFor`, to `plugin:<scope>/<id>` or `manual`. The `oneof` tag on `Sources` and `validateSourceIntents` overlap on purpose: the function also covers identifier entries, which the tag cannot express, and callers that bypass the custom binder (most handler tests).
- **`SeriesEntry` and `ApplyOverrides` are not wire types.** They are assembled from the payload; their generated TS mirrors are unused. `ApplyOverrides.SelectedFields` carries selection presence: omitted means no change, a selected empty optional value means clear, a selected blank Title is rejected. Keep this presence state out of `mediafile.ParsedMetadata`, which is the public plugin contract.
- **The response is `PluginApplyResponse`**: the reloaded Book (with the same `cover_cache_key` `GET /books/:id` computes) plus `warnings`, always a JSON array. A warning is a selected value accepted but skipped (today only covers); the apply still returns 200. Cover helpers return an error for a skip and `errCoverUnchanged` for the identity no-op; `persistMetadata` turns skip errors into warnings with `coverWarning`.

## Scalars

- **A clear nulls the value and its source in one column set.** `applyOptional` treats "value already absent, source still set" as a change, so clearing again heals a leftover source. The healing migration (`20260919000000`) touches plugin sources only; never widen it, because the Edit form stores NULL plus `manual` as a protected empty slot.
- **Sort title follows the Edit form.** A Title change regenerates it only when `sort_title_source` is not `manual` and stamps `filepath`. Never write `manual` or a plugin source there.
- Publisher compares after `FindOrCreatePublisher`, so an alias of the stored publisher is a no-op. Release dates compare by UTC calendar day.

## Relationships

- **Resolve, then compare IDs.** Authors (ordered Person IDs plus role, nil role equals empty), Narrators (ordered IDs), Genres and Tags (unordered, aliases resolving to one ID deduplicated). A no-op keeps the source and skips relationship writes and new-entity `Index*` calls. The route still reindexes the Book and every Series it was or is in via `CollectAffected` and a deferred `ReindexAffected`, changed or not.
- Lookup and insert failures return errors; never drop an entry silently. A complete clear nulls the aggregate source, including a stale one on an already-empty collection. `books.author_source` is nullable but its Go field is a `nullzero` string, so clearing assigns `""`.
- **Series memberships** use `books.series_source`. `applySeries` resolves through `FindOrCreateSeries`, then rejects a Series listed twice by resolved ID (so a rejected apply can leave a freshly created, unattached Series), and compares ID, order, and the Series Number group. The source passed to `FindOrCreateSeries` describes the Series name; never read `Series.NameSource` as membership provenance. Plugin scalar `md.Series` goes through the same function.
- **Malformed Series Number groups are a validation error before anything persists** (`extractSeriesEntries` via `strictSeriesNumberGroupFromFields`), for both array entries and the top-level `series_number*` keys beside a string `fields.series`. `convertFieldsToMetadata` parses leniently, so never persist its Series fields without that check. Hook results instead drop malformed groups (`parsePluginSeriesNumberGroup`).

## Identifiers

Two layers: `file.IdentifierSource` (aggregate, gates Scan replacement, follows `sources.identifiers`) and each entry's `Source` (follows the per-entry intent in `ApplyOverrides.IdentifierIntents`, keyed by trimmed type). `applyIdentifiers` builds rows with intent-mapped sources, then `identifiers.ReconcileSources` (shared with the Book edit handler) restores the stored source on every unchanged `(type, normalized value)` and reports equality; equal sets skip the delete and insert. `validateIdentifierTypes` rejects duplicate types in `applyMetadata` before any field persists, because `BulkCreateFileIdentifiers` dedupes by type and would drop one after the delete ran.

## Covers

- The form sends no cover field to keep the current Cover, or `cover_url` / `cover_page` to accept the proposal. `applyCoverImage` and `applyCoverPage` write the whole Cover state (bare `cover_image_filename`, `cover_mime_type` of the normalized bytes, `cover_source`, and `cover_page` when page-based) in one column set.
- **Gate on `fileutils.NormalizeImage`'s error**, which decodes every pixel. `fileutils.ImageResolution` reads only the header and accepts a truncated body.
- **Never delete the working cover before the row is saved.** Both paths install with `fileutils.WriteFileAtomic`, then return the previous covers by base name (`fileutils.OtherCoverExtensions`); `persistMetadata` removes them only after `UpdateFile` succeeds. The page extractor therefore returns `(filename, mimeType, stale, err)`, and a stub must return the stale paths it wants removed.

## Tests

End-to-end regressions are the `pkg/worker/*identify*_test.go` files. They use native EPUB and M4B fixtures so a cleared scalar is provably restored from embedded metadata; the shared `auto-enricher` fixture proposes a description, a publisher, and two identifiers on every Scan, and `newIdentifyApplyServer` wires `books.NewPluginPageExtractor` as `pkg/server` does.
