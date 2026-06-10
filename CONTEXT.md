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
A publishing entity associated with a file. Publishers form a hierarchy via an optional parent relationship — a publisher can have one parent and many children. An imprint, a division, and a conglomerate are all represented as publishers at different levels of the tree. A file references exactly one publisher (at any level of the hierarchy); ancestor publishers are derived by walking up the tree.
_Avoid_: imprint (as a separate concept — imprints are just publishers with a parent)

**Alias**:
An alternative name for a resource (series, person, genre, tag, or publisher) that resolves to the canonical resource during name-based lookups. Aliases are library-scoped and case-insensitive. A resource can have many aliases; each alias belongs to exactly one resource.
_Avoid_: synonym, alternate name, variant

**Primary Name**:
The canonical display name of a resource, shown in the UI and used as the authoritative label.
_Avoid_: canonical name, display name

**Preferred Cover**:
A per-file designation that prioritizes one file's cover over others in the same format category (ebook or audiobook) when representing the book. At most one file per category per book can be preferred.
_Avoid_: primary cover, default cover, canonical cover

### Workflows

**Scan**:
The process of discovering and importing files from the filesystem into a library, extracting embedded metadata and resolving resources by name.

**Identify**:
An interactive workflow where a user matches a book against an external source (via plugins) to enrich metadata.

**Merge**:
Combining two resources of the same type into one, transferring all associations from the source to the target and deleting the source.

**Resync**:
Re-running the scan pipeline for an existing book or file, in one of three modes: scan (pick up new metadata without overwriting manual edits), refresh (re-scan as if new, re-enriching via plugins), or reset (clear all metadata including manual edits and re-scan from file metadata only, without plugins).
_Avoid_: rescan (in new code; some existing identifiers, comments, and docs/UI copy still say "Rescan")

**Sidecar**:
A `.metadata.json` file placed alongside a book or file that provides metadata overrides.

## Relationships

- A **Library** contains many **Books**, **Series**, **Persons**, **Genres**, **Tags**, and **Publishers**
- A **Book** has one or more **Files**
- A **Book** has many **Genres**, **Tags**, **Authors** (persons), and **Series**
- A **File** has at most one **Publisher** and many **Narrators** (persons)
- A **Publisher** has at most one parent **Publisher** and many child **Publishers** (self-referential hierarchy)
- A **Resource** (series, person, genre, tag, publisher) has many **Aliases**
- An **Alias** belongs to exactly one resource and is unique within its resource type and library

## Example dialogue

> **Dev:** "A plugin returns genre 'Non-fiction' but the library already has 'Nonfiction'. What happens?"
> **Domain expert:** "If 'Non-fiction' is an **alias** of the 'Nonfiction' **genre**, the **scan** resolves it to the existing genre using the **alias**. The **primary name** 'Nonfiction' is what appears in the UI. If there's no alias match, a new genre is created."

> **Dev:** "What happens when I **merge** 'Sci-Fi' into 'Science Fiction'?"
> **Domain expert:** "All books move to 'Science Fiction', 'Sci-Fi' becomes an **alias** of 'Science Fiction', and the old genre is deleted. Any aliases 'Sci-Fi' had also transfer."

## Flagged ambiguities

- The database table for persons is `persons`, but the URL path uses `people` and the Go package is `pkg/people`. The domain term is **Person**.
- **Publisher** is file-level metadata, not book-level. This differs from series/genre/tag/author which are book-level.
