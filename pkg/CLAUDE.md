# Shisho Backend Development

This file documents backend patterns and conventions specific to Shisho.

## Stack

- Go with Echo web framework
- Bun ORM with SQLite database
- Air for hot reload
- mise for task running and tool management

## Architecture

### Entry Point

`cmd/api/main.go` starts both HTTP server and background worker.

### Core Services Pattern

Each domain (books, jobs, libraries, chapters) has:
- `handlers.go` - HTTP request/response logic
- `routes.go` - HTTP endpoint registration
- `service.go` - Business logic and database operations
- `validators.go` - Request/response schemas

### Database Models (`pkg/models/`)

- Use Bun ORM with struct tags for database mapping
- Models include JSON tags for API serialization
- TypeScript types auto-generated via tygo from Go structs

### Background Worker (`pkg/worker/`)

- Processes jobs from database queue
- Main job type: scan job that processes ebook/audiobook files
- Extracts metadata from EPUB (via `pkg/epub/`) and M4B files (via `pkg/mp4/`)
- Generates cover images with filename-based storage strategy
- **Library monitor** (`monitor.go`): watches library paths for filesystem changes via fsnotify, debounces events, and triggers targeted single-file rescans. Remove/Rename events landing on a directory path (which fsnotify emits for the directory itself, not the files inside) are queued as `pendingEvent{IsDirectory: true}` and fan out to per-file cleanup for every DB file whose filepath sits under that directory, so removing or renaming a book folder cleans up its book/file rows instead of leaving them orphaned. **Move detection via content hashing.** When the monitor processes a batch that contains any REMOVE events, it computes sha256 synchronously for CREATE events in the same batch and looks up matches in `file_fingerprints`. If an existing file row has a matching sha256 and its stored path is gone from disk, the monitor repurposes that row's `filepath` rather than deleting + recreating. This preserves book identity and user-edited metadata across folder renames. The scan job performs the same reconciliation as a safety net after its walk phase, handling cases where renames happened while the server was offline. Sha256 hashes are populated by a background `hash_generation` job queued at the end of every scan and every monitor batch that creates new files. Fingerprints are invalidated when a file's size/mtime changes so the next job run recomputes them against the new content.

### scanInternal and File Organization

**CRITICAL — `scanInternal(FilePath)` defers file organization.** When scanning a new file by path, `scanFileCore` is called with `isResync=false`, which skips the book organization step. The caller is responsible for running organization afterward if the library has `OrganizeFileStructure` enabled.

- **`ProcessScanJob`** handles this by collecting book IDs into `booksToOrganize` and running organization in a batch after all files are scanned.
- **`Monitor.processPendingEvents`** handles this by collecting book IDs from `FileCreated` results and calling `organizeBooks()` after processing all events.
- **Any new caller of `scanInternal(FilePath)`** must also handle organization, or files will be left unorganized in the library root.

### Scan Cache Must Include Supplements

**CRITICAL — the `ScanCache.knownFiles` map preloaded in `ProcessScanJob` must contain BOTH main and supplement files.** Supplements can share scannable extensions (`.pdf`/`.epub`/`.cbz`/`.m4b`) with main files — e.g. a user-demoted `Cribsheet.pdf` sitting next to a main `Cribsheet.epub`. During the filesystem walk, every file with a scannable extension becomes a scan target, so a supplement at a tracked path must resolve via the cache and early-return in `scanFileByPath`. If the cache is main-only (the old behavior), the supplement falls through to `scanFileCreateNew`, which tries to insert a duplicate file row and hits `UNIQUE(filepath, library_id)` — warned-and-continued on every scan.

- Preload via `ListAllFilesForLibrary`, not `ListFilesForLibrary` (which is still main-only, used for orphan cleanup).
- `scanFileByPath` early-returns with `&ScanResult{File: existing}` when the cached file has `FileRole == FileRoleSupplement` — supplements have no metadata to rescan.

### Auto-Classification of Supplement-Named PDFs

`scanFileCreateNew` (in `pkg/worker/scan_unified.go`) inspects new PDF files for the auto-supplement rule before creating the file row: when a PDF's basename matches `config.PDFSupplementFilenames` AND a sibling main file (EPUB / CBZ / M4B / plugin-registered extension) exists on disk OR a book row already exists at the same `bookPath`, it's created with `FileRole=Supplement` and **cover extraction is skipped** (matching the existing supplement-discovery path that creates supplements without covers).

When editing the cover-extraction block in `scanFileCreateNew`, preserve the `if !classifyAsSupplement { ... }` guard. Rescans don't re-run this rule — existing main-file PDFs whose names match the list keep their role.

### File Types

- To learn more about all the file types that we support, refer to:
  - EPUB: `pkg/epub/CLAUDE.md`
  - CBZ: `pkg/cbz/CLAUDE.md`
  - M4B: `pkg/mp4/CLAUDE.md`
  - PDF: `pkg/pdf/CLAUDE.md`
  - KePub: `pkg/kepub/CLAUDE.md`

### Primary File System

Each book has an optional `PrimaryFileID` (`*int`) that designates which file is the default for reading/downloading (used by Kobo sync and eReader browser).

**Auto-assignment rules:**
- When the first file is created for a book (`CreateFile`), it becomes primary automatically
- When the primary file is deleted (`DeleteFile`), another file is promoted: main files preferred over supplements, oldest first
- When files are moved between books (`MoveFilesToBook`), source books that lose their primary get a new one promoted, and target books with no primary get the first moved file

**API endpoint:** `PUT /books/:id/primary-file` with `SetPrimaryFilePayload { FileID int }`. Validates that the file belongs to the book.

**Key files:**
- `pkg/books/handlers.go` — `setPrimaryFile` handler and `SetPrimaryFilePayload`
- `pkg/books/service.go` — Auto-promotion in `CreateFile` and `DeleteFile`
- `pkg/books/merge.go` — Primary file handling during `MoveFilesToBook`
- `pkg/kobo/service.go` — Uses `primary_file_id` to determine which file to sync
- `pkg/ereader/handlers.go` — Uses `PrimaryFileID` to select the default download file

### Cover Image System

- Individual file covers: `{filename}.cover.{ext}`
- API endpoints: `/books/{id}/cover` and `/files/{id}/cover`

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

- **Read-side (serving, fingerprinting, file generation):** use `filepath.Join(filepath.Dir(file.Filepath), *file.CoverImageFilename)`. Pure-string, no stat, no synthetic-path trap. Book-cover serving across the books, OPDS, and eReader handlers shares `pkg/covers.ServeBookCover`, which encapsulates this resolution and the ETag-based conditional-GET pattern (see "Conditional-GET for cover endpoints" below). The series handler uses `pkg/covers.SelectFile` and resolves the path itself because it picks the file from the series' first book rather than from a fixed file list. Other examples: `fileCover` in `pkg/books/handlers.go`, `pkg/kobo/handlers.go`, `pkg/filegen/*`, `pkg/downloadcache/fingerprint.go`, and `deleteFileFromDisk`.
- **Write-side (scanner, pre-organize):** use `fileutils.ResolveCoverDirForWrite(bookFilepath, fileFilepath)` when `bookFilepath` may be a synthetic organized-folder path that hasn't been created on disk yet. Falls back to `filepath.Dir(fileFilepath)` when the book path doesn't resolve to a real directory.

**Book sidecars** for root-level books are similarly anchored next to the file — `sidecar.WriteBookSidecarFromModel(book)` falls back via `book.Files[0].Filepath` when `book.Filepath` doesn't resolve to an existing directory. Reads use `sidecar.ReadBookSidecarFromModel(book, fileHint)` and pass the current file as a hint so resolution works before the book's Files relation is loaded.

**Conditional-GET for cover endpoints:** `/files/:id/cover` serves via `c.File()` — `http.ServeContent` handles `Last-Modified`/`If-Modified-Since` from the cover file's on-disk mtime, which is sufficient because the served file's identity is pinned by the URL. `/books/:id/cover` (and its OPDS / eReader mirrors) and `/series/:id/cover` are different: the served file is *selected*, and that selection can change without any change to the newly-selected cover file's mtime — flipping the library's `CoverAspectRatio` setting on a hybrid book (EPUB + M4B) swaps which file's cover is served, removing a file from a hybrid book falls back to the remaining file's cover, and a series' first book can change because of book deletion / re-sorting / series-number changes. Mtime-only revalidation returns stale 304s in those cases. Both `pkg/covers.ServeBookCover` and `pkg/series/handlers.go::seriesCover` therefore issue an `ETag: "<file_id>-<mtime_unix>"` that bakes the selected file's identity into the validator, check `If-None-Match` manually, and pass `time.Time{}` to `http.ServeContent` so it omits `Last-Modified` and skips IMS handling (which would otherwise shortcut to 304 using just the new file's mtime).

### Data Source Priority System

Metadata sources ranked (lower number = higher precedence):
```
0: Manual (highest)
1: Sidecar
2: Plugin (enrichers and file parsers)
3: File Metadata (epub_metadata, cbz_metadata, m4b_metadata)
4: Filepath (lowest)
```

Used to determine which metadata to keep when conflicts occur. During scans, enricher plugins override file-embedded metadata per-field (enricher-first merge in `runMetadataEnrichers`).

### OPDS

- OPDS v1.2 server hosted in the application
- As new functionality is added, keep the OPDS server up-to-date with the new features
- **Cover URLs in feeds must point at `/opds/v1/books/:id/cover`**, not the books API. Reasons mirror eReader: OPDS uses Basic Auth (the books group requires session auth), and in production the Caddy `/opds/*` handler proxies to the backend while bare `/books/*` falls through to the SPA. The cover endpoint lives in `pkg/opds/handlers.go` (`bookCover`) and is built off `apiBase + "/opds/v1"` in `bookToEntryWithKepub` so an `X-Forwarded-Prefix` (e.g. `/api` in dev) is preserved.

### eReader Browser UI (`pkg/ereader/`)

Server-rendered HTML pages for stock eReader browsers (Kobo, Kindle) that can't use OPDS or the React frontend.

**Key files:**
- `handlers.go` - HTTP handlers mirroring OPDS structure
- `templates.go` - Go string templates for HTML rendering
- `middleware.go` - API key authentication from URL path
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
- Selection goes through `pkg/covers.SelectFile`, shared with the books, series, and OPDS handlers. The shared selector must continue to fall back to any available cover type when the preferred aspect ratio has no covers — eReaders showing an audiobook-only book still need to get a cover

### Authentication & Authorization

The app uses Role-Based Access Control (RBAC) with two layers:
1. **Global permissions** - Role-based access to features
2. **Library access** - User-specific library visibility

#### Permission Resources (`pkg/models/role.go`)

| Resource | Description | Used For |
|----------|-------------|----------|
| `libraries` | Library management | Create/update libraries, filesystem operations |
| `books` | Book/file operations | Books, files, covers, chapters, genres, tags, publishers, imprints |
| `people` | Author/narrator management | Create/update/delete/merge people |
| `series` | Series management | Update/delete/merge series |
| `users` | User administration | Create users, manage roles, reset passwords |
| `jobs` | Background jobs | Trigger scans, view job status |
| `config` | Application config | View app configuration |

#### Permission Operations

- `read` - View/retrieve data
- `write` - Create/modify/delete data

#### Predefined Roles

| Role | Permissions |
|------|-------------|
| `admin` | All 14 permissions (full access) |
| `editor` | Read+write: libraries, books, series, people (8 permissions) |
| `viewer` | Read-only: libraries, books, series, people (4 permissions) |

#### Adding Permissions to Routes

**Group-level permission (all routes in group):**
```go
booksGroup := e.Group("/books")
booksGroup.Use(authMiddleware.Authenticate)
booksGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
```

**Individual route permission:**
```go
g.POST("/:id", h.update, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
```

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
3. **Search endpoints need explicit `books:read`** - Search returns book data, so require the permission
4. **User-scoped resources don't need global permissions** - Lists, API keys, settings are user-scoped
5. **Sharing features require `users:read`** - To share, users must see the user list
6. **Both frontend and backend checks required** - Backend for security, frontend for UX

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
2. Add to admin role in migration or update existing admin roles
3. Update `app/components/library/PermissionMatrix.tsx` to display in UI
4. Add permission checks to relevant routes/handlers

### API Conventions

- **JSON field naming**: All JSON request and response payloads use `snake_case` for field names (e.g., `created_at`, `last_accessed_at`, not `createdAt`)
- Go struct tags should use `json:"snake_case_name"` format
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

- **Slice fields need `mod:"dive"` for inner modifiers to fire**: `mold/v4` (used by the binder for `mod:"trim"` etc.) treats slice/array/map/pointer-to-slice fields as opaque single values by default. If a payload field is shaped like `[]Inner` or `*[]Inner` and `Inner` has any `mod:"..."` tags on its fields, the parent slice field must carry `mod:"dive"` — otherwise the inner modifiers are silently no-ops. This mirrors `validator/v10`'s `dive`, but the two are independent: both tags are required on the same field when both validation and modification need to traverse. Reference: `UpdateFilePayload.Identifiers *[]IdentifierPayload` in `pkg/books/validators.go` (carries `mod:"dive" validate:"omitempty,dive"`); regression test in `pkg/binder/binder_test.go` (`TestBind_DiveRequiredForSliceModTraversal`).

- **Bun table aliases in WHERE/ORDER clauses**: Bun auto-aliases tables using the first letter of the table name (e.g., `books` → `b`, `files` → `f`, `persons` → `p`). Always use these aliases in `Where()`, `Order()`, and other SQL clauses — never use the full table name:

```go
// ❌ WRONG - "book" is not a valid alias, causes "no such column" error
q.Where("book.id = ?", id)

// ✅ CORRECT - Bun aliases "books" table as "b"
q.Where("b.id = ?", id)
```

Check existing queries in `pkg/books/service.go` for reference. Common aliases: `b` (books), `f` (files), `a` (authors), `p` (persons), `s` (series), `bs` (book_series), `ch` (chapters), `n` (narrators).

### Config

- Self-hosted app with config file-based configuration
- Each config field is also configurable by environment variables
- **CRITICAL**: If a new field is added to `config.Config` in `pkg/config/config.go`:
  - `shisho.example.yaml` **MUST** be updated with the new field, its env var name, default value, and a description. This file must always be a complete reference of all configurable fields.
  - Exception: `environment` is a test-only internal field and should NOT be included in `shisho.example.yaml`.
  - The Server Settings UI page (`app/components/pages/AdminSettings.tsx`) must be updated to display the new field (all non-secret config fields should be shown)

### Sidecars

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

## File Retrieval and Relations

**CRITICAL**: When calling `WriteFileSidecarFromModel()` or `ComputeFingerprint()`, the file MUST have all relations loaded:

| Function | Required Relations |
|----------|-------------------|
| `WriteFileSidecarFromModel()` | Narrators, Identifiers, Publisher, Imprint, Chapters |
| `ComputeFingerprint()` | Narrators, Identifiers |

**Use the right retrieval method:**
- `RetrieveFile()` - File with Book, Identifiers, and Narrators. Use for most lookups.
- `RetrieveFileWithRelations()` - Complete file with all relations (adds Publisher, Imprint, Chapters). **Use this for sidecar writing or fingerprinting.**
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
    newPath, err := fileutils.RenameOrganizedFile(file.Filepath, organizeOpts)
    if err != nil {
        // Handle error
    }
    // Update file.Filepath in database
}
```

## Adding New Entity Types

When adding a new entity type (like Publisher, Imprint, Genre, Tag) that files or books reference:

1. Create model in `pkg/models/` with appropriate fields and Bun struct tags
2. Create service in `pkg/{entity}/service.go` following the pattern from `pkg/genres/service.go`:
   - Include `FindOrCreate{Entity}()` method for scanner to use
   - Include `Retrieve{Entity}()` and `List{Entity}s()` methods
3. Add service to worker (`pkg/worker/worker.go`) and initialize in `New()`
4. Update scanner to use the new service for entity creation

## Search Index (FTS)

The app uses SQLite Full-Text Search (FTS5) for fast searching.

**Key files:**
- `pkg/search/service.go` - Search service with index methods
- FTS tables: `books_fts`, `series_fts`, `persons_fts`, `genres_fts`, `tags_fts`

**IMPORTANT - Search Index Updates:**

When creating or modifying entities that are searchable, ensure the FTS index is updated:

1. **New entities created via `FindOrCreate*()` methods MUST be indexed** - When `FindOrCreateGenre()`, `FindOrCreateTag()`, etc. create a new entity, call `IndexGenre()`, `IndexTag()`, etc. afterward
2. **Entity updates must re-index** - Call `Index*()` after updating an entity's searchable fields
3. **Entity deletions must remove from index** - Call `DeleteFrom*Index()` when deleting entities
4. **Book metadata changes affecting search** - When book authors, series, genres, or tags change, call `IndexBook()` to update the book's search index

**Example pattern:**
```go
// Track new entity IDs when creating associations
newGenreIDs := make([]int, 0)
for _, name := range params.Genres {
    genre, _ := h.genreService.FindOrCreateGenre(ctx, name, libraryID)
    newGenreIDs = append(newGenreIDs, genre.ID)
}
// Index new entities after creation
for _, id := range newGenreIDs {
    genre, _ := h.genreService.RetrieveGenre(ctx, opts{ID: &id})
    h.searchService.IndexGenre(ctx, genre)
}
```

## File Processing Flow

1. **Scan Job Creation**: User triggers scan via API
2. **File Discovery**: Worker scans library paths for `.epub`, `.m4b`, `.cbz` files
3. **Metadata Extraction**: Parse files to extract title, authors, cover images
4. **Database Storage**: Create/update Book and File records
5. **Cover Generation**: Save individual covers + generate canonical covers
6. **Priority Resolution**: Use data source priority to resolve metadata conflicts

## Chapters System

Chapters are file-level metadata extracted from CBZ, EPUB, and M4B files.

### Database Model (`pkg/models/chapter.go`)

```go
type Chapter struct {
    ID               int
    FileID           int       // Foreign key to files table
    ParentID         *int      // Self-referential for nested chapters (EPUB)
    SortOrder        int       // Order within parent (0-indexed)
    Title            string
    StartPage        *int      // CBZ: 0-indexed page number
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
| GET | `/books/files/:id/chapters` | List chapters (nested tree) |
| PUT | `/books/files/:id/chapters` | Replace chapters (requires write permission) |

### Worker Integration

Chapters are synced during file scan in `pkg/worker/scan.go`:
- After file metadata is saved, chapters from `ParsedMetadata.Chapters` are synced
- Uses `chapterService.ReplaceChapters()` for atomic replacement
- Errors are logged as warnings (non-fatal to scan)

### Position Fields by File Type

| File Type | Position Field | Example |
|-----------|---------------|---------|
| CBZ | `StartPage` | `0` (first page) |
| M4B | `StartTimestampMs` | `3600000` (1 hour) |
| EPUB | `Href` | `"chapter1.xhtml"` |

### Validation

PUT endpoint validates chapters against file constraints:
- CBZ: `start_page` must be < `file.PageCount`
- M4B: `start_timestamp_ms` must be <= `file.AudiobookDurationSeconds * 1000`

## Key Directories

| Purpose | Location |
|---------|----------|
| Entry point | `cmd/api/main.go` |
| Models | `pkg/models/` |
| Domain services | `pkg/{domain}/` (books, jobs, libraries, chapters, etc.) |
| File parsers | `pkg/epub/`, `pkg/cbz/`, `pkg/mp4/` |
| File generators | `pkg/filegen/` |
| Scanner/Worker | `pkg/worker/` |
| Sidecars | `pkg/sidecar/` |
| Search | `pkg/search/` |
| Config | `pkg/config/` |
