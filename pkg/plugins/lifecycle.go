package plugins

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// A plugin lives in four stores: its plugins row, its directory, its
// registered runtime, and the tables derived from its manifest (identifier
// types and hook order, global and per library). Every transition that
// changes one of them (install, update, reload, enable, disable, uninstall,
// startup, and the update check's write) holds the plugin's lock from
// lockPlugin, re-reads the row inside it, and writes only the columns it
// owns. Transitions that load a version finish through applyVersion, so
// they all reconcile the derived tables the same way.

// Transition failures the handlers render as a 422 invalid_state.
var (
	// ErrAlreadyInstalled marks an install of a scope and id that already
	// has a row.
	ErrAlreadyInstalled = errors.New("plugin is already installed")
	// ErrDirectoryExists marks an install whose directory already exists
	// without a row, such as a local plugin that was never scanned.
	ErrDirectoryExists = errors.New("plugin directory already exists")
	// ErrNotActive marks a reload of a plugin that is not Active.
	ErrNotActive = errors.New("plugin must be active to reload")
)

// lifecycleColumns are the plugins columns a lifecycle transition owns.
// Settings (auto_update, confidence_threshold) belong to PATCH and are
// never written here.
var lifecycleColumns = []string{"name", "version", "description", "homepage", "status", "load_error", "update_available_version", "updated_at"}

// maxPathSegmentLen caps a plugin scope or id well below the 255-byte file
// name limit, so a long id fails validation before any download instead of
// failing the rename after it.
const maxPathSegmentLen = 128

// validPathSegment reports whether s can name a plugin scope or id: it must
// be one directory name inside the plugin directory. It rejects the empty
// string, anything longer than maxPathSegmentLen bytes, a leading dot (which
// covers "." and "..", and keeps ids clear of each scope's .staging and
// .trash), path separators, and control characters (bytes below 0x20, which
// include NUL, and 0x7F).
func validPathSegment(s string) bool {
	if s == "" || len(s) > maxPathSegmentLen || strings.HasPrefix(s, ".") || strings.ContainsAny(s, "/\\") {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7F {
			return false
		}
	}
	return true
}

// validatePluginRef rejects a scope or id that is not a safe path segment
// before a route uses it to build a path. It is a 422, not a 404: the
// request is a rejected traversal attempt, not a missing row.
func validatePluginRef(scope, id string) error {
	if !validPathSegment(scope) || !validPathSegment(id) {
		return errcodes.ValidationError("Invalid scope or plugin ID")
	}
	return nil
}

// lockPlugin takes the lifecycle lock of one plugin and returns its unlock.
func (m *Manager) lockPlugin(scope, id string) func() {
	v, _ := m.locks.LoadOrStore(pluginKey(scope, id), &sync.Mutex{})
	mu := v.(*sync.Mutex)
	if !mu.TryLock() {
		if m.onLockWait != nil {
			m.onLockWait(pluginKey(scope, id))
		}
		mu.Lock()
	}
	return mu.Unlock
}

// liveDir is the installed directory of a plugin.
func (m *Manager) liveDir(scope, id string) string {
	return filepath.Join(m.pluginDir, scope, id)
}

// loadRuntime loads the plugin in dir as scope/id and injects the host
// APIs. It changes no store, so a caller can load a staged version and
// decide afterwards. A failure caused by the plugin, including a manifest
// that names another id, is a *LoadError; failing to inject the host APIs
// is a server fault.
func (m *Manager) loadRuntime(dir, scope, id string) (*Runtime, error) {
	rt, err := LoadPlugin(dir, scope, id)
	if err != nil {
		return nil, &LoadError{Err: errors.Wrapf(err, "failed to load plugin %s/%s", scope, id)}
	}
	if rt.manifest.ID != id {
		return nil, &LoadError{Err: errors.Errorf("failed to load plugin %s/%s: manifest id %q does not match", scope, id, rt.manifest.ID)}
	}
	rt.dataDir = filepath.Join(m.pluginDataDir, scope, id)
	if err := InjectHostAPIs(rt, m.service); err != nil {
		return nil, errors.Wrapf(err, "failed to inject host APIs for %s/%s", scope, id)
	}
	return rt, nil
}

// setRuntime registers rt as the plugin's runtime, or unregisters it when rt
// is nil. It takes the write lock of the runtime it replaces, so hooks in
// progress on it finish first.
func (m *Manager) setRuntime(scope, id string, rt *Runtime) {
	key := pluginKey(scope, id)
	m.mu.RLock()
	old := m.plugins[key]
	m.mu.RUnlock()
	if old != nil && old != rt {
		old.mu.Lock()
		defer old.mu.Unlock()
	}
	m.mu.Lock()
	if rt == nil {
		delete(m.plugins, key)
	} else {
		m.plugins[key] = rt
	}
	m.mu.Unlock()
}

// applyVersion stores a loaded version of a plugin. In one transaction it
// inserts the row (insert) or writes its lifecycle columns, and when rt is
// not nil it copies the manifest's name, version, description, and homepage
// onto the row, replaces the identifier types (a manifest without the key
// has none), and reconciles the hook order with rt's hook types. After the
// commit it registers rt if the row is Active and unregisters the plugin
// otherwise. A nil rt is a version that failed to load: only the row is
// written. The caller holds the plugin's lock.
func (m *Manager) applyVersion(ctx context.Context, row *models.Plugin, rt *Runtime, insert bool) error {
	if rt != nil {
		row.Name = rt.manifest.Name
		row.Version = rt.manifest.Version
		row.Description = optionalString(rt.manifest.Description)
		row.Homepage = optionalString(rt.manifest.Homepage)
	}

	err := m.service.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if insert {
			if _, err := tx.NewInsert().Model(row).Exec(ctx); err != nil {
				return errors.WithStack(err)
			}
		} else {
			res, err := tx.NewUpdate().Model(row).Column(lifecycleColumns...).WherePK().Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
			if n, err := res.RowsAffected(); err == nil && n == 0 {
				return errors.WithStack(sql.ErrNoRows)
			}
		}
		if rt == nil {
			return nil
		}
		if err := replaceIdentifierTypes(ctx, tx, row.Scope, row.ID, rt.manifest.Capabilities.IdentifierTypes); err != nil {
			return errors.Wrapf(err, "failed to store identifier types for %s/%s", row.Scope, row.ID)
		}
		if err := reconcileHookOrder(ctx, tx, row.Scope, row.ID, rt.HookTypes()); err != nil {
			return errors.Wrapf(err, "failed to reconcile the hook order for %s/%s", row.Scope, row.ID)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if rt != nil && row.Status == models.PluginStatusActive {
		m.setRuntime(row.Scope, row.ID, rt)
	} else {
		m.setRuntime(row.Scope, row.ID, nil)
	}
	return nil
}

// recordLoadError marks the row Malfunctioned, or Not Supported for a host
// version the plugin does not support, stores the error, and unregisters
// the plugin. The caller holds the plugin's lock.
func (m *Manager) recordLoadError(ctx context.Context, row *models.Plugin, loadErr error) error {
	markLoadFailed(row, loadErr)
	return m.applyVersion(ctx, row, nil, false)
}

// markLoadFailed sets the row's status and load error for a load failure the
// plugin caused.
func markLoadFailed(row *models.Plugin, loadErr error) {
	msg := loadErr.Error()
	row.LoadError = &msg
	if isVersionIncompatible(loadErr) {
		row.Status = models.PluginStatusNotSupported
	} else {
		row.Status = models.PluginStatusMalfunctioned
	}
}

// optionalString returns nil for an empty string.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// replaceIdentifierTypes replaces every identifier type of a plugin with
// types.
func replaceIdentifierTypes(ctx context.Context, db bun.IDB, scope, pluginID string, types []IdentifierTypeCap) error {
	_, err := db.NewDelete().Model((*models.PluginIdentifierType)(nil)).
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
		idTypes[i] = &models.PluginIdentifierType{
			ID:          t.ID,
			Scope:       scope,
			PluginID:    pluginID,
			Name:        t.Name,
			URLTemplate: optionalString(t.URLTemplate),
			Pattern:     optionalString(t.Pattern),
		}
	}
	_, err = db.NewInsert().Model(&idTypes).Exec(ctx)
	return errors.WithStack(err)
}

// reconcileHookOrder makes a plugin's hook order rows match hookTypes. It
// deletes the plugin's rows for hook types it no longer provides, globally
// and in every library. For each hook type it provides, it appends the
// plugin to the end of the global order if missing, and to the end of
// every customized library order that lacks it, with the mode of its global
// row. Rows that already exist keep their position and mode.
func reconcileHookOrder(ctx context.Context, db bun.IDB, scope, pluginID string, hookTypes []string) error {
	for _, model := range []any{(*models.PluginHookConfig)(nil), (*models.LibraryPluginHookConfig)(nil)} {
		q := db.NewDelete().Model(model).
			Where("scope = ?", scope).
			Where("plugin_id = ?", pluginID)
		if len(hookTypes) > 0 {
			q = q.Where("hook_type NOT IN (?)", bun.List(hookTypes))
		}
		if _, err := q.Exec(ctx); err != nil {
			return errors.WithStack(err)
		}
	}

	for _, hookType := range hookTypes {
		global := new(models.PluginHookConfig)
		err := db.NewSelect().Model(global).
			Where("hook_type = ?", hookType).
			Where("scope = ?", scope).
			Where("plugin_id = ?", pluginID).
			Scan(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			var maxPos int
			if err := db.NewSelect().Model((*models.PluginHookConfig)(nil)).
				ColumnExpr("COALESCE(MAX(position), -1)").
				Where("hook_type = ?", hookType).
				Scan(ctx, &maxPos); err != nil {
				return errors.WithStack(err)
			}
			global = &models.PluginHookConfig{HookType: hookType, Scope: scope, PluginID: pluginID, Position: maxPos + 1, Mode: models.PluginModeEnabled}
			if _, err := db.NewInsert().Model(global).Exec(ctx); err != nil {
				return errors.WithStack(err)
			}
		} else if err != nil {
			return errors.WithStack(err)
		}

		// Every library that customized this hook type and does not list the
		// plugin, with the next free position in its order.
		var missing []struct {
			LibraryID int `bun:"library_id"`
			Next      int `bun:"next"`
		}
		if err := db.NewSelect().
			TableExpr("library_plugin_customizations AS lpc").
			ColumnExpr("lpc.library_id").
			ColumnExpr("(SELECT COALESCE(MAX(position), -1) + 1 FROM library_plugin_hook_configs WHERE library_id = lpc.library_id AND hook_type = lpc.hook_type) AS next").
			Where("lpc.hook_type = ?", hookType).
			Where("NOT EXISTS (SELECT 1 FROM library_plugin_hook_configs WHERE library_id = lpc.library_id AND hook_type = lpc.hook_type AND scope = ? AND plugin_id = ?)", scope, pluginID).
			Scan(ctx, &missing); err != nil {
			return errors.WithStack(err)
		}
		for _, lib := range missing {
			entry := &models.LibraryPluginHookConfig{
				LibraryID: lib.LibraryID, HookType: hookType, Scope: scope, PluginID: pluginID,
				Position: lib.Next, Mode: global.Mode,
			}
			if _, err := db.NewInsert().Model(entry).Exec(ctx); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}

// enableLocked loads a plugin from its directory and makes it Active. A
// load failure the plugin caused is recorded on the row and returned as
// loadErr; err is a server fault, which leaves the row alone. The caller
// holds the plugin's lock.
func (m *Manager) enableLocked(ctx context.Context, row *models.Plugin) (loadErr error, err error) {
	rt, err := m.loadRuntime(m.liveDir(row.Scope, row.ID), row.Scope, row.ID)
	if err != nil {
		if !asLoadError(err) {
			return nil, err
		}
		if recordErr := m.recordLoadError(ctx, row, err); recordErr != nil {
			return nil, recordErr
		}
		return err, nil
	}
	row.Status = models.PluginStatusActive
	row.LoadError = nil
	return nil, m.applyVersion(ctx, row, rt, false)
}

// disableLocked makes a plugin Disabled and unregisters it. The caller
// holds the plugin's lock.
func (m *Manager) disableLocked(ctx context.Context, row *models.Plugin) error {
	row.Status = models.PluginStatusDisabled
	row.LoadError = nil
	return m.applyVersion(ctx, row, nil, false)
}

// installStaged installs a staged package as the new plugin row. It moves
// the package into place, loads it, and stores the row with the version's
// derived rows. A version that fails to load is still installed, as
// Malfunctioned or Not Supported with its error on the row. Any other
// failure removes what it installed. The package is consumed either way.
func (m *Manager) installStaged(ctx context.Context, row *models.Plugin, pkg *stagedPackage) error {
	defer pkg.remove()
	unlock := m.lockPlugin(row.Scope, row.ID)
	defer unlock()

	if _, err := m.service.RetrievePlugin(ctx, row.Scope, row.ID); err == nil {
		return ErrAlreadyInstalled
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	// A directory without a row is not ours to replace: it may be a local
	// plugin that has not been scanned yet.
	if _, err := os.Lstat(m.liveDir(row.Scope, row.ID)); err == nil {
		return ErrDirectoryExists
	} else if !os.IsNotExist(err) {
		return errors.Wrap(err, "failed to check the plugin directory")
	}

	swap, err := swapIn(m.pluginDir, row.Scope, row.ID, pkg.dir)
	if err != nil {
		return err
	}

	rt, err := m.loadRuntime(swap.live, row.Scope, row.ID)
	if err != nil && !asLoadError(err) {
		return withRollback(err, swap)
	}
	if err != nil {
		markLoadFailed(row, err)
		row.Name, row.Version = pkg.manifest.Name, pkg.manifest.Version
		row.Description, row.Homepage = optionalString(pkg.manifest.Description), optionalString(pkg.manifest.Homepage)
	} else {
		row.Status = models.PluginStatusActive
		row.LoadError = nil
	}

	if err := m.applyVersion(ctx, row, rt, true); err != nil {
		return withRollback(err, swap)
	}
	swap.commit()
	return nil
}

// updateStaged replaces an installed plugin with a staged package. The new
// version is loaded from the staging directory first; if it does not load
// (a *LoadError) or anything later fails, the installed directory, runtime,
// and row are left as they were. On success the row keeps its status,
// except that a Malfunctioned or Not Supported plugin becomes Active since
// its new version loads, and a Disabled plugin stays unloaded. The package
// is consumed either way.
func (m *Manager) updateStaged(ctx context.Context, scope, id string, pkg *stagedPackage) (*models.Plugin, error) {
	defer pkg.remove()

	rt, err := m.loadRuntime(pkg.dir, scope, id)
	if err != nil {
		return nil, err
	}

	unlock := m.lockPlugin(scope, id)
	defer unlock()

	row, err := m.service.RetrievePlugin(ctx, scope, id)
	if err != nil {
		return nil, err
	}

	if err := carryOverIcon(pkg.dir, m.liveDir(scope, id)); err != nil {
		return nil, err
	}
	swap, err := swapIn(m.pluginDir, scope, id, pkg.dir)
	if err != nil {
		return nil, err
	}

	if row.Status == models.PluginStatusMalfunctioned || row.Status == models.PluginStatusNotSupported {
		row.Status = models.PluginStatusActive
	}
	row.LoadError = nil
	row.UpdateAvailableVersion = nil
	now := time.Now()
	row.UpdatedAt = &now
	if err := m.applyVersion(ctx, row, rt, false); err != nil {
		return nil, withRollback(err, swap)
	}
	swap.commit()
	return row, nil
}

// reload loads an Active plugin again from its directory. On success the
// row takes the new version's manifest fields and the new runtime replaces
// the old one. A load failure the plugin caused is stored on the row and
// returned as loadErr, and the old runtime keeps running.
func (m *Manager) reload(ctx context.Context, scope, id string) (row *models.Plugin, loadErr error, err error) {
	unlock := m.lockPlugin(scope, id)
	defer unlock()

	row, err = m.service.RetrievePlugin(ctx, scope, id)
	if err != nil {
		return nil, nil, err
	}
	if row.Status != models.PluginStatusActive {
		return nil, nil, ErrNotActive
	}

	now := time.Now()
	row.UpdatedAt = &now
	rt, err := m.loadRuntime(m.liveDir(scope, id), scope, id)
	if err != nil {
		if !asLoadError(err) {
			return nil, nil, err
		}
		msg := err.Error()
		row.LoadError = &msg
		if writeErr := m.service.UpdatePluginColumns(ctx, row, "load_error", "updated_at"); writeErr != nil {
			return nil, nil, writeErr
		}
		return row, err, nil
	}
	row.LoadError = nil
	if err := m.applyVersion(ctx, row, rt, false); err != nil {
		return nil, nil, err
	}
	return row, nil, nil
}

// uninstall removes a plugin: it runs the plugin's onUninstalling hook,
// deletes the row (its child rows cascade), unregisters the runtime once
// hooks in progress finish, removes the directory, and with deleteData
// removes the persistent data directory. The row goes first, so a failure
// before it leaves the plugin whole.
func (m *Manager) uninstall(ctx context.Context, scope, id string, deleteData bool) error {
	unlock := m.lockPlugin(scope, id)
	defer unlock()

	if rt := m.GetRuntime(scope, id); rt != nil {
		m.RunOnUninstalling(rt)
	}
	if err := m.service.UninstallPlugin(ctx, scope, id); err != nil {
		return err
	}
	m.setRuntime(scope, id, nil)
	if err := os.RemoveAll(m.liveDir(scope, id)); err != nil {
		return errors.Wrap(err, "failed to remove the plugin directory")
	}
	if deleteData {
		m.DeletePluginData(scope, id)
	}
	return nil
}

// carryOverIcon copies the installed icon.png into a staged package that
// has none, so an update whose repository gives no icon, or whose icon
// download failed, keeps the plugin's icon.
func carryOverIcon(stagedDir, liveDir string) error {
	dest := filepath.Join(stagedDir, "icon.png")
	if _, err := os.Lstat(dest); err == nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(liveDir, "icon.png"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "failed to read the installed plugin icon")
	}
	return errors.Wrap(os.WriteFile(dest, data, 0600), "failed to keep the plugin icon")
}
