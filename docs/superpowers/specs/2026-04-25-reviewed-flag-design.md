# Reviewed Flag

## Summary

Add a per-file `Reviewed` state that tracks whether a file's metadata is "good enough" to leave alone. State is driven by an admin-configurable set of required fields ("review criteria"); a file is auto-flipped to reviewed once all required fields are populated, and back to unreviewed if a required field becomes empty. Users can also manually override either way; manual overrides are sticky in both directions. The work-queue use case is supported by a "Needs review" filter and a per-card badge in the library gallery, plus a "Missing fields" hint on the book/file detail views so users can fix gaps quickly. Bulk multi-select cascades to all files of all selected books.

## Goals

- Surface a shrinking work queue of books that need attention as new content is added or existing content is updated.
- Auto-detect "done" without forcing the user to manually mark every book.
- Let the user manually override when their judgment differs (e.g., a book that will never have a cover).
- Make missing-field diagnosis fast on the detail page.
- Avoid changing default browse behavior — the gallery still shows all books.

## Non-goals

- Per-user review state. The flag is a property of the collection, not the viewer.
- Per-library review criteria. Server-level only for v1; revisit if real demand emerges.
- Surfacing review state to OPDS / Kobo sync / eReader browser. This is a curation concept, not a sync concept.
- Automatic re-running of plugin matching ("identify") when something falls out of review. Identify remains an explicit user action.
- A separate "review queue" page or sidebar item. The filter sheet entry plus the badge are the only UX surfaces.
- A confidence-based gate beyond what already exists. The plugin confidence threshold continues to gate whether enrichment data is *applied*; review state then derives from completeness as usual.

## Mental model

A file's `reviewed` state is the disjunction of two signals:

1. **Auto-completeness** — does the file (plus its book's shared fields) have everything the admin marked as required?
2. **Manual override** — did a user say "yes, reviewed" or "no, still needs review" explicitly?

Manual override wins when present. Otherwise, completeness drives the state. New files start with no override and the auto signal in effect; bulk and book-level user gestures cascade to set overrides on all files.

## Schema

### `files` table — new columns

| Column | Type | Nullable | Notes |
|---|---|---|---|
| `review_override` | TEXT | yes | `'reviewed'`, `'unreviewed'`, or NULL. NULL means "follow auto-completeness." |
| `review_overridden_at` | TIMESTAMP | yes | Set whenever `review_override` is set; cleared when override is cleared. Drives the "Manually set on Apr 10" indicator. |
| `reviewed` | BOOLEAN | yes | Denormalized cache for fast filtering. NULL for supplements (`file_role = 'supplement'`); BOOLEAN for main files. Recomputed on every event listed in [Auto-evaluation triggers](#auto-evaluation-triggers). |

Index: `CREATE INDEX idx_files_book_reviewed ON files(book_id, reviewed) WHERE file_role = 'main';` — supports the book-level aggregation queries.

CHECK constraint on `review_override`: must be one of `'reviewed'`, `'unreviewed'`, or NULL.

### `app_settings` (or equivalent) — new keys

Required-fields config lives at the server level. Storage uses the same mechanism the existing config system uses for runtime-mutable settings (kept distinct from `pkg/config/config.go` static config). Keys:

- `review_criteria.book_fields` — JSON array of universal field names that are required.
- `review_criteria.audio_fields` — JSON array of audio-only field names required (applies when book has at least one M4B file).

Default values:

```json
{
  "review_criteria": {
    "book_fields": ["authors", "description", "cover", "genres"],
    "audio_fields": ["narrators"]
  }
}
```

Candidate fields (the set the admin can choose from):

- **Universal:** `authors`, `description`, `cover`, `genres`, `tags`, `series`, `subtitle`, `publisher`, `imprint`, `identifiers`, `release_date`, `language`, `url`
- **Audio-only:** `narrators`, `chapters`, `abridged`

Excluded from candidate list (auto-derived, intrinsic, or low-signal): `title` (always derivable from filename — always present, no-op to check), `sort_title`, `page_count`, `audiobook_duration_seconds`, `audiobook_bitrate_bps`, `audiobook_codec`, `cover_mime_type`, `name`.

## Completeness rule

For a main file `f` belonging to book `b`, `f` is **complete** when **every** field in the active criteria evaluates as "present":

```
for each field in review_criteria.book_fields:
  must be present on b (or on f for fields that live on the file — see field locations below)
for each field in review_criteria.audio_fields:
  if any file in b is type m4b:
    must be present on every m4b file of b
```

### Field locations

Some fields are book-level (shared across all of the book's files), some are file-level. The completeness check pulls each field from the right place:

| Field | Lives on | Notes |
|---|---|---|
| authors | book | non-empty `Authors` relation |
| description | book | non-null, non-empty `Description` |
| genres | book | non-empty `BookGenres` relation |
| tags | book | non-empty `BookTags` relation |
| series | book | non-empty `BookSeries` relation |
| subtitle | book | non-null, non-empty `Subtitle` |
| cover | file | non-null `CoverImageFilename` |
| publisher | file | non-null `PublisherID` |
| imprint | file | non-null `ImprintID` |
| identifiers | file | non-empty `Identifiers` relation |
| release_date | file | non-null `ReleaseDate` |
| language | file | non-null, non-empty `Language` |
| url | file | non-null, non-empty `URL` |
| narrators | file (audio only) | non-empty `Narrators` relation |
| chapters | file (audio only) | non-empty `Chapters` relation |
| abridged | file (audio only) | non-null `Abridged` |

## Effective `reviewed` value

```
reviewed_for(file f) =
  if f.file_role = 'supplement': NULL
  else if f.review_override = 'reviewed': TRUE
  else if f.review_override = 'unreviewed': FALSE
  else: completeness_for(f)
```

Stored in `files.reviewed`. Recomputed whenever any input changes.

## Book-level aggregation

Used for filtering and the book-detail view. Books are not stored with their own `reviewed` column; aggregation runs at query time:

- **A book is "reviewed"** iff every main file has `reviewed = TRUE`.
- **A book "needs review"** iff at least one main file has `reviewed = FALSE`.
- **A book with zero main files** is degenerate (typically transient during scan); not in either bucket.

Query patterns:

```sql
-- Books needing review (treats NULL as needs-review during migration gap):
SELECT b.* FROM books b
WHERE EXISTS (
  SELECT 1 FROM files f
  WHERE f.book_id = b.id
    AND f.file_role = 'main'
    AND (f.reviewed = FALSE OR f.reviewed IS NULL)
);

-- Reviewed books (require all main files to be explicitly TRUE):
SELECT b.* FROM books b
WHERE NOT EXISTS (
  SELECT 1 FROM files f
  WHERE f.book_id = b.id
    AND f.file_role = 'main'
    AND (f.reviewed = FALSE OR f.reviewed IS NULL)
)
AND EXISTS (
  SELECT 1 FROM files f
  WHERE f.book_id = b.id AND f.file_role = 'main'
);
```

## Auto-evaluation triggers

The `files.reviewed` column is recomputed on any event that could change a file's completeness. Centralized in a `pkg/books/review.RecomputeReviewedForFile(ctx, fileID)` helper called from:

- File create (scan finds a new file): `pkg/books/service.go: CreateFile`
- File update (manual edit, plugin enrichment, scanner overwrite): `pkg/books/service.go: UpdateFile` and `pkg/worker/scan_unified.go` enrich path.
- Book update (book-level fields change → cascades to all files of book): `pkg/books/service.go: UpdateBook` calls `RecomputeReviewedForBook(ctx, bookID)` which iterates main files.
- Identify dialog apply: existing `pkg/plugins/handler_persist_metadata.go` and the `IdentifyBookDialog` apply path.
- Chapters PUT: `pkg/chapters/handlers.go`.
- File-relation changes: anything that mutates the relations checked by completeness — narrators, identifiers, genres, tags, authors, series, publisher, imprint changes.
- File move between books: `MoveFilesToBook` triggers recompute on source files (because their book context is gone) and target book's files (because shared book-level fields may differ).

The override (`review_override`) is set/cleared via dedicated endpoints. Setting an override does **not** itself recompute completeness — the override short-circuits the result. But after clearing an override, completeness is recomputed.

The review-criteria settings change triggers a background job (see [Background recompute job](#background-recompute-job)).

## Auto-evaluation behavior

- Auto-flip can move `reviewed` from FALSE → TRUE when completeness becomes met (no override present).
- Auto-flip can move `reviewed` from TRUE → FALSE when a required field becomes empty (no override present).
- Manual override (`'reviewed'` or `'unreviewed'`) suppresses both directions until cleared.
- Adding a new file to a previously-reviewed book: the new file gets `review_override = NULL`, `reviewed = computed`. The book aggregate flips back to "needs review" if the new file is incomplete. Existing files' overrides are untouched.

## Manual override semantics

User-facing toggle is binary (`Reviewed: yes/no`). Behind the scenes, every click on a review toggle (book-level, file-level, or bulk) sets `review_override` to the chosen value (`'reviewed'` or `'unreviewed'`) and updates `review_overridden_at`. The override is **sticky in both directions** until another user gesture changes it.

Override clearing happens only in two paths, neither of which is part of the common toggle gesture:
- The admin "Recompute review state" action with `clear_overrides = true` (settings change with the checkbox ticked, or the manual recompute button with the option enabled). Clears overrides for all main files in scope.
- Internal callers (e.g., test helpers).

The user is not given a per-book "clear override" UI in v1. If they want to drop a stale override, they re-toggle the book in their preferred direction and let auto take it from there on the next field change. If real demand emerges for a "Reset to auto" gesture, add a menu item later.

## Settings: review criteria

New section in `AdminSettings.tsx` titled **"Review Criteria"**. Two checkbox groups:

- **Required for all books** — checkboxes for each universal candidate field. Default-on: authors, description, cover, genres.
- **Required for audiobooks (additional)** — checkboxes for each audio-only candidate field. Default-on: narrators. Help text: "These apply when a book has any audiobook file."

A **Save** button persists the new criteria, then triggers the background recompute. If `count(files where review_override IS NOT NULL AND library is current/all) > 0`, the save action shows a confirmation dialog **before** persisting:

> Recompute review state?
>
> Auto-detected reviewed status will refresh based on the new criteria. You currently have **N reviewed-overrides set out of M total main files**.
>
> ☐ Also clear manual overrides (default off)
>
> [Cancel] [Save and Recompute]

If the count is 0, no dialog — save and recompute fire directly. The settings save returns synchronously; the recompute runs in the background.

A **"Recompute review state now"** button appears below the criteria groups. Triggers the recompute job without changing any settings. Same confirmation dialog when overrides exist > 0.

## Background recompute job

New job type: `recompute_review`. Payload:

```go
type RecomputeReviewPayload struct {
  ClearOverrides bool `json:"clear_overrides"`
}
```

Worker behavior:

1. If `ClearOverrides`, run `UPDATE files SET review_override = NULL, review_overridden_at = NULL` across all main files.
2. Walk all main files in batches (chunks of N — pick based on existing scan-job batch sizing). For each, recompute `reviewed`.
3. Emit progress via the existing job-status SSE stream.
4. On completion, invalidate book-list and book-retrieve query caches via the existing event mechanism.

Job is queued by:
- Migration (initial population — see [Migration](#migration)).
- Settings save.
- Manual "Recompute now" admin button.

Permission to enqueue: `config:write` (settings change) and a new `jobs:write` is already required for the manual button via existing job-creation routes.

## API surface

### `PATCH /books/files/:id/review`

```go
type SetFileReviewPayload struct {
  // null = clear override; "reviewed" or "unreviewed" otherwise
  Override *string `json:"override" validate:"omitempty,oneof=reviewed unreviewed"`
}
```

Permission: `books:write` + library access on the file's library. Sets `review_override`, updates `review_overridden_at`, recomputes `reviewed`. Returns the updated file.

### `PATCH /books/:id/review` (book-level cascade)

```go
type SetBookReviewPayload struct {
  Override *string `json:"override" validate:"omitempty,oneof=reviewed unreviewed"`
}
```

Cascades to all main files of the book, identical to setting each file individually. Returns the updated book with files.

### `POST /books/bulk/review` (multi-select cascade)

```go
type BulkSetReviewPayload struct {
  BookIDs  []int   `json:"book_ids" validate:"required,min=1,max=500"`
  Override *string `json:"override" validate:"omitempty,oneof=reviewed unreviewed"`
}
```

Cascades to all main files of all listed books. Library-access check is enforced per book; books the user can't access are silently skipped (consistent with existing bulk patterns).

### `GET /books?reviewed_filter=needs_review|reviewed|all`

Add to existing book-list query parameters. `all` is the default and current behavior. `needs_review` and `reviewed` use the aggregation queries from [Book-level aggregation](#book-level-aggregation).

### `GET /settings/review-criteria` and `PUT /settings/review-criteria`

Standard admin settings endpoints. PUT triggers the recompute job; supports the `clear_overrides` query/body flag.

### `POST /jobs` with `type: "recompute_review"`

Manual trigger via the existing job-creation route, gated by `jobs:write`. Body matches the payload above.

## UI

### Library gallery (`Gallery` component) — book card

- Add a small "Needs review" badge (matching existing badge styles, e.g., the `Badge` component with `variant="secondary"`) overlaid on the bottom-right corner of the cover or in the card metadata row. Visible only when the book aggregate is `needs_review`. Reviewed books show no badge — i.e., the absence of a badge means "good."
- No "missing fields" hint on cards.

### Library gallery — `FilterSheet`

Add a new "Review state" section with three radio options:
- All (default)
- Needs review
- Reviewed

The current value participates in `ActiveFilterChips` like other filters.

### Bulk-select toolbar (`SelectionToolbar.tsx`)

The toolbar is already crowded (Add, Download, Merge, Delete, Clear, X). Add a single **"More" button (kebab/three-dots icon)** that opens a popover with:

- "Mark reviewed"
- "Mark needs review"

Tap one → fires `POST /books/bulk/review` with the chosen override → exits selection mode → toast confirmation. Same disabled/enabled rules as Delete (requires `books:write` and library access).

### Book detail page (`BookDetail.tsx`)

Add a small "Review" panel near the top, below or beside the existing actions row. Contents:

- **Toggle**: "Reviewed" — single binary control. Tri-state visual when the book's files are mixed (some reviewed, some not, none manually overridden in a uniform way) — render as indeterminate with a hover tooltip listing per-file states.
- **State indicator** (small text below the toggle):
  - "Auto" if no override is present on any file of the book.
  - "Manually set on {date}" if all files share the same override (use the most recent `review_overridden_at`).
  - "Manually set on multiple files" if overrides differ across files.
- **Missing fields hint** (only when book aggregates to needs-review): "Missing: {fields}" — a comma-separated list aggregated across all main files. If the same field is missing on all files, list it once. If different files miss different fields, qualify with file type or filename — e.g., `Missing: cover (EPUB), narrators (M4B)`.

Toggling the book-level control fires `PATCH /books/:id/review` and cascades.

### File edit dialog (`FileEditDialog.tsx`)

Add a "Review" subsection at the top, mirroring the book-level panel but scoped to the single file:

- Toggle (Reviewed / Needs review).
- State indicator (Auto / Manually set on {date}).
- Missing fields hint (only when the file is currently incomplete).

Toggling fires `PATCH /books/files/:id/review`.

### Admin settings (`AdminSettings.tsx`)

New "Review Criteria" section — see [Settings: review criteria](#settings-review-criteria).

## Migration

New migration: `pkg/migrations/<timestamp>_add_file_review_fields.go`.

1. Add columns to `files`: `review_override`, `review_overridden_at`, `reviewed` (all nullable). No initial population — `reviewed` stays NULL until the recompute job runs.
2. Add `idx_files_book_reviewed`.
3. Add CHECK constraint on `review_override`.
4. Seed the `review_criteria` settings keys with the defaults from [Schema → app_settings](#schema).
5. Enqueue a `recompute_review` job (with `clear_overrides = false`) so all files settle into the correct state asynchronously after the worker picks it up.

NULL semantics during the gap between migration and job completion: queries treat `reviewed IS NULL` for main files as "not yet computed" and include them in the **needs-review** filter (the conservative bucket — better to surface them than to hide them). The reviewed filter excludes NULL. Once the job completes (typically seconds to minutes for libraries up to tens of thousands of files), every main file has a non-NULL `reviewed` and the gap closes.

This keeps the migration trivially fast (column adds only) and uses the existing job mechanism for the actual computation.

## Telemetry / observability

- Recompute job emits standard job logs and progress.
- `reviewed` transitions are not audit-logged in v1. If a user manually toggles, the `review_overridden_at` timestamp is the only trace. (Add audit logging later if it becomes a debugging need.)

## Tests

### Backend

- `pkg/books/review_test.go` — completeness rule unit tests for every candidate field, with both book-level and file-level fields, with audio/non-audio books, with multi-file books, with mixed file types.
- `pkg/books/service_test.go` (extend) — `RecomputeReviewedForFile` and `RecomputeReviewedForBook` happy paths + override interactions.
- `pkg/worker/scan_unified_test.go` (extend) — file create, file update, plugin enrich each trigger recompute and produce the right state.
- `pkg/books/handlers_test.go` (extend) — `PATCH /review` endpoints set state correctly, respect permissions and library access, and the `reviewed_filter` query param filters as specified.
- `pkg/worker/recompute_review_test.go` (new) — job processes all main files, honors `clear_overrides`, ignores supplements.

All new tests use `t.Parallel()` per project convention.

### Frontend

- `Gallery.test.tsx` (extend) — needs-review badge visibility based on book state.
- `FilterSheet.test.tsx` (extend) — new review state filter renders and applies.
- `BookDetail.test.tsx` (extend) — review panel renders all three states (auto / manual / mixed) and missing-fields hint formatting.
- `FileEditDialog.test.tsx` (extend) — file-level review toggle.
- `SelectionToolbar.test.tsx` (extend) — bulk mark popover items fire the right mutation.

### E2E

- `e2e/review-flag.spec.ts` (new) —
  1. Import a book with missing fields → appears in needs-review filter.
  2. Fill in the missing fields via edit dialog → auto-flips out of the filter.
  3. Toggle manual unreviewed → reappears in the filter even though it's complete.
  4. Toggle manual reviewed → drops out of the filter even when missing fields.
  5. Bulk-select two books and "Mark reviewed" → both drop out.

## Documentation

- New page: `website/docs/review-state.md` — explains the concept, the auto-flip rules, manual override semantics, the "Review Criteria" admin setting, and how the gallery filter and badge work.
- Update `website/docs/configuration.md` — only if any of these settings end up in the static config file (which they don't in this design — they live in runtime settings).
- Update `website/docs/users-and-permissions.md` if new permission is added (none added — `books:write` covers manual toggles, `config:write` covers settings, `jobs:write` covers the recompute button).
- Cross-link from `website/docs/metadata.md` and the gallery/library doc (whichever exists) to the new page.

## Open questions / future work

- Per-library criteria override (drop-in extension: add a `library_id`-keyed setting that overrides the server-level one).
- Surfacing reviewed/unreviewed in OPDS as a category facet.
- A "Review reason" annotation per book (free-text note on why a manual override was applied) — only worth doing if users ask for it.
- Audit log entries for review-state transitions.
