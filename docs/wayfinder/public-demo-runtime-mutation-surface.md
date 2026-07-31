# Public Demo runtime mutation surface

This inventory supports [Inventory Shisho's runtime mutation surface](https://github.com/shishobooks/shisho/issues/428) in the [Chart a safe public Shisho demo](https://github.com/shishobooks/shisho/issues/427) wayfinding map.

It describes the application at commit `5378616f5c37e922c53d558f8cedc76b03603302`. It is a static code inventory, not an implementation design. Later decision tickets should use it to define the Demo Mode contract and enforcement architecture.

## Scope and classifications

The inventory covers:

- process startup and shutdown;
- authentication and ordinary authenticated requests;
- browsing, search, cover serving, EPUB/CBZ/PDF reading, and M4B playback;
- user-scoped features that bypass ordinary write permissions;
- administration and catalog mutation;
- workers, monitors, caches, plugins, integrations, and outbound traffic;
- server filesystem, SQLite, process-memory, and browser-local state.

Mutation classes used below:

| Class | Meaning for the Public Demo |
|---|---|
| Prepared corpus | Media, covers, sidecars, and the source database shipped by a deployment. These must remain immutable and replaceable by redeployment. |
| Runtime database | The SQLite file actually opened by the process, including WAL and SHM companions. A disposable copy may still be writable even when the prepared source is immutable. |
| Runtime cache | Generated downloads, extracted pages, rendered pages, temporary files, and cache metadata. The map already permits temporary runtime data. |
| Browser-local | Cookies, browser cache, local or session storage, URL history, and in-memory reader state. The map already permits local preferences and reading state. |
| External side effect | Outbound HTTP, plugin-controlled processes, or writes outside the intended database and cache roots. |

## Executive findings

1. Shisho has no single read-only switch. Disabling the library monitor, setting the scan interval to zero, or giving the shared visitor the Viewer role does not cover the complete mutation surface.
2. The process requires writable runtime storage today. Startup creates cache directories, performs a cache write test, opens SQLite in WAL mode, checks FTS support with schema writes, and runs migrations.
3. Ordinary allowed reading journeys write disposable cache state:
   - opening an EPUB uses the generated-download endpoint;
   - opening a CBZ or PDF lazily extracts or renders pages;
   - generated-download cache hits rewrite access metadata;
   - a download `HEAD` request performs the same generation as `GET`.
4. Ordinary browsing, search, detail retrieval, cover serving, original media streaming, and M4B range streaming do not intentionally write domain state.
5. The Viewer role is not a Demo Mode boundary. Any authenticated user can mutate their own settings, per-library sort settings, lists, list memberships, API keys, password, and related user-scoped state without a global write permission.
6. Worker startup is broader than scheduled scans. It also starts job polling and processors, job retention, an immediate plugin update check, a daily update checker, and optionally the library monitor.
7. Plugins and integrations have independent mutation paths. Manual plugin search/apply, plugin administration, Kobo sync and proxy routes, Audnexus lookup, OPDS/eReader/Kobo generated downloads, and manual resync do not all depend on scheduled workers.
8. Public Demo safety therefore needs an authoritative application boundary. Hiding controls in the frontend and relying only on the shared user's role would leave reachable mutation endpoints.

## Startup and shutdown

The process entry point is [`cmd/api/main.go`](../../cmd/api/main.go). The important startup sequence is:

1. Install the process-global logger tee and in-memory log ring.
2. Load configuration.
3. Initialize writable cache paths.
4. Open and configure SQLite.
5. Create and drop an FTS5 test table.
6. Apply pending migrations.
7. Load active plugins.
8. Construct caches, worker, and HTTP server.
9. Start the listener goroutine.
10. Start the worker runtime immediately afterward. Listener bind success is not synchronized before worker startup.

### Startup writes

| Source | Mutation | Target |
|---|---|---|
| `initCacheDir` in [`cmd/api/main.go`](../../cmd/api/main.go) | Creates `downloads`, `downloads/bulk`, and `cbz`; creates and removes `.write_test` | Configured cache directory |
| `writePortFile` in [`cmd/api/main.go`](../../cmd/api/main.go) | Writes `tmp/api.port` when that path exists relative to the process working directory | Development/runtime filesystem |
| `database.New` in [`pkg/database/database.go`](../../pkg/database/database.go) | Opens or creates SQLite, enables WAL, and may create `-wal` and `-shm` files | Runtime database |
| `database.CheckFTS5Support` in [`pkg/database/database.go`](../../pkg/database/database.go) | Creates and drops `_fts5_check` | Runtime database schema |
| `migrations.BringUpToDate` in [`pkg/migrations/migrations.go`](../../pkg/migrations/migrations.go) | Applies pending DDL, data migrations, seed rows, and migration records | Runtime database |
| `plugins.Manager.LoadAll` in [`pkg/plugins/manager.go`](../../pkg/plugins/manager.go) | Executes active plugin code, updates load status/errors, registers identifier types, and appends missing hook order rows | Runtime database and process memory |
| `worker.Start` in [`pkg/worker/worker.go`](../../pkg/worker/worker.go) | Starts background processors and schedulers | Process memory, then targets listed below |

A database mounted strictly read-only is not compatible with this startup path. A prepared database can remain immutable only if startup operates on a writable disposable copy or the application is changed to avoid these writes.

### Shutdown and detached work

Normal shutdown closes the event broker, stops HTTP service, shuts down and joins the worker, and closes SQLite. Several effects are not fully owned by that wait:

- download-cache cleanup goroutines are detached;
- eReader and Kobo API-key access-time updates use detached background goroutines;
- Kobo sync cleanup is detached;
- already-fired monitor debounce callbacks may use background context.

The plugin update checker is joined, but its repository requests use background context with an HTTP timeout. An in-progress fetch can therefore delay worker shutdown after cancellation.

These behaviors matter for scale-to-zero termination and for ensuring no late write reaches an immutable mount.

## Background runtime

`Worker.Start` in [`pkg/worker/worker.go`](../../pkg/worker/worker.go) starts multiple independent activities.

| Activity | Cadence | Effects |
|---|---|---|
| Job fetcher | Every 5 seconds | Queues pending jobs and `in_progress` jobs whose recorded process ID differs from the current process, including work carried over across restarts. |
| Job processors | Process lifetime | Write job status, progress, job logs, and job-specific results. |
| Scan scheduler | Configured interval | Inserts scan jobs when libraries exist. |
| Job retention | Hourly after an initial delay | Deletes old jobs and cascading job logs. |
| Plugin update checker | Immediately, then every 24 hours | Fetches enabled repositories and updates available-version state. |
| Library monitor | Process lifetime when enabled | Watches library paths and invokes scan, move, delete, hash, organization, cover, sidecar, DB, and FTS mutation paths. |

### Job effects

The job system persists lifecycle state in SQLite. Scan jobs can additionally:

- create, update, merge, or delete catalog records and relations;
- update FTS indexes;
- write covers and metadata sidecars beside source media;
- reorganize and rename media and directories;
- invoke plugin parsers, converters, and enrichers;
- perform outbound requests through plugins.

Hash-generation jobs compute SHA-256 values and store file fingerprints. Bulk-download jobs generate native download-cache entries and bulk ZIP files. Review-recomputation jobs update review state and job progress.

The main implementations are in [`pkg/worker/worker.go`](../../pkg/worker/worker.go), [`pkg/worker/scan.go`](../../pkg/worker/scan.go), [`pkg/worker/scan_unified.go`](../../pkg/worker/scan_unified.go), [`pkg/worker/monitor.go`](../../pkg/worker/monitor.go), [`pkg/worker/hash_generation.go`](../../pkg/worker/hash_generation.go), and [`pkg/worker/bulk_download.go`](../../pkg/worker/bulk_download.go).

### Existing controls are partial

| Existing control | Stops | Does not stop |
|---|---|---|
| `sync_interval_minutes=0` | Scheduled scan creation | Job polling, processors, retention, update checker, manual resync, request-driven generation |
| `library_monitor_enabled=false` | Filesystem-event scans | Scheduled jobs, manual resync, request-driven generation |
| Prepared database has no pending or carried-over `in_progress` jobs | Immediate queued execution | Future scheduled jobs, update checks, request-driven writes |
| Disable plugin hooks | Selected hook execution | Repository checks, plugin management, native generation, Kobo, Audnexus |
| Plugin `auto_update=false` | Refreshing that plugin's `update_available_version` during the periodic check | Repository fetches performed before per-plugin filtering when any plugins are installed |

## Authentication and shared-user behavior

Authentication is stateless JWT authentication. Relevant code is in [`pkg/auth/handlers.go`](../../pkg/auth/handlers.go), [`pkg/auth/service.go`](../../pkg/auth/service.go), and [`pkg/auth/middleware.go`](../../pkg/auth/middleware.go).

| Journey | Server mutation |
|---|---|
| `POST /auth/login` | No server-side session row. Sets a browser session cookie. |
| Authenticated request | Validates JWT and reloads the user and relations. Does not refresh a session row or last-login timestamp. |
| `POST /auth/logout` | No database write. Expires the browser cookie. |
| `GET /auth/status` and `GET /auth/me` | Database reads only. |
| `POST /auth/setup` | Creates the first admin and signs in. It is public but only succeeds when no users exist. |
| `POST /users/:id/reset-password` | Writes credentials and reset state. Self-reset requires authentication but no global users permission. |

The shared visitor credential needs a precreated user in the prepared database. Demo Mode must prevent setup and shared-user password mutation even though normal login itself is compatible with an immutable corpus.

## Ordinary visitor journeys

### Journey matrix

| Journey | Current backend behavior | Mutation class | Main references |
|---|---|---|---|
| Browse libraries and Books | SQLite reads | None | [`pkg/libraries/handlers.go`](../../pkg/libraries/handlers.go), [`pkg/books/handlers.go`](../../pkg/books/handlers.go) |
| Search | FTS and relational reads; no lazy indexing or history | None | [`pkg/search/handlers.go`](../../pkg/search/handlers.go), [`pkg/search/service.go`](../../pkg/search/service.go) |
| View Book/File/metadata details | SQLite reads | None | Domain handlers and services under [`pkg`](../../pkg) |
| View covers | Opens existing cover files; no lazy generation | None | [`pkg/covers/covers.go`](../../pkg/covers/covers.go), book and series cover handlers |
| Open EPUB reader | Calls normal generated download endpoint | Runtime cache write | [`app/hooks/queries/epub.ts`](../../app/hooks/queries/epub.ts), `downloadFile` in [`pkg/books/handlers.go`](../../pkg/books/handlers.go) |
| Open CBZ reader | Lazily extracts requested and prefetched pages | Runtime cache write | [`app/components/pages/PageReader.tsx`](../../app/components/pages/PageReader.tsx), [`pkg/cbzpages/cache.go`](../../pkg/cbzpages/cache.go) |
| Open PDF reader | Lazily renders requested and prefetched pages | Runtime cache write | [`app/components/pages/PageReader.tsx`](../../app/components/pages/PageReader.tsx), [`pkg/pdfpages/cache.go`](../../pkg/pdfpages/cache.go) |
| Play M4B | Streams original file with range support | None beyond buffers and logs | `streamFile` in [`pkg/books/handlers.go`](../../pkg/books/handlers.go) |
| Read chapters | SQLite reads | None | [`pkg/chapters/handlers.go`](../../pkg/chapters/handlers.go) |
| Keep EPUB progress | React/Foliate memory only | Browser-local ephemeral | [`app/components/pages/EPUBReader.tsx`](../../app/components/pages/EPUBReader.tsx) |
| Keep CBZ/PDF page | URL query state and browser history | Browser-local | [`app/components/pages/PageReader.tsx`](../../app/components/pages/PageReader.tsx) |
| Keep M4B position | Browser/React state only | Browser-local ephemeral | [`app/components/pages/M4BReader.tsx`](../../app/components/pages/M4BReader.tsx) |
| Change reader preferences | Upserts `user_settings` | Runtime database write | [`pkg/settings/handlers.go`](../../pkg/settings/handlers.go), [`pkg/settings/service.go`](../../pkg/settings/service.go) |
| Save default library sort | Upserts `user_library_settings` | Runtime database write | [`pkg/settings/library_handlers.go`](../../pkg/settings/library_handlers.go) |
| Connect to SSE | Adds an in-memory subscriber and heartbeat ticker | Process memory | [`pkg/events/handler.go`](../../pkg/events/handler.go), [`pkg/events/broker.go`](../../pkg/events/broker.go) |

### Generated download behavior

`downloadcache.Cache` in [`pkg/downloadcache/cache.go`](../../pkg/downloadcache/cache.go) is used by the web API and external-client download surfaces.

On a miss it:

1. generates a rewritten EPUB, CBZ, PDF, M4B, or KePub in `cache_dir/downloads`;
2. writes JSON metadata beside it;
3. uses temporary files and renames where supported;
4. starts detached cache cleanup.

On a hit it still rewrites the metadata file's `last_accessed_at` value through [`pkg/downloadcache/metadata.go`](../../pkg/downloadcache/metadata.go).

Both `GET` and `HEAD` are registered to the same generation handlers. The frontend's download flow performs `HEAD` before `GET`, so a cold `HEAD` can do the full generation and the following `GET` can touch cache metadata again.

Opening an EPUB in the web reader also uses this generated endpoint rather than `/download/original`.

The plugin output-generator and `GetOrGeneratePlugin` infrastructure exists, but no production request or worker call site reaches it at the inventoried commit. It is latent mutation authority rather than a current Public Demo request path.

### Original-file fallbacks

Blocking only explicitly named original-download routes is insufficient:

- the normal web generated-download endpoint serves supplements directly from their original paths;
- OPDS native and KePub download handlers serve the original on supported generation failures, and the KePub handler also does so for unsupported formats;
- eReader native and KePub download handlers have the same generation-error fallbacks;
- Kobo KePub download serves the original for unsupported formats and generation failures.

These branches expose source bytes through routes that otherwise appear to provide generated media. Demo Mode must either block those branches or establish that a route's successful response cannot fall back to the original.

### Page cache behavior

`GET /books/files/:id/page/:pageNum` writes on a cache miss:

- CBZ: `cache_dir/cbz/{fileID}/page_{N}.{ext}`;
- PDF: `cache_dir/pdf/{fileID}/page_{N}.jpg`.

The page reader may prefetch neighboring pages, so opening the reader can create several files without explicit navigation. Cache hits are filesystem reads and do not update access metadata.

### Browser-local state

Browser-local mutation is limited to acceptable evaluation behavior:

- the `shisho_session` cookie;
- `ui-theme` in local storage after a theme choice;
- `shisho-sidebar-collapsed` in local storage, including an automatic rewrite on layout mount;
- router scroll restoration in session storage;
- search, sorting, pagination, reader page, and tab state in URL/history;
- browser HTTP/image/media caches;
- TanStack Query, Foliate, audio, reader, and search state in memory.

No application service worker, Cache Storage persistence, IndexedDB reader-state persistence, or server-side reading-progress record was found.

## User-scoped writes available without global write permission

The predefined Viewer role prevents ordinary catalog and administration writes, but authenticated user-scoped routes remain mutable by design.

| Surface | Mutations | References |
|---|---|---|
| User settings | Upsert reader and gallery preferences | [`pkg/settings/routes.go`](../../pkg/settings/routes.go) |
| Per-library settings | Upsert or clear saved sort | [`pkg/settings/routes.go`](../../pkg/settings/routes.go) |
| Lists | Create, edit, delete, add/remove/reorder Books, and create templates; users with `users:read` can also create, update, or delete shares | [`pkg/lists/routes.go`](../../pkg/lists/routes.go) |
| Book/list membership | Replace the shared user's editable list memberships for a Book | `POST /books/:id/lists` in [`pkg/books/routes.go`](../../pkg/books/routes.go) |
| API keys | Create, rename, delete, grant permissions, create short URLs, clear Kobo state | [`pkg/apikeys/routes.go`](../../pkg/apikeys/routes.go) |
| Password | Reset the caller's own password | [`pkg/users/routes.go`](../../pkg/users/routes.go) |

A shared Viewer account could therefore change state for all subsequent visitors. Demo Mode needs explicit treatment of these routes. A frontend-only read-only presentation is insufficient.

## State-changing route families

The route files are the authoritative registration source. This section groups mutation families by effect rather than duplicating every method signature.

### Catalog and media

Routes in [`pkg/books/routes.go`](../../pkg/books/routes.go) and [`pkg/chapters/routes.go`](../../pkg/chapters/routes.go) can:

- update Book and File metadata and relations;
- create related Persons, Series, Genres, Tags, and Publishers;
- update or replace chapters and review overrides;
- upload or derive covers;
- resync a File or Book synchronously;
- move or merge Files and Books;
- delete Books, Files, source media, covers, and sidecars;
- reorganize library paths;
- rewrite sidecars;
- update FTS and review state.

Manual resync is important because it calls scanner logic directly from the request. It is not made safe merely by disabling scheduled workers.

### Metadata resources

Write routes under the following packages mutate SQLite and FTS, and some also reorganize source files:

- [`pkg/people`](../../pkg/people): update, merge, delete; Person name changes can reorganize authored Books and narrated Files.
- [`pkg/series`](../../pkg/series): update, merge, delete.
- [`pkg/genres`](../../pkg/genres): update, implicit merge on collision, explicit merge, delete.
- [`pkg/tags`](../../pkg/tags): update, implicit merge on collision, explicit merge, delete.
- [`pkg/publishers`](../../pkg/publishers): update hierarchy, merge, set child, delete.

### Libraries, jobs, configuration, users, and roles

| Package | Mutation surface |
|---|---|
| [`pkg/libraries`](../../pkg/libraries) | Create/update/delete libraries, replace paths, refresh monitor watches, and potentially enqueue scans. |
| [`pkg/jobs`](../../pkg/jobs) | Insert scan, bulk-download, and other jobs; publish events; later execute job-specific effects. |
| [`pkg/settings`](../../pkg/settings) | User settings, per-library settings, global review criteria, and recompute-review job creation. |
| [`pkg/users`](../../pkg/users) | Create/update/deactivate users, library access, roles, and password resets. |
| [`pkg/roles`](../../pkg/roles) | Create/update/delete roles and permissions. |
| [`pkg/cache`](../../pkg/cache) | Recursively clear download, CBZ-page, or PDF-page caches. |

### Plugin administration and execution

Plugin mutation authority spans [`pkg/plugins`](../../pkg/plugins), the plugin data/install roots, OS temporary storage, and capability-approved external resources.

Administration routes can:

- fetch repository indexes and plugin archives;
- install, update, reload, enable, disable, configure, and uninstall plugins;
- write plugin files and persistent plugin data;
- update repository, hook-order, field-setting, identifier-type, status, and version rows;
- execute plugin lifecycle code and external commands.

Request-driven execution can occur outside background jobs:

- `POST /plugins/search` executes metadata-enricher hooks and may perform plugin-approved outbound HTTP or filesystem/process work;
- `POST /plugins/apply` writes metadata, relations, FTS, covers, pages, sidecars, and organized paths;
- plugin availability `GET` requests fetch repository indexes;
- Book/File resync can invoke parser or enrichment hooks synchronously.

Plugin filesystem capabilities in [`pkg/plugins/hostapi_fs.go`](../../pkg/plugins/hostapi_fs.go) include the install directory, persistent data directory, hook temporary directory, hook-provided paths, and broader paths when declared. Shell, FFmpeg, archive, and HTTP host APIs expand the external side-effect surface.

## Read methods with side effects

HTTP method alone cannot define the safe surface.

| Read-looking route | Side effect |
|---|---|
| Generated download `GET` or `HEAD` in web, OPDS, eReader, and Kobo surfaces | Generates/touches runtime cache and may start cleanup. |
| `GET /books/files/:id/page/:pageNum` | Extracts or renders a runtime page cache entry. |
| `GET /plugins/available...` | Fetches enabled plugin repositories. |
| `GET /audnexus/books/:asin/chapters` | Performs outbound HTTP on in-memory cache miss. |
| Kobo `GET .../v1/library/sync` | Creates sync-point snapshots, marks completion, and deletes old snapshots asynchronously. |
| Any eReader or Kobo API-key-authenticated request | Asynchronously updates `api_keys.last_accessed_at`. |
| Kobo catch-all `/v1/*` | Proxies arbitrary methods to `storeapi.kobo.com`; unknown IDs in some handlers also proxy. |

By contrast, ordinary Book/Library/metadata listing and retrieval, search, cover serving, chapter listing, original download, and M4B streaming are operational reads apart from logs, buffers, and browser caches.

## External-client surfaces

### OPDS

OPDS browsing, search, catalog, and cover routes are read-only at the domain level. OPDS native and KePub download routes use the same writable download cache as the web application and can serve the original source file when generation fails. Basic Auth also maintains an in-memory credential cache.

References: [`pkg/opds/routes.go`](../../pkg/opds/routes.go), [`pkg/opds/handlers.go`](../../pkg/opds/handlers.go), and [`pkg/auth/middleware.go`](../../pkg/auth/middleware.go).

### eReader browser

Every API-key-authenticated eReader request asynchronously updates the key's `last_accessed_at`. File and KePub downloads generate or touch the download cache and can serve the original source file when generation fails. Catalog, search, cover, and short-code resolution are otherwise reads.

References: [`pkg/ereader/routes.go`](../../pkg/ereader/routes.go), [`pkg/ereader/middleware.go`](../../pkg/ereader/middleware.go), and [`pkg/ereader/handlers.go`](../../pkg/ereader/handlers.go).

### Kobo

Every API-key-authenticated Kobo request asynchronously updates the key's `last_accessed_at`. Kobo library sync persists snapshots. KePub downloads use the writable cache and can serve the original source file when conversion is unsupported or fails. Unhandled Kobo paths and unknown Kobo IDs may proxy to Kobo's store API.

References: [`pkg/kobo/routes.go`](../../pkg/kobo/routes.go), [`pkg/kobo/middleware.go`](../../pkg/kobo/middleware.go), [`pkg/kobo/handlers.go`](../../pkg/kobo/handlers.go), [`pkg/kobo/service.go`](../../pkg/kobo/service.go), and [`pkg/kobo/proxy.go`](../../pkg/kobo/proxy.go).

These surfaces are not required by the Public Demo destination and should not be assumed safe merely because the SPA does not link to them.

## Filesystem write targets

| Target | Writers |
|---|---|
| Runtime SQLite, WAL, SHM | Startup, migrations, handlers, workers, Kobo/eReader key touches |
| `cache_dir/downloads` | Native and KePub downloads; generator temporary files are generally siblings here |
| `cache_dir/downloads/bulk` | Bulk-download jobs |
| `cache_dir/cbz` | CBZ reader page extraction |
| `cache_dir/pdf` | PDF reader page rendering |
| Library media paths | Scanner, monitor, organization, delete/move/merge/resync, plugin converters |
| Covers beside media | Scanner, cover upload/page selection, metadata apply |
| Book/File sidecars | Scanner and metadata mutation paths |
| Plugin install/data roots | Plugin administration and plugin host APIs; startup loading reads installed files and may write related SQLite rows |
| OS temporary directory | Plugin hooks/install/update and scan input conversion |
| `tmp/api.port` relative to working directory | Development startup when applicable |

The Public Demo can keep the prepared corpus mount read-only only if every path capable of writing beside media is blocked or redirected. Runtime caches and SQLite need separately writable, disposable locations under current behavior.

## Outbound network surface

| Trigger | Destination or authority | References |
|---|---|---|
| Immediate and daily plugin update check | Enabled plugin repository URLs | [`pkg/worker/worker.go`](../../pkg/worker/worker.go), [`pkg/plugins/manager.go`](../../pkg/plugins/manager.go) |
| Plugin availability, repository sync, install, update | Repository indexes, archives, icons | [`pkg/plugins/handler_list.go`](../../pkg/plugins/handler_list.go), [`pkg/plugins/installer.go`](../../pkg/plugins/installer.go) |
| Plugin hooks | Manifest-allowed domains, redirects, shell commands, and FFmpeg | Plugin host APIs under [`pkg/plugins`](../../pkg/plugins) |
| Audnexus chapter lookup | `api.audnex.us` | [`pkg/audnexus/service.go`](../../pkg/audnexus/service.go) |
| Kobo proxy | `storeapi.kobo.com` | [`pkg/kobo/proxy.go`](../../pkg/kobo/proxy.go) |

Ordinary login, browsing, search, detail retrieval, cover serving, EPUB/CBZ/PDF reading, M4B streaming, and native generation make no backend outbound HTTP calls when plugins and the listed integrations are not involved.

## Logging and process-memory mutation

Ordinary requests still create acceptable operational state:

- Echo/application logs are written to the configured logger output;
- the most recent application logs are stored in a fixed-size process-memory ring;
- log and job events are published through the in-memory SSE broker;
- authenticated SPA sessions maintain SSE subscribers and heartbeat timers;
- PDF rendering lazily initializes process-global PDFium/WASM pool state;
- Basic Auth may populate an in-memory credential cache;
- plugin runtimes, monitor maps/timers, and cache coordination locks are mutable runtime state.

Job logs differ from ordinary application logs: they are persisted in SQLite.

## Test-mode risk

When test mode is enabled, unauthenticated `/test/*` routes can create or delete users, libraries, Books, Persons, Series, API keys, plugins, plugin files, and broad related database state. Registration is conditional in [`pkg/server/server.go`](../../pkg/server/server.go), with handlers under [`pkg/testutils`](../../pkg/testutils).

The Public Demo deployment must never run with test mode enabled.

## Constraints this inventory hands to later decisions

This ticket does not choose the enforcement architecture. It establishes that any viable design must account for all of the following:

1. **Prepared versus runtime state:** keep the deployed database and media corpus immutable while providing writable disposable SQLite/WAL/cache/temp locations required by startup and reading.
2. **Authoritative request enforcement:** block disallowed mutations in the backend, not only through hidden UI controls or the Viewer role.
3. **User-scoped exceptions:** explicitly prevent settings, lists, API keys, short URLs, and self-password-reset writes by the shared account unless a later decision intentionally permits and resets them.
4. **Reader cache allowance:** preserve EPUB/CBZ/PDF consumption by allowing bounded disposable cache generation, prebuilding equivalent artifacts, or changing those paths.
5. **Original and bulk downloads:** disable every web and external-client route that explicitly or implicitly exposes original files, including generation fallbacks and supplements, or produces bulk downloads, while preserving reader-required byte delivery.
6. **Background isolation:** stop scan scheduling, job fetching and processing, retention, monitoring, plugin update checks, and other unnecessary asynchronous behavior through a complete worker control rather than relying on one scan setting. Otherwise, pending or stale `in_progress` jobs in the prepared database can execute after startup.
7. **Synchronous mutation paths:** separately block resync, plugin search/apply/admin, metadata/media edits, cache administration, setup, and other request-driven writes.
8. **External clients and outbound traffic:** disable OPDS/eReader/Kobo mutation/download surfaces, plugin repositories and hooks, Audnexus, and Kobo proxying unless explicitly required.
9. **Read-only media mount:** ensure no reachable scanner, organizer, sidecar, cover, converter, delete, move, merge, or plugin path can write to the prepared corpus.
10. **Cold-start safety:** migrations and FTS probing must run only against the disposable runtime database, and plugin loading must not mutate or contact external systems.
11. **Operational state:** ordinary platform logs, in-memory subscriptions, browser-local state, and bounded temporary runtime artifacts remain acceptable under the map's standing notes.
12. **Verification:** acceptance tests need direct HTTP coverage for blocked routes and side-effect checks, not only browser checks for hidden controls.

## High-value code references

- Startup: [`cmd/api/main.go`](../../cmd/api/main.go)
- Database setup: [`pkg/database/database.go`](../../pkg/database/database.go)
- Server route assembly: [`pkg/server/server.go`](../../pkg/server/server.go)
- Worker lifecycle: [`pkg/worker/worker.go`](../../pkg/worker/worker.go)
- Scanner and monitor: [`pkg/worker`](../../pkg/worker)
- Download cache: [`pkg/downloadcache`](../../pkg/downloadcache)
- CBZ/PDF page caches: [`pkg/cbzpages`](../../pkg/cbzpages), [`pkg/pdfpages`](../../pkg/pdfpages)
- Book/file routes and handlers: [`pkg/books/routes.go`](../../pkg/books/routes.go), [`pkg/books/handlers.go`](../../pkg/books/handlers.go)
- User-scoped settings/lists/API keys: [`pkg/settings`](../../pkg/settings), [`pkg/lists`](../../pkg/lists), [`pkg/apikeys`](../../pkg/apikeys)
- Plugin runtime and management: [`pkg/plugins`](../../pkg/plugins)
- External clients: [`pkg/opds`](../../pkg/opds), [`pkg/ereader`](../../pkg/ereader), [`pkg/kobo`](../../pkg/kobo)
- Frontend reader paths: [`app/components/pages/EPUBReader.tsx`](../../app/components/pages/EPUBReader.tsx), [`app/components/pages/PageReader.tsx`](../../app/components/pages/PageReader.tsx), [`app/components/pages/M4BReader.tsx`](../../app/components/pages/M4BReader.tsx)
