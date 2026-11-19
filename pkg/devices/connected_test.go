package devices

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func receiveEvent(t *testing.T, ch <-chan events.Event) events.Event {
	t.Helper()
	select {
	case evt := <-ch:
		return evt
	case <-time.After(5 * time.Second):
		t.Fatal("no event arrived")
		return events.Event{}
	}
}

// A Device's first authenticated request tells its owner it connected.
// Later requests don't, and neither does a request from another Device of
// the same owner that has connected before.
func TestConnections_AnnounceTheFirstRequestOnly(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	ctx := context.Background()
	device, err := service.Create(ctx, 1, "Kindle", DeviceTypeKindle)
	require.NoError(t, err)
	broker := events.NewBroker()
	ch := broker.Subscribe()
	defer broker.Unsubscribe(ch)
	connections := NewConnections(service, broker)

	require.NoError(t, connections.recordAccess(ctx, device))
	evt := receiveEvent(t, ch)
	assert.Equal(t, events.EventTypeDeviceConnected, evt.Type)
	assert.Equal(t, 1, evt.UserID)
	assert.JSONEq(t, `{"device_id":"`+device.ID+`"}`, evt.Data)

	// A later request loads the Device with its last use set.
	device, err = service.Retrieve(ctx, 1, device.ID)
	require.NoError(t, err)
	require.NoError(t, connections.recordAccess(ctx, device))
	assert.Empty(t, ch, "a later request announces nothing")
}

// The KOReader plugin's first connection is announced even when the Device
// reached Shisho another way first, such as over OPDS.
func TestConnections_AnnounceThePluginsFirstConnection(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	ctx := context.Background()
	device, err := service.Create(ctx, 1, "KOReader", DeviceTypeKOReader)
	require.NoError(t, err)
	_, err = service.TouchLastAccessed(ctx, device)
	require.NoError(t, err)
	broker := events.NewBroker()
	ch := broker.Subscribe()
	defer broker.Unsubscribe(ch)
	connections := NewConnections(service, broker)

	require.NoError(t, connections.RecordPluginConnection(ctx, device, "1.0.0"))
	evt := receiveEvent(t, ch)
	assert.Equal(t, events.EventTypeDeviceConnected, evt.Type)
	assert.Equal(t, 1, evt.UserID)

	require.NoError(t, connections.RecordPluginConnection(ctx, device, "1.0.0"))
	assert.Empty(t, ch, "a later connection announces nothing")
}

// Every way a Device signs in records the request, so its first one is
// announced whichever connection it uses.
func TestMiddleware_AnnouncesTheFirstRequest(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	device, err := service.Create(context.Background(), 1, "Kobo", DeviceTypeKobo)
	require.NoError(t, err)
	broker := events.NewBroker()
	ch := broker.Subscribe()
	defer broker.Unsubscribe(ch)

	_, err = runDeviceAuthWith(t, NewMiddleware(service, broker), ConnectionKoboSync, device.Credential)
	require.NoError(t, err)

	evt := receiveEvent(t, ch)
	assert.Equal(t, events.EventTypeDeviceConnected, evt.Type)
}
