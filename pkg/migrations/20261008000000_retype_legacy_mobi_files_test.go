package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const retypeLegacyMOBIMigrationName = "20261008000000"

// Before MOBI was built in, a plugin parser or supplement discovery typed a
// file by its raw extension, so .azw and .prc files were stored as "azw" and
// "prc". The built-in parser only reads type "mobi".
func TestRetypeLegacyMOBIFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, retypeLegacyMOBIMigrationName)

	for _, query := range []string{
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'One', 'book')`,
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source)
			VALUES (1, 1, '/one/book', 'Book', 'filepath', 'Book', 'filepath')`,
		`INSERT INTO files (id, library_id, book_id, filepath, file_type, file_role)
			VALUES (1, 1, 1, '/one/book/a.azw', 'azw', 'main'),
			       (2, 1, 1, '/one/book/b.PRC', 'prc', 'supplement'),
			       (3, 1, 1, '/one/book/c.mobi', 'mobi', 'main'),
			       (4, 1, 1, '/one/book/d.azw3', 'azw3', 'main'),
			       (5, 1, 1, '/one/book/e.epub', 'epub', 'main')`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}

	types := func() map[int]string {
		var rows []struct {
			ID       int    `bun:"id"`
			FileType string `bun:"file_type"`
		}
		require.NoError(t, db.NewRaw("SELECT id, file_type FROM files").Scan(ctx, &rows))
		result := make(map[int]string)
		for _, r := range rows {
			result[r.ID] = r.FileType
		}
		return result
	}

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[int]string{1: "mobi", 2: "mobi", 3: "mobi", 4: "azw3", 5: "epub"}, types())

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[int]string{1: "azw", 2: "prc", 3: "mobi", 4: "azw3", 5: "epub"}, types())

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
}
