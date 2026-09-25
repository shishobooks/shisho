package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/migrate"
)

// PDF outline items with a null destination were stored as chapters with
// start_page -1. The migration deletes them, with their whole subtree, from
// PDF files only, and leaves every other chapter alone.
//
// It runs with foreign keys ON, as in production, and OFF, to show that the
// migration removes a deleted chapter's descendants itself rather than rely
// on ON DELETE CASCADE, which only fires when the pragma is enabled.
func TestDeleteNegativePDFChapters(t *testing.T) {
	t.Parallel()

	for _, foreignKeys := range []string{"ON", "OFF"} {
		t.Run("foreign_keys="+foreignKeys, func(t *testing.T) {
			t.Parallel()
			testDeleteNegativePDFChapters(t, foreignKeys)
		})
	}
}

func testDeleteNegativePDFChapters(t *testing.T, foreignKeys string) {
	ctx := context.Background()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	for _, query := range []string{
		"PRAGMA foreign_keys = " + foreignKeys,
		"CREATE TABLE files (id INTEGER PRIMARY KEY, file_type TEXT NOT NULL)",
		`CREATE TABLE chapters (
			id INTEGER PRIMARY KEY,
			file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
			parent_id INTEGER REFERENCES chapters(id) ON DELETE CASCADE,
			sort_order INTEGER NOT NULL,
			title TEXT NOT NULL,
			start_page INTEGER,
			start_timestamp_ms INTEGER
		)`,
		"INSERT INTO files VALUES (1, 'pdf'), (2, 'cbz'), (3, 'm4b')",
		`INSERT INTO chapters (id, file_id, parent_id, sort_order, title, start_page, start_timestamp_ms) VALUES
			(1, 1, NULL, 0, 'PDF valid', 0, NULL),
			(2, 1, NULL, 1, 'PDF null dest', -1, NULL),
			(3, 1, NULL, 2, 'PDF null dest parent', -1, NULL),
			(4, 3, NULL, 0, 'placeholder', NULL, 0),
			(5, 1, 3, 0, 'PDF child of null dest', 4, NULL),
			(6, 1, 5, 0, 'PDF grandchild of null dest', 5, NULL),
			(7, 1, NULL, 3, 'PDF valid parent', 2, NULL),
			(8, 1, 7, 0, 'PDF null dest child', -1, NULL),
			(9, 1, 7, 1, 'PDF valid child', 3, NULL),
			(10, 1, NULL, 4, 'PDF no page', NULL, NULL),
			(11, 2, NULL, 0, 'CBZ negative', -1, NULL),
			(12, 3, NULL, 1, 'M4B chapter', NULL, 1000)`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err)
	}

	migrationSet := migrate.NewMigrations()
	for _, migration := range Migrations.Sorted() {
		if migration.Name == "20260925000000" {
			migrationSet.Add(migration)
		}
	}
	require.Len(t, migrationSet.Sorted(), 1)
	migrator := newMigrator(db, migrationSet)
	require.NoError(t, migrator.Init(ctx))
	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)

	var remaining []int
	require.NoError(t, db.NewRaw("SELECT id FROM chapters ORDER BY id").Scan(ctx, &remaining))
	assert.Equal(t, []int{1, 4, 7, 9, 10, 11, 12}, remaining,
		"negative PDF chapters and their descendants are deleted; other files and chapters are untouched")

	// The data migration is irreversible, so rolling back succeeds and
	// changes nothing.
	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	var afterRollback []int
	require.NoError(t, db.NewRaw("SELECT id FROM chapters ORDER BY id").Scan(ctx, &afterRollback))
	assert.Equal(t, remaining, afterRollback)
}
