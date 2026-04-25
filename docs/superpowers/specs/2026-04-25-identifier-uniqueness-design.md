# Identifier Uniqueness — Design

## Background

Each file in Shisho can have external identifiers (ISBN-10, ISBN-13, ASIN, UUID, Goodreads, Google, plugin-defined). The `file_identifiers` table has had `UNIQUE(file_id, type)` since migration `20260113000001` — at most one identifier per type per file. The DB invariant exists, but the layers above silently violate it in three places, producing user-visible "I saved two but only one shows up" behavior:

1. **Manual edit (`FileEditDialog` → `updateFile` handler).** The form lets the user add multiple identifiers of the same type. The handler at `pkg/books/handlers.go:1112-1132` deletes all existing identifiers, then loops calling `CreateFileIdentifier` per item. The unique constraint rejects the second insert; the handler logs the error and continues, so one entry is silently dropped and the API returns 200.

2. **Identify merge (`resolveIdentifiers` in `app/components/library/identify-utils.ts:68-93`).** Dedupes by `${type}:${value}` rather than `${type}`. So current `{asin: A}` + incoming `{asin: B}` produces `[{asin:A},{asin:B}]`. That payload then hits the manual-edit path above and one is silently dropped.

3. **Plugin metadata persistence (`pkg/plugins/handler_persist_metadata.go:263-281`).** Same pattern as `updateFile`: per-item create loop with `log.Warn` on conflict. Same silent drop.

The scanner enricher merge in `scan_unified.go` is already type-aware and is unaffected, but `BulkCreateFileIdentifiers` (the persistence helper it ultimately calls) has no defensive dedupe — if a parser/sidecar/plugin ever yields two of the same type in a single payload, the entire bulk insert fails on the unique constraint and the file ends up with no identifiers.

## Invariant

**A file has at most one identifier per type.** Every layer respects this explicitly:

- DB: `UNIQUE(file_id, type)` (already in place).
- API: payloads with duplicate types are rejected `400`.
- Persistence helper: defensive dedupe with warn log on duplicate from any source (parser/sidecar/plugin).
- Identify merge: type-keyed; incoming wins on conflict.
- Manual edit UI: type already present in the form is non-selectable, with a tooltip explaining why.

## Layered design

### 1. Backend write paths

#### `updateFile` handler (`pkg/books/handlers.go`)

Add request validation at the top of the identifiers branch (before any DB mutation). For each item in `params.Identifiers`:

- Reject `400` if `type` is empty.
- Reject `400` if `value` is empty (after trimming whitespace).
- Reject `400` if any `type` appears more than once across the array.

The 400 response identifies the offending type(s). No partial mutation: validation runs before `DeleteFileIdentifiers`.

Switch the per-item loop to a single `BulkCreateFileIdentifiers` call for consistency with the plugin path.

**Source preservation.** Before deleting, read the existing identifiers for this file. For each payload item, compute its source:

- If a row exists with the same `type` and the same canonical (normalized) value → preserve that row's `source`.
- Otherwise → `models.DataSourceManual`.

The `file.identifier_source` aggregate column continues to be set to `manual` whenever the user edits via this handler (it represents the most recent edit's source as a whole, not per-row provenance).

#### `BulkCreateFileIdentifiers` (`pkg/books/service.go`)

Before the insert, build a map keyed by `Type` and overwrite on subsequent occurrences (last-write-wins). For every overwritten entry, emit a structured `log.Warn` containing:

```
file_id, type, dropped_value, dropped_source, kept_value, kept_source
```

The function returns the deduped slice's normalized values just like today.

#### Plugin metadata write path (`pkg/plugins/handler_persist_metadata.go`)

Replace the per-item create loop with a single `BulkCreateFileIdentifiers` call. Items where `Type == ""` or `Value == ""` continue to be filtered before the bulk call (preserving today's behavior — plugin output is treated as best-effort, not a strict contract).

This requires:

- Adding `BulkCreateFileIdentifiers(ctx, []*models.FileIdentifier) error` to the `bookUpdater` interface (`pkg/plugins/handler.go:52`).
- Wiring it through the adapter at `pkg/server/server.go:302`.

#### Cleanup

After the refactor, `CreateFileIdentifier` has no remaining callers. Delete it along with the corresponding entry on the plugin-handler `bookUpdater` interface and the adapter method in `pkg/server/server.go`.

`DeleteFileIdentifiers` (whole-file delete used by `updateFile` and the scanner) and `DeleteIdentifiersForFile` (whole-file delete with row-count return used by plugin/orphan paths) both remain. They serve different callers and the small return-shape difference is established convention.

### 2. Identify merge (frontend)

#### `resolveIdentifiers` (`app/components/library/identify-utils.ts`)

Change the dedupe key from `${type}:${value}` to `${type}`. Algorithm:

1. If `current` is empty and `incoming` is empty → `{ value: [], status: "unchanged" }`.
2. If `current` is empty and `incoming` is non-empty → dedupe `incoming` by type (last-wins), return `{ value: deduped, status: "new" }`.
3. If `incoming` is empty → return `{ value: current, status: "unchanged" }`.
4. Otherwise:
   - Dedupe `incoming` by type (last-wins).
   - Build a `Map<type, IdentifierEntry>` seeded from `current`.
   - For each entry in deduped-incoming, set `map[type] = entry` (incoming wins on conflict).
   - The merged value is `Array.from(map.values())` in a stable order (preserve current's order, append new types in incoming order).
   - Status:
     - `"unchanged"` if every type's value matches what `current` had and no new types were added.
     - `"changed"` otherwise.

Status `"new"` is only used when transitioning from empty `current`, matching the existing convention.

#### Tests

Update `app/components/library/identify-utils.test.ts` and `app/components/library/IdentifyReviewForm.test.ts`. Existing cases that asserted both `{asin:A, asin:B}` survive must be replaced with cases asserting incoming wins. Add coverage for:

- Same-type-same-value → `"unchanged"`.
- Same-type-different-value → incoming wins, status `"changed"`.
- Incoming has duplicate types within itself → last-wins.
- Mixed: one type unchanged, one type replaced → status `"changed"`, both surviving rows correct.

### 3. Manual edit UI

#### `FileEditDialog.tsx` add-identifier form

The "add identifier" `Select` lists all known types (built-ins plus `pluginIdentifierTypes`). For any type already present in the local `identifiers` state, render its `SelectItem` as `disabled` and wrap it in a `Tooltip` with copy:

> "This file already has a {Type Label} identifier. Remove it first to add a different value."

Use the existing `Tooltip` component from `@/components/ui/tooltip`. The visible label and ordering of the dropdown stay the same; only the disabled state and tooltip are added.

If the user removes an existing entry of that type from the badge list, the dropdown re-enables that type.

#### Tests

Update `app/components/library/FileEditDialog.test.tsx` (existing) with:

- Asserting that when an ASIN already exists in the form, the ASIN option in the type dropdown is disabled.
- Asserting that removing the existing ASIN re-enables the option.

### 4. Display sites

`FileDetailsTab.tsx`, `BookDetail.tsx`, `FetchChaptersDialog.tsx`, `FileChaptersTab.tsx`, and `IdentifyReviewForm.tsx` (current-identifiers display) only render whatever the API returns. Because the DB invariant plus the new write-path validation guarantee no duplicates land, these components need no changes.

### 5. Scanner

The enricher merge in `pkg/worker/scan_unified.go` (`mergeEnrichedMetadata`) is already type-aware. The two scanner write call sites (`scan_unified.go:1962` and `:2000`) call `BulkCreateFileIdentifiers` and now inherit the dedupe + warn defense. No scanner-specific code changes.

### 6. Docs

Update `website/docs/metadata.md` (the existing identifiers section) to:

- State the type-uniqueness rule explicitly: at most one identifier per type per file.
- Document identify behavior: when an identify match brings in an identifier whose type already exists on the file, the incoming value replaces the existing one.

No other docs pages need updates. The plugin developer docs already correctly state that values are canonicalized; the new dedupe defense is a server-internal robustness improvement, not a contract change for plugin authors.

## Tests

In addition to the per-section tests above:

- New: `updateFile` handler test asserting `400` on duplicate-type payload, with no DB mutation (existing identifiers preserved).
- New: `updateFile` handler test asserting `400` on blank-type or blank-value payload.
- New: `updateFile` handler test asserting source preservation — payload that re-submits an existing `(type, value)` keeps the prior `source` in the DB row.
- New: `BulkCreateFileIdentifiers` test asserting last-write-wins dedupe and warn log on duplicates.
- New: plugin metadata persistence test asserting duplicate types from a plugin payload don't crash, produce the warn log, and yield a single row of the last-wins value.

## Out of scope

- No DB migration. The unique constraint already exists; no historical duplicates can be present.
- No global identifier uniqueness across files (same ISBN-13 across different files remains allowed).
- No new identifier types and no rename of existing types.
- No on-disk sidecar rewriting when scanner-time dedupe fires. Sidecars naturally reconcile on the next user save via `WriteFileSidecarFromModel`.
- No SDK or payload schema changes. `source` stays off the wire; provenance is preserved server-side via the read-existing-then-match approach.
- No book-level identifiers. The `book_identifiers` table was dropped in migration `20260406000000` and identifiers remain file-level only.
