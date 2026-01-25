package apikeys

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shishobooks/shisho/pkg/migrations"
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

func TestService_Create(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user first
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	apiKey, err := svc.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	assert.NotEmpty(t, apiKey.ID)
	assert.Equal(t, 1, apiKey.UserID)
	assert.Equal(t, "My Kobo", apiKey.Name)
	assert.NotEmpty(t, apiKey.Key)
	assert.GreaterOrEqual(t, len(apiKey.Key), 32, "Key should be at least 32 characters")
	assert.NotZero(t, apiKey.CreatedAt)
	assert.NotZero(t, apiKey.UpdatedAt)
}

func TestService_List(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create two keys
	key1, err := svc.Create(ctx, 1, "Kobo 1")
	require.NoError(t, err)
	key2, err := svc.Create(ctx, 1, "Kobo 2")
	require.NoError(t, err)

	// List keys
	keys, err := svc.List(ctx, 1)
	require.NoError(t, err)

	assert.Len(t, keys, 2)
	assert.Equal(t, key1.ID, keys[0].ID)
	assert.Equal(t, key2.ID, keys[1].ID)
}

func TestService_GetByKey(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create a key
	created, err := svc.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Get by key
	found, err := svc.GetByKey(ctx, created.Key)
	require.NoError(t, err)

	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, created.Key, found.Key)
}

func TestService_GetByKey_NotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	found, err := svc.GetByKey(ctx, "nonexistent")
	assert.Nil(t, found)
	assert.NoError(t, err)
}

func TestService_Delete(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create a key
	created, err := svc.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Delete
	err = svc.Delete(ctx, 1, created.ID)
	require.NoError(t, err)

	// Verify deleted
	keys, err := svc.List(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, keys)
}

func TestService_Delete_WrongUser(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create two test users
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'user1', 'hash', 1), (2, 'user2', 'hash', 1)`)
	require.NoError(t, err)

	// Create a key for user 1
	created, err := svc.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Try to delete as user 2 - should fail
	err = svc.Delete(ctx, 2, created.ID)
	assert.Error(t, err)
}

func TestService_UpdateName(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create a key
	created, err := svc.Create(ctx, 1, "Old Name")
	require.NoError(t, err)

	// Update name
	updated, err := svc.UpdateName(ctx, 1, created.ID, "New Name")
	require.NoError(t, err)

	assert.Equal(t, "New Name", updated.Name)
	assert.True(t, updated.UpdatedAt.After(created.UpdatedAt) || updated.UpdatedAt.Equal(created.UpdatedAt))
}

func TestService_AddPermission(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create a key
	created, err := svc.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Add permission
	updated, err := svc.AddPermission(ctx, 1, created.ID, PermissionEReaderBrowser)
	require.NoError(t, err)

	assert.True(t, updated.HasPermission(PermissionEReaderBrowser))
}

func TestService_AddPermission_Duplicate(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create a key
	created, err := svc.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Add permission twice
	_, err = svc.AddPermission(ctx, 1, created.ID, PermissionEReaderBrowser)
	require.NoError(t, err)
	_, err = svc.AddPermission(ctx, 1, created.ID, PermissionEReaderBrowser)
	require.NoError(t, err) // Should succeed without error (idempotent)
}

func TestService_RemovePermission(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create a key with permission
	created, err := svc.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)
	created, err = svc.AddPermission(ctx, 1, created.ID, PermissionEReaderBrowser)
	require.NoError(t, err)

	// Remove permission
	updated, err := svc.RemovePermission(ctx, 1, created.ID, PermissionEReaderBrowser)
	require.NoError(t, err)

	assert.False(t, updated.HasPermission(PermissionEReaderBrowser))
}

func TestService_GenerateShortURL(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create a key
	created, err := svc.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Generate short URL
	shortURL, err := svc.GenerateShortURL(ctx, 1, created.ID)
	require.NoError(t, err)

	assert.NotEmpty(t, shortURL.ID)
	assert.Equal(t, created.ID, shortURL.APIKeyID)
	assert.Len(t, shortURL.ShortCode, 6)
	assert.Regexp(t, "^[a-z0-9]+$", shortURL.ShortCode)
	assert.True(t, shortURL.ExpiresAt.After(time.Now()))
	assert.True(t, shortURL.ExpiresAt.Before(time.Now().Add(31*time.Minute)))
}

func TestService_ResolveShortCode(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create a key
	created, err := svc.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Generate short URL
	shortURL, err := svc.GenerateShortURL(ctx, 1, created.ID)
	require.NoError(t, err)

	// Resolve
	apiKey, err := svc.ResolveShortCode(ctx, shortURL.ShortCode)
	require.NoError(t, err)

	assert.Equal(t, created.ID, apiKey.ID)
	assert.Equal(t, created.Key, apiKey.Key)
}

func TestService_ResolveShortCode_Expired(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create a key
	created, err := svc.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Insert expired short URL directly
	shortURL := &APIKeyShortURL{
		ID:        uuid.New().String(),
		APIKeyID:  created.ID,
		ShortCode: "expird",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
		CreatedAt: time.Now().Add(-2 * time.Hour),
	}
	_, err = db.NewInsert().Model(shortURL).Exec(ctx)
	require.NoError(t, err)

	// Resolve - should return nil
	apiKey, err := svc.ResolveShortCode(ctx, shortURL.ShortCode)
	require.NoError(t, err)
	assert.Nil(t, apiKey)
}

func TestService_TouchLastAccessed(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create a key
	created, err := svc.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)
	assert.Nil(t, created.LastAccessedAt)

	// Touch
	err = svc.TouchLastAccessed(ctx, created.ID)
	require.NoError(t, err)

	// Verify updated
	keys, err := svc.List(ctx, 1)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.NotNil(t, keys[0].LastAccessedAt)
	assert.Less(t, time.Since(*keys[0].LastAccessedAt), time.Minute)
}
