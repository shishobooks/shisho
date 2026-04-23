# Delete Library — Design

## Problem

There is currently no way to delete a library from Shisho. The data model has an unused `Library.DeletedAt` soft-delete column and a toggle in the update handler, but no UI exposes it and nothing else in the system acts on the flag. Users who create libraries by mistake, rename folders, or decommission libraries have no recourse short of editing the SQLite database.

## Goals

- Allow a user with `libraries:write` to delete a library from the admin libraries area.
- The deletion is a hard delete from the database: the library row, its books, files, series, persons, genres, tags, publishers, imprints, chapters, file identifiers, plugin configs, plugin field settings, plugin hook configs, user library access entries, and user library settings are all removed.
- Full-text search indexes (`books_fts`, `series_fts`, `persons_fts`, `genres_fts`, `tags_fts`) are purged of entries belonging to the deleted library.
- Active scan / hash / plugin jobs targeting the library are cancelled (marked failed) so they stop running.
- The filesystem monitor drops its watches on the removed library paths.
- **No files on disk are deleted.** No sidecars, no cover files, no source files.
- The action is gated behind a confirmation dialog that clearly communicates that the deletion is irreversible, that files on disk are not deleted, and that sidecar / metadata files are not cleaned up.
- The unused `deleted_at` soft-delete plumbing is removed as part of this change.

## Non-Goals

- Deleting files, sidecars, covers, or any other content on disk. If the user wants the directory gone, they can remove it themselves; a future feature can layer this on top.
- Restoring a deleted library. This is a hard delete.
- Bulk delete across multiple libraries.
- Exposing delete to roles other than those that already hold `libraries:write` (Admin and Editor, per the default role definitions).

## Architecture

### Endpoint

`DELETE /api/libraries/:id`

Middleware:
- `Authenticate` (inherited from the libraries group)
- `RequirePermission(libraries, write)`
- `RequireLibraryAccess("id")` — prevents Editors without library access from deleting that library

Response: `204 No Content` on success. Standard error codes on 403 / 404.

### Handler

Registered in `pkg/libraries/routes.go`:

```go
g.DELETE("/:id", h.delete,
    authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite),
    authMiddleware.RequireLibraryAccess("id"))
```

The handler (`h.delete` in `pkg/libraries/handlers.go`) looks up the library, calls `libraryService.DeleteLibrary(ctx, id)`, and — on success — invokes `h.onLibraryChanged` so the monitor refreshes its watch set.

### Service

`DeleteLibrary(ctx, id)` in `pkg/libraries/service.go` runs the following inside a single `RunInTx`, in order:

1. **Collect IDs for FTS cleanup.** Before the cascade removes rows, select the IDs of all books, series, persons, genres, and tags that belong to this library. Hold them in memory for step 3.
2. **Cancel in-flight jobs.** Update `jobs` rows with this `library_id` whose status is `pending` or `running`, setting status to `failed` and an error message like "Library deleted". (Jobs with `library_id = NULL` or non-terminal for other reasons are untouched.)
3. **Purge FTS rows.** `DELETE FROM books_fts WHERE rowid IN (...)` for each of the five FTS tables, using the IDs collected in step 1. This must happen before step 4 because the FTS tables are external content tables keyed by the primary IDs — once the primary rows are gone via CASCADE, we can't reliably correlate.
4. **Delete the library row.** `DELETE FROM libraries WHERE id = ?`. SQLite's `ON DELETE CASCADE` propagates through the FK graph. Requires `PRAGMA foreign_keys = ON`, which is already set in production and test helpers.

After the transaction commits, the handler calls `onLibraryChanged` (non-transactional — it refreshes fsnotify watches in the monitor).

**Rationale for ordering:**
- FTS purge before CASCADE: external-content FTS tables need the primary IDs to be resolvable at delete time.
- Job cancellation before library delete: the `jobs.library_id` FK uses `ON DELETE SET NULL`, so if we deleted the library first, we could still cancel by ID, but doing it first keeps the audit trail (failed job rows retain their `library_id`).
- Everything in one tx: if FTS purge fails, we don't want a dangling half-deleted library.

### Existing CASCADE coverage

Audited in `pkg/migrations/20250321211048_create_initial_tables.go`, `20260124000000_create_library_plugin_tables.go`, `20260128000000_create_plugin_field_settings.go`, `20260406100000_add_fk_cascades.go`, `20260419000000_add_user_library_settings.go`:

| Table | FK to libraries | Behavior |
|---|---|---|
| `library_paths` | `library_id` | CASCADE |
| `books` | `library_id` | CASCADE |
| `files` | `library_id` | CASCADE |
| `persons` | `library_id` | CASCADE |
| `series` | `library_id` | CASCADE |
| `genres` | `library_id` | CASCADE |
| `tags` | `library_id` | CASCADE |
| `publishers` | `library_id` | CASCADE |
| `imprints` | `library_id` | CASCADE |
| `plugin_configs` | `library_id` | CASCADE |
| `plugin_hook_configs` | `library_id` | CASCADE |
| `plugin_field_settings` | `library_id` | CASCADE |
| `user_library_settings` | `library_id` | CASCADE |
| `user_library_access` | `library_id` | CASCADE |
| `jobs` | `library_id` | SET NULL |

Indirectly via CASCADE from `books` / `files`: `book_series`, `authors`, `narrators`, `file_identifiers`, `chapters`, `file_fingerprints`, and any other child of `books` / `files`. These are the plan's responsibility only to the extent that we verify via tests that deletion succeeds and no orphans remain.

### Remove soft-delete plumbing

As part of this work, the unused `deleted_at` mechanism is removed:

- **Model:** Drop the `DeletedAt *time.Time` field from `models.Library` in `pkg/models/library.go`.
- **Service:** Remove `ListLibrariesOptions.IncludeDeleted` and the `deleted_at IS NULL` filter in `listLibrariesWithTotal` (`pkg/libraries/service.go`).
- **Handler:** In `pkg/libraries/handlers.go` `update`, drop the branch that reads `params.Deleted` and mutates `library.DeletedAt` / appends `"deleted_at"` to `opts.Columns`. Drop the `Deleted` field from the update payload struct if it is not used elsewhere.
- **Migration:** New Bun migration that drops the `deleted_at` column from `libraries`. SQLite supports `ALTER TABLE DROP COLUMN` since 3.35; we're on a newer version. Down migration re-adds the nullable column.

Any call sites referencing `IncludeDeleted` need to be updated — grep confirms it's only set inside the service itself in current code.

## Frontend

### Library Settings page

`app/components/pages/LibrarySettings.tsx` gains a **Danger Zone** section at the bottom, conditionally rendered on `hasPermission("libraries", "write")`:

```tsx
{canWrite && (
  <section className="mt-10 border border-destructive/40 rounded-md p-4 md:p-6">
    <h2 className="text-base md:text-lg font-semibold text-destructive mb-1">
      Danger Zone
    </h2>
    <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
      <div>
        <p className="font-medium">Delete this library</p>
        <p className="text-sm text-muted-foreground">
          Permanently removes this library and all of its books, files, and
          metadata from the database. Files on disk are not touched.
        </p>
      </div>
      <Button
        variant="destructive"
        size="sm"
        onClick={() => setDeleteOpen(true)}
        className="shrink-0"
      >
        <Trash2 className="h-4 w-4 sm:mr-2" />
        <span className="hidden sm:inline">Delete library</span>
      </Button>
    </div>
  </section>
)}
```

The section does not participate in the Save-on-form pattern — it is a standalone destructive action.

### DeleteLibraryDialog

New component: `app/components/library/DeleteLibraryDialog.tsx`.

Props:
```ts
interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  library: Pick<Library, "id" | "name">;
}
```

Structure:
- `<Dialog>` with `DialogContent className="max-w-md"`
- `DialogHeader`: `<AlertTriangle>` icon + "Delete library"
- Body:
  - Red warning banner listing the three caveats:
    - "This action is irreversible."
    - "Files on disk will not be deleted."
    - "Sidecar and metadata files will not be cleaned up. You'll need to remove them manually if desired."
  - Summary line: `Are you sure you want to delete “{library.name}”?`
  - Confirmation input: `Type the library name to confirm.` The Delete button is disabled until `typed === library.name` exactly.
- Footer: Cancel button; destructive Delete button with loader.

On success, the component:
- Invalidates `[QueryKey.ListLibraries]` and any library-scoped queries (covered by the mutation hook).
- Shows a success toast: `Library "{name}" deleted.`
- Navigates to `/settings/libraries`.

I am not reusing the existing `DeleteConfirmationDialog` because its `variant` union (`book | books | file`) and its file-size-summary machinery don't apply to libraries, and bolting on a "library" variant would dilute the component's purpose.

### API + query hook

- `app/libraries/api.ts`: add `deleteLibrary(id: number)` calling `fetch("/api/libraries/" + id, { method: "DELETE" })`.
- `app/hooks/queries/libraries.ts`: add `useDeleteLibrary()` returning a `useMutation` that calls `deleteLibrary`. On `onSuccess`, invalidate:
  - `[QueryKey.ListLibraries]`
  - `[QueryKey.RetrieveLibrary]`
  - `[BooksQueryKey.ListBooks]`, `[BooksQueryKey.RetrieveBook]` (a deleted library removes its books from any cached list)

### Generated types

After dropping `DeletedAt` from the Go model, run `mise tygo` to regenerate `app/types/generated/` so the frontend type for `Library` no longer has `deleted_at`.

## Testing

### Backend — Service

`pkg/libraries/service_test.go`:

- `TestDeleteLibrary_RemovesRowAndCascades` — seed a library with books, files, series, persons, genres, tags; call `DeleteLibrary`; assert zero rows remain in each child table for that `library_id`.
- `TestDeleteLibrary_PurgesFTS` — seed searchable entities, index them via the search service, delete the library, assert no surviving rows in any of the five FTS tables keyed to the deleted IDs.
- `TestDeleteLibrary_CancelsActiveJobs` — seed `pending` and `running` jobs scoped to the library (plus one `completed` and one with a different `library_id`), delete, assert the first two are now `failed`, `completed` untouched, other library's job untouched.
- `TestDeleteLibrary_NotFound` — deleting a non-existent ID returns `errcodes.NotFound`.
- `TestDeleteLibrary_Atomicity` — inject a forced failure partway through (e.g., via a test hook on FTS purge) and assert the library row survives.

### Backend — Handler

`pkg/libraries/handlers_test.go`:

- `TestDeleteLibraryHandler_RequiresWritePermission` — viewer role gets 403.
- `TestDeleteLibraryHandler_RequiresLibraryAccess` — editor without access to this library gets 403.
- `TestDeleteLibraryHandler_HappyPath` — admin deletes successfully, 204, row gone.
- `TestDeleteLibraryHandler_NotFound` — 404 on unknown ID.
- `TestDeleteLibraryHandler_TriggersOnLibraryChanged` — handler invokes the callback exactly once on success.

### Frontend

`app/components/library/DeleteLibraryDialog.test.tsx`:

- Delete button is disabled until the typed input matches the library name exactly.
- Delete button is disabled while mutation is pending; shows loader.
- On success, mutation is called with the correct ID and toast/navigation side effects fire.
- On error, dialog stays open, error toast shown.

`app/components/pages/LibrarySettings.test.tsx` (add, if existing tests cover the page):

- Danger zone is not rendered for users without `libraries:write`.
- Danger zone is rendered for users with `libraries:write`.

### Manual verification

1. `mise start`, create a library with a few books.
2. Log in as Viewer — confirm no Danger Zone visible on library settings.
3. Log in as Admin — confirm Danger Zone visible; open dialog, observe warning banner with all three caveats.
4. Try clicking Delete without typing the name — disabled.
5. Type the name, click Delete, confirm:
   - Toast appears
   - Redirected to `/settings/libraries`, deleted library not in list
   - `tmp/library/` still contains all the files
   - Sidecar `.shisho.json` files still exist alongside the originals
   - DB: `sqlite3 tmp/data.sqlite "SELECT COUNT(*) FROM books"` is zero for that library's books
   - Search: global search for a book title from the deleted library returns nothing

## Docs

`website/docs/` update:
- Update `website/docs/libraries.md` (or whichever page documents library management; we'll grep for the closest existing doc during implementation). Add a section "Deleting a library" describing:
  - Where to find the action (library settings → Danger Zone).
  - Required permission (`libraries:write`).
  - Explicit enumeration of what is and is not deleted (DB rows yes; files on disk no; sidecars no; cover files no).
  - That the action is irreversible.

No changes needed to `configuration.md`, `metadata.md`, `users-and-permissions.md` (existing permission is reused), or `supported-formats.md`.

## CLAUDE.md updates

The root `CLAUDE.md` "CASCADE does not clean up FTS indexes" gotcha already covers the pattern we're using. No new rule is needed unless the implementation surfaces one.

## Open questions

None at spec time.

## Risk / Rollback

Low-risk from a product standpoint — gated behind explicit permission and a name-typing confirmation.

Database risk: dropping the `deleted_at` column is a schema change. The down migration re-adds the nullable column but does not restore data (there is no data to restore — the column is unused in production).

Rollback plan if a bug is discovered post-deploy: revert the frontend and backend changes; the removed column is not depended on by any other system, so a forward-only rollback is safe.
