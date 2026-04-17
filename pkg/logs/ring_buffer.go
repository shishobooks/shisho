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
