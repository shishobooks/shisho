package devices

import (
	"context"

	"github.com/shishobooks/shisho/pkg/events"
)

// Connections records each Device's requests and tells its owner, over the
// events broker, the first time the Device connects, so the Connect a device
// flow can switch to "Connected!". A Device connects for the first time on
// its first authenticated request through any connection, and again, for a
// KOReader Device, on the KOReader plugin's first connection.
type Connections struct {
	service *Service
	broker  *events.Broker
}

// NewConnections records requests with service and announces first
// connections on broker. A nil broker announces nothing.
func NewConnections(service *Service, broker *events.Broker) *Connections {
	return &Connections{service: service, broker: broker}
}

// RecordAccess records an authenticated request from device without making
// the request wait for the write.
func (c *Connections) RecordAccess(device *Device) {
	go func() {
		// A missed touch only leaves the last use a request behind, and the
		// write outlives the request, so it may find the server shutting down.
		_ = c.recordAccess(context.Background(), device)
	}()
}

func (c *Connections) recordAccess(ctx context.Context, device *Device) error {
	first, err := c.service.TouchLastAccessed(ctx, device)
	if err != nil {
		return err
	}
	if first {
		c.announce(device)
	}
	return nil
}

// RecordPluginConnection notes that device reached Shisho through the
// KOReader plugin, reporting pluginVersion.
func (c *Connections) RecordPluginConnection(ctx context.Context, device *Device, pluginVersion string) error {
	first, err := c.service.RecordPluginConnection(ctx, device, pluginVersion)
	if err != nil {
		return err
	}
	if first {
		c.announce(device)
	}
	return nil
}

func (c *Connections) announce(device *Device) {
	if c.broker != nil {
		c.broker.Publish(events.NewDeviceConnectedEvent(device.ID, device.UserID))
	}
}
