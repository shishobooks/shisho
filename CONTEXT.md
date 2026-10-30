# Shisho

Shisho is a self-hosted media library server for ebooks and audiobooks. It scans local files, enriches metadata via plugins, and provides a web UI for browsing and managing collections.

## Language

### Resources & Metadata

**Book**:
A logical work in the library, composed of one or more files.
_Avoid_: title, item

**File**:
A single media file (EPUB, KePub, MOBI, AZW3, M4B, CBZ, PDF, etc.) belonging to a book. MOBI is the older Kindle File Type and AZW3 the newer KF8 one.
_Avoid_: asset, media

**File Type**:
The kind of a **File** (EPUB, KePub, MOBI, AZW3, CBZ, PDF, M4B, and so on), and equally the kind delivered after any **Conversion**. A KePub is a File named `.kepub.epub`, or `.kepub` as Calibre stores one; one converted from an EPUB or CBZ exists only as a delivery.
_Avoid_: format, file format

**Reflowable**:
An ebook **File** whose text re-wraps to fit the screen (EPUB, KePub, MOBI, AZW3). Classified by **File Type**, so a fixed-layout EPUB is still reflowable. Positions within it are locations in the text, not page numbers.
_Avoid_: text book, ebook (ebook also includes page-based files)

**Page-based**:
An ebook **File** made of fixed pages (CBZ, PDF). Positions within it are page numbers.
_Avoid_: fixed-layout (that is an EPUB rendering mode, not this distinction)

**Original File**:
A **File** exactly as it sits in the library on disk. The web app calls it the "unmodified file" (**Download unmodified file**), because its download menu uses "original" for the File's own File Type, as opposed to a **Conversion**.
_Avoid_: source file, raw file

**Generated File**:
A **File** rebuilt at delivery time to carry Shisho's current metadata, possibly after a **Conversion**. It is the default download; the **Original File** is the alternative.
_Avoid_: converted file, processed file

**Library**:
A top-level organizational unit that scopes all resources. Each resource belongs to exactly one library.
_Avoid_: collection, shelf

**Series**:
A named sequence of books (e.g., "Harry Potter"). A book belongs to a given series at most once, at a **series number** that is either a single position or a contiguous range (when the book is an **omnibus**).
_Avoid_: sequence, collection

**Series Number**:
A book's position within a series. Usually a single number; for an **omnibus** it is a contiguous range with a start and an end (e.g., 1-3), where the start is also the value used to order the book within the series. For comic (CBZ) books it additionally carries a unit of volume or chapter.
_Avoid_: series index, position (as a separate term)

**Omnibus**:
A single book that collects a contiguous run of a series' entries, recorded as one series membership whose **series number** spans a range (e.g., 1-3), not one membership per entry. Omnibuses are ordered after the individually-numbered books of the series. Non-contiguous bundles (e.g. books 1, 2, 10) are a distinct, out-of-scope concept.
_Avoid_: bind-up, boxed set

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
A per-file designation that prioritizes one file's cover over others in the same category (ebook or audiobook) when representing the book. At most one file per category per book can be preferred.
_Avoid_: primary cover, default cover, canonical cover

### Delivery

**Device**:
One reading app on one piece of hardware, however it connects to Shisho. A Kobo running both Kobo's own reader and KOReader is two Devices, while Kobo's reader syncing and also using the eReader browser is one Device with two connections. A Device belongs to one user, and its Device type (such as Kobo or Kindle) limits the File Types it can receive and supplies preset **Delivery Rules**.
_Avoid_: client, connection, API key (the credential is part of the Device, not the Device itself)

**Delivery Rules**:
An ordered list of rules, at most one per source **File Type**, each naming the File Type to deliver it as (itself, or another reachable through **Conversion**). A source File Type with no rule is not delivered. Among a book's **Files** in one category (ebook or audiobook), the File matching the earliest rule is delivered, along with any File tied with it on that rule.
_Avoid_: format preference, download preference, conversion settings

**Conversion**:
Delivering a **File** as a File Type other than its own, done at delivery time and never stored in the library. Conversions chain: a File Type reachable only through an intermediate one still counts as reachable.
_Avoid_: transcoding, export

**Input Conversion**:
A plugin turning a file into a new **File** of another File Type during a **Scan**. Unlike a **Conversion**, its result is stored in the library.
_Avoid_: conversion (alone)

### Workflows

**Scan**:
The process of discovering and importing files from the filesystem into a library, extracting embedded metadata and resolving resources by name.

**Identify**:
An interactive workflow where a user matches a book against an external source (via plugins) to enrich metadata.

The following terms describe how Identify assigns provenance (ADR 0006).

**Plugin Proposal**:
A metadata value or relationship collection suggested by a plugin during Identify.

**Proposal Acceptance**:
When the user has not made an Explicit Clear, their final metadata result differs semantically from the current metadata and matches the Plugin Proposal. This remains a Proposal Acceptance if the user temporarily edited the proposal before restoring it, unless the final result is also a no-op against the current metadata.

**Metadata Provenance**:
The source responsible for establishing the current meaning of a metadata value or relationship. Under the Identify rules, a no-op preserves existing provenance, an Explicit Clear removes provenance, a Proposal Acceptance has plugin provenance, and any other changed nonempty result has manual provenance.

**Relationship Provenance**:
The provenance of a relationship collection as a whole. The Identify rules use Relationship Provenance for Authors, Genres, Tags, Narrators, and Series memberships. Identifiers additionally retain provenance for each entry.

**Explicit Clear**:
A user's deliberate removal of a metadata value or all entries in a relationship. Under the Identify rules, the resulting absence has no enduring provenance and may be repopulated by a later Scan.

**Merge**:
Combining two resources of the same type into one, transferring all associations from the source to the target and deleting the source.

**Resync**:
Re-running the scan pipeline for an existing book or file, in one of three modes: scan (pick up new metadata without overwriting manual edits), refresh (re-scan as if new, re-enriching via plugins), or reset (clear all metadata including manual edits and re-scan from file metadata only, without plugins).
_Avoid_: rescan (in new code; some existing identifiers, comments, and docs/UI copy still say "Rescan")

**Sidecar**:
A `.metadata.json` file placed alongside a book or file that provides metadata overrides.

### Sharing

**List**:
A user-owned virtual collection of **Books** that may span libraries, either ordered (manual sequence) or unordered (automatic sort). A list never changes the filesystem and never grants library access; a viewer sees only the books in libraries they can already access.
_Avoid_: collection, shelf, playlist

**Share Link**:
An anonymous link that grants whoever holds it access to one **Book** and its **Files** without a Shisho account or library access. A share link is active, expired, or revoked. Only an active link resolves, and only while sharing is on and the link is not paused. An active link is paused while its creator is deactivated or cannot reach the book's library, and resolves again when that access returns. It may expire at a set time, and a book may have many share links at once. It is distinct from a **List Share**, which is a grant to a named user and requires login.
_Avoid_: public link, guest link, share (alone, which is ambiguous with list share)

**List Share**:
A grant of a **List** to a named user at a permission level (viewer, editor, manager). It never grants library access and requires the recipient to log in.
_Avoid_: share (alone)

### Reading

**Reading Status**:
Where one user stands with a **Book**: To Read, Currently Reading, Paused, Read, Did Not Finish, or none. It belongs to the Book, not a **File**, and is private to the user. Currently Reading, Paused, Read, and Did Not Finish describe the user's latest **Read-through**.
_Avoid_: shelf, reading state

**Read-through**:
One pass a user makes through a **Book**, with the day it began if known and, once it ends, the day it ended and whether it was finished or abandoned. Rereading starts a new one; earlier ones stay as history. A Read-through still underway may be paused.
_Avoid_: read (as a noun), session, reading (alone)

**To Read**:
One user's ordered queue of **Books** they intend to read. A book joins at the end, and the user can reorder it so the next books sit at the top. Being on it is not a **Read-through**.
_Avoid_: TBR, want to read, wishlist, Up Next

**Rating**:
One user's score for a **Book**, from a quarter star to five stars in quarter-star steps. It does not depend on **Reading Status**, so an abandoned book can be rated. Private to the user.
_Avoid_: review, score, favorite

**Page flow**:
How the reader lays out a **Page-based** File's pages: paginated, one page at a time, or scrolled, every page in one continuous column for comics made for vertical scrolling. One user chooses it per **Book**; until they do, the reader picks it from the shape of the pages.
_Avoid_: scroll mode, webtoon mode, layout

**Reading Position**:
Where one user is within one **File**: a location in the text for a **Reflowable** File, a page for a **Page-based** one, a time for an audiobook, along with how far through the File that is. It belongs to the File, so a Book's ebook and audiobook each keep their own.
_Avoid_: progress (alone), bookmark, location

**Reading Session**:
One stretch of time a user spent reading or listening to a **File**, from when it started to when it ended, and how far through the File it began and finished. It belongs to the **Read-through** underway at the time.
_Avoid_: reading time, sitting, activity

### Deployment Modes

**Demo Mode**:
A server configuration that presents a prepared, immutable library for evaluation. Visitors sign in with a shared credential and can browse, search, read, and listen. Any request that would persist a change is rejected up front; nothing is accepted and later reset. Preferences kept only in the visitor's browser are allowed.
_Avoid_: read-only mode, sandbox, demo instance (that is the Public Demo)

**Public Demo**:
The publicly hosted Shisho instance at `demo.shishobooks.com`. It runs in Demo Mode, serves the Demo Corpus, and is signed into with one shared credential published alongside every link to it.

**Demo Corpus**:
The prepared set of legally redistributable Books, Files, covers, and the database that the Public Demo serves. It is authored deliberately, replaced only by redeployment, and never changed by visitors. Every work in it carries a recorded source, license basis, credit, and note of any modification.
_Avoid_: sample library, fixtures (those are test data)

## Relationships

- A **Library** contains many **Books**, **Series**, **Persons**, **Genres**, **Tags**, and **Publishers**
- A **Book** has one or more **Files**
- A **Book** has many **Genres**, **Tags**, **Authors** (persons), and **Series**
- A **Book** belongs to a given **Series** at most once, at a **series number** that is a single position or a contiguous range (an **omnibus**)
- A **File** has at most one **Publisher** and many **Narrators** (persons)
- A **Publisher** has at most one parent **Publisher** and many child **Publishers** (self-referential hierarchy)
- A user has many **Devices**; a **Device** belongs to exactly one user and may connect several ways
- A user has one set of default **Delivery Rules**, which follow Shisho's built-in rules until edited; a **Device** follows them, follows its Device type's preset, or has its own
- A **Book** has many **Share Links**; a **Share Link** belongs to exactly one **Book** and is created by one user
- A user has at most one **Reading Status** and one **Rating** per **Book**, many **Read-throughs** per Book (at most one underway), and one **To Read** queue
- A user has at most one **Reading Position** per **File**
- A **Read-through** has many **Reading Sessions**, each on one **File**
- A **Resource** (series, person, genre, tag, publisher) has many **Aliases**
- An **Alias** belongs to exactly one resource and is unique within its resource type and library

## Example dialogue

> **Dev:** "A plugin returns genre 'Non-fiction' but the library already has 'Nonfiction'. What happens?"
> **Domain expert:** "If 'Non-fiction' is an **alias** of the 'Nonfiction' **genre**, the **scan** resolves it to the existing genre using the **alias**. The **primary name** 'Nonfiction' is what appears in the UI. If there's no alias match, a new genre is created."

> **Dev:** "What happens when I **merge** 'Sci-Fi' into 'Science Fiction'?"
> **Domain expert:** "All books move to 'Science Fiction', 'Sci-Fi' becomes an **alias** of 'Science Fiction', and the old genre is deleted. Any aliases 'Sci-Fi' had also transfer."

## Flagged ambiguities

- Older UI copy and the "Supported Formats" docs page say "format" where the domain term is **File Type**.
- The database table for persons is `persons`, but the URL path uses `people` and the Go package is `pkg/people`. The domain term is **Person**.
- "To Read" names both the queue and the **Reading Status** a Book has while on it (when no Read-through is underway). Say "on To Read" for the queue membership.
- **Publisher** is file-level metadata, not book-level. This differs from series/genre/tag/author which are book-level.
