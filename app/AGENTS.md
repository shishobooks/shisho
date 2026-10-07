# Shisho Frontend Development

React 19, TypeScript, Tailwind, TanStack Query, Vite, and Radix UI with shadcn/ui patterns. Server state lives in TanStack Query; there is no global client state library.

**Bar for additions.** Add a rule here only if it applies beyond a single fix and neither the code nor a check can convey it. Prefer writing a check (an ESLint rule or a test) over a sentence. Detail for one area goes in its topic doc under `docs/agents/frontend/`, review judgement in `docs/agents/standards/frontend.md`, and history in the commit message.

## Enforced by checks

Each line is a rule a check already fails on; the check's message or test says how to comply.

- Cover, page, download, and stream URLs come from the builders in `app/utils` (ESLint rejects a literal `/api/.../cover`, `/page/`, `/download`, or `/stream` URL elsewhere).
- Permissions are checked through typed requirements (ESLint rejects `hasPermission(...)` with string literals).
- Queries go through a gated hook in `app/hooks/queries` (ESLint rejects importing `useQuery`/`useQueries` and the other query hooks, or calling `fetchQuery`/`prefetchQuery`/`ensureQueryData`, anywhere else outside tests).
- Every `mutate()` passes `onError`, and every `mutateAsync` promise is caught or handed to a caller that catches it (ESLint, plus `eslint-rules/mutate-async-handled.js`; both reject destructured, aliased, or uncalled `mutate`/`mutateAsync` references).
- Every query hook gates its route's permission (`app/hooks/queries/permissions.test.tsx` fails a new hook until it is added to `QUERY_HOOKS` and its gate matches).
- Dynamic class names compose with `cn()` from `@/libraries/utils`, not template literals (enforced by ESLint).
- Clipboard writes go through `copyText` (`app/utils/clipboard.ts`), not `navigator.clipboard`, which is undefined over plain HTTP on a LAN (enforced by ESLint). `copyText` returns `false` on failure: toast an error then, not a success.
- Relative times use `formatDistanceToNow(date, { addSuffix: true })` from `date-fns`, not a hand-appended "ago" (enforced by ESLint).
- 404 checks use `isNotFoundError`/`isLoadFailure`, not an inline `status === 404` (enforced by ESLint).
- `<Tabs>` never takes `defaultValue` (enforced by ESLint). Tab state held in `useState` is not checked; tabs stay deep-linked through the URL (see `docs/agents/standards/frontend.md`).

`app/eslint-rules.test.ts` pins the local rules, including their known bypasses: a passing lint is a tripwire, not a guarantee.

## API types

Every request and response shape is generated from Go by tygo into `app/types/generated/` (re-exported from `@/types`). Never hand-define a type with a Go counterpart: fix the Go struct in its package's `types.go` and run `mise tygo`. See ADR 0004 (`docs/adr/0004-tygo-generated-api-types.md`).

- Query hooks type their return as the generated `{Entity}Response` (or `List{Entities}Response` for a list), not the bare model. A response may reshape a relation (`GenreResponse.aliases` is `string[]`); read it as typed, with no `as unknown as` cast and no `.map((a) => a.name)`. `app/hooks/queries/genres.ts` and `publishers.ts` (distinct `PublisherListItem` and `PublisherResponse`) are the references.
- `ResourceListResponse<T>` (`app/types/index.ts`) is only the generic `{ items, total }` prop type for `ResourceList`, `BookGallerySection`, and `FileListSection`. Some hooks still type envelopes with it; new hooks use the generated envelope.
- Re-export a `List*Response` from the barrel only when its envelope differs from `{ items, total }` and a hook types its return on it.

HTTP client functions live in `app/libraries/api.ts`; query and mutation hooks wrap them in `app/hooks/queries/`.

## Request errors

- Report a failed request with `toastRequestError(error, fallback)` from `@/libraries/api`, where `fallback` names the action (`"Failed to delete book"`). It shows the server's message when there is one. Inline error text uses `requestErrorMessage(error, fallback)`. Both stay silent on Demo Mode rejections, which `checkStatus` already toasted. Plain `toast.error` is only for client-side validation.
- Render a failed query with `<QueryError query={q} fallback="Failed to load X" />` from `@/components/library/QueryError`, only when `q.error && !q.data`. A query with neither data nor error is waiting on its permission gate: show the spinner or nothing.
- Detail pages keep their Not Found page for a 404 and branch on `isLoadFailure(q)` for any other failure.
- A failed save keeps the dialog open and the draft intact.

Where `QueryError` goes on each page type, what `checkStatus` and the retry policy do, and the mutation hand-off rules: `docs/agents/frontend/request-errors.md`.

## Permissions

Two layers, both built on the typed `Requirement` (one permission, an array for all of them, or `anyOf(...)`):

- **Query hooks gate themselves** with `useRequires(requirement, enabled)` from `app/hooks/queries/permissions.ts`, set after spreading caller options. Callers pass `enabled` only for their own state (`enabled: open` for a dialog), never a restated permission.
- **Components hide controls** with `useCan(requirement)` from `@/hooks/useCan`, called before any early return; use `can` from `useAuth()` where a hook cannot run (loops, callbacks, props). Gate on the permission the backend route requires, not the page type.

Hide a control the role cannot use, and skip mounting the dialogs behind it; do not disable it. The exception is a settings form a role can read but not save: disabled inputs, `<ReadOnlyNotice />`, and no Save button. Action menus gate each entry separately.

The control-to-permission table, what is never gated (list membership, selection, downloads), route and nav guards, library data for roles without Libraries Read, and permission tests: `docs/agents/frontend/permissions.md`.

## Forms

Every form that creates or updates data has unsaved changes protection: dialogs use `FormDialog` (`@/components/ui/form-dialog`) with `useFormDialogClose` to close after a save, and pages use `useUnsavedChanges` (`@/hooks/useUnsavedChanges`) with `UnsavedChangesDialog`. Both compare current state to an `initialValues` snapshot and reset the snapshot after a successful save. Wiring for child editors and tab switches: `docs/agents/frontend/forms.md`.

## Testing

- Unit and component tests are colocated as `*.test.ts(x)` and run with Vitest and Testing Library (`mise test:unit`). E2E patterns live in `e2e/AGENTS.md`.
- Anything that calls `useAuth()` throws outside `AuthProvider`. Use the shared mock and set the role per test; never hand-write a `useAuth` mock:

  ```ts
  vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
  beforeEach(() => setAuth({ permissions: ["books:read", "books:write"] }));
  ```

- `vitest.setup.ts` turns on fake timers globally (`shouldAdvanceTime: true`), so `userEvent.setup()` needs `{ advanceTimers: vi.advanceTimersByTime }` or clicks and typing stall under load.
- `testTimeout` is 15s because `mise check:quiet` runs the unit suite beside the Go tests, linters, and e2e. Keep it. A test that is slow on an idle machine gets cheaper (render one card, not a full page); a load-induced timeout is not fixed with a per-test timeout.
- Mutation failure helpers (`rejectingMutate`, `watchUnhandledRejections`) are in `app/testing/mutations.ts`.

## Gotchas

- **Image URLs need `?v=` even with `immutable` caching.** Browsers keep an in-memory image cache separate from the HTTP cache and reuse the decoded bitmap for any `src` already rendered this session, so a changed image needs a changed URL. Components take the model (book, series, file) and call the builder; render server covers with `CoverImage`, keyed `key={coverUrl}` where the image can change while mounted. Detail: `docs/agents/frontend/image-urls.md`.
- **A custom component used as a Radix `asChild` trigger must `forwardRef` and spread props** onto the underlying `Button`, with `displayName` set (see `SizeButton`, `SortButton`, `FilterButton`). Without the ref, floating content (`Popover`, `DropdownMenu`, `Tooltip`) anchors at `(0, 0)` and renders off-screen, and `Dialog`/`Sheet` cannot restore focus. jsdom runs no positioning, so unit tests pass; only a real browser shows it. A bare `<Button>` child is already forwardRef'd.
- **Sortable lists need stable client-side row keys.** Index and content keys change on reorder and break dnd-kit's drag tracking (flicker, dropped drags, edits landing on the wrong row). Assign an id when a row enters the list and keep it: `FileChaptersTab` uses a monotonic counter (`nextEditKey()`), and `SortableEntityList` keys a `WeakMap` by item reference, so its callers must pass stable references (`useState` or `useMemo`, as `IdentifyReviewForm`'s `narratorItems` does). Server ids work only when every row, including unsaved ones, has one.
- **CBZ and PDF page numbers are 0-indexed in storage and the API, 1-indexed on screen.** Convert at the display edge (`ChapterRow`, `PagePicker`, `CBZReader`, the uncovered-pages warning), never in what you send back.
- **Dialogs focus their container, not the header close button.** `DialogContent` does this when the first tabbable element carries `data-dialog-header-close`; otherwise Radix focuses the first field. A caller's `onOpenAutoFocus` runs first and wins with `preventDefault()`. Close buttons keep `focus-visible:` rings.
- **A dialog opened from a dropdown item**: the `Dialog` wrapper in `app/components/ui/dialog.tsx` already clears the `pointer-events: none` Radix can leave on `<body>`. Give the `DropdownMenuContent` `onCloseAutoFocus={(e) => e.preventDefault()}` so the two focus managers do not fight.
- **File labels come from the server.** Render `fileLabel(file)` from `@/utils/format` (the Go-resolved `display_name`); a supplement's label is its filename, and the Share Link payload has no path to rebuild one from.
- **`Button` defaults `type="button"`**, so a form's submit button must say `type="submit"`. Its kit also covers one-offs: `size="icon-sm"`/`"icon-xs"` for small icon buttons, `variant="link"` for inline text links (no `h-auto p-0` needed), `variant="unstyled"` for rows and tiles, and `BadgeRemoveButton` for the X in a `Badge`.
- New shadcn components are added with `npx shadcn@latest add`.

## Topic docs

- `docs/agents/frontend/request-errors.md`: read when reporting a failed request or query, placing `QueryError` on a page, handing a mutation promise to a caller, or calling `fetch` directly.
- `docs/agents/frontend/permissions.md`: read when adding a mutating control, query hook, route, nav entry, or entity link, or when a test depends on the role.
- `docs/agents/frontend/forms.md`: read when building a form dialog, a settings or edit page, or a child editor with its own Save.
- `docs/agents/frontend/image-urls.md`: read when rendering a cover or page image, adding a cover-changing action, or building a download or stream link.
- `docs/agents/frontend/demo-mode.md`: read when touching write-error reporting, controls hidden in Demo Mode, or stored preferences.
- `docs/agents/frontend/share-links.md`: read when changing `BookDetailBody`, the share dialog, or the `/share/:token` page.
- `docs/agents/frontend/audio-playback.md`: read when changing the M4B player, the chapter audio preview, or `app/utils/audioCodec.ts`.
- `app/components/layout/AGENTS.md`: read when touching the top nav, sidebars, `UserMenu`, or the Demo Mode banner.
- Reviewers: `docs/agents/standards/frontend.md` holds the frontend judgement rules (tokens, layout, mobile, long text, tabs, page titles, forms, cache invalidation).
