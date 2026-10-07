# Permission gating in the frontend

Read this before adding a control that mutates data, a query hook, a route, a nav entry, or a link to another entity's page, or before writing a test that depends on the signed-in role. The model in one line (in `app/AGENTS.md`): query hooks gate themselves with `useRequires`; components hide controls with `useCan`.

## Checking a permission in a component

`useCan(requirement)` from `@/hooks/useCan` takes a typed `Requirement`: one permission (`"books:write"`), an array the role must hold all of, or `anyOf(...)`. Where a hook cannot run once per check (after an early return, in a loop or callback, or for a requirement passed as a prop), use `can` from `useAuth()`, as `ProtectedRoute` and `useLibraryNavItems` do. Both are built on `meetsRequirement`, so a misspelled permission fails to compile.

Call `useCan` before any early return, and put it first in a `&&` chain (`useCan("people:read") && !isShareLink`) so it runs on every render.

ESLint rejects `hasPermission(...)` with string literals. A permission held in a variable gets past it, because `holds` in `app/utils/permissions.ts` and `PermissionMatrix` pass variables legitimately.

## What to gate on

Gate a mutating control on the permission its backend route requires, not on the page's display type:

Genre, tag, and publisher actions need `books:write` (they have no resource of their own); series and person actions need `series:write` and `people:write`; a library scan needs `libraries:write`, or `jobs:read` and `jobs:write`. `writePermissionForEntity(entityType)` from `@/utils/permissions` maps metadata entities; `ResourceDetail` applies it itself. Detail pages fetch their merge dialog's candidate list only for a role that can merge.

Never gated on a write permission:

- **List membership.** Add to list, create list, and per-list actions follow the list's own `permission` field (owner/manager/editor/viewer). Pickers leave out lists the user can only view; saving keeps the book in those lists.
- **Selection mode and downloads.** Only merge, delete, and review actions inside `SelectionToolbar` are hidden.

## Hide, do not disable

Hide the whole control and skip mounting the dialogs behind it (`{canWriteBooks && <RescanDialog … />}`). The one exception is a settings form a role can read but not save: inputs stay, disabled, with `<ReadOnlyNotice />` at the top and the Save button hidden (`PluginConfigForm`, `UserDetail`, `AdminReviewCriteria`, `AdminSharing`). Components that render either way take a flag: `ReviewPanel` `readOnly`, `FileChaptersTab` `canEdit`.

**Action menus gate each entry, not the menu.** Each entry carries its own `visible` flag from its route's permission; the menu renders when at least one entry is visible, and separators appear only between non-empty groups. One `books:write` check around the whole menu is wrong: Share needs a `shares` permission instead. Add to list has no permission of its own, so it joins the menu whenever another entry shows it and is otherwise a standalone button.

## Routes, nav, and links

- Library routes in `router.tsx` pass `requiredPermission` to `ProtectedRoute`. Route guards and the nav hooks (`useLibraryNavItems`, `useAdminNavItems`) share `ROUTE_PERMISSIONS` from `@/utils/permissions`, so a nav entry cannot lead to Access Denied. A new route adds its entry there.
- A name that links to a series or person page renders as plain text without `series:read` or `people:read`. A link to any page follows the same rule: show the link only when the target route would allow it.
- A page that reads data from a route the role lacks hides that part or shows a notice (`ListDetail` hides a list's books without Books Read; `AdvancedOrderSection` shows a notice).

## Library data for roles without Libraries Read

Reader pages never call the Libraries Read routes. They read `LibrarySummary` rows from `useUserLibraries` and its siblings in `hooks/queries/libraries.ts` (`GET /user/libraries`, authenticated only); `useLibrary`/`useLibraries` are for settings pages and user access assignment only. `GET /libraries` accepts `users:write` for the user forms, so a role with only `users:read` sees an empty library picker there. User pickers outside admin pages use `useUserDirectory()`.

## Query hooks gate their own permissions

Every query hook in `app/hooks/queries/` whose route needs a role permission checks it inside the hook. `useRequires(requirement, enabled)` from `hooks/queries/permissions.ts` returns `enabled` when the role meets the requirement and `false` otherwise. Set it after spreading caller options so a caller's `enabled: true` cannot bypass it:

```ts
return useQuery<Book, ShishoAPIError>({
  ...options,
  enabled: useRequires("books:read", options.enabled ?? Boolean(id)),
  queryKey: [QueryKey.RetrieveBook, id],
  queryFn: ...
});
```

- `hooks/queries/permissions.test.tsx` enforces this. A new query hook fails until you add it to `QUERY_HOOKS` with its backend route's requirement, then until its gate matches. The same file holds the allowlist of authenticated-only paths and checks that Demo Mode requests no route the server leaves unregistered there.
- Callers pass `enabled` only for their own state, never a restated permission: `useSharingSettings({ enabled: !isShareLink })`. A query behind a dialog passes `enabled: open`, which matters most where a list mounts one dialog per row.
- `refetch()` runs a query even while disabled, so a manual `refetch()` checks the result's `isEnabled` first.
- `useJob` is ungated because `GET /jobs/:id` also serves a bulk-download job to its creator; `JobDetail`'s route requires Jobs Read instead.
- `AuthProvider` and `Setup` call `/auth/*` through `API.request` directly; there are no auth query hooks. `AuthProvider` clears the query cache on login, logout, and `setAuthUser`, so the next user in a tab never sees cached data.

## Tests

Components and hooks that call `useAuth()` throw outside `AuthProvider`. Tests use the shared mock:

```ts
vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
beforeEach(() => setAuth({ permissions: ["books:read", "books:write"] }));
```

- `setAuth` resets anything it is not given, and its state lives in the module for the whole file, so a file with several roles calls it in `beforeEach`. Options and helpers are in `app/testing/auth.ts`.
- `app/testing/` imports vitest and is excluded from the production build; test-only helpers go there.
- `X.test.tsx` usually mocks query modules. `X.permissions.test.tsx` renders the real query hooks with a spied `API.request` and asserts which requests fire for a role or state (`BookDetailBody.permissions.test.tsx`; `MetadataDetail.permissions.test.tsx` covers a family of pages). Assert that no request fires, not which `enabled` value a caller passed: the hook owns the gate.
