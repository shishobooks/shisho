# Supplementary Files Design

Support for non-primary files (e.g., PDF supplements bundled with audiobooks) that are visible on the book page and downloadable.

## Requirements

- Supplementary files visible on book page, downloadable as-is
- Any file type (not just PDFs)
- Exclude shisho-specific files (covers, sidecars)
- No processing needed - straight download
- Basic metadata only (filename, size)

## Data Model

### Files Table Changes

Add `file_role` column to existing `files` table:

```sql
file_role TEXT NOT NULL DEFAULT 'main'  -- 'main' or 'supplement'
```

Edit the existing migration directly (pre-production app).

### Supplements Use

- `file_role = 'supplement'`
- `book_id` linking to parent book
- `library_id` for consistency
- `filepath`, `filesize_bytes` populated
- `file_type` set to extension (e.g., `pdf`, `txt`) for display

### Supplements Don't Use

- Cover fields (cover_image_path, cover_mime_type, cover_source, cover_page)
- Audiobook fields (duration, bitrate)
- Narrator/publisher/imprint/identifier relations
- These remain nullable, just unused

## Discovery Logic

### Server Configuration

New config option in `pkg/config/config.go` and `shisho.example.yaml`:

```yaml
# Patterns to exclude from supplement discovery
# Default excludes hidden files and common system files
supplement_exclude_patterns:
  - ".*"           # hidden files (dotfiles)
  - ".DS_Store"
  - "Thumbs.db"
  - "desktop.ini"
```

### Directory-Based Books

For a book in `MyBook/`:

1. List all files in book directory **recursively** (including subdirectories)
2. Exclude:
   - Main file types (`.epub`, `.m4b`, `.cbz`)
   - Shisho files: `*.cover.*` and `*.metadata.json`
   - Server-configured exclusion patterns
   - Directories themselves
3. Everything remaining is a supplement

### Root-Level Books

For a main file `MyBook.m4b` at library root:

1. Find files in same directory matching base name prefix
2. `MyBook.m4b` picks up `MyBook.pdf`, `MyBook - Guide.txt`, `MyBook_notes.pdf`
3. Same exclusion rules apply
4. Won't pick up `OtherBook.pdf` (different base name)

### Root-Level File Grouping (New Behavior)

**Current behavior:** Each root-level file becomes its own book.

**New behavior:** Root-level files with same base name are grouped as one book:
- `MyBook.epub` + `MyBook.m4b` → one book with two main files
- `MyBook.pdf` → supplement of that book

This fixes a broader issue where root-level files of the same book weren't grouped.

## API

### No New Endpoints

Existing endpoints handle supplements:

- `GET /files/:id/download/original` - direct download (works as-is)
- `GET /files/:id/download` - modify to delegate to original handler for supplements

### Download Handler Change

In `downloadFile` handler, check if file is a supplement:

```go
if file.FileRole == "supplement" {
    return h.downloadOriginalFile(c)
}
// ... existing processing logic
```

### Book Response

`GET /api/books/:id` includes supplements in `files` array with `file_role` field. Frontend filters by role.

## Frontend

### Book Detail Page

Add "Supplements" section below main files:
- Simple list: filename, file type/extension, size, download button
- Only shown if supplements exist

### FileEditDialog Changes

Same dialog component, different form based on `file_role`:

**For supplements:**
- Role toggle only (to upgrade to main file)
- When upgrading: show full main file form

**For main files:**
- Full form (narrators, identifiers, publisher, imprint, etc.)
- Role toggle (to downgrade to supplement)
- **Downgrade shows confirmation warning** explaining metadata will be cleared

### Downgrade Behavior

When changing `file_role` from `main` to `supplement`:
- Clear: narrators, identifiers, publisher, imprint, cover fields, audiobook fields, release_date, url
- Frontend confirmation required before proceeding

### TypeScript Types

`file_role` will be auto-generated via tygo from Go structs.

## Exclusions

### OPDS

Supplements are **not** included in OPDS feeds. Can be added later if needed.

### Sidecars

No sidecar files written for supplements (no special metadata to store).

### Search

Supplements not indexed for search. They're found via their parent book.

## Implementation Tasks

1. **Database:** Add `file_role` column to files table in existing migration
2. **Config:** Add `supplement_exclude_patterns` to config and example yaml
3. **Scanner:**
   - Implement supplement discovery for directory-based books
   - Implement supplement discovery for root-level books
   - Implement root-level file grouping by base name
4. **API:** Modify download handler to delegate for supplements
5. **Frontend:**
   - Add supplements section to BookDetail
   - Modify FileEditDialog for role-based forms
   - Add downgrade confirmation dialog
6. **Cleanup:** Ensure supplements are cleaned up when book is deleted (FK cascade) and when files are removed from disk
