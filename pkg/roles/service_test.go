package roles

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
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

	db := testdb.New(t)
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

	db := testdb.New(t)
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

	db := testdb.New(t)
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

// Delete refuses a role that users still hold with a validation error. The
// check runs before the DELETE, so callers get this message rather than the
// raw constraint failure from users.role_id's ON DELETE RESTRICT.
func TestServiceDelete_RoleAssignedToUsers(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	svc := NewService(db)
	ctx := context.Background()

	role, err := svc.Create(ctx, "Custom", []PermissionInput{
		{Resource: models.ResourceBooks, Operation: models.OperationRead},
	})
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.User{
		Username:     "holder",
		PasswordHash: "hash",
		RoleID:       role.ID,
		IsActive:     true,
	}).Exec(ctx)
	require.NoError(t, err)

	err = svc.Delete(ctx, role.ID)
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, http.StatusUnprocessableEntity, codeErr.HTTPCode)
	assert.Equal(t, "Cannot delete role that is assigned to users", codeErr.Message)

	stillThere, err := svc.Retrieve(ctx, role.ID)
	require.NoError(t, err)
	assert.Len(t, stillThere.Permissions, 1)

	_, err = db.NewDelete().Model((*models.User)(nil)).Where("role_id = ?", role.ID).Exec(ctx)
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, role.ID))
}
