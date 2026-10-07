# Admin Settings Store

`app_settings` (`pkg/appsettings`) holds admin-editable feature settings as one JSON document per key. A domain package owns its key, its struct, and load and save helpers that fall back to defaults when no row exists: `review.Load`/`review.Save` (`pkg/books/review`, key `review_criteria`) and `sharelinks.LoadSettings`/`sharelinks.SaveSettings` (key `sharing`).

**Where a new setting goes.** Policy an admin decides at runtime goes here; deployment facts go in `config.Config`. A feature switch that carries companion policy and a warning the admin must read belongs in the admin UI with no config field or env var, as sharing does (ADR 0008).

- Endpoints live under `/api/settings/*`. Writes require `config:write` and are rejected in Demo Mode by the global middleware; reads are GETs.
- The sharing endpoints are registered by `sharelinks.RegisterRoutes`, not `pkg/settings`, because `pkg/books` imports `pkg/settings` and `pkg/sharelinks` imports `pkg/books`.
- `PUT /api/settings/sharing` takes pointer fields so each switch saves on its own; the handler loads, merges, and saves outside a transaction. Two admins changing different switches at the same instant can lose one change, which is accepted for rarely edited settings. A document that gains frequent concurrent writers needs the load and save in one transaction.
- The books service reads app settings for the Reviewed recompute, which is why it is built `WithAppSettings` and shared (see "Shared services" in `pkg/AGENTS.md`).
- Cache policy (`pkg/cache`, key `cache`) uses the same store: `GET /api/settings/cache` needs Config Read and `PUT` needs Config Write. The server loads the saved policy before building the shared thumbnail cache, and updates serialize persistence with `SetMaxBytes` on that one cache. There is no config or env override for the limit. Defaults to 1 GiB, accepts fractional GiB, and zero keeps no thumbnails on disk.
