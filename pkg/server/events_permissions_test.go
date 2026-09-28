package server

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/logs"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sseEvent struct {
	Type string
	Data string
}

// readEventsUntil reads SSE events from r until one of type stopType arrives,
// returning every event read, including the stop event.
func readEventsUntil(t *testing.T, r *bufio.Reader, stopType string) []sseEvent {
	t.Helper()
	var got []sseEvent
	var cur sseEvent
	for {
		line, err := r.ReadString('\n')
		if err == io.EOF {
			t.Fatalf("stream ended before %s arrived; saw %v", stopType, got)
		}
		require.NoError(t, err)
		line = strings.TrimSuffix(line, "\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			cur.Type = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.Data = strings.TrimPrefix(line, "data: ")
		case line == "" && cur.Type != "":
			got = append(got, cur)
			if cur.Type == stopType {
				return got
			}
			cur = sseEvent{}
		}
	}
}

// TestEventStream_LogEntriesRequireConfigRead covers issue #528: log.entry
// events carry server log lines, so the shared stream only delivers them to
// users who can read the logs page (Config Read). Job events stay broadcast.
func TestEventStream_LogEntriesRequireConfigRead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	db := newPermissionTestDB(t)
	cfg := newPermissionTestConfig(t)
	broker := events.NewBroker()
	logBuffer := logs.NewRingBuffer(100, broker)
	srv, err := New(cfg, db, worker.New(&config.Config{WorkerProcesses: 1}, db, nil, nil, nil, nil, nil, nil), nil, nil, broker, nil, nil, nil, logBuffer)
	require.NoError(t, err)
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)
	t.Cleanup(broker.Close)

	admin := insertPermissionTestUser(ctx, t, db, "admin", models.RoleAdmin, nil)
	editor := insertPermissionTestUser(ctx, t, db, "editor", models.RoleEditor, nil)
	viewer := insertPermissionTestUser(ctx, t, db, "viewer", models.RoleViewer, nil)
	authSvc := auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration())

	streamCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	subscribe := func(user *models.User) *bufio.Reader {
		t.Helper()
		token, err := authSvc.GenerateToken(user)
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, ts.URL+"/api/events", nil)
		require.NoError(t, err)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		res, err := ts.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { res.Body.Close() })
		require.Equal(t, http.StatusOK, res.StatusCode)
		return bufio.NewReader(res.Body)
	}

	// Headers flush only after the handler subscribes, so every stream is
	// registered with the broker once subscribe returns.
	adminStream := subscribe(admin)
	editorStream := subscribe(editor)
	viewerStream := subscribe(viewer)

	_, err = logBuffer.Write([]byte(`{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"secret server log line"}` + "\n"))
	require.NoError(t, err)
	broker.Publish(events.NewJobEvent("job.created", 7, models.JobStatusPending, models.JobTypeScan, nil))

	for name, stream := range map[string]*bufio.Reader{"editor": editorStream, "viewer": viewerStream} {
		got := readEventsUntil(t, stream, "job.created")
		for _, evt := range got {
			assert.NotEqual(t, events.EventTypeLogEntry, evt.Type, "%s must not receive log lines", name)
			assert.NotContains(t, evt.Data, "secret server log line", "%s must not receive log text", name)
		}
	}

	got := readEventsUntil(t, adminStream, "job.created")
	require.Len(t, got, 2, "admin should receive the log line and the job event")
	assert.Equal(t, events.EventTypeLogEntry, got[0].Type)
	assert.Contains(t, got[0].Data, "secret server log line")
	assert.Equal(t, "job.created", got[1].Type)
}
