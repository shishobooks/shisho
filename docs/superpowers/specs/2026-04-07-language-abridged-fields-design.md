# Language and Abridged Fields

**Date:** 2026-04-07
**Status:** Approved

## Problem

Files in Shisho have no way to track language or abridged status. Both are common metadata fields:
- **Language** is embedded in EPUB (`<dc:language>`), CBZ (`LanguageISO`), and available in M4B/PDF, but parsers currently discard these values since there's no field to store them.
- **Abridged** status is meaningful for all file types (abridged editions exist for print and audio), but has no standard embedded metadata in most formats. M4B can store it via Tone-style freeform atoms.

Users want to see, edit, and filter by these fields. Plugins should be able to set them during parsing and enrichment.

## Solution

Add two new nullable fields to the `files` table — `language` (ISO 639-1 code) and `abridged` (nullable boolean) — with full stack support: parsing, editing, sidecar persistence, plugin SDK, file generation, and UI filtering.

## Data Model

### Database Migration

New columns on `files`:

| Column | Type | Description |
|--------|------|-------------|
| `language` | `TEXT` | ISO 639-1 code (e.g., `"en"`, `"fr"`, `"ja"`), nullable |
| `language_source` | `TEXT` | Data source tracking |
| `abridged` | `INTEGER` | Nullable boolean (0=unabridged, 1=abridged, NULL=unknown) |
| `abridged_source` | `TEXT` | Data source tracking |

### Go Model (`pkg/models/file.go`)

```go
Language       *string `bun:"language" json:"language"`
LanguageSource *string `bun:"language_source" json:"language_source"`
Abridged       *bool   `bun:"abridged" json:"abridged"`
AbridgedSource *string `bun:"abridged_source" json:"abridged_source"`
```

### ParsedMetadata (`pkg/mediafile/mediafile.go`)

```go
Language *string `json:"language,omitempty"` // ISO 639-1 code
Abridged *bool   `json:"abridged,omitempty"`
```

Both are pointer types: nil means "not parsed/not set".

Add `"language"` and `"abridged"` to the `FieldDataSources` key documentation comment.

## Parser Extraction

### EPUB (`pkg/epub/opf.go`)

**Language**: Already parsed from `<dc:language>` into `pkg.Metadata.Language`. Wire it through to `ParsedMetadata.Language`. Normalize to ISO 639-1 (EPUB may contain ISO 639-2/T codes like `"eng"` or BCP 47 tags like `"en-US"` — extract the 2-letter prefix where possible).

**Abridged**: No standard OPF element. Do not extract.

### CBZ (`pkg/cbz/cbz.go`)

**Language**: Already parsed from `<LanguageISO>` into `comicInfo.LanguageISO`. Wire through to `ParsedMetadata.Language`. ComicInfo uses ISO 639-1 natively, so no normalization needed.

**Abridged**: No ComicInfo.xml field. Do not extract.

### M4B (`pkg/mp4/metadata.go`)

**Language**: Extract from one of:
1. Freeform atom `com.apple.iTunes:LANGUAGE` or `com.pilabor.tone:LANGUAGE`
2. Track language from `mdhd` box (media header) — this is ISO 639-2/T, normalize to 639-1

**Abridged**: Extract from freeform atom `com.pilabor.tone:ABRIDGED` (Tone tagger convention). Value `"true"` → `true`, `"false"` → `false`, absent → `nil`.

### PDF (`pkg/pdf/pdf.go`)

**Language**: Extract from pdfcpu's info dict `Language` field if available, or from the document catalog's `Lang` entry. Normalize to ISO 639-1.

**Abridged**: No standard field. Do not extract.

### Language Normalization

Create a shared utility (e.g., `pkg/mediafile/language.go`) that:
1. Validates ISO 639-1 codes against a known list
2. Converts ISO 639-2/T codes (3-letter like `"eng"`) to ISO 639-1 (`"en"`)
3. Extracts the language subtag from BCP 47 tags (`"en-US"` → `"en"`)
4. Returns `nil` for unrecognized values

This utility is also used for API input validation.

## Scanner Integration (`pkg/worker/scan_unified.go`)

In `scanFileCore()`, handle both fields following the existing pattern for fields like URL, release_date, publisher:

1. Check data source priority (manual > sidecar > plugin > file metadata > filepath)
2. Update `file.Language` / `file.Abridged` and their source fields if the new source has equal or higher priority
3. Sidecar values applied with `DataSourceSidecar` priority

## Sidecar Persistence

### FileSidecar (`pkg/sidecar/types.go`)

```go
Language *string `json:"language,omitempty"`
Abridged *bool   `json:"abridged,omitempty"`
```

### Sidecar Conversion (`pkg/sidecar/sidecar.go`)

- `FileSidecarFromModel()`: Copy `file.Language` and `file.Abridged` to sidecar
- Sidecar reading in scanner: Apply language/abridged from sidecar with `DataSourceSidecar` priority

## Edit API

### UpdateFilePayload (`pkg/books/validators.go`)

```go
Language *string `json:"language,omitempty"`
Abridged *string `json:"abridged,omitempty"` // "true", "false", or "" to clear
```

Note: `Abridged` is a `*string` in the payload (not `*bool`) to distinguish between "not sent" (field absent), "clear" (empty string), and "set" (true/false). This follows the nullable-update pattern.

### Handler Logic (`pkg/books/handlers.go`)

**Language**:
- Validate against the ISO 639-1 list using the shared utility
- Empty string clears the value (sets to nil)
- Set source to `DataSourceManual`

**Abridged**:
- Accept `"true"` → `true`, `"false"` → `false`, `""` → nil (clear)
- Set source to `DataSourceManual`

Both fields: after update, write file sidecar and (for language) re-index if language filter needs it.

### Downgrade to Supplement

When a file is downgraded from main to supplement, `abridged` should be cleared (supplements don't carry edition metadata). `language` should be preserved (language is intrinsic to the file content).

## File Generation (Write-Back on Download)

### EPUB (`pkg/filegen/epub.go`)

**Language**: Write `<dc:language>` element in OPF metadata.
**Abridged**: Do not write (no standard element).

### CBZ (`pkg/filegen/cbz.go`)

**Language**: Write `<LanguageISO>` in ComicInfo.xml.
**Abridged**: Do not write (no ComicInfo field).

### M4B (`pkg/filegen/m4b.go`)

**Language**: Write freeform atom `com.pilabor.tone:LANGUAGE`.
**Abridged**: Write freeform atom `com.pilabor.tone:ABRIDGED` as `"true"` or `"false"`. Do not write atom if nil.

### PDF (`pkg/filegen/pdf.go`)

**Language**: Write `Lang` entry in document catalog if pdfcpu supports it.
**Abridged**: Do not write (no standard field).

### KePub (`pkg/kepub/cbz.go`)

**Language**: Carried through from the underlying EPUB/CBZ generation.

## Download Fingerprint (`pkg/downloadcache/fingerprint.go`)

Add `Language *string` and `Abridged *bool` to the `Fingerprint` struct and `ComputeFingerprint()` so the download cache invalidates when these fields change (since they affect generated file content).

## Plugin SDK

### ParsedMetadata (`packages/plugin-sdk/`)

Add to the TypeScript SDK types:

```typescript
language?: string;  // ISO 639-1 code
abridged?: boolean; // true=abridged, false=unabridged, undefined=unknown
```

Plugins can set these fields in file parser results and metadata enricher results.

## Frontend

### FileEditDialog (`app/components/library/FileEditDialog.tsx`)

**Language field**:
- Combobox with client-side search over a static ISO 639-1 list
- Display format: "English (en)", "French (fr)", etc.
- Searchable by both name and code
- Clearable (selecting nothing sets to nil)
- Available for all file types

**Abridged field**:
- Dropdown with three options: "Unknown" (nil), "Unabridged" (false), "Abridged" (true)
- Default: "Unknown"
- Available for all file types

### BookDetail Display (`app/components/pages/BookDetail.tsx`)

Show language (as human-readable name) and abridged status in the file metadata section. Only display if the value is set (non-nil).

### Language Filter on Gallery Page

**Conditional filter**: Only show a language filter dropdown on the gallery/library page when the library has files with 2+ distinct non-null language values.

**Backend support**: Add an endpoint or extend existing library stats to return distinct languages for a library:
- `GET /libraries/:id/languages` → `["en", "fr", "ja"]`
- Or include in existing library metadata response

**Frontend**: Query distinct languages on library page load. If the result has fewer than 2 entries, hide the filter. Otherwise, show a dropdown that filters the book list by language.

**Filter logic**: A book matches the language filter if any of its files have the selected language.

## ISO 639-1 Language List

A static list of ~184 ISO 639-1 codes with English names, stored client-side for the combobox. Also used server-side for validation. Shared as a Go map and a TypeScript constant.

Place the Go list in `pkg/mediafile/language.go` and the TypeScript list in `app/constants/languages.ts` (or similar). The Go map is authoritative; the TS list can be a simple array of `{ code, name }` objects.

## Documentation Updates

- `website/docs/metadata.md`: Document language and abridged as file-level metadata fields
- `website/docs/sidecar-files.md`: Add language and abridged to sidecar format documentation
- `website/docs/plugins/`: Update plugin SDK docs with new ParsedMetadata fields
- `website/docs/supported-formats.md`: Note which formats support language/abridged extraction

## What's NOT Included

- No book-level language or abridged fields (file-level only)
- No FTS indexing of language (it's a filter, not a text search field)
- No file reorganization triggered by language/abridged changes
- No OPDS language filtering (can be added later)
- No eReader browser language filtering (can be added later)
