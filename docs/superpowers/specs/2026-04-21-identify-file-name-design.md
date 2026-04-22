# Identify: Sync File Name With Book Title, Plus Scan/Identify Consistency Fixes

## Problem

When a user identifies a book via the interactive identify flow, the book's
`Title` is updated but the target file's `Name` is not. File organization (and
downloads) prefer `file.Name` over `book.Title`, so the file on disk keeps its
pre-identify name even when `OrganizeFileStructure` is enabled and the book
has been renamed.

While investigating, two related inconsistencies between the scan path
(`pkg/worker/scan_unified.go`) and the identify apply path
(`pkg/plugins/handler.go` `persistMetadata`) surfaced:

1. **CBZ volume normalization** runs unconditionally on scan titles. That
   rewrites titles like `"Naruto v1"` to `"Naruto v001"` even when the title
   came from a user-curated source (plugin enricher, sidecar, manual edit).
   Identify doesn't normalize at all, so the two paths diverge, and the scan
   behavior is surprising when a user sees `"Naruto v1"` in a plugin search
   result and clicks apply.
2. **Description HTML stripping** runs in the identify path and in the
   sidecar branch of the scan path, but not in the scan's metadata branch.
   Plugin enrichers can return HTML descriptions, and the result lands in
   `book.Description` unstripped during scan but stripped during identify.

## Goals

- Make the identified title flow to the target file's `Name` so that renames
  and downloads reflect the book the user just identified.
- Stop rewriting user-curated titles with volume normalization.
- Strip HTML from descriptions in all paths.

## Non-goals

- Changing how supplement files are named. Supplements continue to derive
  their own `Name` from their filename during scan and are not touched by
  identify.
- Changing `generateCBZFileName`'s `Series + Number` fallback for genuinely
  titleless CBZs during scan.
- Propagating chapters through the identify payload. Plugins don't return
  chapters via the enricher hook, and `convertFieldsToMetadata` doesn't
  extract them.
- Reorganizing files on disk when `OrganizeFileStructure` is disabled. The
  DB value of `file.Name` updates regardless; the physical rename still
  gates on the library setting via the existing `OrganizeBookFiles`
  early-return.

## Design

### Change 1 — Identify apply mirrors title onto main file's Name

In `pkg/plugins/handler.go` `persistMetadata`, the Title block
(handler.go:1689-1696) currently updates `book.Title` but not
`targetFile.Name`. Extend it so the same title is mirrored onto the target
file's `Name` and `NameSource` when the target is a main file:

```go
title := strings.TrimSpace(md.Title)
if title != "" {
    book.Title = title
    book.TitleSource = pluginSource
    book.SortTitle = sortname.ForTitle(title)
    book.SortTitleSource = pluginSource
    columns = append(columns, "title", "title_source", "sort_title", "sort_title_source")

    // Mirror the identified title onto the target main file's Name so
    // file organization and downloads reflect it. Supplements keep their
    // own filename-based label.
    if targetFile != nil && targetFile.FileRole == models.FileRoleMain {
        targetFile.Name = &title
        targetFile.NameSource = &pluginSource
        fileColumns = append(fileColumns, "name", "name_source")
    }
}
```

The `FileRole == FileRoleMain` guard is defensive: the identify UI
(`IdentifyBookDialog.tsx:69-70`) already filters `book.files` to main files,
but the API can default `targetFile` to `book.Files[0]`, which could be a
supplement if ordering puts one first. The guard stops that edge case from
overwriting a supplement's distinct filename-based `Name`.

No volume normalization is applied. Whatever the plugin returned (or the
user edited in the review form) is stored verbatim — `"Naruto v1"` stays
`"Naruto v1"`.

`fileColumns` is the existing accumulator used by later blocks
(narrator/publisher/imprint/cover). The single `UpdateFile` call near the
end of the function flushes it.

### Change 2 — Scan only normalizes volume notation for file/filepath sources

In `pkg/worker/scan_unified.go` at lines 810-813, normalization runs
unconditionally. Restrict it to titles that came from the file itself or
its path:

```go
title := strings.TrimSpace(metadata.Title)
titleSource := metadata.SourceForField("title")

// Only normalize volume notation for titles that came from the file itself
// or its path. Plugin/sidecar/manual titles are user-curated and must not
// be rewritten (e.g., "Naruto v1" must not become "Naruto v001").
if models.GetDataSourcePriority(titleSource) >= models.DataSourceFileMetadataPriority {
    if normalizedTitle, hasVolume := fileutils.NormalizeVolumeInTitle(title, file.FileType); hasVolume {
        title = normalizedTitle
    }
}
```

Priority thresholds (see `pkg/models/data-source.go`):

| Source | Priority | Normalize? |
|--------|----------|------------|
| `manual` | 0 | no |
| `sidecar` | 1 | no |
| `plugin` / `plugin:scope/id` | 2 | no |
| `file_metadata`, `epub_metadata`, `cbz_metadata`, `m4b_metadata`, `pdf_metadata` | 3 | yes |
| `filepath` | 4 | yes |

`deriveInitialTitle` and `applyFilepathFallbacks` already normalize
filepath-sourced titles at the point they derive them from the folder or
filename, so filepath titles arrive here already normalized. The gated
block at 810 then only affects `file_metadata` (and variants). This
matches the user's intent: normalization is a cleanup for data we extract
from the filesystem, not for titles a human (or a plugin acting as one)
chose.

The identify apply path (Change 1) does not normalize at all, which stays
consistent with this rule — plugin-sourced titles are priority 2 and skip
normalization on scan too.

**Known behavior change.** Existing libraries where CBZ files have embedded
metadata like `Title = "#7: Some Title"` previously got normalized to
`"Some Title v007"` in `book.Title` on scan; after this change, they'll
land as `"#7: Some Title"`. This is the intended new rule (normalization
only applies to filepath-derived titles, which are already normalized at
the derivation step). Users who relied on the old behavior will see their
CBZ book titles shift back to the raw form on the next scan. Document this
in the release notes.

### Change 3 — Scan strips HTML from description metadata

In `pkg/worker/scan_unified.go` at line 887, swap:

```go
description := strings.TrimSpace(metadata.Description)
```

for:

```go
description := htmlutil.StripTags(strings.TrimSpace(metadata.Description))
```

This mirrors the existing sidecar branch (scan_unified.go:907) and the
`persistMetadata` path. HTML from enrichers never reaches `book.Description`
unstripped regardless of which path wrote it.

## Out of scope but noted

- `persistMetadata` has minor formatting differences from scan (e.g., no
  `TrimSpace` on URL) that aren't worth fixing in this change.
- `applyFilepathFallbacks` extracts series + number from a normalized
  CBZ title as a filepath-only fallback. Identify doesn't do this — a
  plugin returning `title = "My Series v7"` with no explicit series field
  won't populate `book.BookSeries`. This is an edge case and a plugin
  responsibility; not addressed here.
- Description sidecar-from-metadata path already strips HTML; scan's
  metadata branch is the only outlier being fixed.

## Files changed

- `pkg/plugins/handler.go` — Change 1.
- `pkg/worker/scan_unified.go` — Changes 2 and 3.
- `pkg/plugins/handler_apply_metadata_test.go` — new tests (below).
- `pkg/worker/scan_unified_test.go` and/or `scan_helpers_test.go` — new
  tests and updates to any existing tests that assert volume normalization
  on non-filepath sources.

## Test plan

Follow Red-Green-Refactor per the project's testing rules.

### `handler_apply_metadata_test.go`

- `TestApplyMetadata_UpdatesMainFileName_WhenTitleChanges` — apply
  `md.Title = "New Title"` to a book with one main file, assert the file's
  `Name` is `"New Title"` and `NameSource` is `"plugin:<scope>/<id>"`.
- `TestApplyMetadata_DoesNotUpdateSupplementFileName` — call
  `applyMetadata` with a target whose `FileRole` is supplement (simulate
  the pathological fallback case) and assert `Name` is unchanged.
- `TestApplyMetadata_PreservesVolumeNotation_CBZ` — apply
  `md.Title = "Naruto v1"` to a CBZ main file and assert
  `book.Title == "Naruto v1"` and `file.Name == "Naruto v1"` (no `v001`).
- The existing `TestApplyMetadata_OrganizesFiles_WhenTitleChanges` should
  continue to pass; update its assertions if needed so the post-rename
  filename reflects the new title rather than the old.

### `scan_unified_test.go`

- CBZ with embedded metadata title `"#7: Title"` (source
  `cbz_metadata` → priority 3) → assert `book.Title == "Title v007"`
  (existing behavior preserved for file-derived sources).
- CBZ whose enricher returns title `"Naruto v1"` (plugin source) →
  assert `book.Title == "Naruto v1"` (no normalization on plugin source).
- Sidecar-sourced title `"Something v2"` → assert stored as
  `"Something v2"` (no normalization).
- Scan of a file with metadata description `"<p>Hello <b>world</b></p>"`
  → assert `book.Description == "Hello world"` (HTML stripped).

### Manual verification

- Identify a book whose existing file is named `some_messy_filename.epub`
  with `OrganizeFileStructure` enabled. After apply, file on disk should
  be renamed to match the identified title.
- Identify the same book with `OrganizeFileStructure` disabled. After
  apply, `file.Name` in the DB should be updated to the new title but the
  file on disk should not move.
- Identify a book via a plugin that returns `"Naruto v1"`. Verify
  `book.Title` and `file.Name` both read `"Naruto v1"`, not `"Naruto v001"`.

## Docs

User-facing behavior changes:

- Running scan no longer rewrites user-curated titles with CBZ volume
  normalization — note in release notes.
- Identify now renames the main file to match the identified title when
  `OrganizeFileStructure` is enabled.

Website docs impact check: `website/docs/` pages to verify/update:
- `metadata.md` — if it describes the title normalization rules, update
  to reflect the source-gated behavior.
- Any page describing the identify workflow — mention that applying now
  renames the main file.

No new config fields; `shisho.example.yaml` unaffected.
