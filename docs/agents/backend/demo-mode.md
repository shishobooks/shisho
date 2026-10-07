# Demo Mode

`demo_mode` / `DEMO_MODE` (default `false`) turns the server into the read-only Public Demo. Enforcement is `pkg/server/demo_mode.go`, which runs globally through `e.Use` after logger and recovery and before authentication and handlers. It matches `c.Path()` patterns, not raw URLs, so keep it on `e.Use`; on `e.Pre` the route pattern is not yet known.

## Classification

Every route family and download path is allowed, denied, or unregistered. A new one must be classified here, with a middleware or route-registration test (`TestNew_DemoModeRoutes` is the reference), and a new GET or HEAD handler must not make persistent user changes.

- **Allowed:** `GET`, `HEAD`, and `OPTIONS`, subject to normal authentication and permissions, plus `POST /api/auth/login` and `POST /api/auth/logout`.
- **Rejected with 403:** every other method and path, including unknown write paths, with code `demo_mode` and message `This action is unavailable in the demo.` Admins have no bypass.
- **Denied downloads (`deniedDownloads`):** `GET` and `HEAD` for `/api/books/files/:id/download/original`, `/api/books/files/:id/download/kepub`, and `/api/jobs/:id/download`. `HEAD` is denied with `GET` because it still runs the handler, including KePub generation.
- **Still served:** the generated download (`/api/books/files/:id/download`), CBZ/PDF pages, and audio streaming. These hand out complete files for every main format (and the full M4B from `stream` without `Range`), so this is not copy protection. Supplements are reachable through the generated download, so the demo corpus must not include them.
- **Unregistered (GET returns 404, writes still get the global 403):** OPDS (`/opds/*`), eReader (`/ereader/*`, `/e/:shortCode`), Kobo (`/kobo/*`), public Share Links (`/api/share/*`), every `/api/plugins` group, per-library plugin routes (`/api/libraries/:id/plugins/*`), and test routes (`/api/test/*`, even with `SHISHO_TEST_MODE=true`).
- `GET /api/users/directory` is not registered, because it would show every visitor the admin's username. The path falls through to `GET /api/users/:id`, which needs Users Read, so the demo viewer gets 403. `GET /api/user/libraries` is an ordinary allowed read.
- Cache policy reads (`GET /api/settings/cache`, Config Read) are allowed; `PUT /api/settings/cache` hits the global 403.
- Share Link management stays registered: listing a book's links is a read, and create, revoke, and delete hit the global 403 (`TestShareLinks_ManagementRejectedInDemoMode`).

## Startup

`cmd/api/main.go` skips `pluginManager.LoadAll`, `wrkr.Start`, and `wrkr.Shutdown` (which waits for goroutines only `Start` creates). Reader caches and startup migrations still run.

`GET /api/auth/status` exposes the flag before sign-in. The boolean is passed to `auth.RegisterRoutes`, because importing `config` from `auth` creates an import cycle (config routes use auth middleware).
