package models

// DeviceConnected is the payload of a device.connected event: one of the
// receiving user's Devices reached Shisho for the first time.
type DeviceConnected struct {
	DeviceID string `json:"device_id"`
}
