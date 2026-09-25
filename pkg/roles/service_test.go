package roles

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// backdateRole moves a role's timestamps into the past so tests can tell
// whether a later write bumped updated_at.
func backdateRole(ctx context.Context, t *testing.T, db *bun.DB, roleID int) time.Time {
	t.Helper()

	old := time.Now().Add(-48 * time.Hour)
	_, err := db.NewUpdate().
		Model((*models.Role)(nil)).
		Set("created_at = ?", old).
		Set("updated_at = ?", old).
		Where("id = ?", roleID).
		Exec(ctx)
	require.NoError(t, err)

	return old
}

// Bun writes a zero time.Time instead of omitting the column, so the
// table's DEFAULT CURRENT_TIMESTAMP never applies. Create must set both
// timestamps itself.
func TestServiceCreate_SetsTimestamps(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	svc := NewService(db)

	role, err := svc.Create(context.Background(), "Custom", []PermissionInput{
		{Resource: models.ResourceBooks, Operation: models.OperationRead},
	})
	require.NoError(t, err)

	assert.WithinDuration(t, time.Now(), role.CreatedAt, time.Minute)
	assert.WithinDuration(t, time.Now(), role.UpdatedAt, time.Minute)
}

func TestServiceUpdate_Rename_BumpsUpdatedAt(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	role, err := svc.Create(ctx, "Custom", nil)
	require.NoError(t, err)
	old := backdateRole(ctx, t, db, role.ID)

	name := "Renamed"
	updated, err := svc.Update(ctx, role.ID, &name, nil)
	require.NoError(t, err)

	assert.Equal(t, "Renamed", updated.Name)
	assert.WithinDuration(t, time.Now(), updated.UpdatedAt, time.Minute)
	assert.WithinDuration(t, old, updated.CreatedAt, time.Second)
}

func TestServiceUpdate_PermissionsOnly_BumpsUpdatedAt(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	role, err := svc.Create(ctx, "Custom", nil)
	require.NoError(t, err)
	old := backdateRole(ctx, t, db, role.ID)

	perms := []PermissionInput{{Resource: models.ResourceBooks, Operation: models.OperationRead}}
	updated, err := svc.Update(ctx, role.ID, nil, &perms)
	require.NoError(t, err)

	assert.Len(t, updated.Permissions, 1)
	assert.WithinDuration(t, time.Now(), updated.UpdatedAt, time.Minute)
	assert.WithinDuration(t, old, updated.CreatedAt, time.Second)
}
