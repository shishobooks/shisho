package plugins

import (
	"context"
	"database/sql"

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
	_, err := s.db.NewInsert().Model(plugin).Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ListPlugins returns all installed plugins.
func (s *Service) ListPlugins(ctx context.Context) ([]*models.Plugin, error) {
	var plugins []*models.Plugin
	err := s.db.NewSelect().Model(&plugins).OrderExpr("scope ASC, id ASC").Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return plugins, nil
}

// RetrievePlugin returns a single plugin by scope and id.
func (s *Service) RetrievePlugin(ctx context.Context, scope, id string) (*models.Plugin, error) {
	plugin := new(models.Plugin)
	err := s.db.NewSelect().Model(plugin).
		Where("scope = ?", scope).
		Where("id = ?", id).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.WithStack(err)
		}
		return nil, errors.WithStack(err)
	}
	return plugin, nil
}

// UpdatePlugin updates an existing plugin record.
func (s *Service) UpdatePlugin(ctx context.Context, plugin *models.Plugin) error {
	_, err := s.db.NewUpdate().Model(plugin).
		WherePK().
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// UninstallPlugin removes a plugin and its related data (configs, orders cascade via FK).
func (s *Service) UninstallPlugin(ctx context.Context, scope, id string) error {
	_, err := s.db.NewDelete().Model((*models.Plugin)(nil)).
		Where("scope = ?", scope).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// GetConfig returns the configuration values for a plugin. If raw is false,
// secret values are masked with "***". The schema is used to determine which
// fields are secrets.
func (s *Service) GetConfig(ctx context.Context, scope, pluginID string, schema ConfigSchema, raw bool) (map[string]interface{}, error) {
	var configs []*models.PluginConfig
	err := s.db.NewSelect().Model(&configs).
		Where("scope = ?", scope).
		Where("plugin_id = ?", pluginID).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	result := make(map[string]interface{})
	for _, cfg := range configs {
		if cfg.Value == nil {
			result[cfg.Key] = nil
			continue
		}
		if !raw {
			if field, ok := schema[cfg.Key]; ok && field.Secret {
				result[cfg.Key] = "***"
				continue
			}
		}
		result[cfg.Key] = *cfg.Value
	}
	return result, nil
}

// SetConfig upserts a single config key-value pair for a plugin.
func (s *Service) SetConfig(ctx context.Context, scope, pluginID, key, value string) error {
	cfg := &models.PluginConfig{
		Scope:    scope,
		PluginID: pluginID,
		Key:      key,
		Value:    &value,
	}
	_, err := s.db.NewInsert().Model(cfg).
		On("CONFLICT (scope, plugin_id, key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// GetAllConfigRaw returns all raw config key-value pairs for a plugin.
func (s *Service) GetAllConfigRaw(ctx context.Context, scope, pluginID string) (map[string]*string, error) {
	var configs []*models.PluginConfig
	err := s.db.NewSelect().Model(&configs).
		Where("scope = ?", scope).
		Where("plugin_id = ?", pluginID).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	result := make(map[string]*string, len(configs))
	for _, cfg := range configs {
		result[cfg.Key] = cfg.Value
	}
	return result, nil
}

// GetConfigRaw returns the raw value for a single config key.
func (s *Service) GetConfigRaw(ctx context.Context, scope, pluginID, key string) (*string, error) {
	cfg := new(models.PluginConfig)
	err := s.db.NewSelect().Model(cfg).
		Where("scope = ?", scope).
		Where("plugin_id = ?", pluginID).
		Where("key = ?", key).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	return cfg.Value, nil
}

// GetOrder returns the plugin order entries for a hook type, sorted by position.
func (s *Service) GetOrder(ctx context.Context, hookType string) ([]*models.PluginOrder, error) {
	var orders []*models.PluginOrder
	err := s.db.NewSelect().Model(&orders).
		Where("hook_type = ?", hookType).
		OrderExpr("position ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return orders, nil
}

// SetOrder replaces all order entries for a hook type in a transaction.
func (s *Service) SetOrder(ctx context.Context, hookType string, entries []models.PluginOrder) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().Model((*models.PluginOrder)(nil)).
			Where("hook_type = ?", hookType).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

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
		return nil
	})
}

// AppendToOrder appends a plugin to the end of the order for a hook type.
func (s *Service) AppendToOrder(ctx context.Context, hookType, scope, pluginID string) error {
	var maxPos int
	err := s.db.NewSelect().Model((*models.PluginOrder)(nil)).
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
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ListRepositories returns all plugin repositories, ordered by official first then by scope.
func (s *Service) ListRepositories(ctx context.Context) ([]*models.PluginRepository, error) {
	var repos []*models.PluginRepository
	err := s.db.NewSelect().Model(&repos).
		OrderExpr("is_official DESC, scope ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return repos, nil
}

// GetRepository returns a single plugin repository by scope.
func (s *Service) GetRepository(ctx context.Context, scope string) (*models.PluginRepository, error) {
	repo := new(models.PluginRepository)
	err := s.db.NewSelect().Model(repo).
		Where("scope = ?", scope).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return repo, nil
}

// AddRepository inserts a new plugin repository.
func (s *Service) AddRepository(ctx context.Context, repo *models.PluginRepository) error {
	_, err := s.db.NewInsert().Model(repo).Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// RemoveRepository removes a non-official plugin repository by scope.
func (s *Service) RemoveRepository(ctx context.Context, scope string) error {
	_, err := s.db.NewDelete().Model((*models.PluginRepository)(nil)).
		Where("scope = ?", scope).
		Where("is_official = ?", false).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// UpdateRepository updates an existing plugin repository.
func (s *Service) UpdateRepository(ctx context.Context, repo *models.PluginRepository) error {
	_, err := s.db.NewUpdate().Model(repo).
		WherePK().
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ListIdentifierTypes returns all plugin-registered identifier types.
func (s *Service) ListIdentifierTypes(ctx context.Context) ([]*models.PluginIdentifierType, error) {
	var types []*models.PluginIdentifierType
	err := s.db.NewSelect().Model(&types).OrderExpr("scope ASC, plugin_id ASC, id ASC").Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return types, nil
}

// UpsertIdentifierTypes replaces all identifier types for a plugin in a transaction.
func (s *Service) UpsertIdentifierTypes(ctx context.Context, scope, pluginID string, types []IdentifierTypeCap) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().Model((*models.PluginIdentifierType)(nil)).
			Where("scope = ?", scope).
			Where("plugin_id = ?", pluginID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		if len(types) == 0 {
			return nil
		}

		idTypes := make([]*models.PluginIdentifierType, len(types))
		for i, t := range types {
			idType := &models.PluginIdentifierType{
				ID:       t.ID,
				Scope:    scope,
				PluginID: pluginID,
				Name:     t.Name,
			}
			if t.URLTemplate != "" {
				urlTemplate := t.URLTemplate
				idType.URLTemplate = &urlTemplate
			}
			if t.Pattern != "" {
				pattern := t.Pattern
				idType.Pattern = &pattern
			}
			idTypes[i] = idType
		}

		_, err = tx.NewInsert().Model(&idTypes).Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}

// GetPlugin returns a single plugin by scope and ID, or nil if not found.
func (s *Service) GetPlugin(ctx context.Context, scope, id string) (*models.Plugin, error) {
	plugin := new(models.Plugin)
	err := s.db.NewSelect().Model(plugin).
		Where("scope = ? AND id = ?", scope, id).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	return plugin, nil
}

// IsLibraryCustomized checks if a library has customized plugin order for a hook type.
func (s *Service) IsLibraryCustomized(ctx context.Context, libraryID int, hookType string) (bool, error) {
	exists, err := s.db.NewSelect().Model((*models.LibraryPluginCustomization)(nil)).
		Where("library_id = ? AND hook_type = ?", libraryID, hookType).
		Exists(ctx)
	if err != nil {
		return false, errors.WithStack(err)
	}
	return exists, nil
}

// GetLibraryOrder returns the per-library plugin order for a hook type, sorted by position.
func (s *Service) GetLibraryOrder(ctx context.Context, libraryID int, hookType string) ([]*models.LibraryPlugin, error) {
	var entries []*models.LibraryPlugin
	err := s.db.NewSelect().Model(&entries).
		Where("library_id = ? AND hook_type = ?", libraryID, hookType).
		OrderExpr("position ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return entries, nil
}

// SetLibraryOrder replaces all per-library plugin order entries for a hook type.
// Also creates the customization record if it doesn't exist.
func (s *Service) SetLibraryOrder(ctx context.Context, libraryID int, hookType string, entries []models.LibraryPlugin) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Upsert customization record
		customization := &models.LibraryPluginCustomization{
			LibraryID: libraryID,
			HookType:  hookType,
		}
		_, err := tx.NewInsert().Model(customization).
			On("CONFLICT (library_id, hook_type) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete existing entries
		_, err = tx.NewDelete().Model((*models.LibraryPlugin)(nil)).
			Where("library_id = ? AND hook_type = ?", libraryID, hookType).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Insert new entries with positions
		for i := range entries {
			entries[i].LibraryID = libraryID
			entries[i].HookType = hookType
			entries[i].Position = i
		}
		if len(entries) > 0 {
			_, err = tx.NewInsert().Model(&entries).Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	})
}

// ResetLibraryOrder removes per-library customization for a specific hook type.
func (s *Service) ResetLibraryOrder(ctx context.Context, libraryID int, hookType string) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().Model((*models.LibraryPlugin)(nil)).
			Where("library_id = ? AND hook_type = ?", libraryID, hookType).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = tx.NewDelete().Model((*models.LibraryPluginCustomization)(nil)).
			Where("library_id = ? AND hook_type = ?", libraryID, hookType).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// ResetAllLibraryOrders removes all per-library plugin customizations for a library.
func (s *Service) ResetAllLibraryOrders(ctx context.Context, libraryID int) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().Model((*models.LibraryPlugin)(nil)).
			Where("library_id = ?", libraryID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = tx.NewDelete().Model((*models.LibraryPluginCustomization)(nil)).
			Where("library_id = ?", libraryID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// GetFieldSettings returns global field settings for a plugin.
// Returns a map of field name -> enabled status for fields that are explicitly disabled.
// Absence from the map means the field is enabled (default).
func (s *Service) GetFieldSettings(ctx context.Context, scope, pluginID string) (map[string]bool, error) {
	var settings []*models.PluginFieldSetting
	err := s.db.NewSelect().Model(&settings).
		Where("scope = ?", scope).
		Where("plugin_id = ?", pluginID).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	result := make(map[string]bool, len(settings))
	for _, setting := range settings {
		result[setting.Field] = setting.Enabled
	}
	return result, nil
}

// SetFieldSetting upserts a single global field setting.
// If enabled=true, the row is deleted (enabled is the default).
// If enabled=false, the row is upserted.
func (s *Service) SetFieldSetting(ctx context.Context, scope, pluginID, field string, enabled bool) error {
	if enabled {
		// Delete the row - enabled is the default
		_, err := s.db.NewDelete().Model((*models.PluginFieldSetting)(nil)).
			Where("scope = ?", scope).
			Where("plugin_id = ?", pluginID).
			Where("field = ?", field).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	// Upsert disabled setting
	setting := &models.PluginFieldSetting{
		Scope:    scope,
		PluginID: pluginID,
		Field:    field,
		Enabled:  false,
	}
	_, err := s.db.NewInsert().Model(setting).
		On("CONFLICT (scope, plugin_id, field) DO UPDATE").
		Set("enabled = EXCLUDED.enabled").
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// GetLibraryFieldSettings returns per-library field overrides for a plugin.
// Returns a map of field name -> enabled status for fields with explicit overrides.
// Unlike global settings, both enabled and disabled overrides are stored.
func (s *Service) GetLibraryFieldSettings(ctx context.Context, libraryID int, scope, pluginID string) (map[string]bool, error) {
	var settings []*models.LibraryPluginFieldSetting
	err := s.db.NewSelect().Model(&settings).
		Where("library_id = ?", libraryID).
		Where("scope = ?", scope).
		Where("plugin_id = ?", pluginID).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	result := make(map[string]bool, len(settings))
	for _, setting := range settings {
		result[setting.Field] = setting.Enabled
	}
	return result, nil
}

// SetLibraryFieldSetting upserts a per-library field override.
// Both enabled and disabled overrides are stored to allow library-specific overrides
// in either direction.
func (s *Service) SetLibraryFieldSetting(ctx context.Context, libraryID int, scope, pluginID, field string, enabled bool) error {
	setting := &models.LibraryPluginFieldSetting{
		LibraryID: libraryID,
		Scope:     scope,
		PluginID:  pluginID,
		Field:     field,
		Enabled:   enabled,
	}
	_, err := s.db.NewInsert().Model(setting).
		On("CONFLICT (library_id, scope, plugin_id, field) DO UPDATE").
		Set("enabled = EXCLUDED.enabled").
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ResetLibraryFieldSettings removes all per-library field overrides for a plugin.
func (s *Service) ResetLibraryFieldSettings(ctx context.Context, libraryID int, scope, pluginID string) error {
	_, err := s.db.NewDelete().Model((*models.LibraryPluginFieldSetting)(nil)).
		Where("library_id = ?", libraryID).
		Where("scope = ?", scope).
		Where("plugin_id = ?", pluginID).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// GetEffectiveFieldSettings returns the effective field settings for a library,
// merging global settings with library-specific overrides.
// Returns a map of field name -> enabled status for the specified declared fields.
// Priority: library override > global setting > default (enabled).
func (s *Service) GetEffectiveFieldSettings(ctx context.Context, libraryID int, scope, pluginID string, declaredFields []string) (map[string]bool, error) {
	if len(declaredFields) == 0 {
		return make(map[string]bool), nil
	}

	// Get global settings
	globalSettings, err := s.GetFieldSettings(ctx, scope, pluginID)
	if err != nil {
		return nil, err
	}

	// Get library overrides
	libraryOverrides, err := s.GetLibraryFieldSettings(ctx, libraryID, scope, pluginID)
	if err != nil {
		return nil, err
	}

	// Compute effective settings for declared fields
	result := make(map[string]bool, len(declaredFields))
	for _, field := range declaredFields {
		// Check library override first
		if enabled, hasOverride := libraryOverrides[field]; hasOverride {
			result[field] = enabled
			continue
		}

		// Check global setting
		if enabled, hasGlobal := globalSettings[field]; hasGlobal {
			result[field] = enabled
			continue
		}

		// Default: enabled
		result[field] = true
	}
	return result, nil
}
