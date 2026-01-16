---
name: backend
description: You MUST use this before working on any Go backend code or working with the API. Covers Echo handlers, Bun ORM, worker jobs, file parsers, and the metadata sync checklist.
user-invocable: false
---

# Shisho Backend Development

This skill documents backend patterns and conventions specific to Shisho.

## Stack

- Go with Echo web framework
- Bun ORM with SQLite database
- Air for hot reload
- Hivemind for process management

## Architecture

### Entry Point

`cmd/api/main.go` starts both HTTP server and background worker.

### Core Services Pattern

Each domain (books, jobs, libraries) has:
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

### File Types

- To learn more about all the file types that we support, refer to these skills:
  - EPUB: `.claude/skills/epub.md`
  - CBZ: `.claude/skills/cbz.md`
  - M4B: `.claude/skills/m4b.md`
  - KePub: `.claude/skills/kepub.md`

### Cover Image System

- Individual file covers: `{filename}.cover.{ext}`
- Canonical covers: `cover.{ext}` (book priority) or `audiobook_cover.{ext}` (fallback)
- Book model has `ResolveCoverImage()` method that finds covers dynamically
- API endpoints: `/books/{id}/cover` (canonical) and `/files/{id}/cover` (individual)

**CRITICAL - CoverImagePath stores FILENAME ONLY:**

`file.CoverImagePath` stores just the filename (e.g., `MyBook.cbz.cover.jpg`), NOT the full path. The full path is constructed at runtime by joining the book directory with the filename.

When updating `CoverImagePath` (e.g., after renaming a file), always use `filepath.Base()` to extract just the filename:

```go
// ❌ WRONG - stores full path, breaks cover serving
newCoverPath := fileutils.ComputeNewCoverPath(*file.CoverImagePath, newPath)
file.CoverImagePath = &newCoverPath

// ✅ CORRECT - stores filename only
newCoverPath := filepath.Base(fileutils.ComputeNewCoverPath(*file.CoverImagePath, newPath))
file.CoverImagePath = &newCoverPath
```

**Why this matters:** The `bookCover` handler constructs the full path as:
```go
coverPath := filepath.Join(coverDir, *coverFile.CoverImagePath)
```

If `CoverImagePath` contains a full path, this results in an invalid doubled path like `/path/to/path/to/cover.jpg`.

### Data Source Priority System

Metadata sources ranked (lower number = higher precedence):
```
0: Manual (highest)
1: Sidecar
2: EPUB/CBZ Metadata
3: M4B Metadata
4: Filepath (lowest)
```

Used to determine which metadata to keep when conflicts occur.

### OPDS

- OPDS v1.2 server hosted in the application
- As new functionality is added, keep the OPDS server up-to-date with the new features

### Authentication

- RBAC is used throughout the app
- Authn and authz needs to be considered for all pieces of functionality
- Both frontend and backend checks need to be made so that everything is protected on all fronts

### Config

- Self-hosted app with config file-based configuration
- Each config field is also configurable by environment variables
- If a new field is added to `config.Config` in `pkg/config/config.go`, `shisho.example.yaml` should also be updated

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
| `WriteFileSidecarFromModel()` | Narrators, Identifiers, Publisher, Imprint |
| `ComputeFingerprint()` | Narrators, Identifiers |

**Use the right retrieval method:**
- `RetrieveFile()` - File with Book, Identifiers, and Narrators. Use for most lookups.
- `RetrieveFileWithRelations()` - Complete file with all relations (adds Publisher, Imprint). **Use this for sidecar writing or fingerprinting.**
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

## Metadata Sync Checklist

When adding or modifying book/file metadata fields, ensure these files are updated:

1. **Sidecar types** (`pkg/sidecar/types.go`) - Add field to `BookSidecar` or `FileSidecar` struct
2. **Sidecar conversion** (`pkg/sidecar/sidecar.go`) - Update `BookSidecarFromModel()` or `FileSidecarFromModel()`
3. **Download fingerprint** (`pkg/downloadcache/fingerprint.go`) - Add field to `Fingerprint` struct and `ComputeFingerprint()` so cache invalidates when metadata changes
4. **Download filename** (`pkg/downloadcache/filename.go`) - If the field affects display names (like `file.Name`), update `FormatDownloadFilename()` and `FormatKepubDownloadFilename()` to use it
5. **File parsers** - Update to extract the new field:
   - EPUB: `pkg/epub/opf.go`
   - CBZ: `pkg/cbz/cbz.go`
   - M4B: `pkg/mp4/metadata.go`
6. **File generators** - Update to write the field back:
   - EPUB: `pkg/filegen/epub.go`
   - CBZ: `pkg/filegen/cbz.go`
   - M4B: `pkg/filegen/m4b.go`
   - KePub: `pkg/kepub/cbz.go` (for CBZ-to-KePub conversion)
7. **Scanner** (`pkg/worker/scan.go`) - Handle the new field during scanning
8. **ParsedMetadata** (`pkg/mediafile/mediafile.go`) - Add field if it's parsed from files
9. **API relations** (`pkg/books/service.go`) - If adding a relation to File (like Publisher, Imprint), add `.Relation("Files.NewRelation")` to all book query methods: `RetrieveBook`, `RetrieveBookByFilePath`, and `listBooksWithTotal`
10. **File retrieval** (`pkg/books/service.go`) - If the new field is a File relation used by sidecar or fingerprint, add it to `RetrieveFileWithRelations()` and consider adding to `RetrieveFile()` if lightweight
11. **File reorganization** (`pkg/books/handlers.go`) - If the field affects file organization (like `Name`), trigger reorganization when the field changes via the edit endpoint
12. **Organization options** (`pkg/books/service.go`, `pkg/books/handlers.go`) - If the field should be used for file organization, update all places that build `fileutils.OrganizedNameOptions` to use the new file-level field
13. **UI display** (`app/components/pages/BookDetail.tsx`) - Display the new field in the book detail view

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

## Key Directories

| Purpose | Location |
|---------|----------|
| Entry point | `cmd/api/main.go` |
| Models | `pkg/models/` |
| Domain services | `pkg/{domain}/` (books, jobs, libraries, etc.) |
| File parsers | `pkg/epub/`, `pkg/cbz/`, `pkg/mp4/` |
| File generators | `pkg/filegen/` |
| Scanner/Worker | `pkg/worker/` |
| Sidecars | `pkg/sidecar/` |
| Search | `pkg/search/` |
| Config | `pkg/config/` |
