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
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[chan Event]struct{}),
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

// NewJobEvent builds an Event with the standard job payload format.
func NewJobEvent(eventType string, jobID int, status, jobType string, libraryID *int) Event {
	data := fmt.Sprintf(`{"job_id":%d,"status":"%s","type":"%s"}`, jobID, status, jobType)
	if libraryID != nil {
		data = fmt.Sprintf(`{"job_id":%d,"status":"%s","type":"%s","library_id":%d}`, jobID, status, jobType, *libraryID)
	}
	return Event{Type: eventType, Data: data}
}
