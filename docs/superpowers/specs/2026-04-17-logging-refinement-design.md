# Logging Refinement Design

## Overview

Two changes to Shisho's logging:
1. **Disable Caddy access logs by default** with an env var to re-enable
2. **In-memory log viewer** — backend captures its own logs in a ring buffer, exposed via API + SSE for a live-tail UI in admin settings

## 1. Caddy Log Suppression

### Change

In `Caddyfile`, replace:
```
output stdout
```
with:
```
output {$CADDY_ACCESS_LOG_OUTPUT:discard}
```

Caddy's `discard` writer silently drops all access log entries. Users set `CADDY_ACCESS_LOG_OUTPUT=stdout` in their Docker compose to re-enable.

### Documentation

`CADDY_ACCESS_LOG_OUTPUT` must be documented in `website/docs/configuration.md` alongside other environment variables. This is a Docker/Caddy-only env var — it does not touch Go config.

## 2. Ring Buffer (`pkg/logs`)

### golib Dependency

Requires `github.com/robinjoseph08/golib` v0.5.2+ which exports `logger.SetOutput()` and `logger.Output()`.

### RingBuffer

A new `pkg/logs` package with a `RingBuffer` type that:
- Implements `io.Writer`
- Receives raw JSON bytes from zerolog (via `io.MultiWriter`)
- Parses each JSON line into a `LogEntry`
- Stores entries in a fixed-size circular slice (10,000 entries)
- Thread-safe via `sync.RWMutex`
- Publishes `log.entry` SSE events via the event broker on each write

### LogEntry

```go
type LogEntry struct {
    ID        uint64         `json:"id"`         // monotonically increasing sequence number
    Level     string         `json:"level"`      // "debug", "info", "warn", "error", "fatal"
    Timestamp time.Time      `json:"timestamp"`
    Message   string         `json:"message"`
    Data      map[string]any `json:"data"`       // nested data field from zerolog
    Error     *string        `json:"error"`      // error string if present
}
```

`ID` is a monotonically increasing uint64 assigned on write. Used as a cursor for SSE catch-up and pagination.

### Query Method

```go
func (rb *RingBuffer) Query(level, search string, limit int, afterID uint64) []LogEntry
```

- `level` — minimum severity filter
- `search` — case-insensitive substring match on message
- `limit` — max entries to return
- `afterID` — return only entries with ID > afterID

**Performance optimization:** Since entries are ordered by ID, `afterID` filtering uses ring position arithmetic to jump directly to the right slot rather than scanning from the start. The common case (SSE catch-up with a recent afterID) only scans the handful of new entries. Full-buffer scans (initial page load with no afterID) are still sub-millisecond for 10k entries.

### Constructor

```go
func NewRingBuffer(capacity int, broker *events.Broker) *RingBuffer
```

The broker is used to publish `log.entry` SSE events on each write.

## 3. Wiring in `main.go`

Three lines before `logger.New()`:

```go
logBuffer := logs.NewRingBuffer(10_000, broker)
logger.SetOutput(io.MultiWriter(logger.Output(), logBuffer))
log := logger.New()
```

`logBuffer` is passed to `server.New()` alongside existing parameters.

## 4. API Endpoint

### `GET /logs`

Registered in `pkg/logs/routes.go`. Protected with `Authenticate` + `RequirePermission(config, read)`.

**Query params:**

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `level` | string | (all) | Minimum severity: "debug", "info", "warn", "error" |
| `search` | string | (none) | Case-insensitive substring match on message |
| `limit` | int | 100 | Max entries to return (max 1000) |
| `after_id` | uint64 | 0 | Return entries with ID > this value |

**Response:**
```json
{
  "entries": [
    {
      "id": 42,
      "level": "info",
      "timestamp": "2026-04-17T10:30:00Z",
      "message": "starting shisho",
      "data": {"version": "0.0.31"},
      "error": null
    }
  ]
}
```

### SSE Event

`log.entry` events are published to the existing broker on every `Write()` call. Event data is the same JSON shape as a single entry. The frontend uses the last seen `id` as a cursor for catch-up on reconnect.

## 5. Frontend — Admin Logs Page

### Route

`/settings/logs` with `config:read` permission, following existing admin page patterns in `app/router.tsx`.

### Sidebar

New "Logs" nav item in `AdminLayout.tsx` with a lucide icon (e.g. `ScrollText`), placed after "Jobs" in nav order. Gated behind `canViewConfig`.

### Page (`AdminLogs.tsx`)

- **Page title:** "Server Logs" via `usePageTitle`
- **Header:** Title and description ("Real-time server logs from the current session")
- **Filter bar:** Level dropdown (All / Debug / Info / Warn / Error) + search text input, side by side
- **Log list:** Scrollable container with monospace text. Each entry shows:
  - Timestamp (compact format)
  - Colored level badge
  - Message
  - Expandable data/error if present
- **Auto-tail:** Subscribes to `log.entry` SSE events via the existing `EventSource` at `/api/events`. New entries append to the bottom. Auto-scroll follows the tail if the user is at the bottom; pauses if the user scrolls up (with a "Jump to latest" button).
- **Initial load:** `GET /api/logs?limit=200` with current filters to populate history. Subsequent entries arrive via SSE.
- **Filter changes:** Re-fetch from the HTTP endpoint with new filters. SSE entries are filtered client-side to match current filters.

No unsaved changes protection needed — read-only page.
