package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroker_SubscribeAndPublish(t *testing.T) {
	t.Parallel()

	b := NewBroker()
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	evt := Event{Type: "job.status_changed", Data: `{"job_id":1,"status":"in_progress"}`}
	b.Publish(evt)

	select {
	case received := <-ch:
		assert.Equal(t, evt.Type, received.Type)
		assert.Equal(t, evt.Data, received.Data)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBroker_MultipleSubscribers(t *testing.T) {
	t.Parallel()

	b := NewBroker()
	ch1 := b.Subscribe()
	ch2 := b.Subscribe()
	defer b.Unsubscribe(ch1)
	defer b.Unsubscribe(ch2)

	evt := Event{Type: "job.created", Data: `{"job_id":2}`}
	b.Publish(evt)

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case received := <-ch:
			assert.Equal(t, evt.Type, received.Type)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event")
		}
	}
}

func TestBroker_Unsubscribe(t *testing.T) {
	t.Parallel()

	b := NewBroker()
	ch := b.Subscribe()
	b.Unsubscribe(ch)

	// Publishing after unsubscribe should not block
	evt := Event{Type: "job.created", Data: `{}`}
	b.Publish(evt)

	// Channel should be closed
	_, ok := <-ch
	assert.False(t, ok)
}

func TestBroker_SlowSubscriberDropsEvents(t *testing.T) {
	t.Parallel()

	b := NewBroker()
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	// Fill the channel buffer and then some — should not block
	for i := 0; i < 200; i++ {
		b.Publish(Event{Type: "job.created", Data: `{}`})
	}

	// Drain what we can — we just care that Publish didn't block
	drained := 0
	for {
		select {
		case <-ch:
			drained++
		default:
			goto done
		}
	}
done:
	// Should have received up to buffer size (64), not all 200
	require.LessOrEqual(t, drained, 64)
}
