# Authentication and Permissions

Two layers: global role permissions (resource plus `read`/`write`) and per-user library access. Resources, operations, and the predefined roles are in `pkg/models/role.go`; how each route family is guarded is in `pkg/server/server.go` and each package's `routes.go`. The judgement rules a reviewer applies are in `docs/agents/standards/backend.md`.

## Attaching checks

Route middleware is `RequirePermission`, `RequireAnyPermission`, and `RequireLibraryAccess(param)` in `pkg/auth/middleware.go`; any `routes.go` shows the usage.

- **Any-of** fits two kinds of page reading the same data: review criteria take `books:read` (review panel) or `config:read` (settings); `GET /api/libraries` takes `libraries:read` or `users:write` (access picker). When only some routes of a family need the any-of, give them their own group (`libraryListGroup` in `pkg/server/server.go`) rather than weakening the family's group.
- **Authenticated-only groups** run `Authenticate` and no permission, for the caller's own data or data every signed-in user may see. Share Link management is one, so its `shares` checks sit on each route.
- **In a handler**, get the user with `auth.RequireUser(c)` (401 when none, so a route registered without its middleware fails closed). Check permissions with `user.HasPermission` and deny with `errcodes.PermissionDenied(resource, operation)`, which matches `RequirePermission`'s wording; `errcodes.AnyPermissionDenied` matches `RequireAnyPermission`.
- **Library from fetched data:** `auth.RequireLibraryAccessFor(c, file.LibraryID)` returns 401 without a user and `errcodes.LibraryAccessDenied()` (403) without access.
- **Lists of library data** filter by `user.GetAccessibleLibraryIDs()` on the user from `auth.RequireUser`. A nil user's filter is nil, which the queries read as every library.
- Tests set the user with `auth.SetUser`.

## Loading users

Every user is loaded by `auth.LoadUser` (Role, Role.Permissions, LibraryAccess; active only unless `IncludeInactive`), shared by login, session and Basic Auth, `apikeys.Service.AuthenticateOwner`, and `users.Service.Retrieve`. On an authenticating path a missing or deactivated user is `errcodes.UserInactive()`; any other `LoadUser` failure is a 500, never a 401 that signs the user out. `auth.Service.GetUserByID` does this mapping for the session middleware and `/api/auth/me`. Basic Auth challenges (`WWW-Authenticate` plus `errcodes.AuthenticationRequired()`) only for a missing or malformed header or a 401 from `Authenticate`.

## Route families with non-obvious guards

- **Global search** requires `books:read` and fills its `series` and `people` sections only for `series:read` and `people:read` (`search.GlobalSearchSections`); a withheld section is an empty array, not a missing key.
- **Shared-page lookups** such as `GET /api/plugins/identifier-types` and `GET /api/plugins/order/:hookType` live in their own `books:read` group. Inside the `config:write` plugin group they would 403 editors and viewers and the frontend would fail silently.
- **Jobs.** The `/api/jobs` group only authenticates. `GET /api/jobs` and `/:id/logs` need `jobs:read` per route; `POST /api/jobs` needs `jobs:read` and `jobs:write` in the handler, except `bulk_download`, which needs `books:read` plus library access to every existing requested file and stores only `file_ids` and `estimated_size_bytes`. `GET /api/jobs/:id` and `/:id/download` allow `jobs:read` or the creator of a `bulk_download` job (`canReadJob`) and 404 otherwise so IDs cannot be probed. A group-level `jobs:read` would break bulk download for editors and viewers.
- **List sharing** needs no users permission. List handlers check `CanManage`; recipients come from `/api/users/directory`; `createShare` refuses an unknown or inactive `user_id` with one 422 so it cannot probe accounts. Every user embedded in a list payload (`List.User`, `ListShare.User`, `SharedByUser`, `ListBook.AddedByUser`) is a `models.UserRef`, because recipients of any role read those payloads. `GET /api/lists/:id/books` requires `books:read`. `users:read` on the share handlers would make sharing impossible for every stock role but Admin.
- **Self password reset** (`/users/:id/reset-password`) requires only authentication; see root `AGENTS.md`.

## Device routes (Kobo, eReader, OPDS)

- **An entity loaded by URL id is re-checked against the route's listing rules**, because the id is attacker-chosen. `kobo.Service.FileInScope` reuses `scopedFilesQuery` (the sync query) and returns whether the key syncs the file; `requireFileInScope` in `pkg/kobo/handlers.go` turns false into `errcodes.NotFound("File")` (never 403, never the Kobo store proxy). An entity nested under a library path must belong to it, or 404. On eReader a book or file outside the owner's access is a 404 (`requireEntityAccess`); library paths stay 403 through `RequireLibraryAccessFor`.
- **API key routes mount `apikeys.Middleware.APIKeyAuth(permission)`**, which calls `AuthenticateOwner`: it loads the owner with role, permissions, and library access, returns 401 for a missing or deactivated owner and 403 without `books:read`, and stores the owner with `auth.SetUser` and the key for `apikeys.RequireKey`. Handlers read the owner with `auth.RequireUser`. The eReader `/e/:shortCode` redirect calls `AuthenticateOwner` before revealing the key URL. A new key-authenticated family mounts this middleware.

## Adding a permission resource

1. Constant in `pkg/models/role.go`.
2. Add it to `roles.ValidResources` in `pkg/roles/service.go`, or roles using it fail validation.
3. Seed it onto the admin role in a migration (`20260927000000_add_shares_permission.go`).
4. Show it in `app/components/library/PermissionMatrix.tsx`.
5. Guard the routes.
6. Document it in `website/docs/users-and-permissions.md`.
