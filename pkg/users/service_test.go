package users

import (
	"context"
	"database/sql"
	"testing"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func newTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func getRoleIDByName(ctx context.Context, t *testing.T, db *bun.DB, roleName string) int {
	t.Helper()

	role := new(models.Role)
	err := db.NewSelect().
		Model(role).
		Where("name = ?", roleName).
		Scan(ctx)
	require.NoError(t, err)

	return role.ID
}

func TestServiceCreate_SetsMustChangePassword(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	user, err := svc.Create(ctx, CreateUserOptions{
		Username:             "testuser",
		Password:             "password123",
		RoleID:               getRoleIDByName(ctx, t, db, models.RoleViewer),
		AllLibraryAccess:     true,
		RequirePasswordReset: true,
	})
	require.NoError(t, err)

	assert.True(t, user.MustChangePassword)
}

func TestServiceResetPassword_UpdatesMustChangePassword(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	user, err := svc.Create(ctx, CreateUserOptions{
		Username:             "testuser",
		Password:             "password123",
		RoleID:               getRoleIDByName(ctx, t, db, models.RoleViewer),
		AllLibraryAccess:     true,
		RequirePasswordReset: true,
	})
	require.NoError(t, err)

	err = svc.ResetPassword(ctx, user.ID, "newpassword123", false)
	require.NoError(t, err)

	updatedUser, err := svc.Retrieve(ctx, user.ID)
	require.NoError(t, err)
	assert.False(t, updatedUser.MustChangePassword)

	passwordValid, err := svc.VerifyPassword(ctx, user.ID, "newpassword123")
	require.NoError(t, err)
	assert.True(t, passwordValid)

	err = svc.ResetPassword(ctx, user.ID, "anotherpassword123", true)
	require.NoError(t, err)

	updatedUser, err = svc.Retrieve(ctx, user.ID)
	require.NoError(t, err)
	assert.True(t, updatedUser.MustChangePassword)
}
