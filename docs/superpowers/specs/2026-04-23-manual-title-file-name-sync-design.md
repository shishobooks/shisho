# Manual Book Title Edit: Sync File Name When It Matches Old Title

## Problem

When a user edits a book's title via the manual edit form (as opposed to the
identify flow), `book.Title` is updated but `file.Name` is left alone. If the
user hadn't customized the filename, `file.Name` typically still equals the
old title — so the book and its file diverge silently. On the next file
organization pass the file keeps its stale name (file organization prefers
`file.Name` over `book.Title`), downloads use the stale name, and the user
sees two different labels for the same thing.

The recent identify work (#126) fixed the equivalent divergence for the
identify flow by *unconditionally* mirroring the identified title onto the
target file's `Name`. Manual edits still have the old behavior.

## Goals

- When `book.Title` changes via the manual book-update handler, update
  `file.Name` on each main file whose current `Name` is either unset or
  already matches the old title.
- Leave `file.Name` alone when the user has deliberately customized it to
  something different from the book title.

## Non-goals

- Changing the identify flow. Identify is a per-file user action where the
  user is explicitly saying "this file is this book, apply these values";
  it continues to mirror the title onto the target file's `Name`
  unconditionally.
- Changing supplement file naming. Supplements have their own
  filename-derived `Name` and are never touched.
- Changing how file reorganization is triggered. The existing handler
  already sets `shouldOrganizeFiles = true` on a title change; reorg picks
  up the new `file.Name` values for free.

## Design

Only the manual book-update handler changes:
`pkg/books/handlers.go` around lines 238–250 (the title-update block
inside the `update` handler).

### The rule

When `params.Title` is provided and `*params.Title != book.Title`:

1. Capture `oldTitle := book.Title`.
2. Apply the existing title update (`book.Title = *params.Title`,
   `book.TitleSource = DataSourceManual`, sort title, columns list — all
   unchanged).
3. For each `file` in `book.Files`:
   - Skip unless `file.FileRole == models.FileRoleMain`.
   - Compute `matches` as:
     ```
     file.Name == nil || *file.Name == "" ||
       strings.EqualFold(strings.TrimSpace(*file.Name), strings.TrimSpace(oldTitle))
     ```
   - If `matches`:
     - Set `file.Name = &newTitle` (a copy of `*params.Title`).
     - Set `file.NameSource = strPtr(models.DataSourceManual)`.
     - Persist via `h.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{Columns: []string{"name", "name_source"}})`
       (already defined at `pkg/books/service.go:686`).

Strict equality **after** `TrimSpace` and `ToLower` (via `EqualFold`). A
user who meaningfully changed case or added meaningful characters keeps
their value; trailing spaces and capitalization differences don't block
the sync.

### Source priority interaction

`DataSourceManual` is the highest-priority source (0), but scans and
enrichment are the only flows that check priority. Identify (`persistMetadata`)
and manual edits are user-initiated and overwrite unconditionally, so
writing `DataSourceManual` here does not prevent a subsequent identify
from replacing the value.

### Ordering

Run the file-name sweep *before* the existing `UpdateBook` call so a
failure to update a file row doesn't leave the book title persisted while
the files are still out of sync. If a per-file `UpdateFile` call fails,
log a warning and continue to the next file — matches the rest of this
handler's style (see the author/series/genre blocks that log and
continue). Files are independent; one bad row shouldn't block the rest,
and we don't want to block the user's primary edit on a best-effort side
effect.

## Files changed

- `pkg/books/handlers.go` — add the file-name sweep inside the title
  update block.
- `pkg/books/handlers_test.go` (or whichever test file currently covers
  the update handler) — new tests.
- `website/docs/directory-structure.md` — extend the existing Organize
  Files note about identify syncing file.Name to cover manual edits too.

## Test plan

Follow Red-Green-Refactor per the project's testing rules. Write each
test first, watch it fail, then implement.

New tests on the update handler (or service layer if that's where the
existing title-update coverage lives):

- `TestUpdateBook_Title_UpdatesMainFileName_WhenMatchesOldTitle` —
  book.Title = "Foo", file.Name = "Foo". Update title to "Bar". Assert
  file.Name == "Bar", file.NameSource == DataSourceManual.
- `TestUpdateBook_Title_UpdatesNilFileName_ToNewTitle` — book.Title =
  "Foo", file.Name = nil. Update title to "Bar". Assert file.Name ==
  "Bar".
- `TestUpdateBook_Title_UpdatesEmptyFileName_ToNewTitle` — same as above
  but with `file.Name` pointing at `""`.
- `TestUpdateBook_Title_MatchesWithTrimAndCasefold` — book.Title = "Foo
  Bar", file.Name = "  foo bar  ". Update title to "Baz". Assert
  file.Name == "Baz".
- `TestUpdateBook_Title_PreservesCustomFileName_WhenDiffers` —
  book.Title = "Foo", file.Name = "Baz" (deliberately different). Update
  title to "Bar". Assert file.Name is still "Baz".
- `TestUpdateBook_Title_DoesNotTouchSupplementFileName` — book has a
  main file matching the old title and a supplement also matching the
  old title. Update title. Assert the main file's Name updates, the
  supplement's Name does not.
- `TestUpdateBook_Title_Unchanged_DoesNotTouchFileName` — supply
  params.Title equal to the current book.Title. Assert no file updates
  happen (the outer `if` already guards this; this test documents it).
- `TestUpdateBook_Title_MultipleMainFiles_IndependentlyChecked` — book
  with two main files, one matching the old title and one custom.
  Update title. Assert only the matching one changes.

## Docs

`website/docs/directory-structure.md` has a note about identify syncing
file.Name from #126 (in the "Organize Files" section). Extend that note
to say manual book-title edits now do the same thing, with the
conditional rule ("unchanged from the old title, or not set") so users
understand why a custom filename survives the edit. No config fields
change; `shisho.example.yaml` unaffected.
