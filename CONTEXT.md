# Shisho

Shisho is a self-hosted media library server for ebooks and audiobooks. It scans local files, enriches metadata via plugins, and provides a web UI for browsing and managing collections.

## Language

### Resources & Metadata

**Book**:
A logical work in the library, composed of one or more files.
_Avoid_: title, item

**File**:
A single media file (EPUB, M4B, CBZ, PDF, etc.) belonging to a book.
_Avoid_: asset, media

**Library**:
A top-level organizational unit that scopes all resources. Each resource belongs to exactly one library.
_Avoid_: collection, shelf

**Series**:
A named sequence of books (e.g., "Harry Potter"). A book can belong to multiple series with a position number.
_Avoid_: sequence, collection

**Person**:
An individual who participates in creating a book or narrating a file. A person can be an author (book-level) or narrator (file-level).
_Avoid_: creator, contributor

**Genre**:
A category classification for a book (e.g., "Science Fiction").

**Tag**:
A user-defined label for a book, more granular than genre.

**Publisher**:
The publishing entity for a file.

**Imprint**:
A branded subdivision of a publisher, associated with a file.

**Alias**:
An alternative name for a resource (series, person, genre, tag, publisher, or imprint) that resolves to the canonical resource during name-based lookups. Aliases are library-scoped and case-insensitive. A resource can have many aliases; each alias belongs to exactly one resource.
_Avoid_: synonym, alternate name, variant

**Primary Name**:
The canonical display name of a resource, shown in the UI and used as the authoritative label.
_Avoid_: canonical name, display name

### Workflows

**Scan**:
The process of discovering and importing files from the filesystem into a library, extracting embedded metadata and resolving resources by name.

**Identify**:
An interactive workflow where a user matches a book against an external source (via plugins) to enrich metadata.

**Merge**:
Combining two resources of the same type into one, transferring all associations from the source to the target and deleting the source.

**Sidecar**:
A `.metadata.json` file placed alongside a book or file that provides metadata overrides.

## Relationships

- A **Library** contains many **Books**, **Series**, **Persons**, **Genres**, **Tags**, **Publishers**, and **Imprints**
- A **Book** has one or more **Files**
- A **Book** has many **Genres**, **Tags**, **Authors** (persons), and **Series**
- A **File** has at most one **Publisher**, at most one **Imprint**, and many **Narrators** (persons)
- A **Resource** (series, person, genre, tag, publisher, imprint) has many **Aliases**
- An **Alias** belongs to exactly one resource and is unique within its resource type and library

## Example dialogue

> **Dev:** "A plugin returns genre 'Non-fiction' but the library already has 'Nonfiction'. What happens?"
> **Domain expert:** "If 'Non-fiction' is an **alias** of the 'Nonfiction' **genre**, the **scan** resolves it to the existing genre using the **alias**. The **primary name** 'Nonfiction' is what appears in the UI. If there's no alias match, a new genre is created."

> **Dev:** "What happens when I **merge** 'Sci-Fi' into 'Science Fiction'?"
> **Domain expert:** "All books move to 'Science Fiction', 'Sci-Fi' becomes an **alias** of 'Science Fiction', and the old genre is deleted. Any aliases 'Sci-Fi' had also transfer."

## Flagged ambiguities

- The database table for persons is `persons`, but the URL path uses `people` and the Go package is `pkg/people`. The domain term is **Person**.
- **Publisher** and **Imprint** are file-level metadata, not book-level. This differs from series/genre/tag/author which are book-level.
