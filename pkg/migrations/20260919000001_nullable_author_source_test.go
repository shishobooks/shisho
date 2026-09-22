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

func TestNullableAuthorSourcePreservesDataAndIndexes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, query := range []string{
		"PRAGMA foreign_keys = ON",
		"CREATE TABLE books (id INTEGER PRIMARY KEY, title TEXT NOT NULL, author_source TEXT NOT NULL)",
		"CREATE UNIQUE INDEX books_title_unique ON books(title)",
		"CREATE TABLE files (id INTEGER PRIMARY KEY, book_id INTEGER REFERENCES books(id) ON DELETE CASCADE)",
		"INSERT INTO books VALUES (1, 'Manual Book', 'manual'), (2, 'Plugin Book', 'plugin:test/enricher')",
		"INSERT INTO files VALUES (1, 1)",
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err)
	}
	migrationSet := migrate.NewMigrations()
	for _, migration := range Migrations.Sorted() {
		if migration.Name == "20260919000001" {
			migrationSet.Add(migration)
		}
	}
	require.Len(t, migrationSet.Sorted(), 1)
	migrator := newMigrator(db, migrationSet)
	require.NoError(t, migrator.Init(ctx))
	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	var sources []string
	require.NoError(t, db.NewRaw("SELECT author_source FROM books ORDER BY id").Scan(ctx, &sources))
	assert.Equal(t, []string{"manual", "plugin:test/enricher"}, sources)
	var fileCount int
	require.NoError(t, db.NewRaw("SELECT COUNT(*) FROM files").Scan(ctx, &fileCount))
	assert.Equal(t, 1, fileCount)
	_, err = db.ExecContext(ctx, "UPDATE books SET author_source = NULL WHERE id = 1")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO books (id, title) VALUES (3, 'Manual Book')")
	require.ErrorContains(t, err, "UNIQUE")
	_, err = db.ExecContext(ctx, "INSERT INTO files VALUES (2, 99)")
	require.ErrorContains(t, err, "FOREIGN KEY")
	// Rollback deliberately retains the nullable column and cleared state.
	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	var cleared *string
	require.NoError(t, db.NewRaw("SELECT author_source FROM books WHERE id = 1").Scan(ctx, &cleared))
	assert.Nil(t, cleared)
}
