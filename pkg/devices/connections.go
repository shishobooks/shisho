package devices

import (
	"slices"
	"time"

	"github.com/uptrace/bun"
)

// DeviceConnection records that a Device may connect one way.
type DeviceConnection struct {
	bun.BaseModel `bun:"table:device_connections,alias:dc"`

	DeviceID   string    `bun:"device_id,pk"`
	Connection string    `bun:"connection,pk"`
	CreatedAt  time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp"`
}

// typeConnections lists the connections each Device type can use, in the
// order setup instructions show them.
var typeConnections = map[string][]string{
	DeviceTypeKobo:    {ConnectionKoboSync, ConnectionEReaderBrowser},
	DeviceTypeKindle:  {ConnectionEReaderBrowser},
	DeviceTypeBrowser: {ConnectionEReaderBrowser},
}

// TypeAllowsConnection reports whether deviceType can use connection.
func TypeAllowsConnection(deviceType, connection string) bool {
	return slices.Contains(typeConnections[deviceType], connection)
}

// HasConnection reports whether the Device may connect through connection.
func (d *Device) HasConnection(connection string) bool {
	for _, c := range d.Connections {
		if c.Connection == connection {
			return true
		}
	}
	return false
}

// ConnectionNames returns the Device's connections in its type's order.
func (d *Device) ConnectionNames() []string {
	names := []string{}
	for _, connection := range typeConnections[d.DeviceType] {
		if d.HasConnection(connection) {
			names = append(names, connection)
		}
	}
	return names
}
