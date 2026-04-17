# Logging Refinement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> Check the project's root CLAUDE.md and any relevant subdirectory CLAUDE.md files for rules that apply to your work. These contain critical project conventions, gotchas, and requirements (e.g., docs update requirements, testing conventions, naming rules). Violations of these rules are review failures.

**Goal:** Disable Caddy access logs by default (re-enable via env var) and add an in-memory log viewer to the admin UI backed by a ring buffer with live SSE tail.

**Architecture:** A new `pkg/logs` package provides a `RingBuffer` that implements `io.Writer`, receiving JSON bytes from zerolog via `io.MultiWriter`. It parses entries into a circular slice, exposes them via a `GET /logs` endpoint, and publishes `log.entry` SSE events through the existing broker. The frontend adds a `/settings/logs` page with filtering and live tail.

**Tech Stack:** Go (zerolog, Echo), React (Tanstack Query, SSE EventSource), TailwindCSS

**Spec:** `docs/superpowers/specs/2026-04-17-logging-refinement-design.md`

---

### Task 1: Disable Caddy Access Logs by Default

**Files:**
- Modify: `Caddyfile:7`
- Modify: `website/docs/configuration.md` (add Docker/Caddy env vars section)

- [ ] **Step 1: Update Caddyfile**

In `Caddyfile`, change line 7 from:
```
        output stdout
```
to:
```
        output {$CADDY_ACCESS_LOG_OUTPUT:discard}
```

- [ ] **Step 2: Document in website docs**

In `website/docs/configuration.md`, add a new section before the "Authentication" section (before line 97). Add:

```markdown
### Docker / Caddy

These environment variables are only relevant when running Shisho in Docker, where Caddy serves as the reverse proxy.

| Env Variable | Default | Description |
|-------------|---------|-------------|
| `CADDY_ACCESS_LOG_OUTPUT` | `discard` | Caddy access log output. Set to `stdout` to enable access logs. Logs are disabled by default to reduce noise |
| `PUID` | `1000` | User ID for the Shisho process inside the container |
| `PGID` | `1000` | Group ID for the Shisho process inside the container |
| `STARTUP_TIMEOUT_SECONDS` | `120` | Seconds to wait for the backend to start before giving up. Increase for slow storage (e.g., NAS devices) |
| `LOG_FORMAT` | `console` | Log output format. Set to `json` for structured JSON logs (useful for log aggregation) |
```

- [ ] **Step 3: Commit**

```bash
git add Caddyfile website/docs/configuration.md
git commit -m "[Backend] Disable Caddy access logs by default"
```

---

### Task 2: Ring Buffer — Core Data Structure

**Files:**
- Create: `pkg/logs/ring_buffer.go`
- Create: `pkg/logs/ring_buffer_test.go`

- [ ] **Step 1: Write the failing test for RingBuffer.Write and Query**

Create `pkg/logs/ring_buffer_test.go`:

```go
package logs

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRingBuffer_WriteAndQuery(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	// Write a zerolog-formatted JSON line
	line := `{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"starting shisho","data":{"version":"0.0.31"}}` + "\n"
	n, err := rb.Write([]byte(line))
	require.NoError(t, err)
	assert.Equal(t, len(line), n)

	entries := rb.Query("", "", 100, 0)
	require.Len(t, entries, 1)
	assert.Equal(t, "info", entries[0].Level)
	assert.Equal(t, "starting shisho", entries[0].Message)
	assert.Equal(t, uint64(1), entries[0].ID)
	assert.Equal(t, "0.0.31", entries[0].Data["version"])
	assert.Nil(t, entries[0].Error)
}

func TestRingBuffer_ErrorField(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	line := `{"level":"error","timestamp":"2026-04-17T10:30:00Z","message":"db failed","error":"connection refused"}` + "\n"
	_, err := rb.Write([]byte(line))
	require.NoError(t, err)

	entries := rb.Query("", "", 100, 0)
	require.Len(t, entries, 1)
	assert.Equal(t, "error", entries[0].Level)
	require.NotNil(t, entries[0].Error)
	assert.Equal(t, "connection refused", *entries[0].Error)
}

func TestRingBuffer_Wrapping(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(3, nil)

	for i := 1; i <= 5; i++ {
		line := fmt.Sprintf(`{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"msg %d"}`, i) + "\n"
		_, err := rb.Write([]byte(line))
		require.NoError(t, err)
	}

	entries := rb.Query("", "", 100, 0)
	require.Len(t, entries, 3)
	// Should contain the 3 most recent entries (3, 4, 5)
	assert.Equal(t, "msg 3", entries[0].Message)
	assert.Equal(t, "msg 4", entries[1].Message)
	assert.Equal(t, "msg 5", entries[2].Message)
	// IDs should be 3, 4, 5
	assert.Equal(t, uint64(3), entries[0].ID)
	assert.Equal(t, uint64(5), entries[2].ID)
}

func TestRingBuffer_QueryAfterID(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	for i := 1; i <= 5; i++ {
		line := fmt.Sprintf(`{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"msg %d"}`, i) + "\n"
		_, err := rb.Write([]byte(line))
		require.NoError(t, err)
	}

	// Get entries after ID 3 — should return msg 4 and msg 5
	entries := rb.Query("", "", 100, 3)
	require.Len(t, entries, 2)
	assert.Equal(t, "msg 4", entries[0].Message)
	assert.Equal(t, "msg 5", entries[1].Message)
}

func TestRingBuffer_QueryLevelFilter(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	lines := []string{
		`{"level":"debug","timestamp":"2026-04-17T10:30:00Z","message":"debug msg"}` + "\n",
		`{"level":"info","timestamp":"2026-04-17T10:30:01Z","message":"info msg"}` + "\n",
		`{"level":"warn","timestamp":"2026-04-17T10:30:02Z","message":"warn msg"}` + "\n",
		`{"level":"error","timestamp":"2026-04-17T10:30:03Z","message":"error msg"}` + "\n",
	}
	for _, line := range lines {
		_, err := rb.Write([]byte(line))
		require.NoError(t, err)
	}

	// Filter by "warn" — should return warn and error
	entries := rb.Query("warn", "", 100, 0)
	require.Len(t, entries, 2)
	assert.Equal(t, "warn msg", entries[0].Message)
	assert.Equal(t, "error msg", entries[1].Message)
}

func TestRingBuffer_QuerySearchFilter(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	lines := []string{
		`{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"starting shisho"}` + "\n",
		`{"level":"info","timestamp":"2026-04-17T10:30:01Z","message":"database connected"}` + "\n",
		`{"level":"info","timestamp":"2026-04-17T10:30:02Z","message":"server started"}` + "\n",
	}
	for _, line := range lines {
		_, err := rb.Write([]byte(line))
		require.NoError(t, err)
	}

	// Case-insensitive search
	entries := rb.Query("", "DATABASE", 100, 0)
	require.Len(t, entries, 1)
	assert.Equal(t, "database connected", entries[0].Message)
}

func TestRingBuffer_QueryLimit(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	for i := 1; i <= 10; i++ {
		line := fmt.Sprintf(`{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"msg %d"}`, i) + "\n"
		_, err := rb.Write([]byte(line))
		require.NoError(t, err)
	}

	// Limit to 3 — should return the 3 most recent
	entries := rb.Query("", "", 3, 0)
	require.Len(t, entries, 3)
	assert.Equal(t, "msg 8", entries[0].Message)
	assert.Equal(t, "msg 10", entries[2].Message)
}

func TestRingBuffer_MultiLineWrite(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	// zerolog can batch multiple lines in a single Write call
	batch := `{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"first"}` + "\n" +
		`{"level":"info","timestamp":"2026-04-17T10:30:01Z","message":"second"}` + "\n"
	_, err := rb.Write([]byte(batch))
	require.NoError(t, err)

	entries := rb.Query("", "", 100, 0)
	require.Len(t, entries, 2)
	assert.Equal(t, "first", entries[0].Message)
	assert.Equal(t, "second", entries[1].Message)
}

func TestRingBuffer_InvalidJSON(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	// Invalid JSON should be silently dropped
	_, err := rb.Write([]byte("not json\n"))
	require.NoError(t, err)

	entries := rb.Query("", "", 100, 0)
	assert.Len(t, entries, 0)
}

func TestRingBuffer_EmptyBufferQuery(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	entries := rb.Query("", "", 100, 0)
	assert.Len(t, entries, 0)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/logging && go test ./pkg/logs/... -v`
Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Implement RingBuffer**

Create `pkg/logs/ring_buffer.go`:

```go
package logs

import (
	"bufio"
	"bytes"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/events"
)

// LogEntry represents a parsed log entry stored in the ring buffer.
type LogEntry struct {
	ID        uint64         `json:"id"`
	Level     string         `json:"level"`
	Timestamp time.Time      `json:"timestamp"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data,omitempty"`
	Error     *string        `json:"error,omitempty"`
}

// levelSeverity maps level strings to numeric severity for filtering.
var levelSeverity = map[string]int{
	"debug": 0,
	"info":  1,
	"warn":  2,
	"error": 3,
	"fatal": 4,
}

// RingBuffer captures zerolog JSON output into a fixed-size circular buffer.
// It implements io.Writer so it can be used with io.MultiWriter.
type RingBuffer struct {
	mu       sync.RWMutex
	entries  []LogEntry
	capacity int
	head     int // next write position
	count    int // number of entries currently stored
	nextID   atomic.Uint64
	broker   *events.Broker
}

// NewRingBuffer creates a ring buffer that stores up to capacity log entries.
// If broker is non-nil, each new entry is published as a "log.entry" SSE event.
func NewRingBuffer(capacity int, broker *events.Broker) *RingBuffer {
	return &RingBuffer{
		entries:  make([]LogEntry, capacity),
		capacity: capacity,
		broker:   broker,
	}
}

// Write implements io.Writer. It receives raw JSON bytes from zerolog,
// splits by newline, parses each line into a LogEntry, and stores it.
func (rb *RingBuffer) Write(p []byte) (int, error) {
	scanner := bufio.NewScanner(bytes.NewReader(p))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		entry, ok := parseLogLine(line)
		if !ok {
			continue
		}

		entry.ID = rb.nextID.Add(1)

		rb.mu.Lock()
		rb.entries[rb.head] = entry
		rb.head = (rb.head + 1) % rb.capacity
		if rb.count < rb.capacity {
			rb.count++
		}
		rb.mu.Unlock()

		if rb.broker != nil {
			data, err := json.Marshal(entry)
			if err == nil {
				rb.broker.Publish(events.Event{Type: "log.entry", Data: string(data)})
			}
		}
	}
	return len(p), nil
}

// Query returns log entries matching the given filters.
// level: minimum severity ("debug", "info", "warn", "error"). Empty = all.
// search: case-insensitive substring match on message. Empty = all.
// limit: max entries to return. 0 = no limit.
// afterID: return entries with ID > afterID. 0 = all.
func (rb *RingBuffer) Query(level, search string, limit int, afterID uint64) []LogEntry {
	minSeverity := -1
	if level != "" {
		if s, ok := levelSeverity[level]; ok {
			minSeverity = s
		}
	}
	searchLower := strings.ToLower(search)

	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.count == 0 {
		return []LogEntry{}
	}

	// Collect matching entries
	var matched []LogEntry
	start := (rb.head - rb.count + rb.capacity) % rb.capacity

	for i := 0; i < rb.count; i++ {
		idx := (start + i) % rb.capacity
		entry := rb.entries[idx]

		if entry.ID <= afterID {
			continue
		}
		if minSeverity >= 0 {
			if s, ok := levelSeverity[entry.Level]; !ok || s < minSeverity {
				continue
			}
		}
		if searchLower != "" && !strings.Contains(strings.ToLower(entry.Message), searchLower) {
			continue
		}
		matched = append(matched, entry)
	}

	// Apply limit — return the most recent entries
	if limit > 0 && len(matched) > limit {
		matched = matched[len(matched)-limit:]
	}

	return matched
}

// rawLogEntry is the intermediate struct for parsing zerolog JSON.
type rawLogEntry struct {
	Level     string         `json:"level"`
	Timestamp string         `json:"timestamp"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data,omitempty"`
	Error     *string        `json:"error,omitempty"`
}

func parseLogLine(line []byte) (LogEntry, bool) {
	var raw rawLogEntry
	if err := json.Unmarshal(line, &raw); err != nil {
		return LogEntry{}, false
	}
	if raw.Message == "" && raw.Level == "" {
		return LogEntry{}, false
	}

	ts, _ := time.Parse(time.RFC3339, raw.Timestamp)

	return LogEntry{
		Level:     raw.Level,
		Timestamp: ts,
		Message:   raw.Message,
		Data:      raw.Data,
		Error:     raw.Error,
	}, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/logging && go test ./pkg/logs/... -v`
Expected: All 9 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/logs/ring_buffer.go pkg/logs/ring_buffer_test.go
git commit -m "[Backend] Add in-memory log ring buffer"
```

---

### Task 3: API Endpoint and Route Registration

**Files:**
- Create: `pkg/logs/handlers.go`
- Create: `pkg/logs/validators.go`
- Create: `pkg/logs/routes.go`
- Modify: `pkg/server/server.go` (add logs route registration)

- [ ] **Step 1: Create validators**

Create `pkg/logs/validators.go`:

```go
package logs

// ListLogsQuery defines query parameters for GET /logs.
type ListLogsQuery struct {
	Level   *string `query:"level" json:"level,omitempty" validate:"omitempty,oneof=debug info warn error"`
	Search  *string `query:"search" json:"search,omitempty"`
	Limit   *int    `query:"limit" json:"limit,omitempty" validate:"omitempty,min=1,max=1000"`
	AfterID *uint64 `query:"after_id" json:"after_id,omitempty"`
}
```

- [ ] **Step 2: Create handler**

Create `pkg/logs/handlers.go`:

```go
package logs

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

type handler struct {
	buffer *RingBuffer
}

func (h *handler) listLogs(c echo.Context) error {
	params := ListLogsQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	level := ""
	if params.Level != nil {
		level = *params.Level
	}
	search := ""
	if params.Search != nil {
		search = *params.Search
	}
	limit := 100
	if params.Limit != nil {
		limit = *params.Limit
	}
	var afterID uint64
	if params.AfterID != nil {
		afterID = *params.AfterID
	}

	entries := h.buffer.Query(level, search, limit, afterID)

	resp := struct {
		Entries []LogEntry `json:"entries"`
	}{Entries: entries}

	return errors.WithStack(c.JSON(http.StatusOK, resp))
}
```

- [ ] **Step 3: Create routes**

Create `pkg/logs/routes.go`:

```go
package logs

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
)

// RegisterRoutes registers the log viewer endpoint.
func RegisterRoutes(e *echo.Echo, buffer *RingBuffer, authMiddleware *auth.Middleware) {
	h := &handler{buffer: buffer}
	e.GET("/logs", h.listLogs,
		authMiddleware.Authenticate,
		authMiddleware.RequirePermission(models.ResourceConfig, models.OperationRead),
	)
}
```

- [ ] **Step 4: Wire into server.go**

In `pkg/server/server.go`, add the `logs` import and pass the ring buffer through.

Add import:
```go
"github.com/shishobooks/shisho/pkg/logs"
```

Update the `New` function signature from:
```go
func New(cfg *config.Config, db *bun.DB, w *worker.Worker, pm *plugins.Manager, broker *events.Broker, dlCache *downloadcache.Cache) (*http.Server, error) {
```
to:
```go
func New(cfg *config.Config, db *bun.DB, w *worker.Worker, pm *plugins.Manager, broker *events.Broker, dlCache *downloadcache.Cache, logBuffer *logs.RingBuffer) (*http.Server, error) {
```

Add route registration after the SSE event stream registration (after line 104):
```go
	// Log viewer endpoint (admin only)
	logs.RegisterRoutes(e, logBuffer, authMiddleware)
```

- [ ] **Step 5: Commit**

```bash
git add pkg/logs/handlers.go pkg/logs/validators.go pkg/logs/routes.go pkg/server/server.go
git commit -m "[Backend] Add GET /logs endpoint with filtering"
```

---

### Task 4: Wire Ring Buffer in main.go and Update golib

**Files:**
- Modify: `cmd/api/main.go`
- Modify: `go.mod` / `go.sum` (update golib)

- [ ] **Step 1: Update golib dependency**

```bash
cd /Users/robinjoseph/.worktrees/shisho/logging
go get github.com/robinjoseph08/golib@v0.5.2
go mod tidy
```

- [ ] **Step 2: Wire the ring buffer in main.go**

In `cmd/api/main.go`, add import:
```go
"io"
"github.com/shishobooks/shisho/pkg/logs"
```

The broker is created on line 71 (`broker := events.NewBroker()`). Add the ring buffer setup immediately after the broker creation, and move `log := logger.New()` after the ring buffer setup. The beginning of `main()` changes from:

```go
func main() {
	ctx := context.Background()
	log := logger.New()

	log.Info("starting shisho", logger.Data{"version": version.Version})

	cfg, err := config.New()
	// ... lines through broker creation ...
	broker := events.NewBroker()
```

to:

```go
func main() {
	ctx := context.Background()

	broker := events.NewBroker()
	logBuffer := logs.NewRingBuffer(10_000, broker)
	logger.SetOutput(io.MultiWriter(logger.Output(), logBuffer))
	log := logger.New()

	log.Info("starting shisho", logger.Data{"version": version.Version})

	cfg, err := config.New()
	// ... lines that previously created broker now removed from here ...
```

This requires moving the `broker` creation to before `logger.New()` — it was previously on line 71 after config, DB, and plugin setup. The broker has no dependencies (it's just `events.NewBroker()`), so it's safe to move to the top.

Also update the `server.New` call to pass `logBuffer`:

```go
srv, err := server.New(cfg, db, wrkr, pluginManager, broker, dlCache, logBuffer)
```

- [ ] **Step 3: Verify the project builds**

```bash
cd /Users/robinjoseph/.worktrees/shisho/logging
go build ./...
```

- [ ] **Step 4: Run Go tests**

```bash
mise test
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add cmd/api/main.go go.mod go.sum
git commit -m "[Backend] Wire log ring buffer into main startup"
```

---

### Task 5: Frontend — Query Hook and SSE Listener

**Files:**
- Create: `app/hooks/queries/logs.ts`
- Modify: `app/hooks/useSSE.ts` (add `log.entry` listener)

- [ ] **Step 1: Create the logs query hook**

Create `app/hooks/queries/logs.ts`:

```typescript
import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";

export enum QueryKey {
  ListLogs = "ListLogs",
}

export interface LogEntry {
  id: number;
  level: string;
  timestamp: string;
  message: string;
  data?: Record<string, unknown>;
  error?: string;
}

interface ListLogsData {
  entries: LogEntry[];
}

interface UseLogsOptions {
  level?: string;
  search?: string;
  limit?: number;
  afterId?: number;
}

export const useLogs = (
  options: UseLogsOptions = {},
  queryOptions: Omit<
    UseQueryOptions<ListLogsData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListLogsData, ShishoAPIError>({
    ...queryOptions,
    queryKey: [QueryKey.ListLogs, options],
    queryFn: ({ signal }) => {
      const params: Record<string, string> = {};
      if (options.level) {
        params.level = options.level;
      }
      if (options.search) {
        params.search = options.search;
      }
      if (options.limit !== undefined) {
        params.limit = String(options.limit);
      }
      if (options.afterId !== undefined) {
        params.after_id = String(options.afterId);
      }
      return API.request("GET", "/logs", null, params, signal);
    },
  });
};
```

- [ ] **Step 2: Add log.entry SSE listener**

In `app/hooks/useSSE.ts`, the `log.entry` event is broadcast to all authenticated SSE connections but only needs to be consumed by the logs page. The existing `useSSE` hook handles job events globally. For logs, the page will listen directly since it needs local state management (appending to a list). No changes to `useSSE.ts` are needed — the log page will add its own `addEventListener` on the shared `EventSource`. 

Actually, since the current `useSSE` creates the `EventSource` inside the hook and doesn't expose it, the logs page will consume `log.entry` events by invalidating the query cache. Add a listener in `useSSE.ts`:

After the `handleBulkDownloadProgress` listener (line 104), add:

```typescript
    const handleLogEntry = () => {
      queryClient.invalidateQueries({ queryKey: [LogsQueryKey.ListLogs] });
    };

    es.addEventListener("log.entry", handleLogEntry);
```

Add the import at the top:
```typescript
import { QueryKey as LogsQueryKey } from "@/hooks/queries/logs";
```

And add cleanup in the return function:
```typescript
      es.removeEventListener("log.entry", handleLogEntry);
```

**NOTE:** This approach is simple but means every log event triggers a refetch. Given the logs page will debounce queries or the user won't always be on the logs page, this is acceptable. The query won't actually fire unless there's an active `useLogs` query mounted.

- [ ] **Step 3: Commit**

```bash
git add app/hooks/queries/logs.ts app/hooks/useSSE.ts
git commit -m "[Frontend] Add logs query hook and SSE listener"
```

---

### Task 6: Frontend — Admin Logs Page

**Files:**
- Create: `app/components/pages/AdminLogs.tsx`
- Modify: `app/router.tsx` (add route)
- Modify: `app/components/pages/AdminLayout.tsx` (add sidebar item)

- [ ] **Step 1: Create the AdminLogs page**

Create `app/components/pages/AdminLogs.tsx`:

```tsx
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { type LogEntry, useLogs } from "@/hooks/queries/logs";
import { usePageTitle } from "@/hooks/usePageTitle";

const INITIAL_LIMIT = 200;

const levelColors: Record<string, string> = {
  debug:
    "bg-gray-100 text-gray-700 dark:bg-gray-800/50 dark:text-gray-400",
  info: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  warn: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
  error: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
  fatal: "bg-red-200 text-red-900 dark:bg-red-900/50 dark:text-red-300",
};

const formatTimestamp = (ts: string): string => {
  try {
    const d = new Date(ts);
    return d.toLocaleTimeString("en-US", {
      hour12: false,
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  } catch {
    return ts;
  }
};

interface LogRowProps {
  entry: LogEntry;
}

const LogRow = ({ entry }: LogRowProps) => {
  const [expanded, setExpanded] = useState(false);
  const hasExtra = (entry.data && Object.keys(entry.data).length > 0) || entry.error;

  return (
    <div
      className={`px-3 py-1.5 font-mono text-xs hover:bg-muted/50 transition-colors ${hasExtra ? "cursor-pointer" : ""}`}
      onClick={hasExtra ? () => setExpanded(!expanded) : undefined}
    >
      <div className="flex items-start gap-2">
        <span className="text-muted-foreground shrink-0 tabular-nums">
          {formatTimestamp(entry.timestamp)}
        </span>
        <Badge
          className={`${levelColors[entry.level] ?? levelColors.info} text-[10px] px-1.5 py-0 shrink-0 font-mono uppercase`}
          variant="secondary"
        >
          {entry.level.slice(0, 3)}
        </Badge>
        <span className="text-foreground break-all">{entry.message}</span>
      </div>
      {expanded && (
        <div className="mt-1 ml-[7.5rem] space-y-1">
          {entry.error && (
            <div className="text-red-500 dark:text-red-400">
              error: {entry.error}
            </div>
          )}
          {entry.data && Object.keys(entry.data).length > 0 && (
            <pre className="text-muted-foreground whitespace-pre-wrap break-all">
              {JSON.stringify(entry.data, null, 2)}
            </pre>
          )}
        </div>
      )}
    </div>
  );
};

const AdminLogs = () => {
  usePageTitle("Server Logs");

  const [level, setLevel] = useState<string>("");
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const scrollRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const prevEntriesRef = useRef<LogEntry[]>([]);

  // Debounce search input
  useEffect(() => {
    const timer = setTimeout(() => setSearch(searchInput), 300);
    return () => clearTimeout(timer);
  }, [searchInput]);

  const { data, isLoading } = useLogs({
    level: level || undefined,
    search: search || undefined,
    limit: INITIAL_LIMIT,
  });

  const entries = useMemo(() => data?.entries ?? [], [data]);

  // Auto-scroll to bottom when new entries arrive (if user is at the bottom)
  useEffect(() => {
    if (autoScroll && scrollRef.current && entries.length > 0) {
      // Only scroll if entries actually changed
      if (entries !== prevEntriesRef.current) {
        scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
        prevEntriesRef.current = entries;
      }
    }
  }, [entries, autoScroll]);

  const handleScroll = useCallback(() => {
    if (!scrollRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = scrollRef.current;
    // Consider "at bottom" if within 50px of the bottom
    setAutoScroll(scrollHeight - scrollTop - clientHeight < 50);
  }, []);

  const jumpToLatest = useCallback(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
      setAutoScroll(true);
    }
  }, []);

  if (isLoading) {
    return <LoadingSpinner />;
  }

  return (
    <div className="flex flex-col h-[calc(100vh-12rem)]">
      <div className="mb-4">
        <h1 className="text-xl md:text-2xl font-semibold mb-1 md:mb-2">
          Server Logs
        </h1>
        <p className="text-sm md:text-base text-muted-foreground">
          Real-time server logs from the current session.
        </p>
      </div>

      {/* Filter bar */}
      <div className="flex items-center gap-3 mb-4">
        <Select onValueChange={(v) => setLevel(v === "all" ? "" : v)} value={level || "all"}>
          <SelectTrigger className="w-32">
            <SelectValue placeholder="All levels" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All levels</SelectItem>
            <SelectItem value="debug">Debug</SelectItem>
            <SelectItem value="info">Info</SelectItem>
            <SelectItem value="warn">Warn</SelectItem>
            <SelectItem value="error">Error</SelectItem>
          </SelectContent>
        </Select>
        <Input
          className="max-w-xs"
          onChange={(e) => setSearchInput(e.target.value)}
          placeholder="Search messages..."
          value={searchInput}
        />
      </div>

      {/* Log list */}
      <div className="relative flex-1 min-h-0">
        <div
          className="absolute inset-0 overflow-y-auto border border-border rounded-md bg-muted/20 dark:bg-neutral-950/50"
          onScroll={handleScroll}
          ref={scrollRef}
        >
          {entries.length === 0 ? (
            <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
              {search || level ? "No logs matching filters." : "No logs yet."}
            </div>
          ) : (
            <div className="divide-y divide-border/50">
              {entries.map((entry) => (
                <LogRow entry={entry} key={entry.id} />
              ))}
            </div>
          )}
        </div>

        {/* Jump to latest button */}
        {!autoScroll && entries.length > 0 && (
          <div className="absolute bottom-4 left-1/2 -translate-x-1/2">
            <Button
              className="shadow-lg"
              onClick={jumpToLatest}
              size="sm"
              variant="secondary"
            >
              Jump to latest
            </Button>
          </div>
        )}
      </div>
    </div>
  );
};

export default AdminLogs;
```

- [ ] **Step 2: Add route in router.tsx**

In `app/router.tsx`, add import:
```typescript
import AdminLogs from "@/components/pages/AdminLogs";
```

Add the route inside the `/settings` children array, after the plugins route (after line 148):
```typescript
      {
        path: "logs",
        element: (
          <ProtectedRoute
            requiredPermission={{ resource: "config", operation: "read" }}
          >
            <AdminLogs />
          </ProtectedRoute>
        ),
      },
```

- [ ] **Step 3: Add sidebar item in AdminLayout.tsx**

In `app/components/pages/AdminLayout.tsx`, add the `ScrollText` import:
```typescript
import {
  Briefcase,
  Cog,
  Library,
  LogOut,
  Menu,
  Puzzle,
  ScrollText,
  Users,
} from "lucide-react";
```

Add the "Logs" nav item to the `mobileNavItems` array, after the Plugins entry (after line 178):
```typescript
    {
      to: "/settings/logs",
      icon: <ScrollText className="h-4 w-4" />,
      label: "Logs",
      isActive: location.pathname === "/settings/logs",
      show: canViewConfig,
    },
```

Add the desktop sidebar `NavItem` after the Plugins NavItem (after line 248):
```tsx
              {canViewConfig && (
                <NavItem
                  icon={<ScrollText className="h-4 w-4" />}
                  isActive={location.pathname === "/settings/logs"}
                  label="Logs"
                  to="/settings/logs"
                />
              )}
```

- [ ] **Step 4: Verify frontend builds**

```bash
cd /Users/robinjoseph/.worktrees/shisho/logging
pnpm build
```

Expected: Build succeeds.

- [ ] **Step 5: Commit**

```bash
git add app/components/pages/AdminLogs.tsx app/router.tsx app/components/pages/AdminLayout.tsx
git commit -m "[Frontend] Add server logs page with live tail and filtering"
```

---

### Task 7: Manual Verification

**Files:** None (testing only)

- [ ] **Step 1: Start the dev server**

```bash
cd /Users/robinjoseph/.worktrees/shisho/logging
mise start
```

- [ ] **Step 2: Verify Caddy log suppression (Docker only)**

If testing in Docker, verify no access logs appear on stdout by default. Set `CADDY_ACCESS_LOG_OUTPUT=stdout` and verify access logs return.

- [ ] **Step 3: Verify logs page**

1. Log in as admin
2. Navigate to Settings > Logs
3. Verify log entries appear (startup logs should be visible)
4. Trigger a library scan to generate more log entries
5. Verify new entries appear in real-time via SSE
6. Verify level filter works (select "Error" — only error/fatal entries shown)
7. Verify search filter works (type "shisho" — only matching entries shown)
8. Scroll up — verify auto-scroll pauses and "Jump to latest" button appears
9. Click "Jump to latest" — verify scroll jumps to bottom and auto-scroll resumes

- [ ] **Step 4: Verify non-admin access is denied**

1. Log in as a viewer role user
2. Navigate to `/settings/logs` directly
3. Verify access is denied (403 or redirect)
4. Verify the "Logs" nav item is not visible in the sidebar

- [ ] **Step 5: Run full checks**

```bash
mise check:quiet
```

Expected: All checks pass.

- [ ] **Step 6: Commit any fixes from testing**

If any issues were found and fixed during manual testing, commit them.
