package models

import (
	"time"

	"github.com/uptrace/bun"
)

// Plugin hook type constants.
const (
	//tygo:emit export type PluginHookType = typeof PluginHookInputConverter | typeof PluginHookFileParser | typeof PluginHookOutputGenerator | typeof PluginHookMetadataEnricher;
	PluginHookInputConverter   = "inputConverter"
	PluginHookFileParser       = "fileParser"
	PluginHookOutputGenerator  = "outputGenerator"
	PluginHookMetadataEnricher = "metadataEnricher"
)

type PluginRepository struct {
	bun.BaseModel `bun:"table:plugin_repositories,alias:pr" tstype:"-"`

	URL           string     `bun:",pk" json:"url"`
	Scope         string     `bun:",notnull,unique" json:"scope"`
	Name          *string    `json:"name"`
	IsOfficial    bool       `bun:",notnull" json:"is_official"`
	Enabled       bool       `bun:",notnull" json:"enabled"`
	LastFetchedAt *time.Time `json:"last_fetched_at"`
	FetchError    *string    `json:"fetch_error"`
}

type Plugin struct {
	bun.BaseModel `bun:"table:plugins,alias:p" tstype:"-"`

	Scope                  string     `bun:",pk" json:"scope"`
	ID                     string     `bun:",pk" json:"id"`
	Name                   string     `bun:",notnull" json:"name"`
	Version                string     `bun:",notnull" json:"version"`
	Description            *string    `json:"description"`
	Author                 *string    `json:"author"`
	Homepage               *string    `json:"homepage"`
	Enabled                bool       `bun:",notnull" json:"enabled"`
	InstalledAt            time.Time  `bun:",notnull" json:"installed_at"`
	UpdatedAt              *time.Time `json:"updated_at"`
	LoadError              *string    `json:"load_error"`
	UpdateAvailableVersion *string    `json:"update_available_version"`
}

type PluginConfig struct {
	bun.BaseModel `bun:"table:plugin_configs,alias:pc" tstype:"-"`

	Scope    string  `bun:",pk" json:"scope"`
	PluginID string  `bun:",pk" json:"plugin_id"`
	Key      string  `bun:",pk" json:"key"`
	Value    *string `json:"value"`
}

type PluginIdentifierType struct {
	bun.BaseModel `bun:"table:plugin_identifier_types,alias:pit" tstype:"-"`

	ID          string  `bun:",pk" json:"id"`
	Scope       string  `bun:",notnull" json:"scope"`
	PluginID    string  `bun:",notnull" json:"plugin_id"`
	Name        string  `bun:",notnull" json:"name"`
	URLTemplate *string `json:"url_template"`
	Pattern     *string `json:"pattern"`
}

type PluginOrder struct {
	bun.BaseModel `bun:"table:plugin_order,alias:po" tstype:"-"`

	HookType string `bun:",pk" json:"hook_type" tstype:"PluginHookType"`
	Scope    string `bun:",pk" json:"scope"`
	PluginID string `bun:",pk" json:"plugin_id"`
	Position int    `bun:",notnull" json:"position"`
}

type LibraryPluginCustomization struct {
	bun.BaseModel `bun:"table:library_plugin_customizations,alias:lpc" tstype:"-"`

	LibraryID int    `bun:",pk" json:"library_id"`
	HookType  string `bun:",pk" json:"hook_type" tstype:"PluginHookType"`
}

type LibraryPlugin struct {
	bun.BaseModel `bun:"table:library_plugins,alias:lp" tstype:"-"`

	LibraryID int    `bun:",pk" json:"library_id"`
	HookType  string `bun:",pk" json:"hook_type" tstype:"PluginHookType"`
	Scope     string `bun:",pk" json:"scope"`
	PluginID  string `bun:",pk" json:"plugin_id"`
	Enabled   bool   `bun:",notnull" json:"enabled"`
	Position  int    `bun:",notnull" json:"position"`
}

// PluginFieldSetting stores global field enable/disable settings for a plugin.
// Absence of a row means the field is enabled (default).
type PluginFieldSetting struct {
	bun.BaseModel `bun:"table:plugin_field_settings,alias:pfs" tstype:"-"`

	Scope    string `bun:",pk" json:"scope"`
	PluginID string `bun:",pk" json:"plugin_id"`
	Field    string `bun:",pk" json:"field"`
	Enabled  bool   `bun:",notnull" json:"enabled"`
}

// LibraryPluginFieldSetting stores per-library field overrides.
// Only rows with explicit overrides are stored; absence means use global default.
type LibraryPluginFieldSetting struct {
	bun.BaseModel `bun:"table:library_plugin_field_settings,alias:lpfs" tstype:"-"`

	LibraryID int    `bun:",pk" json:"library_id"`
	Scope     string `bun:",pk" json:"scope"`
	PluginID  string `bun:",pk" json:"plugin_id"`
	Field     string `bun:",pk" json:"field"`
	Enabled   bool   `bun:",notnull" json:"enabled"`
}
