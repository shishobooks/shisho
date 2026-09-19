package migrations

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestClearPluginSourcesOnEmptyIdentifyValues(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	t.Cleanup(func() {
		require.NoError(t, sqldb.Close())
	})

	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	_, err = db.ExecContext(ctx, `
		CREATE TABLE books (
			id INTEGER PRIMARY KEY,
			subtitle TEXT, subtitle_source TEXT,
			description TEXT, description_source TEXT
		)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		CREATE TABLE files (
			id INTEGER PRIMARY KEY,
			publisher_id INTEGER, publisher_source TEXT,
			url TEXT, url_source TEXT,
			release_date TIMESTAMP, release_date_source TEXT
		)`)
	require.NoError(t, err)

	// Row 1: the defect. An old Identify clear left a plugin source on an
	// absent value. Row 2: the generic plugin source. Row 3: the Edit form's
	// deliberate protected empty slot. Row 4: a populated plugin value.
	// Row 5: an absent value from a non-plugin source.
	_, err = db.ExecContext(ctx, `
		INSERT INTO books (id, subtitle, subtitle_source, description, description_source) VALUES
			(1, NULL, 'plugin:test/enricher', NULL, 'plugin:test/enricher'),
			(2, NULL, 'plugin', NULL, 'plugin'),
			(3, NULL, 'manual', NULL, 'manual'),
			(4, 'Subtitle', 'plugin:test/enricher', 'Description', 'plugin:test/enricher'),
			(5, NULL, 'epub_metadata', NULL, 'sidecar')
	`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO files (id, publisher_id, publisher_source, url, url_source, release_date, release_date_source) VALUES
			(1, NULL, 'plugin:test/enricher', NULL, 'plugin:test/enricher', NULL, 'plugin:test/enricher'),
			(2, NULL, 'plugin', NULL, 'plugin', NULL, 'plugin'),
			(3, NULL, 'manual', NULL, 'manual', NULL, 'manual'),
			(4, 9, 'plugin:test/enricher', 'https://example.com', 'plugin:test/enricher', '2024-01-02 00:00:00', 'plugin:test/enricher'),
			(5, NULL, 'epub_metadata', NULL, 'sidecar', NULL, 'filepath')
	`)
	require.NoError(t, err)

	require.NoError(t, clearPluginSourcesOnEmptyIdentifyValues(ctx, db))

	sources := func(table string, columns ...string) map[int][]*string {
		t.Helper()
		rows, err := db.QueryContext(ctx, "SELECT id, "+strings.Join(columns, ", ")+" FROM "+table+" ORDER BY id")
		require.NoError(t, err)
		defer func() {
			require.NoError(t, rows.Close())
		}()

		got := make(map[int][]*string)
		for rows.Next() {
			var id int
			values := make([]*string, len(columns))
			dest := []any{&id}
			for i := range values {
				dest = append(dest, &values[i])
			}
			require.NoError(t, rows.Scan(dest...))
			got[id] = values
		}
		require.NoError(t, rows.Err())
		return got
	}

	repeat := func(source *string, n int) []*string {
		out := make([]*string, n)
		for i := range out {
			out[i] = source
		}
		return out
	}
	pluginSource := stringPointer("plugin:test/enricher")

	books := sources("books", "subtitle_source", "description_source")
	assert.Equal(t, repeat(nil, 2), books[1], "plugin source on an absent value is cleared")
	assert.Equal(t, repeat(nil, 2), books[2], "generic plugin source on an absent value is cleared")
	assert.Equal(t, repeat(stringPointer("manual"), 2), books[3], "a manual empty slot stays protected")
	assert.Equal(t, repeat(pluginSource, 2), books[4], "a populated plugin value keeps its source")
	assert.Equal(t, []*string{stringPointer("epub_metadata"), stringPointer("sidecar")}, books[5])

	files := sources("files", "publisher_source", "url_source", "release_date_source")
	assert.Equal(t, repeat(nil, 3), files[1], "plugin source on an absent value is cleared")
	assert.Equal(t, repeat(nil, 3), files[2], "generic plugin source on an absent value is cleared")
	assert.Equal(t, repeat(stringPointer("manual"), 3), files[3], "a manual empty slot stays protected")
	assert.Equal(t, repeat(pluginSource, 3), files[4], "a populated plugin value keeps its source")
	assert.Equal(t, []*string{stringPointer("epub_metadata"), stringPointer("sidecar"), stringPointer("filepath")}, files[5])
}
