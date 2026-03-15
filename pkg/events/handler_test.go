package events

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSEHandler_StreamsEvents(t *testing.T) {
	t.Parallel()

	b := NewBroker()
	h := &handler{broker: b}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/events", nil)

	// Use a context we can cancel to simulate client disconnect
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Run handler in goroutine (it blocks until client disconnects)
	done := make(chan error, 1)
	go func() {
		done <- h.stream(c)
	}()

	// Give handler time to subscribe and start streaming
	time.Sleep(50 * time.Millisecond)

	// Publish an event
	b.Publish(Event{Type: "job.created", Data: `{"job_id":1}`})

	// Give time for event to be written
	time.Sleep(50 * time.Millisecond)

	// Cancel context to stop handler
	cancel()

	err := <-done
	require.NoError(t, err)

	// Check response headers
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "keep-alive", rec.Header().Get("Connection"))

	// Parse SSE output
	body := rec.Body.String()
	scanner := bufio.NewScanner(strings.NewReader(body))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Should contain the event
	assert.Contains(t, lines, "event: job.created")
	assert.Contains(t, lines, `data: {"job_id":1}`)
}
