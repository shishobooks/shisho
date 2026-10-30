package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cleanLibraryPathsMigrationName = "20261030000000"

func TestCleanLibraryPaths(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, cleanLibraryPathsMigrationName)

	for _, query := range []string{
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'One', 'book'), (2, 'Two', 'book')`,
		// Library 1 entered the same folder twice; the repeat goes.
		`INSERT INTO library_paths (id, library_id, filepath) VALUES
			(1, 1, '/books/'),
			(2, 1, '/books'),
			(3, 1, '/audio//sub/../'),
			(4, 2, '/books/'),
			(5, 2, '/')`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)

	var rows []struct {
		ID        int    `bun:"id"`
		LibraryID int    `bun:"library_id"`
		Filepath  string `bun:"filepath"`
	}
	require.NoError(t, db.NewRaw("SELECT id, library_id, filepath FROM library_paths ORDER BY id").Scan(ctx, &rows))
	got := make(map[int]string, len(rows))
	for _, r := range rows {
		got[r.ID] = r.Filepath
	}
	assert.Equal(t, map[int]string{1: "/books", 3: "/audio", 4: "/books", 5: "/"}, got)
}
