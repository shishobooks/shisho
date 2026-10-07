# Server-Sent Events (SSE)

In-memory pub/sub broker (`broker.go`) and the `GET /api/events` stream (`handler.go`, `routes.go`). Build events with `NewJobEvent` (`job.created`, `job.status_changed`) and `NewBulkDownloadProgressEvent` (`bulk_download.progress`) so every publisher sends the same shape; `log.entry` (`EventTypeLogEntry`) comes from the log ring buffer. The frontend consumer is `app/hooks/useSSE.ts`, which invalidates Tanstack Query caches per event.

## Broker

- Publishing never blocks: a subscriber whose buffered channel is full loses the event.
- `SubscribeFiltered` attaches an `EventFilter`, and `Publish` skips subscribers whose filter rejects an event, so rejected events never take buffer space.
- The handler sends a keepalive comment every 30 seconds so proxies do not close idle streams. The Go server skips gzip for `/api/events` and flushes every event; an external reverse proxy must also disable buffering for this route.
- `EventSource` cannot send headers, so the stream relies on the session cookie (`shisho_session`, or `shisho_session_<namespace>` when `shisho_cookie_namespace` is set), which same-origin requests send automatically.

## Permissions

The route has authentication only; per-event permission is the subscription filter built by `eventFilterFor(user)` from the user `Authenticate` stored (read with `auth.RequireUser`; no user is a 401 before headers are written).

- `log.entry` goes only to users with Config Read, the permission `GET /api/logs` requires. Log lines include other users' requests and job output; never broadcast them.
- Every other event (`job.*`, `bulk_download.progress`) goes to every authenticated user, because bulk download creators without Jobs Read must see their job finish. These payloads carry only ids, status, type, library id, and progress counts; never put log text or other permission-gated data in them.
- The stream must subscribe with `SubscribeFiltered`. Plain `Subscribe()` receives everything and is for tests.
- The filter is fixed when the connection opens; a role change applies on reconnect.

## Adding an event type

Publish it with `broker.Publish`, add a listener in `app/hooks/useSSE.ts`, and if its data needs a permission, gate it in `eventFilterFor` with a case in its test and in `pkg/server/events_permissions_test.go`.
