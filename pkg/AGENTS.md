# Shisho Backend Development

This file documents backend patterns and conventions specific to Shisho.

## Stack

- Go with Echo web framework
- Bun ORM with SQLite database
- Air for hot reload
- mise for task running and tool management

## Architecture

### Entry Point

`cmd/api/main.go` starts both HTTP server and background worker. The listener uses `http.Server.Addr`, built with `net.JoinHostPort` from `server_host` and `server_port`.

### HTTP routing

- `pkg/server` registers API routes beneath `e.Group("/api")`. API registration helpers accept `*echo.Group`; device route helpers still accept `*echo.Echo` because `/opds`, `/kobo`, `/ereader`, and `/e` stay at the root, alongside `/health`.
- Every route package exports `RegisterRoutes(router, deps...)` with no return value. The router is the first argument: the `/api` group, a group the server already configured with authentication and a permission, or `*echo.Echo` for device routes. A package mounted in more than one place adds `Register<Scope>Routes` (`books.RegisterLibraryRoutes`, `plugins.RegisterIdentifyRoutes`, `plugins.RegisterLookupRoutes`, `plugins.RegisterReadRoutes`, `libraries.RegisterListRoutes`, `libraries.RegisterUserRoutes`, `users.RegisterDirectoryRoutes`). A registration function never builds a service another package needs and never returns one; `pkg/server` builds it and passes it in, as it does for the auth service (see "Shared services"). Do not add parameters a package does not use.
- The router serves `pkg/frontend.Handler()` for unknown GET/HEAD paths outside the API and device prefixes. Reserved prefixes match whole path segments, so `/apiary` is a frontend route. Missing API/device routes return JSON 404, including omitted device routes in demo mode.
- Use per-router `RouteNotFound` handlers, never mutate `echo.NotFoundHandler`. Echo adds authenticated group fallbacks when `Group.Use` runs; server construction replaces those fallbacks after all routes are registered so missing paths return JSON 404 rather than authentication errors. Existing endpoint authentication is unchanged.
- Vite proxies `/api` unchanged, without rewriting paths or adding `X-Forwarded-Prefix`. OPDS is available at `/opds` in development too. Frontend routes and APIs share one origin; the server does not enable CORS.
- Forwarded-header sanitization runs with `e.Pre` before routing. Request logging, recovery, security headers, compression, and demo enforcement run with `e.Use`. Keep the frontend fallback pattern `/*` recognizable to request logging.

### Demo Mode

`demo_mode` / `DEMO_MODE` defaults to `false`. When enabled, `pkg/server/demo_mode.go` runs globally via `e.Use` after logger and recovery, before authentication and handlers. Use matched `c.Path()` patterns, not raw URLs; do not move this middleware to `e.Pre`.

- Allow `GET`, `HEAD`, and `OPTIONS`, subject to normal authentication and permissions.
- Allow only `POST /api/auth/login` and `POST /api/auth/logout` among write methods.
- Deny `GET` and `HEAD` for `/api/books/files/:id/download/original`, `/api/books/files/:id/download/kepub`, and `/api/jobs/:id/download` (`deniedDownloads`). A `HEAD` still runs the handler, including KePub generation, so it is denied with the `GET`.
- Reject every other method/path with `403`, code `demo_mode`, message `This action is unavailable in the demo.` Admins have no bypass. Unknown write paths are rejected too.
- Keep generated reader downloads (`/api/books/files/:id/download`), CBZ/PDF pages, and audio streaming available. These serve complete files for every main format (the generated download for EPUB, CBZ, PDF, and M4B, and the full M4B from `stream` without a `Range` header), so this is not copy protection; supplements can also be served through the generated download route, so the Public Demo must not include them.
- Do not register OPDS (`/opds/*`), eReader (`/ereader/*` and `/e/:shortCode`), Kobo (`/kobo/*`), the public Share Link family (`/api/share/*`), any `/api/plugins` group, per-library plugin routes (`/api/libraries/:id/plugins/*`), or test routes (`/api/test/*`, even with `ENVIRONMENT=test`). GET requests to omitted families return `404`; write methods still receive the global `403`.
- `GET /api/user/libraries` is allowed like any read. `GET /api/users/directory` is not registered, because it would show every visitor the admin's username; the path falls through to `GET /api/users/:id`, which requires Users Read, so the demo viewer gets `403`. `TestNew_DemoModeRoutes` covers both.
- Share Link management stays registered: `GET /api/books/:id/share-links` is allowed like any read, and create, revoke, and delete are rejected by the global `403`. `TestNew_DemoModeRoutes` and `TestShareLinks_ManagementRejectedInDemoMode` cover them.
- Skip `pluginManager.LoadAll` and `wrkr.Start` in `cmd/api/main.go`. Also skip `wrkr.Shutdown`, which waits for goroutines that only `Start` creates. Reader caches and startup migrations still run.
- `GET /api/auth/status` exposes the flag before sign-in. Pass the boolean to `auth.RegisterRoutes`; importing `config` from `auth` creates an import cycle because config routes use auth middleware.

Any new route family or download path must be classified here as allowed, denied, or unregistered in Demo Mode, with corresponding middleware or route-registration tests. New GET/HEAD handlers must not introduce persistent user changes.

### Admin Settings Store

`app_settings` (`pkg/appsettings`) holds admin-editable feature settings as one JSON document per key, with a domain package owning the key, the struct, and load and save helpers (`review.Load`/`review.Save`, `sharelinks.LoadSettings`/`sharelinks.SaveSettings`) that fall back to defaults when no row exists. The review criteria (`pkg/books/review`, key `review_criteria`) set the precedent. Sharing (`pkg/sharelinks`, key `sharing`) follows it and is the first feature switch stored there instead of in `config.Config`: a switch that carries companion policy and a warning the admin must read belongs in the admin UI, with no config field or env var (ADR 0008). Prefer this store for new policy an admin decides at runtime; keep deployment facts in config.

The endpoints live under `/api/settings/*`. The sharing endpoints are registered by `sharelinks.RegisterRoutes`, not `pkg/settings`, because `pkg/books` imports `pkg/settings` and `pkg/sharelinks` imports `pkg/books`. `PUT /api/settings/sharing` takes pointer fields so each switch saves on its own, and the handler loads, merges, and saves the document outside a transaction. Two admins changing different switches at the same instant can lose one change; that is accepted for rarely edited admin settings, but wrap the load and save in a transaction if a document ever gets frequent concurrent writers. Writes require `config:write` and are rejected in Demo Mode by the global middleware; reads are GETs and stay allowed.

### Share Links (`pkg/sharelinks`)

A Share Link grants anonymous access to one Book (see `CONTEXT.md`). The package owns the model's service, the sharing settings (`RegisterRoutes`), and two route families:

- **Management** (`RegisterBookRoutes`) mounts on its own `/books` group that only authenticates, not the Books Read books group, because a role may hold only `shares` operations. `GET /api/books/:id/share-links` requires `shares:read` or `shares:write` (a sharer must be able to copy the links they make) and `POST` requires `shares:write`; both check the book's library access in the handler. The list is a bare array of `ShareLinkResponse` (the model plus a derived `state`, a `paused_reason` on an active link its creator has paused, and `created_by_username`). Create refuses with `403` while sharing is off, `422` for a missing `expires_at` when expiration is required, and `422` for a past one. The server takes an absolute timestamp and knows nothing about the client's presets. `POST /api/books/:id/share-links/:linkId/revoke` (returns the `ShareLinkResponse`) and `DELETE /api/books/:id/share-links/:linkId` (`204`) also require `shares:write` and library access, and return `404` for a link on another book. They work while sharing is off: only create checks the switch, so an admin never has to turn sharing back on, and so briefly restore every link, just to pull one that leaked (`TestShareLinks_RevokeAndDeleteWorkWhileSharingDisabled`). Revoke stamps `revoked_at` once and never clears it, so revoking again is a no-op; delete removes the row in any state.
- **Public** (`RegisterPublicRoutes`) is `/api/share/:token` with the book, book cover, file cover, and file download (GET and HEAD). It is unauthenticated and unregistered in Demo Mode.

**Public handlers resolve through the token and never mount authenticated handlers.** The books download and cover handlers only check library access when a user is in context, so mounting them on an unauthenticated route would serve any file. Every public endpoint calls `publicHandler.resolve` first, and any future public route must do the same. `resolve` returns `errcodes.NotFound("Share Link")` for every failure (malformed token, sharing off, unknown token, revoked or expired, paused because the creator is deactivated or cannot reach the book's library) so a recipient cannot tell the cases apart; a file outside the link's book is `NotFound("File")`, the same status and code. A deleted creator or book cascades the row away. The pause check is `models.ShareLink.PausedReason(libraryID)`, and the management responses report the same value as `paused_reason`, so the dialog and the resolver cannot disagree; any service query whose link reaches either must load `CreatedByUser.LibraryAccess`. The creator checks are evaluated on every request, so restoring the creator's library access (or reactivating them, which only the API can do) brings the link back, as turning sharing back on does. Add new refusal rules to `resolve`, not to individual handlers.

- The token is 32 bytes from `crypto/rand`, unpadded base64url (43 characters), with no prefix. `wellFormedToken` rejects other shapes before any query.
- State is derived by `models.ShareLink.State(now)`, never stored: revoked if `revoked_at` is set, otherwise expired once `expires_at` is at or before now, otherwise active.
- The book payload (`SharedBookResponse`) is the generated `Book` with `blankForRecipient` applied: book and file paths emptied, cover filenames and scan errors removed, the library dropped, and series cover filenames removed. It also drops the library-facing fields the recipient page hides (sort title, file URLs, file identifiers). It is not a strict mirror of the page: timestamps (file cover URLs use `updated_at` as their cache key), `*_source` fields, and review flags stay, and the downloaded file still carries the book's full metadata. Compute `cover_cache_key` before blanking, since it reads cover filenames. `blankForRecipient` resolves each supplement's `display_name` before clearing its path, so it still shows its filename. A main file without a name gets an empty `display_name` (the page shows its type) rather than its on-disk filename. The payload adds `shared_by`, `expires_at`, and the library's `cover_aspect_ratio`.
- Usage counts: the book fetch calls `RecordOpen`, and a download calls `RecordDownload` just before the file is served, once the file to serve (generated or original) is known. `startsDownload` counts only a GET with no `Range` or one starting at `bytes=0-`, so HEAD, resumed transfers, and a download manager's later segments do not count. Both set `last_accessed_at` and leave `updated_at` alone. Covers count nothing. The increment is a single `UPDATE ... SET x = x + 1`, and a failed count is logged, not returned, so the recipient still gets the page or file.
- Downloads serve the generated file through the shared download cache with `private, no-store`. Supplements and file types with no built-in generator (formats only a plugin parses) serve as-is. A `filegen.GenerationError` falls back to the original file, as the eReader download does, because a recipient has no Download Original button. Covers use `covers.CacheControlImmutable`.

`pkg/server/share_links_test.go` drives all of this through the real route table.

### Core Services Pattern

Each domain (books, jobs, libraries, chapters) has:
- `handlers.go` - HTTP request/response logic
- `routes.go` - HTTP endpoint registration
- `service.go` - Business logic and database operations
- `types.go` - Request/response schemas

### Shared services

`pkg/server/server.go` collects a fixed set of services in a `sharedServices` value inside `New` and injects them into every route family that needs them. It builds the `books.Service` (built `WithAppSettings`), the `appsettings.Service`, and the `sharelinks.Service` itself. The `plugins.Service` and the download, CBZ page, and PDF page caches are passed in from `cmd/api/main.go`, which also hands the same `plugins.Service` to the plugin `Manager` and `worker.New`. The books service is shared because it must carry app settings. The app settings, plugin, and Share Link services hold only the database handle, but they are shared anyway so each has one construction site outside tests. The books routes, library languages route, chapters, genres, tags, people (and its file organizer), series, publishers, plugin identify, OPDS (routes and `opds.NewService`), eReader, Kobo, and the public Share Link routes all take the shared books service; settings and the Share Link settings and management routes take the shared app settings service, and the Share Link management and public routes take the shared `sharelinks.Service`. A route package must not build its own `books.Service`, `appsettings.Service`, `plugins.Service`, `sharelinks.Service`, or page cache. A `books.Service` without app settings silently skips the Reviewed recompute on every mutation, and a second page cache instance can drift from the one the cache admin routes size and clear.

Other services that hold only the `*bun.DB` handle (search, aliases, libraries, jobs, settings, API keys, and the entity services such as people, genres, tags, publishers, and series) are cheap and stateless, so route packages may build those locally. The books routes read app settings from the injected books service (`AppSettings()`) rather than taking a second argument that could disagree with it. The background worker is built in `cmd/api/main.go`, not by the server, and builds its own `books.Service` with app settings and `appsettings.Service`; it takes the shared `plugins.Service`. It also receives the same CBZ and PDF page caches that `main.go` passes to `server.New`, because the scan drops a changed file's cached pages (`invalidatePageCaches`); a worker with its own cache instances would clear the wrong one. When `server.New` receives nil page caches or a nil plugin service (tests), it builds them itself, the caches from the config, so page, cover-page, and plugin routes still work. `worker.New` likewise builds a `plugins.Service` when handed nil. `pkg/server/shared_services_test.go` covers the page cache, plugin service, and books wiring through the real routes.

### Database Models (`pkg/models/`)

- Use Bun ORM with struct tags for database mapping
- Models include JSON tags for API serialization
- TypeScript types auto-generated via tygo from Go structs
- **`File.display_name` is resolved when files are loaded.** `models.File` has a non-persisted `DisplayName` field filled from `ResolveDisplayName()`. A main file shows its `name`, falling back to the filename. A supplement shows its filename unless `name_source` is `manual`, because the scanner-stored name goes stale when the book is renamed. `sidecar` does not count: book edits write every file's sidecar (supplements included) with its stored name, so a rescan restores a stale stem with source `sidecar`. A Bun `AfterScanRow` hook on `*File` fills it for direct file queries (`RetrieveFile`, file lists). Bun does not run row hooks for has-many relations, so every loader that selects books with `Relation("Files")` (or `Book.Files`) and whose result reaches a JSON response must call `models.ResolveBookFileDisplayNames` after scanning. Today that is `RetrieveBook`, `RetrieveBookByFilePath`, `listBooksWithTotal`, and `GetFirstBookInSeriesByID` in the books service, and `listBooksWithTotal` in the lists service; `DeleteBookAndFiles` and the plugin enrich handler load files only internally. A new loader must add the call, and `pkg/server/file_display_name_test.go` should cover its endpoint. Code that changes `Name` or `Filepath` in memory before responding must re-resolve. The Share Link payload sets it before blanking paths. The frontend renders `fileLabel(file)` (`@/utils/format`) and does not rebuild the label from `name` and `filepath`.

### Background Worker (`pkg/worker/`)

- Processes jobs from database queue
- Main job type: scan job that processes ebook/audiobook files
- Extracts metadata from EPUB, CBZ, M4B, and PDF files through the format packages listed below
- Generates cover images with filename-based storage strategy
- **Library monitor** (`monitor.go`): watches library paths for filesystem changes via fsnotify, debounces events, and triggers targeted single-file rescans. Remove/Rename events landing on a directory path (which fsnotify emits for the directory itself, not the files inside) are queued as `pendingEvent{IsDirectory: true}` and fan out to per-file cleanup for every DB file whose filepath sits under that directory, so removing or renaming a book folder cleans up its book/file rows instead of leaving them orphaned. **Move detection via content hashing.** When the monitor processes a batch that contains any REMOVE events, it computes sha256 synchronously for CREATE events in the same batch and looks up matches in `file_fingerprints`. If an existing file row has a matching sha256 and its stored path is gone from disk, the monitor repurposes that row's `filepath` rather than deleting + recreating. This preserves book identity and user-edited metadata across folder renames. The scan job performs the same reconciliation as a safety net after its walk phase, handling cases where renames happened while the server was offline. Sha256 hashes are populated by a background `hash_generation` job queued at the end of every scan and every monitor batch that creates new files. Fingerprints are invalidated when a file's size/mtime changes so the next job run recomputes them against the new content.

### Unreadable Files and the Extension Mime Check

- **`files.scan_error` records a parse failure for a file already in the library.** `scanFileByID` sets it (innermost cause only, e.g. `zip: not a valid zip file`) when `parseFileMetadata` fails and clears it on the next successful parse. New files that fail to parse are never inserted, so they have no row to flag; the job log warning is the only signal for them. The frontend renders `FileScanErrorBadge` on file rows and an alert on the file details tab. `fileContentChanged` treats a flagged file as changed so the size/mtime shortcut cannot strand the flag, and `tryDetectMove` rescans a repurposed row that carries one.
- **Sidecar removal is deferred until the file has parsed.** When a file is swapped on disk or scanned with refresh, the stale sidecar must go, but only after the replacement file parses. Removing it first and then failing to parse would destroy the last on-disk record of the file's metadata. Keep the `discardSidecar` decision before the parse and the `removeFileSidecar` call after it.
- **`checkExpectedMimeType` is the single content check for built-in extensions.** Both the scan walker (`ProcessScanJob`) and the monitor's new-file path (`processEvent`) call it, so a file can never be rejected by one and imported by the other. `.m4b` accepts `audio/x-m4a` (`M4A ` brand), `audio/mp4` (`M4B ` brand), and `video/mp4` (`isom`/`mp42` brands); all three are real audiobooks depending on the tool that wrote them. Files already in the DB skip the check in both places, including a Create event on a tracked path (temp file + rename), which is rescanned so a now-unreadable replacement gets flagged instead of silently ignored.

### scanInternal and File Organization

**CRITICAL — `scanInternal(FilePath)` defers file organization.** When scanning a new file by path, `scanFileCore` is called with `isResync=false`, which skips the book organization step. The caller is responsible for running organization afterward if the library has `OrganizeFileStructure` enabled.

- **`ProcessScanJob`** handles this by collecting book IDs into `booksToOrganize` and running organization in a batch after all files are scanned.
- **`Monitor.processPendingEvents`** handles this by collecting book IDs from `FileCreated` results and calling `organizeBooks()` after processing all events.
- **Any new caller of `scanInternal(FilePath)`** must also handle organization, or files will be left unorganized in the library root.
- **Resync narrator changes must trigger organization after relationship persistence.** The earlier filename check uses the pre-scan narrators. Include M4B narrator updates in the post-`UpdateBookRelationships` organization condition, even when Title and Authors are unchanged. A hybrid book's EPUB may restore Authors before its M4B restores Narrators, so an author-only trigger leaves the audiobook filename stale.

### Scan Cache Must Include Supplements

**CRITICAL — the `ScanCache.knownFiles` map preloaded in `ProcessScanJob` must contain BOTH main and supplement files.** Supplements can share scannable extensions (`.pdf`/`.epub`/`.cbz`/`.m4b`) with main files — e.g. a user-demoted `Cribsheet.pdf` sitting next to a main `Cribsheet.epub`. During the filesystem walk, every file with a scannable extension becomes a scan target, so a supplement at a tracked path must resolve via the cache and early-return in `scanFileByPath`. If the cache is main-only (the old behavior), the supplement falls through to `scanFileCreateNew`, which tries to insert a duplicate file row and hits `UNIQUE(filepath, library_id)` — warned-and-continued on every scan.

- Preload via `ListAllFilesForLibrary`, not `ListFilesForLibrary` (which is still main-only, used for orphan cleanup).
- `scanFileByPath` early-returns with `&ScanResult{File: existing}` when the cached file has `FileRole == FileRoleSupplement` — supplements have no metadata to rescan.

### Auto-Classification of Supplement-Named PDFs

`scanFileCreateNew` (in `pkg/worker/scan_unified.go`) inspects new PDF files for the auto-supplement rule before creating the file row: when a PDF's basename matches `config.PDFSupplementFilenames` AND a sibling main file (EPUB / CBZ / M4B / plugin-registered extension) exists on disk OR a book row already exists at the same `bookPath`, it's created with `FileRole=Supplement` and **cover extraction is skipped** (matching the existing supplement-discovery path that creates supplements without covers).

When editing the cover-extraction block in `scanFileCreateNew`, preserve the `if !classifyAsSupplement { ... }` guard. Rescans don't re-run this rule — existing main-file PDFs whose names match the list keep their role.

### File Types

- To learn more about all the file types that we support, refer to:
  - EPUB: `pkg/epub/AGENTS.md`
  - CBZ: `pkg/cbz/AGENTS.md`
  - M4B: `pkg/mp4/AGENTS.md`
  - PDF: `pkg/pdf/AGENTS.md`
  - KePub: `pkg/kepub/AGENTS.md`

### Cover Image System

- Individual file covers: `{filename}.cover.{ext}`
- API endpoints: `/api/books/{id}/cover` and `/api/books/files/{id}/cover`

**CRITICAL - CoverImageFilename stores FILENAME ONLY:**

`file.CoverImageFilename` stores just the filename (e.g., `MyBook.cbz.cover.jpg`), NOT the full path. The full path is constructed at runtime by joining the book directory with the filename.

When updating `CoverImageFilename` (e.g., after renaming a file), always use `filepath.Base()` to extract just the filename:

```go
// ❌ WRONG - stores full path, breaks cover serving
newCoverPath := fileutils.ComputeNewCoverPath(*file.CoverImageFilename, newPath)
file.CoverImageFilename = &newCoverPath

// ✅ CORRECT - stores filename only
newCoverPath := filepath.Base(fileutils.ComputeNewCoverPath(*file.CoverImageFilename, newPath))
file.CoverImageFilename = &newCoverPath
```

**Why this matters:** Handlers resolve the full path at runtime by joining `book.Filepath` with `CoverImageFilename`. If `CoverImageFilename` contains a full path, this results in an invalid doubled path like `/path/to/path/to/cover.jpg`.

**Always resolve cover paths via the file, not the book**, because `book.Filepath` can be a synthetic organized-folder path that never exists on disk (root-level books in libraries with `OrganizeFileStructure` disabled — see `scanFileCreateNew`). The cover lives alongside the file for both root-level and directory-backed books.

- **Read-side (serving, fingerprinting, file generation):** use `filepath.Join(filepath.Dir(file.Filepath), *file.CoverImageFilename)`. Pure-string resolution (no stat needed to find the path), no synthetic-path trap. Book-cover serving across the books, series, OPDS, and eReader handlers shares `pkg/covers.ServeBookCover`, which encapsulates this resolution, the missing-cover 404, and the ETag-based conditional-GET pattern (see "Conditional-GET for cover endpoints" below). The series handler passes its first book's files and `"Series cover"` as the 404 resource; every other caller passes `"Cover"`. Other examples: `covers.FileCoverPath` (a file's own cover, used by `fileCover` in `pkg/books/handlers.go` and the Share Link file cover), `pkg/kobo/handlers.go`, `pkg/filegen/*`, `pkg/downloadcache/fingerprint.go`, and `deleteFileFromDisk`.
- **Write-side (scanner, pre-organize):** use `fileutils.ResolveCoverDirForWrite(bookFilepath, fileFilepath)` when `bookFilepath` may be a synthetic organized-folder path that hasn't been created on disk yet. Falls back to `filepath.Dir(fileFilepath)` when the book path doesn't resolve to a real directory.
- **Cover writes never destroy a working cover.** Write the replacement with `fileutils.WriteFileAtomic` (temp file plus rename, 0644), collect the previous covers at other extensions with `fileutils.OtherCoverExtensions`, and remove them with `books.RemoveStaleCovers` only after the database write that names the replacement has succeeded. Extractors (`books.ExtractCoverPageToFile`, the scanner's `extractCBZPageCover` and `extractPDFPageCover`) return the stale list rather than deleting; the caller that owns the `UpdateFile` owns the removal. Never delete first and write second, and never delete before the row is updated. When the bytes come from outside the file (an enricher, a download, an upload), the gate is `fileutils.NormalizeImage`'s error, which decodes every pixel; `fileutils.ImageResolution` only reads the header and accepts a truncated body, so it is for resolution comparison, not validation. Embedded covers from the file's own parser are still written best-effort when they do not decode.

**Book sidecars** for root-level books are similarly anchored next to the file — `sidecar.WriteBookSidecarFromModel(book)` falls back via `book.Files[0].Filepath` when `book.Filepath` doesn't resolve to an existing directory. Reads use `sidecar.ReadBookSidecarFromModel(book, fileHint)` and pass the current file as a hint so resolution works before the book's Files relation is loaded.

**Conditional-GET for cover endpoints:** `/files/:id/cover` stats the cover first (a missing cover returns `errcodes.NotFound("Cover")`, not Echo's generic 404), then serves via `c.File()`. `http.ServeContent` handles `Last-Modified`/`If-Modified-Since` from the cover file's on-disk mtime, which is sufficient because the served file's identity is pinned by the URL. `/books/:id/cover` (and its OPDS / eReader mirrors) and `/series/:id/cover` are different: the served file is *selected*, and that selection can change without any change to the newly-selected cover file's mtime. Flipping the library's `CoverAspectRatio` setting on a hybrid book (EPUB + M4B) swaps which file's cover is served, removing a file from a hybrid book falls back to the remaining file's cover, and a series' first book can change because of book deletion / re-sorting / series-number changes. Mtime-only revalidation returns stale 304s in those cases. `pkg/covers.ServeBookCover`, which the series handler also uses, therefore issues an `ETag: "<file_id>-<mtime_unix>"` that bakes the selected file's identity into the validator, checks `If-None-Match` manually, and passes `time.Time{}` to `http.ServeContent` so it omits `Last-Modified` and skips IMS handling (which would otherwise shortcut to 304 using just the new file's mtime).

### Cache-Control Headers for File-Serving Endpoints

All file-serving endpoints must include a `Cache-Control` header to prevent reverse proxies (OpenResty, Nginx, Cloudflare) from heuristically caching responses that include `Last-Modified` but no `Cache-Control`, which can serve stale content even after Shisho's internal caches are cleared.

| Endpoint Category | Header | Reason |
|---|---|---|
| **API cover endpoints** (`/api/*/cover`) | `Cache-Control: private, max-age=31536000, immutable` | Browser caches forever; URL changes via `?v=cover_cache_key` when cover changes |
| **External cover endpoints** (OPDS, eReader, Kobo) | `Cache-Control: private, no-cache` | External clients don't use `?v=` cache-busting; revalidation via `Last-Modified`/`ETag` |
| **Download/stream endpoints** (`/download`, `/stream`) | `Cache-Control: private, no-store` | No proxy caching at all — generated files can change without the URL changing |
| **Page endpoint** (`getPage`) | `Cache-Control: private, max-age=31536000, immutable` | The frontend keys page URLs on `?v=file.updated_at` (`filePageUrl`). A rescan that finds the file changed bumps `updated_at` and drops the file's cached pages. `private` because the route is authenticated; the browser still caches it |
| **Plugin icon** (`/api/plugins/installed/:scope/:id/image`) | `Cache-Control: private, no-cache` | The URL has no cache-busting version and the icon changes when the plugin is updated; revalidation via `Last-Modified` |

Before serving a file's bytes, check that it still exists with `books.RequireFileOnDisk(c, file, resource)` rather than an inline `os.Stat`. It returns `errcodes.NotFound(resource)` and warn-logs the file ID and path (skipped on HEAD). The books download, stream, and page handlers and the OPDS, eReader, and Kobo download handlers use it. The page handler checks before the page cache lookup, so a cached page is not served for a missing source. The books generated-download and KePub handlers pass `"Source file"`; every other site passes `"File"`.

When adding a new endpoint that serves files via `c.File()` or `c.Stream()`, set the appropriate `Cache-Control` header before the call. Cover endpoints should use the constants in `pkg/covers/covers.go` (`covers.CacheControlImmutable` for API endpoints, `covers.CacheControlNoCache` for external):

```go
c.Response().Header().Set("Cache-Control", "private, no-store")
return c.File(path)
```

### Data Source Priority System

Metadata sources ranked (lower number = higher precedence):
```
0: Manual (highest)
1: Sidecar
2: Plugin (enrichers and file parsers)
3: File Metadata (epub_metadata, cbz_metadata, m4b_metadata, pdf_metadata)
4: Filepath (lowest)
```

Used to determine which metadata to keep when conflicts occur. During scans, enricher plugins override file-embedded metadata per-field (enricher-first merge in `runMetadataEnrichers`).

**Series memberships have their own source.** `books.series_source` is aggregate provenance for a Book's ordered membership collection and Series Number groups; `series.name_source` only describes the Series resource's name. The scanner, Identify, and the Edit form gate and stamp `books.series_source`. Never read `Series.NameSource` as a proxy for membership provenance (ADR 0006). A `FindOrCreateSeries` call still carries a name source, which may lower `name_source` on an existing Series, and that is independent of the membership source.

**Deleting a shared resource stamps `manual` on every affected owner's source.** `DeletePublisher`, `DeleteGenre`, `DeleteTag`, `DeleteSeries`, and `DeletePerson` set `publisher_source`, `genre_source`, `tag_source`, `series_source`, `author_source`, or `narrator_source` to `manual` on every Book or File that used the resource, inside the delete's transaction and before the join rows (or the foreign key) go, whatever the prior source was and even when the collection keeps other members. Without it, the sidecar written by the last Scan re-creates the deleted resource on the next ordinary Scan (ADR 0006). Merges and orphan cleanup are exempt. A new delete path for a resource that owners reference needs the same stamp; `pkg/worker/collection_delete_scan_test.go` shows the Scan-level test.

**Deleting a shared resource recomputes Reviewed for every affected Book after commit.** `DeletePublisher`, `DeleteGenre`, `DeleteTag`, `DeleteSeries`, and `DeletePerson` return the IDs of the affected Books (for a Publisher, the Books that own a File it published; for a Person, the Books it authored plus the Books that own a File it narrated; each once). The delete handler passes the IDs to `RecomputeReviewedForBooks`, which loads the review criteria once, because losing the Publisher or the last Genre, Tag, Series, Author, or Narrator can take a Book out of Reviewed. The search index follows the one rule in "Search Index (FTS)" below: the Series and Person handlers collect the affected ids before the delete and reindex after it. `pkg/books` imports publishers, genres, tags, and people, so their handlers take a `BookReviewRecomputer` interface. Series also uses the books service for covers and book listings, so its `RegisterRoutes` takes the full `*books.Service`. `pkg/server/server.go` builds one `books.Service` with `WithAppSettings` and passes it to all five, and to `pkg/chapters` (whose replace handler recomputes Reviewed for the File), since without app settings the recompute silently does nothing (see "Shared services" above). `pkg/server/resource_delete_test.go` covers the wiring through the real routes, including the People merge and single-File delete below. `deleteSeries` in `pkg/series/handlers.go` is the reference.

**Merges.** Every resource merge (People, Series, Genres, Tags, Publishers) follows one checklist. The People merge (`merge` in `pkg/people/handlers.go`, `MergePeople` in `pkg/people/service.go`) and `merge.CheckPreconditions` in `pkg/merge` are the reference; a new merge or re-point mutation goes through the same items:

1. **Retrieve both sides first.** The handler retrieves the target and the source, so a missing one is a 404 from the retrieve instead of a 500 from inside the transaction.
2. **Run `merge.CheckPreconditions`.** It returns 403 when the user cannot access either side's Library (the merge deletes the source, so both need access), 422 on a self-merge, and 422 when the two sides are in different Libraries.
3. **Keep a self-merge backstop in the service.** Each `Merge*` service method returns `merge.SelfMergeError` when target and source are equal, because a self-merge deletes the target and every link to it.
4. **Dedupe join rows before re-pointing.** Where the target already has the row the source would move (the same Book for a Genre, Tag, or Series; the same Book and role for an Author; the same File for a Narrator), drop the source's row, or the re-point violates the unique index. Compare nullable columns with `IS`: `ux_authors_book_person_role` treats NULL roles as distinct, so `=` would list a generic Author twice. `MergeSeries` also copies the source's number group (number, end, and unit together) into a target row that has no number. A column reference like `files.publisher_id` cannot collide. A tree like the Publisher hierarchy must not gain a cycle: `MergePublishers` moves a target that sits below the source to the source's parent (a root when the source has none, or when a pre-existing cycle would loop) before re-parenting the source's children to it. `descendantIDsSubquery` walks with `UNION` so corrupt circular data cannot loop it.
5. **Aliases go through `aliases.TransferAliasesOnMerge`.** It moves the source's aliases, adds the source's name as an alias of the target under the target's Library, and skips a name that is already the target's name or alias, or another resource's alias.
6. **Reindex after commit.** Collect the target and the source (and for People and Series, their Books and Series follow through the expansion) and defer `ReindexAffected`, as "Search Index (FTS)" below describes. It drops the deleted source's row, so never call `Index*` on the target model directly: after a self-merge that re-inserted a ghost row.
7. **Leave Reviewed and sources alone.** A merge does not change which resources a Book has, so it does not recompute Reviewed, and it is exempt from the manual source stamp that deletes apply.

Renames that land on another resource's name differ by kind. Genres, Tags, and Publishers merge the renamed resource into the existing one (guarded by `existing.ID != id`). Series and People reject the rename with a 422 that tells the user to merge instead, so combining two Series or two People is always an explicit merge. The Books merge (`mergeBooks` in `pkg/books/handlers.go`) is a file move: it rejects a source listed twice, skips the target when it is listed as a source, requires every source to share the target's Library, and runs `CleanupOrphanedEntities` when it deletes a source, as `deleteBook` does. The Book edit handler stores a Person once per role when the same author name (in any case) is sent twice.

**Single-file deletes.** `deleteFile` in `pkg/books/handlers.go` runs `books.CleanupOrphanedPeople`, the people kind of `CleanupOrphanedEntities` alone, when other Files remain, because the deleted File's Narrators may have narrated nothing else; when the last File goes it runs the full orphan cleanup instead. Deletes reindex through `ReindexAffected` (see "Search Index (FTS)").

**Identifier per-entry sources are reconciled through one helper.** Both the Book edit handler (`pkg/books`) and Identify (`pkg/plugins`) build the incoming `[]*models.FileIdentifier` with the source a new or replaced entry should get, then call `identifiers.ReconcileSources(existing, incoming)`. It keys both sides on `identifiers.Key` (type plus normalized value), so a stored value that predates normalization still matches. `pkg/books` imports `pkg/plugins`, so shared identifier logic must live in `pkg/identifiers`, never in either handler package. Reject duplicate identifier types before any delete; `BulkCreateFileIdentifiers` dedupes by type and would otherwise hide the problem by dropping a row.

**Scan carries identifier provenance per entry.** `mergeEnrichedMetadata` unions identifiers by type (earlier contributor keeps a shared type) and stamps each appended entry's `mediafile.ParsedIdentifier.Source` (`json:"-"`, never on the wire). `FieldDataSources["identifiers"]` stays with the first contributor, which is the highest priority because enrichers merge before the file-parser fallback. The persistence block writes each `file_identifiers.source` from the entry's `Source`, falling back to the field source when empty. Do not overwrite the field source per appended entry; that mislabels a mixed collection with the lowest-priority origin. `shouldUpdateRelationship` compares only values, so the identifier block also calls `identifierAttributionStale` when values are unchanged: an ordinary Scan rewrites only when the incoming aggregate strictly outranks the stored one (repairing collections mislabeled before #492 while leaving manual and sidecar collections alone), and a forced refresh rewrites when any entry's origin differs. Intended consequence, not a bug: Identify accepting a proposal that omits an embedded identifier saves a plugin-sourced collection without it, and the next ordinary Scan restores the embedded entry because the union's aggregate is the same plugin priority (`TestScan_MixedIdentifierCollection_OrdinaryScanRestoresEmbeddedAfterIdentifyAccept`).

**The scanner canonicalizes incoming series names before its name-based comparison.** `shouldUpdateParsedSeries` and `seriesSidecarMatches` still compare names (unlike Identify, which compares resolved IDs), because `FindOrCreateSeries` has side effects and cannot run before the priority gate. So `canonicalAttachedSeriesName` in `scan_unified.go` first maps an incoming series name (parsed metadata, enricher result, or sidecar) to the attached Series' current name when it matches that Series' name or one of its Aliases. Without it, renaming a Series while keeping the old name as an Alias made every scan delete and reinsert the membership and, without the Alias, create a duplicate Series. Restoring or replacing a membership during a resync also triggers reorganization (`seriesChanged`) when the book has a main CBZ, since the series number is part of the organized folder name for CBZ and hybrid books.

### OPDS

- OPDS v1.2 server hosted in the application
- As new functionality is added, keep the OPDS server up-to-date with the new features
- **Cover URLs in feeds must point at `/opds/v1/books/:id/cover`**, not the books API. OPDS uses Basic Auth while `/api/books` requires session auth. The Go server serves OPDS directly; bare `/books/*` paths belong to the SPA. The cover endpoint lives in `pkg/opds/handlers.go` (`bookCover`) and is built off `apiBase + "/opds/v1"` in `bookToEntryWithKepub` so an `X-Forwarded-Prefix` from a trusted prefix-stripping proxy is preserved. Vite does not set that header.

### eReader Browser UI (`pkg/ereader/`)

Server-rendered HTML pages for stock eReader browsers (Kobo, Kindle) that can't use OPDS or the React frontend.

**Key files:**
- `handlers.go` - HTTP handlers mirroring OPDS structure
- `templates.go` - Go string templates for HTML rendering
- `middleware.go` - API key authentication from URL path. Loads the key's owner once and stores it in context; handlers read library access from it (see Best Practice 10 under Authentication & Authorization)
- `routes.go` - Routes under `/ereader/key/:apiKey/*`

**eReader Browser Limitations:**
- No flexbox/modern CSS
- Minimal JavaScript support
- Cookies cleared on browser close (Kobo)
- No Basic Auth support

**Styling for Simple Browsers:**
- Use inline styles instead of CSS attribute selectors (`input[type="text"]`)
- Use `display: block` explicitly for links that should be block-level
- Stack form elements vertically (input on one line, button on next)
- Large tap targets: 12px+ padding on buttons/links
- Explicit borders (2px solid #000) for visibility
- Full-width elements (`width: 100%`) instead of percentages
- Use `<input type="submit">` instead of `<button>` for better compatibility

**Cover Images:**
- eReader routes use API key auth, so covers need their own endpoint at `/ereader/key/:apiKey/cover/:bookId`
- Cannot use `/api/books/{id}/cover` (requires session auth)
- Selection goes through `pkg/covers.SelectFile`, shared via `pkg/covers.ServeBookCover` with the books, series, OPDS, and eReader handlers. The shared selector must continue to fall back to any available cover type when the preferred aspect ratio has no covers, because eReaders showing an audiobook-only book still need to get a cover

### Authentication & Authorization

The app uses Role-Based Access Control (RBAC) with two layers:
1. **Global permissions** - Role-based access to features
2. **Library access** - User-specific library visibility

#### Permission Resources (`pkg/models/role.go`)

| Resource | Description | Used For |
|----------|-------------|----------|
| `libraries` | Library management | Create/update libraries, filesystem operations. `read` covers the libraries family except the list (also open to `users:write`) and per-library languages (`books:read`) |
| `books` | Book/file operations | Books, files, covers, chapters, genres, tags, publishers, search, and a list's books. `read` also covers the review criteria (or `config:read`), identifier types, hook order, and library languages lookups; `write` covers plugin Identify (search and apply) and the Audnexus chapter lookup |
| `people` | Author/narrator management | Create/update/delete/merge people |
| `series` | Series management | Update/delete/merge series |
| `users` | User administration | Create users, manage roles, reset passwords. `write` also lists libraries (`GET /api/libraries`) to assign access. List sharing and `GET /api/users/directory` need no users permission |
| `jobs` | Background jobs | Trigger scans, view job status. Not needed for `bulk_download` (see below) |
| `config` | Application config and admin tools | `read`: view app configuration, list caches, view logs and live log events, read the sharing settings and the review criteria, and the plugin manager's reads (`plugins.RegisterReadRoutes`: installed, available, repositories, and an installed plugin's config with secrets masked, fields, manifest, and image). `write`: plugin management mutations, clear caches, edit the review criteria and the sharing settings |
| `shares` | Share Links | View (`read`) and create, revoke, delete (`write`) a book's Share Links. Either operation also lists a book's links and reads the sharing settings, so a write-only role can still copy what it creates |

#### Permission Operations

- `read` - View/retrieve data
- `write` - Create/modify/delete data

#### Predefined Roles

| Role | Permissions |
|------|-------------|
| `admin` | All 16 permissions (full access) |
| `editor` | Read+write: libraries, books, series, people (8 permissions) |
| `viewer` | Read-only: libraries, books, series, people (4 permissions) |

#### Adding Permissions to Routes

**Group-level permission (all routes in group):**
```go
api := e.Group("/api")
booksGroup := api.Group("/books")
booksGroup.Use(authMiddleware.Authenticate)
booksGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
```

**Individual route permission:**
```go
g.POST("/:id", h.update, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
```

**Any of several permissions:**
```go
g.GET("/sharing", h.get, authMiddleware.RequireAnyPermission(
    auth.Permission{Resource: models.ResourceShares, Operation: models.OperationRead},
    auth.Permission{Resource: models.ResourceConfig, Operation: models.OperationRead},
))
```

Use the any-of form when two kinds of page read the same data: `GET /api/settings/review-criteria` takes `books:read` (review panel) or `config:read` (settings page), and `GET /api/libraries` takes `libraries:read` or `users:write` (library access picker). When the any-of applies to only some routes of a family, give those routes their own group rather than weakening the family's group (`libraryListGroup` in `pkg/server/server.go`).

**Authenticated-only groups:** a route that returns only the caller's own data, or data every signed-in user may see, goes on a group that runs `Authenticate` and no permission. `/api/user/api-keys`, `/api/user/libraries` (the caller's accessible libraries as `LibrarySummary` rows, no paths), `/api/users/directory` (active users' `id` and `username` only; not registered in Demo Mode), `/api/lists`, and the Share Link management routes (whose `shares` checks sit on each route) work this way. Such a route must narrow its payload to what any role may see: scan into a struct that holds only the returned fields (`users.Service.ListDirectory`, `models.UserRef`), rather than blanking or excluding columns on a full model, which still emits its other keys as zero values.

**Library access check (for routes with library ID param):**
```go
g.GET("/:id", h.retrieve, authMiddleware.RequireLibraryAccess("id"))
```

#### Handler-Level Permission Checks

For inline permission checks (e.g., when feature depends on multiple permissions):
```go
user, ok := c.Get("user").(*models.User)
if !ok {
    return errcodes.Unauthorized("User not found in context")
}
if !user.HasPermission(models.ResourceUsers, models.OperationRead) {
    return errcodes.Forbidden("You need users:read permission for this action")
}
```

`errcodes.Forbidden` (like every other `errcodes` constructor) uses its argument verbatim as the user-facing message, so pass a full sentence. It used to append " is not allowed.", which produced toasts like "You don't have permission to read jobs is not allowed."

#### Handler-Level Library Access Checks

When library ID comes from fetched data (not URL param):
```go
file, _ := h.bookService.RetrieveFile(ctx, opts)
if user, ok := c.Get("user").(*models.User); ok {
    if !user.HasLibraryAccess(file.LibraryID) {
        return errcodes.Forbidden("You don't have access to this library")
    }
}
```

#### Best Practices

1. **New routes MUST consider permissions** - Ask: what resource does this affect? What operation?
2. **Routes returning book/file data need library access checks** - Either via middleware or inline
3. **Search endpoints need explicit `books:read`** - Search returns book data, so require the permission. Global search (`GET /api/search`) also fills its `series` and `people` sections only for roles holding `series:read` and `people:read` (`search.GlobalSearchSections`); a section left out is an empty array, not a missing key
4. **User-scoped resources don't need global permissions** - Lists, API keys, settings are user-scoped
5. **List sharing needs no users permission** - The list handlers check owner or manager permission on the list (`CanManage`). Recipients come from `GET /api/users/directory`, and `createShare` refuses an unknown or inactive `user_id` with one `422` so the route cannot probe for accounts. Every user embedded in a list payload (`List.User`, `ListShare.User` and `SharedByUser`, `ListBook.AddedByUser`) is a `models.UserRef` (`id` and `username` only), because any role can now share a list and its recipients read those payloads. Do not point these relations back at `models.User`. Do not re-add `users:read` to the share handlers; it made sharing impossible for every stock role but Admin. `GET /api/lists/:id/books` requires `books:read` because it returns book data. Share Links use the separate `shares` resource instead
6. **Both frontend and backend checks required** - Backend for security, frontend for UX
7. **Read-only lookups used on shared pages must not inherit an admin group's permission** - A GET called by pages that every role can open (for example `GET /api/plugins/identifier-types`, rendered on book and file pages, and `GET /api/plugins/order/:hookType`, read by the identify dialog) belongs in its own group with the read permission its consumers hold (`books:read` here). Registering it inside the `config:write` plugin management group returns 403 to editors and viewers, and the frontend fails silently.
8. **Bulk download does not need Jobs permissions** - The `/api/jobs` group only authenticates. `GET /api/jobs` and `GET /api/jobs/:id/logs` require `jobs:read` per route. `POST /api/jobs` requires `jobs:read` and `jobs:write` in the handler, except `bulk_download`, which requires `books:read` plus library access to every existing requested file and stores only `file_ids` and `estimated_size_bytes` with no `library_id`. `GET /api/jobs/:id` and `/:id/download` allow `jobs:read` or the job's creator (`jobs.created_by_user_id`) for a `bulk_download` job (`canReadJob`), and return 404 otherwise so job IDs cannot be probed. Do not re-add a group-level `jobs:read` middleware; it breaks bulk download for editors and viewers.
9. **Device routes (Kobo, eReader, OPDS) that load an entity by id must re-check scope and library access** - The id in the URL is attacker-chosen, so check it against the same rules the route uses to list entities. `kobo.Service.FileInScope` is the reference: it reuses `scopedFilesQuery`, the query behind the sync, so the download, cover, and metadata routes can only serve files the key syncs, and returns `errcodes.NotFound("File")` otherwise (never 403, never the Kobo store proxy). An entity nested under a library path (a series under `/libraries/:id/series/:id`) must belong to that library, or 404. Do not copy the web handlers' `if user, ok := c.Get("user")...` shape into key-authenticated routes, because it fails open when no user is set.
10. **API key middleware loads the key's owner** - Kobo and eReader `APIKeyAuth` call `apikeys.Service.AuthenticateOwner`, which loads the owner with `Role`, `Role.Permissions`, and `LibraryAccess`, returns 401 for a missing or deactivated owner and 403 without `books:read`, and stores the owner in the request context (`kobo.GetUserFromContext`, `ereader.GetUserFromContext`). Handlers reuse that user instead of re-querying it, and treat a missing one as 401. The eReader `/e/:shortCode` redirect also calls `AuthenticateOwner` before revealing the key URL. A new API key middleware must do the same.

#### Permission Check Flow

```
Request → Authenticate → RequirePermission → RequireLibraryAccess → Handler
                             ↓                      ↓
                        Role check            User library access
```

#### Adding a New Permission Resource

1. Add constant to `pkg/models/role.go`:
   ```go
   const ResourceNewFeature = "newfeature"
   ```
2. Add it to `roles.ValidResources` in `pkg/roles/service.go`, or creating or updating a role with it fails validation
3. Seed it onto the admin role in a migration (`20260927000000_add_shares_permission.go` is the reference)
4. Update `app/components/library/PermissionMatrix.tsx` to display in UI
5. Add permission checks to relevant routes/handlers
6. List it in `website/docs/users-and-permissions.md` and the resource table above

### API Conventions

- **JSON field naming**: All JSON request and response payloads use `snake_case` for field names (e.g., `created_at`, `last_accessed_at`, not `createdAt`). Exception: plugin manifest and repository-index passthrough fields keep their camelCase wire format (see the Plugin API surface amendment in ADR 0004)
- Go struct tags should use `json:"snake_case_name"` format

- **Errors are built with `pkg/errcodes`, never `echo.NewHTTPError`.** The error handler turns an `echo.HTTPError` into a wire code by snake-casing its message (`invalid library ID` became `invalid_library_id`), so each call site invents a code. Use the constructor for the status (`errcodes.NotFound`, `BadRequest`, `ValidationError`, `Forbidden`, `Conflict`, and so on). A server fault, such as a response writer that cannot flush, returns a plain `errors.New` or `errors.WithStack` error, which renders as `internal_server_error`. This applies to the test-only routes in `pkg/testutils` too.
- **`errcodes.NotFound` takes the resource noun only.** It appends " not found.", so pass `"Download file"`, not `"Download file has expired from cache"`, which rendered "Download file has expired from cache not found."
- **A numeric path or scope ID that does not parse returns `errcodes.NotFound(resource)`**, the same 404 as a well-formed ID with no row, because it names no row either. `RequireLibraryAccess` returns the same 404 for a library ID param that does not parse, so the handlers behind it agree with it. Keep `ValidationError` for payload and query values, such as a non-numeric page number. String path IDs checked for path safety are the exception: the plugin image and manifest handlers return `ValidationError("Invalid scope or plugin ID")` for a scope or plugin ID containing `..` or a slash, because that is a rejected traversal attempt, not a missing row.

- **Response shapes are named Go structs generated to TS via tygo (no anonymous responses).** Go is the single source of truth for every request and response shape; the frontend imports the generated type and never restates it. See ADR 0004 (`docs/adr/0004-tygo-generated-api-types.md`). Rules:
  - Every request/response payload is a named, exported struct in the package's `types.go`. No handler returns an anonymous struct, `echo.Map`, or `map[string]any`. A response carrying nothing the client cannot derive returns `204 No Content`.
  - **Reuse the model by embedding it with `tstype:",extends"`** instead of re-listing fields. Embed by **value** (`models.Genre`), not pointer: a pointer embed (`*models.Genre`) generates `extends Partial<Genre>` in TS, which is wrong for a response that always carries those fields. Shadowing fields (same `json` tag) keep the wire format byte-identical to a hand-built struct.
  - **A value embed needs two `tygo.yaml` additions** for the package entry: a `frontmatter` import of the model (e.g. `import { Genre } from "@/types";`), and a `type_mappings` entry mapping the package-qualified selector to the bare interface name (e.g. `models.Genre: "Genre"`). Without the mapping, tygo emits `extends models.Genre` (an unresolved reference); without the frontmatter, `Genre` is undefined.
  - **Reshaped model relations get `tstype:"-"` on the model field.** When a response returns a relation in a different shape than the model (e.g. `aliases []string` vs the model's `Aliases []*GenreAlias`), exclude the model's relation from TS generation with `tstype:"-"` (keep the `json` tag, the Go wire format is unchanged) so the generated `Entity` interface drops the relation and the response's `extends` does not collide. Only safe when no consumer reads that relation as objects.
  - **Naming**: single-resource `{Entity}Response`; list envelope `List{Entities}Response` shaped `{ items, total }`; a list-item shape that genuinely differs from the single-resource shape `{Entity}ListItem`.
  - **Bare-model rule**: a single-resource endpoint returns the bare generated model when the response adds nothing to it. An `{Entity}Response` wrapper is required only when the response reshapes or extends the model (computed fields like `book_count`, flattened relations like `aliases []string`). Existing practice: books, files, users, and roles return bare models; don't flag those in review, and don't add passthrough wrappers like `UserResponse`/`RoleResponse`.
  - **Two-tier collection rule**: paginated list endpoints return the `{ items, total }` envelope; unpaginated full-collection endpoints return a bare array of a named type. Existing practice: book lists, list shares, list templates, library languages.
  - **Same-package embeds need no frontmatter/type_mappings**: when a response embeds a struct from its own package with `tstype:",extends"` (e.g. `plugins.AnnotatedPluginVersion` embedding `PluginVersion`), tygo emits the bare name directly — the two `tygo.yaml` additions are only required for cross-package embeds.
  - **Request payloads may carry `,omitempty` purely for tygo optionality**: an optional non-pointer field on a request struct (e.g. `plugins.InstallPluginPayload.Name`) generates as required `name: string` unless the json tag has `omitempty`. Requests are only ever unmarshaled by the server, so adding `omitempty` there has no wire-format effect — unlike on response structs, where it changes what gets marshaled. Reference: `pkg/plugins/types.go`.
  - **Tri-state nullable pointers get `tstype:"string | null"`**: a `*string` field without `omitempty` generates as `field?: string` by default, which hides that `null` is a legal wire value with distinct meaning (omit = leave untouched, null = clear, string = set). Tag the field `tstype:"string | null"` so the generated type captures the contract: `field?: string | null`. Apply it to both the payload and the response that echo the field. Reference: `SortSpec` in `pkg/settings/types.go` (`UpdateLibrarySettingsPayload`, `LibrarySettingsResponse`).

  Reference implementation: `pkg/genres/types.go` (`GenreResponse`, `ListGenresResponse`), `pkg/models/genre.go` (`Aliases` field tagged `tstype:"-"`), and the genres entry in `tygo.yaml`. The wire-level regression net is `TestList_ResponseAliasesSerializeAsStringArray` in `pkg/genres/handlers_test.go`.

  **Hierarchical / multi-shape reference: `pkg/publishers/`.** Publishers extend the genres pattern with a distinct list-item vs detail shape and a hierarchy:
  - **List item differs from detail**, so it's a `PublisherListItem` (light: `file_count`, `descendant_file_count`, `descendant_publisher_count`, `parent_name`, `aliases []string`) returned in `ListPublishersResponse`; the full `PublisherResponse` adds `ancestors`, `descendant_ids`, and flattened `children`. Computing the full hierarchy per list row would be an N+1, which is exactly why the two shapes exist.
  - **Two reshaped relations get `tstype:"-"`**: `Publisher.Aliases` (→ `[]string`) AND `Publisher.Children` (→ flattened `ChildResponse`, not `Publisher[]`). `Parent` stays as `Publisher` (not reshaped).
  - **Shared builder**: `buildPublisherResponse(ctx, publisher)` (a `*handler` method) assembles the full `PublisherResponse` and is called by retrieve, update, AND merge so every mutation returns the same full shape (enabling client `setQueryData`). When adding a mutation that returns the entity, route it through this helper rather than hand-building a partial struct. Its unit test is `TestBuildPublisherResponse_FullHierarchy` in `pkg/publishers/handlers_test.go`.
  - **Sub-resource list of a foreign model**: the files sub-resource returns `ListPublisherFilesResponse` with `Items []*models.File`. Tag the slice field with a `tstype:"File[]"` override and add the `frontmatter` import of `File`, so tygo emits a clean `items: File[]` instead of `(any /* models.File */ | undefined)[]`. Use the field-level override rather than a global `models.File` `type_mappings` entry — the override keeps the mapping local to the one field that needs it and yields `File[]` rather than the looser `(File | undefined)[]` a bare selector mapping produces. This mirrors `ListTagBooksResponse.Items []*models.Book` (tagged `tstype:"Book[]"`) in `pkg/tags/types.go`.
- **`//tygo:emit` directives only fire on `const` and `type` declarations.** tygo skips `var` declarations entirely, so an emit comment attached to a `var` is silently dropped from the generated TS. When a Go `var` (e.g. an allowed-values slice like `models.PlaybackSpeeds`) needs a TS mirror, attach the emit lines to an adjacent `const` or `type` declaration and note that the two must stay in sync. Reference: the `PlaybackSpeeds` const block in `pkg/models/user_settings.go`.
- **Request binding must use structs**: The custom binder (`pkg/binder/`) uses mold (conform) and validator, which only work with structs. Never bind directly to a slice/array — wrap it in a struct:

```go
// ❌ WRONG - mold can't process a slice, causes nil pointer error
var entries []orderEntry
if err := c.Bind(&entries); err != nil { ... }

// ✅ CORRECT - wrap in a struct
type setOrderPayload struct {
    Order []orderEntry `json:"order" validate:"required"`
}
var payload setOrderPayload
if err := c.Bind(&payload); err != nil { ... }
```

- **Slice fields need `mod:"dive"` for inner modifiers to fire**: `mold/v4` (used by the binder for `mod:"trim"` etc.) treats slice/array/map/pointer-to-slice fields as opaque single values by default. If a payload field is shaped like `[]Inner` or `*[]Inner` and `Inner` has any `mod:"..."` tags on its fields, the parent slice field must carry `mod:"dive"` — otherwise the inner modifiers are silently no-ops. This mirrors `validator/v10`'s `dive`, but the two are independent: both tags are required on the same field when both validation and modification need to traverse. Reference: `UpdateFilePayload.Identifiers *[]IdentifierPayload` in `pkg/books/types.go` (carries `mod:"dive" validate:"omitempty,dive"`); regression test in `pkg/binder/binder_test.go` (`TestBind_DiveRequiredForSliceModTraversal`).

- **Bun table aliases in WHERE/ORDER clauses**: Bun auto-aliases tables using the first letter of the table name (e.g., `books` → `b`, `files` → `f`, `persons` → `p`). Always use these aliases in `Where()`, `Order()`, and other SQL clauses — never use the full table name:

```go
// ❌ WRONG - "book" is not a valid alias, causes "no such column" error
q.Where("book.id = ?", id)

// ✅ CORRECT - Bun aliases "books" table as "b"
q.Where("b.id = ?", id)
```

Check existing queries in `pkg/books/service.go` for reference. Common aliases: `b` (books), `f` (files), `a` (authors), `p` (persons), `s` (series), `bs` (book_series), `ch` (chapters), `n` (narrators).

- **Unified `LogLevel` enum (`pkg/models/log_level.go`)**: A single `LogLevel` (`debug`, `info`, `warn`, `error`, `fatal`) is the canonical log-severity enum, shared by `models.JobLog.Level` (job logs) and `logs.LogEntry.Level` (app/server logs). There is no separate `JobLogLevel` — use `models.LogLevel*` consts everywhere, and `tstype:"LogLevel"` on level fields / `tstype:"LogLevel[]"` on level query filters. Both level query validators (`joblogs.ListJobLogsQuery.Level`, `logs.ListLogsQuery.Level`) carry the full `oneof=debug info warn error fatal`. The frontend imports `LogLevel*` from `@/types` (generated into `models.ts`).

- **Jobs / job logs / app logs follow the `{ items, total }` envelope** like every other list endpoint: `jobs.ListJobsResponse` (`items: Job[]`), `joblogs.ListJobLogsResponse` (`items: JobLog[]`), and `logs.ListLogsResponse` (`items: LogEntry[]`). The foreign-model slices use a field-level `tstype:"Job[]"` / `tstype:"JobLog[]"` override plus a frontmatter import (the publishers `ListPublisherFilesResponse` pattern) so tygo emits a clean `Job[]` rather than `(any | undefined)[]`. **The joblogs list response no longer bundles the `job`** — the handler only does an existence check (404 on unknown job); the client fetches the job separately via `GET /api/jobs/:id` (the `useJob` hook). `POST /api/auth/logout` returns `204 No Content` (pure acknowledgment), not a JSON message body.

### Config

- Self-hosted app with config file-based configuration
- Each config field is also configurable by environment variables
- **CRITICAL**: If a new field is added to `config.Config` in `pkg/config/config.go`:
  - `shisho.example.yaml` **MUST** be updated with the new field, its env var name, default value, and a description. This file must always be a complete reference of all configurable fields.
  - Exception: `environment` is a test-only internal field and should NOT be included in `shisho.example.yaml`.
  - The Server Settings UI page (`app/components/pages/AdminSettings.tsx`) must be updated to display the new field (all non-secret config fields should be shown)

### Sidecars

- **Series number groups are atomic:** `series_number`, `series_number_end`, and `series_number_unit` must always come from one metadata source. Copy, merge, clear, validate, and sidecar-overlay all three together. An end requires a finite start, both endpoints must be finite, and external ranges require end greater than start. Malformed external groups (hook results, sidecars) are discarded as a whole; the Identify apply endpoint instead rejects them as a validation error in both the array and scalar shapes (see `pkg/plugins/AGENTS.md`).
- Sidecar metadata files kept for every file parsed into the system
- Don't store non-modifiable intrinsic properties (e.g., bitrate, duration)
- Source fields (e.g., title_source, name_source) shouldn't be saved into the sidecar

### Request Context Propagation

**Always pass `context.Context` through to long-running operations** to ensure request cancellations are respected. When a client disconnects or cancels a request, Go's context gets cancelled automatically - but only if we propagate it.

**Pattern:**
```go
// In handlers - get context from Echo
func (h *Handler) downloadFile(c echo.Context) error {
    ctx := c.Request().Context()
    result, err := h.service.GenerateFile(ctx, fileID)
    // ...
}

// In services/utilities - accept and use context
func (s *Service) GenerateFile(ctx context.Context, fileID int) (*File, error) {
    // Check for cancellation at key points
    if err := ctx.Err(); err != nil {
        return nil, err
    }
    // Pass context to downstream operations
    return s.generator.Generate(ctx, file)
}
```

**Key points:**
- Handlers get context via `c.Request().Context()`
- Pass context as the first parameter to functions that do significant work
- Check `ctx.Err()` before expensive operations (file I/O, loops over content)
- Return early with `ctx.Err()` if cancelled - don't cache partial results

### Publishing Cache Files

Concurrent requests read the reader and download caches, so a cache file must never be visible at its final path until it is complete. For bytes already in memory, use `fileutils.WriteFileAtomic`. For streamed output, write under a unique temporary name in the same directory (`os.CreateTemp`), close it, then `os.Rename` it into place. The temporary name must not match whatever the cache-hit check looks for (`cbzpages` globs `page_<n>.*`, so its temp files start with `.extracting-`). A fixed `dest + ".tmp"` name is not enough: two requests share it and truncate or remove each other's output.

`downloadcache.Cache` goes further because generation is expensive: `getOrGenerate` serializes work per destination path with a context-aware keyed lock, rechecks the cache after acquiring it, has the generator write into a private `.staging-*` directory, renames the result into place, and only then writes metadata (also via temp+rename). Concurrent cold reads of one file generate it once; unrelated files proceed in parallel. The lock lives on the `Cache` instance, so every consumer (books, OPDS, eReader, Kobo, the bulk download worker) must share the one `dlCache` built in `cmd/api/main.go` rather than calling `NewCache` for the same directory. New generated formats should go through `getOrGenerate` rather than calling a generator against the final path. A process killed mid-write leaves its temporary files behind; nothing sweeps them automatically, and the admin cache clear removes them.

## File Retrieval and Relations

**CRITICAL**: When calling `WriteFileSidecarFromModel()`, `WriteBookSidecarFromModel()`, or `ComputeFingerprint()`, the model MUST have all relations loaded:

| Function | Required Relations |
|----------|-------------------|
| `WriteFileSidecarFromModel()` | Narrators, Identifiers, Publisher, Chapters |
| `WriteBookSidecarFromModel()` | Authors.Person, BookSeries.Series, BookGenres.Genre, BookTags.Tag |
| `ComputeFingerprint()` | Narrators, Identifiers |

**Use the right retrieval method:**
- `RetrieveFile()` - File with Book, Identifiers, and Narrators. Use for most lookups.
- `RetrieveFileWithRelations()` - Complete file with all relations (adds Publisher, Chapters). **Use this for sidecar writing or fingerprinting.**
- Book queries (`RetrieveBook`) - Already include `Files.Identifiers`, `Files.Narrators`, etc.

**Common mistake**: Retrieving a file with `RetrieveFile()` then passing it to `WriteFileSidecarFromModel()` or `ComputeFingerprint()`. The sidecar/fingerprint will be missing data because relations aren't loaded.

**Correct pattern:**
```go
// For sidecar writing after file updates
file, _ := h.bookService.RetrieveFileWithRelations(ctx, file.ID)
sidecar.WriteFileSidecarFromModel(file)

// For fingerprinting in download handlers - use file from book.Files
book, _ := h.bookService.RetrieveBook(ctx, opts)
for _, f := range book.Files {
    if f.ID == targetFileID {
        downloadcache.ComputeFingerprint(book, f) // f has all relations
    }
}
```

## Adding or Modifying Metadata Fields

**Invoke the `metadata-field` skill when adding or significantly modifying a metadata field on books or files.** The skill walks you through discovery (finding existing touchpoints via grep, not a static list), planning, implementation order, and — most importantly — a verification phase that catches parallel code paths that would otherwise be missed.

Do not rely on a static checklist here: the codebase has accumulated multiple parallel code paths (three separate JS→Go parsers in the plugin bridge, several merge/filter/persist functions in the scanner, per-format file generators, OPDS, Kobo sync, frontend edit/display/filter/identify-review paths) and a hand-maintained list drifts out of date. The skill uses grep against an existing similar field as the authoritative source of truth.

## File-Level vs Book-Level Fields

Some metadata exists at both the book level and file level (e.g., `book.Title` vs `file.Name`). When both exist:

- **Download filenames**: Prefer file-level field (e.g., `file.Name` over `book.Title`)
- **File organization**: Prefer file-level field for individual file naming
- **Display**: Show file-level field in file-specific contexts, book-level in book contexts

**Pattern for organization/download:**
```go
// Use file.Name for title if available, otherwise book.Title
title := book.Title
if file.Name != nil && *file.Name != "" {
    title = *file.Name
}
```

## Triggering File Reorganization

When a metadata field that affects file paths is edited via API, trigger file reorganization if the library has `OrganizeFileStructure` enabled.

**Fields that trigger reorganization:**
- `file.Name` - affects the filename portion
- `file.Narrators` - affects the filename for audiobooks
- `book.Authors` - affects the directory structure
- `book.Title` - affects the directory structure
- `book.BookSeries` membership and series number fields - affect CBZ and hybrid book folder suffixes

Path-affecting removal operations must also trigger reorganization. For example, Identify represents clearing all series memberships as a present but empty series collection, which must remain distinct from an absent series field.

For directory-backed books, folder organization owns the book sidecar. Rename the files inside that folder with `RenameOrganizedFileOnly`, not `RenameOrganizedFile`. A file previously moved from the library root can carry a leftover basename-based book sidecar. Renaming that sidecar during a narrator change can overwrite the current folder-based sidecar and restore stale metadata on the next Scan.

**Pattern in handlers:**
```go
// After updating the field
if fieldChanged && library.OrganizeFileStructure {
    // Build OrganizedNameOptions with current metadata
    organizeOpts := fileutils.OrganizedNameOptions{
        AuthorNames:   authorNames,
        NarratorNames: narratorNames,
        Title:         title,  // Use file.Name if available
        FileType:      file.FileType,
    }
    newPath, err := fileutils.RenameOrganizedFileOnly(file.Filepath, organizeOpts)
    if err != nil {
        // Handle error
    }
    // Update file.Filepath in database
}
```

## Adding New Entity Types

When adding a new entity type (like Publisher, Genre, Tag) that files or books reference:

1. Create model in `pkg/models/` with appropriate fields and Bun struct tags
2. Create service in `pkg/{entity}/service.go` following the pattern from `pkg/genres/service.go`:
   - Include `FindOrCreate{Entity}()` method for scanner to use
   - Include `Retrieve{Entity}()` and `List{Entity}s()` methods
3. Add service to worker (`pkg/worker/worker.go`) and initialize in `New()`
   - If the entity can be orphaned, add its cleanup and FTS removal to `books.CleanupOrphanedEntities` (`pkg/books/orphans.go`)
4. Update scanner to use the new service for entity creation

## Search Index (FTS)

The app uses SQLite Full-Text Search (FTS5) for fast searching.

**Key files:**
- `pkg/search/service.go` - Search service with index methods
- FTS tables: `books_fts`, `series_fts`, `persons_fts`, `genres_fts`, `tags_fts`, `publishers_fts`

**FTS rows are keyed by rowid = entity id.** Every insert into an FTS table sets `rowid` explicitly to the entity id (`INSERT OR REPLACE INTO books_fts (rowid, book_id, ...) VALUES (?, ?, ...)`, or `SELECT b.id AS rowid, b.id, ...` in bulk), and every per-entity delete filters on `WHERE rowid = ?` through `deleteFTSRow`. The stored id columns (`book_id`, `series_id`, and so on) are `UNINDEXED`, so `WHERE book_id = ?` scans the whole table and made per-Book re-indexing superlinear on large libraries (#534). Keep the id columns because search queries read them, but never filter a per-entity delete on them. A new insert path that leaves `rowid` unset breaks later deletes of that row. Use `OR REPLACE` so two writers indexing the same entity at once converge on one row instead of failing on the rowid conflict. Migration `20260927153000` rebuilt existing rows into this shape.

**One rule for keeping the index current: collect affected ids before mutating, call `ReindexAffected` after commit, in a defer.** FTS tables have no triggers (re-aggregating `GROUP_CONCAT` inside every write transaction is too costly during scans and person renames), so every write path reindexes explicitly:

```go
affected := h.searchService.CollectAffected(ctx, search.Affected{BookIDs: []int{id}})
defer h.searchService.ReindexAffected(ctx, affected)
// ... mutate; append ids learned along the way, e.g.
affected.BookIDs = append(affected.BookIDs, newBook.ID)
```

- `search.Affected` holds `BookIDs`, `SeriesIDs`, `PersonIDs`, `GenreIDs`, `TagIDs`, and `PublisherIDs`. Name the entities the mutation touches; the service expands the rest. A Person expands to the Books it authors or narrates, a Series to its Books, and every Book, given or expanded, to the Series holding it. Genres, Tags, and Publishers reindex their own rows only.
- `CollectAffected` reads that expansion before the mutation, because a delete or relink drops links (CASCADE removes a deleted Book's `book_series` rows) and the far side is then unreachable. `ReindexAffected` expands again afterwards, so links the mutation added are covered too, rewrites each row once, and deletes the row of an entity that no longer exists, so a caller need not also call `DeleteFrom*Index` for an entity it passed in. Orphan cleanup and the Genre, Tag, and Publisher delete handlers still remove their own rows with `DeleteFrom*Index`; running both is harmless.
- The `defer` is the point: a rename commits before `SyncAliases` can reject the aliases, and a plugin apply commits the title before a duplicate series fails it. The reindex must run on those error paths too. It detaches from ctx cancellation, logs failures, and never fails the request.
- Keep transaction boundaries and source stamps as they are. The reindex runs after them.
- Both methods are nil-safe, so worker code needs no `searchService != nil` guard around them.
- A full scan does not index as it goes (FilePath mode, `isResync=false`); `ProcessScanJob` runs `RebuildAllIndexes` when it finishes, and in a defer when it fails, so what it committed is searchable. A scan cancelled by shutdown skips that rebuild, since it would race the shutdown deadline; instead `fetchJobs` calls `rebuildSearchAfterIncompleteScan` at startup, which rebuilds when the most recent scan job that started is not `completed`. `RebuildAllIndexes` runs in one transaction, so a failure leaves the old index in place. The database pool has one connection, so every other query waits for the whole rebuild (a few seconds on a large library).
- A resync (`scanFileCore` with `isResync=true`) collects and defers the reindex of the Book and its Series like a handler; `indexBookRelations` only writes the rows of newly attached People, Genres, Tags, and Publishers. A changed file that a full scan hands to `scanFileByID` skips both, because `ProcessScanJob` marks its ctx with `withSearchRebuildPending` and its closing rebuild covers every row.
- New entities created through `FindOrCreate*()` still need their own row: index them with `Index*()` where they are attached (the book update handler, `indexBookRelations` in the scanner, the plugin relationship helpers) or add their ids to `Affected`.
- Orphan cleanup after a Book or File delete, a Scan, or a monitor batch goes through `books.CleanupOrphanedEntities`, which deletes every unreferenced Series, Person, Genre, Tag, and Publisher and removes each from its FTS table; call it from any new delete path rather than pasting the per-kind loop. Deleting one File of a Book that keeps other Files runs `books.CleanupOrphanedPeople`, the people kind alone. The Book and File update handlers clean up only the kinds the edit changed, so they keep those targeted loops inline. `CleanupOrphanedSeries` lives on `books.Service` only, because `pkg/series` imports `pkg/books`.

**Every indexer shares one SQL statement per table.** `pkg/search/service.go` holds one `INSERT ... SELECT` per FTS table. `RebuildAllIndexes` runs it unfiltered, and the per-entity methods (`ReindexBookByID`, `ReindexSeriesByID`, and the `Index*` methods, which read only the model's ID) append `WHERE <alias>.id = ?`. An edit and a scan therefore write identical rows, and indexing never depends on which relations a caller loaded. `TestIndexMethods_MatchRebuildAllIndexes` diffs the per-entity paths and `ReindexAffected` against the rebuild; extend its fixture when adding a column.

**Column matrix.** When a write changes a source column below, every FTS row that copies it is stale until reindexed. `ReindexAffected`'s expansion follows exactly these edges; if a new column adds an edge, extend `expandAffected` in `pkg/search/affected.go`.

| FTS table | Column | Copied from |
|-----------|--------|-------------|
| `books_fts` | `title`, `subtitle`, `filepath` | `books` |
| `books_fts` | `filenames` | `files.filepath` |
| `books_fts` | `authors` | `authors` joined to `persons.name`, plus `person_aliases` |
| `books_fts` | `narrators` | `files`, `narrators`, `persons.name`, plus `person_aliases` |
| `books_fts` | `series_names` | `book_series`, `series.name`, plus `series_aliases` |
| `series_fts` | `name` | `series.name` plus `series_aliases` |
| `series_fts` | `description` | `series.description` |
| `series_fts` | `book_titles` | `books.title` via `book_series` |
| `series_fts` | `book_authors` | `persons.name` of those Books' authors (no aliases) |
| `persons_fts` | `name`, `sort_name` | `persons.name` plus `person_aliases`; `persons.sort_name` |
| `genres_fts`, `tags_fts`, `publishers_fts` | `name` | `name` plus the matching `*_aliases` table |

`pkg/testutils` seeds `books_fts` through `ReindexAffected`, so E2E search sees the same authors and series names as production.

## File Processing Flow

1. **Scan Job Creation**: User triggers scan via API
2. **File Discovery**: Worker scans library paths for native `.epub`, `.m4b`, `.cbz`, and `.pdf` files plus extensions registered by plugin parsers and converters
3. **Metadata Extraction**: Parse files to extract title, authors, cover images
4. **Database Storage**: Create/update Book and File records
5. **Cover Generation**: Save individual covers + generate canonical covers
6. **Priority Resolution**: Use data source priority to resolve metadata conflicts

## Chapters System

Chapters are file-level metadata extracted from CBZ, PDF, EPUB, and M4B files.

### Database Model (`pkg/models/chapter.go`)

```go
type Chapter struct {
    ID               int
    FileID           int       // Foreign key to files table
    ParentID         *int      // Self-referential for nested chapters (EPUB)
    SortOrder        int       // Order within parent (0-indexed)
    Title            string
    StartPage        *int      // CBZ/PDF: 0-indexed page number
    StartTimestampMs *int64    // M4B: milliseconds from start
    Href             *string   // EPUB: content document href
    Children         []*Chapter // Loaded via relation
}
```

### Service Layer (`pkg/chapters/service.go`)

```go
// List chapters for a file, returns nested tree structure
func (svc *Service) ListChapters(ctx, fileID) ([]*models.Chapter, error)

// Replace all chapters for a file (transactional delete + insert)
func (svc *Service) ReplaceChapters(ctx, fileID, []mediafile.ParsedChapter) error

// Delete all chapters for a file
func (svc *Service) DeleteChaptersForFile(ctx, fileID) error
```

### API Endpoints (`pkg/chapters/handlers.go`, `routes.go`)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/books/files/:id/chapters` | List chapters (nested tree) |
| PUT | `/api/books/files/:id/chapters` | Replace chapters (requires write permission) |

### Worker Integration

Chapters are synced during file scan in `pkg/worker/scan_unified.go` (`scanFileCore`):
- After file metadata is saved, chapters from `ParsedMetadata.Chapters` are synced
- Uses `chapterService.ReplaceChapters()` for atomic replacement; a failure fails the file's scan
- **Negative start pages are dropped on scan, whatever the source.** Parsed chapters (built-in parsers and plugin file parsers) and sidecar chapters both go through `dropNegativeStartPageChapters` before `ReplaceChapters`. It drops any chapter with a negative `start_page` along with its children, and the parsed-metadata path warn-logs the file path, chapter source, dropped count, and `stored` (whether the remaining chapters were applied or lost on priority). Sidecar chapters reach it through `convertSidecarChapters`. Older Scans wrote PDF bookmarks with no page into sidecars as `start_page` -1, and plugin file parsers can return any negative `startPage`. Enricher search results do not carry chapters (`parseSearchResponse` never reads them). A stored negative page shows as "Page 0", is skipped by downloads, and makes `validateChapters` reject every chapter save on the file. If every chapter is dropped, the file's existing chapters are left alone (`ShouldUpdateChapters` never applies an empty list).

### Position Fields by File Type

| File Type | Position Field | Example |
|-----------|---------------|---------|
| CBZ | `StartPage` | `0` (first page) |
| PDF | `StartPage` | `0` (first page) |
| M4B | `StartTimestampMs` | `3600000` (1 hour) |
| EPUB | `Href` | `"chapter1.xhtml"` |

### Validation

PUT endpoint validates chapters against file constraints:
- CBZ/PDF: `start_page` must be >= 0 (always) and < `file.PageCount` (when known)
- M4B: `start_timestamp_ms` must be <= `file.AudiobookDurationSeconds * 1000`

## Key Directories

| Purpose | Location |
|---------|----------|
| Entry point | `cmd/api/main.go` |
| Models | `pkg/models/` |
| Domain services | `pkg/{domain}/` (books, jobs, libraries, chapters, etc.) |
| File parsers | `pkg/epub/`, `pkg/cbz/`, `pkg/mp4/`, `pkg/pdf/` |
| File generators | `pkg/filegen/` |
| Scanner/Worker | `pkg/worker/` |
| Sidecars | `pkg/sidecar/` |
| Search | `pkg/search/` |
| Config | `pkg/config/` |
