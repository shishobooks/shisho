# Share Links (`pkg/sharelinks`)

A Share Link grants anonymous access to one Book (see `CONTEXT.md`). The package owns the service, the sharing settings (`RegisterRoutes`, see `admin-settings.md`), and two route families. `pkg/server/share_links_test.go` drives everything below through the real route table.

## Management (`RegisterBookRoutes`)

Mounted on its own `/books` group that only authenticates, not the Books Read group, because a role may hold only `shares` operations. Each route checks its `shares` permission and the book's library access itself.

- `GET /api/books/:id/share-links` takes `shares:read` or `shares:write` (a sharer must be able to copy the links they make) and returns a bare `ShareLinkResponse[]`: the model plus derived `state`, `paused_reason`, and `created_by_username`.
- `POST` takes `shares:write`. It refuses with 403 while sharing is off and 422 for a missing `expires_at` when expiration is required or a past one. The server takes an absolute timestamp and knows nothing about the client's presets.
- Revoke (`POST .../:linkId/revoke`, returns the `ShareLinkResponse`) and delete (`DELETE .../:linkId`, 204) take `shares:write` and return 404 for a link on another book. They work while sharing is off, because only create checks the switch: an admin pulling a leaked link must not have to turn sharing back on and so restore every link. Revoke stamps `revoked_at` once and never clears it; delete removes the row in any state.

## Public (`RegisterPublicRoutes`)

`/api/share/:token` serves the book, book cover, file cover, and file download (GET and HEAD). It is unauthenticated and unregistered in Demo Mode.

**Every public handler calls `publicHandler.resolve` first and never mounts an authenticated handler.** The books download and cover handlers need an authenticated user (`auth.RequireLibraryAccessFor`), so they would 401 every recipient and skip the refusal rules. New refusal rules go in `resolve`, not in individual handlers.

- `resolve` returns `errcodes.NotFound("Share Link")` for every failure (malformed token, sharing off, unknown, revoked, expired, paused) so a recipient cannot tell them apart. A file outside the link's book is `NotFound("File")`, the same status and code. A deleted creator or book cascades the row away.
- Paused means the creator is deactivated or cannot reach the book's library: `models.ShareLink.PausedReason(libraryID)`, the same value management reports as `paused_reason`, so the dialog and the resolver agree. Any query whose links reach either must load `CreatedByUser.LibraryAccess`. The check runs per request, so restoring the creator's access (or reactivating them, API only) brings the link back.
- State is derived by `models.ShareLink.State(now)`, never stored: revoked if `revoked_at` is set, else expired once `expires_at` is at or before now, else active.
- The token is 32 bytes from `crypto/rand`, unpadded base64url (43 characters), no prefix. `wellFormedToken` rejects other shapes before any query.

## Recipient payload

`SharedBookResponse` is the generated `Book` with `blankForRecipient` applied: book and file paths emptied, cover filenames and scan errors removed, the library dropped, series cover filenames removed, and the library-facing fields the recipient page hides (sort title, file URLs, file identifiers) dropped. It adds `shared_by`, `expires_at`, and the library's `cover_aspect_ratio`.

- It is not a strict mirror of the page: timestamps (file cover URLs key on `updated_at`), `*_source` fields, and review flags stay, and the downloaded file still carries full metadata.
- Compute `cover_cache_key` before blanking, since it reads cover filenames.
- `blankForRecipient` resolves each supplement's `display_name` before clearing its path; a main file with no name gets an empty `display_name` (the page shows its type).

## Usage counts and downloads

- The book fetch calls `RecordOpen`; a download calls `RecordDownload` after the file has been served, so a 404 or 500 is not counted. The count runs under `context.WithoutCancel` because the recipient may be gone by then. `startsDownload` counts only a GET with no `Range` or one starting at `bytes=0-`, so HEAD, resumes, and download-manager segments do not count. Covers count nothing.
- Counts set `last_accessed_at` and leave `updated_at` alone. Each is a single `UPDATE ... SET x = x + 1`; a failed count is logged, not returned.
- Downloads go through `books.ResolveFallbackDownload` (see `covers-and-file-serving.md`): a recipient has no Download Original, so the original stands in when generation cannot run. Covers use `covers.CacheControlImmutable`.
