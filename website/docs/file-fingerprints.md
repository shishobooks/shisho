---
sidebar_position: 6
---

# File Fingerprints & Move Detection

Shisho stores a sha256 content hash for every file in your library. This lets
the library monitor detect when you rename or move files on disk, so your
books keep their identity (and any metadata you've edited) instead of being
deleted and re-imported.

## How it works

Every file that Shisho tracks has an entry in the `file_fingerprints` table
with a sha256 of its contents. When Shisho sees a new file appear on disk, it
checks whether any existing file has the same sha256 whose stored path no
longer exists on disk. If so, the existing file is updated to point at the
new path — no duplicate book row is created, and your custom metadata stays
intact.

## When move detection runs

**In real time (library monitor)** — When you rename a folder in Finder or
Explorer, Shisho's filesystem watcher fires within a few seconds. If the batch
of events contains any deletions, Shisho computes sha256 synchronously for the
new files and looks for matches. Typical folder renames are detected
immediately.

**On the next scan (safety net)** — If you rename files while Shisho is not
running, the monitor won't see the events. The next scan reconciles the
library's state with disk — any file rows whose paths are missing get matched
against newly-discovered files by size and sha256 before being deleted.

## When does move detection *not* work?

- **On the very first scan after upgrading.** Fingerprints are populated by a
  background job that runs after each scan. Until that job has processed your
  library at least once, there are no fingerprints to match against. After
  the first hash-generation job completes, move detection is fully enabled.
- **If you rename a file AND change its contents at the same time.** The new
  file has a different sha256, so it doesn't match — Shisho treats it as a
  fresh import and deletes the old row.
- **Across libraries.** Move detection only works within a single library.
  Moving a file from one library to another is treated as a delete + create.

## Background hash generation

Shisho computes sha256 hashes asynchronously in a background job so large
audiobooks don't block the scan. You can see it running in the Jobs view with
progress like "Hashing files (142/500)". The job is queued automatically:

- At the end of every scan job, for any files in that library still missing
  a sha256 hash
- At the end of every monitor batch that created new files

The job is idempotent — running it multiple times does not produce duplicate
fingerprints, and a pending job is never created if one is already pending or
running for that library.

## Future fingerprint types

The `file_fingerprints` table is designed to hold more than just sha256.
Future Shisho releases will add perceptual/fuzzy fingerprints for detecting
duplicates that aren't byte-identical:

- **Cover pHash** — "same cover, different file" detection across formats
- **Text SimHash** — "same book, different edition" for EPUBs
- **Chromaprint** — acoustic fingerprints for audiobooks encoded differently
- **CBZ page pHash** — comic book rescans and re-encodes
- **TLSH** — fuzzy hash fallback for arbitrary formats

These will all share the same table and generation infrastructure as the
exact-match sha256 hashes.
