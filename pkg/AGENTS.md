# Shisho Backend Development

Go backend: Echo, Bun ORM on SQLite, Air for hot reload. `cmd/api/main.go` starts the HTTP server and the background worker; each domain package has `handlers.go`, `routes.go`, `service.go`, and `types.go`.

**Bar for additions.** Add a rule here only when it applies beyond a single fix and neither the code nor a check can convey it; prefer adding a check. Topic detail goes in the topic doc, reviewer judgement in `docs/agents/standards/backend.md`, and bug history in the commit message.

## Topic docs

- Read `docs/agents/backend/demo-mode.md` before adding a route family, a download path, or a GET handler with side effects.
- Read `docs/agents/backend/share-links.md` before touching `pkg/sharelinks`, the public `/api/share` routes, or the recipient payload.
- Read `docs/agents/backend/admin-settings.md` before adding a runtime setting an admin edits, or deciding between it and `config.Config`.
- Read `docs/agents/backend/scanner.md` before touching the scan job, the library monitor, unreadable-file handling, supplement classification, chapter sync, or file reorganization after an edit.
- Read `docs/agents/backend/covers-and-file-serving.md` before touching covers, any route that serves file bytes, `Cache-Control`, or download fallback.
- Read `docs/agents/backend/data-sources-and-merges.md` before touching scan metadata merging, identifier provenance, series number groups, resource deletes, or resource merges.
- Read `docs/agents/backend/auth-and-permissions.md` before adding a route, a permission resource, a handler permission check, or a device (Kobo, eReader, OPDS) route.
- Read `docs/agents/backend/search-fts.md` before touching FTS, entity deletes, or any write that changes indexed names, titles, or links.
- Read `docs/agents/backend/ereader-opds.md` before touching `pkg/opds`, `pkg/ereader`, or hand-built HTML.
- Reviewers: apply `docs/agents/standards/backend.md` to backend diffs.

## Checks that already enforce rules

Do not restate these in code review or new docs; the check names the fix.

- golangci `forbidigo`: `echo.Context` `Get`/`Set` outside `pkg/auth`, `pkg/apikeys`, `pkg/binder`, and the request ID middleware (use `auth.RequireUser`/`auth.SetUser`, which fail closed); `c.File`, `c.Attachment`, `c.Inline`, `http.ServeFile` (use `httputil.ServeFile`); `os.Chmod` and `(*os.File).Chmod` outside tests (some NAS ACLs refuse chmod with `EPERM`); `echo.NewHTTPError` (use `pkg/errcodes`, since the handler would invent a code from the message); `os.CreateTemp` (use `fileutils.CreateTemp`, since `os.CreateTemp` is always 0600); `migrate.NewMigrator` outside `pkg/migrations` (use `migrations.NewMigrator`).
- Schema tests in `pkg/migrations/schema_invariants_test.go`: `TestSchema_ForeignKeysHaveOnDelete`, `TestSchema_ForeignKeysAreIndexed` (leading column of a non-partial index), `TestSchema_TableNamesArePlural`, `TestSource_NoLowerNameComparisons`.

## HTTP routing

- `pkg/server` registers API routes under `e.Group("/api")`. API helpers take `*echo.Group`; device helpers take `*echo.Echo` because `/opds`, `/kobo`, `/ereader`, `/e`, and `/health` stay at the root.
- Every route package exports `RegisterRoutes(router, deps...)` with no return value, router first. A package mounted in more than one place adds `Register<Scope>Routes`. A registration function never builds a service another package needs and never returns one; `pkg/server` builds it and passes it in. Take only the parameters the package uses.
- Unknown GET/HEAD paths outside the API and device prefixes go to `pkg/frontend.Handler()`. Prefixes match whole segments (`/apiary` is a frontend route). Missing API and device routes, including routes omitted in Demo Mode, return JSON 404.
- Use per-router `RouteNotFound` handlers, never mutate `echo.NotFoundHandler`. `Group.Use` adds authenticated fallbacks; server construction replaces them after registration so missing paths are 404, not 401.
- Vite proxies `/api` unchanged (no path rewrite, no `X-Forwarded-Prefix`). One origin, so no CORS.
- Forwarded-header sanitization runs in `e.Pre`. Logging, recovery, security headers, compression, and demo enforcement run in `e.Use`. Keep the frontend fallback pattern `/*` so request logging recognizes it.

## Shared services

`New` in `pkg/server/server.go` collects shared services in `sharedServices` and injects them. **A route package never builds its own `books.Service`, `appsettings.Service`, `plugins.Service`, `sharelinks.Service`, or page cache.**

- The books service must be built `WithAppSettings`; one without it silently skips the Reviewed recompute on every mutation. Books routes read settings through `AppSettings()` rather than a second argument that could disagree.
- `cmd/api/main.go` builds the `plugins.Service`, the download cache, and the CBZ and PDF page caches, and hands the same instances to `server.New`, the plugin `Manager`, and `worker.New`. A second page cache drifts from the one the cache admin routes size and clear, and the scan's `invalidatePageCaches` would clear the wrong one.
- The worker builds its own `books.Service` (with app settings) and `appsettings.Service`.
- `server.New` and `worker.New` build missing caches and plugin services when handed nil (tests).
- Stateless services that hold only `*bun.DB` (search, aliases, libraries, jobs, settings, API keys, people, genres, tags, publishers, series) may be built locally.
- `pkg/server/shared_services_test.go` covers the wiring through real routes.

## Data Source Priority System

Lower number wins (`pkg/models/data-source.go`):

```
0: Manual
1: Sidecar
2: Plugin (enrichers and file parsers; source string plugin:scope/id)
3: File Metadata (epub_metadata, cbz_metadata, m4b_metadata, pdf_metadata)
4: Filepath
```

**Series memberships have their own source.** `books.series_source` is the provenance of a Book's ordered membership collection and number groups; `series.name_source` describes only the Series' name. The scanner, Identify, and the Edit form gate on and stamp `books.series_source`; never read `Series.NameSource` as a proxy (ADR 0006). `FindOrCreateSeries` still carries a name source that can lower an existing Series' `name_source`, independently.

Merge mechanics, identifier provenance, and delete stamping: `docs/agents/backend/data-sources-and-merges.md`.

## Core gotchas

### Loading files

- **`File.display_name` is resolved on load.** `models.File.DisplayName` (not persisted) comes from `ResolveDisplayName()`: a main file's `name`, else its filename; a supplement's filename unless `name_source` is `manual` (a `sidecar` name goes stale after a book rename, because book edits write every file's sidecar). An `AfterScanRow` hook fills it for direct file queries, but Bun runs no row hooks for has-many relations, so **every loader that selects books with `Relation("Files")` and reaches a JSON response calls `models.ResolveBookFileDisplayNames`** after scanning, and `pkg/server/file_display_name_test.go` covers its endpoint. Code that changes `Name` or `Filepath` in memory before responding re-resolves.
- **Sidecar writes and fingerprints need full relations.** `WriteFileSidecarFromModel` needs Narrators, Identifiers, Publisher, Chapters; `WriteBookSidecarFromModel` needs Authors.Person, BookSeries.Series, BookGenres.Genre, BookTags.Tag; `downloadcache.ComputeFingerprint` needs the file's Narrators and Identifiers. `RetrieveFile()` loads only Book, Identifiers, and Narrators, so use `RetrieveFileWithRelations()` before a sidecar write, or a file from `RetrieveBook`'s `book.Files`, which carry their relations. Otherwise the sidecar or fingerprint silently drops data.
- **Find-or-create finishes with `database.RetrieveOnUniqueViolation`**, which returns the winner's row when a concurrent insert lost on the unique index. Detect UNIQUE violations elsewhere with `database.IsUniqueViolation`.

### File-level vs book-level fields

When both levels exist (`file.Name` vs `book.Title`), download filenames and file organization prefer the file-level value when non-empty; display uses file-level in file contexts and book-level in book contexts.

### Organization

- **`scanInternal(FilePath)` defers organization.** It calls `scanFileCore` with `isResync=false`, which skips organizing. Any new caller must organize the returned books itself when the library has `OrganizeFileStructure`, or files stay in the library root. Existing callers are in `docs/agents/backend/scanner.md`.
- **Disk and database agree after every move.** Organizing moves the file, then records the path; if the write fails, the next scan deletes the Book and reimports the file without its metadata. Every rename or move site sets `OrganizedNameOptions.Claimed` (`books.Service.FilepathClaimedByOtherFile`) so a path another `files` row holds counts as taken, and calls `books.Service.RecordOrganizedFilepath` right after the move. On failure that moves the file, covers, and sidecars back (`fileutils.UndoOrganizedMove`). Pass `includeBookSidecar` false for `RenameOrganizedFileOnly`, true otherwise.
- **Directory cleanup deletes only Shisho's patterns.** `CleanupEmptyDirectory` and `CleanupEmptyParentDirectories` take `fileutils.JunkFilePatterns()` or `fileutils.DirectoryCleanupPatterns()`, never `config.SupplementExcludePatterns`: those only hide files from discovery, and a user's `*.txt` would delete their files.

### Sidecars

Every parsed file has a sidecar. Sidecars hold editable metadata only: no intrinsic properties (bitrate, duration) and no `*_source` fields.

### File Modes Are Set at Creation

- **Set the mode at creation, never chmod afterwards.** Use `fileutils.CreateTemp`/`MkdirTemp`, or pass the mode to `os.OpenFile`/`os.WriteFile`; `fileutils.WriteFileAtomic` takes a mode. Move with `fileutils.MoveFile`, whose cross-device copy keeps the source's bits. Generated output other host accounts may read (downloads, covers, page caches, bulk zips) is 0644; private state (the download cache's `*.meta.json`) is 0600. Plugin writes follow "Host APIs" in `pkg/plugins/AGENTS.md`. The umask and inherited ACLs decide the final mode; a chmod would also override an operator's stricter umask. Mode-asserting tests set the umask with `testumask.Set`, live in a `//go:build unix` file, and skip `t.Parallel()` because the umask is process-wide.

### Publishing Cache Files

- **Publish cache files complete.** Readers hit the page and download caches concurrently, so a file appears at its final path only when complete: `fileutils.WriteFileAtomic` for in-memory bytes, or a unique temp name in the same directory (`fileutils.CreateTemp`) that is closed and then renamed. The temp name must not match the cache-hit check (`cbzpages` globs `page_<n>.*`, so temps start `.extracting-`). A fixed `dest + ".tmp"` collides between requests.
- **Generated downloads go through `downloadcache.Cache.getOrGenerate`**, which locks per destination, rechecks, stages in a private `.staging-*` directory, renames, then writes metadata. The lock lives on the instance, so every consumer (books, OPDS, eReader, Kobo, bulk download) shares the one `dlCache` from `cmd/api/main.go`. Temp files from a killed process stay until the admin cache clear.

## API Conventions

- **Error statuses**, in short (full rules for reviewers in `docs/agents/standards/backend.md`):
  - Build errors with `pkg/errcodes` constructors; a server fault is a plain wrapped error (500).
  - 422 `validation_error` for a rejected request value; 422 `invalid_state` for a valid request the target's state cannot honor.
  - 400 comes only from the binder. Return its error unchanged: `return errors.WithStack(err)`.
  - 404 only when the row is missing (`sql.ErrNoRows`) or the file does not exist (`os.IsNotExist`); other DB and filesystem errors are 500.
  - An unparseable numeric path ID is `NotFound(resource)` via `httputil.ParamID`.
  - 502 `upstream_error` when an upstream server fails.
  - Distinguish service errors with exported sentinels and `errors.Is`; only those may put their text in a 4xx.
  - `errcodes.NotFound` takes a noun ("Cover") and appends " not found."; other constructors use the message verbatim, so pass a sentence.
  - Every 4xx mapping gets a test that injects a fault and asserts 500.
- **Handlers return named structs**, never `echo.Map` or a map passed to `c.JSON`, so tygo can generate the type.
- **JSON is `snake_case`**, except plugin manifest and repository-index passthrough fields (ADR 0004).
- **Request binding uses structs.** The binder (`pkg/binder`) runs mold and validator, which only process structs, so wrap a slice in a struct (`Order []orderEntry` with `validate:"required"`). Binding to a slice panics.
- **Slice fields need `mod:"dive"` for inner `mod` tags to run.** mold treats slices, maps, and pointers to slices as opaque; without `dive`, inner `mod:"trim"` is a silent no-op. It is independent of validator's `dive`, so a field needing both carries both (`UpdateFilePayload.Identifiers` in `pkg/books/types.go`).
- **Bun aliases:** each model sets `alias:` in its `bun:"table:..."` tag in `pkg/models/`. Most are the first letter (`books` `b`, `files` `f`, `persons` `p`) but not all (`publishers` `pub`, `file_identifiers` `fi`, `file_fingerprints` `ffp`), so read the tag. Use the alias in `Where`, `Order`, and other clauses; the table name gives "no such column".
- **`LogLevel` (`pkg/models/log_level.go`) is the one severity enum** for job logs and app logs. Use `models.LogLevel*`, `tstype:"LogLevel"` on fields, and `oneof=debug info warn error fatal` on level filters.

### tygo

Go is the source of truth for every request and response shape (ADR 0004); types generate into `app/types/generated/` via `tygo.yaml`. Shape and naming rules are in `docs/agents/standards/backend.md`. Mechanics:

- **Embed the model by value with `tstype:",extends"`.** A pointer embed generates `extends Partial<Genre>`. A cross-package value embed needs two `tygo.yaml` additions on the package entry: a `frontmatter` import (`import { Genre } from "@/types";`) and a `type_mappings` entry (`models.Genre: "Genre"`); without them tygo emits an unresolved `extends models.Genre` or an undefined name. Same-package embeds need neither.
- **A relation the response reshapes gets `tstype:"-"` on the model field** (keep its `json` tag), so the generated interface drops it and the `extends` does not collide. Only when no consumer reads the relation as objects. Reference: `pkg/genres/types.go` and `Aliases` in `pkg/models/genre.go`.
- **A slice of a foreign model** gets a field-level override (`Items []*models.File` tagged `tstype:"File[]"`) plus a frontmatter import, not a global `type_mappings` entry, which yields `(File | undefined)[]`.
- **Optional non-pointer request fields** need `,omitempty` to generate as optional; on requests it has no wire effect.
- **Tri-state `*string` fields** (omit leaves untouched, null clears) get `tstype:"string | null"` on both payload and response (`SortSpec` in `pkg/settings/types.go`).
- **`//tygo:emit` only fires on `const` and `type`**; on a `var` it is silently dropped. Attach the emit to an adjacent const and keep the two in sync (`PlaybackSpeeds` in `pkg/models/user_settings.go`).

## Config

- Every field is settable from the config file and an env var. A new `config.Config` field also lands in `shisho.example.yaml`, `website/docs/configuration.md`, and `app/components/pages/AdminSettings.tsx` (root `AGENTS.md`).
- Validate ranges with `validate` tags (`min=1`, `min=0,max=1`, `min=1ms`). `validationMessage` names the key, env variable, and range; a secret (`json:"-"`) value is never echoed. `[]string` fields split comma-separated env values.
- Problems that should not stop an existing install go in `Config.StartupWarnings()`, which `cmd/api/main.go` logs at warn (a short `jwt_secret` warns; the public example placeholder fails validation).
- Derived values go in `ConfigResponse` (`pkg/config/types.go`), not recomputed in the frontend.

## Database query logging

`database.New` installs `queryLogHook` (`pkg/database/database.go`). Queries over 250 ms log at warn with only operation, table, and duration. **That tier never carries SQL text, arguments, or error text**: Bun inlines parameter values (password hashes, share tokens, plugin config) and Settings > Logs is readable with `config:read`. Full SQL (truncated to 2 KB) logs only at the debug tier, enabled by `database_debug`. Build the hook after `logger.SetOutput`, or its lines miss the log buffer.

## Adding things

- **Metadata fields:** invoke the `metadata-field` skill. The scanner, plugin bridge, generators, OPDS, Kobo, and frontend have parallel paths that a static list misses; the skill greps an existing similar field instead.
- **Entity types** that books or files reference: model in `pkg/models/`; service in `pkg/{entity}/service.go` patterned on `pkg/genres/service.go` with `FindOrCreate{Entity}`, `Retrieve{Entity}`, and `List{Entity}s`; wire it into `worker.New`; use it in the scanner; add its orphan cleanup and FTS removal to `books.CleanupOrphanedEntities`.
- **Format parsers:** see `pkg/epub/AGENTS.md`, `pkg/cbz/AGENTS.md`, `pkg/mp4/AGENTS.md`, `pkg/pdf/AGENTS.md`, and `pkg/kepub/AGENTS.md`.
