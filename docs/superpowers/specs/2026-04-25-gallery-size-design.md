# Gallery Size — Per-User Cover Sizing

## Problem

The book gallery renders covers at a fixed width (`sm:w-32`, 128px), producing roughly 8×3 books per page on a wide desktop. That density works for skimming a large library but is too small when the user wants to study covers (illustrated comics, art books, audiobook art, etc.). Users have no way to make covers larger or smaller.

## Goal

Let each user pick their preferred gallery cover size — Small, Medium, Large, or Extra Large — applied across every gallery page they see. Changeable on-the-fly via a toolbar popover, persistable as their saved default, and also editable from User Settings.

Sort's existing "Save as default" copy is bundled into this PR to fix an adjacent UX gap (the current wording reads as if it sets a library-wide default for all users; it actually only sets the current user's default).

## Non-goals

- Per-library size overrides. (User-global only. Trade-off documented in Decisions.)
- Sizing for non-book gallery pages (Genres list, Tags list, People list, Publishers list, Imprints list — these are text rows, not cover grids).
- Virtualized scrolling or infinite scroll.
- Mobile (sub-`sm:`) sizing — covers stay 2-col on mobile regardless of saved size; the popover is hidden.
- Server-side cover thumbnailing. Source covers are large enough that 224px display works without resampling.

## Decisions

### Scope: per-user, global

Stored on the existing `user_settings` row (one per user). The same value applies to every gallery page the user views.

Trade-off considered and accepted: a user with both a comics library and a text library can't have different defaults for each. Mitigated by the in-toolbar popover + URL-based ad-hoc state — a user can switch sizes per-session without overwriting their saved default.

### Sizes and widths

Four named sizes. Width is the cover container width applied at `sm:` (640px) and above. Below `sm:`, the existing `w-[calc(50%-0.5rem)]` mobile rule continues to apply (forced 2-col), so size has no visual effect on mobile.

| Size | Tailwind | Width |
|------|----------|-------|
| S    | `sm:w-24` | 96px  |
| M    | `sm:w-32` | 128px (current default) |
| L    | `sm:w-44` | 176px |
| XL   | `sm:w-56` | 224px |

Default is **M** so existing users see no visual change.

Cover height continues to be derived from `library.cover_aspect_ratio` (book = 2:3, audiobook = 1:1). Width-driven sizing keeps the existing aspect-ratio system untouched.

### Items per page

Items per page scales with size to keep perceived screen density roughly constant:

| Size | Items/page |
|------|------------|
| S    | 48         |
| M    | 24 (current) |
| L    | 16         |
| XL   | 12         |

Backend's max list limit is 50, so S=48 is within bounds. The constants file includes a comment guarding against bumps that would clip silently.

### Pagination preservation on size change

When the user switches size mid-page, the new page lands at or before their current first-visible item:

```
new_page = floor(old_offset / new_limit) + 1
```

Example: at M (limit 24) on page 5, `old_offset = 96`. Switching to S (limit 48): `new_page = floor(96/48) + 1 = 3`. Page 3 covers offsets 96–143, so the user sees their first-visible book at the top of the new page.

Same direction-agnostic behavior holds for any size change — the new page always starts at offset `<= old_offset`, so the user never "skips ahead" past books they hadn't seen.

### Storage

Add `gallery_size` column to `user_settings`:

- Type: `TEXT NOT NULL DEFAULT 'm'`
- Allowed values: `'s' | 'm' | 'l' | 'xl'`
- Tygo emits a TS literal `GallerySize` type, mirroring how `EpubTheme` is currently emitted from `pkg/models/user_settings.go`.

### API endpoint rename

The current `/settings/viewer` endpoint reads/writes the `user_settings` row but is named for the EPUB viewer because that was the only field stored. Adding `gallery_size` makes the name semantically wrong.

Rename in this PR:
- Backend route `/settings/viewer` → `/settings/user`
- Files `pkg/settings/viewer_handlers.go` → `user_handlers.go`, `viewer_service_test.go` → `user_service_test.go`, `viewer_handlers_test.go` → `user_handlers_test.go`
- Validator types `ViewerSettingsPayload` → `UserSettingsPayload`, `ViewerSettingsResponse` → `UserSettingsResponse`
- Frontend hook `useViewerSettings` → `useUserSettings`, mutation `useUpdateViewerSettings` → `useUpdateUserSettings`, query key `ViewerSettings` → `UserSettings`
- Interface `ViewerSettings` → `UserSettings`

Existing field names within the row (`viewer_epub_*`) keep their `viewer_` prefix — those genuinely are viewer fields, the rename is only about the row-level wrapper.

### Toolbar control: SizePopover

A new popover-triggered control in the gallery toolbar, placed alongside Sort/Filter:

- **Trigger:** small button with a "size" icon and text label "Size", showing a dirty dot when the active size differs from the user's saved default. Mirrors `SortButton`'s pattern.
- **Popover content:** a row of four segmented buttons labeled S / M / L / XL. The active button is highlighted. Clicking a button updates the URL `?size=` param immediately (ad-hoc state).
- **Save-as-default affordance:** when the active size differs from the saved default, a dashed-bordered card appears below the buttons with:
  - Body text: "Other users won't be affected."
  - Button: "Save as my default everywhere"
- **Mobile:** popover trigger is `hidden sm:flex`. Below `sm:`, the user can only edit via the User Settings page (since size has no visual effect on mobile anyway).

### URL parameter

`?size=s|m|l|xl` carries ad-hoc state on a single page. Resolution order on every gallery page:

1. Valid `?size=` URL param wins
2. Else user's saved `gallery_size` from `user_settings`
3. Else default `'m'`

Invalid values fall through silently to step 2 (matches sort's resolution behavior).

The URL param is **per-page** — clicking a `<Link>` to another page does not carry it. To make a change persistent across pages, the user clicks "Save as my default everywhere".

When the user saves a new default, the `?size=` URL param is cleared (since it now matches saved), matching how `handleSaveSortAsDefault` clears `?sort=`.

### Settings load gating

Each gallery page that reads `gallery_size` must wait for `useUserSettings()` to resolve before rendering, or the user with a saved L would see a flash of M-sized covers. The library home page additionally waits for `librarySettingsQuery` (the existing sort gate). Both must resolve; `Loader2` spinner during the wait, identical to today's behavior.

### Sort copy fix (bundled)

The existing `SortSheet` "Save as default" card reads:

> "Save this as the default for this library?"
> [Save as default]

…which is ambiguous about per-user vs. library-wide. Updated copy in this PR:

> "Other users won't be affected."
> [Save as my default for this library]

Same `<Save>` icon, same dirty-state gating, same handler. Two lines of JSX.

### User Settings page integration

`UserSettings.tsx` already has an **Appearance** section (theme picker). Add Gallery Size to the same section, below the theme row:

- Label: "Gallery cover size"
- Control: same four segmented buttons as the popover, reused as a shared `SizeSelector` component
- Wired to `useUserSettings()` + `useUpdateUserSettings()`

No saved-as-default affordance needed here — every change persists immediately, the page is the settings UI itself.

## Scope of pages affected

Apply the size + items-per-page logic to every page that renders the book Gallery:

- `app/components/pages/Home.tsx` (library books gallery)
- `app/components/pages/SeriesList.tsx`
- `app/components/pages/SeriesDetail.tsx`
- `app/components/pages/GenreDetail.tsx`
- `app/components/pages/TagDetail.tsx`
- `app/components/pages/PersonDetail.tsx`
- `app/components/pages/ListDetail.tsx`

(Cross-checked: these are the call sites of `<Gallery>` rendering book items. The text list pages — GenresList, TagsList, PersonList, PublishersList, ImprintsList — are excluded; they have no covers.)

## Architecture sketch

### New constants and utils

`app/constants/gallerySize.ts`:
```ts
export const GALLERY_SIZES = ["s", "m", "l", "xl"] as const;
export type GallerySize = (typeof GALLERY_SIZES)[number];
export const DEFAULT_GALLERY_SIZE: GallerySize = "m";

// sm: applies at >= 640px; below sm: w-[calc(50%-0.5rem)] (forced 2-col) takes over.
export const COVER_WIDTH_CLASS: Record<GallerySize, string> = {
  s: "sm:w-24",
  m: "sm:w-32",
  l: "sm:w-44",
  xl: "sm:w-56",
};

// Backend max limit is 50. If you increase any of these to >50, the API will
// silently clip and pagination will break.
export const ITEMS_PER_PAGE_BY_SIZE: Record<GallerySize, number> = {
  s: 48,
  m: 24,
  l: 16,
  xl: 12,
};
```

`app/libraries/gallerySize.ts` (pure functions, easy to unit-test):
```ts
export const parseGallerySize = (raw: string | null): GallerySize | null => { ... };
export const pageForSizeChange = (oldOffset: number, newLimit: number): number => { ... };
```

### New components

- `app/components/library/SizePopover.tsx` — popover with 4 segmented buttons + dirty-state save-as-default card. Same desktop-popover / mobile-drawer split as `SortSheet`, except the popover is hidden below `sm:` so we can use a plain Popover and skip the drawer entirely.
- `app/components/library/SizeButton.tsx` — toolbar trigger with dirty dot. Mirrors `SortButton`.
- `app/components/library/SizeSelector.tsx` — the four segmented buttons themselves, reused by both `SizePopover` and `UserSettings.tsx`.

### Hook rename

`app/hooks/queries/settings.ts`:
- `ViewerSettings` interface → `UserSettings`
- `ViewerSettings` enum value → `UserSettings`
- `useViewerSettings` → `useUserSettings`
- `useUpdateViewerSettings` → `useUpdateUserSettings`
- Add `gallery_size: GallerySize` to interface and update payload
- Update endpoint URLs `"/settings/viewer"` → `"/settings/user"`
- Update all callers (EPUB viewer + new gallery pages)

### Page-level wiring (representative diff for Home.tsx)

```ts
const userSettingsQuery = useUserSettings();
const updateUserSettings = useUpdateUserSettings();

const urlSize = parseGallerySize(searchParams.get("size"));
const savedSize = userSettingsQuery.data?.gallery_size ?? DEFAULT_GALLERY_SIZE;
const effectiveSize: GallerySize = urlSize ?? savedSize;
const isSizeDirty = urlSize !== null && urlSize !== savedSize;

const itemsPerPage = ITEMS_PER_PAGE_BY_SIZE[effectiveSize];

// gate render — both library + user settings must resolve
const settingsResolved =
  (libraryIdNum === undefined || librarySettingsQuery.isSuccess || librarySettingsQuery.isError) &&
  (userSettingsQuery.isSuccess || userSettingsQuery.isError);

// Apply size change; preserve position via floor-page math
const applyGallerySize = (next: GallerySize) => {
  setSearchParams((prev) => {
    const params = new URLSearchParams(prev);
    if (next === savedSize) {
      params.delete("size");
    } else {
      params.set("size", next);
    }
    const newPage = pageForSizeChange(offset, ITEMS_PER_PAGE_BY_SIZE[next]);
    params.set("page", String(newPage));
    return params;
  });
};

const handleSaveSizeAsDefault = () => {
  updateUserSettings.mutate(
    { gallery_size: effectiveSize },
    { onSuccess: () => setSearchParams((prev) => {
        const params = new URLSearchParams(prev);
        params.delete("size");
        return params;
      }) }
  );
};
```

`<Gallery itemsPerPage={itemsPerPage} ... />` and the renderer passes `gallerySize={effectiveSize}` down through `SelectableBookItem` → `BookItem`.

### BookItem cover width

Replace the hard-coded `sm:w-32` with a lookup:

```ts
className={cn(
  "w-[calc(50%-0.5rem)] group/card relative",
  COVER_WIDTH_CLASS[gallerySize],
  isSelectionMode && "cursor-pointer",
)}
```

`gallerySize` becomes a prop on `BookItem` and `SelectableBookItem`, defaulting to `'m'` for any caller that doesn't pass it (keeps existing test fixtures and any future caller from breaking).

## Testing

### Backend

- `pkg/settings/user_handlers_test.go` (renamed from viewer): add cases for valid `gallery_size` accepted, invalid values rejected, partial updates, default returned on missing row.
- Migration up/down round-trip is covered by the standard migration test harness.

### Frontend (unit)

- `app/libraries/gallerySize.test.ts`:
  - `parseGallerySize` accepts `s|m|l|xl`, returns null for `"medium"`, `""`, `null`, `"S"` (case-sensitive — same as sort).
  - `pageForSizeChange` math: known offsets and limits produce expected pages, including the edge cases at offset 0 and offset == old_limit.
- `app/components/library/SizePopover.test.tsx`:
  - Renders four segmented buttons with active highlight on the effective size.
  - Save-as-default card is hidden when `effectiveSize === savedSize`.
  - Save-as-default card is shown when sizes differ; clicking the button calls the save handler.
  - Helper text "Other users won't be affected." is present.
- `app/components/pages/UserSettings.test.tsx`: gallery size selector renders four buttons, picking one calls the update mutation with the right payload.

### E2E

Out of scope — the existing sort flow has no E2E coverage either, and the unit tests above plus the rename test cover the API path well enough.

## Documentation

- New page `website/docs/gallery-size.md`, mirroring `gallery-sort.md`'s shape: Overview, How to use, URL parameter, Saved default. Cross-link to it from the existing `gallery-sort.md`.
- Update `website/sidebars.ts` to include the new page.
- No `configuration.md` update — gallery size is a per-user UI preference, not a server config field.

## Risks and open questions

None outstanding. Two areas to be careful about during implementation:

1. **API rename touches the EPUB viewer.** The frontend `useViewerSettings` hook is consumed by viewer pages. Must update those import sites at the same time as the rename, or builds break.
2. **Settings gate AND on Home.** Home already gates on `librarySettingsQuery`; adding `userSettingsQuery` to that gate must use AND, not OR — else the gallery flashes default-M before the saved size loads.
