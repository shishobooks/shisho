# Shisho Backend Development

Go backend: Echo, Bun ORM on SQLite, Air for hot reload. Each domain package has `handlers.go`, `routes.go`, `service.go`, and `types.go`.

## Topic docs

- Read `docs/agents/standards/backend.md` before mapping an error to a status, shaping a response, or adding an authorization check. Reviewers apply it to every backend diff.
- Read `docs/agents/backend/demo-mode.md` before adding a route family, a download path, or a GET handler with side effects.
- Read `docs/agents/backend/auth-and-permissions.md` before adding a route, a permission resource, a handler permission check, a user-loading path, or a device (Kobo, eReader, OPDS) route.
- Read `docs/agents/backend/covers-and-file-serving.md` before writing covers, adding a route that serves file bytes, or a device download.
- Read `docs/agents/backend/data-sources-and-merges.md` before writing metadata from a new source, editing identifiers or series numbers, or adding a resource delete, merge, or rename.
- Read `docs/agents/backend/search-fts.md` before adding a write that changes indexed names, titles, or links, an entity delete, or an FTS column.

golangci `forbidigo` and `pkg/migrations/schema_invariants_test.go` enforce several backend rules; their messages name the fix.

## HTTP routing

- `pkg/server` registers API routes under `e.Group("/api")`. API helpers take `*echo.Group`; device helpers take `*echo.Echo` because device families mount at the root, outside `/api`.
- Every route package exports `RegisterRoutes(router, deps...)` with no return value, router first. A package mounted in more than one place adds `Register<Scope>Routes`. Take only the parameters the package uses.
- Dev and production are one origin (Vite proxies `/api` unchanged), so the server adds no CORS.

## Shared services

**A route package never builds a service or cache that holds state or settings.** `New` in `pkg/server/server.go` builds those once in `sharedServices` and passes them in; `cmd/api/main.go` builds the ones the worker also uses. Services that hold only `*bun.DB` may be built locally.

## Data sources

Lower priority number wins (`pkg/models/data-source.go`).

**Series memberships have their own source.** `books.series_source` is the provenance of a Book's membership collection and number groups; `series.name_source` describes only the Series' name. Gate on and stamp `books.series_source`; never read `Series.NameSource` as a proxy (ADR 0006).

## Core gotchas

- **Loaders that select books with `Relation("Files")` and reach a JSON response call `models.ResolveBookFileDisplayNames`** after scanning. Bun runs no row hooks for has-many relations, so `display_name` is otherwise empty. Code that changes a file's `Name` or `Filepath` in memory before responding re-resolves.
- **Sidecar writes and fingerprints need full relations.** `RetrieveFile` loads too few, and the write silently drops data. Use `RetrieveFileWithRelations`, or a file from `RetrieveBook`'s `book.Files`; the writers' doc comments name what they read.
- **Code that replaces a file's bytes on disk invalidates its CBZ and PDF page caches**, as the scan does in `invalidatePageCaches`. The caches key on file id and never notice a swap, so the reader keeps serving the old pages.
- **Find-or-create finishes with `database.RetrieveOnUniqueViolation`**, which returns the winner's row when a concurrent insert lost on the unique index. Detect UNIQUE violations elsewhere with `database.IsUniqueViolation`.
- **Bun aliases are not always the table's first letter.** Read the `alias:` in the model's `bun:"table:..."` tag and use it in `Where`, `Order`, and other clauses; the table name gives "no such column".
- **A file's type is not always its extension**: some extensions map to another type. Type a path with `models.FileTypeForPath`, and test types with the `models` helpers (`IsBuiltInFileType`, `IsEbookFileType`) instead of listing them, so a new format reaches every site.
- **`models.LogLevel` is the one severity enum** for job logs and app logs. Reuse it (`tstype:"LogLevel"`) rather than adding level strings.

### File-level vs book-level fields

When both levels exist (`file.Name` vs `book.Title`), download filenames and file organization prefer the file-level value when non-empty; display uses file-level in file contexts and book-level in book contexts.

### Organization

- **Disk and database agree after every move.** Organizing moves the file, then records the path; if the write fails, the next scan deletes the Book and reimports the file without its metadata. Every rename or move site sets `OrganizedNameOptions.Claimed` (`books.Service.FilepathClaimedByOtherFile`) and calls `books.Service.RecordOrganizedFilepath` right after the move, which undoes the move on failure. `reorganizeFileAfterMetadataChange` in `pkg/books/handlers.go` is the pattern.
- **An API edit that changes a field in the organized path reorganizes** when the library has `OrganizeFileStructure` on. Removals count: an empty collection that clears memberships must stay distinct from an absent field.
- **Directory cleanup deletes only Shisho's patterns.** Pass `fileutils.JunkFilePatterns()` or `fileutils.DirectoryCleanupPatterns()`, never `config.SupplementExcludePatterns`: those only hide files from discovery, and a user's `*.txt` would delete their files.

### Sidecars

Every parsed file has a sidecar. Sidecars hold editable metadata only: no intrinsic properties (bitrate, duration) and no `*_source` fields.

### File Modes Are Set at Creation

- **Set the mode at creation, never chmod afterwards.** Some NAS ACLs refuse chmod, and a chmod overrides an operator's stricter umask. Use `fileutils.CreateTemp`/`MkdirTemp`, pass the mode to `os.OpenFile`/`os.WriteFile`/`fileutils.WriteFileAtomic`, and move with `fileutils.MoveFile`. Output other host accounts may read (downloads, covers, page caches) is 0644; private state is 0600.
- Mode-asserting tests set the umask with `testumask.Set`, live in a `//go:build unix` file, and skip `t.Parallel()` because the umask is process-wide.

### Publishing cache files

Readers hit caches concurrently, so a file appears at its final path only when complete: `fileutils.WriteFileAtomic` for in-memory bytes, or a unique temp name in the same directory (`fileutils.CreateTemp`) that is closed and then renamed. The temp name must not match the cache-hit check, and a fixed `dest + ".tmp"` collides between requests. Generated downloads go through the shared `downloadcache.Cache`.

### Hand-built HTML

Pages built by string concatenation (`pkg/ereader`, the KePub XHTML) escape every interpolated value, URLs and safe-looking values included. Stored plain text is not tag-free: titles and names come straight from file metadata, and `htmlutil.StripTags` output decodes only one entity level. Multi-line text escapes first and adds markup after.

## API Conventions

Error statuses, response shapes, and naming are in `docs/agents/standards/backend.md`. Go is the source of truth for every request and response shape (ADR 0004); types generate into `app/types/generated/` via `tygo.yaml`.

- **Slice fields need `mod:"dive"` for inner `mod` tags to run.** mold treats slices, maps, and pointers to slices as opaque, so without `dive` an inner `mod:"trim"` is a silent no-op. It is independent of validator's `dive`; a field needing both carries both.

tygo mechanics:

- **Embed the model by value with `tstype:",extends"`.** A pointer embed generates `extends Partial<...>`. A cross-package value embed needs a `frontmatter` import and a `type_mappings` entry on the package's `tygo.yaml` entry, or tygo emits an unresolved name. Same-package embeds need neither.
- **A relation the response reshapes gets `tstype:"-"` on the model field** (keep its `json` tag), so the generated interface drops it and the `extends` does not collide. Only when no consumer reads the relation as objects.
- **A slice of a foreign model** gets a field-level `tstype:"File[]"`-style override plus a frontmatter import, not a global `type_mappings` entry, which yields `(File | undefined)[]`.
- **Optional non-pointer request fields** need `,omitempty` to generate as optional; on requests it has no wire effect.
- **Tri-state `*string` fields** (omit leaves untouched, null clears) get `tstype:"string | null"` on both payload and response.
- **`//tygo:emit` only fires on `const` and `type`**; on a `var` it is silently dropped. Attach the emit to an adjacent const and keep the two in sync.

## Config

- Validate ranges with `validate` tags; `validationMessage` turns failures into messages naming the key and env variable.
- Problems that should not stop an existing install go in `Config.StartupWarnings()`, not validation.
- Derived values go in `ConfigResponse` (`pkg/config/types.go`), not recomputed in the frontend.

## Runtime settings

`app_settings` (`pkg/appsettings`) holds admin-editable policy as one JSON document per key; deployment facts go in `config.Config`. A domain package owns its key, its struct, and load and save helpers that fall back to defaults when no row exists (`pkg/books/review` is the pattern). A feature switch that carries companion policy and a warning the admin must read belongs in the admin UI with no config field or env var (ADR 0008). Writes require `config:write`.

## Adding things

- **Metadata fields:** invoke the `metadata-field` skill. The scanner, plugin bridge, generators, OPDS, Kobo, and frontend have parallel paths that a static list misses; the skill greps an existing similar field instead.
- **Entity types** that books or files reference: model in `pkg/models/`; service patterned on `pkg/genres/service.go`; wire it into `worker.New`; use it in the scanner; add its orphan cleanup and FTS removal to `books.CleanupOrphanedEntities`.
