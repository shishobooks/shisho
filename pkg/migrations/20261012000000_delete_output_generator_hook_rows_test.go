package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const deleteOutputGeneratorHookRowsMigrationName = "20261012000000"

// The outputGenerator hook was removed from the plugin contract, so its
// hook-order rows would name a hook type the API and frontend no longer know.
func TestDeleteOutputGeneratorHookRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, deleteOutputGeneratorHookRowsMigrationName)

	for _, query := range []string{
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'One', 'book')`,
		`INSERT INTO plugins (scope, id, name, version) VALUES ('s', 'p', 'P', '1.0.0')`,
		`INSERT INTO plugin_hook_configs (hook_type, scope, plugin_id, position)
			VALUES ('outputGenerator', 's', 'p', 0), ('fileParser', 's', 'p', 0)`,
		`INSERT INTO library_plugin_customizations (library_id, hook_type)
			VALUES (1, 'outputGenerator'), (1, 'metadataEnricher')`,
		`INSERT INTO library_plugin_hook_configs (library_id, hook_type, scope, plugin_id, position)
			VALUES (1, 'outputGenerator', 's', 'p', 0), (1, 'inputConverter', 's', 'p', 0)`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}

	hookTypes := func(table string) []string {
		var types []string
		require.NoError(t, db.NewRaw("SELECT hook_type FROM "+table+" ORDER BY hook_type").Scan(ctx, &types))
		return types
	}

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"fileParser"}, hookTypes("plugin_hook_configs"))
	assert.Equal(t, []string{"metadataEnricher"}, hookTypes("library_plugin_customizations"))
	assert.Equal(t, []string{"inputConverter"}, hookTypes("library_plugin_hook_configs"))

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
}
