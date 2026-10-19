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
	URLScopePending bool       `bun:"url_scope_pending,notnull" json:"url_scope_pending"`
	CreatedAt       time.Time  `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt       time.Time  `bun:"updated_at,notnull" json:"updated_at"`
	LastAccessedAt  *time.Time `bun:"last_accessed_at" json:"last_accessed_at"`

	Connections []*DeviceConnection `bun:"rel:has-many,join:id=device_id" json:"-"`
	Owner       *models.UserRef     `bun:"rel:belongs-to,join:user_id=id" json:"-"`
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

const (
	//tygo:emit export type DeviceType = typeof DeviceTypeKobo | typeof DeviceTypeKindle | typeof DeviceTypeBrowser;
	DeviceTypeKobo    = "kobo"
	DeviceTypeKindle  = "kindle"
	DeviceTypeBrowser = "browser"
)

const (
	//tygo:emit export type Connection = typeof ConnectionKoboSync | typeof ConnectionEReaderBrowser;
	ConnectionKoboSync       = "kobo_sync"
	ConnectionEReaderBrowser = "ereader_browser"
)

const (
	//tygo:emit export type ScopeType = typeof ScopeAll | typeof ScopeLibrary | typeof ScopeList;
	ScopeAll     = "all"
	ScopeLibrary = "library"
	ScopeList    = "list"
)
