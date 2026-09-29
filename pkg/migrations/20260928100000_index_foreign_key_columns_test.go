package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const indexForeignKeyColumnsMigrationName = "20260928100000"

// Each foreign key column below had no index whose leading column is the
// foreign key, so looking up children by parent (alias subqueries during
// reindexing, ON DELETE actions on the parent) scanned the whole table. The
// migration adds one plain index per column and rolling back drops them.
func TestIndexForeignKeyColumns(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)

	// Bring the schema up to the migration just before this one.
	migrator := migrateToBefore(ctx, t, db, indexForeignKeyColumnsMigrationName)

	columns := []struct{ table, column string }{
		{"person_aliases", "person_id"},
		{"series_aliases", "series_id"},
		{"genre_aliases", "genre_id"},
		{"tag_aliases", "tag_id"},
		{"publisher_aliases", "publisher_id"},
		{"publishers", "parent_id"},
		{"jobs", "library_id"},
		{"list_books", "added_by_user_id"},
		{"list_shares", "shared_by_user_id"},
		{"user_library_settings", "library_id"},
	}

	assertIndexed := func() {
		t.Helper()
		for _, c := range columns {
			name := "ix_" + c.table + "_" + c.column
			assert.True(t, indexExists(ctx, t, db, name), "index %s should exist", name)
			// A lookup of children by parent is an index search on the new
			// index, not a scan.
			assert.Contains(t,
				queryPlan(ctx, t, db, "SELECT * FROM "+c.table+" WHERE "+c.column+" = 1"),
				"SEARCH "+c.table+" USING INDEX "+name+" ("+c.column+"=?)",
			)
		}
	}

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)
	assertIndexed()

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	for _, c := range columns {
		name := "ix_" + c.table + "_" + c.column
		assert.False(t, indexExists(ctx, t, db, name), "index %s should be dropped on rollback", name)
	}

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	assertIndexed()
}
