# Server-Sent Events (SSE)

In-memory event broker and SSE streaming endpoint for pushing real-time updates to the frontend.

## Architecture

- **Broker** (`broker.go`): Goroutine-safe pub/sub fan-out using `sync.RWMutex` and buffered channels. Subscribers get a channel; publishers send to all channels. `SubscribeFiltered` attaches an `EventFilter`, and `Publish` skips subscribers whose filter rejects the event, so rejected events never take up buffer space. Slow subscribers (full buffer) have events dropped to avoid blocking.
- **Handler** (`handler.go`): Echo handler that opens a streaming HTTP response with `text/event-stream` content type, subscribes to the broker, and writes SSE-formatted lines until the client disconnects. Sends keepalive comments every 30 seconds to prevent proxy idle timeouts.
- **Routes** (`routes.go`): Registers `GET /api/events` with authentication middleware only. Per-event permissions are applied by the handler's subscription filter (see [Permissions](#permissions)).

## Event Format

Events follow the SSE spec with named event types:

```
event: job.created
data: {"job_id":1,"status":"pending","type":"scan"}

event: job.status_changed
data: {"job_id":1,"status":"in_progress","type":"scan","library_id":2}
```

Use `NewJobEvent()` to build job events consistently across callers (worker, HTTP handler). Use `NewBulkDownloadProgressEvent()` for bulk download progress events.

## Event Types

| Event | Published When | Publishers |
|-------|---------------|------------|
| `job.created` | Job created via API or scheduler | `pkg/jobs/handlers.go`, `pkg/worker/worker.go` (scheduler) |
| `job.status_changed` | Job transitions status (pending→in_progress, →completed, →failed) | `pkg/worker/worker.go` |
| `bulk_download.progress` | Bulk download file generation progress (per-file updates, zipping status) | `pkg/worker/bulk_download.go` |
| `log.entry` (`EventTypeLogEntry`) | Every server log line captured by the ring buffer | `pkg/logs/ring_buffer.go` |

## Permissions

The stream is shared, so the handler subscribes each connection with a filter built by `eventFilterFor(user)` from the user that `Authenticate` stores on the Echo context (`c.Get("user")`, role and permissions loaded):

- `log.entry` goes only to users with Config Read, the permission `GET /api/logs` requires. Log lines include request logs for other users and job output, so never broadcast them.
- Every other event (`job.*`, `bulk_download.progress`) goes to every authenticated user. Bulk download creators without Jobs Read rely on this to see their job complete (#523). These payloads carry ids, status, type, library id, and progress counts only. Never put log text or other permission-gated data in them.
- The stream handler must always subscribe with `SubscribeFiltered`. Plain `Subscribe()` receives every event and is only for tests.
- The filter is fixed when the connection opens. A role change takes effect when the client reconnects.

When adding an event type whose data needs a permission, gate it in `eventFilterFor` and add a case to its test and to `pkg/server/events_permissions_test.go`.

## Adding New Event Types

1. Define the event type name (e.g., `library.updated`)
2. Use `NewJobEvent()` if it's a job event, or construct `Event{Type: "...", Data: "..."}` directly
3. Call `broker.Publish(event)` from the appropriate place
4. If the data is permission-gated, filter it in `eventFilterFor` (see [Permissions](#permissions))
5. Add a listener in `app/hooks/useSSE.ts` via `es.addEventListener("event.type", handler)`

## Frontend Integration

The `useSSE` hook (`app/hooks/useSSE.ts`) opens an `EventSource` to `/api/events` when authenticated. It listens for job events and invalidates relevant Tanstack Query caches so the UI updates instantly without polling.

## Authentication

`EventSource` doesn't support custom headers, but the backend uses cookie-based auth (`shisho_session`). Since `EventSource` sends cookies automatically on same-origin requests, authentication works out of the box.

## Streaming and compression

The Go server skips gzip for `/api/events`. The handler flushes headers and every event directly to the client. Any external reverse proxy must also disable response buffering for this route, or events may arrive late.
