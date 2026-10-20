package devices

import (
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/models"
)

// CreateDevicePayload is the request body for POST /user/devices. A new
// Device gets every connection its type can use.
type CreateDevicePayload struct {
	Name       string `json:"name" mod:"trim" validate:"required,max=100"`
	DeviceType string `json:"device_type" validate:"required,oneof=kobo kindle opds browser" tstype:"DeviceType"`
}

// UpdateDevicePayload is the request body for PATCH /user/devices/:id. An
// omitted field keeps its saved value.
type UpdateDevicePayload struct {
	Name *string `json:"name,omitempty" mod:"trim" validate:"omitempty,min=1,max=100" tstype:"string"`
	// DeviceType changes the Device's type, which puts it back on the rules
	// the new type starts with.
	DeviceType *string `json:"device_type,omitempty" validate:"omitempty,oneof=kobo kindle opds browser" tstype:"DeviceType"`
	// DeliveryRules sets which Delivery Rules the Device follows. It applies
	// after DeviceType.
	DeliveryRules *DeviceDeliveryRulesPayload `json:"delivery_rules,omitempty" tstype:"DeviceDeliveryRulesPayload"`
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

// DeviceDeliveryRulesPayload makes a Device follow its owner's default,
// follow its type's preset, or use Rules as its own list. Rules is read only
// for a custom list, which can't be empty or deliver a File Type the
// Device's type can't receive.
type DeviceDeliveryRulesPayload struct {
	Mode  string          `json:"mode" validate:"required" tstype:"DeliveryRulesMode"`
	Rules []delivery.Rule `json:"rules,omitempty" validate:"dive" mod:"dive" tstype:"Rule[]"`
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
	// DeliveryRules are the rules the Device follows now, whichever way it
	// follows them. A custom list starts as a copy of them.
	DeliveryRules []delivery.Rule `json:"delivery_rules" tstype:"Rule[]"`
	// DeliveryRuleOptions lists every source File Type a custom rule can
	// name, with only the File Types the Device's type can receive.
	DeliveryRuleOptions []delivery.RuleOption `json:"delivery_rule_options" tstype:"RuleOption[]"`
	// HasPreset is true when the Device's type has a preset to follow.
	HasPreset bool `json:"has_preset"`
}

// AdminDeviceResponse is a Device in the admin listing of every user's
// Devices. It never carries the credential, so an admin cannot act as the
// Device's owner.
type AdminDeviceResponse struct {
	Device      `tstype:",extends"`
	Connections []string       `json:"connections" tstype:"Connection[]"`
	Owner       models.UserRef `json:"owner" tstype:"UserRef"`
}
