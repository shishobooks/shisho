# EPUB Reader Design

## Summary

Add an in-app EPUB reader, matching the role that `PageReader` plays for CBZ and PDF files. EPUBs are reflowable HTML/CSS documents, so they need a different rendering stack than the page-image model used by CBZ and PDF. This design uses [foliate-js](https://github.com/johnfactotum/foliate-js) for rendering, serves EPUB bytes via the existing `/api/books/files/:id/download` endpoint, and introduces three new EPUB-specific viewer settings (font size, theme, flow).

Reading progress persistence, annotations, highlights, and in-book search are explicit non-goals for v1.

## Motivation

Shisho already lets users open CBZ and PDF files in a browser-based reader (`PageReader`). EPUBs currently fall through to a "Reading is not supported for this file type" message in `FileReader.tsx`. Users who want to read EPUBs must download them and open them in another app. Closing this gap makes EPUB a first-class format for in-app reading.

## Rendering library: foliate-js

We evaluated epub.js and foliate-js. Both are read-only rendering libraries; neither supports structural editing of EPUBs (editing is a separate, backend-side concern and not in scope here).

foliate-js was chosen for three reasons:

- **Modular, multi-format toolkit.** Separate parser/renderer modules for EPUB, MOBI/KF8, FB2, CBZ, and PDF. A future MOBI-support task and a potential consolidation of the CBZ/PDF readers both become realistic.
- **CFI position tracking.** foliate-js emits a `relocate` event on every page turn containing a CFI, a 0–1 progress fraction, and the current TOC item. This is the same W3C standard epub.js uses and is the foundation we'd build reading-progress persistence on later.
- **Modern ESM, no build step.** Distributed as plain ES modules; fits Vite's native-ESM pipeline without additional tooling.

Tradeoff: smaller community than epub.js, so we'll read source more than docs. The codebase is small and readable, so this is acceptable.

## Architecture

### Component structure

- New component: `app/components/pages/EPUBReader.tsx`.
- Wired into `app/components/pages/FileReader.tsx` by adding a `case FileTypeEPUB:` branch that renders `<EPUBReader file={file} bookTitle={book?.title} libraryId={libraryId} />`.
- `EPUBReader` is a sibling of `PageReader`, not a wrapper around it. `PageReader` assumes an integer page model (`currentPage`, `getPageUrl(n)`, `preload_count`) that does not map cleanly onto reflowable content. Shared chrome can be extracted later if it proves useful, but no speculative abstraction in v1.

### Library distribution

foliate-js has no official bundled npm release. We vendor the needed modules into `app/libraries/foliate/`:

- The EPUB parser and its dependencies (ZIP reader, OPF/nav parsers)
- The `<foliate-view>` renderer custom element and its dependencies

Estimated surface: ~20 files. The project's Vite setup already supports no-build ESM imports, so this is a straightforward drop-in.

### EPUB delivery

Fetch from **`/api/books/files/:id/download`**. This endpoint regenerates the EPUB on the fly via `downloadCache.GetOrGenerate`, embedding the current cover and metadata into the EPUB before serving.

Why not `/download/original`: the raw on-disk EPUB retains its *original* embedded cover and metadata. Custom covers are stored as sibling files (`{filename}.cover.{ext}`) and are only baked into the EPUB at generation time. Using `/original` would surface a visible correctness bug — the reader would show the pre-upload cover on the title/first-page spread.

Why not `/download/kepub`: KePub conversion only happens when the frontend explicitly routes to `/download/kepub` based on the library's `download_format_preference`. The `/download` endpoint always returns a plain EPUB for EPUB files, regardless of library settings. Library preference has no effect on the reader.

Tradeoffs:

- **First open is slow** (generation time scales with EPUB size). Mitigated by the loading UX below.
- **Subsequent opens are fast.** `downloadCache` is fingerprint-keyed on book + file metadata; unchanged books hit cache.
- **After metadata edits,** the cache invalidates and the next open regenerates once. Correct behavior.

Frontend fetches the response as a `Blob`, wraps it in a `File`, and hands it to foliate's `View.open()` method. The fetch is wrapped in a React Query hook (`useEpubBlob(fileId)`) so re-entering the reader during the same session is instant. Default `staleTime` of 5 minutes is fine.

### UI chrome

Layout matches `PageReader` for consistency:

**Header.** `← Back` link returning to the book detail page. TOC dropdown populated from foliate's parsed TOC (not from our DB chapters — foliate's TOC entries carry the internal refs foliate needs to navigate, and foliate preserves nested structure natively). Settings popover trigger.

**Main.** The `<foliate-view>` custom element fills the content area. Left-third and right-third transparent tap zones for prev/next page (mirrors `PageReader`). Keyboard bindings: `ArrowLeft`/`a`/`A` for prev, `ArrowRight`/`d`/`D` for next — identical to `PageReader`.

**Footer.** Progress bar driven by the `relocate` event's `fraction` field (0–1). Clicking the bar calls foliate's `goToFraction`. Above the bar, show the current section title from the `relocate` event's `tocItem`. A page-N-of-M counter does not apply to reflowable content; display `Math.round(fraction * 100)%` instead.

### Settings

Extend the existing `viewer_settings` model (`pkg/settings/` and `app/hooks/queries/settings.ts`) with three EPUB-specific fields:

| Field | Type | Default | Range |
|-------|------|---------|-------|
| `epub_font_size` | int (percentage) | 100 | 50–200 |
| `epub_theme` | string enum | `"light"` | `"light" \| "dark" \| "sepia"` |
| `epub_flow` | string enum | `"paginated"` | `"paginated" \| "scrolled"` |

Rationale for extending `viewer_settings` rather than introducing a new table: the existing model is a per-user key-value bag and already holds reader-adjacent settings (`preload_count`, `fit_mode`). One model means one migration, one hook, one API surface.

**Settings popover contents:**

- Font-size slider, 50%–200%, step 10.
- Three theme buttons (Light / Dark / Sepia), matching the button-group pattern used by `PageReader`'s fit-mode selector.
- Two flow buttons (Paginated / Scrolled).

Settings apply live without remounting the view: foliate-view exposes `renderer.setStyles({ fontSize, theme })` and a `flow` property that accept updates on an already-loaded book.

### Loading and error UX

The slow path is server-side generation on first open; client-side parse is comparatively fast. The spinner label is framed accordingly.

**Loading state.** Full-screen centered spinner with label `Preparing book…`, visible from mount until foliate's first `relocate` event fires. No fake progress bar — we do not have reliable progress for either generation or parse.

**Extended wait hint.** If the loading state persists past 10 seconds, add a secondary line underneath the spinner: `This may take a moment for large books`. Worst case the hint never appears; on genuinely slow first opens it reassures the user that something is still happening.

**Error state.** If the fetch fails or foliate throws during parse, replace the spinner with a centered error card containing the message and a `Retry` button that re-triggers the fetch. Mirror the visual style of existing error cards in the codebase (`app/components/common/`).

## Out of scope for v1

Explicit non-goals, documented here so future scope creep has a clear answer:

- Reading progress persistence (CFI save/restore across sessions).
- Annotations, highlights, bookmarks.
- Text search within a book.
- Custom font files or themes beyond the three built-ins.
- Structural editing of EPUBs (reorder chapters, rename sections, edit embedded metadata from the reader). This is a backend-side feature and orthogonal to the reader.
- OPDS or Kobo integration changes. This reader is in-app only.
- MOBI / KF8 / FB2 support. foliate-js supports them; we'll add the file types in a follow-up.
- Consolidating CBZ and PDF readers onto foliate-js. Possible later given the shared toolkit, but not v1.

## Documentation

- Update `website/docs/supported-formats.md` to note EPUB is now supported for in-app reading (currently implies download-only).

No new config options, permissions, or API endpoints, so no other user-facing documentation touchpoints.

## Testing

- Unit tests for any pure helpers introduced (e.g., mapping settings state to foliate's style/flow inputs).
- Component tests for `EPUBReader` mount, loading state, error state, and retry behavior. The foliate-view element itself can be mocked; we're not testing foliate's rendering.
- Manual verification in the browser with a small EPUB (first-open generation), a large EPUB (extended-wait hint), and an EPUB with a custom cover (correct cover renders).
- E2E coverage is deferred — the existing `e2e/ereader.spec.ts` covers the stock-browser ereader UI, not the in-app reader. An in-app reader E2E can be added alongside CBZ/PDF coverage when that coverage is built out.

## Implementation order (high level)

1. Vendor foliate-js modules into `app/libraries/foliate/`.
2. Add `epub_font_size`, `epub_theme`, `epub_flow` to `viewer_settings` (migration + Go model + TypeScript types via `mise tygo`).
3. Write the `useEpubBlob(fileId)` React Query hook against `/api/books/files/:id/download`.
4. Implement `EPUBReader.tsx`: mount foliate-view, wire TOC dropdown, progress bar, keyboard/tap nav, settings popover, loading/error/retry states, extended-wait hint.
5. Add the `FileTypeEPUB` branch to `FileReader.tsx`.
6. Update `website/docs/supported-formats.md`.
7. Run `mise check:quiet`.
