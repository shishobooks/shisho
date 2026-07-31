# Libraries, Scanning, and File Organization

A library connects Shisho to one or more directories that contain your media. This page explains how paths, scans, organization, and file move detection work.

## Creating a Library

Go to **Settings > Libraries**, then select **Add Library**. Choose a name, cover aspect ratio, download preference, and one or more paths.

Paths must be visible from inside the Shisho container. A host path such as `/mnt/books` is not usable unless it is mounted into the container, and the library path must use the container side of that mount. Shisho also needs suitable read and write permissions if file organization is enabled.

Saving a library queues a global scan of all libraries, unless a scan is already pending or running. The new library may therefore take a little time to appear populated. If you have `jobs:read`, follow progress under **Settings > Jobs**.

### Cover and Download Preferences

**Cover Display Aspect Ratio** controls gallery covers and which cover Shisho prefers for books with both ebook and audiobook editions:

- **Book Cover (2:3)** prefers an ebook cover and always displays it in a 2:3 frame. If only an audiobook cover is available, it uses that cover in the same 2:3 frame.
- **Audiobook Cover (1:1)** prefers an audiobook cover and always displays it in a square frame. If only an ebook cover is available, it uses that cover in the same square frame.
- The two fallback modes use the preferred cover type when available, but adopt the available cover's native frame shape when falling back to the other type.

**Download Format Preference** controls EPUB and CBZ downloads:

- **Original format** downloads the original type.
- **KePub (Kobo-optimized)** generates a KePub.
- **Ask on download** lets the user choose each time.

These settings do not affect M4B or PDF downloads. See [Reading and Playback](./reading-and-playback.md) for the in-app readers.

## Directory Grouping

Shisho works best when each book has its own directory. Supported main files in that directory, such as an EPUB and an M4B edition, can be grouped as one book. Files discovered beside the main files may be tracked as [supplements](./supplement-files.md).

A simple unmanaged layout is:

```text
/library/
├── Project Hail Mary/
│   ├── Project Hail Mary.epub
│   └── Project Hail Mary.m4b
└── The Martian/
    └── The Martian.epub
```

Keep unrelated books in separate directories when you manage the layout yourself.

## File Organization

The per-library **Organize file structure during scans** setting is backed by `organize_file_structure` and defaults to `true` for new libraries. When enabled, Shisho can move and rename media into its standardized author and title layout during scans and after metadata changes, identification, file moves, or book merges. Associated covers and file sidecars move with their media files.

:::danger[This Setting Changes Files on Disk]
File organization is not only a display preference. Shisho can rename files, create directories, and move content. Confirm the library paths and container permissions, and keep a backup before enabling it for an existing collection.
:::

Turn organization off if another application or your own process controls the directory layout. Shisho can still group and manage records, but actions such as moving a file between existing books change its grouping in Shisho rather than relocating it on disk.

See [Managing Books and Files](./managing-books-and-files.md) before deleting, moving, or merging content.

## Automatic and Manual Scans

Shisho discovers and reconciles content in three ways:

- **Scheduled scans:** `sync_interval_minutes` controls the global schedule and defaults to 60 minutes. Set it to `0` to disable scheduled scans.
- **Filesystem monitor:** `library_monitor_enabled` is enabled by default. It watches library paths and performs targeted rescans after changes settle. The default `library_monitor_delay_seconds` of 60 seconds acts as a debounce, so each new event restarts the wait. Some network filesystems do not provide reliable filesystem events, so scheduled or manual scans remain important.
- **Manual scans:** A user with `jobs:write` can open **Settings > Jobs** and select **Trigger Scan** for an immediate full reconciliation. The scan runs as a background job; `jobs:read` is required to monitor its progress and errors.

See [Configuration](./configuration.md) for the server settings behind the schedule and monitor.

## Scan Outcomes

A scan walks every configured library path and updates Shisho to reflect supported files that were added, changed, moved, or removed. It extracts available metadata and covers, applies the configured metadata priority, discovers supplements, and may organize files when organization is enabled.

A normal scan preserves higher-priority values, including manual edits. Use the refresh and reset choices only when you intentionally want to replace metadata. See [Metadata](./metadata.md#normal-scans-refresh-and-reset).

Review the job log when a scan finishes with warnings. Common causes include unreadable paths, unsupported or damaged files, plugin failures, and files changing while the scan is running.

## File Identity and Move Detection

Shisho uses a SHA-256 content fingerprint to recognize a tracked file after a rename or move. When a match is found, Shisho keeps the existing file and book identity, including your edits, instead of importing a duplicate.

Move detection requires all of the following:

- The existing file already has a SHA-256 fingerprint.
- The old stored path no longer exists.
- The new file is in the same library and has the same supported file type.
- The file contents are byte-for-byte unchanged.

If Shisho was offline during a move, the next full scan performs the same reconciliation for main files. Offline reconciliation does not match supplement moves. A move that also rewrites or retags the file changes its fingerprint, so Shisho treats it as a removed file and a new import.

Fingerprint generation runs as a background hash-generation job after discovery. Before renaming or moving many files outside Shisho, wait for that job to finish under **Settings > Jobs**. This is especially important after a first scan or an upgrade that introduces hashes for existing files.

## Deleting a Library

:::danger[Library Deletion Cannot Be Undone]
Deleting a library permanently removes its metadata, settings, access grants, and other Shisho records. Its media, covers, and sidecars remain on disk, but recovering the removed Shisho state requires restoring a database backup. Back up `/config` and verify the selected library before continuing.
:::

Wait for active global scans to finish before deleting a library. At the bottom of the library settings page, **Delete library** removes the library and its Shisho-managed records. Pending and in-progress jobs assigned specifically to that library are marked failed, but already executing work may not stop immediately. A global scan is not assigned to one library and may still be running.

Deleting a library does **not** delete its media, covers, or sidecars from disk. This is different from deleting a book or file, which is destructive. Type the library name to confirm only after checking that you selected the intended library.

See [Users and Permissions](./users-and-permissions.md) for library management permissions.
