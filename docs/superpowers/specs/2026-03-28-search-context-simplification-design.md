# Search Context Simplification

**Date:** 2026-03-28
**Status:** Approved
**Scope:** `pkg/plugins/`, `pkg/worker/`, `app/components/library/`, `packages/plugin-sdk/`, `website/docs/plugins/`

## Problem

The plugin search hook receives a rich `context.book` object (title, authors, series, identifiers, etc.) and `context.file` (fileType, filePath), but the identify dialog only lets users edit the query string. If the book's authors or identifiers are wrong, plugins use that bad data and the user has no way to correct it. The mismatch between what's in the search context and what the user can control makes debugging scan results difficult — the same search produces different results in identify vs. scan because the contexts differ.

## Design

### New SearchContext

Replace the current nested `{ query, book: {...}, file: {...} }` with a flat structure:

```typescript
interface SearchContext {
  /** Search query — title or free-text. Always present. */
  query: string;
  /** Author name to narrow results. Optional. */
  author?: string;
  /** Structured identifiers for direct lookup (ISBN, ASIN, etc.). Optional. */
  identifiers?: Array<{ type: string; value: string }>;
}
```

- `context.book` and `context.file` are removed entirely
- Same shape for both interactive identify and automatic scan enrichment
- Three clear search vectors: free-text query, author name, structured identifiers

### Identify Dialog Changes

The dialog gains two new controls below the existing query input:

**Author field:**
- Optional text input, pre-filled with the book's first author name
- User can edit or clear it
- Sent as `author` in the search payload

**Identifier chips:**
- Pre-filled from the book's current identifiers (collected from all files)
- Each displayed as a dismissable chip: `ISBN-13: 9781234567890 [x]`
- Uses existing `formatIdentifierType()` for label formatting
- Not editable — just clearable individually
- If all are cleared, `identifiers` is omitted from the payload
- Users who want to search by a specific ISBN paste it into the query field
- Must use the same visual treatment as the identifier display on the review step (IdentifyReviewForm) — both steps show identifiers as read-only dismissable chips with formatted type labels

### API Changes

**Search payload** (`searchPayload` in handler.go):

```go
type searchPayload struct {
    Query       string                       `json:"query" validate:"required"`
    BookID      int                          `json:"book_id" validate:"required"`
    Author      string                       `json:"author"`
    Identifiers []mediafile.ParsedIdentifier `json:"identifiers"`
}
```

`BookID` is still needed so the server knows which book to apply metadata to. But the search context sent to plugins is built from the payload fields, not from loading the book's database state.

### Backend Changes

**searchMetadata handler:**
- Builds `SearchContext` from the payload directly: `{ query: payload.Query, author: payload.Author, identifiers: payload.Identifiers }`
- No longer calls `buildSearchBookContext()` — that function is deleted
- Still loads the book (needed for library access check and disabled fields computation), but doesn't pass book data to plugins

**Scan pipeline (`runMetadataEnrichers`):**
- Builds the same flat context: query from title, author from first author name, identifiers from file metadata
- `buildBookContext()` and `buildFileContext()` deleted (only used in this flow)
- The search context shape matches what the interactive flow sends

### Plugin SDK Changes

**`packages/plugin-sdk/hooks.d.ts`:**
- `SearchContext` updated to the new flat shape
- `context.book` and `context.file` removed

**Plugin migration (pre-prod, no backwards compat needed):**
- `context.book.title` → `context.query`
- `context.book.authors[0].name` → `context.author`
- `context.book.identifiers` → `context.identifiers`
- `context.file.fileType` → removed (plugins that need file type should declare `fileTypes` in their manifest's `metadataEnricher` capability, which already filters by file type before the hook is called)

### Documentation Changes

- `website/docs/plugins/development.md` — update SearchContext docs and examples
- `pkg/plugins/CLAUDE.md` — update hook documentation and scan pipeline section

## Files Affected

### Go (modify)
- `pkg/plugins/handler.go` — update `searchPayload`, rewrite `searchMetadata` to build flat context, delete `buildSearchBookContext()`
- `pkg/worker/scan_unified.go` — update `runMetadataEnrichers` to build flat context, delete `buildBookContext()` and `buildFileContext()`
- `pkg/plugins/hooks_test.go` — update tests that build search contexts

### Frontend (modify)
- `app/components/library/IdentifyBookDialog.tsx` — add author input and identifier chips, update search mutation payload
- `app/hooks/queries/plugins.ts` — update `usePluginSearch` mutation payload type

### TypeScript SDK (modify)
- `packages/plugin-sdk/hooks.d.ts` — update `SearchContext` interface

### Test data (modify)
- `pkg/plugins/testdata/hooks-enricher/main.js` — update to use new context shape

### Docs (modify)
- `website/docs/plugins/development.md` — update SearchContext docs
- `pkg/plugins/CLAUDE.md` — update hook docs and scan pipeline section
