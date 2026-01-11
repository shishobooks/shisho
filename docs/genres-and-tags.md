# Genres and Tags

This document describes the genres and tags feature in Shisho, including how they are stored, parsed, generated, and used throughout the application.

## Overview

Genres and tags are metadata categories that help organize books:
- **Genres**: Classification categories (e.g., Fantasy, Science Fiction, Romance)
- **Tags**: User-defined labels (e.g., Must Read, Favorites, To Review)

Both support:
- Many-to-many relationships with books
- Library-scoped normalization (case-insensitive deduplication)
- Parsing from file metadata during scanning
- Writing back to file metadata during downloads
- Frontend filtering and browsing

## Database Schema

### Tables

```sql
-- Genres table (normalized per library)
CREATE TABLE genres (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    UNIQUE(name COLLATE NOCASE, library_id)
);

-- Book-genre associations
CREATE TABLE book_genres (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    genre_id INTEGER NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    UNIQUE(book_id, genre_id)
);

-- Tags table (same pattern)
CREATE TABLE tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    UNIQUE(name COLLATE NOCASE, library_id)
);

-- Book-tag associations
CREATE TABLE book_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    UNIQUE(book_id, tag_id)
);

-- Full-text search
CREATE VIRTUAL TABLE genres_fts USING fts5(name, content='genres', content_rowid='id');
CREATE VIRTUAL TABLE tags_fts USING fts5(name, content='tags', content_rowid='id');
```

### Book Table Additions

```sql
genre_source TEXT,  -- DataSource for genre metadata
tag_source TEXT     -- DataSource for tag metadata
```

## File Format Support

### EPUB

**Parsing:**
- Genres from `<dc:subject>` elements (one per genre)
- Tags from `<meta name="calibre:tags" content="Tag1, Tag2"/>`

**Generation:**
- Genres written as individual `<dc:subject>` elements
- Tags written as `<meta name="calibre:tags" content="..."/>`

### CBZ

**Parsing:**
- Genres from `<Genre>` field in ComicInfo.xml (comma-separated)
- Tags from `<Tags>` field in ComicInfo.xml (comma-separated)

**Generation:**
- Genres written to `<Genre>` as comma-separated list
- Tags written to `<Tags>` as comma-separated list

### M4B

**Parsing:**
- Genres from `©gen` atom (primary genre, or comma-separated)
- Tags from `----:com.shisho:tags` freeform atom

**Generation:**
- Genres written to `©gen` atom as comma-separated list
- Tags written to `----:com.shisho:tags` freeform atom

### KePub (CBZ conversion)

When converting CBZ to KePub:
- Genres written as `<dc:subject>` elements
- Tags written as `<meta name="calibre:tags">`

## API Endpoints

### Genres

```
GET    /api/genres                    # List all genres
GET    /api/genres?library_id=1       # List genres for library
GET    /api/genres?search=fantasy     # Search genres
GET    /api/genres/:id                # Get genre by ID
PATCH  /api/genres/:id                # Update genre name
DELETE /api/genres/:id                # Delete genre
```

### Tags

```
GET    /api/tags                      # List all tags
GET    /api/tags?library_id=1         # List tags for library
GET    /api/tags?search=read          # Search tags
GET    /api/tags/:id                  # Get tag by ID
PATCH  /api/tags/:id                  # Update tag name
DELETE /api/tags/:id                  # Delete tag
```

### Books Filtering

```
GET /api/books?genre_ids=1,2,3        # Filter by genre IDs
GET /api/books?tag_ids=4,5            # Filter by tag IDs
```

### Book Updates

```json
POST /api/books/:id
{
    "genres": ["Fantasy", "Adventure"],
    "tags": ["Must Read", "Favorites"]
}
```

## Frontend

### Routes

- `/libraries/:libraryId/genres` - Browse genres list
- `/libraries/:libraryId/tags` - Browse tags list

### TopNav

Genres and Tags buttons in the library navigation bar.

### Home Page Filtering

- URL params: `?genre_ids=1,2&tag_ids=3`
- Active filter badges with remove functionality

## Data Source Priority

Genre and tag sources follow the standard priority:
1. Manual (user edits via API)
2. Sidecar (metadata.json)
3. FileMetadata (parsed from file)
4. Filepath (directory structure)

Lower number = higher priority. Manual edits always take precedence.

## Download Cache Fingerprinting

Genres and tags are included in the download cache fingerprint (`pkg/downloadcache/fingerprint.go`). This means:
- When genres or tags are updated for a book, the cached download is invalidated
- The next download request generates a new file with the updated metadata
- Genres and tags are sorted alphabetically for consistent hashing

## Normalization

Genres and tags are normalized per library:
- Leading/trailing whitespace trimmed
- Case-insensitive matching (COLLATE NOCASE)
- Duplicate prevention within library scope
- Empty names rejected

Example: "fantasy", "Fantasy", and " Fantasy " all map to the same genre.

## Orphan Cleanup

After scanning, orphaned genres/tags (with no book associations) are automatically cleaned up via:
- `genreService.CleanupOrphanedGenres(ctx)`
- `tagService.CleanupOrphanedTags(ctx)`

## Sidecar Support

Genres and tags are stored in the metadata sidecar:

```json
{
    "title": "Book Title",
    "genres": ["Fantasy", "Adventure"],
    "tags": ["Must Read", "Favorites"]
}
```

## Related Files

### Backend
- `pkg/models/genre.go` - Genre and BookGenre models
- `pkg/models/tag.go` - Tag and BookTag models
- `pkg/genres/` - Genre service package
- `pkg/tags/` - Tag service package
- `pkg/books/service.go` - Book-genre/tag associations
- `pkg/books/handlers.go` - Genre/tag update handling
- `pkg/books/validators.go` - Genre/tag filtering params
- `pkg/mediafile/mediafile.go` - ParsedMetadata types
- `pkg/epub/opf.go` - EPUB genre/tag parsing
- `pkg/cbz/cbz.go` - CBZ genre/tag parsing
- `pkg/mp4/metadata.go` - M4B genre/tag parsing
- `pkg/filegen/epub.go` - EPUB genre/tag generation
- `pkg/filegen/cbz.go` - CBZ genre/tag generation
- `pkg/filegen/m4b.go` - M4B genre/tag generation
- `pkg/kepub/cbz.go` - KePub genre/tag generation
- `pkg/sidecar/types.go` - Sidecar genre/tag fields
- `pkg/worker/scan.go` - Scanner genre/tag handling
- `pkg/downloadcache/fingerprint.go` - Download cache fingerprint with genres/tags

### Frontend
- `app/router.tsx` - Genre/tag page routes
- `app/components/pages/GenresList.tsx` - Genres list page
- `app/components/pages/TagsList.tsx` - Tags list page
- `app/components/pages/Home.tsx` - Genre/tag filtering
- `app/components/library/TopNav.tsx` - Genre/tag navigation
- `app/hooks/queries/genres.ts` - Genre query hooks
- `app/hooks/queries/tags.ts` - Tag query hooks
- `app/types/generated/genres.ts` - Generated genre types
- `app/types/generated/tags.ts` - Generated tag types
