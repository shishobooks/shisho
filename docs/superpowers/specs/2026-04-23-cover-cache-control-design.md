# Cover image cache control — design

**Date:** 2026-04-23
**Branch:** `task/series-cache-busting`
**Status:** Design

## Problem

Cover images displayed in the React frontend sometimes fail to update after the underlying cover is replaced on the server. A user updating a cover from the book detail page saw the new cover there, but the series detail page kept showing the old one — even after hard refresh — unless DevTools had "Disable cache" enabled.

### Root cause

Two overlapping issues in the current cache-busting scheme:

1. **Stale URL cache buster.** Cover URLs in the frontend include a `?t=<timestamp>` query parameter derived from the owning entity's `updated_at` field (e.g., `book.updated_at`, `series.updated_at`). Cover mutations on the backend (`uploadFileCover`, `updateFileCoverPage`, and scan-path cover regeneration in `recoverMissingCover` / `applyPageCover` / the reset-extract block) update the `file` row but never bump `book.updated_at` or `series.updated_at`. So the `?t=` value never changes, the URL stays identical, and the browser serves the cached image — even after a full page refresh, because the server response still sets `Cache-Control: public, max-age=86400` on series covers and relies on heuristic caching on book/file covers.
2. **Missing query invalidation.** The `SeriesBooks` query isn't invalidated when a book cover is uploaded, so even soft-nav to the series detail page returns stale data. Secondary issue — would be masked on reload.

### Why the obvious fix is wrong

Bumping `book.updated_at` from every cover-writing code path would work, but it (a) conflates "book row changed" with "file row changed", (b) requires touching six call sites in services/handlers/scan plus their tests, and (c) still leaves the system dependent on correct timestamp propagation for cache correctness. That's a fragile design that has already produced this class of bug.

## Goals

- Cover updates become immediately visible across all views without cache-buster query parameters.
- Mechanism is HTTP-native (conditional requests) rather than application-level timestamp tracking.
- Works in dev (Vite) and prod (Caddy reverse proxy) with no infrastructure changes.
- Frontend code becomes simpler: no cache buster props, no timestamp passing, stable cover URLs.

## Non-goals

- Changing cover storage or generation logic on the server.
- Changing query invalidation patterns beyond what the design requires.
- Modifying Kobo or eReader device sync behavior beyond updating their cover endpoints to use the same caching model.
- User-facing documentation (`website/docs/`) — this is an internal implementation change with no observable API/config surface.

## Design

### Backend — `Cache-Control: private, no-cache` + Last-Modified

All cover endpoints set `Cache-Control: private, no-cache` on every response. This tells the browser to store the image but revalidate with the server before each use, via a conditional `GET` carrying `If-Modified-Since: <last mtime>`. The server returns `304 Not Modified` (a few hundred bytes, no body) when the cover file on disk hasn't changed, or `200 OK` with the new image bytes when it has.

No explicit `ETag` is used. Echo's `c.File()` delegates to `http.ServeFile` → `http.ServeContent`, which already emits `Last-Modified` from the file's mtime and handles `If-Modified-Since` automatically. A hypothetical `ETag` built from mtime+size would duplicate `Last-Modified` with no added precision; content hashing on every request is too expensive to justify.

`private` ensures shared proxies (CDNs, ISP caches) don't store these responses, which is required because cover endpoints are session-authenticated.

#### Endpoint-by-endpoint changes

| Handler | File:line | Current `Cache-Control` | New | Notes |
|---|---|---|---|---|
| `bookCover` | `pkg/books/handlers.go:1466` | _(none)_ | `private, no-cache` | Uses `c.File()` — `Last-Modified` + conditional GET handled automatically |
| `fileCover` | `pkg/books/handlers.go:1269` | _(none)_ | `private, no-cache` | Same as above |
| `seriesCover` | `pkg/series/handlers.go` (header set at `:276`) | `public, max-age=86400` | `private, no-cache` | Same as above |
| eReader `Cover` | `pkg/ereader/handlers.go:692` | _(none)_ | `private, no-cache` | Uses `c.File()` — same free handling |
| Kobo resized cover | `pkg/kobo/handlers.go:285` | `public, max-age=86400` | `private, no-cache` | **Manual 304 handling required** — see below |

#### Kobo special case

The Kobo handler reads the cover file, decodes, resizes, and re-encodes into JPEG on every request, writing directly to `c.Response().Writer`. It does not use `c.File()`, so `http.ServeContent`'s automatic `Last-Modified` / `If-Modified-Since` machinery is not involved. To get conditional-request behavior without doing the expensive resize work on every request:

1. Stat the source cover file to get its mtime.
2. Set `Last-Modified: <formatted mtime>` on the response.
3. Parse `If-Modified-Since` from the request. If the request's timestamp is ≥ the source mtime, write `304 Not Modified` and return without doing any image work.
4. Otherwise, proceed with the resize/encode as today.

This is standard `net/http` behavior and can be implemented in ~10 lines.

### Frontend — stable URLs + remount-on-mutation

Cover URLs become stable — no query parameters:

- `` `/api/books/${id}/cover` ``
- `` `/api/books/files/${id}/cover` ``
- `` `/api/series/${id}/cover` ``

#### Call sites updated

Nine cover-URL construction sites drop `?t=...`:

| File | Line(s) |
|---|---|
| `app/components/library/BookItem.tsx` | 154 |
| `app/components/pages/SeriesList.tsx` | 53 |
| `app/components/library/GlobalSearch.tsx` | 62 |
| `app/components/pages/BookDetail.tsx` | 813 |
| `app/components/library/FileCoverThumbnail.tsx` | 33-35 |
| `app/components/library/FileEditDialog.tsx` | 753, 780 |
| `app/components/library/CoverGalleryTabs.tsx` | 84-86 |
| `app/components/library/IdentifyReviewForm.tsx` | 617 |

#### Prop/state cleanup

Remove `cacheBuster` props and associated state:

- `FileCoverThumbnail`: drop `cacheBuster?: number` prop.
- `CoverGalleryTabs`: drop `cacheBuster?: number` prop.
- `GlobalSearch` (internal `GlobalSearchCoverImage`): drop `cacheBuster: number` prop.
- `BookDetail.tsx`: drop `const coverCacheBuster = bookQuery.dataUpdatedAt;` and its four call-site uses (lines 238, 1073, 1355, 1406).

#### Remount pattern for same-page updates

When the user triggers a cover mutation on the same page where a cover is displayed (e.g., uploading from `BookDetail`), the `<img>` is already mounted with the stable URL. Invalidating a query doesn't change the `src` string, so React reconciliation leaves the DOM node alone, the browser doesn't refetch, and the user sees the old cover.

The fix: add `key={<query>.dataUpdatedAt}` to the `<img>` element. When the relevant query refetches after invalidation, `dataUpdatedAt` changes, the key changes, React unmounts and remounts the element, and the browser makes a fresh HTTP request — which triggers the `Last-Modified` revalidation flow and pulls in the new bytes.

Components that need the key:

| Component | Key source |
|---|---|
| `BookDetail.tsx` — main book `<img>` | `bookQuery.dataUpdatedAt` |
| `CoverGalleryTabs.tsx` — selected-file `<img>` | `bookQuery.dataUpdatedAt` (passed as prop) |
| `FileCoverThumbnail.tsx` — used inside `FileEditDialog` | `bookQuery.dataUpdatedAt` (passed as prop) |
| `FileEditDialog.tsx` — the two inline `<img>`s (753, 780) | `bookQuery.dataUpdatedAt` |

Components that display covers but can't trigger cover mutations from their own page do not need a key — navigating to them naturally remounts the `<img>`, which revalidates via `Last-Modified`. This includes `SeriesList`, `SeriesDetail`, `GlobalSearch`, `BookItem`, `IdentifyReviewForm`, and `Home`.

#### Query invalidation

No new invalidations are required for cover freshness. The existing `useUploadFileCover` and `useSetFileCoverPage` mutations invalidate `ListBooks` and `RetrieveBook`, which is sufficient to drive `bookQuery.dataUpdatedAt` on the pages that need the remount key. `SeriesBooks` invalidation is **not** needed — the stable URL plus browser-level revalidation handles staleness when the user navigates to the series view.

#### `coverCache` utility

The `isCoverLoaded` / `markCoverLoaded` utility in `app/utils/coverCache.ts` is kept as-is. Its purpose (suppress placeholder flicker when the same URL was previously loaded) is unaffected by the change — if anything, stable URLs make it more reliable, since the same URL is shared across all views.

### Infrastructure — Caddy

No changes. The production Caddyfile is a pure reverse proxy: `uri strip_prefix /api` + `reverse_proxy localhost:3689`. It does not add caching, strip headers, or interfere with conditional requests. `Cache-Control`, `Last-Modified`, `If-Modified-Since`, and `304 Not Modified` pass through unchanged. `encode gzip zstd` is present but will not meaningfully compress already-compressed JPEG/PNG bodies and does not affect headers.

### Dev mode (Vite)

Vite's dev proxy passes `/api/*` through to the Go backend and forwards response headers verbatim. No dev-mode changes required.

## Testing

### Backend

Red-green tests for each endpoint, following the project's TDD convention (`pkg/CLAUDE.md`):

1. **200 on fresh request** — `GET /api/books/:id/cover` with no `If-Modified-Since`. Assert status 200, body is the cover bytes, `Last-Modified` header present, `Cache-Control: private, no-cache` set.
2. **304 when unchanged** — `GET` with `If-Modified-Since: <Last-Modified from test 1>`. Assert status 304, empty body.

Coverage per endpoint:

- `pkg/books/handlers_cover_test.go` — `bookCover` and `fileCover`
- `pkg/series/handlers_test.go` — `seriesCover`
- `pkg/ereader/handlers_test.go` — eReader `Cover` handler
- `pkg/kobo/handlers_test.go` — resized-cover handler, including verification that the resize work is skipped when 304 is returned (not just that the response status is correct). The exact assertion mechanism depends on handler structure; likely a counter or a sentinel on the resize path.

### Frontend

- Update `app/components/library/CoverGalleryTabs.test.tsx` — the existing tests that assert on `?t=111` / `?t=222` URLs become assertions on the stable URL with no query string.
- Light smoke tests for `BookItem`, `SeriesList`, and `GlobalSearchCoverImage` cover URL shape (matches `/api/.../cover` exactly).
- No test for `<img key={dataUpdatedAt}>` itself — that's React semantics, not our logic.

### Manual verification (implementation-plan checklist, not part of the suite)

1. Upload a new cover on `BookDetail` → cover updates on that page without a page refresh.
2. Navigate to `SeriesList` / `SeriesDetail` / `GlobalSearch` → new cover shows without cache-buster URL.
3. DevTools Network tab on revisit → cover requests return `304 Not Modified` with ~few hundred bytes, not `200` with full payload.
4. Behind Caddy (prod mode) → same Network-tab check confirms `Cache-Control`, `Last-Modified`, and `304` responses pass through untouched.

### Not tested

- Browser cache behavior under `Cache-Control: no-cache` (trusting the HTTP spec).
- Kobo device behavior with the new headers (can't easily test against real hardware; relying on standard-library HTTP client conformance).

## Documentation updates

### `CLAUDE.md` (project root, "Critical Gotchas")

Current `CLAUDE.md:86-92` reads:

> **Cover images need cache busting** — All cover image URLs must include a `?t=` parameter to ensure updated covers display without caching issues:
> ```
> const coverUrl = `/api/books/${id}/cover?t=${query.dataUpdatedAt}`;
> ```

Replace with:

> **Cover images use HTTP revalidation, not URL cache-busting** — Cover endpoints set `Cache-Control: private, no-cache` and rely on `Last-Modified` for revalidation. Never append a `?t=...` query param to cover URLs. For same-page updates (e.g., uploading a new cover on BookDetail), add `key={query.dataUpdatedAt}` to the `<img>` so React remounts it and the browser refetches.

### `app/CLAUDE.md` (frontend, "Cover Image Cache Busting" section at 296-340)

Rewrite the whole section. New title: **"Cover Image Freshness"**.

Contents:
- **Why:** Browsers cache images; we need updates to show without hard refresh.
- **How:** Backend sets `Cache-Control: private, no-cache` plus `Last-Modified` (via Echo `c.File()`). Browser revalidates on every `<img>` mount; server returns `304 Not Modified` if unchanged, `200 OK` with new bytes if changed.
- **Don't:** Add `?t=...` query params to cover URLs. They are not needed and create URL-variant cache pollution.
- **Do:** For pages that display a cover AND can trigger a mutation that changes it, add `key={query.dataUpdatedAt}` to the `<img>` tag so React remounts on refetch. The remount forces the browser to make a fresh HTTP request, which triggers the `Last-Modified` revalidation flow.
- **Endpoints:** Table of `/api/books/:id/cover`, `/api/books/files/:id/cover`, `/api/series/:id/cover` (same as current section, minus the cache-buster column).
- **New checklist:**
  - [ ] Cover URL does NOT include `?t=` query param.
  - [ ] For pages that mutate covers, `<img>` has `key={query.dataUpdatedAt}` so remount triggers revalidation.
  - [ ] Cover-mutating mutation invalidates the query whose `dataUpdatedAt` drives the key (default: `RetrieveBook` via `useUploadFileCover` / `useSetFileCoverPage` — already correct, just verify for new mutations).

### `CHANGELOG.md`

No retroactive edits — existing entries at lines 357 and 473 are release snapshots and should not be rewritten. A new entry for this change will be added under a `[Backend]` or `[Fix]` category when the commit lands, per normal release flow.

### Not updated

- `website/docs/` — no user-facing behavior or config changes.
- `docs/plans/2026-01-21-cbz-cover-page-selection-*.md` and `docs/plans/2026-01-14-placeholder-covers-implementation.md` — historical plan documents; they reflect design decisions at the time of writing and should not be retroactively edited.

## Open questions

None. Ready for implementation plan.

---

## Errata (post-merge)

The original design assumed `Cache-Control: private, no-cache` + `Last-Modified` would force browsers to revalidate on every `<img>` mount with the same URL. **This was incorrect.** Chromium and Firefox maintain an in-memory image cache separate from the HTTP cache; when an `<img>` element's src matches a URL previously rendered, the browser serves the cached decoded bitmap without consulting HTTP.

The corrective design:
- Cover URLs include `?v=${cacheKey}` where `cacheKey` bumps on data refetches (typically `query.dataUpdatedAt` or `file.updated_at`).
- Backend `Cache-Control: private, no-cache` + `Last-Modified` retained as defense-in-depth and for WebKit clients, which do revalidate correctly.
- React `<img key>` remount pattern retained (still useful for same-page mutation flows).

See `app/CLAUDE.md` "Cover Image Freshness" section for authoritative current documentation and references to the relevant browser bug reports.
