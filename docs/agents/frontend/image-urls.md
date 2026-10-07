# Cover, page, download, and stream URLs

Read this before rendering a cover or page image, adding a cover-mutating action, or building a download or stream link.

## Why every image URL carries `?v=`

Cover and page endpoints are served `private, max-age=31536000, immutable`, and browsers also keep an in-memory image cache (the HTML spec's "list of available images") separate from the HTTP cache: an `<img>` whose `src` matches a URL already rendered this session gets the cached bitmap with no request at all. The only way to show a new image is a new URL, so every URL carries a key that changes exactly when the image does.

## Covers

Build every cover URL with a helper from `app/utils/coverUrl.ts`, each of which documents its key. `fileCoverUploadUrl` is the one without a key: it is a POST target, so never render it.

- **Pass the model, not a key.** Components that render a cover take the book, series, or file and call the helper (`BookItem`, `SeriesCard`, `FileCoverThumbnail`, `CoverGalleryTabs`, `M4BReader`).
- **Render server covers with `CoverImage`** (`app/components/library/CoverImage.tsx`), passing the base URL from a helper; it measures its box and requests a sized thumbnail. Size tiers, aspects, and the render key come from Go via `app/types/generated/covers.ts`; never duplicate them in TS.
- **Use a plain `<img>` for images that are not server covers**: local upload previews, provider image previews, and CBZ/PDF pages.
- **Key `CoverImage` on the base URL** (`key={coverUrl}`) where the cover can change while mounted, so React remounts it and a failed load does not stick. Loading-state keys also use the base URL; `CoverImage` builds the sized request URL.
- Tests that render cover components call `mockCoverDimensions()` from `app/testing/coverDimensions.ts`, because jsdom computes no layout.
- A cover-mutating mutation invalidates the query whose data drives the key.
- Sources without a key: search results carry no `cover_cache_key`, so `GlobalSearch` uses `String(searchQuery.dataUpdatedAt)`; `FileEditDialog` uses the time of its last cover mutation so the preview refreshes before the parent refetches.

## Page images

In a component, `const filePageUrl = useFilePageUrl()` (`app/hooks/useFilePageUrl.ts`) and build every page URL with it. It wraps `filePageUrl(file, page, pdfRenderKey)` from `app/utils/pageUrl.ts`, which appends `?v=` with `fileCacheKey(file)` and, for PDFs, `&r=` with the server's render key (`pdf_render_key` from `GET /auth/status`, in the auth context as `pdfRenderKey`). A URL without the keys shows old pages for a year after the file is replaced or the PDF render settings change.

- Page components take the file (`PageSourceFile`: `id`, `updated_at`, `file_type`), not a bare `fileId` (`PagePicker`, `PagePreview`, `ChapterRow`).
- Tests need the shared auth mock; its `pdfRenderKey` defaults to `200-85`.
- `updated_at` is the key because every rescan that re-reads a changed file bumps it, and the same scan drops the server's cached pages (`invalidatePageCaches` in `pkg/worker/scan_unified.go`). Unrelated metadata edits also change it, which only costs a refetch. A size-plus-mtime key would churn less but would not change on a forced refresh, the manual fix after a replacement that kept size and mtime.

## Download and stream URLs

These are served `private, no-store` and carry no key, but are still built in one place: `app/utils/downloadUrl.ts`.

## The lint rule is a tripwire

ESLint rejects a literal `/api/.../cover`, `/page/`, `/download`, or `/stream` URL outside `app/utils` (pinned in `app/eslint-rules.test.ts`). It needs the `/api/` prefix and the segment as literals in the same template, string, or `+` chain, so it misses a prefix held in a variable, `[...].join("/")`, `.concat()`, and a path without the leading slash.
