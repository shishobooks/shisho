package devices

import "slices"

// typeConnections lists the connections each Device type uses, in the order
// setup instructions show them. A Device's type alone decides how it can
// connect: every connection uses the same credential and reaches the same
// scope, so turning one off would protect nothing.
var typeConnections = map[string][]string{
	DeviceTypeKobo:    {ConnectionKoboSync, ConnectionEReaderBrowser},
	DeviceTypeKindle:  {ConnectionEReaderBrowser},
	DeviceTypeBrowser: {ConnectionEReaderBrowser},
}

// HasConnection reports whether the Device's type connects through
// connection.
func (d *Device) HasConnection(connection string) bool {
	return slices.Contains(typeConnections[d.DeviceType], connection)
}

// ConnectionNames returns the connections the Device's type uses.
func (d *Device) ConnectionNames() []string {
	return slices.Clone(typeConnections[d.DeviceType])
}
