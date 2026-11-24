package serverinstance

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_KeepsOneIDAcrossStarts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)

	first, err := Load(ctx, appsettings.NewService(db))
	require.NoError(t, err)
	assert.Len(t, first, 32)

	// A restart loads the same database again.
	again, err := Load(ctx, appsettings.NewService(db))
	require.NoError(t, err)
	assert.Equal(t, first, again)
}

func TestLoad_AFreshDatabaseGetsADifferentID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	one, err := Load(ctx, appsettings.NewService(testdb.New(t)))
	require.NoError(t, err)
	other, err := Load(ctx, appsettings.NewService(testdb.New(t)))
	require.NoError(t, err)
	assert.NotEqual(t, one, other)
}

// A database restored from a backup keeps the id it was saved with.
func TestLoad_KeepsTheIDOfARestoredDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := appsettings.NewService(testdb.New(t))
	require.NoError(t, store.SetJSON(ctx, settingsKey, "restored-id"))

	id, err := Load(ctx, store)
	require.NoError(t, err)
	assert.Equal(t, "restored-id", id)
}

// An empty id, as a database edited by hand could hold, would match no app
// and keep every one signing in again, so it's replaced.
func TestLoad_ReplacesAnEmptyID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := appsettings.NewService(testdb.New(t))
	for _, empty := range []any{"", nil} {
		require.NoError(t, store.SetJSON(ctx, settingsKey, empty))

		id, err := Load(ctx, store)
		require.NoError(t, err)
		assert.Len(t, id, 32)
		again, err := Load(ctx, store)
		require.NoError(t, err)
		assert.Equal(t, id, again)
	}
}
