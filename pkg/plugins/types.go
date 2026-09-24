package plugins

import (
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

// This file holds every request and response payload for the plugin HTTP API
// (ADR 0004: if it crosses the HTTP API, Go owns it and tygo generates it).
// The manifest-shaped types these payloads reference (Capabilities,
// ConfigSchema, AnnotatedPluginVersion, ...) live in manifest.go and
// repository.go, which are also tygo inputs.
//
// Wire-format note: several responses intentionally use camelCase keys
// (declaredFields, fieldSettings, imageUrl) — manifest and repository-index
// passthrough keeps its source format per the documented exemption in
// ADR 0004. Server-added fields on the same responses use snake_case
// (is_official, confidence_threshold).

// ---------------------------------------------------------------------------
// Responses
// ---------------------------------------------------------------------------

// PluginConfigResponse is the body of GET /plugins/installed/:scope/:id/config.
// Schema/declaredFields come from the parsed manifest (camelCase exemption);
// values holds the stored config with secrets masked.
type PluginConfigResponse struct {
	Schema              ConfigSchema           `json:"schema"`
	Values              map[string]interface{} `json:"values"`
	DeclaredFields      []string               `json:"declaredFields"`
	FieldSettings       map[string]bool        `json:"fieldSettings"`
	ConfidenceThreshold *float64               `json:"confidence_threshold"`
}

// SyncRepositoryResponse is the body of POST /plugins/repositories/:scope/sync.
// It embeds the repository by value (so tygo emits `extends PluginRepository`
// and the fields stay at the top level for backwards-compat with clients
// reading them) and adds an optional update_refresh_error populated when the
// post-sync update refresh failed.
type SyncRepositoryResponse struct {
	models.PluginRepository `tstype:",extends"`
	UpdateRefreshError      *string `json:"update_refresh_error,omitempty"`
}

// LibraryPluginOrderResponse is the body of
// GET /libraries/:id/plugins/order/:hookType.
type LibraryPluginOrderResponse struct {
	Customized bool                       `json:"customized"`
	Plugins    []LibraryPluginOrderPlugin `json:"plugins"`
}

// LibraryPluginOrderPlugin is one entry in LibraryPluginOrderResponse,
// resolved to the plugin's display name.
type LibraryPluginOrderPlugin struct {
	Scope string `json:"scope"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Mode  string `json:"mode" tstype:"PluginMode"`
}

// AvailablePluginResponse is the per-plugin shape returned by
// GET /plugins/available (as a bare array) and GET /plugins/available/:scope/:id.
// imageUrl is repository-index passthrough (camelCase exemption); is_official
// and compatible are server-added.
type AvailablePluginResponse struct {
	Scope       string                   `json:"scope"`
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Overview    string                   `json:"overview"`
	Description string                   `json:"description"`
	Homepage    string                   `json:"homepage"`
	ImageURL    string                   `json:"imageUrl"`
	IsOfficial  bool                     `json:"is_official"`
	Versions    []AnnotatedPluginVersion `json:"versions"`
	Compatible  bool                     `json:"compatible"`
}

// FieldSettingsResponse is the body of GET /plugins/installed/:scope/:id/fields.
type FieldSettingsResponse struct {
	Fields map[string]bool `json:"fields"`
}

// LibraryFieldSettingsResponse is the body of
// GET /libraries/:id/plugins/:scope/:pluginId/fields.
type LibraryFieldSettingsResponse struct {
	Fields     map[string]bool `json:"fields"`
	Customized bool            `json:"customized"`
}

// EnrichSearchResult wraps ParsedMetadata with server-added fields for the
// search HTTP response (sent to the frontend, not used by plugins).
type EnrichSearchResult struct {
	mediafile.ParsedMetadata `tstype:",extends"`
	PluginScope              string   `json:"plugin_scope"`
	PluginID                 string   `json:"plugin_id"`
	DisabledFields           []string `json:"disabled_fields,omitempty"`
}

// PluginSearchError reports a plugin whose search() hook failed so the
// frontend can surface it to the user instead of silently dropping it.
type PluginSearchError struct {
	PluginScope string `json:"plugin_scope"`
	PluginID    string `json:"plugin_id"`
	PluginName  string `json:"plugin_name"`
	Message     string `json:"message"`
}

// PluginSearchSkipped reports an enricher that was skipped because it does
// not declare support for the target file type. The frontend uses this to
// distinguish "no plugins handle this file type" from "plugins ran and
// returned nothing".
type PluginSearchSkipped struct {
	PluginScope string `json:"plugin_scope"`
	PluginID    string `json:"plugin_id"`
	PluginName  string `json:"plugin_name"`
}

// PluginSearchResponse is the HTTP response body for POST /plugins/search.
// TotalPlugins is the number of candidate enricher runtimes considered for
// this search (after library + mode filtering but before file-type skipping),
// which lets the frontend distinguish "every enricher was skipped" from
// "some enrichers ran and returned nothing".
type PluginSearchResponse struct {
	Results        []EnrichSearchResult  `json:"results"`
	Errors         []PluginSearchError   `json:"errors,omitempty"`
	SkippedPlugins []PluginSearchSkipped `json:"skipped_plugins,omitempty"`
	TotalPlugins   int                   `json:"total_plugins"`
}

// ---------------------------------------------------------------------------
// Request payloads
//
// omitempty on these tags exists so tygo marks optional fields with `?` in
// the generated TypeScript; payloads are only ever unmarshaled by the
// server, so omitempty has no wire-format effect in production.
// ---------------------------------------------------------------------------

// InstallPluginPayload is the body of POST /plugins/installed. Either both
// download_url and sha256 are provided, or neither (install from repository).
type InstallPluginPayload struct {
	Scope       string `json:"scope" validate:"required"`
	ID          string `json:"id" validate:"required"`
	Name        string `json:"name,omitempty"`
	Version     string `json:"version,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

// UpdatePluginPayload is the body of PATCH /plugins/installed/:scope/:id.
type UpdatePluginPayload struct {
	Enabled                  *bool             `json:"enabled"`
	AutoUpdate               *bool             `json:"auto_update"`
	Config                   map[string]string `json:"config,omitempty"`
	ConfidenceThreshold      *float64          `json:"confidence_threshold"`
	ClearConfidenceThreshold *bool             `json:"clear_confidence_threshold"`
}

// PluginSearchPayload is the body of POST /plugins/search.
type PluginSearchPayload struct {
	Query       string                       `json:"query" validate:"required"`
	BookID      int                          `json:"book_id" validate:"required"`
	FileID      *int                         `json:"file_id"`
	Author      string                       `json:"author,omitempty"`
	Identifiers []mediafile.ParsedIdentifier `json:"identifiers,omitempty" tstype:"ParsedIdentifier[]"`
}

// SourceIntent is Identify's per-field attribution intent (ADR 0006). The
// browser holds the Plugin Proposal, so it reports whether the final value
// equals the proposal; the server decides no-op against stored state and maps
// the intent to a canonical DataSource. Intent values never reach a source
// column verbatim. SourceIntents is the payload map, emitted here because a
// tstype tag cannot contain the comma in Record<K, V>.
const (
	//tygo:emit export type SourceIntent = typeof SourceIntentPlugin | typeof SourceIntentUser;
	//tygo:emit export type SourceIntents = Record<string, SourceIntent>;
	SourceIntentPlugin = "plugin"
	SourceIntentUser   = "user"
)

// SourcesKeyFileName is the PluginApplyPayload.Sources key for the file Name,
// which travels as the top-level file_name rather than inside Fields.
const SourcesKeyFileName = "file_name"

// PluginApplyPayload is the body of POST /plugins/apply. Fields carries the
// user-selected metadata as an untyped map (converted server-side by
// convertFieldsToMetadata / extractSeriesEntries).
type PluginApplyPayload struct {
	BookID int            `json:"book_id" validate:"required"`
	FileID *int           `json:"file_id"`
	Fields map[string]any `json:"fields" validate:"required"`
	// FileName is an optional override for file.Name. Omitted leaves the name
	// untouched; an explicitly selected blank string clears it.
	FileName *string `json:"file_name"`
	// Sources is the per-field SourceIntent, keyed by the same keys as Fields
	// plus SourcesKeyFileName. A selected field with no entry is treated as
	// "user". Identifier objects inside Fields["identifiers"] additionally
	// carry an optional per-entry "source" intent with the same values.
	Sources     map[string]string `json:"sources,omitempty" validate:"omitempty,dive,oneof=plugin user" tstype:"SourceIntents"`
	PluginScope string            `json:"plugin_scope" validate:"required"`
	PluginID    string            `json:"plugin_id" validate:"required"`
}

// AddRepositoryPayload is the body of POST /plugins/repositories.
type AddRepositoryPayload struct {
	URL   string `json:"url" validate:"required,url"`
	Scope string `json:"scope" validate:"required"`
}

// OrderEntry is one entry in SetOrderPayload. An empty mode defaults to
// "enabled" server-side.
type OrderEntry struct {
	Scope string `json:"scope" validate:"required"`
	ID    string `json:"id" validate:"required"`
	Mode  string `json:"mode" tstype:"PluginMode"`
}

// SetOrderPayload is the body of PUT /plugins/order/:hookType.
type SetOrderPayload struct {
	Order []OrderEntry `json:"order" validate:"required"`
}

// LibraryOrderEntry is one entry in SetLibraryOrderPayload. An empty mode
// defaults to "enabled" server-side.
type LibraryOrderEntry struct {
	Scope string `json:"scope" validate:"required"`
	ID    string `json:"id" validate:"required"`
	Mode  string `json:"mode" tstype:"PluginMode"`
}

// SetLibraryOrderPayload is the body of PUT /libraries/:id/plugins/order/:hookType.
type SetLibraryOrderPayload struct {
	Plugins []LibraryOrderEntry `json:"plugins" validate:"required"`
}

// SetFieldSettingsPayload is the body of PUT /plugins/installed/:scope/:id/fields
// and PUT /libraries/:id/plugins/:scope/:pluginId/fields.
type SetFieldSettingsPayload struct {
	Fields map[string]bool `json:"fields" validate:"required"`
}

// ---------------------------------------------------------------------------
// Apply-path internals
//
// These are not bound from JSON directly — they are parsed out of
// PluginApplyPayload.Fields' untyped map — so their generated TypeScript
// mirrors are a tygo side effect the frontend never imports.
// ---------------------------------------------------------------------------

// SeriesEntry represents a single series association in the multi-series
// apply payload. Used by the identify form which supports multiple series
// per book, unlike the plugin SDK which models a single series.
type SeriesEntry struct {
	Name             string
	Number           *float64
	NumberEnd        *float64
	SeriesNumberUnit *string
}

// ApplyOverrides carries apply-path-only signals that don't belong on
// mediafile.ParsedMetadata (which is part of the public plugin SDK
// contract). These come exclusively from the identify apply payload —
// plugins do not model them.
type ApplyOverrides struct {
	// SelectedFields records valid fields explicitly included in the Identify
	// apply payload. It preserves the distinction between omitted zero values
	// (leave untouched) and selected zero values (clear).
	SelectedFields map[string]bool
	// FileName is the normalized value to write to file.Name. Nil = no change.
	// A pointer to an empty string explicitly clears the name.
	FileName *string
	// Intents is the validated per-field SourceIntent from the apply payload,
	// keyed like PluginApplyPayload.Sources. persistMetadata maps it to a
	// canonical source only for fields whose value actually changed.
	Intents map[string]string
	// IdentifierIntents is the validated per-entry SourceIntent from the
	// identifier objects in Fields["identifiers"], keyed by trimmed type.
	// Types are unique within a collection (validateIdentifierTypes).
	IdentifierIntents map[string]string
	// SeriesEntries, when non-nil, replaces the book's series associations
	// with the provided list. An empty slice clears all series. Nil means
	// "don't touch series" (the identify form's series checkbox was off).
	SeriesEntries *[]SeriesEntry
}

// PluginApplyResponse is the reloaded book after POST /plugins/apply, plus
// any warnings for selected fields that were accepted but not applied (for
// example a proposed cover that could not be downloaded or decoded). The
// apply itself succeeded, so the status is 200 and the warnings are the only
// signal the client has to surface.
type PluginApplyResponse struct {
	models.Book `tstype:",extends"`
	Warnings    []string `json:"warnings"`
}
