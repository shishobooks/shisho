package lists

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

func setupTestDB(t *testing.T) *bun.DB {
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

func createTestUser(t *testing.T, db *bun.DB, username string) *models.User {
	t.Helper()
	user := &models.User{
		Username:     username,
		PasswordHash: "test",
		RoleID:       1,
		IsActive:     true,
	}
	_, err := db.NewInsert().Model(user).Exec(context.Background())
	require.NoError(t, err)
	return user
}

func TestService_CreateList(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	user := createTestUser(t, db, "testuser")

	t.Run("creates unordered list with default sort", func(t *testing.T) {
		list, err := svc.CreateList(ctx, CreateListOptions{
			UserID:    user.ID,
			Name:      "My List",
			IsOrdered: false,
		})
		require.NoError(t, err)
		assert.Equal(t, "My List", list.Name)
		assert.Equal(t, user.ID, list.UserID)
		assert.False(t, list.IsOrdered)
		assert.Equal(t, models.ListSortAddedAtDesc, list.DefaultSort)
	})

	t.Run("creates ordered list with manual sort", func(t *testing.T) {
		list, err := svc.CreateList(ctx, CreateListOptions{
			UserID:    user.ID,
			Name:      "Ordered List",
			IsOrdered: true,
		})
		require.NoError(t, err)
		assert.True(t, list.IsOrdered)
		assert.Equal(t, models.ListSortManual, list.DefaultSort)
	})

	t.Run("creates list with description", func(t *testing.T) {
		desc := "A description for the list"
		list, err := svc.CreateList(ctx, CreateListOptions{
			UserID:      user.ID,
			Name:        "List With Desc",
			Description: &desc,
		})
		require.NoError(t, err)
		assert.NotNil(t, list.Description)
		assert.Equal(t, desc, *list.Description)
	})

	t.Run("creates list with custom default sort", func(t *testing.T) {
		list, err := svc.CreateList(ctx, CreateListOptions{
			UserID:      user.ID,
			Name:        "Custom Sort List",
			DefaultSort: models.ListSortTitleAsc,
		})
		require.NoError(t, err)
		assert.Equal(t, models.ListSortTitleAsc, list.DefaultSort)
	})
}

func TestService_ListLists(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	user1 := createTestUser(t, db, "user1")
	user2 := createTestUser(t, db, "user2")

	// User1 creates two lists
	_, err := svc.CreateList(ctx, CreateListOptions{UserID: user1.ID, Name: "User1 List1"})
	require.NoError(t, err)
	_, err = svc.CreateList(ctx, CreateListOptions{UserID: user1.ID, Name: "User1 List2"})
	require.NoError(t, err)

	// User2 creates one list
	_, err = svc.CreateList(ctx, CreateListOptions{UserID: user2.ID, Name: "User2 List"})
	require.NoError(t, err)

	t.Run("returns only user's owned lists", func(t *testing.T) {
		lists, total, err := svc.ListListsWithTotal(ctx, ListListsOptions{UserID: user1.ID})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, lists, 2)
	})

	t.Run("includes shared lists", func(t *testing.T) {
		// Create a list owned by user2 and share with user1
		sharedList, err := svc.CreateList(ctx, CreateListOptions{UserID: user2.ID, Name: "Shared with User1"})
		require.NoError(t, err)

		_, err = svc.CreateShare(ctx, CreateShareOptions{
			ListID:         sharedList.ID,
			UserID:         user1.ID,
			Permission:     models.ListPermissionViewer,
			SharedByUserID: user2.ID,
		})
		require.NoError(t, err)

		lists, total, err := svc.ListListsWithTotal(ctx, ListListsOptions{UserID: user1.ID})
		require.NoError(t, err)
		// user1 should now see their 2 owned lists + 1 shared list
		assert.Equal(t, 3, total)
		assert.Len(t, lists, 3)
	})
}

func TestService_Pagination(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	user := createTestUser(t, db, "user1")

	// Create three lists
	_, err := svc.CreateList(ctx, CreateListOptions{UserID: user.ID, Name: "List A"})
	require.NoError(t, err)
	_, err = svc.CreateList(ctx, CreateListOptions{UserID: user.ID, Name: "List B"})
	require.NoError(t, err)
	_, err = svc.CreateList(ctx, CreateListOptions{UserID: user.ID, Name: "List C"})
	require.NoError(t, err)

	// Test without limit returns all lists
	lists, total, err := svc.ListListsWithTotal(ctx, ListListsOptions{UserID: user.ID})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, lists, 3)

	// Test limit using ListLists (not ListListsWithTotal)
	// Note: There's a known bug with bun's ScanAndCount + Limit in SQLite in-memory tests,
	// so we use ListLists which uses Scan instead of ScanAndCount.
	limit := 1
	lists, err = svc.ListLists(ctx, ListListsOptions{
		UserID: user.ID,
		Limit:  &limit,
	})
	require.NoError(t, err)
	assert.Len(t, lists, 1, "should return only 1 list with limit=1")

	// Test offset using ListLists
	limit = 2
	offset := 1
	lists, err = svc.ListLists(ctx, ListListsOptions{
		UserID: user.ID,
		Limit:  &limit,
		Offset: &offset,
	})
	require.NoError(t, err)
	assert.Len(t, lists, 2, "should return 2 lists with limit=2 offset=1")
}

func TestService_Permissions(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	owner := createTestUser(t, db, "owner")
	viewer := createTestUser(t, db, "viewer")
	editor := createTestUser(t, db, "editor")
	manager := createTestUser(t, db, "manager")
	outsider := createTestUser(t, db, "outsider")

	list, err := svc.CreateList(ctx, CreateListOptions{UserID: owner.ID, Name: "Test List"})
	require.NoError(t, err)

	// Create shares
	_, err = svc.CreateShare(ctx, CreateShareOptions{ListID: list.ID, UserID: viewer.ID, Permission: models.ListPermissionViewer, SharedByUserID: owner.ID})
	require.NoError(t, err)
	_, err = svc.CreateShare(ctx, CreateShareOptions{ListID: list.ID, UserID: editor.ID, Permission: models.ListPermissionEditor, SharedByUserID: owner.ID})
	require.NoError(t, err)
	_, err = svc.CreateShare(ctx, CreateShareOptions{ListID: list.ID, UserID: manager.ID, Permission: models.ListPermissionManager, SharedByUserID: owner.ID})
	require.NoError(t, err)

	t.Run("owner has all permissions", func(t *testing.T) {
		isOwner, err := svc.IsOwner(ctx, list.ID, owner.ID)
		require.NoError(t, err)
		canView, err := svc.CanView(ctx, list.ID, owner.ID)
		require.NoError(t, err)
		canEdit, err := svc.CanEdit(ctx, list.ID, owner.ID)
		require.NoError(t, err)
		canManage, err := svc.CanManage(ctx, list.ID, owner.ID)
		require.NoError(t, err)

		assert.True(t, isOwner)
		assert.True(t, canView)
		assert.True(t, canEdit)
		assert.True(t, canManage)
	})

	t.Run("viewer can only view", func(t *testing.T) {
		isOwner, err := svc.IsOwner(ctx, list.ID, viewer.ID)
		require.NoError(t, err)
		canView, err := svc.CanView(ctx, list.ID, viewer.ID)
		require.NoError(t, err)
		canEdit, err := svc.CanEdit(ctx, list.ID, viewer.ID)
		require.NoError(t, err)
		canManage, err := svc.CanManage(ctx, list.ID, viewer.ID)
		require.NoError(t, err)

		assert.False(t, isOwner)
		assert.True(t, canView)
		assert.False(t, canEdit)
		assert.False(t, canManage)
	})

	t.Run("editor can view and edit", func(t *testing.T) {
		isOwner, err := svc.IsOwner(ctx, list.ID, editor.ID)
		require.NoError(t, err)
		canView, err := svc.CanView(ctx, list.ID, editor.ID)
		require.NoError(t, err)
		canEdit, err := svc.CanEdit(ctx, list.ID, editor.ID)
		require.NoError(t, err)
		canManage, err := svc.CanManage(ctx, list.ID, editor.ID)
		require.NoError(t, err)

		assert.False(t, isOwner)
		assert.True(t, canView)
		assert.True(t, canEdit)
		assert.False(t, canManage)
	})

	t.Run("manager can view, edit, and manage", func(t *testing.T) {
		isOwner, err := svc.IsOwner(ctx, list.ID, manager.ID)
		require.NoError(t, err)
		canView, err := svc.CanView(ctx, list.ID, manager.ID)
		require.NoError(t, err)
		canEdit, err := svc.CanEdit(ctx, list.ID, manager.ID)
		require.NoError(t, err)
		canManage, err := svc.CanManage(ctx, list.ID, manager.ID)
		require.NoError(t, err)

		assert.False(t, isOwner)
		assert.True(t, canView)
		assert.True(t, canEdit)
		assert.True(t, canManage)
	})

	t.Run("outsider has no permissions", func(t *testing.T) {
		isOwner, err := svc.IsOwner(ctx, list.ID, outsider.ID)
		require.NoError(t, err)
		canView, err := svc.CanView(ctx, list.ID, outsider.ID)
		require.NoError(t, err)
		canEdit, err := svc.CanEdit(ctx, list.ID, outsider.ID)
		require.NoError(t, err)
		canManage, err := svc.CanManage(ctx, list.ID, outsider.ID)
		require.NoError(t, err)

		assert.False(t, isOwner)
		assert.False(t, canView)
		assert.False(t, canEdit)
		assert.False(t, canManage)
	})

	t.Run("non-existent list returns false for all permissions", func(t *testing.T) {
		nonExistentListID := 99999

		isOwner, err := svc.IsOwner(ctx, nonExistentListID, owner.ID)
		require.NoError(t, err)
		canView, err := svc.CanView(ctx, nonExistentListID, owner.ID)
		require.NoError(t, err)
		canEdit, err := svc.CanEdit(ctx, nonExistentListID, owner.ID)
		require.NoError(t, err)
		canManage, err := svc.CanManage(ctx, nonExistentListID, owner.ID)
		require.NoError(t, err)

		assert.False(t, isOwner)
		assert.False(t, canView)
		assert.False(t, canEdit)
		assert.False(t, canManage)
	})
}

func TestService_RetrieveList(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	user := createTestUser(t, db, "testuser")

	t.Run("retrieves existing list", func(t *testing.T) {
		created, err := svc.CreateList(ctx, CreateListOptions{
			UserID: user.ID,
			Name:   "Test List",
		})
		require.NoError(t, err)

		retrieved, err := svc.RetrieveList(ctx, RetrieveListOptions{ID: &created.ID})
		require.NoError(t, err)
		assert.Equal(t, created.ID, retrieved.ID)
		assert.Equal(t, "Test List", retrieved.Name)
		assert.NotNil(t, retrieved.User)
		assert.Equal(t, user.ID, retrieved.User.ID)
	})

	t.Run("returns not found for non-existent list", func(t *testing.T) {
		nonExistentID := 99999
		_, err := svc.RetrieveList(ctx, RetrieveListOptions{ID: &nonExistentID})
		assert.Error(t, err)
	})
}

func TestService_UpdateList(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	user := createTestUser(t, db, "testuser")

	list, err := svc.CreateList(ctx, CreateListOptions{
		UserID: user.ID,
		Name:   "Original Name",
	})
	require.NoError(t, err)

	t.Run("updates name", func(t *testing.T) {
		list.Name = "Updated Name"
		err := svc.UpdateList(ctx, list, UpdateListOptions{Columns: []string{"name"}})
		require.NoError(t, err)

		retrieved, err := svc.RetrieveList(ctx, RetrieveListOptions{ID: &list.ID})
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", retrieved.Name)
	})

	t.Run("does nothing with empty columns", func(t *testing.T) {
		err := svc.UpdateList(ctx, list, UpdateListOptions{Columns: []string{}})
		require.NoError(t, err)
	})
}

func TestService_DeleteList(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	user := createTestUser(t, db, "testuser")

	list, err := svc.CreateList(ctx, CreateListOptions{
		UserID: user.ID,
		Name:   "To Be Deleted",
	})
	require.NoError(t, err)

	t.Run("deletes list", func(t *testing.T) {
		err := svc.DeleteList(ctx, list.ID)
		require.NoError(t, err)

		_, err = svc.RetrieveList(ctx, RetrieveListOptions{ID: &list.ID})
		assert.Error(t, err)
	})
}

func TestService_Shares(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	owner := createTestUser(t, db, "owner")
	sharedUser := createTestUser(t, db, "shared")

	list, err := svc.CreateList(ctx, CreateListOptions{
		UserID: owner.ID,
		Name:   "Shared List",
	})
	require.NoError(t, err)

	t.Run("creates share", func(t *testing.T) {
		share, err := svc.CreateShare(ctx, CreateShareOptions{
			ListID:         list.ID,
			UserID:         sharedUser.ID,
			Permission:     models.ListPermissionEditor,
			SharedByUserID: owner.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, list.ID, share.ListID)
		assert.Equal(t, sharedUser.ID, share.UserID)
		assert.Equal(t, models.ListPermissionEditor, share.Permission)
		assert.NotNil(t, share.SharedByUserID)
		assert.Equal(t, owner.ID, *share.SharedByUserID)
	})

	t.Run("lists shares", func(t *testing.T) {
		shares, err := svc.ListShares(ctx, list.ID)
		require.NoError(t, err)
		assert.Len(t, shares, 1)
		assert.Equal(t, sharedUser.ID, shares[0].UserID)
	})

	t.Run("updates share permission", func(t *testing.T) {
		shares, err := svc.ListShares(ctx, list.ID)
		require.NoError(t, err)
		require.Len(t, shares, 1)

		err = svc.UpdateShare(ctx, shares[0].ID, models.ListPermissionManager)
		require.NoError(t, err)

		updatedShares, err := svc.ListShares(ctx, list.ID)
		require.NoError(t, err)
		assert.Equal(t, models.ListPermissionManager, updatedShares[0].Permission)
	})

	t.Run("deletes share", func(t *testing.T) {
		shares, err := svc.ListShares(ctx, list.ID)
		require.NoError(t, err)
		require.Len(t, shares, 1)

		err = svc.DeleteShare(ctx, shares[0].ID)
		require.NoError(t, err)

		sharesAfterDelete, err := svc.ListShares(ctx, list.ID)
		require.NoError(t, err)
		assert.Empty(t, sharesAfterDelete)
	})
}
