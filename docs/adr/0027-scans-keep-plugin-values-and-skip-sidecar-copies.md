# Scans keep plugin values, skip Shisho's own sidecar copies, and leave cleared fields empty

Status: accepted

## Context and decision

The library-wide Scan for new metadata exists so a metadata plugin installed after an import reaches the Books already there. Three rules of an ordinary Scan stopped it from doing that, or made the result unsafe:

- **Sidecars.** Shisho rewrites a Book's and each File's Sidecar after every Scan and every edit, so a Sidecar normally just copies the database. Sidecars outrank plugins, so the copy written at import put the embedded value back over the plugin's value and relabeled the field `sidecar`, which then blocked every later plugin.
- **Plugin ties.** Two plugin values tie on priority and the newer wins. A Scan whose automatic search matched a different book than the editor chose in Identify would replace the Identify fix. Only the Sidecar copy hid this.
- **Explicit Clears.** A clear removed the value and its source (ADR 0006), so the next Scan's automatic search could put back a value the editor removed from a bad match.

An ordinary Scan now settles a field like this:

1. **A Sidecar field equal to the value stored before the Scan is ignored.** It is Shisho's own copy, not metadata from elsewhere. A Sidecar field that differs, because it came from another library or was edited on disk, keeps its priority over plugins as before. The Book's Sidecar is compared whenever the Book was already in the library, including when a new File joins it, and a File's Sidecar whenever the File was. A new Book's Sidecars always apply, since nothing is stored to compare with. A Book's Files rescan one at a time so each compares against what the one before it stored.
2. **A plugin value already stored is never replaced by another plugin value.** This holds for the Book's fields when the Book was already in the library, and for the File's fields and its Cover when the File was, whether or not the file changed on disk. The Scan still fills empty fields and replaces values from the file or the filepath, and a plugin file parser's values count as the file's own metadata, so an enricher can replace them. A Cover the user or a plugin chose is kept; one from the file can still be upgraded to a larger enricher Cover. A tie between two file-derived values still goes to the newer one. The library monitor refreshes a file edited while Shisho runs, so its new embedded metadata still arrives.
3. **An Explicit Clear leaves a protected empty slot.** The value is removed and the source becomes `manual`, the protected empty slot the Edit form stores for a cleared field. The Edit form now does this for every field, including Name, Language, and Abridged, which it used to leave unprotected. Clearing a field that is already empty changes nothing unless it holds a leftover plugin source, which is stamped `manual`; the Identify form sends empty rows it selects in bulk, and protecting those would block plugins from fields the editor never touched. This amends ADR 0006, which rejected tombstones for clears so a fresh Scan could refill the field.

The priority ranking does not change. Refresh all metadata and Reset to file metadata bypass all three rules, as they bypass source priority and Sidecars today. Identify writes the selected values whatever their sources, so picking a better match later still replaces an earlier one.

## Considered options

- **Leave the rules and document that the library-wide scan only fills empty fields.** Rejected: it fails the reason the scan exists, and the `sidecar` relabel quietly locks fields against every future plugin.
- **Ignore every Sidecar during the library-wide scan.** Rejected: metadata carried over from another library or edited on disk would be overwritten, which is what Sidecars are for.
- **Let a newer plugin value win only when it comes from the same plugin.** Rejected: the common bad case is the same plugin matching a different book, so this would not protect an Identify fix.
- **Let a File that changed on disk take newer plugin values.** Rejected: in a Book with several Files, retagging one would undo an Identify fix to the whole Book, and the library monitor already refreshes files edited while Shisho runs.
- **Keep refilling cleared fields, and tell editors to clear through the Edit form.** Rejected: clearing a value a bad match set is a routine part of fixing it in Identify, and the refill is exactly the bad value.
- **Stamp `manual` on every value Identify applies.** Rejected: ADR 0006 records accepted values as plugin-sourced on purpose, and rule 2 already protects them from ordinary Scans.

## Consequences

- A metadata plugin installed late replaces values that came from files, across the library, when the library-wide Scan for new metadata runs. A plugin that picks wrong editions does more damage than before; the confidence threshold and manual edits limit it, and Identify fixes a Book.
- Plugin data that changes at its source, or a plugin update, no longer reaches existing Books through an ordinary Scan, and neither does an enricher's result for a File replaced on disk while Shisho was offline. Refresh all metadata or Identify picks it up.
- A manual Cover is no longer replaced by a larger enricher Cover during a Scan.
- A field the editor clears in Identify stays empty until Refresh, Reset, Identify, or the Edit form sets it.
- Fields already relabeled `sidecar` by the old rule stay protected, since they can't be told apart from carried-over metadata. Refresh all metadata repairs a Book.
- An identifier collection accepted in Identify stays as applied. An ordinary Scan no longer adds back an embedded identifier the proposal left out.
- Sidecars still carry no source fields. The comparison in rule 1 is against the stored value, so a field whose Sidecar write failed reads as edited on disk, as it did before. Renaming a Person, Series, Genre, Tag, or Publisher, or merging one into another, marks the Books that name it (for a merge, only the source's Books) with `books.sidecars_stale_at` and queues a background job that rewrites their Sidecars, since doing it in the request is too slow for a name on thousands of Books. A Scan that reaches a marked Book first rewrites its Sidecars from what is stored, or skips them when they can't be written, so the Sidecar doesn't bring the old name back. Before, it either relabeled the field `sidecar` or, when the old name wasn't kept as an alias, re-created the old resource and undid the rename for that Book.
