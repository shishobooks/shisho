package server

import (
	"bufio"
	"context"
	"crypto/md5" //nolint:gosec // KOReader's protocol.
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/koreaderplugin"
	"github.com/shishobooks/shisho/pkg/logs"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
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

	db := testdb.New(t)
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

// A Device's first authenticated request, through any of its connections,
// tells its owner's open pages that it connected. No other user hears of it,
// an admin included.
func TestEventStream_DeviceConnectedReachesOnlyItsOwner(t *testing.T) {
	t.Parallel()

	md5Hex := func(s string) string {
		sum := md5.Sum([]byte(s)) //nolint:gosec // KOReader's protocol.
		return hex.EncodeToString(sum[:])
	}
	tests := []struct {
		name       string
		deviceType string
		request    func(t *testing.T, db *bun.DB, serverURL, username string, device *devices.Device) *http.Request
	}{
		{"eReader browser", devices.DeviceTypeBrowser, func(_ *testing.T, _ *bun.DB, serverURL, _ string, device *devices.Device) *http.Request {
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, serverURL+"/ereader/key/"+device.Credential+"/", nil)
			return req
		}},
		{"OPDS", devices.DeviceTypeOPDS, func(_ *testing.T, _ *bun.DB, serverURL, username string, device *devices.Device) *http.Request {
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, serverURL+"/opds/v1/catalog", nil)
			req.SetBasicAuth(username, device.Credential)
			return req
		}},
		{"KOReader progress sync", devices.DeviceTypeKOReader, func(_ *testing.T, _ *bun.DB, serverURL, username string, device *devices.Device) *http.Request {
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, serverURL+"/koreader/users/auth", nil)
			req.Header.Set("x-auth-user", username)
			req.Header.Set("x-auth-key", md5Hex(device.Credential))
			return req
		}},
		{"KOReader plugin pairing", devices.DeviceTypeKOReader, func(t *testing.T, db *bun.DB, serverURL, _ string, device *devices.Device) *http.Request {
			short, err := devices.NewService(db).GenerateShortURL(context.Background(), device.UserID, device.ID)
			require.NoError(t, err)
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, serverURL+"/koreader/plugin/pair", strings.NewReader(`{"code":"`+short.ShortCode+`"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req.Header.Set(koreaderplugin.HeaderPluginVersion, "0.7.0")
			req.Header.Set(koreaderplugin.HeaderPluginProtocol, strconv.Itoa(koreaderplugin.Protocol))
			return req
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			db := testdb.New(t)
			cfg := newPermissionTestConfig(t)
			broker := events.NewBroker()
			srv, err := New(cfg, db, worker.New(&config.Config{WorkerProcesses: 1}, db, nil, nil, nil, nil, nil, nil), nil, nil, broker, nil, nil, nil, logs.NewRingBuffer(100, broker))
			require.NoError(t, err)
			ts := httptest.NewServer(srv.Handler)
			t.Cleanup(ts.Close)
			t.Cleanup(broker.Close)

			owner := insertPermissionTestUser(ctx, t, db, "owner", models.RoleViewer, nil)
			admin := insertPermissionTestUser(ctx, t, db, "admin", models.RoleAdmin, nil)
			device, err := devices.NewService(db).Create(ctx, owner.ID, "Reader", tt.deviceType)
			require.NoError(t, err)
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
			ownerStream := subscribe(owner)
			adminStream := subscribe(admin)

			res, err := ts.Client().Do(tt.request(t, db, ts.URL, owner.Username, device))
			require.NoError(t, err)
			res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)

			got := readEventsUntil(t, ownerStream, events.EventTypeDeviceConnected)
			assert.JSONEq(t, `{"device_id":"`+device.ID+`"}`, got[len(got)-1].Data)

			// The marker reaches everyone, after the Device's event would
			// have, had it been sent to the admin.
			broker.Publish(events.NewJobEvent("job.created", 1, models.JobStatusPending, models.JobTypeScan, nil))
			for _, evt := range readEventsUntil(t, adminStream, "job.created") {
				assert.NotEqual(t, events.EventTypeDeviceConnected, evt.Type, "only the owner hears of it")
			}
		})
	}
}
