package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

const repairMissingParentsMigrationName = "20261005000000"

func countRows(ctx context.Context, t *testing.T, db *bun.DB, query string) int {
	t.Helper()
	var count int
	require.NoError(t, db.NewRaw(query).Scan(ctx, &count))
	return count
}

// Rows left behind when a Book was deleted on a connection with foreign keys
// off: the cascade never ran, so the Book's files, the files' children, and
// the Book's join rows still point at it.
func TestRepairRowsWithMissingParents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, repairMissingParentsMigrationName)

	_, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF")
	require.NoError(t, err)
	for _, query := range []string{
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'One', 'book')`,
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source)
			VALUES (1, 1, '/one/kept', 'Kept', 'filepath', 'Kept', 'filepath'),
			       (2, 1, '/one/gone', 'Gone', 'filepath', 'Gone', 'filepath')`,
		`INSERT INTO persons (id, library_id, name, sort_name, sort_name_source) VALUES (1, 1, 'Author', 'Author', 'filepath')`,
		`INSERT INTO series (id, library_id, name, sort_name, sort_name_source, name_source) VALUES (1, 1, 'Series', 'Series', 'filepath', 'filepath')`,
		`INSERT INTO publishers (id, library_id, name) VALUES (1, 1, 'Gone Publisher')`,
		`INSERT INTO files (id, library_id, book_id, filepath, file_type, publisher_id)
			VALUES (1, 1, 1, '/one/kept/kept.epub', 'epub', 1),
			       (2, 1, 2, '/one/gone/gone.m4b', 'm4b', NULL)`,
		`INSERT INTO chapters (file_id, sort_order, title) VALUES (1, 0, 'Kept'), (2, 0, 'Gone')`,
		`INSERT INTO narrators (file_id, person_id, sort_order) VALUES (2, 1, 0)`,
		`INSERT INTO authors (book_id, person_id, sort_order) VALUES (1, 1, 0), (2, 1, 0)`,
		`INSERT INTO book_series (book_id, series_id, sort_order) VALUES (1, 1, 0), (2, 1, 0)`,
		`INSERT INTO books_fts (rowid, book_id, library_id, title, filepath, subtitle, authors, filenames, narrators, series_names)
			VALUES (1, 1, 1, 'Kept', '/one/kept', '', 'Author', '/one/kept/kept.epub', '', 'Series'),
			       (2, 2, 1, 'Gone', '/one/gone', '', 'Author', '/one/gone/gone.m4b', 'Author', 'Series')`,
		`INSERT INTO series_fts (rowid, series_id, library_id, name, description, book_titles, book_authors)
			VALUES (1, 1, 1, 'Series', '', 'Kept Gone', 'Author')`,
		`DELETE FROM books WHERE id = 2`,
		`DELETE FROM publishers WHERE id = 1`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}
	_, err = db.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	require.Positive(t, foreignKeyViolations(ctx, t, db))

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)

	assert.Zero(t, foreignKeyViolations(ctx, t, db))

	assert.Equal(t, 1, countRows(ctx, t, db, "SELECT COUNT(*) FROM files"))
	assert.Equal(t, 1, countRows(ctx, t, db, "SELECT COUNT(*) FROM files WHERE id = 1 AND publisher_id IS NULL"),
		"a missing publisher is cleared, as ON DELETE SET NULL would")
	assert.Equal(t, 1, countRows(ctx, t, db, "SELECT COUNT(*) FROM chapters"))
	assert.Zero(t, countRows(ctx, t, db, "SELECT COUNT(*) FROM narrators"))
	assert.Equal(t, 1, countRows(ctx, t, db, "SELECT COUNT(*) FROM authors WHERE book_id = 1"))
	assert.Zero(t, countRows(ctx, t, db, "SELECT COUNT(*) FROM authors WHERE book_id = 2"))
	assert.Equal(t, 1, countRows(ctx, t, db, "SELECT COUNT(*) FROM book_series WHERE book_id = 1"))
	assert.Zero(t, countRows(ctx, t, db, "SELECT COUNT(*) FROM book_series WHERE book_id = 2"))

	assert.Equal(t, []int{1}, ftsRowIDs(ctx, t, db, "books_fts"), "the deleted Book's search row is gone")
	var bookTitles string
	require.NoError(t, db.NewRaw("SELECT book_titles FROM series_fts WHERE rowid = 1").Scan(ctx, &bookTitles))
	assert.Equal(t, "Kept", bookTitles, "the Series no longer lists the deleted Book")

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
}

func TestRepairRowsWithMissingParents_NothingToRepair(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, repairMissingParentsMigrationName)

	for _, query := range []string{
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'One', 'book')`,
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source)
			VALUES (1, 1, '/one/kept', 'Kept', 'filepath', 'Kept', 'filepath')`,
		`INSERT INTO files (id, library_id, book_id, filepath, file_type) VALUES (1, 1, 1, '/one/kept/kept.epub', 'epub')`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, countRows(ctx, t, db, "SELECT COUNT(*) FROM files"))
	assert.Zero(t, foreignKeyViolations(ctx, t, db))
}

func ftsRowIDs(ctx context.Context, t *testing.T, db *bun.DB, table string) []int {
	t.Helper()
	var ids []int
	require.NoError(t, db.NewRaw("SELECT rowid FROM "+table+" ORDER BY rowid").Scan(ctx, &ids))
	return ids
}
