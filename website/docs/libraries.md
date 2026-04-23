---
sidebar_position: 15
---

# Libraries

Shisho organizes your collection into **libraries**, each pointing at one or more directories on disk. This page covers how to create, configure, and delete a library.

## Creating a Library

Go to **Admin → Libraries → Add Library**. Choose a name, pick a cover aspect ratio (book, audiobook, or a fallback mode), and add one or more directory paths. Saving the library kicks off a scan of the configured paths automatically.

## Library Settings

Each library has its own settings page reachable from **Admin → Libraries → Settings** on the corresponding row. Settings cover:

- **Library name** and **paths** — rename or add/remove scanned directories.
- **Cover display aspect ratio** — how book and series covers render in gallery views.
- **Download format preference** — original / KePub / Ask-on-download for EPUB and CBZ files.
- **Organize file structure during scans** — when enabled, Shisho moves and renames files into a standardized layout. See [Directory Structure](./directory-structure.md) for the naming rules and triggering events.
- **Plugin order** — override the global plugin order for this library.

## Deleting a Library

At the bottom of the library settings page, users with `libraries:write` permission (Admin and Editor roles by default) see a **Danger Zone** section with a **Delete library** button.

Click the button, type the library name to confirm, and click **Delete**. The confirmation dialog surfaces these caveats:

- **The action is irreversible.** There is no undo.
- **Files on disk are not deleted.** Book files, audiobooks, comics, and PDFs remain exactly where they are.
- **Sidecar and metadata files are not cleaned up.** `.shisho.json` sidecars, `.cover.jpg` images, and other generated metadata remain on disk. Remove them manually if you want a truly clean slate.

### What is deleted

- The library row itself.
- All books, files, series, persons (authors and narrators), genres, tags, publishers, and imprints scoped to the library.
- All file identifiers and chapters for those files.
- Per-library plugin configuration, hook configuration, and field settings.
- Per-user library access grants and per-user library settings (sort spec, etc.).
- Full-text search entries for the above so searches no longer surface stale results.

### What happens to active jobs

Any pending or in-progress scan, hash, or plugin job scoped to the deleted library is cancelled (marked `failed`) as part of the deletion. Global jobs and jobs targeting other libraries are untouched.

See [Users and Permissions](./users-and-permissions.md) for how to grant or revoke `libraries:write`.
