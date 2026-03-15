package events

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

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
	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-ch:
			if !ok {
				return nil
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, evt.Data)
			flusher.Flush()
		}
	}
}
