# Server-Sent Events (SSE)

In-memory pub/sub broker and the `GET /api/events` stream. The frontend consumer is `app/hooks/useSSE.ts`, which invalidates Tanstack Query caches per event.

## Permissions

The route requires authentication only; per-event permission is the subscription filter built by `eventFilterFor(user)`. Every event it does not gate goes to every authenticated user, so an ungated payload carries only ids, status, type, library id, and progress counts: never log text or other permission-gated data.

## Adding an event type

Build it with a constructor beside `NewJobEvent` so every publisher sends the same shape, publish it with `broker.Publish`, and add a listener in `app/hooks/useSSE.ts`. If its data needs a permission, gate it in `eventFilterFor` with a case in its test and in `pkg/server/events_permissions_test.go`.
