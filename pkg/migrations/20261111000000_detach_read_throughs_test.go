package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

const detachReadThroughsMigrationName = "20261111000000"

func seedDetachReadThroughRows(ctx context.Context, t *testing.T, db *bun.DB) {
	t.Helper()
	for _, query := range []string{
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'One', 'book')`,
		`INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'reader', 'hash', 1)`,
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source)
			VALUES (1, 1, '/one/a', 'A', 'filepath', 'A', 'filepath'),
			       (2, 1, '/one/b', 'B', 'filepath', 'B', 'filepath')`,
		`INSERT INTO files (id, library_id, book_id, filepath, file_type)
			VALUES (1, 1, 1, '/one/a/a.epub', 'epub'),
			       (2, 1, 2, '/one/b/b.m4b', 'm4b')`,
		`INSERT INTO read_throughs (id, user_id, book_id, begin_day, end_day, outcome)
			VALUES (1, 1, 1, '2026-03-01', '2026-03-04', 'finished'),
			       (2, 1, 2, '2026-03-05', NULL, 'underway')`,
		// Bun writes instants with nanoseconds and a zone offset.
		`INSERT INTO reading_sessions (id, user_id, file_id, read_through_id, source, started_at, ended_at, start_percentage, end_percentage, measured)
			VALUES (1, 1, 1, 1, 'web_reader', '2026-03-01 10:00:00.250000000+00:00', '2026-03-01 10:30:00.250000000+00:00', 0, 0.2, TRUE),
			       (2, 1, 2, 2, 'web_reader', '2026-03-05 21:00:00+00:00', '2026-03-05 22:15:30+00:00', 0, 0.1, TRUE),
			       (3, 1, NULL, 1, 'kobo', '2026-03-02 08:00:00+00:00', '2026-03-02 08:00:45+00:00', 0.2, 0.3, FALSE)`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}
}

func TestDetachReadThroughs_DeletingABookKeepsItsReadThroughs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, detachReadThroughsMigrationName)
	seedDetachReadThroughRows(ctx, t, db)

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `DELETE FROM books WHERE id = 1`)
	require.NoError(t, err)

	var bookID *int
	require.NoError(t, db.NewRaw(`SELECT book_id FROM read_throughs WHERE id = 1`).Scan(ctx, &bookID))
	assert.Nil(t, bookID, "the Read-through outlives its Book")
	var sessions int
	require.NoError(t, db.NewRaw(`SELECT COUNT(*) FROM reading_sessions WHERE read_through_id = 1`).Scan(ctx, &sessions))
	assert.Equal(t, 2, sessions, "its Sessions stay with it")

	// The copy columns exist for the delete to fill.
	_, err = db.ExecContext(ctx, `UPDATE read_throughs SET book_title = 'A', book_authors = '["Ann"]' WHERE id = 1`)
	require.NoError(t, err)

	// A detached Read-through underway doesn't block another one.
	_, err = db.ExecContext(ctx, `DELETE FROM books WHERE id = 2`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO read_throughs (user_id, book_id, outcome) VALUES (1, NULL, 'underway')`)
	require.NoError(t, err)
}

func TestDetachReadThroughs_FillsSessionKindAndDuration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, detachReadThroughsMigrationName)
	seedDetachReadThroughRows(ctx, t, db)

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)

	var rows []struct {
		ID       int    `bun:"id"`
		Kind     string `bun:"kind"`
		Duration int    `bun:"duration"`
	}
	require.NoError(t, db.NewRaw(`SELECT id, kind, duration FROM reading_sessions ORDER BY id`).Scan(ctx, &rows))
	require.Len(t, rows, 3)
	assert.Equal(t, "reading", rows[0].Kind)
	assert.Equal(t, 30*60, rows[0].Duration)
	assert.Equal(t, "listening", rows[1].Kind, "a Session on an audiobook is listening")
	assert.Equal(t, 75*60+30, rows[1].Duration)
	assert.Equal(t, "reading", rows[2].Kind, "a Session whose File is gone counts as reading")
	assert.Equal(t, 45, rows[2].Duration)

	_, err = db.ExecContext(ctx, `UPDATE reading_sessions SET kind = 'watching' WHERE id = 1`)
	require.Error(t, err, "kind is reading or listening")
}

func TestDetachReadThroughs_RollsBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, detachReadThroughsMigrationName)
	seedDetachReadThroughRows(ctx, t, db)

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `DELETE FROM books WHERE id = 1`)
	require.NoError(t, err)

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)

	// The old schema can't hold a Read-through without a Book, so the
	// detached one goes, with its Sessions.
	var ids []int
	require.NoError(t, db.NewRaw(`SELECT id FROM read_throughs ORDER BY id`).Scan(ctx, &ids))
	assert.Equal(t, []int{2}, ids)
	var sessions []int
	require.NoError(t, db.NewRaw(`SELECT id FROM reading_sessions ORDER BY id`).Scan(ctx, &sessions))
	assert.Equal(t, []int{2}, sessions)

	_, err = db.ExecContext(ctx, `DELETE FROM books WHERE id = 2`)
	require.NoError(t, err)
	var remaining int
	require.NoError(t, db.NewRaw(`SELECT COUNT(*) FROM read_throughs`).Scan(ctx, &remaining))
	assert.Equal(t, 0, remaining, "deleting a Book cascades again")

	var columns []string
	require.NoError(t, db.NewRaw(`SELECT name FROM pragma_table_info('reading_sessions')`).Scan(ctx, &columns))
	assert.NotContains(t, columns, "kind")
	assert.NotContains(t, columns, "duration")
}
