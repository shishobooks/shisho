package testdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

type orderHook struct {
	name  string
	calls *[]string
}

func (h orderHook) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	*h.calls = append(*h.calls, "before "+h.name)
	return ctx
}

func (h orderHook) AfterQuery(_ context.Context, _ *bun.QueryEvent) {
	*h.calls = append(*h.calls, "after "+h.name)
}

// A hook added after code took hold of the database still sees that code's
// queries, the way services built before the hook was added run them.
func TestAddQueryHook_SeenByEarlierHolders(t *testing.T) {
	t.Parallel()
	db := New(t)
	holder := struct{ db *bun.DB }{db: db}

	var calls []string
	AddQueryHook(t, db, orderHook{name: "a", calls: &calls})

	_, err := holder.db.ExecContext(t.Context(), "SELECT 1")
	require.NoError(t, err)
	assert.Equal(t, []string{"before a", "after a"}, calls)
}

// Hooks run in the order bun runs its own: BeforeQuery in the order added,
// AfterQuery in reverse.
func TestAddQueryHook_RunsInBunOrder(t *testing.T) {
	t.Parallel()
	db := NewShared(t)

	var calls []string
	AddQueryHook(t, db, orderHook{name: "a", calls: &calls})
	AddQueryHook(t, db, orderHook{name: "b", calls: &calls})

	_, err := db.ExecContext(t.Context(), "SELECT 1")
	require.NoError(t, err)
	assert.Equal(t, []string{"before a", "before b", "after b", "after a"}, calls)
}
