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

const keyFTSRowsMigrationName = "20260927153000"

// A database indexed before this migration has FTS rows with auto-assigned
// rowids. The migration rebuilds all six FTS tables from their source tables
// so each row's rowid equals its entity id, which lets the search service
// delete one entity's row with a rowid lookup. The rebuild also drops rows
// for entities that no longer exist and duplicate rows for one entity.
func TestKeyFTSRowsByEntityID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	// Bring the schema up to the migration just before this one.
	previous := migrate.NewMigrations()
	target := migrate.NewMigrations()
	for _, migration := range Migrations.Sorted() {
		switch {
		case migration.Name < keyFTSRowsMigrationName:
			previous.Add(migration)
		case migration.Name == keyFTSRowsMigrationName:
			target.Add(migration)
		}
	}
	require.Len(t, target.Sorted(), 1, "migration %s should be registered", keyFTSRowsMigrationName)
	previousMigrator := newMigrator(db, previous)
	require.NoError(t, previousMigrator.Init(ctx))
	_, err = previousMigrator.Migrate(ctx)
	require.NoError(t, err)

	// Entity ids are deliberately out of step with the order the old FTS rows
	// were inserted in, so their auto-assigned rowids do not match.
	for _, query := range []string{
		"INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'Library', 'book')",
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source, subtitle) VALUES
			(20, 1, '/lib/carrie', 'Carrie', 'file', 'Carrie', 'file', 'A Novel'),
			(30, 1, '/lib/tower', 'The Gunslinger', 'file', 'Gunslinger, The', 'file', NULL)`,
		`INSERT INTO persons (id, library_id, name, sort_name, sort_name_source) VALUES
			(50, 1, 'Stephen King', 'King, Stephen', 'file'),
			(60, 1, 'Frank Muller', 'Muller, Frank', 'file')`,
		"INSERT INTO person_aliases (person_id, name, library_id) VALUES (50, 'Richard Bachman', 1)",
		"INSERT INTO authors (book_id, person_id, sort_order) VALUES (20, 50, 0), (30, 50, 0)",
		"INSERT INTO files (id, library_id, book_id, filepath, file_type) VALUES (80, 1, 30, '/lib/tower/tower.m4b', 'm4b')",
		"INSERT INTO narrators (file_id, person_id, sort_order) VALUES (80, 60, 0)",
		`INSERT INTO series (id, library_id, name, name_source, sort_name, sort_name_source, description) VALUES
			(70, 1, 'The Dark Tower', 'file', 'Dark Tower, The', 'file', 'Roland')`,
		"INSERT INTO series_aliases (series_id, name, library_id) VALUES (70, 'Mid-World', 1)",
		"INSERT INTO book_series (book_id, series_id, sort_order) VALUES (30, 70, 0)",
		"INSERT INTO genres (id, library_id, name) VALUES (40, 1, 'Horror')",
		"INSERT INTO genre_aliases (genre_id, name, library_id) VALUES (40, 'Scary', 1)",
		"INSERT INTO tags (id, library_id, name) VALUES (44, 1, 'Classic')",
		"INSERT INTO tag_aliases (tag_id, name, library_id) VALUES (44, 'Canon', 1)",
		"INSERT INTO publishers (id, library_id, name) VALUES (48, 1, 'Doubleday')",
		"INSERT INTO publisher_aliases (publisher_id, name, library_id) VALUES (48, 'DD', 1)",

		// FTS rows written the old way: auto rowids, stale text, a duplicate
		// row for book 20, and orphans for entities that no longer exist.
		"INSERT INTO books_fts (book_id, library_id, title) VALUES (999, 1, 'orphan')",
		"INSERT INTO books_fts (book_id, library_id, title) VALUES (30, 1, 'stale')",
		"INSERT INTO books_fts (book_id, library_id, title) VALUES (20, 1, 'stale')",
		"INSERT INTO books_fts (book_id, library_id, title) VALUES (20, 1, 'stale duplicate')",
		"INSERT INTO series_fts (series_id, library_id, name) VALUES (999, 1, 'orphan')",
		"INSERT INTO series_fts (series_id, library_id, name) VALUES (70, 1, 'stale')",
		"INSERT INTO persons_fts (person_id, library_id, name) VALUES (60, 1, 'stale')",
		"INSERT INTO persons_fts (person_id, library_id, name) VALUES (50, 1, 'stale')",
		"INSERT INTO genres_fts (genre_id, library_id, name) VALUES (40, 1, 'stale')",
		"INSERT INTO tags_fts (tag_id, library_id, name) VALUES (44, 1, 'stale')",
		"INSERT INTO publishers_fts (publisher_id, library_id, name) VALUES (999, 1, 'orphan')",
		"INSERT INTO publishers_fts (publisher_id, library_id, name) VALUES (48, 1, 'stale')",
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}

	type ftsRow struct {
		RowID    int    `bun:"rowid"`
		EntityID int    `bun:"entity_id"`
		Text     string `bun:"text"`
	}
	rowsOf := func(table, idColumn, textExpr string) []ftsRow {
		t.Helper()
		var rows []ftsRow
		require.NoError(t, db.NewRaw(
			"SELECT rowid, "+idColumn+" AS entity_id, "+textExpr+" AS text FROM "+table+" ORDER BY rowid",
		).Scan(ctx, &rows))
		return rows
	}
	matches := func(table, idColumn, query string) []int {
		t.Helper()
		var ids []int
		require.NoError(t, db.NewRaw(
			"SELECT "+idColumn+" FROM "+table+" WHERE "+table+" MATCH ? ORDER BY "+idColumn, query,
		).Scan(ctx, &ids))
		return ids
	}

	assertRebuilt := func() {
		t.Helper()
		assert.Equal(t, []ftsRow{
			{RowID: 20, EntityID: 20, Text: "Carrie|A Novel|Richard Bachman Stephen King|||"},
			{RowID: 30, EntityID: 30, Text: "The Gunslinger||Richard Bachman Stephen King|/lib/tower/tower.m4b|Frank Muller|Mid-World The Dark Tower"},
		}, rowsOf("books_fts", "book_id", "title || '|' || subtitle || '|' || authors || '|' || filenames || '|' || narrators || '|' || series_names"))
		assert.Equal(t, []ftsRow{
			{RowID: 70, EntityID: 70, Text: "The Dark Tower Mid-World|Roland|The Gunslinger|Stephen King"},
		}, rowsOf("series_fts", "series_id", "name || '|' || description || '|' || book_titles || '|' || book_authors"))
		assert.Equal(t, []ftsRow{
			{RowID: 50, EntityID: 50, Text: "Stephen King Richard Bachman|King, Stephen"},
			{RowID: 60, EntityID: 60, Text: "Frank Muller|Muller, Frank"},
		}, rowsOf("persons_fts", "person_id", "name || '|' || sort_name"))
		assert.Equal(t, []ftsRow{{RowID: 40, EntityID: 40, Text: "Horror Scary"}}, rowsOf("genres_fts", "genre_id", "name"))
		assert.Equal(t, []ftsRow{{RowID: 44, EntityID: 44, Text: "Classic Canon"}}, rowsOf("tags_fts", "tag_id", "name"))
		assert.Equal(t, []ftsRow{{RowID: 48, EntityID: 48, Text: "Doubleday DD"}}, rowsOf("publishers_fts", "publisher_id", "name"))

		// The rebuilt rows are searchable, including through aliases.
		assert.Equal(t, []int{20, 30}, matches("books_fts", "book_id", "Bachman"))
		assert.Equal(t, []int{70}, matches("series_fts", "series_id", "Mid*"))
	}

	migrator := newMigrator(db, target)
	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	assertRebuilt()

	// Rolling back leaves the rebuilt rows in place: the previous code deletes
	// by the stored id column, so rowids equal to the entity id work for it
	// too. Re-applying rebuilds to the same result.
	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	assertRebuilt()
	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	assertRebuilt()
}
