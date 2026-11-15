package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun/migrate"
)

// An .m4a is an M4B (ADR 0018). Files Shisho already tracked under an .m4a
// path, as supplements typed by their extension or as main Files a plugin
// parsed, become M4B main Files with their IDs, and lose their stored mtime
// so the next scan reads them. Every other File is left alone.
func TestRetypeM4AFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)

	for _, query := range []string{
		`CREATE TABLE files (
			id INTEGER PRIMARY KEY,
			filepath TEXT NOT NULL,
			file_type TEXT NOT NULL,
			file_role TEXT NOT NULL,
			file_modified_at TIMESTAMPTZ
		)`,
		`INSERT INTO files VALUES
			(1, '/lib/Book/Bonus.m4a', 'm4a', 'supplement', '2026-01-01 00:00:00'),
			(2, '/lib/Other/Audio.M4A', 'm4a', 'main', '2026-01-01 00:00:00'),
			(3, '/lib/Book/Book.epub', 'epub', 'main', '2026-01-01 00:00:00'),
			(4, '/lib/Book/Notes.mp3', 'mp3', 'supplement', '2026-01-01 00:00:00'),
			(5, '/lib/Audio/Book.m4b', 'm4b', 'main', '2026-01-01 00:00:00'),
			(6, '/lib/m4a/readme.txt', 'txt', 'supplement', '2026-01-01 00:00:00')`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err)
	}

	migrationSet := migrate.NewMigrations()
	for _, migration := range Migrations.Sorted() {
		if migration.Name == "20261115035343" {
			migrationSet.Add(migration)
		}
	}
	require.Len(t, migrationSet.Sorted(), 1)
	migrator := newMigrator(db, migrationSet)
	require.NoError(t, migrator.Init(ctx))
	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)

	type row struct {
		ID             int     `bun:"id"`
		FileType       string  `bun:"file_type"`
		FileRole       string  `bun:"file_role"`
		FileModifiedAt *string `bun:"file_modified_at"`
	}
	read := func() []row {
		var rows []row
		require.NoError(t, db.NewRaw("SELECT id, file_type, file_role, file_modified_at FROM files ORDER BY id").Scan(ctx, &rows))
		return rows
	}
	stamp := "2026-01-01 00:00:00"
	want := []row{
		{1, "m4b", "main", nil},
		{2, "m4b", "main", nil},
		{3, "epub", "main", &stamp},
		{4, "mp3", "supplement", &stamp},
		{5, "m4b", "main", &stamp},
		{6, "txt", "supplement", &stamp},
	}
	assert.Equal(t, want, read())

	// The data migration is irreversible, so rolling back succeeds and
	// changes nothing.
	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	assert.Equal(t, want, read())
}
