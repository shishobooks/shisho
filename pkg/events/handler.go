package events

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
)

const heartbeatInterval = 30 * time.Second

type handler struct {
	broker *Broker
}

// eventFilterFor returns which events a connected user may receive. Log
// lines follow the permission of GET /api/logs (Config Read). Every other
// event, including job events, goes to every authenticated user.
func eventFilterFor(user *models.User) EventFilter {
	canReadLogs := user != nil && user.HasPermission(models.ResourceConfig, models.OperationRead)
	return func(evt Event) bool {
		return evt.Type != EventTypeLogEntry || canReadLogs
	}
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

	// Authenticate stores the user with its role and permissions loaded. The
	// filter is fixed for the life of the connection, so a permission change
	// applies when the client reconnects.
	user, _ := c.Get("user").(*models.User)
	ch := h.broker.SubscribeFiltered(eventFilterFor(user))
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
		case <-h.broker.Done():
			// Server is shutting down; exit the stream so srv.Shutdown can
			// return without waiting the full timeout for this handler.
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
