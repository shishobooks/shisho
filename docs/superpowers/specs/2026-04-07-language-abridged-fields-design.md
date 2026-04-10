# Language and Abridged Fields

**Date:** 2026-04-07
**Status:** Approved

## Problem

Files in Shisho have no way to track language or abridged status. Both are common metadata fields:
- **Language** is embedded in EPUB (`<dc:language>`), CBZ (`LanguageISO`), and available in M4B/PDF, but parsers currently discard these values since there's no field to store them.
- **Abridged** status is meaningful for all file types (abridged editions exist for print and audio), but has no standard embedded metadata in most formats. M4B can store it via Tone-style freeform atoms.

Users want to see, edit, and filter by these fields. Plugins should be able to set them during parsing and enrichment.

## Solution

Add two new nullable fields to the `files` table — `language` (BCP 47 tag) and `abridged` (nullable boolean) — with full stack support: parsing, editing, sidecar persistence, plugin SDK, file generation, and UI filtering.

## Data Model

### Database Migration

New columns on `files`:

| Column | Type | Description |
|--------|------|-------------|
| `language` | `TEXT` | BCP 47 tag (e.g., `"en"`, `"en-US"`, `"zh-Hans"`), nullable |
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
Language *string `json:"language,omitempty"` // BCP 47 tag
Abridged *bool   `json:"abridged,omitempty"`
```

Both are pointer types: nil means "not parsed/not set".

Add `"language"` and `"abridged"` to the `FieldDataSources` key documentation comment.

## Parser Extraction

### EPUB (`pkg/epub/opf.go`)

**Language**: Already parsed from `<dc:language>` into `pkg.Metadata.Language`. Wire it through to `ParsedMetadata.Language`. EPUB 3 uses BCP 47 natively, so pass through as-is. For EPUB 2 files that may use ISO 639-2/T codes (e.g., `"eng"`), normalize to the BCP 47 equivalent (`"en"`).

**Abridged**: No standard OPF element. Do not extract.

### CBZ (`pkg/cbz/cbz.go`)

**Language**: Already parsed from `<LanguageISO>` into `comicInfo.LanguageISO`. Wire through to `ParsedMetadata.Language`. ComicInfo uses ISO 639-1 natively, which is valid BCP 47 — no normalization needed.

**Abridged**: No ComicInfo.xml field. Do not extract.

### M4B (`pkg/mp4/metadata.go`)

**Language**: Extract from one of:
1. Freeform atom `com.apple.iTunes:LANGUAGE` or `com.pilabor.tone:LANGUAGE`
2. Track language from `mdhd` box (media header) — this is ISO 639-2/T, normalize to 639-1

**Abridged**: Extract from freeform atom `com.pilabor.tone:ABRIDGED` (Tone tagger convention). Value `"true"` → `true`, `"false"` → `false`, absent → `nil`.

### PDF (`pkg/pdf/pdf.go`)

**Language**: Extract from pdfcpu's info dict `Language` field if available, or from the document catalog's `Lang` entry. Normalize to ISO 639-1.

**Abridged**: No standard field. Do not extract.

### Language Validation and Normalization

Create a shared utility (e.g., `pkg/mediafile/language.go`) using `golang.org/x/text/language` that:
1. Parses and validates BCP 47 tags via `language.Parse()` — rejects structurally invalid tags
2. Converts ISO 639-2/T codes (3-letter like `"eng"`) to their BCP 47 equivalent (`"en"`) for EPUB 2 compatibility
3. Canonicalizes tags (e.g., `"EN-us"` → `"en-US"`)
4. Returns `nil` for unrecognized/invalid values

This utility is used by all parsers (to normalize extracted values) and by the API (to validate user input and custom free-text tags).

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
- Validate using `language.Parse()` from the shared utility — accepts any valid BCP 47 tag
- Canonicalize the tag before storing (e.g., `"en-us"` → `"en-US"`)
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
language?: string;  // BCP 47 tag (e.g., "en", "en-US", "zh-Hans")
abridged?: boolean; // true=abridged, false=unabridged, undefined=unknown
```

Plugins can set these fields in file parser results and metadata enricher results.

## Frontend

### FileEditDialog (`app/components/library/FileEditDialog.tsx`)

**Language field**:
- Combobox with client-side search over a merged list: the curated common tags PLUS any languages already in use in the library (from the `GET /libraries/:id/languages` endpoint). This ensures that if someone sets a non-standard tag like `oc` (Occitan) on one file, it appears as a selectable option for all other files without re-typing.
- Display format: "English (en)", "English - United States (en-US)", "Chinese - Simplified (zh-Hans)", etc. Tags from the library that aren't in the curated list show as just the tag code (e.g., "oc") since we don't have a display name for them.
- Searchable by both name and code
- **Free-text fallback**: If the user types a tag not in the merged list, show a "Use custom tag: {input}" option (similar to publisher/imprint "Create: ..." pattern). Backend validates the tag structure via `language.Parse()`.
- Clearable (selecting nothing sets to nil)
- Available for all file types

**Abridged field**:
- Single checkbox: "This is an abridged edition"
- Checked = `true`, unchecked = clear (nil). The UI does not expose an explicit `false` state — the data model still supports it (parsers and plugins can set it), but the common case is "most books are unabridged, only mark abridged ones" so a binary opt-in is less noisy than a three-state dropdown.
- At initialization, both `false` and `nil` are normalized to unchecked so that accidentally toggling the checkbox on a file where a plugin set explicit `false` doesn't clobber the value. Only an explicit check (→ `true`) or check-then-uncheck on a previously-true file (→ clear) results in a write.
- Available for all file types.

### BookDetail Display (`app/components/pages/BookDetail.tsx`)

Show language (as human-readable name) and abridged status in the file metadata section.

- **Language**: Only display if set (non-nil).
- **Abridged**: For M4B files, always show the row (audiobooks historically had abridged versions, so the distinction is meaningful at a glance). For other file types, only show when explicitly `true`. In both cases, a nil value is rendered as "Unknown", not "Unabridged".

### FileDetailsTab Display (`app/components/files/FileDetailsTab.tsx`)

The dedicated file details page enumerates every field, so Language and Abridged are always shown for all file types. Nil values render as "Unknown".

### Language Filter on Gallery Page

**Conditional filter**: Only show a language filter dropdown on the gallery/library page when the library has files with 2+ distinct non-null language values.

**Backend support**: Add an endpoint or extend existing library stats to return distinct languages for a library:
- `GET /libraries/:id/languages` → `["en", "en-US", "fr", "ja"]`
- Or include in existing library metadata response

**Frontend**: Query distinct languages on library page load. If the result has fewer than 2 entries, hide the filter. Otherwise, show a dropdown that filters the book list by language.

**Filter grouping**: Group by base language subtag in the filter dropdown. If a library has both `en` and `en-US`, show a single "English" option that matches both. If it has `en-US` and `en-GB` (but no bare `en`), show separate "English - United States" and "English - United Kingdom" options since the user distinguished them.

**Filter logic**: A book matches the language filter if any of its files have the selected language (or a language with the same base subtag, when grouped).

## Curated BCP 47 Language List

A curated list of ~200-300 common BCP 47 tags with English display names, used client-side for the combobox. Includes:
- All ISO 639-1 languages (the base ~184 codes like `en`, `fr`, `ja`)
- Common script variants (`zh-Hans`, `zh-Hant`, `sr-Latn`, `sr-Cyrl`)
- Common regional variants where the distinction matters for books (`en-US`, `en-GB`, `pt-BR`, `pt-PT`, `es-419`, `fr-CA`)

**Server-side validation** uses `golang.org/x/text/language` to validate any BCP 47 tag — it is NOT restricted to the curated list. The curated list is purely a UI convenience.

Place the curated list in `app/constants/languages.ts` as an array of `{ tag, name }` objects. The Go side does not need the list since it validates structurally via `language.Parse()`.

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
