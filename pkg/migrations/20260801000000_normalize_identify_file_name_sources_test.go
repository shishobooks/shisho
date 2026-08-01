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
)

func TestNormalizeIdentifyFileNameSources(t *testing.T) {
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

	_, err = db.ExecContext(ctx, `CREATE TABLE files (id INTEGER PRIMARY KEY, name_source TEXT)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO files (id, name_source) VALUES
			(1, 'user'),
			(2, 'plugin'),
			(3, 'plugin:test/enricher'),
			(4, 'manual'),
			(5, 'epub_metadata'),
			(6, NULL)
	`)
	require.NoError(t, err)

	require.NoError(t, normalizeIdentifyFileNameSources(ctx, db))

	rows, err := db.QueryContext(ctx, `SELECT id, name_source FROM files ORDER BY id`)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, rows.Close())
	}()

	got := make(map[int]*string)
	for rows.Next() {
		var id int
		var source *string
		require.NoError(t, rows.Scan(&id, &source))
		got[id] = source
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, stringPointer("manual"), got[1])
	assert.Equal(t, stringPointer("plugin"), got[2])
	assert.Equal(t, stringPointer("plugin:test/enricher"), got[3])
	assert.Equal(t, stringPointer("manual"), got[4])
	assert.Equal(t, stringPointer("epub_metadata"), got[5])
	assert.Nil(t, got[6])
}

func stringPointer(value string) *string { return &value }
