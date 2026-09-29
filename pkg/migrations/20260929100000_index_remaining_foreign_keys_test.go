package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const indexRemainingForeignKeysMigrationName = "20260929100000"

// These foreign keys had no index whose leading columns are the key, so the
// ON DELETE action on every library, API key, or plugin delete scanned the
// child table. The migration adds one index per key, and a lookup of children
// by parent searches it instead of scanning. Rolling back drops them.
func TestIndexRemainingForeignKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)

	// Bring the schema up to the migration just before this one.
	migrator := migrateToBefore(ctx, t, db, indexRemainingForeignKeysMigrationName)

	indexes := []struct {
		name, table, where, search string
	}{
		{"ix_api_key_short_urls_api_key_id", "api_key_short_urls", "api_key_id = 'k'", "(api_key_id=?)"},
		{"ix_person_aliases_library_id", "person_aliases", "library_id = 1", "(library_id=?)"},
		{"ix_series_aliases_library_id", "series_aliases", "library_id = 1", "(library_id=?)"},
		{"ix_genre_aliases_library_id", "genre_aliases", "library_id = 1", "(library_id=?)"},
		{"ix_tag_aliases_library_id", "tag_aliases", "library_id = 1", "(library_id=?)"},
		{"ix_publisher_aliases_library_id", "publisher_aliases", "library_id = 1", "(library_id=?)"},
		{"ix_library_plugin_field_settings_scope_plugin_id", "library_plugin_field_settings", "scope = 's' AND plugin_id = 'p'", "(scope=? AND plugin_id=?)"},
		{"ix_library_plugin_hook_configs_scope_plugin_id", "library_plugin_hook_configs", "scope = 's' AND plugin_id = 'p'", "(scope=? AND plugin_id=?)"},
		{"ix_plugin_hook_configs_scope_plugin_id", "plugin_hook_configs", "scope = 's' AND plugin_id = 'p'", "(scope=? AND plugin_id=?)"},
	}

	assertIndexed := func(want bool) {
		t.Helper()
		for _, ix := range indexes {
			assert.Equal(t, want, indexExists(ctx, t, db, ix.name), "index %s exists", ix.name)
			search := "SEARCH " + ix.table + " USING INDEX " + ix.name + " " + ix.search
			query := "SELECT * FROM " + ix.table + " WHERE " + ix.where
			if want {
				assert.Contains(t, queryPlan(ctx, t, db, query), search)
			} else {
				assert.NotContains(t, queryPlan(ctx, t, db, query), search)
			}
		}
	}

	assertIndexed(false)

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)
	assertIndexed(true)

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	assertIndexed(false)

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	assertIndexed(true)
}
