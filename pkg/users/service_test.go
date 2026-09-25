package users

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

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

	// Shared-cache in-memory DSN (keyed on the test name) so every pooled
	// connection sees the same DB. A bare ":memory:" gives each connection its
	// own empty database, which breaks handler tests that issue queries on a
	// different connection than the one that ran migrations.
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	sqldb, err := sql.Open(sqliteshim.ShimName, dsn)
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	// Enable foreign keys to match production behavior
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

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

// Bun writes a zero time.Time instead of omitting the column, so the
// table's DEFAULT CURRENT_TIMESTAMP never applies. Create must set both
// timestamps itself.
func TestServiceCreate_SetsTimestamps(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	user, err := svc.Create(ctx, CreateUserOptions{
		Username:         "testuser",
		Password:         "password123",
		RoleID:           getRoleIDByName(ctx, t, db, models.RoleViewer),
		AllLibraryAccess: true,
	})
	require.NoError(t, err)

	assert.WithinDuration(t, time.Now(), user.CreatedAt, time.Minute)
	assert.WithinDuration(t, time.Now(), user.UpdatedAt, time.Minute)
}

// backdateUser moves a user's timestamps into the past so tests can tell
// whether a later write bumped updated_at.
func backdateUser(ctx context.Context, t *testing.T, db *bun.DB, userID int) time.Time {
	t.Helper()

	old := time.Now().Add(-48 * time.Hour)
	_, err := db.NewUpdate().
		Model((*models.User)(nil)).
		Set("created_at = ?", old).
		Set("updated_at = ?", old).
		Where("id = ?", userID).
		Exec(ctx)
	require.NoError(t, err)

	return old
}

func TestServiceUpdate_BumpsUpdatedAt(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	created, err := svc.Create(ctx, CreateUserOptions{
		Username:         "testuser",
		Password:         "password123",
		RoleID:           getRoleIDByName(ctx, t, db, models.RoleViewer),
		AllLibraryAccess: true,
	})
	require.NoError(t, err)
	old := backdateUser(ctx, t, db, created.ID)

	// Mirror the handler: load the user, change a field, then update.
	user, err := svc.Retrieve(ctx, created.ID)
	require.NoError(t, err)
	email := "new@example.com"
	user.Email = &email
	require.NoError(t, svc.Update(ctx, user, UpdateOptions{Columns: []string{"email"}}))

	updated, err := svc.Retrieve(ctx, created.ID)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), updated.UpdatedAt, time.Minute)
	assert.WithinDuration(t, old, updated.CreatedAt, time.Second)
}

func TestServiceUpdate_LibraryAccessOnly_BumpsUpdatedAt(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	created, err := svc.Create(ctx, CreateUserOptions{
		Username:         "testuser",
		Password:         "password123",
		RoleID:           getRoleIDByName(ctx, t, db, models.RoleViewer),
		AllLibraryAccess: true,
	})
	require.NoError(t, err)
	old := backdateUser(ctx, t, db, created.ID)

	user, err := svc.Retrieve(ctx, created.ID)
	require.NoError(t, err)
	require.NoError(t, svc.Update(ctx, user, UpdateOptions{
		UpdateLibraryAccess: true,
		LibraryIDs:          []int{},
	}))

	updated, err := svc.Retrieve(ctx, created.ID)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), updated.UpdatedAt, time.Minute)
	assert.WithinDuration(t, old, updated.CreatedAt, time.Second)
}
