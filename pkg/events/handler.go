package events

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

const heartbeatInterval = 30 * time.Second

type handler struct {
	broker *Broker
}

func (h *handler) stream(c echo.Context) error {
	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.Writer.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	ch := h.broker.Subscribe()
	defer h.broker.Unsubscribe(ch)

	// Flush headers so client receives them immediately.
	flusher.Flush()

	ctx := c.Request().Context()
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeat.C:
			// SSE comment line keeps the connection alive through proxies.
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case evt, ok := <-ch:
			if !ok {
				return nil
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, evt.Data)
			flusher.Flush()
		}
	}
}
