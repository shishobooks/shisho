# Chapter filename parsing for CBZ files

## Background

`pkg/fileutils/naming.go` has special parsing logic for CBZ filenames that recognizes volume indicators (`v01`, `vol.5`, `volume 12`, `#001`, bare trailing numbers) and normalizes them into a `Title v001` form. The parsed volume number is plumbed through to `book_series.series_number` so manga and comics can be filtered, sorted, and displayed by volume.

CBZ files released as individual chapters — common in scanlator and ongoing manga workflows — currently fall through this pipeline. A file named `One Piece Ch.5.cbz` keeps the literal `Ch.5` in its title, gets no series number extracted, and is not recognized as belonging to a series via filename. After organization it becomes neither `v005` nor `c005` — just an unparsed filename.

This spec extends the CBZ filename pipeline to recognize chapter indicators alongside volume indicators, and propagates a new `series_number_unit` field end-to-end so the unit survives parsing, storage, file organization, sidecars, and display.

## Goals

- Recognize chapter indicators (`Ch.5`, `chapter 5`, `c042`) in CBZ filenames during scan.
- Store the parsed unit (`"volume"`, `"chapter"`, or null) alongside the existing series number.
- Preserve the unit through file organization round-trips: a chapter file stays a chapter file after organize → rescan.
- Display the unit correctly in the UI (`Vol. 5` vs `Ch. 42`).

## Non-goals

- ComicInfo.xml `<Number>` field handling. The CBZ ComicInfo parser already reads `<Volume>` for series number; recognizing `<Number>` as a separate chapter field is a follow-up.
- Backfilling existing rows. Existing `book_series` rows keep `series_number_unit = NULL`, which renders as today's volume default for CBZ at display time.
- M4B and EPUB indicators. This change is CBZ-only, matching the current scope of `NormalizeVolumeInTitle`.

## Design decisions

- **Reuse `book_series.series_number` plus a new unit marker.** The alternatives — adding a separate `chapter_number` column, or losing the volume/chapter distinction by reusing series number alone — were ruled out for being too heavyweight and too lossy respectively.
- **Non-CBZ files keep `unit = NULL`.** EPUB/M4B/PDF use series number as "book N in series"; there's no volume/chapter ambiguity to disambiguate. The unit is a CBZ-only concern today.
- **Ambiguous indicators (`#001`, bare trailing numbers) keep defaulting to `volume`.** This preserves current behavior for the existing CBZ ecosystem; rescanning files already in the DB doesn't shift their semantics.
- **Organized output reflects the parsed unit.** A chapter file gets renamed to `Naruto c042.cbz`, not `Naruto v042.cbz`. This avoids silently flipping the unit on rescan.

## Parsing changes

### `pkg/fileutils/naming.go`

Rename and extend the title normalizer:

- `NormalizeVolumeInTitle(title, fileType) (string, bool)` becomes `NormalizeSeriesNumberInTitle(title, fileType) (normalized string, unit string, ok bool)` where `unit` is one of `""`, `"volume"`, or `"chapter"`.
- Pattern order (first match wins, all anchored to end-of-string):
  1. `(?i)\s*chapter\s*(\d+(?:\.\d+)?)\s*$` → chapter
  2. `(?i)\s*ch\.?\s*(\d+(?:\.\d+)?)\s*$` → chapter
  3. `(?i)\s*c(\d+(?:\.\d+)?)\s*$` → chapter (compact; placed after explicit `ch` so `chN` isn't eaten by the compact pattern)
  4. `(?i)\s*#(\d+(?:\.\d+)?)\s*$` → volume (ambiguous default)
  5. `(?i)\s*v(\d+(?:\.\d+)?)\s*$` → volume
  6. `(?i)\s*vol\.?\s*(\d+(?:\.\d+)?)\s*$` → volume
  7. `(?i)\s*volume\s*(\d+(?:\.\d+)?)\s*$` → volume
  8. `\s+(\d+(?:\.\d+)?)\s*$` → volume (ambiguous default)
- Normalized output: `Title v001` for volume, `Title c001` for chapter. Zero-padding and fractional formatting (`v001.5`/`c001.5`) match today's volume behavior.

Update the helpers:

- `formatVolumeNumber(volume float64, fileType string) string` → `formatSeriesNumber(number float64, unit string, fileType string) string`. For `unit == "chapter"` outputs `c{n}`; otherwise outputs `v{n}` (preserves today's behavior when unit is empty or `"volume"`).
- `extractVolumeFromTitle(title string) *float64` → `extractSeriesNumberFromTitle(title string) (*float64, string)`. Recognizes both `v{n}` and `c{n}` suffixes; returns the unit alongside the number.
- `ExtractSeriesFromTitle(title, fileType) (string, *float64, bool)` → `(string, *float64, string, bool)` — adds unit to the return tuple.
- `IsOrganizedName` regex gains `c\d+(?:\.\d+)?` alongside the existing `v\d+|#\d+`.
- `OrganizedNameOptions` gains `SeriesNumberUnit *string`. `GenerateOrganizedFolderName` passes it to `formatSeriesNumber` and uses `extractSeriesNumberFromTitle` (instead of the old volume-only check) to decide whether to skip stamping when the title already encodes a number.

## Data model

### `models.BookSeries` (`pkg/models/series.go`)

Add:

```go
SeriesNumberUnit *string `json:"series_number_unit,omitempty"`
```

Bun column: `series_number_unit`. Nullable.

### Constants (`pkg/models/series.go`, alongside the model)

```go
const (
    SeriesNumberUnitVolume  = "volume"
    SeriesNumberUnitChapter = "chapter"
)
```

In-memory representation: parsing functions return unit as a plain `string` where `""` means "no unit parsed". When persisted to `models.BookSeries.SeriesNumberUnit` (`*string`) or `mediafile.ParsedMetadata.SeriesNumberUnit` (`*string`), `""` maps to `nil`; the constants above are the only non-nil values written.

### Migration

New migration adds a nullable `series_number_unit TEXT` column to `book_series`. No backfill — null rows render as today.

### `mediafile.ParsedMetadata` (`pkg/mediafile/mediafile.go`)

Add `SeriesNumberUnit *string` so file parsers and plugins can populate the unit.

### Plugin SDK (`packages/plugin-sdk/`)

Add the optional `seriesNumberUnit` field to the TypeScript `ParsedMetadata` shape so plugins compile cleanly. Optional / additive only — no breaking change.

## Scanner integration (`pkg/worker/`)

Three call sites are affected:

1. **`scan.go` filename-based title derivation (~line 93)** and **`scan_unified.go` `deriveInitialTitle` (~line 2599, 2639):** switch to `NormalizeSeriesNumberInTitle`. The unit it returns flows back to the caller along with the title. Where these helpers feed `metadata.Title`, they also set `metadata.SeriesNumberUnit` if a unit was parsed.
2. **`scan_unified.go` `applyFilepathFallbacks` (~line 2706):** when `metadata.Series` is empty and `ExtractSeriesFromTitle` succeeds, capture the unit and set `metadata.SeriesNumberUnit`. The data source for the unit follows the data source for the series number (`DataSourceFilepath`).
3. **`scan_unified.go` `applyParsedMetadata` (~line 830):** when writing `book_series.series_number`, also write `series_number_unit` from `metadata.SeriesNumberUnit`. Use the same priority/source gating as `SeriesNumber` so manual overrides aren't clobbered.

Existing `extractVolumeFromTitle` callers that gate "title already encodes a number — skip stamping" must use the new `extractSeriesNumberFromTitle` so a `Naruto c042` title isn't double-stamped with `v042`.

## File organization

`OrganizedNameOptions.SeriesNumberUnit` flows from the book's `BookSeries` row into `RenameOrganizedFile`. Output format is `c{n}` for chapter and `v{n}` for volume or null.

`pkg/books/handlers.go` is the trigger from API edits. Wherever it constructs `OrganizedNameOptions`, populate `SeriesNumberUnit` from the relevant `BookSeries` row (the one that drives the existing `SeriesNumber`).

## Sidecars (`pkg/sidecar/`)

Add `SeriesNumberUnit *string` to whichever sidecar struct holds `SeriesNumber` today (book sidecar). Round-trip: read populates `metadata.SeriesNumberUnit`, write emits the field. Old sidecars omit it; readers tolerate the absence.

## API and frontend

### API

Responses already serialize `BookSeries`, so the unit ships automatically once tygo regenerates types. Update payloads (`pkg/books/validators.go`) accept `series_number_unit` alongside `series_number` for any endpoint that lets a user edit the series number.

### Frontend

- Display: render `Vol. 5` when unit is `"volume"` *or* null on a CBZ; render `Ch. 42` when `"chapter"`. Non-CBZ files keep current rendering. Centralize this decision in a small helper to avoid duplication across book card, book details, and series rows.
- Edit form: add a unit dropdown next to the series number input on the book metadata edit form. Options: `Volume`, `Chapter`, `(unspecified)`.

### Docs

- `pkg/cbz/CLAUDE.md` — note the chapter-vs-volume parsing in the filename pipeline.
- `website/docs/metadata.md` — document the new `series_number_unit` field, including how filenames map to it.

## Testing

Per project convention, follow Red-Green-Refactor TDD.

- `pkg/fileutils/naming_test.go`: table-driven tests for each pattern, ambiguous defaults, round-trip parse → format → parse, and `IsOrganizedName` recognition for `c{n}` shapes. Add `t.Parallel()`.
- `pkg/worker/scan_unified_test.go`: integration coverage that a CBZ filename like `One Piece Ch.5.cbz` results in a book with `series_number = 5` and `series_number_unit = "chapter"`, and a `Naruto v01.cbz` results in `series_number = 1` and `series_number_unit = "volume"`.
- Sidecar round-trip test for the new field.
- Frontend display helper tests.

## Open considerations

- The `c{n}` compact pattern is sensitive to titles ending in lowercase `c` followed by digits. The pattern requires a leading whitespace boundary (`\s*c(\d…)`), which keeps real-world manga titles safe but may misfire on contrived edge cases. Acceptable risk; the same risk exists today for `v{n}`.
- A title like `Title Vol.1 Ch.3.cbz` matches `Ch.3` first (end-anchored). The volume is dropped. Acceptable for v1 — single-unit semantics — and revisitable if real files surface this shape.
