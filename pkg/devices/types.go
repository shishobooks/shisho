package devices

import "github.com/shishobooks/shisho/pkg/models"

// CreateDevicePayload is the request body for POST /user/devices. A new
// Device gets every connection its type can use.
type CreateDevicePayload struct {
	Name       string `json:"name" mod:"trim" validate:"required,max=100"`
	DeviceType string `json:"device_type" validate:"required,oneof=kobo kindle browser" tstype:"DeviceType"`
}

// UpdateDevicePayload is the request body for PATCH /user/devices/:id. An
// omitted field keeps its saved value.
type UpdateDevicePayload struct {
	Name *string `json:"name,omitempty" mod:"trim" validate:"omitempty,min=1,max=100" tstype:"string"`
	// Scope replaces the Device's scope, and stops a migrated Kobo from
	// taking its scope from its old sync URL.
	Scope *DeviceScopePayload `json:"scope,omitempty" tstype:"DeviceScopePayload"`
}

// DeviceScopePayload sets a Device's scope. LibraryID is required for a
// library scope and ListID for a list scope.
type DeviceScopePayload struct {
	Type      string `json:"type" validate:"required,oneof=all library list" tstype:"ScopeType"`
	LibraryID *int   `json:"library_id,omitempty" tstype:"number"`
	ListID    *int   `json:"list_id,omitempty" tstype:"number"`
}

// DeviceResponse is a Device as its owner sees it, with its credential, the
// connections its type uses, and the name of its scope's library or list.
// Only routes under /user/devices, which return the caller's own Devices,
// send it.
type DeviceResponse struct {
	Device      `tstype:",extends"`
	Credential  string   `json:"credential"`
	Connections []string `json:"connections" tstype:"Connection[]"`
	// ScopeName is the name of the scope's library or list. It is omitted
	// for All Books, and for a scope whose library or list was deleted.
	ScopeName string `json:"scope_name,omitempty"`
}

// AdminDeviceResponse is a Device in the admin listing of every user's
// Devices. It never carries the credential, so an admin cannot act as the
// Device's owner.
type AdminDeviceResponse struct {
	Device      `tstype:",extends"`
	Connections []string       `json:"connections" tstype:"Connection[]"`
	Owner       models.UserRef `json:"owner" tstype:"UserRef"`
}
