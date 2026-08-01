# Metadata

Shisho combines metadata from file paths, embedded file data, plugins, sidecars, and your edits. Books hold shared descriptive metadata, while files hold edition-specific details.

## Books and Files

A **book** groups one or more main files. Its shared fields include title, sort title, subtitle, description, authors, series, genres, and tags.

A **file** represents a particular EPUB, CBZ, M4B, or PDF edition. File fields include display name, narrators, publisher, release date, language, URL, identifiers, chapters, and abridged status. [Supplements](./supplement-files.md) belong to a book but are excluded from metadata review and most main-file behavior.

### Preferred Covers

When a book has multiple main files in the same cover category, edit a file and select **Preferred ebook cover** or **Preferred audiobook cover**. Ebook preference covers EPUB, CBZ, and PDF files; audiobook preference covers M4B files. The chosen file must have a cover. Choosing one clears the same preference from other files in that category.

## People and Roles

A person can appear as an author or narrator. CBZ creators can also have roles such as writer, penciller, inker, colorist, letterer, cover artist, editor, or translator. Renaming the person updates every place that uses that shared record.

People and series support sort names. Shisho generates them automatically, but a manual sort name stays in effect until you clear it.

## Series and Ranges

A book can belong to multiple series. Series positions may be integers, decimals such as `1.5`, or contiguous omnibus ranges such as `1-3`. Some export targets only support one numeric position, so they use the range start.

For CBZ books, a series position may also be marked as a volume or chapter. Edit the advanced settings on a series row to manage range and unit values.

## Genres, Tags, and Publishers

Genres and tags categorize books. Genres often originate in file metadata, while tags are commonly curated by users.

Publishers belong to files, so different editions can have different publishers. Publishers can be arranged in a manually curated parent and child hierarchy for imprints and related organizations.

## Identifiers

Identifiers belong to files. A file can have one value for each identifier type, including ISBN-10, ISBN-13, ASIN, UUID, Goodreads, Google, and types added by plugins. Shisho normalizes common formatting, such as ISBN hyphens and ASIN letter case, for reliable matching.

## Aliases and Resource Merges

People, series, genres, tags, and publishers can have aliases. Name lookups use aliases to resolve variants to one canonical resource. Renaming a resource can preserve its old name as an alias.

Merging these resources moves their relationships to the target, adds the source name and aliases to the target, and removes the source resource. This is different from [merging books](./managing-books-and-files.md#merging-books), which keeps the target book metadata and moves source files without combining source book metadata.

## Editing and Identify

:::warning[Identify Can Move Files]
When **Organize file structure during scans** is enabled, applying path-affecting fields can rename or move media files. These path changes are not automatically reversible. Back up the media and verify the selected library before applying changes, or turn off organization under **Settings > Libraries** first if you need to preserve the current layout.
:::

Use **Edit** on a book or file for direct changes. Use **Identify** to search configured metadata plugins, compare proposed book and file values, and choose which fields to apply. Review every checked field when identifying a second edition because shared book metadata may differ between editions.

When [file organization](./libraries.md#file-organization) is enabled for the library, Identify reorganizes files after applying path-affecting changes. This includes an explicitly selected file **Name** and removal of the final series membership, which removes obsolete series-number suffixes from organized CBZ and hybrid book folders.

When you apply a plugin-proposed file Name unchanged, it remains plugin-sourced. If you edit the proposed Name before applying it, Shisho treats the result as a manual edit, so normal scans protect it from plugin, embedded, and filepath metadata.

Each Identify field has its own apply checkbox. An unchecked field is left untouched.

:::warning[Clearing Metadata]
Clearing a checked field removes that value or relationship from the book or file and rewrites its sidecar metadata. It does not delete the media file or shared people, genre, tag, series, or publisher records. Before applying, verify every checked row and back up the database and sidecars if the current metadata matters. You can restore a value by editing it, identifying again, or scanning it from an available source, but a custom value may not be recoverable. Leave a field unchecked to preserve it.
:::

If you check a field and remove its value, applying the result clears that metadata, including lists such as authors, genres, narrators, and identifiers. Title is required and cannot be cleared. Cover selection only replaces a cover, and narrators are available only for M4B files.

## Metadata Source Priority

During a normal scan, Shisho resolves conflicting values in this order:

1. Manual edits
2. [Sidecar Files](./sidecar-files.md)
3. Plugin metadata
4. Embedded file metadata
5. Filepath-derived values

Manual values are protected during normal scans. They are not permanently immutable: **Refresh all metadata** and **Reset to file metadata** can overwrite or remove them.

## Normal Scans, Refresh, and Reset

The rescan choices have intentionally different effects:

- **Scan for new metadata** respects source priority and protects manual values from lower-priority sources.
- **Refresh all metadata** bypasses source priority, can overwrite manual values, rebuilds metadata from current sources, and runs plugins again.
- **Reset to file metadata** clears existing values, including manual values, then rebuilds metadata from the file without plugin enrichment. Missing title and author data can fall back to the filepath.

Use refresh or reset only after reviewing what will be replaced. Keep sidecars and a backup if the current metadata matters.

## Reviewing Metadata

Review state is tracked per main file. A book is **Reviewed** only when all of its main files are reviewed; if any main file needs review, the book appears under **Needs review**. Supplements do not affect the result.

Automatic review checks the fields configured under **Settings > Review Criteria**. Defaults require authors, description, cover, and genres for every main file, plus narrators for M4B files. Adding a missing required value can mark the file reviewed, and removing one can return it to needs review.

A manual **Reviewed** or **Needs review** choice overrides automatic calculation in either direction. The override is sticky until it is cleared, even if metadata or review criteria later change. Admins can change the required fields and recompute review state from **Review Criteria**.

Use the gallery's **Review state** filter to work through the queue. Bulk selection offers **Mark reviewed** and **Mark needs review**.

## Fetch Chapters from Audible

For an M4B file with an Audible ASIN, a user with `books:write` can open chapter editing and choose **Fetch from Audible**. Shisho sends the ASIN to [Audnexus](https://audnex.us), an external service that provides Audible chapter data.

The dialog compares the Audible runtime and chapter count with the local file. It can account for a removed Audible intro, but you should verify the detected offset. A substantial duration difference often indicates a different edition, and imported timestamps may not align.

Choose one of these staged changes:

- **Apply titles only** keeps local timestamps and is available only when chapter counts match.
- **Apply titles + timestamps** replaces both using the fetched data and selected intro-offset setting.

Fetched chapters remain staged until you click **Save** in the chapter editor. Spot-check playback before saving, or cancel to discard the staged changes.

Audnexus can be unavailable, rate-limited, or time out. After correcting the ASIN or waiting for the service, use **Retry**. Fetching sends the ASIN outside your Shisho server.
