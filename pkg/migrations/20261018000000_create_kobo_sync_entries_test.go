package migrations

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const createKoboSyncEntriesMigrationName = "20261018000000"

// Before this migration, a synced file's RevisionId was its Shisho id plus
// the first 8 characters of the file hash in the key's latest completed sync
// point. The migration records that RevisionId for every entry so a change
// after the upgrade goes out under the RevisionId the Kobo already has.
func TestCreateKoboSyncEntries_BackfillsHashFormRevisionIDs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, createKoboSyncEntriesMigrationName)

	for _, query := range []string{
		`INSERT INTO users (id, username, password_hash, role_id, is_active) VALUES (1, 'u', 'x', 1, 1)`,
		`INSERT INTO api_keys (id, user_id, name, key, created_at, updated_at)
			VALUES ('k1', 1, 'one', 'ak_1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
			       ('k2', 1, 'two', 'ak_2', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'One', 'book')`,
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source)
			VALUES (1, 1, '/one/book', 'Book', 'filepath', 'Book', 'filepath')`,
		`INSERT INTO files (id, library_id, book_id, filepath, file_type, file_role)
			VALUES (1, 1, 1, '/one/book/a.epub', 'epub', 'main'),
			       (2, 1, 1, '/one/book/b.epub', 'epub', 'main'),
			       (3, 1, 1, '/one/book/c.epub', 'epub', 'main'),
			       (4, 1, 1, '/one/book/d.epub', 'epub', 'main')`,
		// k1: an abandoned point older than the latest completed one, an
		// older completed point, the latest completed point, and a newer
		// unfinished point. The latest completed point wins; the unfinished
		// one adds only files the completed one lacks. k2 has only an
		// unfinished point.
		`INSERT INTO kobo_sync_points (id, api_key_id, created_at, completed_at)
			VALUES ('abandoned', 'k1', '2026-01-15', NULL),
			       ('old', 'k1', '2026-01-01', '2026-01-01'),
			       ('latest', 'k1', '2026-02-01', '2026-02-01'),
			       ('ongoing', 'k1', '2026-03-01', NULL),
			       ('k2-ongoing', 'k2', '2026-02-01', NULL)`,
		`INSERT INTO kobo_sync_point_books (id, sync_point_id, file_id, file_hash, file_size, metadata_hash)
			VALUES ('b1', 'old', 1, 'aaaaaaaa11111111', 1, 'm'),
			       ('b2', 'latest', 1, 'bbbbbbbb22222222', 1, 'm'),
			       ('b3', 'latest', 2, 'cccccccc33333333', 1, 'm'),
			       ('b4', 'latest', 99, 'dddddddd44444444', 1, 'm'),
			       ('b5', 'ongoing', 1, 'eeeeeeee55555555', 1, 'm'),
			       ('b6', 'ongoing', 3, '77777777aaaaaaaa', 1, 'm'),
			       ('b7', 'abandoned', 4, '88888888bbbbbbbb', 1, 'm'),
			       ('b8', 'k2-ongoing', 2, 'ffffffff66666666', 1, 'm'),
			       ('b9', 'k2-ongoing', 4, 'abc', 1, 'm')`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)

	var rows []struct {
		APIKeyID   string `bun:"api_key_id"`
		FileID     int    `bun:"file_id"`
		RevisionID string `bun:"revision_id"`
	}
	require.NoError(t, db.NewRaw(`SELECT api_key_id, file_id, revision_id FROM kobo_sync_entries ORDER BY api_key_id, file_id`).Scan(ctx, &rows))
	got := map[string]string{}
	for _, r := range rows {
		got[fmt.Sprintf("%s/%d", r.APIKeyID, r.FileID)] = r.RevisionID
	}
	// File 99 no longer exists, so it has no entry. A hash shorter than 8
	// characters is used whole, as the old RevisionId was.
	assert.Equal(t, map[string]string{
		"k1/1": "shisho-1-bbbbbbbb",
		"k1/2": "shisho-2-cccccccc",
		"k1/3": "shisho-3-77777777",
		"k2/2": "shisho-2-ffffffff",
		"k2/4": "shisho-4-abc",
	}, got)

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
}
