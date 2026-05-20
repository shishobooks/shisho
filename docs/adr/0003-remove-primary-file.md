# Remove the primary file concept

A book can have multiple files (editions, formats, multi-part audiobooks). Since v0.0.12, a persisted `primary_file_id` FK on the `books` table designated one file as the default for device sync, eReader downloads, OPDS metadata priority, gallery sorting, batch downloads, and identify-dialog defaults. The user could override the auto-selected primary via a "Set as primary" action. Write-side logic auto-assigned the primary on first file creation and auto-promoted a new primary when the current one was deleted or moved — non-trivial code that is also removed.

We're removing the concept because every consumer either doesn't need a persisted selection or is better served by a different mechanism:

| Consumer | Current behavior | Replacement |
|----------|-----------------|-------------|
| Kobo sync | Only syncs primary file | Sync all compatible files; `file.Name` disambiguates editions |
| eReader browser | Downloads primary file only | Show all files; user picks |
| OPDS feeds | Primary file's metadata takes priority | List each file as a separate acquisition; user selects |
| Gallery sorting | Sort by primary file's release date / page count / duration | Sort by newest file's value, fall back to any file with a non-null value |
| Batch download | Downloads primary file per book; skips books without a primary | File-type checkboxes let users pick which formats to include |
| Identify decisions | Book-level changed fields default ON only when identifying the primary file (or the sole main file) | Default based on source priority — ON when field source is filepath/file-metadata, OFF when already plugin/manual |
| `getPrimaryFileType` (series number formatting) | Uses primary file's type to decide "Vol." vs plain number | Any-CBZ check across all files |
| Identify dialog (initial file selection) | Prefers primary after unreviewed heuristic | Drop primary preference; unreviewed heuristic + first main file is sufficient |

The main argument for primary file was preventing Kobo duplicates, but the common multi-file case is cross-format (EPUB + M4B) where Kobo's format filter already deduplicates. Same-format editions are uncommon and have different covers and `file.Name` values — e.g., "Foobar (5th Edition)" — making them distinguishable on the device, the same as having bought both editions from the Kobo store.

## Considered Options

- **Keep primary file as-is** — Working, but adds a persisted FK, auto-promotion logic, a dedicated API endpoint, manual override UI, and a concept users must understand, all for marginal value.
- **Remove the persisted FK but keep computed selection** — Drop the column and UI, but have each consumer compute "which file to use" via a shared heuristic (e.g., newest main file). This is effectively what some replacements do (gallery sorting picks newest file's value), but formalizing a single computed primary would re-introduce the coupling we're trying to remove — different consumers genuinely want different things (Kobo wants all compatible files, batch download wants user-selected types, identify wants source-priority logic).
- **Remove primary file, replace per-consumer** — Each consumer gets a more appropriate mechanism (format filters, source priority, user selection). More flexibility, less hidden magic.
