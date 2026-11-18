package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Existing users get Getting Started only while a forced password reset is
// pending: they never finished signing in, so they are still new. Everyone
// else starts with it off. Rolling back drops the columns.
func TestAddUserGettingStarted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, "20261116215650")

	_, err := db.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role_id, must_change_password)
		VALUES (1, 'signed-in', 'hash', 1, FALSE),
		       (2, 'pending-reset', 'hash', 3, TRUE)
	`)
	require.NoError(t, err)

	type state struct {
		ID             int     `bun:"id"`
		Enabled        bool    `bun:"getting_started_enabled"`
		AutoOpen       bool    `bun:"getting_started_auto_open"`
		DismissedAt    *string `bun:"getting_started_dismissed_at"`
		NotForMe       string  `bun:"getting_started_not_for_me"`
		DismissedSteps string  `bun:"getting_started_dismissed_steps"`
	}

	for range 2 {
		_, err = migrator.Migrate(ctx)
		require.NoError(t, err)

		var rows []state
		require.NoError(t, db.NewRaw(`
			SELECT id, getting_started_enabled, getting_started_auto_open,
				getting_started_dismissed_at, getting_started_not_for_me,
				getting_started_dismissed_steps
			FROM users ORDER BY id
		`).Scan(ctx, &rows))
		assert.Equal(t, []state{
			{ID: 1, Enabled: false, AutoOpen: false, NotForMe: "[]", DismissedSteps: "[]"},
			{ID: 2, Enabled: true, AutoOpen: true, NotForMe: "[]", DismissedSteps: "[]"},
		}, rows)

		_, err = migrator.Rollback(ctx)
		require.NoError(t, err)
		assert.NotContains(t, columnNames(ctx, t, db, "users"), "getting_started_enabled")
		assert.NotContains(t, columnNames(ctx, t, db, "users"), "getting_started_not_for_me")
		assert.NotContains(t, columnNames(ctx, t, db, "users"), "getting_started_dismissed_steps")
	}
}
