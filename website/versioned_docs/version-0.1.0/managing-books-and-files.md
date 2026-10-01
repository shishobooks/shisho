# Managing Books and Files

Books group one or more main files and any supplements. Before deleting, moving, or merging them, understand whether Shisho will change records only or also change the filesystem.

:::danger[Keep a Backup]
Book and file deletion removes content from disk. File moves and merges can also relocate content when file organization is enabled. Test your workflow on copied media before making large changes.
:::

## Deleting Libraries, Books, and Files

These actions have different consequences:

| Action | Shisho Records | Files on Disk |
|---|---|---|
| Delete a library | Removes the library and its records; associated jobs are marked failed but executing work may not stop immediately | Preserves media, covers, and sidecars |
| Delete a book | Removes the book and its files | Deletes disk content |
| Delete a file | Removes that file and updates or removes its book as needed | Deletes the media file, its cover, and its file sidecar |

### Deleting a Book

Deleting a book is destructive. When **Organize file structure during scans** is enabled and the book has an organized directory, Shisho removes the entire directory. This can delete untracked files that happen to be inside it, not only content visible in Shisho.

When organization is disabled, Shisho deletes each tracked file separately, along with its associated cover and file sidecar. Do not use book deletion as a way to remove only Shisho's record.

### Deleting a File

Deleting a file removes the media file, its generated cover, and its file sidecar from disk. If other main files remain, the book remains.

If you delete the last main file, Shisho promotes the oldest supported supplement, such as an EPUB, CBZ, M4B, or PDF, to become the new main file when possible. If no supplement can be promoted, Shisho deletes the remaining supplement files and the book record. Review the whole book before confirming deletion of its last main file.

## Main Files and Supplements

A main file represents an edition of the book. A supplement is related content that does not normally supply the book's metadata or review state. See [Supplement Files](./supplement-files.md) for discovery rules.

You can promote a supported supplement to a main file or demote a main file to a supplement from file editing. Promotion causes Shisho to treat the file as an edition and scan its metadata. Demotion removes it from main-file behavior. Make sure another appropriate main file remains, especially before later deletions or merges.

## Moving Files Between Books

Use **Move to...** on a file to move it to another book. Both books must be in the same library.

- With file organization enabled, Shisho physically moves the media file, its cover, and its file sidecar into the target book's directory.
- With file organization disabled, Shisho changes the file's book grouping but leaves the disk path unchanged.

A source book may be removed when no files remain. Review the source and target after the operation.

## Merging Books

Merge is also limited to books in the same library. Choose the target carefully:

- The target book keeps its metadata.
- Source book metadata is not combined into the target.
- Files from source books move to the target.
- Source books may be removed after their files move.
- With file organization enabled, the files, covers, and file sidecars move physically into the target directory.
- With file organization disabled, only Shisho's grouping changes.

If source books contain metadata you need, copy it to the target before merging. Check for filename conflicts, untracked directory contents, and a current backup before confirming.

## Safer Workflow

1. Back up the library directory and sidecars.
2. Wait for active scan and hash-generation jobs to finish.
3. Confirm that every item belongs to the same intended library.
4. For merges, edit the target so it already contains the metadata you want to keep.
5. Perform a small operation first and verify both Shisho and the filesystem.
6. Run a scan after large external filesystem changes.

For organization and move-detection behavior, see [Libraries, Scanning, and File Organization](./libraries.md).
