# Gallery Sort Design

## Overview

Multi-level sort for the library gallery view. Users can sort by one or more fields (author, series, date added, etc.), reorder sort levels, and pick ascending/descending per level. Non-default sort lives in the URL (`?sort=…`) so it's shareable, bookmarkable, and survives a refresh. Users can promote their current sort to a persistent default for that library via a "Save as default" action, stored server-side and keyed on `(user × library)` so it follows them across devices.

A visual design reference is checked in at `/private/tmp/shisho-gallery-sort-mockups.html` (standalone HTML mockup covering the three UI states, URL/storage semantics, and the mobile drawer variant).

## Goals

- Let users sort the gallery by any of a fixed whitelist of fields, optionally with multiple sort levels
- Make non-default sort URL-addressable (shareable, survives refresh)
- Let each user save a per-library default sort that persists across devices
- Apply the user's saved default consistently across every authenticated book-listing surface (Gallery, OPDS, eReader) so the reader sees the same order everywhere

## Non-Goals

- Global per-user default sort (a "use this sort on every library") — can be added later; `user_settings` is the natural home
- Ad-hoc sort by fields outside the v1 whitelist
- Server-side persisted "sort templates" (named saved sort configurations)
- Sort within non-gallery book-centric views (series detail, search results) — out of scope for v1
- Re-sorting Lists (which already have their own ordering model — see Cross-Surface Sort Application)
- Re-sorting Kobo sync output (Kobo's sync protocol doesn't surface an order to the user)

## Data Model

### New Table: `user_library_settings`

Generic per-`(user × library)` settings bucket, parallel to the existing per-user `user_settings` table. Structured so future per-library preferences (default view mode, default filter, grid density, etc.) can land as additional columns without spawning more tables.

```sql
CREATE TABLE user_library_settings (
  id          INTEGER  PRIMARY KEY AUTOINCREMENT,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  user_id     INTEGER  NOT NULL REFERENCES users     (id) ON DELETE CASCADE,
  library_id  INTEGER  NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
  sort_spec   TEXT  -- nullable; NULL = "use the builtin frontend default" for sort
);

CREATE UNIQUE INDEX ux_user_library_settings
  ON user_library_settings (user_id, library_id);
```

Each feature-level column is **nullable** so resetting one setting (e.g. clearing the saved sort) doesn't wipe the row and clobber other settings. A missing row and a row with `sort_spec = NULL` are semantically identical for sort.

### Bun Model

`pkg/models/user_library_settings.go`:

```go
package models

import (
    "time"

    "github.com/uptrace/bun"
)

type UserLibrarySettings struct {
    bun.BaseModel `bun:"table:user_library_settings,alias:uls" tstype:"-"`

    ID        int       `bun:",pk,autoincrement"                        json:"id"`
    CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
    UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
    UserID    int       `bun:",notnull"                                 json:"user_id"`
    LibraryID int       `bun:",notnull"                                 json:"library_id"`
    SortSpec  *string   `bun:",nullzero"                                json:"sort_spec"`
}
```

### Migration

`pkg/migrations/20260419000000_add_user_library_settings.go` — creates the table and the unique composite index in an `up`, drops them in `down`. Follows the existing migration pattern (see `pkg/migrations/20260116100000_add_user_settings.go` for the analogous per-user settings migration).

## URL Scheme

Non-default sort is serialized as a single `sort` query param:

```
?sort=<field>:<dir>[,<field>:<dir>]...
```

- `<field>` must be in the v1 whitelist (see below)
- `<dir>` is `asc` or `desc`
- Order is significant — leftmost is the primary sort key
- No `sort` param = "use the resolved default" (DB → builtin, see Resolution Logic)
- Invalid specs are rejected with 400 on the backend and cleaned from the URL on the frontend

Examples:

```
?sort=author:asc,series:asc
?sort=date_added:desc
?sort=title:asc
```

Rationale for this shape over parallel arrays (`?sort_by=author,series&sort_dir=asc,asc`): pairing field and direction together eliminates the "arrays drift out of sync" failure mode. Parser is trivial either way.

## Sort Fields (v1 whitelist)

The backend accepts exactly these field tokens. Any other token is rejected.

| Field             | Source (primary)                                 | Fallback for missing primary            | Contextual direction labels |
|-------------------|--------------------------------------------------|-----------------------------------------|-----------------------------|
| `title`           | `books.sort_title`                               | —                                       | A → Z / Z → A               |
| `author`          | primary author's `persons.sort_name` (primary = `authors` row for this book with lowest `sort_order`, ties broken by `authors.id ASC`; books with zero authors sort last) | — | A → Z / Z → A |
| `series`          | series `sort_name`, then `book_series.series_number ASC` | — (books with no series always sort last, regardless of direction; see NULLS-LAST note below) | A → Z / Z → A |
| `date_added`      | `books.created_at`                               | —                                       | Newest / Oldest             |
| `date_released`   | primary file's `release_date`                    | any other file on the book with a non-NULL `release_date` | Newest / Oldest |
| `page_count`      | primary file's `page_count`                      | any other file on the book with a non-NULL `page_count`  | Shortest / Longest |
| `duration`        | primary file's `audiobook_duration_seconds`      | any other file on the book with a non-NULL `audiobook_duration_seconds` | Shortest / Longest |

### "Primary file with fallback" semantics

For `date_released`, `page_count`, and `duration`, the sort key is resolved as:

> Prefer the value from the book's primary file (`books.primary_file_id`). If the primary file has `NULL` for that field, use the value from any other file on the book that has a non-NULL value for that field. If no file on the book has the field, the book sorts last (NULLS LAST) regardless of direction.

SQL pattern (generalized; the outer query is `ListBooks`, aliased `b` per `pkg/CLAUDE.md`):

```sql
COALESCE(
  (SELECT f.<field> FROM files f WHERE f.id = b.primary_file_id),
  (SELECT f.<field> FROM files f
     WHERE f.book_id = b.id AND f.<field> IS NOT NULL
     ORDER BY f.id
     LIMIT 1)
)
```

### NULLS LAST

Every sort level emits `NULLS LAST` semantics — books missing the sort key always sit at the end, regardless of direction. SQLite predates native `NULLS LAST`, so it's emulated with a leading `<expr> IS NULL` term:

```sql
ORDER BY <expr> IS NULL, <expr> ASC   -- or DESC
```

This applies uniformly to `author` (zero authors), `series` (no series row), and the three primary-file-backed fields (`date_released`, `page_count`, `duration`).

### Series sort implicit secondary

When `series` is in the sort spec, it expands on the backend to `series_sort_name <dir>, series_number ASC`. That is, a user-selected `series:asc` is internally equivalent to "by series sort name, then by series number ascending within each series" — Stormlight #1 always comes before #2 regardless of whether the user picked asc or desc for series name. This is not exposed as two separate levels in the UI.

## Resolution Logic

On initial page load the frontend renders **nothing from the gallery** until the resolved sort is known. Resolution order:

1. **URL** — if `?sort=` is present and parses, use it. Set the "dot" indicator (sort differs from the saved default).
2. **DB default** — fetch `GET /settings/libraries/:library_id`; if `sort_spec` is a non-NULL valid spec, use it. No dot.
3. **Builtin default** — `date_added:desc` (mockup shows this as "Date added, newest first"). No dot.

Blocking the gallery render on step 2 is acceptable because the DB lookup is a single indexed row fetch on `(user_id, library_id)`; assume sub-30ms. This avoids a flash-of-wrong-default that would otherwise require reshuffling the grid.

The "dot" on the Sort button is shown whenever the *effective* sort differs from the resolved default (i.e. URL took precedence over a stored or builtin default).

### "Save as default" action

When the user clicks **Save as default** in the sheet:

1. Frontend sends `PUT /settings/libraries/:library_id` with `{ sort_spec: "<current URL spec>" }`, optimistically.
2. Frontend clears the `?sort=…` URL param. (The gallery keeps rendering the same data — the resolved sort is unchanged.)
3. On success: dot disappears, "Sorted by" chip bar disappears.
4. On error: restore the URL param, re-show the dot + bar, toast the error.

### "Reset" link

Clears the `?sort=…` URL param without touching the DB. The next resolution round reads from DB (or builtin).

## API Changes

All three endpoints live in `pkg/settings/` next to the existing `GET/PUT /settings/viewer`. Auth required via `authMiddleware.Authenticate` on the `/settings` group.

### Read: `GET /settings/libraries/:library_id`

Response body:

```json
{ "sort_spec": "author:asc,series:asc" }
```

Returns `{ "sort_spec": null }` when no row exists. Validates the caller has access to the library (inline check via `user.HasLibraryAccess(libraryID)`), returns 403 if not.

### Write: `PUT /settings/libraries/:library_id`

Request body (struct-wrapped per the binder rule — see `pkg/CLAUDE.md`):

```go
type updateLibrarySettingsPayload struct {
    SortSpec *string `json:"sort_spec"` // null clears the stored default
}
```

Behavior:

- Validates `sort_spec` against the v1 whitelist parser before writing. Invalid → 400.
- Upserts into `user_library_settings` via Bun's `ON CONFLICT (user_id, library_id) DO UPDATE`, setting only `sort_spec` + `updated_at` (future columns will add themselves here).
- Returns the updated row.

### Book list: add `sort` param to `GET /books`

Extend `ListBooksQuery` in `pkg/books/validators.go` with one new field:

```go
Sort string `query:"sort" validate:"omitempty,max=200"`
```

Parsed in the handler, passed to `ListBooksOptions.Sort` as a structured `[]SortLevel`.

**Sort resolution inside the handler** (same logic is reused by OPDS and eReader — see Cross-Surface Sort Application below):

1. If the request carries an explicit `sort` query param, parse it and use it.
2. Else, if the request is scoped to a single library (i.e. `library_id` filter is present) and the caller has a stored `user_library_settings.sort_spec` for that `(user, library)`, use that.
3. Else, fall back to the existing hard-coded default (`sort_title ASC`, or `series_number ASC, sort_title ASC` when filtering by series).

Series-detail requests (`library_id=X&series_id=Y`) hit the same handler as the gallery and go through the same resolution path. A user's stored sort will therefore also shape the series-detail view. This is fine: sort levels that are redundant with the filter (e.g. sorting a one-author series feed by author) still produce valid output — they just don't reorder anything for that level, and subsequent levels take over.

Validation rejects unknown fields, malformed directions, and duplicate fields (`?sort=author:asc,author:desc`). Over 10 sort levels rejected as 400.

Note: prior to this change, the API always applied the hard-coded default when no `sort` param was passed. After this change, authenticated callers listing books within a single library will see their stored preference honored by default. This is intentional — the whole point of the design is that the user's "default sort" is a real default, not a gallery-only affordance.

## Backend Changes

### New package: `pkg/sortspec/`

Parsing, validation, SQL expression building, and user-preference resolution for sort specs. The parser/whitelist/SQL layers are pure functions with no DB dependency; only `resolve.go` touches the database (via an injected settings service).

```
pkg/sortspec/
  spec.go          // SortLevel type, Parse(string) ([]SortLevel, error)
  whitelist.go     // canonical field list + validation
  sql.go           // OrderExpressions(levels []SortLevel) []OrderExpr
  resolve.go       // ResolveForLibrary(ctx, reader, userID, libraryID, explicit) []SortLevel
```

The resolver depends on a narrow interface rather than the concrete `*settings.Service` to avoid an import cycle (`pkg/settings` needs `pkg/sortspec` for validation at write time):

```go
type LibrarySettingsReader interface {
    GetLibrarySettings(ctx context.Context, userID, libraryID int) (*models.UserLibrarySettings, error)
}

func ResolveForLibrary(
    ctx context.Context,
    reader LibrarySettingsReader,
    userID, libraryID int,
    explicit []SortLevel,
) []SortLevel
```

`*settings.Service` satisfies this interface by exposing `GetLibrarySettings`; callers pass it directly.

`sql.go`'s `OrderExpressions` returns a slice of expressions ready to pass to Bun's `.OrderExpr(...)`. The `series` level expands to two expressions (series name, then series number). The `date_released`/`page_count`/`duration` levels build the `COALESCE(primary, any-other-file)` subquery. `NULL`s are forced last via `<expr> IS NULL`.

`resolve.go`'s `ResolveForLibrary` is the single source of truth for "what sort does this caller get?":

- If `explicit` is non-empty (handler parsed a URL param), return it unchanged.
- Else fetch `user_library_settings` for `(userID, libraryID)`; if present and `sort_spec` parses, return those levels.
- Else return `nil` (the caller falls back to whatever hard-coded default it already used).

Every surface that lists books (gallery handler, OPDS, eReader) goes through this function, so they can't drift out of sync.

Unit tests in `spec_test.go` (grammar), `whitelist_test.go` (validation errors), `sql_test.go` (expression shape — snapshot-style), `resolve_test.go` (all three branches: explicit wins, stored spec used, missing row returns nil).

### `pkg/books/service.go`

- `ListBooksOptions` gains a `Sort []sortspec.SortLevel` field.
- `ListBooksWithTotal` calls `sortspec.OrderExpressions(opts.Sort)` and uses the returned expressions via `q.OrderExpr(...)` when non-empty. When empty, the existing default order stays.
- Existing callers (handler only) pass `nil` until wired up.

### `pkg/books/handlers.go`

In the `list()` handler, after binding `ListBooksQuery` and resolving the user + library:

```go
var explicit []sortspec.SortLevel
if q.Sort != "" {
    levels, err := sortspec.Parse(q.Sort)
    if err != nil {
        return errcodes.BadRequest("invalid sort spec: " + err.Error())
    }
    explicit = levels
}

// ResolveForLibrary returns explicit when set, otherwise looks up the stored
// preference, otherwise returns nil so ListBooks uses its hard-coded default.
// Only resolve against the DB when the caller is scoping to a single library.
if q.LibraryID != nil {
    opts.Sort = sortspec.ResolveForLibrary(ctx, h.settingsService, user.ID, *q.LibraryID, explicit)
} else {
    opts.Sort = explicit
}
```

### `pkg/settings/` — new routes

Extend `pkg/settings/routes.go`:

```go
g.GET("/libraries/:library_id", h.getLibrarySettings)
g.PUT("/libraries/:library_id", h.updateLibrarySettings)
```

Add `getLibrarySettings`, `updateLibrarySettings` handlers in `pkg/settings/handlers.go` (split out from `service.go` if it isn't already — today the service file also contains handler-like code; extracting handlers keeps the domain file pattern from `pkg/CLAUDE.md`).

Add to `pkg/settings/service.go`:

```go
// GetLibrarySettings returns the row for (userID, libraryID), or a zero-valued one if none.
func (svc *Service) GetLibrarySettings(ctx context.Context, userID, libraryID int) (*models.UserLibrarySettings, error)

// UpsertLibrarySort writes just the sort_spec column for (userID, libraryID),
// leaving other columns untouched on conflict.
func (svc *Service) UpsertLibrarySort(ctx context.Context, userID, libraryID int, sortSpec *string) (*models.UserLibrarySettings, error)
```

`UpsertLibrarySort` uses the same `.On("CONFLICT (user_id, library_id) DO UPDATE").Set(...)` pattern as `UpdateViewerSettings`, touching only `sort_spec` + `updated_at`.

### Library access check

Both new handlers validate `user.HasLibraryAccess(libraryID)` inline after fetching the user from the Echo context. Pattern matches the existing book/file handlers per `pkg/CLAUDE.md`'s Handler-Level Library Access Checks section.

### Cross-Surface Sort Application

A user's saved library sort should feel like a real default — the same order should appear everywhere the app lists books for that user, not just in the gallery. The authoritative resolver (the `ListBooks` handler) already honors the stored preference; the job in the other surfaces is to route their calls through (or mirror) that resolver.

The inventory below is complete as of this design. Every authenticated book-listing surface is covered either by applying the sort or by documenting why it can't.

#### Surfaces that WILL apply the user's library sort

Each of these surfaces has a resolvable user identity and enumerates books to the user. They all need to:

1. Resolve the caller's user ID.
2. When the listing is scoped to a single library, look up `user_library_settings.sort_spec` for `(user_id, library_id)`.
3. Pass that as `Sort` to `ListBooks` (falling back to the hard-coded default if absent).

The resolution logic is centralized in `pkg/sortspec.ResolveForLibrary` (signature above), so every surface makes the same decision the same way. The series-detail API (`GET /books?series_id=…`) is deliberately *not* a separate row below — it's the same handler as the gallery, just with an extra filter; the resolver treats it identically.

| Surface | Auth | File / line | User resolution |
|---------|------|-------------|-----------------|
| Books gallery + series-detail API (`GET /books`) | Session | `pkg/books/handlers.go:152` | `c.Get("user").(*models.User)` |
| OPDS all-books root feed | Basic Auth | `pkg/opds/service.go:132` | `c.Get("user").(*models.User)` (set in `BasicAuth` middleware, `pkg/auth/middleware.go:182`) |
| OPDS library feed | Basic Auth | `pkg/opds/service.go:176` | same as above |
| OPDS "recently added" feed | Basic Auth | `pkg/opds/service.go:278` | same as above |
| OPDS author feed | Basic Auth | `pkg/opds/service.go:330` | same as above |
| OPDS series feed | Basic Auth | `pkg/opds/service.go:487` | same as above |
| OPDS genre feed | Basic Auth | `pkg/opds/service.go:602` | same as above |
| OPDS tag feed | Basic Auth | `pkg/opds/service.go:651` | same as above |
| eReader library index | API key | `pkg/ereader/handlers.go:169` | `apiKey.UserID` (API key struct stored in context) |
| eReader author page | API key | `pkg/ereader/handlers.go:188` | same as above |
| eReader series page | API key | `pkg/ereader/handlers.go:308` | same as above |
| eReader genre page | API key | `pkg/ereader/handlers.go:499` | same as above |

For OPDS feeds that are *not* scoped to a single library (currently only the "all books" root feed when the user has access to multiple libraries), we skip the DB lookup and fall through to the hard-coded default. Rationale: `user_library_settings` is per-library by design, and cross-library aggregate ordering isn't a setting we've committed to.

For eReader surfaces, `apiKey.UserID` is already available in the handler (see the existing `getUserLibraryIDs` calls at lines 84, 155, 235, 290, 352, 407, 481, 555) so no additional query is required to identify the user.

#### Surfaces that will NOT apply the user's library sort

| Surface | Why excluded |
|---------|--------------|
| Lists (`pkg/lists/service.go:309`) | Lists have their own five-mode ordering system (`manual`, `title`, `author`, etc., set per-list). Libraries and lists are separate ordering domains; sharing one setting between them would be surprising. Future work could add a `user_list_settings` parallel, but it's not this design. |
| Kobo sync (`pkg/kobo/service.go:177`) | Kobo's sync protocol pushes books to the device but doesn't expose an order; the device re-sorts locally per the user's on-device settings. No behavior to align. |
| Search (`pkg/search/service.go:125-178`) | Search results are ranked by FTS5 `rank`, not by the fields in our whitelist. A sorted-by-title search loses the "most relevant first" signal. Leaving search alone. |

#### Implementation touchpoints for cross-surface application

- **`pkg/sortspec/resolve.go`** — defined above. Returns `explicit` if non-empty; otherwise queries `user_library_settings` via the injected `LibrarySettingsReader` and parses `sort_spec`; otherwise returns `nil` (meaning "let the caller fall back to its default"). Single place to get sort-resolution logic.
- **`pkg/books/handlers.go`** — `list` handler calls `ResolveForLibrary` before passing `Sort` to `ListBooks`.
- **`pkg/opds/service.go`** — each of the seven listed feed handlers resolves the user from `c.Get("user")`, calls `ResolveForLibrary`, and passes the result as `Sort` to its `ListBooks` call. Only applies when the feed is already filtering by `library_id`.
- **`pkg/ereader/handlers.go`** — each of the four listed handlers uses `apiKey.UserID` + the feed's library ID (or the library containing the entity for author/series/genre pages) and calls `ResolveForLibrary` the same way.
- No changes needed in `pkg/lists/`, `pkg/kobo/`, or `pkg/search/`.

## Frontend Changes

### New files

- `app/hooks/queries/librarySettings.ts` — mirrors `app/hooks/queries/settings.ts`:
  - `useLibrarySettings(libraryId: number)` — `GET /settings/libraries/:id`
  - `useUpdateLibrarySettings()` — `PUT /settings/libraries/:id`, invalidates `[QueryKey.LibrarySettings, libraryId]` and `[BooksQueryKey.ListBooks]`
- `app/components/library/SortSheet.tsx` — structural mirror of `FilterSheet.tsx`. Desktop: `Sheet`; mobile (under `md`): `Drawer`. Body contains the sort levels (drag-reorderable with `@dnd-kit/sortable`, already in deps), a "Then by…" adder, and the "Save as default" block.
- `app/components/library/SortedByChips.tsx` — the "Sorted by: [chip] [chip] reset to default" row, mirroring `ActiveFilterChips`. Clicking a chip **removes that sort level** from the URL (matching the "click a filter chip to remove that filter" interaction in `ActiveFilterChips`). Removing the last level collapses the row and clears `?sort=` entirely; the gallery falls back to the next resolution step (stored default → builtin). The chip itself is the remove affordance — no separate X button.
- `app/lib/sortSpec.ts` — pure TS parser mirroring the Go `sortspec` package. Same grammar, same whitelist constant.

### Modified files

- `app/components/pages/Home.tsx`:
  - Adds a `sort` param to its `useSearchParams()` handling.
  - Fetches `useLibrarySettings(libraryId)` and blocks gallery render until either (a) URL sort is present and parses, or (b) library settings query has resolved. Existing `useBooks()` call gets the resolved sort as an input to its query key so changes re-fetch.
  - Renders `SortedByChips` when the resolved sort differs from the stored/builtin default.
  - Adds the Sort button to the toolbar (mirrors Filters button).

- `app/components/library/FilterSheet.tsx`:
  - No functional changes. Its structure is the reference.

### Builtin default

The frontend's builtin default — used when the URL has no `sort` param and the DB has no saved default — is `date_added:desc`. Defined as a constant in `app/lib/sortSpec.ts`.

### Cache invalidation

- Saving a library sort default → invalidate `[QueryKey.LibrarySettings, libraryId]`. (No need to invalidate books; the displayed sort is unchanged.)
- Changing the URL sort → `useBooks()` re-fetches because its query key includes the resolved sort.

## Migration

Single migration, `pkg/migrations/20260419000000_add_user_library_settings.go`:

```go
package migrations

import (
    "context"

    "github.com/pkg/errors"
    "github.com/uptrace/bun"
)

func init() {
    up := func(_ context.Context, db *bun.DB) error {
        _, err := db.Exec(`
            CREATE TABLE user_library_settings (
                id          INTEGER PRIMARY KEY AUTOINCREMENT,
                created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                user_id     INTEGER NOT NULL REFERENCES users     (id) ON DELETE CASCADE,
                library_id  INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
                sort_spec   TEXT
            )
        `)
        if err != nil {
            return errors.WithStack(err)
        }
        _, err = db.Exec(`CREATE UNIQUE INDEX ux_user_library_settings ON user_library_settings (user_id, library_id)`)
        return errors.WithStack(err)
    }

    down := func(_ context.Context, db *bun.DB) error {
        _, err := db.Exec(`DROP INDEX IF EXISTS ux_user_library_settings`)
        if err != nil {
            return errors.WithStack(err)
        }
        _, err = db.Exec(`DROP TABLE IF EXISTS user_library_settings`)
        return errors.WithStack(err)
    }

    Migrations.MustRegister(up, down)
}
```

No data backfill needed — absence of a row means "use builtin default", which is the same thing every existing user sees today.

## Testing Plan

### Go

- `pkg/sortspec/spec_test.go` — Parse: valid inputs (all whitelist fields × both directions, multi-level specs, extra whitespace rejected), invalid inputs (unknown field, bad direction, duplicate field, >10 levels, empty parts).
- `pkg/sortspec/sql_test.go` — `OrderExpressions` produces the expected list for: single level, multi-level, series (expanded to two), primary-file-preferred fields (COALESCE shape).
- `pkg/sortspec/resolve_test.go` — `ResolveForLibrary`: explicit takes precedence over stored; stored wins when explicit is empty; returns nil when both are empty; invalid stored spec falls through (returns nil, doesn't 500).
- `pkg/books/service_test.go` — `ListBooksWithTotal` with `Sort` set honors the order; with `Sort` nil preserves the current default; series spec sorts `#1` before `#2` regardless of series-name direction; primary-file-preferred fields fall back to any other file when primary is NULL.
- `pkg/books/handlers_test.go` — `list` handler: explicit `?sort=` wins; no `?sort=` + stored preference applied; no `?sort=` + no preference → default; no `library_id` filter skips the DB lookup.
- `pkg/settings/service_test.go` — `GetLibrarySettings` returns zero row for missing, real row when present; `UpsertLibrarySort` inserts then updates; user delete cascades; library delete cascades.
- `pkg/settings/handlers_test.go` — Library-access check returns 403 for inaccessible libraries; invalid sort spec returns 400.
- `pkg/opds/service_test.go` — Each of the seven library-scoped feeds (library, recently-added, author, series, genre, tag, and the library-filtered root) honors the stored preference when present and the hard-coded default when absent. Cross-library root feed ignores the preference.
- `pkg/ereader/handlers_test.go` — Each of the four book-list handlers (library index, author, series, genre) honors the stored preference for the resolved `apiKey.UserID`, and falls back to the hard-coded default when absent.

All Go tests use `t.Parallel()` per project convention except any that mutate global config.

### Frontend

- `app/lib/sortSpec.test.ts` — round-trips parse ↔ serialize; rejects same inputs the Go parser does; whitelist constant matches the Go side (maintained manually — failing test if they drift).
- `app/components/library/SortSheet.test.tsx` — adding/removing levels, drag-reorder updates the URL, direction toggle updates the URL, "Save as default" fires the mutation and clears the URL.
- `app/components/library/SortedByChips.test.tsx` — clicking a chip removes that level from the URL; removing the last level clears `?sort=` and hides the chip bar; chip labels render with contextual direction text (A → Z, Newest / Oldest, etc.).
- `e2e/gallery-sort.spec.ts` — Playwright E2E covering: default load → change sort → URL updates → dot appears → save as default → URL clears, dot disappears → reload → saved sort renders → reset → builtin renders. Runs in both Chromium and Firefox per project convention.

### Docs

No user-facing docs page for v1 (sort is a gallery affordance, not a config knob). If v2 adds a global default in Settings, the Settings docs page picks it up then. No update to `website/docs/` required for this change.

## Implementation Task Outline

Detail lives in the separate implementation plan; high-level order:

1. Migration — create `user_library_settings` table + unique index. Verify up/down.
2. Bun model — `pkg/models/user_library_settings.go`.
3. `pkg/settings` — service methods for library settings (needed before the resolver). Unit tests.
4. `pkg/sortspec` package — parser, whitelist, SQL expressions, `ResolveForLibrary`. Unit tests.
5. `pkg/books` — wire `Sort` through `ListBooksQuery` → `ListBooksOptions` → query; call `ResolveForLibrary` in the handler. Unit + handler tests.
6. `pkg/settings` — new `GET/PUT /settings/libraries/:library_id` handlers + route registration. Handler tests.
7. `pkg/opds` — update each of the seven library-scoped feed handlers to call `ResolveForLibrary` before `ListBooks`. Integration tests.
8. `pkg/ereader` — update each of the four book-list handlers to call `ResolveForLibrary` using `apiKey.UserID`. Integration tests.
9. Tygo regen — `mise tygo` to update `app/types/generated/`.
10. `app/lib/sortSpec.ts` — TS parser + builtin default constant. Unit tests.
11. `app/hooks/queries/librarySettings.ts` — TanStack Query hooks.
12. `app/components/library/SortedByChips.tsx` + `SortSheet.tsx`.
13. `Home.tsx` — wire sort state, Sort button, block-on-settings-load, chips.
14. E2E test.
15. Final check pass (`mise check:quiet`).

## Resolved UX Decisions

Captured here for posterity so implementation doesn't accidentally re-litigate them:

- **Filters button:** icon-only (unchanged from today). No "Filters" text label.
- **Clicking a "Sorted by" chip:** removes that level from the URL. Parallels the "click a filter chip to remove the filter" interaction in `ActiveFilterChips`. UX consistency with filter chips outweighs the discoverability of opening the sheet.
- **Direction labels:** contextual (A → Z / Z → A, Newest / Oldest, Shortest / Longest). Generic `↑ / ↓` rejected as less readable.
- **Save-as-default scope:** per-library only for v1 (this design). A global "always sort this way" preference could be added later as a column on `user_settings`; no schema changes needed in `user_library_settings` to support that.

## Open UX Questions (non-blocking)

These are polish details that can be adjusted during implementation without any schema or API changes.

1. **Direction-label sizing in the sort sheet.** The contextual labels ("A → Z", "Shortest / Longest") are wider than a simple "↑ / ↓" glyph, and they sit next to the field dropdown. There's a concern the two controls will feel cramped on narrow sheet widths (especially mobile drawer). Plan: build it with contextual labels, visual-check during implementation, and fall back to truncated labels or an icon-only variant *inside the sheet only* if it's noticeably tight. Chip labels stay contextual regardless.
