package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const reclassifyKePubMigrationName = "20261023000000"

// Before KePub was a File Type, a .kepub.epub was typed by its last
// extension, as an EPUB. That the reclassified Files miss their EPUB-era
// cache entries is TestComputeFingerprint_SourceFileTypeChangesHash. A bare
// .kepub supplement was typed by its raw extension and keeps it both ways.
func TestReclassifyKePubFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, reclassifyKePubMigrationName)

	for _, query := range []string{
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'One', 'book')`,
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source)
			VALUES (1, 1, '/one/book', 'Book', 'filepath', 'Book', 'filepath')`,
		`INSERT INTO files (id, library_id, book_id, filepath, file_type, file_role)
			VALUES (1, 1, 1, '/one/book/a.kepub.epub', 'epub', 'main'),
			       (2, 1, 1, '/one/book/b.KePub.EPUB', 'epub', 'supplement'),
			       (3, 1, 1, '/one/book/c.epub', 'epub', 'main'),
			       (4, 1, 1, '/one/book.kepub/d.epub', 'epub', 'main'),
			       (5, 1, 1, '/one/book/e.cbz', 'cbz', 'main'),
			       (6, 1, 1, '/one/book/f.kepub', 'kepub', 'supplement')`,
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
	assert.Equal(t, map[int]string{1: "kepub", 2: "kepub", 3: "epub", 4: "epub", 5: "cbz", 6: "kepub"}, types())

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[int]string{1: "epub", 2: "epub", 3: "epub", 4: "epub", 5: "cbz", 6: "kepub"}, types())

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
}
