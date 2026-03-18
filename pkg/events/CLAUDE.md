# Server-Sent Events (SSE)

In-memory event broker and SSE streaming endpoint for pushing real-time updates to the frontend.

## Architecture

- **Broker** (`broker.go`): Goroutine-safe pub/sub fan-out using `sync.RWMutex` and buffered channels. Subscribers get a channel; publishers send to all channels. Slow subscribers (full buffer) have events dropped to avoid blocking.
- **Handler** (`handler.go`): Echo handler that opens a streaming HTTP response with `text/event-stream` content type, subscribes to the broker, and writes SSE-formatted lines until the client disconnects. Sends keepalive comments every 30 seconds to prevent proxy idle timeouts.
- **Routes** (`routes.go`): Registers `GET /events` with authentication middleware. No permission check beyond authentication — all authenticated users receive all events.

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

## Adding New Event Types

1. Define the event type name (e.g., `library.updated`)
2. Use `NewJobEvent()` if it's a job event, or construct `Event{Type: "...", Data: "..."}` directly
3. Call `broker.Publish(event)` from the appropriate place
4. Add a listener in `app/hooks/useSSE.ts` via `es.addEventListener("event.type", handler)`

## Frontend Integration

The `useSSE` hook (`app/hooks/useSSE.ts`) opens an `EventSource` to `/api/events` when authenticated. It listens for job events and invalidates relevant Tanstack Query caches so the UI updates instantly without polling.

## Authentication

`EventSource` doesn't support custom headers, but the backend uses cookie-based auth (`shisho_session`). Since `EventSource` sends cookies automatically on same-origin requests, authentication works out of the box.

## Proxy Configuration

The Caddy reverse proxy must have `flush_interval -1` to disable response buffering for SSE. Without this, events are buffered until the buffer fills, breaking real-time delivery.
