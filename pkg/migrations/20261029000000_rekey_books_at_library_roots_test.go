package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rekeyBooksAtLibraryRootsMigrationName = "20261029000000"

func TestRekeyBooksAtLibraryRoots(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, rekeyBooksAtLibraryRootsMigrationName)

	for _, query := range []string{
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'One', 'book'), (2, 'Two', 'book'), (3, 'Three', 'book'), (4, 'Four', 'book')`,
		// Library paths are stored as entered, so one may end in a slash.
		`INSERT INTO library_paths (library_id, filepath) VALUES (1, '/one'), (2, '/two'), (3, '/three/'), (4, '/four')`,
		// 1: one main file plus a supplement at the root, so it is rekeyed.
		// 2: two main files, so it is left alone.
		// 3: in a folder, so it is left alone.
		// 4: at a root stored with a trailing slash, so it is rekeyed.
		// 5: its file's path is already another Book's key, so it is left alone.
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source)
			VALUES (1, 1, '/one', 'Split', 'filepath', 'Split', 'filepath'),
			       (2, 2, '/two', 'Pair', 'filepath', 'Pair', 'filepath'),
			       (3, 1, '/one/Folder', 'Folder', 'filepath', 'Folder', 'filepath'),
			       (4, 3, '/three', 'Slash', 'filepath', 'Slash', 'filepath'),
			       (5, 4, '/four', 'Taken', 'filepath', 'Taken', 'filepath'),
			       (6, 4, '/four/x.epub', 'Holder', 'filepath', 'Holder', 'filepath')`,
		`INSERT INTO files (id, library_id, book_id, filepath, file_type, file_role)
			VALUES (1, 1, 1, '/one/Split.epub', 'epub', 'main'),
			       (2, 1, 1, '/one/Split.pdf', 'pdf', 'supplement'),
			       (3, 2, 2, '/two/a.epub', 'epub', 'main'),
			       (4, 2, 2, '/two/b.epub', 'epub', 'main'),
			       (5, 1, 3, '/one/Folder/Folder.epub', 'epub', 'main'),
			       (6, 3, 4, '/three/s.epub', 'epub', 'main'),
			       (7, 4, 5, '/four/x.epub', 'epub', 'main')`,
		`INSERT INTO books_fts (rowid, book_id, library_id, title, filepath, subtitle, authors, filenames, narrators, series_names)
			VALUES (1, 1, 1, 'Split', '/one', '', '', '/one/Split.epub', '', '')`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)

	bookPath := func(id int) string {
		var path string
		require.NoError(t, db.NewRaw("SELECT filepath FROM books WHERE id = ?", id).Scan(ctx, &path))
		return path
	}
	assert.Equal(t, "/one/Split.epub", bookPath(1))
	assert.Equal(t, "/two", bookPath(2))
	assert.Equal(t, "/one/Folder", bookPath(3))
	assert.Equal(t, "/three/s.epub", bookPath(4))
	assert.Equal(t, "/four", bookPath(5))

	var ftsPath string
	require.NoError(t, db.NewRaw("SELECT filepath FROM books_fts WHERE rowid = 1").Scan(ctx, &ftsPath))
	assert.Equal(t, "/one/Split.epub", ftsPath)
}
