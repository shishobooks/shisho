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

	// Collect matching entries in chronological order
	matched := []LogEntry{}
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

// knownFields are top-level zerolog fields that are extracted into dedicated
// LogEntry fields (or intentionally excluded like hostname). Everything else
// is merged into Data so root-level fields from log.Root() are visible.
var knownFields = map[string]bool{
	"level": true, "timestamp": true, "message": true,
	"data": true, "error": true, "hostname": true, "stack": true,
}

// skipRoutes are API routes whose "request handled" entries are excluded
// from the ring buffer to prevent feedback loops (e.g. the logs page
// fetching /logs generates a log entry that triggers an SSE event that
// causes another fetch).
var skipRoutes = map[string]bool{
	"/logs":   true,
	"/events": true,
}

func parseLogLine(line []byte) (LogEntry, bool) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return LogEntry{}, false
	}

	level, _ := raw["level"].(string)
	message, _ := raw["message"].(string)
	if level == "" && message == "" {
		return LogEntry{}, false
	}

	// Skip log viewer and SSE routes to prevent feedback loops.
	if route, ok := raw["route"].(string); ok && skipRoutes[route] {
		return LogEntry{}, false
	}

	timestampStr, _ := raw["timestamp"].(string)
	ts, _ := time.Parse(time.RFC3339, timestampStr)

	var errStr *string
	if e, ok := raw["error"].(string); ok {
		errStr = &e
	}

	// Build data from nested "data" object + root-level extras.
	data := make(map[string]any)
	if nested, ok := raw["data"].(map[string]any); ok {
		for k, v := range nested {
			data[k] = v
		}
	}
	for k, v := range raw {
		if !knownFields[k] {
			data[k] = v
		}
	}
	if len(data) == 0 {
		data = nil
	}

	return LogEntry{
		Level:     level,
		Timestamp: ts,
		Message:   message,
		Data:      data,
		Error:     errStr,
	}, true
}
