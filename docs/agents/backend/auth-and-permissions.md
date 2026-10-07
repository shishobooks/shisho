# Authentication and Permissions

Two layers: global role permissions (resource plus `read`/`write`) and per-user library access. Resources, operations, and roles are in `pkg/models/role.go`; how a route family is guarded is in `pkg/server/server.go` and the package's `routes.go`. Review rules are under "Authorization" in `docs/agents/standards/backend.md`.

## Attaching checks

Route middleware is `RequirePermission`, `RequireAnyPermission`, and `RequireLibraryAccess(param)` in `pkg/auth/middleware.go`.

- **Any-of** fits two kinds of page reading the same data. When only some routes of a family need it, give them their own group rather than weakening the family's group.
- **Authenticated-only groups** run `Authenticate` and no permission, for the caller's own data, data every signed-in user may see, or a family whose routes need different permissions; those check per route or in the handler.
- **In a handler**, get the user with `auth.RequireUser(c)` (401 when none, so a route registered without its middleware fails closed). Check with `user.HasPermission` and deny with `errcodes.PermissionDenied`, which matches `RequirePermission`'s wording (`errcodes.AnyPermissionDenied` matches `RequireAnyPermission`).
- **Library from fetched data:** `auth.RequireLibraryAccessFor(c, libraryID)`.
- **Lists of library data** filter by `user.GetAccessibleLibraryIDs()`. A nil filter means every library.
- Tests set the user with `auth.SetUser`.

## Loading users

Load every user through `auth.LoadUser`. On an authenticating path a missing or deactivated user is `errcodes.UserInactive()`; any other load failure is a 500, never a 401 that signs the user out.

## Device routes (Kobo, eReader, OPDS)

- **An entity loaded by URL id is re-checked against the route's listing scope**, because the id is attacker-chosen, and is a 404 outside it (never 403, which confirms it exists). An entity nested under a library path must belong to that library.
- **A new API-key-authenticated family mounts `apikeys.Middleware.APIKeyAuth`**, and its handlers read the owner with `auth.RequireUser`.

## Adding a permission resource

1. Constant in `pkg/models/role.go`.
2. Add it to `roles.ValidResources` in `pkg/roles/service.go`, or roles using it fail validation.
3. Seed it onto the admin role in a migration (`20260927000000_add_shares_permission.go` is the pattern).
4. Show it in `app/components/library/PermissionMatrix.tsx`.
5. Guard the routes.
6. Document it in `website/docs/users-and-permissions.md`.
