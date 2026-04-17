package events

import (
	"fmt"
	"sync"
)

// Event represents a server-sent event with a named type and JSON data.
type Event struct {
	Type string
	Data string
}

// Broker fans out events to all subscribed SSE connections.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
	// done is closed by Close() to broadcast "server is shutting down" to
	// every SSE handler. Without this signal, streaming handlers would wait
	// for the client to disconnect before returning, stalling srv.Shutdown
	// until its timeout expires.
	done chan struct{}
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[chan Event]struct{}),
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
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
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
	for ch := range b.subscribers {
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
