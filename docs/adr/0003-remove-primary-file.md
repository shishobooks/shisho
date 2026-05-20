# Remove the primary file concept

A book can have multiple files (editions, formats, multi-part audiobooks). Since v0.0.30, a persisted `primary_file_id` FK on the `books` table designated one file as the default for device sync, eReader downloads, OPDS metadata priority, gallery sorting, batch downloads, and identify-dialog defaults. The user could override the auto-selected primary via a "Set as primary" action.

We're removing the concept because every consumer either doesn't need a persisted selection or is better served by a different mechanism:

| Consumer | Current behavior | Replacement |
|----------|-----------------|-------------|
| Kobo sync | Only syncs primary file | Sync all compatible files; `file.Name` disambiguates editions |
| eReader browser | Downloads primary file only | Show all files; user picks |
| OPDS feeds | Primary file's metadata takes priority | List each file as a separate acquisition; user selects |
| Gallery sorting | Sort by primary file's release date / page count / duration | Sort by newest file's value, fall back to any file with a non-null value |
| Batch download | Downloads primary file per book | File-type checkboxes let users pick which formats to include |
| Identify decisions | Book-level changed fields default ON only for primary file | Default based on source priority — ON when field source is filepath/file-metadata, OFF when already plugin/manual |
| `getPrimaryFileType` (series number formatting) | Uses primary file's type to decide "Vol." vs plain number | Any-CBZ check across all files |
| Identify dialog (initial file selection) | Prefers primary after unreviewed heuristic | Drop primary preference; unreviewed heuristic + first main file is sufficient |

The main argument for primary file was preventing Kobo duplicates, but the common multi-file case is cross-format (EPUB + M4B) where Kobo's format filter already deduplicates. Same-format editions (~1-5% of libraries) have different covers and `file.Name` values (e.g., "Foobar (5th Edition)"), making them distinguishable on the device — the same as having bought both editions from the Kobo store.

## Considered Options

- **Keep primary file as-is** — Working, but adds a persisted FK, auto-promotion logic, a dedicated API endpoint, manual override UI, and a concept users must understand, all for marginal value.
- **Remove primary file, replace per-consumer** — Each consumer gets a more appropriate mechanism (format filters, source priority, user selection). More flexibility, less hidden magic.
