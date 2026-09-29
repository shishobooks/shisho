# Shisho Frontend Development

This file documents frontend patterns and conventions specific to Shisho.

## Stack

- React 19 with TypeScript
- TailwindCSS for styling (dark/light theme support)
- Tanstack Query for server state
- Vite for bundling
- Radix UI primitives with shadcn/ui patterns

## Design Tokens

Use semantic color tokens exclusively. Never use hardcoded Tailwind colors (`dark:bg-neutral-*`, `dark:text-violet-*`, `text-gray-*`, `bg-neutral-*`). The CSS variables in the theme already handle dark mode; manual `dark:` color overrides cause drift.

| Pattern | Classes |
|---------|---------|
| Page titles (all pages) | `text-2xl font-semibold` |
| Dialog titles | `text-sm font-semibold` |
| Page header margin | `mb-6 md:mb-8` |
| Card/section padding | `p-4 md:p-6` |
| Dialog body spacing | `space-y-6` |
| Border radius (page components) | `rounded-md` (not `rounded-lg`) |
| Hover backgrounds | `hover:bg-muted/50` |
| Selected card | `border-primary bg-primary/5` + `border-transparent` when unselected |
| Selected toggle chip | `border-primary bg-primary/5 text-primary` |
| Inset inline warning | `rounded-md bg-destructive/10 border border-destructive/20 p-3` |
| Full-width dialog error banner above footer | `shrink-0 border-t border-destructive/20 bg-destructive/10 px-5 py-3 text-sm text-destructive`, square edges and no side borders |
| Danger zone section | `space-y-3 rounded-md border border-destructive/40 p-4 md:p-6` with `text-lg font-semibold text-destructive` title (see `PluginDangerZone.tsx`) |
| Muted status badge | `bg-muted text-muted-foreground` |

## Architecture

### React Router (`app/router.tsx`)

- Single page app with client-side routing
- Main route loads Home page with book gallery

### State Management

- **Tanstack Query** for server state (books, jobs, libraries)
- **React Context** for theme management
- No global client state management library

### API Integration

- `app/libraries/api.ts` contains HTTP client functions
- Query hooks in `app/hooks/queries/` wrap API calls with Tanstack Query
- TypeScript types auto-imported from `app/types/generated/`

**Never hand-define a type that has a Go counterpart.** Every API request and response shape is generated from a Go struct via tygo into `app/types/generated/` (re-exported from `@/types`). The frontend imports those types; it does not restate them. If a needed type is missing or wrong, fix the Go struct in the package's `types.go` and run `mise tygo`, do not write it in TypeScript. See ADR 0004 (`docs/adr/0004-tygo-generated-api-types.md`).

- **Response types are `{Entity}Response`; list envelopes are the generated `List{Entities}Response`** (re-exported from `@/types`). Go names every list envelope `List{Entities}Response` (the root and `pkg/AGENTS.md` rule), and hooks type their return on it (users, roles, libraries, lists, list books, publishers, publisher files, tag books, jobs, job logs, logs). The list hooks in `genres.ts`, `series.ts`, `people.ts`, `tags.ts`, and `books.ts` still hand-type envelopes that have generated counterparts (`ResourceListResponse<...>`); they are pending migration to the generated `List{Entities}Response`, so do not copy them. `ResourceListResponse<T>` in `app/types/index.ts` stays only as the generic prop type for `ResourceList`, `BookGallerySection`, and `FileListSection`, which take any `{ items, total }` envelope. Query hooks type their return as the generated response type, not the bare model. A response struct embeds the model (TS `extends Genre`) and may reshape a relation: e.g. `GenreResponse` carries `aliases: string[]`, not the model `Genre`'s relation. Consume the response field directly (`genre.aliases`), never cast it (`as unknown as string[]`) and never call `.map((a) => a.name)` expecting relation objects. Reference: `useGenresList`/`useGenre`/`useUpdateGenre` in `app/hooks/queries/genres.ts` (typed `GenreResponse`), and `GenresList.tsx`/`GenreDetail.tsx` reading `aliases` as `string[]`. For a hierarchical entity with a distinct list vs detail shape, see publishers: `usePublishersList` is typed `ListPublishersResponse` (items are `PublisherListItem`), `usePublisher`/`useUpdatePublisher` are typed `PublisherResponse`; `PublishersList.tsx` reads `publisher.aliases` directly (no `.map((a) => a.name)`) and `PublisherDetail.tsx` reads `aliases`/`children`/`ancestors`/`descendant_ids` directly with no `as unknown as string[]` cast. Do not hand-define `PublisherDetail`/`PublisherListItem` in TS; import the generated types from `@/types`.

- **Do not re-export the per-entity `List{Entities}Response` from the barrel when the entity's list uses the generic `ResourceListResponse<{Entity}Response>` envelope.** The Go handler returns a named `List{Entities}Response` struct (so it is not a `map[string]any`) and tygo emits a TS mirror as a side effect, but the frontend consumes `ResourceListResponse<{Entity}Response>` for the trivial `{ items, total }` envelope and never imports the concrete `List*Response`. Re-exporting the unused interface invites importing a type the convention says not to use. Only re-export a `List*Response` when its envelope genuinely differs from `{ items, total }` (e.g. `ListPublisherFilesResponse`, `ListTagBooksResponse`, `ListListsResponse`, `ListUsersResponse`) and a hook actually types its return on it.

**IMPORTANT - List Limits:**
- **Default list limit is 50** - All list endpoints have a max limit of 50 items per request
- **Always use server-side search** - Never rely on client-side filtering for searchable lists; always pass search queries to the API. This ensures users can find items beyond the initial 50 loaded.

### Request errors and retries

- `ShishoAPI.checkStatus` always rejects with a `ShishoAPIError`, never a JSON `SyntaxError`. It reads the body as text and then tries `JSON.parse`, because a non-JSON body can only come from a reverse proxy or load balancer in front of the server. A 2xx with an empty or whitespace-only body (including `204`) resolves to `undefined`. A 2xx with a non-JSON body rejects, since every endpoint returns JSON or nothing. A non-2xx without the Go `{ error: { code, message } }` body rejects with a status-based message such as `Request failed with status 504 (Gateway Timeout)`. `ShishoAPIError.code` is `undefined` in those cases, so do not assume it is set. `app/libraries/api.test.ts` pins the exact messages.
- Code that must call `fetch` directly for a JSON endpoint (e.g. a `FormData` upload, which `API.request` would JSON-encode) passes the response to `API.checkStatus` instead of calling `response.json()` itself. `useUploadFileCover` is the example.
- The shared QueryClient does not retry `ShishoAPIError` responses with status `401`, `403`, `404`, or `422`. Other query failures retain the three-retry limit. Keep permission failures out of the retry path, including Demo Mode rejections.
- Async UI event handlers must consume mutation rejections and show an inline error or toast (except for Demo Mode rejections, see below). `BookEditDialog` shows metadata/review save errors inline and preserves its draft; the top-nav `ResyncButton` reports scan-creation failures with a toast.
- `CreateListDialog` treats a resolved `onCreate`/`onUpdate` promise as success. Parent callbacks that show an error toast must rethrow so the dialog stays open and retains unsaved-changes protection. Test these flows through their callers, not just the dialog with a rejecting stub: a caller swallowing the rejection is the failure to catch.

### Demo Mode

`useAuth()` exposes `demoMode`, sourced from the unauthenticated `GET /auth/status` response. Use this flag for Demo Mode UI behavior rather than checking the hostname or username.

`ShishoAPI.checkStatus` is the only place that reports a Demo Mode rejection. On a `403` with the `demo_mode` code it shows the toast `This action is unavailable in the demo.` (id `demo-mode`, so concurrent rejections collapse into one) before rejecting. Callers still receive the rejection, so dialogs stay open and keep their drafts, but they must not report it again:

- Toast request failures with `toastRequestError(error, message)` from `@/libraries/api`, not `toast.error`. It shows `message` unless `isDemoModeError(error)`. Plain `toast.error` is for client-side validation that never reached the server.
- Inline error UI (the `BookEditDialog` and `FileEditDialog` banners, the `MetadataEditDialog` and `PublisherEditDialog` server errors) skips a Demo Mode rejection with `isDemoModeError(error)` and renders every other error as before.

Test a caller that toasts through the real `API` with a `demo_mode` 403 and a real `<Toaster />`, and count the visible messages; a mocked rejection never reaches `checkStatus`. A dialog that only renders an injected `onSave` rejection inline can be tested with a rejected `ShishoAPIError` directly.

Demo Mode hides these controls on top of the role-based hiding below: the file download button and format popover, the supplement download button, the bulk download action, the admin gear and mobile drawer admin entries, and the security settings route. Anything else a role may use stays visible and gets the toast. The backend remains the write boundary.

Preferences stay browser-local in Demo Mode. User settings use the `shisho-demo-user-settings` local storage key. Per-library settings use `shisho-demo-library-settings-{libraryId}`. The query hooks fetch server defaults first, merge stored values over those defaults, and write Demo Mode mutations to local storage and the TanStack Query cache without sending a write request.

### Permission-gated controls

Check permissions with `useCan(requirement)` from `@/hooks/useCan`, which takes the same typed `Requirement` as `useRequires`: `useCan("books:write")`, `useCan(["jobs:read", "jobs:write"])` for all of a list, or `useCan(anyOf("shares:read", "shares:write"))`. Where a hook cannot run once per check (after an early return, in a loop or callback, or for a requirement passed as a prop), use `can` from `useAuth()`, as `ProtectedRoute` and `useLibraryNavItems` do. Both are built on `meetsRequirement`, so a misspelled permission fails to compile. `useAuth()` exposes no untyped `hasPermission` or `canWrite`, and ESLint rejects any `hasPermission(...)` call with string literals (`app/eslint-rules.test.ts` pins that rule and the query rule below). Call `useCan` before any early return, and put it first in a `&&` chain (`useCan("people:read") && !isShareLink`) so it runs on every render.

Every control that fires a mutating request must be hidden when the user lacks the permission the backend route requires. Gate on the route's permission, not the page's display type:

| Control | Permission |
|---------|------------|
| Book and file metadata, covers, chapters, review state, Identify, rescan, merge, move, delete | `books:write` |
| Genre, tag, and publisher edit/merge/delete/set-child | `books:write` |
| Series edit/merge/delete | `series:write` |
| Person edit/merge/delete | `people:write` |
| Library settings and scans | `libraries:write`, or `jobs:read` and `jobs:write` (already gated) |

Use `writePermissionForEntity(entityType)` from `@/utils/permissions` for the metadata entity mapping; `ResourceDetail` applies it itself, so the genre, tag, and person pages need no extra gating of their buttons. `PublisherDetail` owns its own edit dialog (via `onEditClick`) and gates that dialog on the same mapping; `SeriesDetail` has its own header and gates on `series:write` directly. Each detail page also fetches its merge dialog's candidate list only for a role that can merge.

Do not gate on a write permission:

- **List membership.** Add to list, create list, and the per-list actions follow the list's own `permission` field (owner/manager/editor/viewer), never Books Write. `AddToListPopover`, `AddToListDialog`, and the `SelectionToolbar` add menu leave out lists the user can only view; saving from the dialog keeps the book in those lists.
- **Selection mode and downloads.** Selection stays available for lists and downloads; only merge, delete, and review actions inside `SelectionToolbar` are hidden.
- **Demo Mode.** `useCan` and `can` reflect role permissions only. Role-based hiding still applies in Demo Mode; Demo Mode hides only the extra controls listed above, and any other control the role can use relies on the backend rejection plus toast.

Hide the whole control rather than disabling it, and skip mounting the mutation dialogs behind it (`{canWriteBooks && <RescanDialog … />}`). `ReviewPanel` takes `readOnly` to show the reviewed state as a label with no switch. `FileChaptersTab` takes a required `canEdit` that suppresses every view-mode entry into editing: the empty-state Add Chapter and Fetch from Audible buttons, and the clickable uncovered-pages banner (rendered as a plain notice instead).

**Action menus gate each entry, not the menu.** The Book Detail action menu is built from entry groups where every entry carries its own `visible` flag, computed from the permission its backend route requires. The menu renders when at least one entry is visible, and separators appear only between non-empty groups. Do not wrap the whole menu in a single `books:write` check: a user can hold one entry's permission without another's (the Share entry needs a `shares` permission, not `books:write`). Add to list has no permission of its own, so it joins the menu whenever another entry puts the menu on screen and is otherwise a standalone button.

**Roles without Libraries Read.** Reader pages never call the Libraries Read routes. `useUserLibraries()` (`GET /user/libraries`, authenticated only) returns the signed-in user's accessible libraries as `LibrarySummary` rows (id, name, cover aspect ratio, download preference, organize flag, no paths), and `useUserLibrary(libraryId)` selects one from the same cache. Breadcrumbs, cover aspect ratio, KePub preference, the Kobo sync scope, and the Merge and Move dialogs (`BookDetailBody`, `SelectionToolbar`, `MergeBooksDialog`, `MergeIntoDialog`, `MoveFilesDialog` take `LibrarySummary`) all read it. `useLibrary`/`useLibraries` are for settings pages only: `LibrarySettings` (route needs `libraries:read` and `libraries:write`), `AdminLibraries`, and user access assignment in `CreateUser`/`UserDetail` (`GET /libraries` also accepts `users:write`, so a `users:read`-only role sees an empty picker). `LibraryRedirect` sends a role with Books Read to its first accessible library, a role with no libraries to `/settings/libraries` when it holds Libraries Read and to `/lists` otherwise, and a role without Books Read to `/lists`. The library picker, `MobileDrawer`, and `LibraryRedirect` read `useNavLibraries()` instead: the same cache, gated on Books Read because every library page needs it, and empty without it even when the Kobo sync scope has filled the cache. `LibraryListPicker` renders nothing and `MobileDrawer` lists no libraries without Books Read. The Kobo sync scope keeps `useUserLibraries()` so it works for every role. `LibraryBreadcrumbs` shows `libraryName`, or a "Library" placeholder while it loads; pages that hold a book pass `libraryQuery.data?.name ?? book.library?.name`. `LanguageCombobox` relies on `useLibraryLanguages` (Books Read) and still offers the curated languages and custom tags without it. `ShareListDialog` picks users from `useUserDirectory()` (`GET /users/directory`, authenticated only); the server does not register that route in Demo Mode, so the hook is off there and the dialog shows a notice instead of the picker.

**Pages and links a role cannot read.** Library routes in `router.tsx` pass `requiredPermission` to `ProtectedRoute` (an array means all of them): `books:read` for Home, book, file, reader, genre, tag, and publisher pages, plus `series:read` or `people:read` for series and people pages. `useLibraryNavItems` hides each entry its route would deny. `/settings` has no page of its own: `SettingsIndexRedirect` opens the first visible `useAdminNavItems` entry, and the gear in `TopNav` and `MobileDrawer` shows only when one is visible. Names that link to a series or person page render as plain text without `series:read` or `people:read` (`BookDetailBody`, `BookItem`, `FileDetailsTab`). `GlobalSearch` renders nothing without Books Read. `ListDetail` hides a list's books, sort, and covers without Books Read, since `/lists/:id/books` and covers are book data. `ResyncButton` needs `jobs:read` and `jobs:write`; `AdminReviewCriteria` hides Save without `config:write` and Recompute without both jobs permissions. `TopNav`'s mobile search toggle needs a library and Books Read. The plugin order in the Advanced plugin settings reads Books Read routes, so `AdvancedOrderSection` shows a notice to a Config Read role without Books Read. Route guards and the navigation hooks share `ROUTE_PERMISSIONS` from `@/utils/permissions`, so a nav entry cannot lead to Access Denied. `AdminLibraries` links a library's name only for Books Read, shows its Settings button only for `ROUTE_PERMISSIONS.librarySettings`, and opens a newly created default library only for Books Read.

Components and query hooks that call `useAuth()` throw outside `AuthProvider`. Their tests replace the module with the shared mock in `app/testing/auth.ts` and set the role per test with `setAuth`:

```ts
vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

beforeEach(() => setAuth({ permissions: ["books:read", "books:write"] }));

it("hides Edit for a read-only role", () => {
  setAuth({ permissions: ["books:read"] });
  // ...
});
```

`setAuth` also takes `demoMode` and `user` (`null` for signed out), and resets anything left out. State lives in the module for the whole test file, so a file whose tests need different roles calls `setAuth` in `beforeEach`; a file with one role may call it once at module scope. `ALL_PERMISSIONS` grants every permission, derived from the generated `Resource*` constants. `app/testing/` imports vitest, so `tsconfig.app.json` excludes it from the production build and `tsconfig.test.json` type-checks it; keep test-only code there. A test that renders a real `AuthContext.Provider` passes it `authValue()` (see `useSSE.test.ts`). Do not hand-write a `useAuth` mock.

**Test file suffixes.** `X.test.tsx` usually mocks the query modules a component uses. `X.permissions.test.tsx` renders it with the real query hooks and a spied `API.request`, and asserts which requests fire for a role or state, such as a closed dialog (`SecuritySettings.permissions.test.tsx`, `BookDetailBody.permissions.test.tsx`). One file may cover a family of pages that share a behavior: `MetadataDetail.permissions.test.tsx` checks the merge candidate list on every metadata detail page. Assert that no request fires rather than which `enabled` value a caller passed: the hook owns the permission gate, so a test that pins the caller's `enabled` pins a duplicate check.

### Query hooks gate their own permissions

Every query hook in `hooks/queries/` whose route needs a role permission checks it inside the hook, so a caller cannot forget. `useRequires(requirement, enabled)` from `hooks/queries/permissions.ts` returns `enabled` when the signed-in role meets the requirement and `false` otherwise. A requirement is one permission (`"books:read"`), an array the role must hold all of, or `anyOf(...)` for a route that accepts several. Set it after spreading caller options so a caller's `enabled: true` cannot bypass it:

```ts
return useQuery<Book, ShishoAPIError>({
  ...options,
  enabled: useRequires("books:read", options.enabled ?? Boolean(id)),
  queryKey: [QueryKey.RetrieveBook, id],
  queryFn: ...
});
```

Plugin routes are also absent in Demo Mode, so `plugins.ts` wraps the check in `usePluginRouteEnabled`, which is off in Demo Mode too. `useUserDirectory` is authenticated only and off in Demo Mode for the same reason.

`hooks/queries/permissions.test.tsx` enforces the rule. `QUERY_HOOKS` records each query hook's expected requirement (the backend route's permission) and arguments that enable it. The test renders every exported query hook in `hooks/queries/` (mutation hooks are recognized by `mutate` and skipped) and checks that it sends a request with exactly its required permissions (each alternative alone for `anyOf`), sends nothing with every permission except a required one, requests only authenticated-only paths with no permissions (`/auth/*`, `/settings/user`, `/lists`, `/lists/:id`, `/lists/:id/shares`, `/lists/templates`, `/user/api-keys`, `/user/libraries`, `/users/directory`, `/jobs/:id`, `/settings/libraries/:id`, `/events`, and the anonymous `/share/:token`), and in Demo Mode requests no route the server leaves unregistered there. A new query hook fails until you add it to `QUERY_HOOKS`, and then until its gate matches the recorded requirement. `GET /jobs/:id` also serves a bulk-download job to its creator, so `useJob` is not gated and `JobDetail`'s route requires Jobs Read instead. Query hooks call `useAuth()`, so a test that renders one needs an `AuthProvider` or the shared `@/testing/auth` mock.

ESLint forbids importing `useQuery`, `useQueries`, and the other query hooks from `@tanstack/react-query` outside `app/hooks/queries` (tests excepted), and calling `fetchQuery`, `prefetchQuery`, `ensureQueryData`, or their infinite forms there, so every query goes through a gated hook (`MergeBooksDialog` uses `useBooksByIds`). `refetch()` runs a query even while it is disabled, so a component that calls it by hand checks the result's `isEnabled` first (`AdminJobs`, `IdentifyBookDialog`, `FetchChaptersDialog`, `EPUBReader`).

Callers pass `enabled` only for their own state, never for a permission the hook already checks: `useSharingSettings({ enabled: !isShareLink })`, not a restated `anyOf`. A query behind a dialog passes `enabled: open`, so a closed dialog sends nothing; this matters most where a list mounts one dialog per row or card (`AddToListDialog` in `BookItem`, `KoboSetupDialog` in Security Settings).

`AuthProvider` and `Setup` call the `/auth/*` routes through `API.request` directly. They own the session state, and those routes need no permission, so there are no auth query hooks.

`AuthProvider` clears the shared query cache on login, logout, and `setAuthUser`, so the next user in a tab never sees the previous user's cached data, such as their accessible libraries.

### Book Detail body and Share Link context

`BookDetail.tsx` (the page) only fetches: it calls `useBook` and `useUserLibrary`, handles loading and not-found, renders `LibraryLayout` and breadcrumbs, and passes the book and library to `BookDetailBody` (`app/components/library/BookDetailBody.tsx`). The body owns everything else: cover, metadata, resource lists, the file list with download and read controls, the action menus, and the dialogs behind them. It takes a `Book` payload rather than calling `useBook` itself, so a parent can hand it data from any source.

Passing `shareLink` (a `ShareLinkContext`) switches the body into Share Link context, for a recipient who has no access to the library:

- Author, series, genre, tag, narrator, publisher, and file names render as plain text through `ResourceLink` (a `Link` when given a path, a `span` when given `null`).
- The book action menu, Add to list, review toggle, per-file menus, Select, and Read and Listen are hidden regardless of the viewer's permissions (`canWriteBooks` is forced false).
- The sort title, the Created/Updated/Library/File Path block, the per-file filename row, and each file's URL and identifiers are omitted. File labels still come from `display_name`, which the server resolves before blanking paths, so a supplement shows its filename. A file whose only details are those hidden ones gets no expander.
- Downloads go to `shareLink.downloadUrl(file)` with the same HEAD-then-navigate flow; there is no format popover and no Download Original fallback.
- Covers use `shareLink.bookCoverUrl(book)` and `shareLink.fileCoverUrl(file)`. `FileCoverThumbnail` and `CoverGalleryTabs` accept the same `getCoverUrl` builder; returning `null` shows the placeholder.
- The plugin identifier types and sharing settings queries are disabled, so the body makes no authenticated requests in Share Link context.
- `shareLink.coverAspectRatio` stands in for the library's cover aspect ratio, which sizes the cover box.

New controls added to the body must decide how they behave in Share Link context. Anything that links into the app or mutates data must be hidden or rendered as plain text when `isShareLink` is true, and `BookDetailBody.test.tsx` should cover it.

### Share Links (sharer and recipient)

- **Share entry.** `useSharingSettings` fetches the sharing settings only for a role holding a `shares` operation or `config:read` (the endpoint's permissions), and `BookDetailBody` shows **Share** once the settings have loaded whenever the user holds either `shares` operation, even while sharing is off, so a sharer can revoke or delete a link without an admin turning sharing back on. Either one puts the action menu on screen, with Add to list beside Share. `ShareLinkDialog` takes `sharingEnabled`: while false it shows a notice in place of the new-link form, disables every copy button (no link works), and reworded revoke and delete confirmations, but keeps Revoke and Delete. The notice names Settings > Sharing, and links there (closing the dialog) when `canManageSharing`, which Book Detail sets from `config:write`, the permission the page needs. With sharing on it shows the new-link form for `shares:write` (its `canWrite` prop), each row's Revoke and Delete actions for `shares:write`, and the list for either operation (the list endpoint accepts both, so a sharer can always copy what they create). Every row shows opens, downloads, and last used. An active link with a `paused_reason` shows a muted **paused** badge and the reason, and cannot be copied. Revoke is offered on active links, paused ones included; Delete on every link. Both confirm through a `ConfirmDialog` rendered beside the `FormDialog`, not inside it, as `RoleDialog` does. The expiration presets are client-side; the dialog sends an absolute `expires_at`. The copy button builds `${window.location.origin}/share/<token>` and copies through `copyText` (`app/utils/clipboard.ts`), which falls back to `execCommand` because `navigator.clipboard` does not exist over plain HTTP on a LAN.
- **Recipient route.** `shareRoutes` (`app/components/pages/shareRoutes.ts`) mounts `SharedBook.tsx` at `/share/:token` plus a `/share/*` splat, so a truncated link shows the unavailable page instead of the router's error page. They are top-level public routes beside `/login` and `/setup`, outside `Root` and `ProtectedRoute`: no login redirect, no nav, no demo banner. It renders its own `<Toaster />` for download toasts, and a `ShareNotice` strip under the header (styled like the Demo Mode banner) saying who shared the book and when the link expires. It still sits inside `AuthProvider` (the body calls `useAuth`), which makes an anonymous `/auth/status` and `/auth/me` call. `useSSE` skips any `/share/` path, so the event stream never opens there, even for a signed-in user. A 404 renders the single `ShareUnavailable` page; any other failure is retried once and then shows a Try again page, since the link may still be fine.
- The share payload blanks cover filenames, so `SharedBook` keys the book cover off `cover_cache_key` and requests a cover for every main file; a missing file cover 404s and falls back to the placeholder.

### React Query Cache Invalidation

When a mutation modifies a resource (update/delete/merge), invalidate related queries so the UI refreshes.

**Cross-resource invalidation is required**: When metadata entities (genres, tags, series, people, publishers) are modified, also invalidate `ListBooks` and `RetrieveBook` queries since books display this metadata.

**Hierarchy changes must invalidate publisher detail + file query families**: For hierarchical metadata such as publishers, mutations that reparent a node (`parent_id` edits, "set child" actions) affect descendant-inclusive detail/file data for the moved node, both sides of the move, and any cached ancestors. Invalidate the `RetrievePublisher` and `PublisherFiles` query families for hierarchy changes rather than only the edited node.

**Pattern:**
```typescript
import { QueryKey as BooksQueryKey } from "./books";

// In mutation onSuccess:
onSuccess: () => {
  queryClient.invalidateQueries({ queryKey: [QueryKey.ListGenres] });
  // Also invalidate book queries since they display genre info
  queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
  queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
}
```

### UI Components

- Custom components in `app/components/` using Radix UI primitives
- Tailwind CSS for styling with dark/light theme support
- Components follow shadcn/ui patterns
- Add new shadcn components using `npx shadcn@latest add`
- **Copy to the clipboard through `copyText` (`app/utils/clipboard.ts`), never `navigator.clipboard` directly.** The async clipboard is undefined over plain HTTP (a LAN address), so a direct call throws there. `copyText` falls back to `execCommand` and returns `false` on failure; toast an error in that case instead of a success.
- **File labels come from the server.** Render `fileLabel(file)` from `@/utils/format`: the Go-resolved `display_name` (see `pkg/AGENTS.md`), or the file type when it is empty, which happens only in the Share Link payload for a main file without a name. Never rebuild the label from `file.name || getFilename(file.filepath)`. A supplement's label is its filename, and the Share Link payload has no path to rebuild it from.
- **Pluralize counts.** Use `formatPageCount` from `@/utils/format` for page counts, and a `count === 1` check for other nouns, never a hard-coded plural ("1 pages").
- **Dialogs focus themselves on open, not the close button.** Radix focuses the first tabbable element on open. With a `DialogHeader` that is the close button, and a dialog opened from a dropdown menu inherits the menu's keyboard-style focus, so even a `focus-visible:` ring showed. `DialogContent` therefore focuses its own container when the first tabbable element is the header close button (marked `data-dialog-header-close`), and leaves Radix's default alone otherwise, so a dialog whose first tabbable element is a field still focuses that field. A caller's `onOpenAutoFocus` runs first and wins if it calls `preventDefault()`. Close buttons (dialog and sheet) keep `focus-visible:` rings so they show only when a keyboard user tabs to them.
- **Icon-only buttons need an accessible name.** Give them an `aria-label`, and give disclosure toggles `aria-expanded` with a label that says what they do ("Show file details" / "Hide file details"). Tests find them by that name.

### Tabbed Navigation (Deep Linking Required)

**All tabbed navigation MUST be deeply linked via URL parameters.** Tabs should never use local state alone (`defaultValue` / `useState`). Instead, sync tab state with the URL so tabs are bookmarkable and support browser back/forward.

**Pattern:**
1. Add `/:tab?` to the route in `app/router.tsx`
2. Extract the tab param with `useParams()`
3. Validate against allowed values, defaulting to the first tab
4. Use `navigate()` in `onValueChange` to update the URL
5. Pass controlled `value` and `onValueChange` to `<Tabs>`

```tsx
const validTabs = ["details", "settings"] as const;
type TabValue = (typeof validTabs)[number];

const MyPage = () => {
  const { tab } = useParams<{ tab?: string }>();
  const navigate = useNavigate();

  const activeTab: TabValue = validTabs.includes(tab as TabValue)
    ? (tab as TabValue)
    : "details";

  const handleTabChange = (value: string) => {
    if (value === "details") {
      navigate("/my-page"); // Clean URL for default tab
    } else {
      navigate(`/my-page/${value}`);
    }
  };

  return (
    <Tabs onValueChange={handleTabChange} value={activeTab}>
      <TabsTrigger value="details">Details</TabsTrigger>
      <TabsTrigger value="settings">Settings</TabsTrigger>
      {/* ... */}
    </Tabs>
  );
};
```

**When tabs are distinct routes:** Some pages give each tab its own explicit route (e.g., `AdminPlugins` has `/settings/plugins`, `/settings/plugins/installed`, `/settings/plugins/discover` in `app/router.tsx`). Explicit routes enable cleaner bookmarks and allow `<Navigate>` redirects for legacy paths (e.g., `/settings/plugins/browse` → `/settings/plugins/discover`). In that case, derive `activeTab` from `useLocation()` instead of `useParams()`:

```tsx
const location = useLocation();
const activeTab = location.pathname.endsWith("/discover") ? "discover" : "installed";
```

Use the `useParams` pattern by default; reach for `useLocation` only when tabs are distinct top-level routes.

### Page Titles

**All pages MUST set a browser title** using the `usePageTitle` hook. This improves UX by showing meaningful titles in browser tabs and history.

**Hook:** `app/hooks/usePageTitle.ts`

```tsx
import { usePageTitle } from "@/hooks/usePageTitle";

// Static title for list pages
const GenresList = () => {
  usePageTitle("Genres");
  // ...
};

// Dynamic title for detail pages
const BookDetail = () => {
  const { data: book } = useBook(id);
  usePageTitle(book?.title);
  // ...
};
```

**Title Format:** `{Page Title} - Shisho` (e.g., "The Great Gatsby - Shisho")

**Guidelines:**
- List pages: Use plural noun (e.g., "Genres", "Tags", "Users & Roles")
- Detail pages: Use entity name/title from data (e.g., book title, username, series name)
- Settings pages: Use descriptive name (e.g., "User Settings", "Server Settings")
- Call hook early in component, before any early returns if using static title
- For dynamic titles, pass `undefined` while loading - hook handles this gracefully

## Handling Long Text in UI

When displaying user-generated content that may be long (names, titles, etc.):

### Dialogs
- Use `overflow-x-hidden` on `DialogContent` to prevent horizontal scrolling
- Avoid `overflow-hidden` on inner containers as it clips focus rings

### Dialog Headers
- Add `pr-8` to `DialogHeader` to leave room for the close button
- Let titles wrap naturally rather than truncating

### Page Headers with Buttons
```tsx
<div className="flex items-start justify-between gap-4">
  <h1 className="min-w-0 break-words">{title}</h1>
  <div className="shrink-0">{buttons}</div>
</div>
```

### Badges with Long Text
```tsx
<Badge className="max-w-full">
  <span className="truncate" title={text}>{text}</span>
  <button className="shrink-0">×</button>
</Badge>
```

### Flex Containers with Truncation
- Parent needs `min-w-0` for `truncate` to work on children

### Dropdowns/Command Items
```tsx
<CommandItem>
  <Icon className="shrink-0" />
  <span className="truncate" title={text}>{text}</span>
</CommandItem>
```

## Mobile Responsiveness

### Breakpoints

Tailwind breakpoints used in Shisho:
- `sm:` (640px) - Small tablets, large phones in landscape
- `md:` (768px) - Tablets, where desktop sidebar appears
- `lg:` (1024px) - Desktop

### Page Headers with Actions

**Never put title and action buttons on the same row.** On mobile, long titles wrap while buttons stay at top, creating awkward layouts.

```tsx
// Good - stacked layout
<div className="flex flex-col gap-3 mb-6">
  <h1 className="text-2xl font-semibold">{title}</h1>
  <div className="flex items-center gap-2">
    <Button size="sm" variant="outline">
      <Edit className="h-4 w-4 sm:mr-2" />
      <span className="hidden sm:inline">Edit</span>
    </Button>
  </div>
</div>

// Bad - side-by-side causes layout issues
<div className="flex items-start justify-between">
  <h1 className="text-2xl font-semibold">{title}</h1>
  <Button>Edit</Button>
</div>
```

### Admin Page Headers

For settings/admin pages with title, description, and action buttons:

```tsx
<div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6 md:mb-8">
  <div>
    <h1 className="text-2xl font-semibold mb-1 md:mb-2">
      Page Title
    </h1>
    <p className="text-sm md:text-base text-muted-foreground">
      Description text here.
    </p>
  </div>
  <div className="flex items-center gap-2 shrink-0">
    <Button size="sm">
      <Plus className="h-4 w-4 sm:mr-2" />
      <span className="hidden sm:inline">Add Item</span>
    </Button>
  </div>
</div>
```

### Icon-Only Buttons on Mobile

Use `hidden sm:inline` pattern for button text:

```tsx
<Button size="sm" variant="outline">
  <Edit className="h-4 w-4 sm:mr-2" />
  <span className="hidden sm:inline">Edit</span>
</Button>
```

### List Row Items

For rows with multiple pieces of info (stats, actions), stack on mobile:

```tsx
// File/item row with stats
<div className="py-2 space-y-1">
  {/* Primary row - always horizontal */}
  <div className="flex items-center gap-2">
    <Badge>{type}</Badge>
    <span className="truncate min-w-0 flex-1">{name}</span>
  </div>

  {/* Stats + actions row - must wrap (see below) */}
  <div className="flex flex-wrap items-center gap-x-2 gap-y-1 min-w-0 text-xs text-muted-foreground pl-6">
    <span>8h 44m</span>
    <span className="text-muted-foreground/50">·</span>
    <span>64 kbps</span>
    <span className="text-muted-foreground/50">·</span>
    <span>244.8 MB</span>
    {/* Action buttons */}
  </div>
</div>
```

**Stat + action rows must wrap.** A single-line stats/actions row with no `flex-wrap` packs its widest content into one non-shrinking row. For audiobooks that is duration, bitrate, codec, and filesize plus the download/listen/more buttons, which exceeds the content column on a narrow viewport, bleeds past the container, and (because the shared `LibraryLayout` `<main>` has no `overflow-x-hidden`) becomes page-level horizontal scroll. Always give these rows `flex-wrap` plus `gap-y-1` (wrapped-row spacing) and `min-w-0` so the stats and buttons wrap instead of overflowing. This bit `BookDetail.tsx`'s mobile file row (issue #398). Prefer per-element wrapping/breaking (`break-words`, `break-all`, `min-w-0`, grid `minmax(0,1fr)` value tracks) over a blanket `overflow-x-hidden` clip, which can mask other bugs and clip popovers.

### Dot Separators for Stats

Use middle dots (·) with faded styling to separate inline stats:

```tsx
<span>{duration}</span>
<span className="text-muted-foreground/50">·</span>
<span>{bitrate}</span>
<span className="text-muted-foreground/50">·</span>
<span>{fileSize}</span>
```

### Cover Images

Center and constrain cover images on mobile:

```tsx
<div className="w-48 sm:w-64 lg:w-full mx-auto lg:mx-0">
  <img className="w-full h-full object-cover" src={cover} />
</div>
```

## Cover Image Caching

API cover endpoints use `Cache-Control: private, max-age=31536000, immutable`, so the browser caches the response forever. Freshness is driven by changing the URL via `?v=${cacheKey}`, where `cacheKey` is a backend-computed `cover_cache_key` field (format `"<fileId>-<updatedAt.Unix()>"`) that only changes when the actual cover changes. This is much better than the old `dataUpdatedAt` approach, which changed on every TanStack Query refetch and defeated caching.

### Cache key sources by endpoint

| Endpoint | Cache key source |
|----------|-----------------|
| `/api/books/:id/cover` | `book.cover_cache_key` from API response |
| `/api/books/files/:id/cover` | `file.updated_at` from API response |
| `/api/series/:id/cover` | `series.cover_cache_key` from API response |

### Why URL-based busting is still required

Chromium and Firefox maintain an in-memory image cache (the HTML spec's "list of available images") that is **separate from the HTTP cache**. When an `<img>` element's `src` matches a URL previously rendered in the session, the browser serves the cached decoded bitmap without hitting HTTP, even with `immutable`. Changing the `?v=` param changes the URL, forcing a new network fetch.

### Rules

- **Append `?v=${cacheKey}`** to cover URLs where `cacheKey` comes from the backend (`book.cover_cache_key`, `series.cover_cache_key`, or `file.updated_at`).
- **For pages that mutate covers** (BookDetail, FileEditDialog), also add `key={cacheKey}` to the `<img>` tag. React remounting combined with URL change gives reliable refresh.
- **For child components that render covers**, accept a `cacheKey?: string` prop. Parents pass the appropriate cache key from the API response.

### Exceptions (no change needed)

- `GlobalSearch.tsx`: keep `searchQuery.dataUpdatedAt` (search results don't include `cover_cache_key`, small number of covers)
- `FileEditDialog.tsx`: keep `Date.now()` for immediate preview after cover mutation
- `IdentifyReviewForm.tsx`: keep `new Date(file.updated_at).getTime()`

### Checklist for new cover components

- [ ] Cover URL includes `?v=${cacheKey}` with the appropriate backend-provided cache key
- [ ] For mutation-capable pages, `<img key={cacheKey}>` for React remount
- [ ] Cover-mutating mutations invalidate the query whose data drives the key

### Page images

The CBZ/PDF page endpoint (`/api/books/files/:id/page/:n`) is also served `private, max-age=31536000, immutable`, so page URLs follow the same rule. Build every page URL with `filePageUrl(file, page)` from `app/utils/pageUrl.ts`, which appends `?v=` with the file's `updated_at`. Never write the page URL inline: a URL without the key keeps showing the old pages for a year after the file is replaced on disk.

- Components that render pages take the file (`PageSourceFile`, which is `id` plus `updated_at`), not a bare `fileId`, so they can build the keyed URL. `PagePicker`, `PagePreview`, and `ChapterRow` follow this.
- `updated_at` is the key because every rescan that re-reads a changed file bumps it. The same scan drops the server's cached pages for that file (`invalidatePageCaches` in `pkg/worker/scan_unified.go`), so the new URL never gets an old render. The key also changes on unrelated metadata edits; that only causes a refetch, never a stale page. A size-plus-mtime key would churn less, but it would not change on a forced refresh, which is the manual fix after a replacement that kept the same size and mtime.

### Breadcrumbs

Make breadcrumbs responsive with truncation:

```tsx
<nav className="text-xs sm:text-sm text-muted-foreground">
  <ol className="flex items-center gap-1 sm:gap-2 flex-wrap">
    <li className="shrink-0">
      <Link to="/">Home</Link>
    </li>
    <li className="shrink-0">›</li>
    <li className="truncate max-w-[120px] sm:max-w-none">
      <Link to="/book">{longBookTitle}</Link>
    </li>
    <li className="shrink-0">›</li>
    <li className="truncate">{currentPage}</li>
  </ol>
</nav>
```

### Card/Section Padding

Reduce padding on mobile:

```tsx
<div className="border rounded-md p-4 md:p-6">
  <h2 className="text-base md:text-lg font-semibold mb-3 md:mb-4">
    Section Title
  </h2>
</div>
```

### Config/Settings Rows

Stack label and value on mobile for long values:

```tsx
<div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-1 sm:gap-4">
  <span className="text-sm font-medium">{label}</span>
  <span className="text-xs sm:text-sm text-muted-foreground font-mono break-all sm:break-normal">
    {value}
  </span>
</div>
```

### Mobile Navigation

- Sidebar is hidden on mobile (`hidden md:block`)
- Use `MobileDrawer` component with hamburger menu in header
- Drawer slides in from left with backdrop blur
- `MobileNavContext` provides `isOpen`, `open`, `close`, `toggle`

### Form Inputs on Mobile

Remove focus ring glow for cleaner mobile appearance:

```tsx
<Input
  className={cn(
    fullWidth && "focus-visible:ring-0 focus-visible:border-border"
  )}
/>
```

### Fixed Position Dropdowns

When a dropdown is inside an `overflow-hidden` container (like collapsible sections), use fixed positioning:

```tsx
<div
  className={cn(
    "bg-background border rounded-lg shadow-lg z-50",
    isMobile
      ? "fixed left-4 right-4 top-28"
      : "absolute top-full mt-2 left-0 w-80"
  )}
>
```

### Tabs on Mobile

Make tabs scrollable and use smaller text:

```tsx
<TabsList className="w-full justify-start overflow-x-auto">
  <TabsTrigger className="text-xs sm:text-sm" value="tab1">
    Tab 1
  </TabsTrigger>
  <TabsTrigger className="text-xs sm:text-sm" value="tab2">
    Longer Tab Name
  </TabsTrigger>
</TabsList>
```

### Spacing Patterns

Use responsive spacing throughout:

```tsx
// Margins
className="mb-6 md:mb-8"
className="mb-1 md:mb-2"

// Gaps
className="gap-4 md:gap-8"
className="space-y-4 md:space-y-6"

// Padding
className="py-3 md:py-4 px-4 md:px-6"
```

## Testing

### Test Stack

| Level | Framework | Purpose |
|-------|-----------|---------|
| Unit + Component | Vitest + React Testing Library | Fast, native Vite integration |
| E2E | Playwright | Browser automation |

### Running Tests

```bash
mise test:unit      # Run Vitest unit/component tests with coverage
mise test:js        # Run all JS tests (unit + E2E, used in `mise check`)
mise test:e2e       # Run E2E tests (Chromium + Firefox in parallel)
```

### Test File Locations

- **Unit/Component tests**: Colocated with source files as `*.test.ts(x)`
- **E2E tests**: Separate `e2e/` directory

### Writing Unit Tests

```typescript
import { describe, expect, it } from "vitest";
import { myFunction } from "./myFile";

describe("myFunction", () => {
  it("returns expected value", () => {
    expect(myFunction("input")).toBe("output");
  });
});
```

### Writing Component Tests

```typescript
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import MyComponent from "./MyComponent";

describe("MyComponent", () => {
  it("renders correctly", () => {
    render(<MyComponent prop="value" />);
    expect(screen.getByText("expected text")).toBeInTheDocument();
  });
});
```

### Fake Timers and `userEvent`

- `vitest.setup.ts` enables fake timers globally with `shouldAdvanceTime: true`
- When a test uses `userEvent.setup()`, pass `advanceTimers: vi.advanceTimersByTime` so clicks and typing don't stall or hit the test timeout under heavy load

```typescript
const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
```

- `testTimeout` is 15s (`vitest.config.ts`), not vitest's 5s default. `mise check:quiet` runs the unit suite in parallel with the Go tests, linters, and e2e pipelines, and heavy jsdom tests that take 2-3s idle were timing out at random under that load. Don't lower it, and don't "fix" a load-induced timeout by bumping one test's own timeout. If a test is slow on an idle machine, make it cheaper instead (for example, render one card rather than a full page when the assertion doesn't depend on the count).

### E2E Tests

**See `e2e/AGENTS.md` for detailed E2E patterns**, including:
- Test independence via `beforeAll` hooks
- Test-only API endpoints (`ENVIRONMENT=test`)
- Common pitfalls (shared database, toast assertions, redirect expectations)

### Coverage

- Collected automatically with unit tests (V8 provider)
- Reports: text (console), lcov, HTML in `coverage/`
- Not enforced, tracked for visibility

## Metadata Edit Dialogs

### First-Class Metadata Fields

All editable metadata fields should be treated as first-class citizens. Don't add confusing helper text that implies the field is secondary or derived.

**Don't do this:**
```tsx
<Label htmlFor="name">Name</Label>
<Input id="name" value={name} onChange={...} />
<p className="text-muted-foreground">
  Leave empty to use the title from file metadata.
</p>
```

**Do this instead:**
```tsx
<Label htmlFor="name">Name</Label>
<Input id="name" value={name} onChange={...} />
```

**Why:** Helper text like "Leave empty to use..." implies the field is optional or secondary. This confuses users about what the field does and what happens when they clear it. Metadata fields should be straightforward - what you enter is what you get.

### Field Clearing Behavior

When a user clears a metadata field, the cleared value should be saved (not revert to some default). The scanner will repopulate the field from the source file on the next scan if needed.

## Unsaved Changes Protection (Required)

**All forms that create or update data MUST have unsaved changes protection.** This prevents users from accidentally losing their work when navigating away or closing a dialog.

### When Protection is Required

- Create forms (CreateUser, CreateLibrary, Setup)
- Edit forms (UserDetail, LibrarySettings, BookEditDialog)
- Any dialog or page where users enter data and click "Save"

### When Protection is NOT Required

- Action dialogs that execute immediately (merge, move, delete confirmation)
- Quick actions with no pending state (add to list popover)
- View-only pages
- Settings that apply immediately on change (theme selection)

### Pattern 1: Dialog Forms (FormDialog)

For forms inside dialogs, use `FormDialog` instead of `Dialog`:

```tsx
import { FormDialog } from "@/components/ui/form-dialog";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";

const MyEditDialog = ({ open, onOpenChange, initialData }) => {
  const [name, setName] = useState("");
  const [initialValues, setInitialValues] = useState<{ name: string } | null>(null);

  // Initialize form when dialog opens
  useEffect(() => {
    if (open && initialData) {
      setName(initialData.name);
      setInitialValues({ name: initialData.name });
    }
  }, [open, initialData]);

  // Compute hasChanges by comparing current values to initial
  const hasChanges = useMemo(() => {
    if (!initialValues) return false;
    return name !== initialValues.name;
  }, [name, initialValues]);

  // useFormDialogClose handles closing after save (waits for hasChanges to update)
  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  const handleSave = async () => {
    await mutation.mutateAsync({ name });
    setInitialValues({ name }); // Update initial values so hasChanges becomes false
    requestClose(); // Close after state updates
  };

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent>
        {/* form fields */}
        <Button onClick={handleSave}>Save</Button>
      </DialogContent>
    </FormDialog>
  );
};
```

**Key points:**
- `FormDialog` wraps `Dialog` and intercepts close attempts when `hasChanges` is true
- Shows `UnsavedChangesDialog` confirmation before discarding
- Also adds `beforeunload` handler for browser close/refresh
- `useFormDialogClose` ensures dialog closes only after `hasChanges` updates to false

### Pattern 2: Full-Page Forms (useUnsavedChanges)

For forms on full pages (not dialogs), use the `useUnsavedChanges` hook:

```tsx
import { useUnsavedChanges } from "@/hooks/useUnsavedChanges";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";

const MySettingsPage = () => {
  const [name, setName] = useState("");
  const [initialValues, setInitialValues] = useState<{ name: string } | null>(null);
  const [isInitialized, setIsInitialized] = useState(false);

  // Reset when navigating to different entity
  useEffect(() => {
    setIsInitialized(false);
  }, [entityId]);

  // Initialize form when data loads
  useEffect(() => {
    if (query.isSuccess && query.data && !isInitialized) {
      setName(query.data.name);
      setInitialValues({ name: query.data.name });
      setIsInitialized(true);
    }
  }, [query.isSuccess, query.data, isInitialized]);

  // Compute hasChanges
  const hasChanges = useMemo(() => {
    if (!initialValues || !isInitialized) return false;
    return name !== initialValues.name;
  }, [name, initialValues, isInitialized]);

  // Hook provides blocker dialog state and handlers
  const { showBlockerDialog, proceedNavigation, cancelNavigation } =
    useUnsavedChanges(hasChanges);

  const handleSave = async () => {
    await mutation.mutateAsync({ name });
    setInitialValues({ name }); // hasChanges becomes false
  };

  return (
    <>
      {/* form content */}
      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />
    </>
  );
};
```

**Key points:**
- `useUnsavedChanges` blocks SPA navigation via `react-router`'s `useBlocker`
- Also adds `beforeunload` handler for browser close/refresh
- Must render `UnsavedChangesDialog` and wire up the handlers

### Pattern 3: Child Components with Unsaved State

When a child component has its own save button and unsaved state (like LibraryPluginsTab inside LibrarySettings), expose the changes via callback:

**Child component:**
```tsx
interface Props {
  onHasChangesChange?: (hasChanges: boolean) => void;
}

const ChildEditor = ({ onHasChangesChange }) => {
  const [localData, setLocalData] = useState(null);

  const hasChanges = localData !== null && /* comparison logic */;

  useEffect(() => {
    onHasChangesChange?.(hasChanges);
  }, [hasChanges, onHasChangesChange]);

  // ... rest of component
};
```

**Parent component:**
```tsx
const ParentPage = () => {
  const [childHasChanges, setChildHasChanges] = useState(false);

  // Combine parent form changes with child changes
  const hasChanges = formHasChanges || childHasChanges;

  const { showBlockerDialog, ... } = useUnsavedChanges(hasChanges);

  return (
    <>
      <ChildEditor onHasChangesChange={setChildHasChanges} />
      {/* ... */}
    </>
  );
};
```

### Pattern 4: Tab Switching with Unsaved Changes

When a page has tabs and one tab has inline editing, intercept tab changes:

```tsx
const [pendingTabChange, setPendingTabChange] = useState<string | null>(null);

const handleTabChange = (value: string) => {
  if (hasChanges) {
    setPendingTabChange(value);
    return;
  }
  navigateToTab(value);
};

const handleConfirmTabChange = () => {
  if (pendingTabChange) {
    setIsEditing(false);
    navigateToTab(pendingTabChange);
    setPendingTabChange(null);
  }
};

return (
  <>
    <Tabs onValueChange={handleTabChange} value={activeTab}>
      {/* ... */}
    </Tabs>
    <UnsavedChangesDialog
      onDiscard={handleConfirmTabChange}
      onStay={() => setPendingTabChange(null)}
      open={pendingTabChange !== null}
    />
  </>
);
```

### Comparing Values with Arrays/Objects

For complex state with arrays or objects, use `fast-deep-equal`:

```tsx
import equal from "fast-deep-equal";

const hasChanges = useMemo(() => {
  if (!initialValues) return false;
  return (
    name !== initialValues.name ||
    !equal(selectedItems, initialValues.selectedItems)
  );
}, [name, selectedItems, initialValues]);
```

### Checklist for New Forms

- [ ] Form uses `FormDialog` (dialogs) or `useUnsavedChanges` (pages)
- [ ] `initialValues` state stores values when form loads
- [ ] `hasChanges` computed by comparing current state to initial values
- [ ] After successful save, update `initialValues` so `hasChanges` becomes false
- [ ] `UnsavedChangesDialog` rendered with proper handlers
- [ ] Child components with own save buttons expose `onHasChangesChange`
- [ ] Tab switching intercepted if tabs have inline editing

## UI/UX Consistency Requirements

### List Page Patterns

All list pages (Books, Series, People, Genres, Tags) should follow consistent patterns:

**Required Elements:**
1. **Page header** with title and subtitle in `<div className="mb-6">`
2. **Search input** with `max-w-xs` and appropriate placeholder
3. **Item count display**: Show "Showing X-Y of Z [items]" above the list **only when total > 0** (hide when empty to avoid "Showing 1-0 of 0")
4. **Loading state**: Use `<LoadingSpinner />` component, not raw text
5. **Pagination**: Use shadcn/ui `Pagination` components, never raw `<button>` elements

**Use the Gallery Component for Grid Layouts:**
For pages displaying items in a grid (books, series), use the `Gallery` component which provides:
- Consistent "Showing X-Y of Z" count
- Pagination with proper shadcn/ui components
- Loading state handling

```tsx
<Gallery
  isLoading={query.isLoading}
  isSuccess={query.isSuccess}
  itemLabel="books"
  items={query.data?.items ?? []}
  itemsPerPage={24}
  renderItem={renderItem}
  total={query.data?.total ?? 0}
/>
```

**For List-Based Pages (People, Genres, Tags):**
Even though these pages don't use Gallery, they should still:
- Show "Showing X-Y of Z [items]" count **only when total > 0**
- Use `<LoadingSpinner />` for loading states
- Use shadcn/ui Pagination components
- Have consistent empty state messages that differentiate between "no results" and "no results matching search"

**Item Count Pattern:**
```tsx
{total > 0 && (
  <div className="mb-4 text-sm text-muted-foreground">
    Showing {offset + 1}-{Math.min(offset + limit, total)} of {total} items
  </div>
)}
```

**Empty State Messages:**
```tsx
// With search context
{searchQuery
  ? "No people found matching your search."
  : "No people in this library yet."}

// Without search context (less ideal)
"No genres found"
```

### Detail Page Patterns

All metadata detail pages (Series, Person, Genre, Tag) follow a consistent structure:

**Header Section:** the mutating buttons render only for a role that can write the entity (see "Permission-gated controls"), and the dialogs behind them mount only then.
```tsx
const canMutate = useCan(writePermissionForEntity(entityType));

<div className="mb-6 md:mb-8">
  <div className="flex items-start justify-between gap-4 mb-2">
    <h1 className="text-2xl font-semibold min-w-0 break-words">{name}</h1>
    {canMutate && (
      <div className="flex gap-2 shrink-0">
        <Button onClick={() => setEditOpen(true)} size="sm" variant="outline">
          <Edit className="h-4 w-4 mr-2" />
          Edit
        </Button>
        <Button onClick={() => setMergeOpen(true)} size="sm" variant="outline">
          <GitMerge className="h-4 w-4 mr-2" />
          Merge
        </Button>
        {canDelete && (
          <Button onClick={() => setDeleteOpen(true)} size="sm" variant="outline">
            <Trash2 className="h-4 w-4 mr-2" />
            Delete
          </Button>
        )}
      </div>
    )}
  </div>
  {/* Optional: Sort name if different */}
  {sortName !== name && (
    <p className="text-muted-foreground mb-2">Sort name: {sortName}</p>
  )}
  <Badge variant="secondary">{count} book{count !== 1 ? "s" : ""}</Badge>
</div>
```

**Content Sections:**
```tsx
<section className="mb-10">
  <h2 className="text-xl font-semibold mb-4">Books in Series</h2>
  {/* ... content ... */}
</section>
```

**Empty States:**
```tsx
<div className="text-center py-8 text-muted-foreground">
  This [entity] has no associated books.
</div>
```

### Always Use Button Component

**Never use raw `<button>` elements.** Always use the shadcn/ui `Button` component for:
- Consistent styling across the app
- Built-in cursor-pointer behavior
- Proper disabled states
- Accessibility features

```tsx
// Bad - raw button
<button className="px-3 py-1 rounded-md border">Previous</button>

// Good - Button component
<Button variant="outline" size="sm">Previous</Button>
```

### Cursor Styles for Interactive Elements

**All clickable elements MUST have `cursor-pointer`**. This is a fundamental UX requirement that signals interactivity to users.

**Components that need `cursor-pointer`:**
- Buttons (already in base `buttonVariants`)
- Checkboxes
- Select triggers and items
- Tab triggers
- Command items (in dropdowns/comboboxes)
- Dropdown menu items (including checkbox/radio items and sub-triggers)
- Dialog close buttons
- Pagination links
- Any custom clickable element (raw `<button>` or clickable `<div>`)

**Pattern for shadcn/ui components:**
When adding or modifying UI components, ensure `cursor-pointer` is in the base className:

```tsx
// Good - cursor-pointer included
className={cn(
  "flex items-center justify-center cursor-pointer",
  "disabled:cursor-not-allowed disabled:opacity-50",
  className,
)}

// Bad - missing cursor-pointer
className={cn(
  "flex items-center justify-center",
  "disabled:cursor-not-allowed disabled:opacity-50",
  className,
)}
```

**Pattern for raw buttons:**
When using raw `<button>` elements outside of the Button component, always add `cursor-pointer`:

```tsx
// Good
<button className="px-4 py-2 rounded-md cursor-pointer" onClick={...}>

// Bad
<button className="px-4 py-2 rounded-md" onClick={...}>
```

**Why this matters:**
- Users rely on cursor changes to understand what's clickable
- Missing cursor-pointer feels broken/unresponsive
- Consistency across the UI is essential for professional UX

### Dynamic Class Composition with `cn()`

**Always use `cn()` from `@/libraries/utils` for dynamic className composition.** Never use template literals to concatenate class strings.

```tsx
// Good - cn()
<span className={cn("inline-flex h-5 w-5 rounded-sm", colorClass)}>

// Good - cn() with conditionals
<button className={cn(
  "h-8 rounded-md border px-3 text-xs cursor-pointer",
  isSelected
    ? "border-primary bg-primary text-primary-foreground"
    : "border-border bg-card hover:bg-accent",
)}>

// Bad - template literal
<span className={`inline-flex h-5 w-5 rounded-sm ${colorClass}`}>
```

`cn()` wraps `clsx` + `tailwind-merge`, so it handles conditional classes, deduplication, and Tailwind conflict resolution. Template literals bypass all of that.

## Sortable List Row Keys

**Sortable lists (dnd-kit-backed `SortableList` and consumers like `SortableEntityList`, `FileChaptersTab`) MUST use stable client-side row keys that survive reorder/remove.** Index-based keys (`${index}`) and content-based keys (`${item.name}-${index}`) both change on every reorder, which confuses dnd-kit's drag tracking: the active drag's identity changes mid-gesture, causing flicker, dropped drags, or rows that mutate the wrong sibling after sorting.

**Pattern:** Assign each row a stable id when it first enters the list (mount or append) and preserve it across reorder/remove. `FileChaptersTab` uses an `EditedChapter._editKey` field generated by a module-level monotonic counter (`nextEditKey()`); `SortableEntityList` uses the same counter pattern but stores the key in a `useRef<WeakMap<T, string>>` keyed by item reference (so callers don't need to inject a `_key` field on their own types).

**Don't** rely on labels, indices, or any field that changes during normal editing as the sortable id. Use a server-side stable id (e.g., `chapter.id`) only when every row actually has one; newly-added rows that haven't been persisted yet need a client-side counter or WeakMap-tracked id.

**Caller responsibility for `SortableEntityList`:** the WeakMap is keyed by item *reference*, so callers must pass stable item references across renders. `items={list.map((x) => ({ name: x }))}` re-creates the wrapper objects every render and defeats the WeakMap: each row gets a fresh key every render and dnd-kit sees a brand-new identity set. Either store the wrapped shape in `useState`, or wrap the `.map()` in a `useMemo` keyed on the source array. See `IdentifyReviewForm.tsx`'s `narratorItems` for the pattern.

## Known Radix UI Issues

### Dialog + DropdownMenu pointer-events Bug

**Problem:** When a Dialog is triggered from a DropdownMenu item, Radix's DismissableLayer incorrectly sets `pointer-events: none` on the body during unmount, leaving the page unclickable after the dialog closes.

**Solution:** Already fixed globally in `app/components/ui/dialog.tsx`. The custom `Dialog` wrapper includes:
1. A cleanup effect that clears `pointer-events` when `open` changes to `false`
2. An unmount cleanup effect for conditionally rendered dialogs

The 300ms delay ensures cleanup runs after Radix's buggy unmount effects complete.

**If you encounter similar issues:**
1. Use browser DevTools to check if `pointer-events: none` is stuck on `<body>`
2. Use a MutationObserver or setter trap to identify what's setting the style
3. Add a delayed cleanup effect that runs after Radix's effects complete

**Related:** DropdownMenu components that trigger dialogs should also have `onCloseAutoFocus={(e) => e.preventDefault()}` on `DropdownMenuContent` to prevent focus management conflicts.

### asChild trigger components must forwardRef

**Problem:** When a Radix `XxxTrigger asChild` wraps a custom React function component (instead of a direct `<Button>` or DOM element), the component must be a `forwardRef` that spreads incoming props onto the underlying button. Otherwise:

- For **floating** primitives (`Popover`, `DropdownMenu`, `HoverCard`, `Tooltip` with positioning, `ContextMenu`): the popper has no DOM ref to anchor to, so Floating UI falls back to the document origin `(0, 0)` and the content renders **off-screen** (often above the viewport). The trigger's onClick still fires, so the component appears to do nothing.
- For **non-floating** primitives (`Sheet`, `Drawer`, `Dialog`): the panel still renders correctly because it's positioned relative to the viewport, not the trigger. But focus management on close can't restore focus to the trigger, and screen reader / keyboard semantics suffer.

This bug is **invisible in jsdom unit tests**: Radix's positioning math doesn't run there. Caught only in a real browser.

**Required pattern for any custom component used as an asChild trigger:**

```tsx
import { forwardRef } from "react";

export const MyButton = forwardRef<
  HTMLButtonElement,
  { isDirty: boolean } & React.ComponentPropsWithoutRef<typeof Button>
>(({ isDirty, ...props }, ref) => (
  <Button ref={ref} {...props}>
    {/* ... */}
  </Button>
));
MyButton.displayName = "MyButton";
```

Three things matter:
1. `forwardRef`: receives the ref from Radix's Slot
2. `ref={ref}` on the underlying `<Button>`: passes the ref to a DOM element (Button itself is forwardRef'd)
3. `{...props}`: Radix's Slot adds `onClick`, `aria-expanded`, `aria-controls`, `data-state` etc. via `React.cloneElement`; these must reach the button

Direct `<Button>` (the shadcn/ui primitive) is already forwardRef'd, so the common case `<PopoverTrigger asChild><Button>...</Button></PopoverTrigger>` works without ceremony. The footgun is when you wrap that Button in a custom presentational component (`SizeButton`, `SortButton`, `FilterButton`).

**Existing forwardRef'd trigger components:** `SizeButton`, `SortButton`, `FilterButton`. Follow the same shape if you add another.

## Key Files

| Purpose | Location |
|---------|----------|
| Router | `app/router.tsx` |
| API client | `app/libraries/api.ts` |
| Query hooks | `app/hooks/queries/` |
| Page title hook | `app/hooks/usePageTitle.ts` |
| Unsaved changes hook | `app/hooks/useUnsavedChanges.ts` |
| Form dialog close hook | `app/hooks/useFormDialogClose.ts` |
| FormDialog component | `app/components/ui/form-dialog.tsx` |
| UnsavedChangesDialog | `app/components/ui/unsaved-changes-dialog.tsx` |
| Generated types | `app/types/generated/` |
| Components | `app/components/` |
| Pages | `app/components/pages/` |
| Vitest config | `vitest.config.ts` |
| Playwright config | `playwright.config.ts` |
| Unit/component tests | `app/**/*.test.{ts,tsx}` |
| E2E tests | `e2e/*.spec.ts` |
