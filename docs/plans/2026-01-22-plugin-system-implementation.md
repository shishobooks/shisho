# Plugin System Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the plugin system described in `docs/plans/2026-01-22-plugin-system-design.md`, enabling third-party JavaScript plugins to extend Shisho with new file formats, converters, enrichers, and output generators.

**Architecture:** A plugin manager (`pkg/plugins/`) orchestrates goja JavaScript runtimes. Plugins are loaded from disk (`/config/plugins/installed/{scope}/{id}/`), configured in the database, and integrated into the scan pipeline and download system. A repository system backed by GitHub enables discovery and installation.

**Tech Stack:** Go (goja JS interpreter, Echo, Bun ORM, SQLite), React 19 (Tanstack Query, TailwindCSS), esbuild (plugin template builds)

---

## Phase 1: Core Infrastructure

### Task 1: Database Migration — Plugin Tables

**Files:**
- Create: `pkg/migrations/20260123000000_create_plugin_tables.go`

**Step 1: Write the migration**

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			CREATE TABLE plugin_repositories (
				url TEXT PRIMARY KEY,
				scope TEXT NOT NULL UNIQUE,
				name TEXT,
				is_official BOOLEAN NOT NULL DEFAULT false,
				enabled BOOLEAN NOT NULL DEFAULT true,
				last_fetched_at TIMESTAMP,
				fetch_error TEXT
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`
			CREATE TABLE plugins (
				scope TEXT NOT NULL,
				id TEXT NOT NULL,
				name TEXT NOT NULL,
				version TEXT NOT NULL,
				description TEXT,
				author TEXT,
				homepage TEXT,
				enabled BOOLEAN NOT NULL DEFAULT true,
				installed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP,
				load_error TEXT,
				update_available_version TEXT,
				PRIMARY KEY (scope, id)
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`
			CREATE TABLE plugin_configs (
				scope TEXT NOT NULL,
				plugin_id TEXT NOT NULL,
				key TEXT NOT NULL,
				value TEXT,
				PRIMARY KEY (scope, plugin_id, key),
				FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`
			CREATE TABLE plugin_identifier_types (
				id TEXT PRIMARY KEY,
				scope TEXT NOT NULL,
				plugin_id TEXT NOT NULL,
				name TEXT NOT NULL,
				url_template TEXT,
				pattern TEXT,
				FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for listing identifier types by plugin
		_, err = db.Exec(`CREATE INDEX ix_plugin_identifier_types_plugin ON plugin_identifier_types(scope, plugin_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`
			CREATE TABLE plugin_order (
				hook_type TEXT NOT NULL,
				scope TEXT NOT NULL,
				plugin_id TEXT NOT NULL,
				position INTEGER NOT NULL,
				PRIMARY KEY (hook_type, scope, plugin_id),
				FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for listing plugins in order by hook type
		_, err = db.Exec(`CREATE INDEX ix_plugin_order_hook_position ON plugin_order(hook_type, position)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Seed official repository
		_, err = db.Exec(`
			INSERT INTO plugin_repositories (url, scope, name, is_official, enabled)
			VALUES ('https://raw.githubusercontent.com/shishobooks/plugins/master/repository.json', 'shisho', 'Official Shisho Plugins', true, true)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP INDEX IF EXISTS ix_plugin_order_hook_position`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP INDEX IF EXISTS ix_plugin_identifier_types_plugin`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS plugin_order`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS plugin_identifier_types`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS plugin_configs`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS plugins`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS plugin_repositories`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

**Step 2: Run migration and verify**

Run: `make db:migrate`
Expected: Migration runs successfully, tables created.

Run: `make db:rollback && make db:migrate`
Expected: Rollback drops tables, re-migrate creates them again. Proves idempotency.

**Step 3: Commit**

```bash
git add pkg/migrations/20260123000000_create_plugin_tables.go
git commit -m "[Plugins] Add database migration for plugin system tables"
```

---

### Task 2: Plugin Models

**Files:**
- Create: `pkg/models/plugin.go`

**Step 1: Write the models**

These are Bun ORM models matching the migration schema. They'll be used by the plugin service layer.

```go
package models

import "time"

// PluginRepository represents a configured plugin repository.
type PluginRepository struct {
	BaseModel     `bun:"table:plugin_repositories" tstype:"-"`
	URL           string     `bun:",pk" json:"url"`
	Scope         string     `bun:",notnull,unique" json:"scope"`
	Name          *string    `json:"name"`
	IsOfficial    bool       `bun:",notnull" json:"is_official"`
	Enabled       bool       `bun:",notnull" json:"enabled"`
	LastFetchedAt *time.Time `json:"last_fetched_at"`
	FetchError    *string    `json:"fetch_error"`
}

// Plugin represents an installed plugin.
type Plugin struct {
	BaseModel               `bun:"table:plugins" tstype:"-"`
	Scope                   string     `bun:",pk" json:"scope"`
	ID                      string     `bun:",pk" json:"id"`
	Name                    string     `bun:",notnull" json:"name"`
	Version                 string     `bun:",notnull" json:"version"`
	Description             *string    `json:"description"`
	Author                  *string    `json:"author"`
	Homepage                *string    `json:"homepage"`
	Enabled                 bool       `bun:",notnull" json:"enabled"`
	InstalledAt             time.Time  `bun:",notnull" json:"installed_at"`
	UpdatedAt               *time.Time `json:"updated_at"`
	LoadError               *string    `json:"load_error"`
	UpdateAvailableVersion  *string    `json:"update_available_version"`
}

// PluginConfig represents a single configuration value for a plugin.
type PluginConfig struct {
	BaseModel `bun:"table:plugin_configs" tstype:"-"`
	Scope     string  `bun:",pk" json:"scope"`
	PluginID  string  `bun:",pk" json:"plugin_id"`
	Key       string  `bun:",pk" json:"key"`
	Value     *string `json:"value"`
}

// PluginIdentifierType represents a plugin-registered identifier type.
type PluginIdentifierType struct {
	BaseModel   `bun:"table:plugin_identifier_types" tstype:"-"`
	ID          string  `bun:",pk" json:"id"`
	Scope       string  `bun:",notnull" json:"scope"`
	PluginID    string  `bun:",notnull" json:"plugin_id"`
	Name        string  `bun:",notnull" json:"name"`
	URLTemplate *string `json:"url_template"`
	Pattern     *string `json:"pattern"`
}

// PluginOrder represents the processing order for a plugin in a specific hook type.
type PluginOrder struct {
	BaseModel `bun:"table:plugin_order" tstype:"-"`
	HookType  string `bun:",pk" json:"hook_type"`
	Scope     string `bun:",pk" json:"scope"`
	PluginID  string `bun:",pk" json:"plugin_id"`
	Position  int    `bun:",notnull" json:"position"`
}

// Hook type constants for plugin_order table.
const (
	PluginHookInputConverter   = "inputConverter"
	PluginHookFileParser       = "fileParser"
	PluginHookOutputGenerator  = "outputGenerator"
	PluginHookMetadataEnricher = "metadataEnricher"
)
```

**Step 2: Run tygo to generate TypeScript types**

Run: `make tygo`
Expected: TypeScript types generated in `app/types/generated/` (may say "Nothing to be done" if already up-to-date — this is normal).

**Step 3: Commit**

```bash
git add pkg/models/plugin.go
git commit -m "[Plugins] Add Bun ORM models for plugin tables"
```

---

### Task 3: Plugin Manifest Parsing

**Files:**
- Create: `pkg/plugins/manifest.go`
- Create: `pkg/plugins/manifest_test.go`

**Step 1: Write test for manifest parsing**

```go
package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseManifest_Valid(t *testing.T) {
	data := []byte(`{
		"manifestVersion": 1,
		"id": "test-plugin",
		"name": "Test Plugin",
		"version": "1.0.0",
		"description": "A test plugin",
		"author": "Test Author",
		"capabilities": {
			"metadataEnricher": {
				"description": "Enriches metadata",
				"fileTypes": ["epub", "cbz"]
			},
			"httpAccess": {
				"description": "Makes HTTP requests",
				"domains": ["example.com", "api.example.com"]
			}
		},
		"configSchema": {
			"apiKey": {
				"type": "string",
				"label": "API Key",
				"required": true,
				"secret": true
			}
		}
	}`)

	m, err := ParseManifest(data)
	require.NoError(t, err)
	assert.Equal(t, 1, m.ManifestVersion)
	assert.Equal(t, "test-plugin", m.ID)
	assert.Equal(t, "Test Plugin", m.Name)
	assert.Equal(t, "1.0.0", m.Version)
	assert.NotNil(t, m.Capabilities.MetadataEnricher)
	assert.Equal(t, []string{"epub", "cbz"}, m.Capabilities.MetadataEnricher.FileTypes)
	assert.NotNil(t, m.Capabilities.HTTPAccess)
	assert.Equal(t, []string{"example.com", "api.example.com"}, m.Capabilities.HTTPAccess.Domains)
	assert.True(t, m.ConfigSchema["apiKey"].Required)
	assert.True(t, m.ConfigSchema["apiKey"].Secret)
}

func TestParseManifest_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name string
		data string
		err  string
	}{
		{"missing id", `{"manifestVersion":1,"name":"X","version":"1.0.0"}`, "id is required"},
		{"missing name", `{"manifestVersion":1,"id":"x","version":"1.0.0"}`, "name is required"},
		{"missing version", `{"manifestVersion":1,"id":"x","name":"X"}`, "version is required"},
		{"missing manifest version", `{"id":"x","name":"X","version":"1.0.0"}`, "manifestVersion is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseManifest([]byte(tt.data))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.err)
		})
	}
}

func TestParseManifest_UnsupportedManifestVersion(t *testing.T) {
	data := []byte(`{"manifestVersion":99,"id":"x","name":"X","version":"1.0.0"}`)
	_, err := ParseManifest(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported manifest version")
}

func TestParseManifest_InputConverter(t *testing.T) {
	data := []byte(`{
		"manifestVersion": 1,
		"id": "pdf-converter",
		"name": "PDF Converter",
		"version": "1.0.0",
		"capabilities": {
			"inputConverter": {
				"description": "Converts PDF to EPUB",
				"sourceTypes": ["pdf"],
				"mimeTypes": ["application/pdf"],
				"targetType": "epub"
			}
		}
	}`)

	m, err := ParseManifest(data)
	require.NoError(t, err)
	assert.Equal(t, []string{"pdf"}, m.Capabilities.InputConverter.SourceTypes)
	assert.Equal(t, "epub", m.Capabilities.InputConverter.TargetType)
	assert.Equal(t, []string{"application/pdf"}, m.Capabilities.InputConverter.MIMETypes)
}

func TestParseManifest_FileParser(t *testing.T) {
	data := []byte(`{
		"manifestVersion": 1,
		"id": "pdf-parser",
		"name": "PDF Parser",
		"version": "1.0.0",
		"capabilities": {
			"fileParser": {
				"description": "Parses PDF files",
				"types": ["pdf", "djvu"],
				"mimeTypes": ["application/pdf", "image/vnd.djvu"]
			}
		}
	}`)

	m, err := ParseManifest(data)
	require.NoError(t, err)
	assert.Equal(t, []string{"pdf", "djvu"}, m.Capabilities.FileParser.Types)
}

func TestParseManifest_IdentifierTypes(t *testing.T) {
	data := []byte(`{
		"manifestVersion": 1,
		"id": "goodreads",
		"name": "Goodreads",
		"version": "1.0.0",
		"capabilities": {
			"identifierTypes": [
				{
					"id": "goodreads",
					"name": "Goodreads ID",
					"urlTemplate": "https://www.goodreads.com/book/show/{value}",
					"pattern": "^[0-9]+$"
				}
			]
		}
	}`)

	m, err := ParseManifest(data)
	require.NoError(t, err)
	require.Len(t, m.Capabilities.IdentifierTypes, 1)
	assert.Equal(t, "goodreads", m.Capabilities.IdentifierTypes[0].ID)
	assert.Equal(t, "^[0-9]+$", m.Capabilities.IdentifierTypes[0].Pattern)
}

func TestParseManifest_FileParserReservedExtensions(t *testing.T) {
	data := []byte(`{
		"manifestVersion": 1,
		"id": "epub-alt",
		"name": "Alt EPUB Parser",
		"version": "1.0.0",
		"capabilities": {
			"fileParser": {
				"description": "Tries to override epub",
				"types": ["epub", "pdf"]
			}
		}
	}`)

	m, err := ParseManifest(data)
	require.NoError(t, err)
	// Parsing succeeds but reserved types should be flagged during load
	assert.Contains(t, m.Capabilities.FileParser.Types, "epub")
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugins-sp && go test ./pkg/plugins/ -run TestParseManifest -v`
Expected: FAIL (package doesn't exist yet)

**Step 3: Write manifest types and parser**

```go
package plugins

import (
	"encoding/json"

	"github.com/pkg/errors"
)

// SupportedManifestVersions lists manifest versions this Shisho release supports.
var SupportedManifestVersions = []int{1}

// Manifest represents a parsed plugin manifest.json.
type Manifest struct {
	ManifestVersion int          `json:"manifestVersion"`
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	Version         string       `json:"version"`
	Description     string       `json:"description"`
	Author          string       `json:"author"`
	Homepage        string       `json:"homepage"`
	License         string       `json:"license"`
	MinShishoVersion string      `json:"minShishoVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ConfigSchema    ConfigSchema `json:"configSchema"`
}

// Capabilities declares what hooks and access a plugin requires.
type Capabilities struct {
	InputConverter   *InputConverterCap   `json:"inputConverter"`
	FileParser       *FileParserCap       `json:"fileParser"`
	OutputGenerator  *OutputGeneratorCap  `json:"outputGenerator"`
	MetadataEnricher *MetadataEnricherCap `json:"metadataEnricher"`
	IdentifierTypes  []IdentifierTypeCap  `json:"identifierTypes"`
	HTTPAccess       *HTTPAccessCap       `json:"httpAccess"`
	FileAccess       *FileAccessCap       `json:"fileAccess"`
	FFmpegAccess     *FFmpegAccessCap     `json:"ffmpegAccess"`
}

// InputConverterCap declares inputConverter capabilities.
type InputConverterCap struct {
	Description string   `json:"description"`
	SourceTypes []string `json:"sourceTypes"`
	MIMETypes   []string `json:"mimeTypes"`
	TargetType  string   `json:"targetType"`
}

// FileParserCap declares fileParser capabilities.
type FileParserCap struct {
	Description string   `json:"description"`
	Types       []string `json:"types"`
	MIMETypes   []string `json:"mimeTypes"`
}

// OutputGeneratorCap declares outputGenerator capabilities.
type OutputGeneratorCap struct {
	Description string   `json:"description"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	SourceTypes []string `json:"sourceTypes"`
}

// MetadataEnricherCap declares metadataEnricher capabilities.
type MetadataEnricherCap struct {
	Description string   `json:"description"`
	FileTypes   []string `json:"fileTypes"`
}

// IdentifierTypeCap declares a custom identifier type.
type IdentifierTypeCap struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URLTemplate string `json:"urlTemplate"`
	Pattern     string `json:"pattern"`
}

// HTTPAccessCap declares network access requirements.
type HTTPAccessCap struct {
	Description string   `json:"description"`
	Domains     []string `json:"domains"`
}

// FileAccessCap declares file system access level.
type FileAccessCap struct {
	Level       string `json:"level"` // "read" or "readwrite"
	Description string `json:"description"`
}

// FFmpegAccessCap declares FFmpeg subprocess access.
type FFmpegAccessCap struct {
	Description string `json:"description"`
}

// ConfigSchema maps config key names to their schema definitions.
type ConfigSchema map[string]ConfigField

// ConfigField defines a single configuration field.
type ConfigField struct {
	Type        string         `json:"type"` // string, boolean, number, select, textarea
	Label       string         `json:"label"`
	Description string         `json:"description"`
	Required    bool           `json:"required"`
	Secret      bool           `json:"secret"`
	Default     interface{}    `json:"default"`
	Min         *float64       `json:"min"`
	Max         *float64       `json:"max"`
	Options     []SelectOption `json:"options"`
}

// SelectOption is a value/label pair for select-type config fields.
type SelectOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ParseManifest parses and validates a manifest.json byte slice.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, errors.Wrap(err, "failed to parse manifest JSON")
	}

	if err := validateManifest(&m); err != nil {
		return nil, err
	}

	return &m, nil
}

func validateManifest(m *Manifest) error {
	if m.ManifestVersion == 0 {
		return errors.New("manifestVersion is required")
	}
	if !isSupportedVersion(m.ManifestVersion) {
		return errors.Errorf("unsupported manifest version %d (supported: %v)", m.ManifestVersion, SupportedManifestVersions)
	}
	if m.ID == "" {
		return errors.New("id is required")
	}
	if m.Name == "" {
		return errors.New("name is required")
	}
	if m.Version == "" {
		return errors.New("version is required")
	}
	return nil
}

func isSupportedVersion(v int) bool {
	for _, sv := range SupportedManifestVersions {
		if v == sv {
			return true
		}
	}
	return false
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugins-sp && go test ./pkg/plugins/ -run TestParseManifest -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add pkg/plugins/manifest.go pkg/plugins/manifest_test.go
git commit -m "[Plugins] Add manifest parsing and validation"
```

---

### Task 4: Plugin Service — CRUD Operations

**Files:**
- Create: `pkg/plugins/service.go`
- Create: `pkg/plugins/service_test.go`

**Step 1: Write tests for the service**

Test that the service can create, list, retrieve, update, and delete plugin records. Test config get/set with secret masking. Use an in-memory SQLite database.

The test file should cover:
- `TestService_InstallPlugin` — inserts plugin row, returns it
- `TestService_ListPlugins` — lists all installed plugins
- `TestService_UpdatePlugin_EnableDisable` — toggles enabled field
- `TestService_UninstallPlugin` — deletes plugin row and cascaded configs/orders
- `TestService_GetConfig` — returns config values, masks secrets
- `TestService_SetConfig` — validates and stores config, rejects invalid required fields
- `TestService_GetOrder` / `TestService_SetOrder` — manages plugin hook ordering

**Step 2: Run tests to verify they fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugins-sp && go test ./pkg/plugins/ -run TestService -v`
Expected: FAIL

**Step 3: Write the service implementation**

```go
package plugins

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// Service provides database operations for plugin management.
type Service struct {
	db *bun.DB
}

// NewService creates a new plugin service.
func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// InstallPlugin inserts a new plugin record.
func (s *Service) InstallPlugin(ctx context.Context, plugin *models.Plugin) error {
	plugin.InstalledAt = time.Now()
	_, err := s.db.NewInsert().Model(plugin).Exec(ctx)
	return errors.WithStack(err)
}

// ListPlugins returns all installed plugins.
func (s *Service) ListPlugins(ctx context.Context) ([]*models.Plugin, error) {
	var plugins []*models.Plugin
	err := s.db.NewSelect().Model(&plugins).OrderExpr("scope, id").Scan(ctx)
	return plugins, errors.WithStack(err)
}

// RetrievePlugin returns a single plugin by scope and ID.
func (s *Service) RetrievePlugin(ctx context.Context, scope, id string) (*models.Plugin, error) {
	var plugin models.Plugin
	err := s.db.NewSelect().Model(&plugin).
		Where("scope = ? AND id = ?", scope, id).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &plugin, nil
}

// UpdatePlugin updates a plugin record (enabled, load_error, update_available_version).
func (s *Service) UpdatePlugin(ctx context.Context, plugin *models.Plugin) error {
	_, err := s.db.NewUpdate().Model(plugin).
		Where("scope = ? AND id = ?", plugin.Scope, plugin.ID).
		Exec(ctx)
	return errors.WithStack(err)
}

// UninstallPlugin removes a plugin and all cascaded data.
func (s *Service) UninstallPlugin(ctx context.Context, scope, id string) error {
	_, err := s.db.NewDelete().Model((*models.Plugin)(nil)).
		Where("scope = ? AND id = ?", scope, id).
		Exec(ctx)
	return errors.WithStack(err)
}

// GetConfig returns all config values for a plugin.
// Secret values are masked with "***" unless raw=true.
func (s *Service) GetConfig(ctx context.Context, scope, pluginID string, schema ConfigSchema, raw bool) (map[string]interface{}, error) {
	var configs []*models.PluginConfig
	err := s.db.NewSelect().Model(&configs).
		Where("scope = ? AND plugin_id = ?", scope, pluginID).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	result := make(map[string]interface{})
	for _, cfg := range configs {
		if !raw {
			if field, ok := schema[cfg.Key]; ok && field.Secret {
				if cfg.Value != nil {
					masked := "***"
					result[cfg.Key] = masked
				}
				continue
			}
		}
		if cfg.Value != nil {
			result[cfg.Key] = *cfg.Value
		}
	}
	return result, nil
}

// SetConfig upserts a config value for a plugin.
func (s *Service) SetConfig(ctx context.Context, scope, pluginID, key, value string) error {
	config := &models.PluginConfig{
		Scope:    scope,
		PluginID: pluginID,
		Key:      key,
		Value:    &value,
	}
	_, err := s.db.NewInsert().Model(config).
		On("CONFLICT (scope, plugin_id, key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Exec(ctx)
	return errors.WithStack(err)
}

// GetConfigRaw returns the raw value of a single config key (for runtime use).
func (s *Service) GetConfigRaw(ctx context.Context, scope, pluginID, key string) (*string, error) {
	var config models.PluginConfig
	err := s.db.NewSelect().Model(&config).
		Where("scope = ? AND plugin_id = ? AND key = ?", scope, pluginID, key).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return config.Value, nil
}

// GetOrder returns the ordered list of plugin IDs for a hook type.
func (s *Service) GetOrder(ctx context.Context, hookType string) ([]*models.PluginOrder, error) {
	var orders []*models.PluginOrder
	err := s.db.NewSelect().Model(&orders).
		Where("hook_type = ?", hookType).
		Order("position ASC").
		Scan(ctx)
	return orders, errors.WithStack(err)
}

// SetOrder replaces the ordering for a hook type with the given list.
func (s *Service) SetOrder(ctx context.Context, hookType string, entries []models.PluginOrder) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	defer tx.Rollback()

	// Delete existing order for this hook type
	_, err = tx.NewDelete().Model((*models.PluginOrder)(nil)).
		Where("hook_type = ?", hookType).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	// Insert new order
	for i := range entries {
		entries[i].HookType = hookType
		entries[i].Position = i
	}
	if len(entries) > 0 {
		_, err = tx.NewInsert().Model(&entries).Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return errors.WithStack(tx.Commit())
}

// AppendToOrder appends a plugin to the end of a hook type's order.
func (s *Service) AppendToOrder(ctx context.Context, hookType, scope, pluginID string) error {
	// Get current max position
	var maxPos int
	err := s.db.NewSelect().
		TableExpr("plugin_order").
		ColumnExpr("COALESCE(MAX(position), -1)").
		Where("hook_type = ?", hookType).
		Scan(ctx, &maxPos)
	if err != nil {
		return errors.WithStack(err)
	}

	order := &models.PluginOrder{
		HookType: hookType,
		Scope:    scope,
		PluginID: pluginID,
		Position: maxPos + 1,
	}
	_, err = s.db.NewInsert().Model(order).Exec(ctx)
	return errors.WithStack(err)
}

// ListRepositories returns all configured repositories.
func (s *Service) ListRepositories(ctx context.Context) ([]*models.PluginRepository, error) {
	var repos []*models.PluginRepository
	err := s.db.NewSelect().Model(&repos).Order("is_official DESC", "scope ASC").Scan(ctx)
	return repos, errors.WithStack(err)
}

// AddRepository inserts a new repository.
func (s *Service) AddRepository(ctx context.Context, repo *models.PluginRepository) error {
	_, err := s.db.NewInsert().Model(repo).Exec(ctx)
	return errors.WithStack(err)
}

// RemoveRepository deletes a non-official repository.
func (s *Service) RemoveRepository(ctx context.Context, scope string) error {
	_, err := s.db.NewDelete().Model((*models.PluginRepository)(nil)).
		Where("scope = ? AND is_official = false", scope).
		Exec(ctx)
	return errors.WithStack(err)
}

// UpdateRepository updates a repository record (last_fetched_at, fetch_error, etc.).
func (s *Service) UpdateRepository(ctx context.Context, repo *models.PluginRepository) error {
	_, err := s.db.NewUpdate().Model(repo).
		Where("url = ?", repo.URL).
		Exec(ctx)
	return errors.WithStack(err)
}

// UpsertIdentifierTypes replaces all identifier types for a plugin.
func (s *Service) UpsertIdentifierTypes(ctx context.Context, scope, pluginID string, types []IdentifierTypeCap) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	defer tx.Rollback()

	// Delete existing
	_, err = tx.NewDelete().Model((*models.PluginIdentifierType)(nil)).
		Where("scope = ? AND plugin_id = ?", scope, pluginID).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	// Insert new
	if len(types) > 0 {
		models := make([]*models.PluginIdentifierType, len(types))
		for i, t := range types {
			urlTemplate := t.URLTemplate
			pattern := t.Pattern
			models[i] = &models.PluginIdentifierType{
				ID:          t.ID,
				Scope:       scope,
				PluginID:    pluginID,
				Name:        t.Name,
				URLTemplate: &urlTemplate,
				Pattern:     &pattern,
			}
		}
		_, err = tx.NewInsert().Model(&models).Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return errors.WithStack(tx.Commit())
}
```

**Step 4: Run tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugins-sp && go test ./pkg/plugins/ -run TestService -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add pkg/plugins/service.go pkg/plugins/service_test.go
git commit -m "[Plugins] Add plugin service for database CRUD operations"
```

---

### Task 5: Goja Runtime — Plugin Loader

**Files:**
- Create: `pkg/plugins/runtime.go`
- Create: `pkg/plugins/runtime_test.go`
- Create: `pkg/plugins/testdata/simple-plugin/manifest.json`
- Create: `pkg/plugins/testdata/simple-plugin/main.js`

**Step 1: Create test fixtures**

`testdata/simple-plugin/manifest.json`:
```json
{
  "manifestVersion": 1,
  "id": "simple-plugin",
  "name": "Simple Plugin",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Test enricher",
      "fileTypes": ["epub"]
    }
  }
}
```

`testdata/simple-plugin/main.js`:
```javascript
var plugin = (function() {
  var metadataEnricher = {
    name: "Simple Enricher",
    fileTypes: ["epub"],
    enrich: function(context) {
      return { modified: false };
    }
  };
  return { metadataEnricher: metadataEnricher };
})();
```

**Step 2: Write tests for runtime loading**

Test cases:
- `TestRuntime_LoadPlugin` — loads a valid plugin from testdata, extracts hook names
- `TestRuntime_LoadPlugin_MissingMainJS` — returns error for missing main.js
- `TestRuntime_LoadPlugin_InvalidJS` — returns error for syntax errors in main.js
- `TestRuntime_LoadPlugin_MissingHook` — warns when manifest declares capability but JS doesn't export it

**Step 3: Run tests to verify they fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugins-sp && go test ./pkg/plugins/ -run TestRuntime -v`
Expected: FAIL

**Step 4: Write the runtime implementation**

The `Runtime` struct wraps a goja VM for a single plugin. It loads `main.js`, evaluates it, reads the `plugin` global object, and extracts hook function references.

```go
package plugins

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/dop251/goja"
	"github.com/pkg/errors"
)

// Runtime represents an isolated JavaScript runtime for a single plugin.
type Runtime struct {
	vm       *goja.Runtime
	mu       sync.RWMutex // Read lock for hook invocation, write lock for reload
	manifest *Manifest
	scope    string
	pluginID string

	// Hook function references (nil if not provided by the plugin)
	inputConverter   goja.Value
	fileParser       goja.Value
	outputGenerator  goja.Value
	metadataEnricher goja.Value
}

// LoadPlugin creates a new Runtime by reading manifest.json and executing main.js
// from the given plugin directory.
func LoadPlugin(dir, scope, pluginID string) (*Runtime, error) {
	// Read and parse manifest
	manifestPath := filepath.Join(dir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read manifest.json")
	}

	manifest, err := ParseManifest(manifestData)
	if err != nil {
		return nil, errors.Wrap(err, "invalid manifest")
	}

	// Read main.js
	mainJSPath := filepath.Join(dir, "main.js")
	mainJS, err := os.ReadFile(mainJSPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read main.js")
	}

	// Create goja runtime
	vm := goja.New()

	// Execute main.js (IIFE that assigns to `plugin` global)
	_, err = vm.RunString(string(mainJS))
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute main.js")
	}

	// Read the `plugin` global
	pluginObj := vm.Get("plugin")
	if pluginObj == nil || goja.IsUndefined(pluginObj) || goja.IsNull(pluginObj) {
		return nil, errors.New("main.js did not define a `plugin` global object")
	}

	rt := &Runtime{
		vm:       vm,
		manifest: manifest,
		scope:    scope,
		pluginID: pluginID,
	}

	// Extract hook references
	obj := pluginObj.ToObject(vm)
	rt.inputConverter = obj.Get("inputConverter")
	rt.fileParser = obj.Get("fileParser")
	rt.outputGenerator = obj.Get("outputGenerator")
	rt.metadataEnricher = obj.Get("metadataEnricher")

	// Validate: manifest declares capability but JS doesn't export it
	var warnings []string
	if manifest.Capabilities.InputConverter != nil && !rt.hasHook(rt.inputConverter) {
		warnings = append(warnings, "manifest declares inputConverter but main.js does not export it")
	}
	if manifest.Capabilities.FileParser != nil && !rt.hasHook(rt.fileParser) {
		warnings = append(warnings, "manifest declares fileParser but main.js does not export it")
	}
	if manifest.Capabilities.OutputGenerator != nil && !rt.hasHook(rt.outputGenerator) {
		warnings = append(warnings, "manifest declares outputGenerator but main.js does not export it")
	}
	if manifest.Capabilities.MetadataEnricher != nil && !rt.hasHook(rt.metadataEnricher) {
		warnings = append(warnings, "manifest declares metadataEnricher but main.js does not export it")
	}

	// Also validate the reverse: JS exports hook but manifest doesn't declare it
	if rt.hasHook(rt.inputConverter) && manifest.Capabilities.InputConverter == nil {
		return nil, errors.New("main.js exports inputConverter but manifest does not declare the capability")
	}
	if rt.hasHook(rt.fileParser) && manifest.Capabilities.FileParser == nil {
		return nil, errors.New("main.js exports fileParser but manifest does not declare the capability")
	}
	if rt.hasHook(rt.outputGenerator) && manifest.Capabilities.OutputGenerator == nil {
		return nil, errors.New("main.js exports outputGenerator but manifest does not declare the capability")
	}
	if rt.hasHook(rt.metadataEnricher) && manifest.Capabilities.MetadataEnricher == nil {
		return nil, errors.New("main.js exports metadataEnricher but manifest does not declare the capability")
	}

	_ = warnings // TODO: log warnings

	return rt, nil
}

// hasHook returns true if the goja.Value is a non-null, non-undefined object.
func (rt *Runtime) hasHook(v goja.Value) bool {
	return v != nil && !goja.IsUndefined(v) && !goja.IsNull(v)
}

// Manifest returns the parsed manifest for this plugin.
func (rt *Runtime) Manifest() *Manifest {
	return rt.manifest
}

// HookTypes returns the list of hook types this plugin provides.
func (rt *Runtime) HookTypes() []string {
	var types []string
	if rt.hasHook(rt.inputConverter) {
		types = append(types, "inputConverter")
	}
	if rt.hasHook(rt.fileParser) {
		types = append(types, "fileParser")
	}
	if rt.hasHook(rt.outputGenerator) {
		types = append(types, "outputGenerator")
	}
	if rt.hasHook(rt.metadataEnricher) {
		types = append(types, "metadataEnricher")
	}
	return types
}

// Lock acquires a read lock for hook invocation.
func (rt *Runtime) Lock() { rt.mu.RLock() }

// Unlock releases the read lock.
func (rt *Runtime) Unlock() { rt.mu.RUnlock() }
```

**Step 5: Run tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugins-sp && go test ./pkg/plugins/ -run TestRuntime -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add pkg/plugins/runtime.go pkg/plugins/runtime_test.go pkg/plugins/testdata/
git commit -m "[Plugins] Add goja runtime loader for plugin execution"
```

---

### Task 6: Plugin Host APIs — shisho.log, shisho.config

**Files:**
- Create: `pkg/plugins/hostapi.go`
- Create: `pkg/plugins/hostapi_test.go`

**Step 1: Write tests**

Test that `shisho.log.info("msg")` calls the Go logger, and `shisho.config.get("key")` returns the correct value from the database.

**Step 2: Run tests to verify failure**

**Step 3: Write the host API injector**

The `InjectHostAPIs` function sets up the `shisho` global object in a goja runtime with `log`, `config`, `fs`, `http`, `xml`, `archive`, and `ffmpeg` namespaces. Each namespace delegates to Go functions.

For Phase 1, implement `log` and `config` only. Other APIs (`http`, `fs`, `archive`, `xml`, `ffmpeg`) are stubs that return errors until Phase 2.

Key patterns:
- `shisho.log.info(msg)` → calls `logger.Info(msg, data{"plugin": "scope/id"})`
- `shisho.config.get(key)` → reads from `PluginConfig` table via service
- `shisho.config.getAll()` → reads all config for the plugin

**Step 4: Run tests**

**Step 5: Commit**

```bash
git add pkg/plugins/hostapi.go pkg/plugins/hostapi_test.go
git commit -m "[Plugins] Add shisho.log and shisho.config host APIs"
```

---

### Task 7: Plugin Manager — Registry & Lifecycle

**Files:**
- Create: `pkg/plugins/manager.go`
- Create: `pkg/plugins/manager_test.go`

**Step 1: Write tests**

- `TestManager_LoadAll` — loads all enabled plugins from a test directory
- `TestManager_LoadPlugin` — loads a single plugin, registers its hooks
- `TestManager_UnloadPlugin` — unloads a plugin, deregisters hooks
- `TestManager_HotReload` — replaces a plugin runtime without restart
- `TestManager_GetOrderedPlugins` — returns plugins in user-defined order for a hook type

**Step 2: Run tests to verify failure**

**Step 3: Write the manager**

The `Manager` holds a map of loaded `Runtime` instances indexed by `scope/id`. It coordinates loading at startup and hot-reloading on install/update/enable.

```go
package plugins

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
)

// Manager coordinates plugin loading, unloading, and hook dispatch.
type Manager struct {
	mu       sync.RWMutex
	plugins  map[string]*Runtime // key: "scope/id"
	service  *Service
	pluginDir string
}

// NewManager creates a new plugin manager.
func NewManager(service *Service, pluginDir string) *Manager {
	return &Manager{
		plugins:   make(map[string]*Runtime),
		service:   service,
		pluginDir: pluginDir,
	}
}

// LoadAll loads all enabled plugins from the database at startup.
func (m *Manager) LoadAll(ctx context.Context) error {
	plugins, err := m.service.ListPlugins(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list plugins")
	}

	log := logger.FromContext(ctx)

	for _, p := range plugins {
		if !p.Enabled {
			continue
		}

		if err := m.loadPlugin(ctx, p.Scope, p.ID); err != nil {
			// Store load error in DB, continue with other plugins
			loadErr := err.Error()
			p.LoadError = &loadErr
			_ = m.service.UpdatePlugin(ctx, p)
			log.Warn("failed to load plugin", logger.Data{
				"plugin": p.Scope + "/" + p.ID,
				"error":  loadErr,
			})
			continue
		}

		// Clear any previous load error
		if p.LoadError != nil {
			p.LoadError = nil
			_ = m.service.UpdatePlugin(ctx, p)
		}
	}

	return nil
}

// loadPlugin loads a single plugin from disk.
func (m *Manager) loadPlugin(ctx context.Context, scope, id string) error {
	dir := filepath.Join(m.pluginDir, scope, id)

	rt, err := LoadPlugin(dir, scope, id)
	if err != nil {
		return err
	}

	// Inject host APIs
	if err := InjectHostAPIs(rt, m.service); err != nil {
		return errors.Wrap(err, "failed to inject host APIs")
	}

	key := scope + "/" + id
	m.mu.Lock()
	m.plugins[key] = rt
	m.mu.Unlock()

	// Register identifier types
	if len(rt.manifest.Capabilities.IdentifierTypes) > 0 {
		if err := m.service.UpsertIdentifierTypes(ctx, scope, id, rt.manifest.Capabilities.IdentifierTypes); err != nil {
			return errors.Wrap(err, "failed to register identifier types")
		}
	}

	// Register hook ordering (append new hooks)
	for _, hookType := range rt.HookTypes() {
		if err := m.service.AppendToOrder(ctx, hookType, scope, id); err != nil {
			// Ignore duplicate key errors (already in order table)
			continue
		}
	}

	return nil
}

// UnloadPlugin removes a plugin from the manager.
func (m *Manager) UnloadPlugin(scope, id string) {
	key := scope + "/" + id
	m.mu.Lock()
	delete(m.plugins, key)
	m.mu.Unlock()
}

// ReloadPlugin performs a hot-reload of a plugin.
func (m *Manager) ReloadPlugin(ctx context.Context, scope, id string) error {
	key := scope + "/" + id

	m.mu.RLock()
	oldRT := m.plugins[key]
	m.mu.RUnlock()

	// Load new runtime
	dir := filepath.Join(m.pluginDir, scope, id)
	newRT, err := LoadPlugin(dir, scope, id)
	if err != nil {
		return err
	}

	if err := InjectHostAPIs(newRT, m.service); err != nil {
		return errors.Wrap(err, "failed to inject host APIs")
	}

	// Acquire write lock on old runtime (waits for in-progress hooks)
	if oldRT != nil {
		oldRT.mu.Lock()
		defer oldRT.mu.Unlock()
	}

	// Swap
	m.mu.Lock()
	m.plugins[key] = newRT
	m.mu.Unlock()

	// Update identifier types
	if len(newRT.manifest.Capabilities.IdentifierTypes) > 0 {
		_ = m.service.UpsertIdentifierTypes(ctx, scope, id, newRT.manifest.Capabilities.IdentifierTypes)
	}

	return nil
}

// GetRuntime returns the runtime for a plugin (nil if not loaded).
func (m *Manager) GetRuntime(scope, id string) *Runtime {
	key := scope + "/" + id
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.plugins[key]
}

// GetOrderedRuntimes returns runtimes for a hook type in user-defined order.
func (m *Manager) GetOrderedRuntimes(ctx context.Context, hookType string) ([]*Runtime, error) {
	orders, err := m.service.GetOrder(ctx, hookType)
	if err != nil {
		return nil, err
	}

	var runtimes []*Runtime
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, order := range orders {
		key := order.Scope + "/" + order.PluginID
		if rt, ok := m.plugins[key]; ok {
			runtimes = append(runtimes, rt)
		}
	}
	return runtimes, nil
}

// RegisteredFileExtensions returns all file extensions registered by plugin fileParsers.
func (m *Manager) RegisteredFileExtensions() map[string]struct{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	exts := make(map[string]struct{})
	for _, rt := range m.plugins {
		if rt.manifest.Capabilities.FileParser != nil {
			for _, ext := range rt.manifest.Capabilities.FileParser.Types {
				// Skip reserved built-in extensions
				if ext == "epub" || ext == "cbz" || ext == "m4b" {
					continue
				}
				exts[ext] = struct{}{}
			}
		}
	}
	return exts
}

// RegisteredConverterExtensions returns source extensions that have input converters.
func (m *Manager) RegisteredConverterExtensions() map[string]struct{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	exts := make(map[string]struct{})
	for _, rt := range m.plugins {
		if rt.manifest.Capabilities.InputConverter != nil {
			for _, ext := range rt.manifest.Capabilities.InputConverter.SourceTypes {
				exts[ext] = struct{}{}
			}
		}
	}
	return exts
}
```

**Step 4: Run tests**

**Step 5: Commit**

```bash
git add pkg/plugins/manager.go pkg/plugins/manager_test.go
git commit -m "[Plugins] Add plugin manager for lifecycle and hook dispatch"
```

---

### Task 8: Wire Plugin Manager into App Startup

**Files:**
- Modify: `pkg/config/config.go` — add `PluginDir` field
- Modify: `cmd/api/main.go` — create and start plugin manager
- Modify: `pkg/server/server.go` — pass manager to route registration
- Modify: `pkg/worker/worker.go` — accept and store plugin manager

**Step 1: Add PluginDir to config**

Add a `PluginDir` field to `Config` struct, defaulting to `/config/plugins/installed` (Docker) or `tmp/plugins/installed` (development).

**Step 2: Create plugin manager in main.go**

After migrations run and before worker starts:
```go
pluginService := plugins.NewService(db)
pluginManager := plugins.NewManager(pluginService, cfg.PluginDir)
if err := pluginManager.LoadAll(ctx); err != nil {
    log.Warn("plugin load errors occurred", logger.Data{"error": err.Error()})
}
```

**Step 3: Pass manager to worker and server**

Update `worker.New()` to accept `*plugins.Manager`. Update `server.New()` to accept `*plugins.Manager`.

**Step 4: Run full build**

Run: `make build`
Expected: Compiles successfully.

**Step 5: Commit**

```bash
git add cmd/api/main.go pkg/config/config.go pkg/server/server.go pkg/worker/worker.go
git commit -m "[Plugins] Wire plugin manager into app startup and worker"
```

---

### Task 9: Plugin API Endpoints — Installed Plugins CRUD

**Files:**
- Create: `pkg/plugins/routes.go`
- Create: `pkg/plugins/handler.go`

**Step 1: Write the handler with standard Echo patterns**

Endpoints:
- `GET /plugins/installed` — list installed plugins
- `POST /plugins/installed` — install a plugin `{scope, id, version?}`
- `DELETE /plugins/installed/:scope/:id` — uninstall
- `PATCH /plugins/installed/:scope/:id` — update enabled/config
- `GET /plugins/order/:hookType` — get ordering
- `PUT /plugins/order/:hookType` — set ordering

Follow the existing pattern: handler struct with injected services, route registration via `RegisterRoutesWithGroup`.

**Step 2: Register routes in server.go**

Add a `pluginsGroup` in `registerProtectedRoutes` with admin-only permissions.

**Step 3: Run the app and test with curl**

Run: `make start:air`
Then: `curl -H "Authorization: Bearer <token>" http://localhost:3000/plugins/installed`
Expected: `[]` (empty array, no plugins installed)

**Step 4: Commit**

```bash
git add pkg/plugins/routes.go pkg/plugins/handler.go pkg/server/server.go
git commit -m "[Plugins] Add API endpoints for plugin management"
```

---

### Task 10: Plugin API Endpoints — Repositories

**Files:**
- Modify: `pkg/plugins/handler.go` — add repository handlers
- Modify: `pkg/plugins/routes.go` — add repository routes

**Step 1: Add repository handlers**

Endpoints:
- `GET /plugins/repositories` — list repos
- `POST /plugins/repositories` — add repo `{url, scope}`
- `DELETE /plugins/repositories/:scope` — remove repo (not official)
- `POST /plugins/repositories/:scope/sync` — refresh manifest

**Step 2: Add URL validation for GitHub-only URLs**

Validate that repository URLs match `https://raw.githubusercontent.com/{owner}/{repo}/...` pattern.

**Step 3: Test with curl**

**Step 4: Commit**

```bash
git add pkg/plugins/handler.go pkg/plugins/routes.go
git commit -m "[Plugins] Add repository management API endpoints"
```

---

### Task 11: DataSource Priority — Add Plugin Level

**Files:**
- Modify: `pkg/models/data-source.go` — add `DataSourcePlugin` and priority level

**Step 1: Add the new data source**

Between `DataSourceSidecarPriority` (1) and `DataSourceFileMetadataPriority` (2), add `DataSourcePluginPriority` = 2. Shift file metadata to 3 and filepath to 4.

```go
const (
    DataSourcePlugin = "plugin"
)

// Updated priorities:
// Manual: 0, Sidecar: 1, Plugin: 2, FileMetadata: 3, Filepath: 4
```

Also update the `DataSource` tstype emit line to include `DataSourcePlugin`.

**Step 2: Run make tygo**

Run: `make tygo`

**Step 3: Run tests to check for regressions**

Run: `make test`
Expected: All pass (priority changes shouldn't break existing logic since plugin sources don't exist yet)

**Step 4: Commit**

```bash
git add pkg/models/data-source.go
git commit -m "[Plugins] Add DataSourcePlugin priority level for enricher results"
```

---

## Phase 2: Hook Implementations

### Task 12: Host API — shisho.http.fetch

**Files:**
- Create: `pkg/plugins/hostapi_http.go`
- Create: `pkg/plugins/hostapi_http_test.go`

**Step 1: Write tests**

- Allowed domain resolves and returns response
- Blocked domain returns error
- Redirect to blocked domain returns error
- Response .json(), .text(), .bytes() work

**Step 2: Implement the HTTP client**

The `fetch` function checks the URL against the plugin's `httpAccess.domains` list, makes the request using a standard `http.Client` with redirect policy that blocks cross-domain redirects, and returns a Promise-like object to the JS runtime.

Key details:
- Domain check is exact match (subdomains must be listed)
- Only ports 80/443 unless explicitly specified
- Uses `goja.Runtime.NewPromise()` for async behavior

**Step 3: Run tests**

**Step 4: Commit**

```bash
git add pkg/plugins/hostapi_http.go pkg/plugins/hostapi_http_test.go
git commit -m "[Plugins] Add shisho.http.fetch host API with domain restrictions"
```

---

### Task 13: Host API — shisho.fs

**Files:**
- Create: `pkg/plugins/hostapi_fs.go`
- Create: `pkg/plugins/hostapi_fs_test.go`

**Step 1: Write tests**

- `readFile` / `readTextFile` work for allowed paths
- `writeFile` / `writeTextFile` work with readwrite access
- `tempDir()` returns consistent path per invocation
- Access to non-allowed paths returns error
- Hook-provided paths always accessible

**Step 2: Implement fs operations**

The file access level is checked against the manifest's `fileAccess.level`. A context object tracks which paths are accessible for this invocation (hook-provided paths are always allowed).

**Step 3: Run tests**

**Step 4: Commit**

```bash
git add pkg/plugins/hostapi_fs.go pkg/plugins/hostapi_fs_test.go
git commit -m "[Plugins] Add shisho.fs host API with access level enforcement"
```

---

### Task 14: Host API — shisho.archive and shisho.xml

**Files:**
- Create: `pkg/plugins/hostapi_archive.go`
- Create: `pkg/plugins/hostapi_xml.go`
- Create: `pkg/plugins/hostapi_archive_test.go`
- Create: `pkg/plugins/hostapi_xml_test.go`

**Step 1: Write tests for archive**

- `extractZip` extracts files to destination
- `createZip` creates a valid ZIP
- `readZipEntry` reads a specific file from ZIP
- `listZipEntries` lists all entries

**Step 2: Write tests for XML**

- `parse` creates an XMLDocument
- `querySelector` finds elements with namespace support
- `querySelectorAll` returns multiple matches

**Step 3: Implement both**

Archive uses Go's `archive/zip` package. XML uses `encoding/xml` with a custom CSS selector engine supporting namespace pipe notation.

**Step 4: Run tests**

**Step 5: Commit**

```bash
git add pkg/plugins/hostapi_archive.go pkg/plugins/hostapi_xml.go pkg/plugins/hostapi_archive_test.go pkg/plugins/hostapi_xml_test.go
git commit -m "[Plugins] Add shisho.archive and shisho.xml host APIs"
```

---

### Task 15: Host API — shisho.ffmpeg

**Files:**
- Create: `pkg/plugins/hostapi_ffmpeg.go`
- Create: `pkg/plugins/hostapi_ffmpeg_test.go`

**Step 1: Write tests**

- `run` executes ffmpeg with args and returns result
- Network protocols are disabled via `-protocol_whitelist file,pipe`
- Missing `ffmpegAccess` capability returns error
- Timeout kills the subprocess

**Step 2: Implement ffmpeg execution**

Uses `exec.CommandContext` to spawn the bundled FFmpeg binary. Prepends `-protocol_whitelist file,pipe` to the args. Returns `FFmpegResult{exitCode, stdout, stderr}`.

**Step 3: Run tests**

**Step 4: Commit**

```bash
git add pkg/plugins/hostapi_ffmpeg.go pkg/plugins/hostapi_ffmpeg_test.go
git commit -m "[Plugins] Add shisho.ffmpeg host API for subprocess execution"
```

---

### Task 16: Hook Execution — Input Converter

**Files:**
- Create: `pkg/plugins/hooks.go`
- Create: `pkg/plugins/hooks_test.go`

**Step 1: Write tests**

- `TestRunInputConverter` — calls the converter's `convert` function with context, returns target path
- `TestRunInputConverter_Timeout` — times out after 5 minutes
- `TestRunInputConverter_MIMEValidation` — skips files with wrong MIME type

**Step 2: Implement hook execution**

```go
// RunInputConverter invokes a plugin's inputConverter.convert() hook.
func (m *Manager) RunInputConverter(ctx context.Context, rt *Runtime, sourcePath, targetDir string) (*ConvertResult, error) {
    // Acquire read lock
    rt.mu.RLock()
    defer rt.mu.RUnlock()

    // Create context object
    // Call rt.vm's inputConverter.convert(context)
    // Parse result {success, targetPath}
}
```

**Step 3: Run tests**

**Step 4: Commit**

```bash
git add pkg/plugins/hooks.go pkg/plugins/hooks_test.go
git commit -m "[Plugins] Add input converter hook execution"
```

---

### Task 17: Hook Execution — File Parser

**Files:**
- Modify: `pkg/plugins/hooks.go` — add `RunFileParser`
- Modify: `pkg/plugins/hooks_test.go` — add parser tests

**Step 1: Write tests**

- `TestRunFileParser` — calls parser's `parse` function, maps result to `ParsedMetadata`
- `TestRunFileParser_AllFields` — verifies all optional fields are mapped correctly
- `TestRunFileParser_Timeout` — times out after 1 minute

**Step 2: Implement**

The `RunFileParser` function calls the plugin's `fileParser.parse(context)` and maps the JS object to a `mediafile.ParsedMetadata` struct.

**Step 3: Run tests**

**Step 4: Commit**

```bash
git add pkg/plugins/hooks.go pkg/plugins/hooks_test.go
git commit -m "[Plugins] Add file parser hook execution"
```

---

### Task 18: Hook Execution — Metadata Enricher

**Files:**
- Modify: `pkg/plugins/hooks.go` — add `RunMetadataEnricher`
- Modify: `pkg/plugins/hooks_test.go` — add enricher tests

**Step 1: Write tests**

- `TestRunMetadataEnricher` — returns modified metadata
- `TestRunMetadataEnricher_NotModified` — returns `{modified: false}`
- `TestRunMetadataEnricher_Timeout` — times out after 1 minute

**Step 2: Implement**

`RunMetadataEnricher` calls `metadataEnricher.enrich(context)` and returns an `EnrichmentResult` with the metadata fields to apply.

**Step 3: Run tests**

**Step 4: Commit**

```bash
git add pkg/plugins/hooks.go pkg/plugins/hooks_test.go
git commit -m "[Plugins] Add metadata enricher hook execution"
```

---

### Task 19: Hook Execution — Output Generator

**Files:**
- Modify: `pkg/plugins/hooks.go` — add `RunOutputGenerator` and `RunFingerprint`
- Modify: `pkg/plugins/hooks_test.go` — add generator tests

**Step 1: Write tests**

- `TestRunOutputGenerator` — calls `generate` with context, writes output file
- `TestRunFingerprint` — calls `fingerprint` and returns stable string
- `TestRunOutputGenerator_Timeout` — times out after 5 minutes

**Step 2: Implement**

**Step 3: Run tests**

**Step 4: Commit**

```bash
git add pkg/plugins/hooks.go pkg/plugins/hooks_test.go
git commit -m "[Plugins] Add output generator hook execution"
```

---

### Task 20: Scan Integration — File Discovery with Plugin Extensions

**Files:**
- Modify: `pkg/worker/scan.go` — extend `extensionsToScan` with plugin-registered extensions

**Step 1: Write test**

Create a test that registers a plugin with `fileParser: {types: ["pdf"]}`, then verifies that `.pdf` files are discovered during scan.

**Step 2: Run test to verify it fails**

**Step 3: Modify `ProcessScanJob`**

In the file discovery walk, after checking `extensionsToScan`, also check `m.pluginManager.RegisteredFileExtensions()`. Files matching plugin extensions are added to `filesToScan` (skip MIME validation here — that happens at parse time per the design).

Also check `m.pluginManager.RegisteredConverterExtensions()` for input converter source types.

**Step 4: Run tests**

Run: `make test`
Expected: All pass

**Step 5: Commit**

```bash
git add pkg/worker/scan.go pkg/worker/scan_test.go
git commit -m "[Plugins] Extend file discovery to include plugin-registered extensions"
```

---

### Task 21: Scan Integration — Input Converter Pipeline

**Files:**
- Modify: `pkg/worker/scan.go` — run input converters after file discovery

**Step 1: Write test**

Create a test with a mock plugin that converts `.pdf` → `.epub`. Verify that after scan, both the PDF and EPUB are indexed.

**Step 2: Run test to verify it fails**

**Step 3: Implement converter pipeline**

After file discovery but before parsing, iterate through discovered files and run applicable input converters (ordered by user preference). For each source→target pair, only the first converter runs. Successfully converted files are added back to the scan list.

Insert after `filesToScan` collection, before the main scan loop:
```go
// Run input converters on discovered files
convertedFiles := m.runInputConverters(ctx, filesToScan, library, jobLog)
filesToScan = append(filesToScan, convertedFiles...)
```

**Step 4: Run tests**

**Step 5: Commit**

```bash
git add pkg/worker/scan.go pkg/worker/scan_test.go
git commit -m "[Plugins] Integrate input converter hooks into scan pipeline"
```

---

### Task 22: Scan Integration — Plugin File Parser

**Files:**
- Modify: `pkg/worker/scan_unified.go` — extend `parseFileMetadata` to use plugin parsers

**Step 1: Write test**

Test that a file with a plugin-registered extension (e.g., `.pdf`) gets parsed by the plugin's `fileParser.parse()` function.

**Step 2: Run test to verify it fails**

**Step 3: Modify `parseFileMetadata`**

Add a `default` case that checks if a plugin parser exists for the file type. If so, call `m.pluginManager.RunFileParser()`. The worker needs access to the plugin manager — add it as a field.

```go
default:
    // Check for plugin file parser
    if w.pluginManager != nil {
        rt := w.pluginManager.GetParserForType(fileType)
        if rt != nil {
            return w.pluginManager.RunFileParser(ctx, rt, path)
        }
    }
    return nil, errors.Errorf("unsupported file type: %s", fileType)
```

**Step 4: Run tests**

**Step 5: Commit**

```bash
git add pkg/worker/scan_unified.go pkg/worker/worker.go
git commit -m "[Plugins] Integrate plugin file parsers into scan pipeline"
```

---

### Task 23: Scan Integration — Metadata Enricher Pipeline

**Files:**
- Modify: `pkg/worker/scan_unified.go` — run enrichers after metadata parsing

**Step 1: Write test**

Test that after file parsing, metadata enrichers run in order and the "first non-empty wins" rule is applied per field.

**Step 2: Run test to verify it fails**

**Step 3: Implement enricher pipeline**

In `scanFileCore`, after metadata is parsed and before saving to DB, run the enricher pipeline:

```go
// Run metadata enrichers (plugin hooks)
if w.pluginManager != nil {
    enrichedMeta := w.runMetadataEnrichers(ctx, metadata, book, file)
    // Merge enriched fields into metadata using priority rules
}
```

The enricher uses `DataSourcePlugin` as the source, which sits between sidecar and file metadata in priority.

**Step 4: Run tests**

**Step 5: Commit**

```bash
git add pkg/worker/scan_unified.go
git commit -m "[Plugins] Integrate metadata enricher hooks into scan pipeline"
```

---

### Task 24: Download Integration — Plugin Output Generators

**Files:**
- Modify: `pkg/downloadcache/cache.go` — support plugin format generation
- Modify: `pkg/filegen/generator.go` — add plugin generator adapter

**Step 1: Write test**

Test that requesting a download with `format=mobi` (a plugin-registered format) calls the plugin's `outputGenerator.generate()` and returns the generated file.

**Step 2: Run test to verify it fails**

**Step 3: Implement plugin generator adapter**

Create a `PluginGenerator` that implements the `filegen.Generator` interface by delegating to the plugin manager:

```go
type PluginGenerator struct {
    manager    *plugins.Manager
    generatorID string
    scope      string
    pluginID   string
}

func (g *PluginGenerator) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error {
    rt := g.manager.GetRuntime(g.scope, g.pluginID)
    return g.manager.RunOutputGenerator(ctx, rt, srcPath, destPath, book, file)
}
```

Extend `filegen.GetGenerator` to check plugin-registered generators when no built-in generator matches.

**Step 4: Run tests**

**Step 5: Commit**

```bash
git add pkg/downloadcache/cache.go pkg/filegen/generator.go
git commit -m "[Plugins] Integrate plugin output generators into download system"
```

---

## Phase 3: Repository & UI

### Task 25: Repository Fetcher

**Files:**
- Create: `pkg/plugins/repository.go`
- Create: `pkg/plugins/repository_test.go`

**Step 1: Write tests**

- `TestFetchRepository` — fetches and parses a repository manifest JSON
- `TestFetchRepository_InvalidURL` — rejects non-GitHub URLs
- `TestFetchRepository_ParseError` — handles malformed JSON

**Step 2: Implement repository fetcher**

Fetches the JSON manifest from a GitHub raw URL, parses it into a `RepositoryManifest` struct, validates the structure, and returns the list of available plugins with their versions.

**Step 3: Run tests**

**Step 4: Commit**

```bash
git add pkg/plugins/repository.go pkg/plugins/repository_test.go
git commit -m "[Plugins] Add repository manifest fetcher"
```

---

### Task 26: Plugin Installer (Download + Verify + Extract)

**Files:**
- Create: `pkg/plugins/installer.go`
- Create: `pkg/plugins/installer_test.go`

**Step 1: Write tests**

- Downloads plugin ZIP from GitHub releases URL
- Verifies SHA256 checksum
- Extracts to correct directory
- Rejects non-GitHub download URLs

**Step 2: Implement installer**

Downloads the ZIP, verifies checksum, extracts to `/config/plugins/installed/{scope}/{id}/`, reads manifest.json, and returns the manifest for capabilities display.

**Step 3: Run tests**

**Step 4: Commit**

```bash
git add pkg/plugins/installer.go pkg/plugins/installer_test.go
git commit -m "[Plugins] Add plugin installer with download, verify, and extract"
```

---

### Task 27: Available Plugins API (from repositories)

**Files:**
- Modify: `pkg/plugins/handler.go` — add available plugins endpoints
- Modify: `pkg/plugins/routes.go` — register routes

**Step 1: Add handlers**

- `GET /plugins/available` — aggregates plugins from all enabled repositories
- `GET /plugins/available/:scope/:id` — plugin details with all versions

**Step 2: Register routes**

**Step 3: Test with curl**

**Step 4: Commit**

```bash
git add pkg/plugins/handler.go pkg/plugins/routes.go
git commit -m "[Plugins] Add available plugins API from repositories"
```

---

### Task 28: Frontend — Plugin Settings Pages (Installed, Browser, Ordering, Repos)

**Files:**
- Create: `app/components/pages/PluginsInstalled.tsx`
- Create: `app/components/pages/PluginsBrowser.tsx`
- Create: `app/components/pages/PluginsOrder.tsx`
- Create: `app/components/pages/PluginsRepositories.tsx`
- Create: `app/hooks/queries/plugins.ts`
- Modify: `app/App.tsx` (or router) — add routes

Use the `frontend` skill for implementation details.

**Step 1: Create query hooks**

Define Tanstack Query hooks for all plugin API endpoints:
- `usePluginsInstalled()`, `usePluginsBrowser()`, `usePluginOrder(hookType)`, `usePluginRepositories()`
- Mutations: `useInstallPlugin()`, `useUninstallPlugin()`, `useUpdatePluginConfig()`, `useSetPluginOrder()`

**Step 2: Build the Installed Plugins page**

List view with enable/disable toggles, configure button, update badge, uninstall button. Shows load errors in red.

**Step 3: Build the Plugin Browser page**

Search/filter UI showing available plugins from repositories. Install button triggers download + capabilities confirmation dialog.

**Step 4: Build the Ordering page**

Tabs per hook type. Drag-and-drop reordering of enabled plugins.

**Step 5: Build the Repositories page**

List repos with scope, URL, last sync, add/remove buttons. Official repo has star icon, cannot be removed.

**Step 6: Add routes**

Register `/settings/plugins/installed`, `/settings/plugins/browse`, `/settings/plugins/order`, `/settings/plugins/repositories` in the router.

**Step 7: Commit**

```bash
git add app/components/pages/Plugins*.tsx app/hooks/queries/plugins.ts app/App.tsx
git commit -m "[Plugins] Add frontend plugin management pages"
```

---

### Task 29: Security Warning Dialog

**Files:**
- Create: `app/components/plugins/CapabilitiesWarning.tsx`

**Step 1: Build the warning component**

A dialog shown during plugin installation that displays the plugin's requested permissions based on its manifest capabilities:
- Network access domains
- File access level
- FFmpeg execution
- Hook types (what the plugin can do)

**Step 2: Integrate into install flow**

After download, show this dialog. User must confirm before plugin is enabled.

**Step 3: Commit**

```bash
git add app/components/plugins/CapabilitiesWarning.tsx
git commit -m "[Plugins] Add capabilities warning dialog for plugin installation"
```

---

## Phase 4: Polish

### Task 30: Daily Update Check Job

**Files:**
- Modify: `pkg/worker/worker.go` — add scheduled update check

**Step 1: Write test**

Test that the update checker fetches all repository manifests, compares versions, and sets `update_available_version` for plugins with newer compatible versions.

**Step 2: Implement**

Add a daily scheduled job (via the existing scheduler goroutine pattern) that:
1. Fetches all enabled repository manifests
2. For each installed plugin, finds the latest compatible version
3. Sets `plugins.update_available_version` if newer than installed

**Step 3: Run tests**

**Step 4: Commit**

```bash
git add pkg/worker/worker.go
git commit -m "[Plugins] Add daily update check for installed plugins"
```

---

### Task 31: Plugin Update Flow

**Files:**
- Modify: `pkg/plugins/handler.go` — add update endpoint handler
- Modify: `pkg/plugins/installer.go` — add update logic

**Step 1: Implement POST /plugins/installed/:scope/:id/update**

Downloads new version, verifies SHA256, replaces files on disk, hot-reloads the plugin.

**Step 2: Test manually**

**Step 3: Commit**

```bash
git add pkg/plugins/handler.go pkg/plugins/installer.go
git commit -m "[Plugins] Add plugin update flow with hot-reload"
```

---

### Task 32: Manual Install Scan

**Files:**
- Modify: `pkg/plugins/handler.go` — add scan endpoint

**Step 1: Implement POST /plugins/scan**

Walks `/config/plugins/installed/local/` for directories not in the DB with scope `"local"`. Inserts rows with `enabled = false`.

**Step 2: Test manually**

**Step 3: Commit**

```bash
git add pkg/plugins/handler.go
git commit -m "[Plugins] Add manual plugin scan for local development"
```

---

### Task 33: Plugin Config UI

**Files:**
- Create: `app/components/plugins/PluginConfigDialog.tsx`

**Step 1: Build dynamic config form**

Renders form fields based on the plugin's `configSchema`:
- `string` → text input (password input if `secret: true`)
- `boolean` → checkbox
- `number` → number input with min/max
- `select` → dropdown
- `textarea` → textarea

Validates `required`, `min`, `max` on save.

**Step 2: Wire into installed plugins page**

"Configure" button opens the dialog.

**Step 3: Commit**

```bash
git add app/components/plugins/PluginConfigDialog.tsx
git commit -m "[Plugins] Add dynamic plugin configuration UI"
```

---

### Task 34: Job Logs — Plugin Filter

**Files:**
- Modify: `app/components/pages/JobLogs.tsx` (or equivalent) — add plugin filter dropdown
- Modify: `pkg/joblogs/service.go` — add plugin filter parameter

**Step 1: Add filter to backend**

Add an optional `plugin` query parameter to the job logs list endpoint that filters by the structured `plugin` field in log data.

**Step 2: Add filter to frontend**

Add a dropdown in the job logs UI to filter by plugin scope/id.

**Step 3: Commit**

```bash
git add app/components/pages/JobLogs.tsx pkg/joblogs/service.go
git commit -m "[Plugins] Add plugin filter to job logs UI"
```

---

### Task 35: Final Integration Testing

**Files:**
- Create: `pkg/plugins/integration_test.go`

**Step 1: Write integration test**

End-to-end test that:
1. Creates a test plugin with all hook types
2. Installs it via the API
3. Runs a scan and verifies converter, parser, and enricher ran
4. Downloads in plugin format and verifies output generator ran
5. Uninstalls and verifies cleanup

**Step 2: Run test**

Run: `make test`
Expected: All pass

**Step 3: Commit**

```bash
git add pkg/plugins/integration_test.go
git commit -m "[Plugins] Add integration test for full plugin lifecycle"
```

---

### Task 36: Run Full Check Suite

**Step 1: Run all checks**

Run: `make check`
Expected: All tests pass, no lint errors, no type errors.

**Step 2: Verify app starts cleanly**

Run: `make start`
Expected: App starts, plugin manager logs "loaded 0 plugins" (or similar), no errors.

---

## Key Reference Files

| Purpose | File |
|---------|------|
| Design doc | `docs/plans/2026-01-22-plugin-system-design.md` |
| Migration pattern | `pkg/migrations/20260112000000_add_job_logs.go` |
| Model pattern | `pkg/models/book.go`, `pkg/models/file.go` |
| Service pattern | `pkg/books/service.go` |
| Handler pattern | `pkg/books/routes.go` |
| Route registration | `pkg/server/server.go` |
| Worker struct | `pkg/worker/worker.go` |
| Scan pipeline | `pkg/worker/scan.go`, `pkg/worker/scan_unified.go` |
| File extensions | `pkg/worker/scan.go:22` (`extensionsToScan`) |
| Parser switch | `pkg/worker/scan_unified.go:2068` (`parseFileMetadata`) |
| Download cache | `pkg/downloadcache/cache.go` |
| File generators | `pkg/filegen/generator.go` |
| Data sources | `pkg/models/data-source.go` |
| Config | `pkg/config/config.go` |
| Frontend queries | `app/hooks/queries/` |
| Frontend pages | `app/components/pages/` |

## Dependencies to Add

Add to `go.mod`:
```
github.com/dop251/goja  // JS interpreter
```

All other dependencies (archive/zip, encoding/xml, net/http, os/exec) are stdlib.
