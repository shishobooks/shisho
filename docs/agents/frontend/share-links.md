# Share Links and the Book Detail body

Read this before changing `BookDetailBody`, adding a control to Book Detail, or touching the sharer dialog or the `/share/:token` recipient page.

## `BookDetail` fetches, `BookDetailBody` renders

`BookDetail.tsx` only fetches (`useBook`, `useUserLibrary`), handles loading and not-found, and renders `LibraryLayout` and breadcrumbs. `BookDetailBody` (`app/components/library/BookDetailBody.tsx`) owns the cover, metadata, resource lists, file list, action menus, and the dialogs behind them. It takes a `Book` payload rather than calling `useBook`, so a parent can hand it data from any source.

## Share Link context

Passing `shareLink` (a `ShareLinkContext`) switches the body into Share Link context, for a recipient with no access to the library:

- Author, series, genre, tag, narrator, publisher, and file names render as plain text through `ResourceLink` (a `Link` given a path, a `span` given `null`).
- The book action menu, Add to list, review toggle, per-file menus, Select, and Read/Listen are hidden regardless of permissions (`canWriteBooks` is forced false).
- Sort title, the Created/Updated/Library/File Path block, the per-file filename row, and each file's URL and identifiers are omitted. File labels still come from `display_name`, which the server resolves before blanking paths. A file whose only details are hidden ones gets no expander.
- Downloads go to `shareLink.downloadUrl(file)` with the same HEAD-then-navigate flow; no format popover, no Download Original fallback.
- Covers use `shareLink.bookCoverUrl(book)` and `shareLink.fileCoverUrl(file)`. `FileCoverThumbnail` and `CoverGalleryTabs` accept the same `getCoverUrl` builder; returning `null` shows the placeholder.
- The plugin identifier types and sharing settings queries are disabled, so the body makes no authenticated requests.
- `shareLink.coverAspectRatio` stands in for the library's cover aspect ratio.

**Every new control in the body decides its Share Link behavior.** Anything that links into the app or mutates data is hidden or rendered as plain text when `isShareLink` is true, with a case in `BookDetailBody.test.tsx`.

## Sharer side

- `useSharingSettings` fetches only for a role holding a `shares` operation or `config:read`. `BookDetailBody` shows **Share** once settings load whenever the user holds either `shares` operation, even while sharing is off, so a sharer can still revoke or delete links. Share's own permission is why the action menu gates per entry.
- `ShareLinkDialog` takes `sharingEnabled`. While false it shows a notice instead of the new-link form, disables every copy button, rewords the revoke and delete confirmations, and keeps Revoke and Delete. The notice names Settings > Sharing and links there (closing the dialog) when `canManageSharing` (Book Detail sets it from `config:write`).
- With sharing on: the new-link form and each row's Revoke and Delete need `shares:write` (`canWrite`); the list shows for either operation.
- Every row shows opens, downloads, and last used. An active link with a `paused_reason` shows a muted **paused** badge and the reason and cannot be copied. Revoke is offered on active links, paused included; Delete on every link. Both confirm through a `ConfirmDialog` rendered beside the `FormDialog`, not inside it (as `RoleDialog` does).
- Expiration presets are client-side; the dialog sends an absolute `expires_at`.
- The copy button builds `${window.location.origin}/share/<token>` and copies through `copyText`.

## Recipient side

- `shareRoutes` (`app/components/pages/shareRoutes.ts`) mounts `SharedBook.tsx` at `/share/:token` plus a `/share/*` splat, so a truncated link shows the unavailable page instead of the router error page. These are top-level public routes beside `/login` and `/setup`, outside `Root` and `ProtectedRoute`: no login redirect, nav, or demo banner.
- `SharedBook` renders its own `<Toaster />` and a `ShareNotice` strip under the header saying who shared the book and when the link expires.
- It still sits inside `AuthProvider` (the body calls `useAuth`), which makes anonymous `/auth/status` and `/auth/me` calls. `useSSE` skips any `/share/` path, so the event stream never opens there.
- A 404 renders `ShareUnavailable`; any other failure is retried once and then shows a Try again page, since the link may still be fine.
- The share payload blanks cover filenames, so `SharedBook` keys the book cover off `cover_cache_key` and requests a cover for every main file; a missing file cover 404s and falls back to the placeholder.
