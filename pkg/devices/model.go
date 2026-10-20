package devices

import (
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// Device is one reading app on one piece of hardware (ADR 0011). It belongs
// to one user and owns the credential every one of its connections uses.
type Device struct {
	bun.BaseModel `bun:"table:devices,alias:d" json:"-"`

	ID         string `bun:"id,pk" json:"id"`
	UserID     int    `bun:"user_id,notnull" json:"user_id"`
	Name       string `bun:"name,notnull" json:"name"`
	DeviceType string `bun:"device_type,notnull" json:"device_type" tstype:"DeviceType"`
	// Credential is only sent to the Device's owner, through
	// DeviceResponse, so an admin listing can never act as another user.
	Credential     string `bun:"credential,notnull,unique" json:"-"`
	ScopeType      string `bun:"scope_type,notnull" json:"scope_type" tstype:"ScopeType"`
	ScopeLibraryID *int   `bun:"scope_library_id" json:"scope_library_id"`
	ScopeListID    *int   `bun:"scope_list_id" json:"scope_list_id"`
	// URLScopePending marks a Kobo migrated from an API key whose scope
	// still comes from the scope segment of its old sync URL. Its first
	// request sets the Device's scope and clears it, as do saving a scope
	// and regenerating the credential, which retires the old URL.
	URLScopePending bool `bun:"url_scope_pending,notnull" json:"url_scope_pending"`
	// DeliveryRulesMode says which Delivery Rules the Device follows: its
	// owner's default, its type's preset, or its own list (Service.Rules).
	DeliveryRulesMode string     `bun:"delivery_rules_mode,notnull" json:"delivery_rules_mode" tstype:"DeliveryRulesMode"`
	CreatedAt         time.Time  `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt         time.Time  `bun:"updated_at,notnull" json:"updated_at"`
	LastAccessedAt    *time.Time `bun:"last_accessed_at" json:"last_accessed_at"`

	Owner        *models.UserRef `bun:"rel:belongs-to,join:user_id=id" json:"-"`
	ScopeLibrary *models.Library `bun:"rel:belongs-to,join:scope_library_id=id" json:"-"`
	ScopeList    *models.List    `bun:"rel:belongs-to,join:scope_list_id=id" json:"-"`
}

// DeviceShortURL is a short-lived code that redirects to a Device's eReader
// browser URL, so it can be typed on an e-ink keyboard.
type DeviceShortURL struct {
	bun.BaseModel `bun:"table:device_short_urls,alias:dsu" json:"-"`

	ID        string    `bun:"id,pk" json:"id"`
	DeviceID  string    `bun:"device_id,notnull" json:"device_id"`
	ShortCode string    `bun:"short_code,notnull,unique" json:"short_code"`
	ExpiresAt time.Time `bun:"expires_at,notnull" json:"expires_at"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"created_at"`

	Device *Device `bun:"rel:belongs-to,join:device_id=id" json:"-"`
}

// DeviceDeliveryRule is one rule of a Device's own Delivery Rules. Only a
// Device whose DeliveryRulesMode is custom has rows.
type DeviceDeliveryRule struct {
	bun.BaseModel `bun:"table:device_delivery_rules,alias:ddr"`

	DeviceID          string `bun:"device_id,pk"`
	SourceFileType    string `bun:"source_file_type,pk"`
	DeliveredFileType string `bun:"delivered_file_type,notnull"`
	Position          int    `bun:"position,notnull"`
}

const (
	//tygo:emit export type DeviceType = typeof DeviceTypeKobo | typeof DeviceTypeKindle | typeof DeviceTypeOPDS | typeof DeviceTypeBrowser;
	DeviceTypeKobo    = "kobo"
	DeviceTypeKindle  = "kindle"
	DeviceTypeOPDS    = "opds"
	DeviceTypeBrowser = "browser"
)

const (
	//tygo:emit export type Connection = typeof ConnectionKoboSync | typeof ConnectionEReaderBrowser | typeof ConnectionOPDS;
	ConnectionKoboSync       = "kobo_sync"
	ConnectionEReaderBrowser = "ereader_browser"
	ConnectionOPDS           = "opds"
)

const (
	//tygo:emit export type DeliveryRulesMode = typeof RulesModeDefault | typeof RulesModePreset | typeof RulesModeCustom;
	// RulesModeDefault follows the owner's default Delivery Rules.
	RulesModeDefault = "default"
	// RulesModePreset follows the Device type's preset.
	RulesModePreset = "preset"
	// RulesModeCustom is the Device's own list, in device_delivery_rules.
	RulesModeCustom = "custom"
)

const (
	//tygo:emit export type ScopeType = typeof ScopeAll | typeof ScopeLibrary | typeof ScopeList;
	ScopeAll     = "all"
	ScopeLibrary = "library"
	ScopeList    = "list"
)
