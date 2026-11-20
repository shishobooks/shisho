package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each user gets their username with its case folded as a key, except users
// whose usernames fold to the same key: the old ASCII-only uniqueness let
// them in, and the upgrade keeps both by leaving them without one. Rolling
// back drops the column and its index.
func TestAddUsersUsernameKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, "20261119231820")

	_, err := db.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role_id)
		VALUES (1, 'Émile', 'hash', 1),
		       (2, 'émile', 'hash', 1),
		       (3, 'Straße', 'hash', 1),
		       (4, 'STRASSE', 'hash', 1),
		       (5, 'Renée', 'hash', 1),
		       (6, 'Rene' || char(769) || 'e2', 'hash', 1),
		       (7, 'bob', 'hash', 1)
	`)
	require.NoError(t, err)

	type row struct {
		ID  int            `bun:"id"`
		Key sql.NullString `bun:"username_key"`
	}
	keyed := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

	for range 2 {
		_, err = migrator.Migrate(ctx)
		require.NoError(t, err)

		var rows []row
		require.NoError(t, db.NewRaw(`SELECT id, username_key FROM users ORDER BY id`).Scan(ctx, &rows))
		assert.Equal(t, []row{
			{ID: 1}, {ID: 2},
			{ID: 3}, {ID: 4},
			{ID: 5, Key: keyed("renée")},
			{ID: 6, Key: keyed("renée2")},
			{ID: 7, Key: keyed("bob")},
		}, rows)
		assert.True(t, indexExists(ctx, t, db, "ux_users_username_key"))

		_, err = db.ExecContext(ctx, `INSERT INTO users (username, username_key, password_hash, role_id) VALUES ('BOB2', 'bob', 'hash', 1)`)
		require.Error(t, err, "the key is unique")

		_, err = migrator.Rollback(ctx)
		require.NoError(t, err)
		assert.NotContains(t, columnNames(ctx, t, db, "users"), "username_key")
		assert.False(t, indexExists(ctx, t, db, "ux_users_username_key"))
	}
}
