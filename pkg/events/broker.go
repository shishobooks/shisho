package events

import (
	"fmt"
	"sync"
)

// EventTypeLogEntry is the type of events carrying server log lines. The
// stream handler delivers them only to users with Config Read, the same
// permission GET /api/logs requires.
const EventTypeLogEntry = "log.entry"

// Event represents a server-sent event with a named type and JSON data.
type Event struct {
	Type string
	Data string
}

// EventFilter reports whether a subscriber receives an event. A nil filter
// accepts every event.
type EventFilter func(evt Event) bool

// Broker fans out events to all subscribed SSE connections.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[chan Event]EventFilter
	// done is closed by Close() to broadcast "server is shutting down" to
	// every SSE handler. Without this signal, streaming handlers would wait
	// for the client to disconnect before returning, stalling srv.Shutdown
	// until its timeout expires.
	done chan struct{}
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[chan Event]EventFilter),
		done:        make(chan struct{}),
	}
}

// Done returns a channel that is closed when Close is called. SSE handlers
// select on this channel alongside the request context so they exit promptly
// during server shutdown instead of blocking until the client disconnects.
func (b *Broker) Done() <-chan struct{} {
	return b.done
}

// Close broadcasts shutdown to all current and future subscribers. Idempotent:
// safe to call multiple times.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case <-b.done:
		// already closed
	default:
		close(b.done)
	}
}

// Subscribe returns a channel that receives all future published events.
// The caller must call Unsubscribe when done.
func (b *Broker) Subscribe() chan Event {
	return b.SubscribeFiltered(nil)
}

// SubscribeFiltered returns a channel that receives the future published
// events accepted by filter. Rejected events are never queued, so they
// cannot fill the subscriber's buffer. The caller must call Unsubscribe when
// done.
func (b *Broker) SubscribeFiltered(filter EventFilter) chan Event {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subscribers[ch] = filter
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel and closes it.
func (b *Broker) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	delete(b.subscribers, ch)
	close(ch)
	b.mu.Unlock()
}

// Publish sends an event to all subscribers. Slow subscribers that have
// a full buffer will have this event dropped (non-blocking send).
func (b *Broker) Publish(evt Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch, filter := range b.subscribers {
		if filter != nil && !filter(evt) {
			continue
		}
		select {
		case ch <- evt:
		default:
			// Subscriber buffer full — drop event to avoid blocking.
		}
	}
}

// NewBulkDownloadProgressEvent builds a progress event for bulk download jobs.
func NewBulkDownloadProgressEvent(jobID int, status string, current, total int, estimatedSizeBytes int64) Event {
	data := fmt.Sprintf(
		`{"job_id":%d,"status":"%s","current":%d,"total":%d,"estimated_size_bytes":%d}`,
		jobID, status, current, total, estimatedSizeBytes,
	)
	return Event{Type: "bulk_download.progress", Data: data}
}

// NewJobEvent builds an Event with the standard job payload format.
func NewJobEvent(eventType string, jobID int, status, jobType string, libraryID *int) Event {
	data := fmt.Sprintf(`{"job_id":%d,"status":"%s","type":"%s"}`, jobID, status, jobType)
	if libraryID != nil {
		data = fmt.Sprintf(`{"job_id":%d,"status":"%s","type":"%s","library_id":%d}`, jobID, status, jobType, *libraryID)
	}
	return Event{Type: eventType, Data: data}
}
