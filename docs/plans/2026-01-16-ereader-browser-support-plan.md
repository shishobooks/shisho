# eReader Browser Support Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable stock eReader browsers (Kobo, Kindle) to browse and download books via a minimal HTML interface with API key authentication.

**Architecture:** Create a new `pkg/apikeys` package for API key management, and `pkg/ereader` for HTML UI. The eReader UI reuses the existing OPDS service layer for data access. Authentication uses API keys embedded in URL paths since eReader browsers don't support Basic Auth. Kobo devices auto-convert EPUB/CBZ to KePub format.

**Tech Stack:** Go (Echo, Bun ORM, SQLite), React/TypeScript (TanStack Query), existing kepub and downloadcache packages.

---

## Task 1: Database Migrations for API Keys

**Files:**
- Create: `pkg/migrations/20260116000000_add_api_keys.go`

**Step 1: Create the migration file**

```go
package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		// Create api_keys table
		_, err := db.Exec(`
			CREATE TABLE api_keys (
				id TEXT PRIMARY KEY,
				user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				name TEXT NOT NULL,
				key TEXT NOT NULL UNIQUE,
				created_at DATETIME NOT NULL,
				updated_at DATETIME NOT NULL,
				last_accessed_at DATETIME
			);
			CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
			CREATE INDEX idx_api_keys_key ON api_keys(key);

			CREATE TABLE api_key_permissions (
				id TEXT PRIMARY KEY,
				api_key_id TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
				permission TEXT NOT NULL,
				created_at DATETIME NOT NULL,
				UNIQUE(api_key_id, permission)
			);
			CREATE INDEX idx_api_key_permissions_api_key_id ON api_key_permissions(api_key_id);

			CREATE TABLE api_key_short_urls (
				id TEXT PRIMARY KEY,
				api_key_id TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
				short_code TEXT NOT NULL UNIQUE,
				expires_at DATETIME NOT NULL,
				created_at DATETIME NOT NULL
			);
			CREATE INDEX idx_short_urls_code ON api_key_short_urls(short_code);
		`)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			DROP TABLE IF EXISTS api_key_short_urls;
			DROP TABLE IF EXISTS api_key_permissions;
			DROP TABLE IF EXISTS api_keys;
		`)
		return err
	})
}
```

**Step 2: Run migration to verify it works**

Run: `make db:migrate`
Expected: Migration applies successfully with no errors

**Step 3: Test rollback**

Run: `make db:rollback`
Expected: Rollback succeeds, tables are dropped

**Step 4: Re-apply migration**

Run: `make db:migrate`
Expected: Migration re-applies successfully

**Step 5: Commit**

```bash
git add pkg/migrations/20260116000000_add_api_keys.go
git commit -m "[Database] Add migrations for API keys tables"
```

---

## Task 2: API Key Models

**Files:**
- Create: `pkg/apikeys/model.go`

**Step 1: Create the model file**

```go
package apikeys

import (
	"time"

	"github.com/uptrace/bun"
)

// ApiKey represents a user's API key for programmatic access
type ApiKey struct {
	bun.BaseModel `bun:"table:api_keys,alias:ak" json:"-"`

	ID             string     `bun:"id,pk" json:"id"`
	UserID         int        `bun:"user_id,notnull" json:"userId"`
	Name           string     `bun:"name,notnull" json:"name"`
	Key            string     `bun:"key,notnull,unique" json:"key"`
	CreatedAt      time.Time  `bun:"created_at,notnull" json:"createdAt"`
	UpdatedAt      time.Time  `bun:"updated_at,notnull" json:"updatedAt"`
	LastAccessedAt *time.Time `bun:"last_accessed_at" json:"lastAccessedAt"`

	Permissions []*ApiKeyPermission `bun:"rel:has-many,join:id=api_key_id" json:"permissions"`
}

// ApiKeyPermission represents a permission granted to an API key
type ApiKeyPermission struct {
	bun.BaseModel `bun:"table:api_key_permissions,alias:akp" json:"-"`

	ID         string    `bun:"id,pk" json:"id"`
	ApiKeyID   string    `bun:"api_key_id,notnull" json:"apiKeyId"`
	Permission string    `bun:"permission,notnull" json:"permission"`
	CreatedAt  time.Time `bun:"created_at,notnull" json:"createdAt"`
}

// ApiKeyShortUrl represents a temporary short URL for eReader setup
type ApiKeyShortUrl struct {
	bun.BaseModel `bun:"table:api_key_short_urls,alias:aksu" json:"-"`

	ID        string    `bun:"id,pk" json:"id"`
	ApiKeyID  string    `bun:"api_key_id,notnull" json:"apiKeyId"`
	ShortCode string    `bun:"short_code,notnull,unique" json:"shortCode"`
	ExpiresAt time.Time `bun:"expires_at,notnull" json:"expiresAt"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"createdAt"`

	ApiKey *ApiKey `bun:"rel:belongs-to,join:api_key_id=id" json:"-"`
}

// PermissionEReaderBrowser is the permission for accessing the eReader browser UI
const PermissionEReaderBrowser = "ereader_browser"

// HasPermission checks if the API key has a specific permission
func (ak *ApiKey) HasPermission(permission string) bool {
	for _, p := range ak.Permissions {
		if p.Permission == permission {
			return true
		}
	}
	return false
}

// PermissionStrings returns a list of permission strings
func (ak *ApiKey) PermissionStrings() []string {
	perms := make([]string, len(ak.Permissions))
	for i, p := range ak.Permissions {
		perms[i] = p.Permission
	}
	return perms
}
```

**Step 2: Generate TypeScript types**

Run: `make tygo`
Expected: Types generated (or "Nothing to be done" if already up-to-date)

**Step 3: Commit**

```bash
git add pkg/apikeys/model.go
git commit -m "[Models] Add API key models with permissions"
```

---

## Task 3: API Key Service - Core CRUD

**Files:**
- Create: `pkg/apikeys/service.go`
- Create: `pkg/apikeys/service_test.go`

**Step 1: Write the failing test for key generation**

```go
package apikeys

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"shisho/pkg/database"
)

func setupTestDB(t *testing.T) *bun.DB {
	db := database.NewTestDB(t)
	return db
}

func TestService_Create(t *testing.T) {
	db := setupTestDB(t)
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
	assert.True(t, len(apiKey.Key) >= 32, "Key should be at least 32 characters")
	assert.NotZero(t, apiKey.CreatedAt)
	assert.NotZero(t, apiKey.UpdatedAt)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/apikeys/... -run TestService_Create -v`
Expected: FAIL - service not implemented

**Step 3: Write minimal service implementation**

```go
package apikeys

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// generateKey creates a cryptographically secure random API key
func generateKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "ak_" + base64.URLEncoding.EncodeToString(bytes), nil
}

// Create creates a new API key for a user
func (s *Service) Create(ctx context.Context, userID int, name string) (*ApiKey, error) {
	key, err := generateKey()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	apiKey := &ApiKey{
		ID:        uuid.New().String(),
		UserID:    userID,
		Name:      name,
		Key:       key,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = s.db.NewInsert().Model(apiKey).Exec(ctx)
	if err != nil {
		return nil, err
	}

	return apiKey, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/apikeys/... -run TestService_Create -v`
Expected: PASS

**Step 5: Write test for List**

Add to `service_test.go`:

```go
func TestService_List(t *testing.T) {
	db := setupTestDB(t)
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
```

**Step 6: Run test to verify it fails**

Run: `go test ./pkg/apikeys/... -run TestService_List -v`
Expected: FAIL - List not implemented

**Step 7: Implement List**

Add to `service.go`:

```go
// List returns all API keys for a user
func (s *Service) List(ctx context.Context, userID int) ([]*ApiKey, error) {
	var keys []*ApiKey
	err := s.db.NewSelect().
		Model(&keys).
		Relation("Permissions").
		Where("user_id = ?", userID).
		Order("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return keys, nil
}
```

**Step 8: Run test to verify it passes**

Run: `go test ./pkg/apikeys/... -run TestService_List -v`
Expected: PASS

**Step 9: Write test for GetByKey**

Add to `service_test.go`:

```go
func TestService_GetByKey(t *testing.T) {
	db := setupTestDB(t)
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
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	found, err := svc.GetByKey(ctx, "nonexistent")
	assert.Nil(t, found)
	assert.NoError(t, err)
}
```

**Step 10: Run tests to verify they fail**

Run: `go test ./pkg/apikeys/... -run TestService_GetByKey -v`
Expected: FAIL - GetByKey not implemented

**Step 11: Implement GetByKey**

Add to `service.go`:

```go
// GetByKey retrieves an API key by its key value
func (s *Service) GetByKey(ctx context.Context, key string) (*ApiKey, error) {
	apiKey := new(ApiKey)
	err := s.db.NewSelect().
		Model(apiKey).
		Relation("Permissions").
		Where("key = ?", key).
		Scan(ctx)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return apiKey, nil
}
```

**Step 12: Run tests to verify they pass**

Run: `go test ./pkg/apikeys/... -run TestService_GetByKey -v`
Expected: PASS

**Step 13: Write test for Delete**

Add to `service_test.go`:

```go
func TestService_Delete(t *testing.T) {
	db := setupTestDB(t)
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
	db := setupTestDB(t)
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
```

**Step 14: Run tests to verify they fail**

Run: `go test ./pkg/apikeys/... -run TestService_Delete -v`
Expected: FAIL - Delete not implemented

**Step 15: Implement Delete**

Add to `service.go`:

```go
import (
	"errors"
	// ... other imports
)

var ErrNotFound = errors.New("api key not found")

// Delete removes an API key (only if owned by the user)
func (s *Service) Delete(ctx context.Context, userID int, keyID string) error {
	result, err := s.db.NewDelete().
		Model((*ApiKey)(nil)).
		Where("id = ?", keyID).
		Where("user_id = ?", userID).
		Exec(ctx)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
```

**Step 16: Run tests to verify they pass**

Run: `go test ./pkg/apikeys/... -run TestService_Delete -v`
Expected: PASS

**Step 17: Write test for UpdateName**

Add to `service_test.go`:

```go
func TestService_UpdateName(t *testing.T) {
	db := setupTestDB(t)
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
```

**Step 18: Run test to verify it fails**

Run: `go test ./pkg/apikeys/... -run TestService_UpdateName -v`
Expected: FAIL - UpdateName not implemented

**Step 19: Implement UpdateName**

Add to `service.go`:

```go
// UpdateName updates an API key's name
func (s *Service) UpdateName(ctx context.Context, userID int, keyID string, name string) (*ApiKey, error) {
	now := time.Now()
	result, err := s.db.NewUpdate().
		Model((*ApiKey)(nil)).
		Set("name = ?", name).
		Set("updated_at = ?", now).
		Where("id = ?", keyID).
		Where("user_id = ?", userID).
		Exec(ctx)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, ErrNotFound
	}

	// Fetch the updated key
	var apiKey ApiKey
	err = s.db.NewSelect().
		Model(&apiKey).
		Relation("Permissions").
		Where("ak.id = ?", keyID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	return &apiKey, nil
}
```

**Step 20: Run test to verify it passes**

Run: `go test ./pkg/apikeys/... -run TestService_UpdateName -v`
Expected: PASS

**Step 21: Run all tests and commit**

Run: `go test ./pkg/apikeys/... -v`
Expected: All tests pass

```bash
git add pkg/apikeys/service.go pkg/apikeys/service_test.go
git commit -m "[API Keys] Add service with CRUD operations"
```

---

## Task 4: API Key Service - Permissions

**Files:**
- Modify: `pkg/apikeys/service.go`
- Modify: `pkg/apikeys/service_test.go`

**Step 1: Write test for AddPermission**

Add to `service_test.go`:

```go
func TestService_AddPermission(t *testing.T) {
	db := setupTestDB(t)
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
	db := setupTestDB(t)
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/apikeys/... -run TestService_AddPermission -v`
Expected: FAIL - AddPermission not implemented

**Step 3: Implement AddPermission**

Add to `service.go`:

```go
// AddPermission adds a permission to an API key
func (s *Service) AddPermission(ctx context.Context, userID int, keyID string, permission string) (*ApiKey, error) {
	// Verify ownership
	var apiKey ApiKey
	err := s.db.NewSelect().
		Model(&apiKey).
		Where("id = ?", keyID).
		Where("user_id = ?", userID).
		Scan(ctx)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Insert permission (ignore if already exists)
	perm := &ApiKeyPermission{
		ID:         uuid.New().String(),
		ApiKeyID:   keyID,
		Permission: permission,
		CreatedAt:  time.Now(),
	}
	_, err = s.db.NewInsert().
		Model(perm).
		On("CONFLICT (api_key_id, permission) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch updated key with permissions
	err = s.db.NewSelect().
		Model(&apiKey).
		Relation("Permissions").
		Where("ak.id = ?", keyID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	return &apiKey, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/apikeys/... -run TestService_AddPermission -v`
Expected: PASS

**Step 5: Write test for RemovePermission**

Add to `service_test.go`:

```go
func TestService_RemovePermission(t *testing.T) {
	db := setupTestDB(t)
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
```

**Step 6: Run test to verify it fails**

Run: `go test ./pkg/apikeys/... -run TestService_RemovePermission -v`
Expected: FAIL - RemovePermission not implemented

**Step 7: Implement RemovePermission**

Add to `service.go`:

```go
// RemovePermission removes a permission from an API key
func (s *Service) RemovePermission(ctx context.Context, userID int, keyID string, permission string) (*ApiKey, error) {
	// Verify ownership
	var apiKey ApiKey
	err := s.db.NewSelect().
		Model(&apiKey).
		Where("id = ?", keyID).
		Where("user_id = ?", userID).
		Scan(ctx)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Delete permission
	_, err = s.db.NewDelete().
		Model((*ApiKeyPermission)(nil)).
		Where("api_key_id = ?", keyID).
		Where("permission = ?", permission).
		Exec(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch updated key with permissions
	err = s.db.NewSelect().
		Model(&apiKey).
		Relation("Permissions").
		Where("ak.id = ?", keyID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	return &apiKey, nil
}
```

**Step 8: Run test to verify it passes**

Run: `go test ./pkg/apikeys/... -run TestService_RemovePermission -v`
Expected: PASS

**Step 9: Run all tests and commit**

Run: `go test ./pkg/apikeys/... -v`
Expected: All tests pass

```bash
git add pkg/apikeys/service.go pkg/apikeys/service_test.go
git commit -m "[API Keys] Add permission management"
```

---

## Task 5: API Key Service - Short URLs

**Files:**
- Modify: `pkg/apikeys/service.go`
- Modify: `pkg/apikeys/service_test.go`

**Step 1: Write test for GenerateShortUrl**

Add to `service_test.go`:

```go
func TestService_GenerateShortUrl(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create a key
	created, err := svc.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Generate short URL
	shortUrl, err := svc.GenerateShortUrl(ctx, 1, created.ID)
	require.NoError(t, err)

	assert.NotEmpty(t, shortUrl.ID)
	assert.Equal(t, created.ID, shortUrl.ApiKeyID)
	assert.Len(t, shortUrl.ShortCode, 6)
	assert.Regexp(t, "^[a-z0-9]+$", shortUrl.ShortCode)
	assert.True(t, shortUrl.ExpiresAt.After(time.Now()))
	assert.True(t, shortUrl.ExpiresAt.Before(time.Now().Add(31*time.Minute)))
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/apikeys/... -run TestService_GenerateShortUrl -v`
Expected: FAIL - GenerateShortUrl not implemented

**Step 3: Implement GenerateShortUrl**

Add to `service.go`:

```go
const (
	shortCodeLength = 6
	shortCodeChars  = "abcdefghijklmnopqrstuvwxyz0123456789"
	shortUrlTTL     = 30 * time.Minute
)

// generateShortCode creates a random short code
func generateShortCode() (string, error) {
	bytes := make([]byte, shortCodeLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	for i := range bytes {
		bytes[i] = shortCodeChars[int(bytes[i])%len(shortCodeChars)]
	}
	return string(bytes), nil
}

// GenerateShortUrl creates a temporary short URL for an API key
func (s *Service) GenerateShortUrl(ctx context.Context, userID int, keyID string) (*ApiKeyShortUrl, error) {
	// Verify ownership
	var apiKey ApiKey
	err := s.db.NewSelect().
		Model(&apiKey).
		Where("id = ?", keyID).
		Where("user_id = ?", userID).
		Scan(ctx)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Generate unique short code
	var shortCode string
	for i := 0; i < 10; i++ {
		shortCode, err = generateShortCode()
		if err != nil {
			return nil, err
		}
		// Check if already exists
		exists, err := s.db.NewSelect().
			Model((*ApiKeyShortUrl)(nil)).
			Where("short_code = ?", shortCode).
			Exists(ctx)
		if err != nil {
			return nil, err
		}
		if !exists {
			break
		}
	}

	now := time.Now()
	shortUrl := &ApiKeyShortUrl{
		ID:        uuid.New().String(),
		ApiKeyID:  keyID,
		ShortCode: shortCode,
		ExpiresAt: now.Add(shortUrlTTL),
		CreatedAt: now,
	}

	_, err = s.db.NewInsert().Model(shortUrl).Exec(ctx)
	if err != nil {
		return nil, err
	}

	return shortUrl, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/apikeys/... -run TestService_GenerateShortUrl -v`
Expected: PASS

**Step 5: Write test for ResolveShortCode**

Add to `service_test.go`:

```go
func TestService_ResolveShortCode(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create a key
	created, err := svc.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Generate short URL
	shortUrl, err := svc.GenerateShortUrl(ctx, 1, created.ID)
	require.NoError(t, err)

	// Resolve
	apiKey, err := svc.ResolveShortCode(ctx, shortUrl.ShortCode)
	require.NoError(t, err)

	assert.Equal(t, created.ID, apiKey.ID)
	assert.Equal(t, created.Key, apiKey.Key)
}

func TestService_ResolveShortCode_Expired(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create a key
	created, err := svc.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Insert expired short URL directly
	shortUrl := &ApiKeyShortUrl{
		ID:        uuid.New().String(),
		ApiKeyID:  created.ID,
		ShortCode: "expird",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
		CreatedAt: time.Now().Add(-2 * time.Hour),
	}
	_, err = db.NewInsert().Model(shortUrl).Exec(ctx)
	require.NoError(t, err)

	// Resolve - should return nil
	apiKey, err := svc.ResolveShortCode(ctx, shortUrl.ShortCode)
	assert.NoError(t, err)
	assert.Nil(t, apiKey)
}
```

**Step 6: Run tests to verify they fail**

Run: `go test ./pkg/apikeys/... -run TestService_ResolveShortCode -v`
Expected: FAIL - ResolveShortCode not implemented

**Step 7: Implement ResolveShortCode**

Add to `service.go`:

```go
// ResolveShortCode looks up a short code and returns the associated API key
func (s *Service) ResolveShortCode(ctx context.Context, shortCode string) (*ApiKey, error) {
	var shortUrl ApiKeyShortUrl
	err := s.db.NewSelect().
		Model(&shortUrl).
		Relation("ApiKey").
		Where("short_code = ?", shortCode).
		Where("expires_at > ?", time.Now()).
		Scan(ctx)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}

	return shortUrl.ApiKey, nil
}
```

**Step 8: Run tests to verify they pass**

Run: `go test ./pkg/apikeys/... -run TestService_ResolveShortCode -v`
Expected: PASS

**Step 9: Run all tests and commit**

Run: `go test ./pkg/apikeys/... -v`
Expected: All tests pass

```bash
git add pkg/apikeys/service.go pkg/apikeys/service_test.go
git commit -m "[API Keys] Add short URL generation and resolution"
```

---

## Task 6: API Key Service - Touch Last Accessed

**Files:**
- Modify: `pkg/apikeys/service.go`
- Modify: `pkg/apikeys/service_test.go`

**Step 1: Write test for TouchLastAccessed**

Add to `service_test.go`:

```go
func TestService_TouchLastAccessed(t *testing.T) {
	db := setupTestDB(t)
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
	assert.True(t, time.Since(*keys[0].LastAccessedAt) < time.Minute)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/apikeys/... -run TestService_TouchLastAccessed -v`
Expected: FAIL - TouchLastAccessed not implemented

**Step 3: Implement TouchLastAccessed**

Add to `service.go`:

```go
// TouchLastAccessed updates the last_accessed_at timestamp for an API key
func (s *Service) TouchLastAccessed(ctx context.Context, keyID string) error {
	_, err := s.db.NewUpdate().
		Model((*ApiKey)(nil)).
		Set("last_accessed_at = ?", time.Now()).
		Where("id = ?", keyID).
		Exec(ctx)
	return err
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/apikeys/... -run TestService_TouchLastAccessed -v`
Expected: PASS

**Step 5: Run all tests and commit**

Run: `go test ./pkg/apikeys/... -v`
Expected: All tests pass

```bash
git add pkg/apikeys/service.go pkg/apikeys/service_test.go
git commit -m "[API Keys] Add last accessed tracking"
```

---

## Task 7: API Key Handlers

**Files:**
- Create: `pkg/apikeys/handlers.go`
- Create: `pkg/apikeys/handlers_test.go`

**Step 1: Create handler struct and List handler**

```go
package apikeys

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"shisho/pkg/auth"
	"shisho/pkg/errcodes"
)

type handler struct {
	service *Service
}

func newHandler(service *Service) *handler {
	return &handler{service: service}
}

// List returns all API keys for the current user
func (h *handler) List(c echo.Context) error {
	user := auth.GetUserFromContext(c.Request().Context())
	if user == nil {
		return errcodes.NewError(http.StatusUnauthorized, "unauthorized", "User not authenticated")
	}

	keys, err := h.service.List(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, keys)
}
```

**Step 2: Write test for List handler**

```go
package apikeys

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shisho/pkg/auth"
	"shisho/pkg/database"
	"shisho/pkg/models"
)

func setupTestHandler(t *testing.T) (*handler, *echo.Echo) {
	db := database.NewTestDB(t)
	svc := NewService(db)
	h := newHandler(svc)
	e := echo.New()
	return h, e
}

func TestHandler_List(t *testing.T) {
	h, e := setupTestHandler(t)
	ctx := context.Background()

	// Create test user
	user := &models.User{ID: 1, Username: "testuser"}
	_, err := h.service.db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create an API key
	_, err = h.service.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/user/api-keys", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetRequest(req.WithContext(auth.ContextWithUser(req.Context(), user)))

	// Call handler
	err = h.List(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)

	var keys []*ApiKey
	err = json.Unmarshal(rec.Body.Bytes(), &keys)
	require.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Equal(t, "My Kobo", keys[0].Name)
}
```

**Step 3: Run test to verify it passes**

Run: `go test ./pkg/apikeys/... -run TestHandler_List -v`
Expected: PASS

**Step 4: Add Create handler**

Add to `handlers.go`:

```go
type CreateRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}

// Create creates a new API key for the current user
func (h *handler) Create(c echo.Context) error {
	user := auth.GetUserFromContext(c.Request().Context())
	if user == nil {
		return errcodes.NewError(http.StatusUnauthorized, "unauthorized", "User not authenticated")
	}

	var req CreateRequest
	if err := c.Bind(&req); err != nil {
		return errcodes.NewError(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if err := c.Validate(&req); err != nil {
		return err
	}

	apiKey, err := h.service.Create(c.Request().Context(), user.ID, req.Name)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, apiKey)
}
```

**Step 5: Add UpdateName handler**

Add to `handlers.go`:

```go
type UpdateNameRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}

// UpdateName updates an API key's name
func (h *handler) UpdateName(c echo.Context) error {
	user := auth.GetUserFromContext(c.Request().Context())
	if user == nil {
		return errcodes.NewError(http.StatusUnauthorized, "unauthorized", "User not authenticated")
	}

	keyID := c.Param("id")
	if keyID == "" {
		return errcodes.NewError(http.StatusBadRequest, "invalid_request", "Key ID required")
	}

	var req UpdateNameRequest
	if err := c.Bind(&req); err != nil {
		return errcodes.NewError(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if err := c.Validate(&req); err != nil {
		return err
	}

	apiKey, err := h.service.UpdateName(c.Request().Context(), user.ID, keyID, req.Name)
	if err != nil {
		if err == ErrNotFound {
			return errcodes.NewError(http.StatusNotFound, "not_found", "API key not found")
		}
		return err
	}

	return c.JSON(http.StatusOK, apiKey)
}
```

**Step 6: Add Delete handler**

Add to `handlers.go`:

```go
// Delete deletes an API key
func (h *handler) Delete(c echo.Context) error {
	user := auth.GetUserFromContext(c.Request().Context())
	if user == nil {
		return errcodes.NewError(http.StatusUnauthorized, "unauthorized", "User not authenticated")
	}

	keyID := c.Param("id")
	if keyID == "" {
		return errcodes.NewError(http.StatusBadRequest, "invalid_request", "Key ID required")
	}

	err := h.service.Delete(c.Request().Context(), user.ID, keyID)
	if err != nil {
		if err == ErrNotFound {
			return errcodes.NewError(http.StatusNotFound, "not_found", "API key not found")
		}
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
```

**Step 7: Add Permission handlers**

Add to `handlers.go`:

```go
// AddPermission adds a permission to an API key
func (h *handler) AddPermission(c echo.Context) error {
	user := auth.GetUserFromContext(c.Request().Context())
	if user == nil {
		return errcodes.NewError(http.StatusUnauthorized, "unauthorized", "User not authenticated")
	}

	keyID := c.Param("id")
	permission := c.Param("permission")
	if keyID == "" || permission == "" {
		return errcodes.NewError(http.StatusBadRequest, "invalid_request", "Key ID and permission required")
	}

	apiKey, err := h.service.AddPermission(c.Request().Context(), user.ID, keyID, permission)
	if err != nil {
		if err == ErrNotFound {
			return errcodes.NewError(http.StatusNotFound, "not_found", "API key not found")
		}
		return err
	}

	return c.JSON(http.StatusOK, apiKey)
}

// RemovePermission removes a permission from an API key
func (h *handler) RemovePermission(c echo.Context) error {
	user := auth.GetUserFromContext(c.Request().Context())
	if user == nil {
		return errcodes.NewError(http.StatusUnauthorized, "unauthorized", "User not authenticated")
	}

	keyID := c.Param("id")
	permission := c.Param("permission")
	if keyID == "" || permission == "" {
		return errcodes.NewError(http.StatusBadRequest, "invalid_request", "Key ID and permission required")
	}

	apiKey, err := h.service.RemovePermission(c.Request().Context(), user.ID, keyID, permission)
	if err != nil {
		if err == ErrNotFound {
			return errcodes.NewError(http.StatusNotFound, "not_found", "API key not found")
		}
		return err
	}

	return c.JSON(http.StatusOK, apiKey)
}
```

**Step 8: Add GenerateShortUrl handler**

Add to `handlers.go`:

```go
// GenerateShortUrl creates a temporary short URL for an API key
func (h *handler) GenerateShortUrl(c echo.Context) error {
	user := auth.GetUserFromContext(c.Request().Context())
	if user == nil {
		return errcodes.NewError(http.StatusUnauthorized, "unauthorized", "User not authenticated")
	}

	keyID := c.Param("id")
	if keyID == "" {
		return errcodes.NewError(http.StatusBadRequest, "invalid_request", "Key ID required")
	}

	shortUrl, err := h.service.GenerateShortUrl(c.Request().Context(), user.ID, keyID)
	if err != nil {
		if err == ErrNotFound {
			return errcodes.NewError(http.StatusNotFound, "not_found", "API key not found")
		}
		return err
	}

	return c.JSON(http.StatusCreated, shortUrl)
}
```

**Step 9: Run all tests and commit**

Run: `go test ./pkg/apikeys/... -v`
Expected: All tests pass

```bash
git add pkg/apikeys/handlers.go pkg/apikeys/handlers_test.go
git commit -m "[API Keys] Add HTTP handlers"
```

---

## Task 8: API Key Routes

**Files:**
- Create: `pkg/apikeys/routes.go`
- Modify: `pkg/server/server.go`

**Step 1: Create routes file**

```go
package apikeys

import (
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"

	"shisho/pkg/auth"
)

// RegisterRoutes registers API key management routes
func RegisterRoutes(e *echo.Echo, db *bun.DB, authMiddleware *auth.Middleware) {
	service := NewService(db)
	h := newHandler(service)

	// All routes require authentication
	g := e.Group("/user/api-keys", authMiddleware.Authenticate)

	g.GET("", h.List)
	g.POST("", h.Create)
	g.PATCH("/:id", h.UpdateName)
	g.DELETE("/:id", h.Delete)
	g.POST("/:id/permissions/:permission", h.AddPermission)
	g.DELETE("/:id/permissions/:permission", h.RemovePermission)
	g.POST("/:id/short-url", h.GenerateShortUrl)
}
```

**Step 2: Register routes in server.go**

Add to the imports in `pkg/server/server.go`:

```go
import (
	// ... existing imports
	"shisho/pkg/apikeys"
)
```

Add in the `New` function after other route registrations:

```go
// API Keys routes
apikeys.RegisterRoutes(e, db, authMiddleware)
```

**Step 3: Run the build to verify**

Run: `make build`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add pkg/apikeys/routes.go pkg/server/server.go
git commit -m "[API Keys] Register routes in server"
```

---

## Task 9: eReader Package - Templates

**Files:**
- Create: `pkg/ereader/templates.go`

**Step 1: Create HTML templates**

```go
package ereader

import (
	"fmt"
	"html"
	"strings"
)

const baseTemplate = `<!DOCTYPE html>
<html>
<head>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Shisho</title>
  <style>
    body { font-family: sans-serif; margin: 8px; }
    a { color: #000; text-decoration: underline; }
    .item { padding: 12px 0; border-bottom: 1px solid #ccc; }
    .item-title { font-size: 1.1em; font-weight: bold; }
    .item-meta { font-size: 0.9em; color: #666; }
    .nav { margin: 16px 0; }
    .filter { margin-bottom: 12px; }
    form { margin: 16px 0; }
    input[type="text"] { font-size: 16px; padding: 8px; width: 80%%; }
    button { font-size: 16px; padding: 8px 16px; }
  </style>
</head>
<body>
  %s
</body>
</html>`

// Navigation bar template
func navBar(backURL, homeURL string) string {
	var parts []string
	if backURL != "" {
		parts = append(parts, fmt.Sprintf(`<a href="%s">← Back</a>`, html.EscapeString(backURL)))
	}
	if homeURL != "" {
		parts = append(parts, fmt.Sprintf(`<a href="%s">Home</a>`, html.EscapeString(homeURL)))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf(`<div class="nav">%s</div>`, strings.Join(parts, " | "))
}

// Pagination template
func pagination(currentPage, totalPages int, baseURL string) string {
	if totalPages <= 1 {
		return ""
	}

	var parts []string
	if currentPage > 1 {
		parts = append(parts, fmt.Sprintf(`<a href="%s?page=%d">← Prev</a>`, baseURL, currentPage-1))
	} else {
		parts = append(parts, "← Prev")
	}

	parts = append(parts, fmt.Sprintf("Page %d of %d", currentPage, totalPages))

	if currentPage < totalPages {
		parts = append(parts, fmt.Sprintf(`<a href="%s?page=%d">Next →</a>`, baseURL, currentPage+1))
	} else {
		parts = append(parts, "Next →")
	}

	return fmt.Sprintf(`<div class="nav">%s</div>`, strings.Join(parts, " | "))
}

// Item template for lists
func itemHTML(title, url, meta string) string {
	return fmt.Sprintf(`<div class="item">
  <div class="item-title"><a href="%s">%s</a></div>
  <div class="item-meta">%s</div>
</div>`, html.EscapeString(url), html.EscapeString(title), html.EscapeString(meta))
}

// Search form template
func searchForm(actionURL, query string) string {
	return fmt.Sprintf(`<form action="%s" method="get">
  <input type="text" name="q" value="%s" placeholder="Search...">
  <button type="submit">Search</button>
</form>`, html.EscapeString(actionURL), html.EscapeString(query))
}

// RenderPage wraps content in the base template
func RenderPage(content string) string {
	return fmt.Sprintf(baseTemplate, content)
}
```

**Step 2: Commit**

```bash
git add pkg/ereader/templates.go
git commit -m "[eReader] Add HTML templates"
```

---

## Task 10: eReader Package - Middleware

**Files:**
- Create: `pkg/ereader/middleware.go`
- Create: `pkg/ereader/middleware_test.go`

**Step 1: Write the test for API key middleware**

```go
package ereader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shisho/pkg/apikeys"
	"shisho/pkg/database"
)

func TestMiddleware_ApiKeyAuth(t *testing.T) {
	db := database.NewTestDB(t)
	apiKeyService := apikeys.NewService(db)
	mw := NewMiddleware(apiKeyService)
	ctx := context.Background()

	// Create test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create API key with permission
	apiKey, err := apiKeyService.Create(ctx, 1, "Test Key")
	require.NoError(t, err)
	apiKey, err = apiKeyService.AddPermission(ctx, 1, apiKey.ID, apikeys.PermissionEReaderBrowser)
	require.NoError(t, err)

	e := echo.New()

	t.Run("valid key with permission", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ereader/key/"+apiKey.Key+"/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/ereader/key/:apiKey/*")
		c.SetParamNames("apiKey")
		c.SetParamValues(apiKey.Key)

		handler := mw.ApiKeyAuth(apikeys.PermissionEReaderBrowser)(func(c echo.Context) error {
			return c.String(http.StatusOK, "success")
		})

		err := handler(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("invalid key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ereader/key/invalid/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/ereader/key/:apiKey/*")
		c.SetParamNames("apiKey")
		c.SetParamValues("invalid")

		handler := mw.ApiKeyAuth(apikeys.PermissionEReaderBrowser)(func(c echo.Context) error {
			return c.String(http.StatusOK, "success")
		})

		err := handler(c)
		assert.Error(t, err)
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/ereader/... -run TestMiddleware_ApiKeyAuth -v`
Expected: FAIL - middleware not implemented

**Step 3: Implement middleware**

```go
package ereader

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"shisho/pkg/apikeys"
	"shisho/pkg/errcodes"
	"shisho/pkg/models"
)

type contextKey string

const (
	ContextKeyApiKey contextKey = "api_key"
	ContextKeyUser   contextKey = "user"
)

type Middleware struct {
	apiKeyService *apikeys.Service
}

func NewMiddleware(apiKeyService *apikeys.Service) *Middleware {
	return &Middleware{apiKeyService: apiKeyService}
}

// ApiKeyAuth validates the API key from the URL path and checks for required permission
func (m *Middleware) ApiKeyAuth(requiredPermission string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			apiKeyValue := c.Param("apiKey")
			if apiKeyValue == "" {
				return errcodes.NewError(http.StatusUnauthorized, "unauthorized", "API key required")
			}

			apiKey, err := m.apiKeyService.GetByKey(c.Request().Context(), apiKeyValue)
			if err != nil {
				return err
			}
			if apiKey == nil {
				return errcodes.NewError(http.StatusUnauthorized, "unauthorized", "Invalid API key")
			}

			if !apiKey.HasPermission(requiredPermission) {
				return errcodes.NewError(http.StatusForbidden, "forbidden", "API key lacks required permission")
			}

			// Touch last accessed (fire and forget)
			go func() {
				_ = m.apiKeyService.TouchLastAccessed(context.Background(), apiKey.ID)
			}()

			// Store API key in context
			ctx := context.WithValue(c.Request().Context(), ContextKeyApiKey, apiKey)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}

// GetApiKeyFromContext retrieves the API key from context
func GetApiKeyFromContext(ctx context.Context) *apikeys.ApiKey {
	if apiKey, ok := ctx.Value(ContextKeyApiKey).(*apikeys.ApiKey); ok {
		return apiKey
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/ereader/... -run TestMiddleware_ApiKeyAuth -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/ereader/middleware.go pkg/ereader/middleware_test.go
git commit -m "[eReader] Add API key authentication middleware"
```

---

## Task 11: eReader Package - Handlers (Libraries List)

**Files:**
- Create: `pkg/ereader/handlers.go`

**Step 1: Create handler struct and libraries list**

```go
package ereader

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"

	"shisho/pkg/apikeys"
	"shisho/pkg/books"
	"shisho/pkg/downloadcache"
	"shisho/pkg/opds"
)

type handler struct {
	db            *bun.DB
	opdsService   *opds.Service
	bookService   *books.Service
	downloadCache *downloadcache.Cache
}

func newHandler(db *bun.DB, opdsService *opds.Service, bookService *books.Service, downloadCache *downloadcache.Cache) *handler {
	return &handler{
		db:            db,
		opdsService:   opdsService,
		bookService:   bookService,
		downloadCache: downloadCache,
	}
}

func (h *handler) baseURL(c echo.Context) string {
	apiKey := c.Param("apiKey")
	return "/ereader/key/" + apiKey
}

// Libraries lists all libraries the user has access to
func (h *handler) Libraries(c echo.Context) error {
	ctx := c.Request().Context()
	apiKey := GetApiKeyFromContext(ctx)
	if apiKey == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "API key not found")
	}

	// Get user to check library access
	var user struct {
		ID int `bun:"id"`
	}
	err := h.db.NewSelect().
		TableExpr("users").
		Column("id").
		Where("id = ?", apiKey.UserID).
		Scan(ctx, &user)
	if err != nil {
		return err
	}

	// Get accessible libraries
	feed, err := h.opdsService.BuildCatalogFeed(ctx, apiKey.UserID)
	if err != nil {
		return err
	}

	var content strings.Builder
	content.WriteString(navBar("", ""))
	content.WriteString("<h1>Libraries</h1>")

	for _, entry := range feed.Entries {
		libraryURL := fmt.Sprintf("%s/libraries/%s", h.baseURL(c), entry.ID)
		content.WriteString(itemHTML(entry.Title, libraryURL, ""))
	}

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}
```

**Step 2: Commit**

```bash
git add pkg/ereader/handlers.go
git commit -m "[eReader] Add libraries list handler"
```

---

## Task 12: eReader Package - Handlers (Library Navigation)

**Files:**
- Modify: `pkg/ereader/handlers.go`

**Step 1: Add library navigation handler**

Add to `handlers.go`:

```go
import (
	"strconv"
	// ... existing imports
)

// LibraryNav shows navigation options for a library
func (h *handler) LibraryNav(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)

	var content strings.Builder
	content.WriteString(navBar(baseURL+"/", baseURL+"/"))
	content.WriteString(fmt.Sprintf("<h1>Library</h1>"))

	// Navigation options
	content.WriteString(itemHTML("All Books", fmt.Sprintf("%s/libraries/%s/all", baseURL, libraryID), "Browse all books"))
	content.WriteString(itemHTML("Series", fmt.Sprintf("%s/libraries/%s/series", baseURL, libraryID), "Browse by series"))
	content.WriteString(itemHTML("Authors", fmt.Sprintf("%s/libraries/%s/authors", baseURL, libraryID), "Browse by author"))
	content.WriteString(itemHTML("Search", fmt.Sprintf("%s/libraries/%s/search", baseURL, libraryID), "Search for books"))

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}
```

**Step 2: Commit**

```bash
git add pkg/ereader/handlers.go
git commit -m "[eReader] Add library navigation handler"
```

---

## Task 13: eReader Package - Handlers (All Books with Pagination)

**Files:**
- Modify: `pkg/ereader/handlers.go`

**Step 1: Add all books handler with pagination**

Add to `handlers.go`:

```go
const defaultPageSize = 50

// LibraryAllBooks shows paginated list of all books
func (h *handler) LibraryAllBooks(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)
	apiKey := GetApiKeyFromContext(ctx)

	// Parse page number
	page := 1
	if pageStr := c.QueryParam("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	libraryIDInt, err := strconv.Atoi(libraryID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid library ID")
	}

	// Get books feed
	feed, err := h.opdsService.BuildLibraryAllBooksFeed(ctx, apiKey.UserID, libraryIDInt, page, defaultPageSize)
	if err != nil {
		return err
	}

	var content strings.Builder
	libNavURL := fmt.Sprintf("%s/libraries/%s", baseURL, libraryID)
	content.WriteString(navBar(libNavURL, baseURL+"/"))
	content.WriteString("<h1>All Books</h1>")

	for _, entry := range feed.Entries {
		meta := formatBookMeta(entry)
		bookURL := fmt.Sprintf("%s/download/%s", baseURL, entry.ID)
		content.WriteString(itemHTML(entry.Title, bookURL, meta))
	}

	// Pagination
	totalPages := (feed.TotalResults + defaultPageSize - 1) / defaultPageSize
	currentURL := fmt.Sprintf("%s/libraries/%s/all", baseURL, libraryID)
	content.WriteString(pagination(page, totalPages, currentURL))

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}

func formatBookMeta(entry opds.Entry) string {
	var parts []string
	if entry.Author.Name != "" {
		parts = append(parts, entry.Author.Name)
	}
	// Format could be in entry content or links
	return strings.Join(parts, " • ")
}
```

**Step 2: Commit**

```bash
git add pkg/ereader/handlers.go
git commit -m "[eReader] Add all books handler with pagination"
```

---

## Task 14: eReader Package - Handlers (Series and Authors)

**Files:**
- Modify: `pkg/ereader/handlers.go`

**Step 1: Add series list and books handlers**

Add to `handlers.go`:

```go
// SeriesList shows all series in a library
func (h *handler) SeriesList(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)
	apiKey := GetApiKeyFromContext(ctx)

	libraryIDInt, err := strconv.Atoi(libraryID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid library ID")
	}

	feed, err := h.opdsService.BuildLibrarySeriesListFeed(ctx, apiKey.UserID, libraryIDInt)
	if err != nil {
		return err
	}

	var content strings.Builder
	libNavURL := fmt.Sprintf("%s/libraries/%s", baseURL, libraryID)
	content.WriteString(navBar(libNavURL, baseURL+"/"))
	content.WriteString("<h1>Series</h1>")

	for _, entry := range feed.Entries {
		seriesURL := fmt.Sprintf("%s/libraries/%s/series/%s", baseURL, libraryID, entry.ID)
		content.WriteString(itemHTML(entry.Title, seriesURL, fmt.Sprintf("%d books", entry.TotalResults)))
	}

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}

// SeriesBooks shows all books in a series
func (h *handler) SeriesBooks(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	seriesID := c.Param("seriesId")
	baseURL := h.baseURL(c)
	apiKey := GetApiKeyFromContext(ctx)

	libraryIDInt, err := strconv.Atoi(libraryID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid library ID")
	}
	seriesIDInt, err := strconv.Atoi(seriesID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid series ID")
	}

	feed, err := h.opdsService.BuildLibrarySeriesBooksFeed(ctx, apiKey.UserID, libraryIDInt, seriesIDInt)
	if err != nil {
		return err
	}

	var content strings.Builder
	seriesListURL := fmt.Sprintf("%s/libraries/%s/series", baseURL, libraryID)
	content.WriteString(navBar(seriesListURL, baseURL+"/"))
	content.WriteString(fmt.Sprintf("<h1>%s</h1>", html.EscapeString(feed.Title)))

	for _, entry := range feed.Entries {
		meta := formatBookMeta(entry)
		bookURL := fmt.Sprintf("%s/download/%s", baseURL, entry.ID)
		content.WriteString(itemHTML(entry.Title, bookURL, meta))
	}

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}
```

Add html import at the top:
```go
import (
	"html"
	// ... other imports
)
```

**Step 2: Add authors list and books handlers**

Add to `handlers.go`:

```go
// AuthorsList shows all authors in a library
func (h *handler) AuthorsList(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)
	apiKey := GetApiKeyFromContext(ctx)

	libraryIDInt, err := strconv.Atoi(libraryID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid library ID")
	}

	feed, err := h.opdsService.BuildLibraryAuthorsListFeed(ctx, apiKey.UserID, libraryIDInt)
	if err != nil {
		return err
	}

	var content strings.Builder
	libNavURL := fmt.Sprintf("%s/libraries/%s", baseURL, libraryID)
	content.WriteString(navBar(libNavURL, baseURL+"/"))
	content.WriteString("<h1>Authors</h1>")

	for _, entry := range feed.Entries {
		// URL encode the author name for the URL
		authorURL := fmt.Sprintf("%s/libraries/%s/authors/%s", baseURL, libraryID, url.PathEscape(entry.ID))
		content.WriteString(itemHTML(entry.Title, authorURL, fmt.Sprintf("%d books", entry.TotalResults)))
	}

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}

// AuthorBooks shows all books by an author
func (h *handler) AuthorBooks(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	authorName := c.Param("authorName")
	baseURL := h.baseURL(c)
	apiKey := GetApiKeyFromContext(ctx)

	libraryIDInt, err := strconv.Atoi(libraryID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid library ID")
	}

	feed, err := h.opdsService.BuildLibraryAuthorBooksFeed(ctx, apiKey.UserID, libraryIDInt, authorName)
	if err != nil {
		return err
	}

	var content strings.Builder
	authorsListURL := fmt.Sprintf("%s/libraries/%s/authors", baseURL, libraryID)
	content.WriteString(navBar(authorsListURL, baseURL+"/"))
	content.WriteString(fmt.Sprintf("<h1>%s</h1>", html.EscapeString(authorName)))

	for _, entry := range feed.Entries {
		meta := formatBookMeta(entry)
		bookURL := fmt.Sprintf("%s/download/%s", baseURL, entry.ID)
		content.WriteString(itemHTML(entry.Title, bookURL, meta))
	}

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}
```

Add url import at the top:
```go
import (
	"net/url"
	// ... other imports
)
```

**Step 3: Commit**

```bash
git add pkg/ereader/handlers.go
git commit -m "[eReader] Add series and authors handlers"
```

---

## Task 15: eReader Package - Handlers (Search)

**Files:**
- Modify: `pkg/ereader/handlers.go`

**Step 1: Add search handler**

Add to `handlers.go`:

```go
// Search shows search form and results
func (h *handler) Search(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)
	apiKey := GetApiKeyFromContext(ctx)
	query := c.QueryParam("q")

	libraryIDInt, err := strconv.Atoi(libraryID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid library ID")
	}

	var content strings.Builder
	libNavURL := fmt.Sprintf("%s/libraries/%s", baseURL, libraryID)
	content.WriteString(navBar(libNavURL, baseURL+"/"))
	content.WriteString("<h1>Search</h1>")

	searchURL := fmt.Sprintf("%s/libraries/%s/search", baseURL, libraryID)
	content.WriteString(searchForm(searchURL, query))

	if query != "" {
		feed, err := h.opdsService.BuildLibrarySearchFeed(ctx, apiKey.UserID, libraryIDInt, query)
		if err != nil {
			return err
		}

		content.WriteString(fmt.Sprintf("<p>Found %d results</p>", len(feed.Entries)))

		for _, entry := range feed.Entries {
			meta := formatBookMeta(entry)
			bookURL := fmt.Sprintf("%s/download/%s", baseURL, entry.ID)
			content.WriteString(itemHTML(entry.Title, bookURL, meta))
		}
	}

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}
```

**Step 2: Commit**

```bash
git add pkg/ereader/handlers.go
git commit -m "[eReader] Add search handler"
```

---

## Task 16: eReader Package - Handlers (Download with Kobo Detection)

**Files:**
- Modify: `pkg/ereader/handlers.go`

**Step 1: Add download handler with Kobo auto-conversion**

Add to `handlers.go`:

```go
import (
	"path/filepath"
	// ... other imports
)

// isKoboUserAgent detects if the request is from a Kobo device
func isKoboUserAgent(userAgent string) bool {
	return strings.Contains(strings.ToLower(userAgent), "kobo")
}

// Download serves a file, auto-converting to KePub for Kobo devices
func (h *handler) Download(c echo.Context) error {
	ctx := c.Request().Context()
	fileID := c.Param("fileId")
	apiKey := GetApiKeyFromContext(ctx)

	fileIDInt, err := strconv.Atoi(fileID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid file ID")
	}

	// Get file and book info
	book, file, err := h.bookService.GetBookAndFileByFileID(ctx, apiKey.UserID, fileIDInt)
	if err != nil {
		return err
	}
	if book == nil || file == nil {
		return echo.NewHTTPError(http.StatusNotFound, "File not found")
	}

	userAgent := c.Request().UserAgent()
	isKobo := isKoboUserAgent(userAgent)

	// Determine if we should convert to KePub
	ext := strings.ToLower(filepath.Ext(file.Path))
	shouldConvertKepub := isKobo && (ext == ".epub" || ext == ".cbz")

	var cachedPath, downloadFilename string
	if shouldConvertKepub {
		cachedPath, downloadFilename, err = h.downloadCache.GetOrGenerateKepub(ctx, book, file)
	} else {
		cachedPath, downloadFilename, err = h.downloadCache.GetOrGenerate(ctx, book, file)
	}
	if err != nil {
		return err
	}

	return c.Attachment(cachedPath, downloadFilename)
}
```

**Step 2: Commit**

```bash
git add pkg/ereader/handlers.go
git commit -m "[eReader] Add download handler with Kobo auto-conversion"
```

---

## Task 17: eReader Package - Short URL Handler

**Files:**
- Modify: `pkg/ereader/handlers.go`

**Step 1: Add short URL resolution handler**

Add to `handlers.go`:

```go
// ResolveShortUrl redirects a short code to the full eReader URL
func (h *handler) ResolveShortUrl(c echo.Context, apiKeyService *apikeys.Service) error {
	shortCode := c.Param("shortCode")

	apiKey, err := apiKeyService.ResolveShortCode(c.Request().Context(), shortCode)
	if err != nil {
		return err
	}
	if apiKey == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Short URL not found or expired")
	}

	redirectURL := fmt.Sprintf("/ereader/key/%s/", apiKey.Key)
	return c.Redirect(http.StatusFound, redirectURL)
}
```

**Step 2: Commit**

```bash
git add pkg/ereader/handlers.go
git commit -m "[eReader] Add short URL resolution handler"
```

---

## Task 18: eReader Package - Routes

**Files:**
- Create: `pkg/ereader/routes.go`
- Modify: `pkg/server/server.go`

**Step 1: Create routes file**

```go
package ereader

import (
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"

	"shisho/pkg/apikeys"
	"shisho/pkg/books"
	"shisho/pkg/config"
	"shisho/pkg/downloadcache"
	"shisho/pkg/opds"
)

// RegisterRoutes registers eReader browser UI routes
func RegisterRoutes(e *echo.Echo, db *bun.DB, cfg *config.Config) {
	apiKeyService := apikeys.NewService(db)
	opdsService := opds.NewService(db)
	bookService := books.NewService(db)
	downloadCache := downloadcache.NewCache(cfg.DownloadCacheDir, cfg.DownloadCacheMaxSizeBytes())

	mw := NewMiddleware(apiKeyService)
	h := newHandler(db, opdsService, bookService, downloadCache)

	// Short URL resolution (no auth required - the short code IS the auth)
	e.GET("/e/:shortCode", func(c echo.Context) error {
		return h.ResolveShortUrl(c, apiKeyService)
	})

	// eReader browser UI with API key auth
	ereader := e.Group("/ereader/key/:apiKey", mw.ApiKeyAuth(apikeys.PermissionEReaderBrowser))

	ereader.GET("/", h.Libraries)
	ereader.GET("/libraries/:libraryId", h.LibraryNav)
	ereader.GET("/libraries/:libraryId/all", h.LibraryAllBooks)
	ereader.GET("/libraries/:libraryId/series", h.SeriesList)
	ereader.GET("/libraries/:libraryId/series/:seriesId", h.SeriesBooks)
	ereader.GET("/libraries/:libraryId/authors", h.AuthorsList)
	ereader.GET("/libraries/:libraryId/authors/:authorName", h.AuthorBooks)
	ereader.GET("/libraries/:libraryId/search", h.Search)
	ereader.GET("/download/:fileId", h.Download)
}
```

**Step 2: Register routes in server.go**

Add to the imports in `pkg/server/server.go`:

```go
import (
	// ... existing imports
	"shisho/pkg/ereader"
)
```

Add in the `New` function after OPDS route registration:

```go
// eReader browser UI routes
ereader.RegisterRoutes(e, db, cfg)
```

**Step 3: Run the build to verify**

Run: `make build`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add pkg/ereader/routes.go pkg/server/server.go
git commit -m "[eReader] Register eReader routes in server"
```

---

## Task 19: Update Vite Proxy Config

**Files:**
- Modify: `vite.config.ts`

**Step 1: Read current vite config**

Run: Read `vite.config.ts` to understand current proxy setup

**Step 2: Add proxy rules for eReader routes**

Add to the proxy configuration in `vite.config.ts`:

```typescript
proxy: {
  '/api': {
    target: 'http://localhost:8080',
    changeOrigin: true,
  },
  '/opds': {
    target: 'http://localhost:8080',
    changeOrigin: true,
  },
  '/e': {
    target: 'http://localhost:8080',
    changeOrigin: true,
  },
  '/ereader': {
    target: 'http://localhost:8080',
    changeOrigin: true,
  },
},
```

**Step 3: Commit**

```bash
git add vite.config.ts
git commit -m "[Config] Add eReader routes to Vite proxy"
```

---

## Task 20: Frontend - API Client for API Keys

**Files:**
- Modify: `app/libraries/api.ts`

**Step 1: Read current API client**

Run: Read `app/libraries/api.ts` to understand current patterns

**Step 2: Add API key types and endpoints**

API key types are auto-generated by tygo. Add endpoint methods to the API class:

```typescript
// In the ShishoAPI class:

// API Keys
listApiKeys(): Promise<ApiKey[]> {
  return this.request("GET", "/user/api-keys");
}

createApiKey(name: string): Promise<ApiKey> {
  return this.request("POST", "/user/api-keys", { name });
}

updateApiKeyName(id: string, name: string): Promise<ApiKey> {
  return this.request("PATCH", `/user/api-keys/${id}`, { name });
}

deleteApiKey(id: string): Promise<void> {
  return this.request("DELETE", `/user/api-keys/${id}`);
}

addApiKeyPermission(id: string, permission: string): Promise<ApiKey> {
  return this.request("POST", `/user/api-keys/${id}/permissions/${permission}`);
}

removeApiKeyPermission(id: string, permission: string): Promise<ApiKey> {
  return this.request("DELETE", `/user/api-keys/${id}/permissions/${permission}`);
}

generateApiKeyShortUrl(id: string): Promise<ApiKeyShortUrl> {
  return this.request("POST", `/user/api-keys/${id}/short-url`);
}
```

**Step 3: Commit**

```bash
git add app/libraries/api.ts
git commit -m "[Frontend] Add API key endpoints to API client"
```

---

## Task 21: Frontend - API Key Query Hooks

**Files:**
- Create: `app/hooks/queries/apiKeys.ts`

**Step 1: Create query hooks file**

```typescript
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { API, ShishoAPIError } from "@/libraries/api";
import type { ApiKey, ApiKeyShortUrl } from "@/types/generated/apikeys";

export enum QueryKey {
  ListApiKeys = "ListApiKeys",
}

export const useApiKeys = () => {
  return useQuery<ApiKey[], ShishoAPIError>({
    queryKey: [QueryKey.ListApiKeys],
    queryFn: () => API.listApiKeys(),
  });
};

export const useCreateApiKey = () => {
  const queryClient = useQueryClient();
  return useMutation<ApiKey, ShishoAPIError, string>({
    mutationFn: (name) => API.createApiKey(name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListApiKeys] });
    },
  });
};

export const useUpdateApiKeyName = () => {
  const queryClient = useQueryClient();
  return useMutation<ApiKey, ShishoAPIError, { id: string; name: string }>({
    mutationFn: ({ id, name }) => API.updateApiKeyName(id, name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListApiKeys] });
    },
  });
};

export const useDeleteApiKey = () => {
  const queryClient = useQueryClient();
  return useMutation<void, ShishoAPIError, string>({
    mutationFn: (id) => API.deleteApiKey(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListApiKeys] });
    },
  });
};

export const useAddApiKeyPermission = () => {
  const queryClient = useQueryClient();
  return useMutation<ApiKey, ShishoAPIError, { id: string; permission: string }>({
    mutationFn: ({ id, permission }) => API.addApiKeyPermission(id, permission),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListApiKeys] });
    },
  });
};

export const useRemoveApiKeyPermission = () => {
  const queryClient = useQueryClient();
  return useMutation<ApiKey, ShishoAPIError, { id: string; permission: string }>({
    mutationFn: ({ id, permission }) => API.removeApiKeyPermission(id, permission),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListApiKeys] });
    },
  });
};

export const useGenerateShortUrl = () => {
  return useMutation<ApiKeyShortUrl, ShishoAPIError, string>({
    mutationFn: (id) => API.generateApiKeyShortUrl(id),
  });
};
```

**Step 2: Commit**

```bash
git add app/hooks/queries/apiKeys.ts
git commit -m "[Frontend] Add API key query hooks"
```

---

## Task 22: Frontend - Security Settings Page

**Files:**
- Create: `app/components/pages/SecuritySettings.tsx`
- Modify: `app/router.tsx`

**Step 1: Create Security Settings page**

```typescript
import { useState } from "react";
import { toast } from "sonner";
import { Eye, EyeOff, Copy, Trash2, Key, ExternalLink } from "lucide-react";

import { TopNav } from "@/components/shared/TopNav";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

import { useResetPassword } from "@/hooks/useAuth";
import {
  useApiKeys,
  useCreateApiKey,
  useDeleteApiKey,
  useAddApiKeyPermission,
  useRemoveApiKeyPermission,
  useGenerateShortUrl,
} from "@/hooks/queries/apiKeys";
import type { ApiKey } from "@/types/generated/apikeys";

const PERMISSION_EREADER_BROWSER = "ereader_browser";

export function SecuritySettings() {
  return (
    <>
      <TopNav title="Security Settings" />
      <div className="container max-w-2xl py-8 space-y-8">
        <ChangePasswordSection />
        <Separator />
        <ApiKeysSection />
      </div>
    </>
  );
}

function ChangePasswordSection() {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const resetPassword = useResetPassword();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (newPassword !== confirmPassword) {
      toast.error("Passwords do not match");
      return;
    }
    try {
      await resetPassword.mutateAsync({ currentPassword, newPassword });
      toast.success("Password updated");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    } catch {
      toast.error("Failed to update password");
    }
  };

  return (
    <div>
      <h2 className="text-lg font-semibold mb-4">Change Password</h2>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <Label htmlFor="current-password">Current Password</Label>
          <Input
            id="current-password"
            type="password"
            value={currentPassword}
            onChange={(e) => setCurrentPassword(e.target.value)}
          />
        </div>
        <div>
          <Label htmlFor="new-password">New Password</Label>
          <Input
            id="new-password"
            type="password"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
          />
        </div>
        <div>
          <Label htmlFor="confirm-password">Confirm New Password</Label>
          <Input
            id="confirm-password"
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
          />
        </div>
        <Button type="submit" disabled={resetPassword.isPending}>
          Update Password
        </Button>
      </form>
    </div>
  );
}

function ApiKeysSection() {
  const { data: apiKeys, isLoading } = useApiKeys();
  const [createDialogOpen, setCreateDialogOpen] = useState(false);

  if (isLoading) {
    return <div>Loading...</div>;
  }

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h2 className="text-lg font-semibold">API Keys</h2>
        <CreateApiKeyDialog open={createDialogOpen} onOpenChange={setCreateDialogOpen} />
      </div>
      <div className="space-y-4">
        {apiKeys?.map((key) => (
          <ApiKeyCard key={key.id} apiKey={key} />
        ))}
        {apiKeys?.length === 0 && (
          <p className="text-muted-foreground">No API keys yet. Create one to access your library from an eReader.</p>
        )}
      </div>
    </div>
  );
}

function CreateApiKeyDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [name, setName] = useState("");
  const createApiKey = useCreateApiKey();
  const addPermission = useAddApiKeyPermission();

  const handleCreate = async () => {
    try {
      const key = await createApiKey.mutateAsync(name);
      // Auto-add ereader_browser permission
      await addPermission.mutateAsync({ id: key.id, permission: PERMISSION_EREADER_BROWSER });
      toast.success("API key created");
      setName("");
      onOpenChange(false);
    } catch {
      toast.error("Failed to create API key");
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm">
          <Key className="w-4 h-4 mr-2" />
          Create API Key
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create API Key</DialogTitle>
          <DialogDescription>
            Give your API key a name to help you identify it later.
          </DialogDescription>
        </DialogHeader>
        <div className="py-4">
          <Label htmlFor="key-name">Name</Label>
          <Input
            id="key-name"
            placeholder="e.g., Kobo Libra"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={handleCreate} disabled={!name || createApiKey.isPending}>
            Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ApiKeyCard({ apiKey }: { apiKey: ApiKey }) {
  const [showKey, setShowKey] = useState(false);
  const [setupDialogOpen, setSetupDialogOpen] = useState(false);
  const deleteApiKey = useDeleteApiKey();
  const addPermission = useAddApiKeyPermission();
  const removePermission = useRemoveApiKeyPermission();

  const hasEReaderPermission = apiKey.permissions?.some(
    (p) => p.permission === PERMISSION_EREADER_BROWSER
  );

  const handleCopy = () => {
    navigator.clipboard.writeText(apiKey.key);
    toast.success("Copied to clipboard");
  };

  const handleDelete = async () => {
    if (!confirm("Delete this API key? This cannot be undone.")) return;
    try {
      await deleteApiKey.mutateAsync(apiKey.id);
      toast.success("API key deleted");
    } catch {
      toast.error("Failed to delete API key");
    }
  };

  const handlePermissionChange = async (checked: boolean) => {
    try {
      if (checked) {
        await addPermission.mutateAsync({ id: apiKey.id, permission: PERMISSION_EREADER_BROWSER });
      } else {
        await removePermission.mutateAsync({ id: apiKey.id, permission: PERMISSION_EREADER_BROWSER });
      }
    } catch {
      toast.error("Failed to update permission");
    }
  };

  const maskedKey = apiKey.key.slice(0, 7) + "•".repeat(20);
  const lastUsed = apiKey.lastAccessedAt
    ? `Last used: ${new Date(apiKey.lastAccessedAt).toLocaleDateString()}`
    : "Never used";

  return (
    <div className="border rounded-lg p-4 space-y-3">
      <div className="flex justify-between items-start">
        <div>
          <div className="font-medium">{apiKey.name}</div>
          <div className="text-sm text-muted-foreground">{lastUsed}</div>
        </div>
        <div className="flex gap-1">
          <Button variant="ghost" size="icon" onClick={() => setShowKey(!showKey)}>
            {showKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
          </Button>
          <Button variant="ghost" size="icon" onClick={handleCopy}>
            <Copy className="w-4 h-4" />
          </Button>
          {hasEReaderPermission && (
            <SetupDialog apiKey={apiKey} open={setupDialogOpen} onOpenChange={setSetupDialogOpen} />
          )}
          <Button variant="ghost" size="icon" onClick={handleDelete}>
            <Trash2 className="w-4 h-4" />
          </Button>
        </div>
      </div>
      <div className="font-mono text-sm bg-muted p-2 rounded">
        {showKey ? apiKey.key : maskedKey}
      </div>
      <div className="flex items-center space-x-2">
        <Checkbox
          id={`perm-${apiKey.id}`}
          checked={hasEReaderPermission}
          onCheckedChange={handlePermissionChange}
        />
        <Label htmlFor={`perm-${apiKey.id}`}>eReader Browser Access</Label>
      </div>
    </div>
  );
}

function SetupDialog({ apiKey, open, onOpenChange }: { apiKey: ApiKey; open: boolean; onOpenChange: (open: boolean) => void }) {
  const generateShortUrl = useGenerateShortUrl();
  const [shortUrl, setShortUrl] = useState<string | null>(null);
  const [expiresAt, setExpiresAt] = useState<Date | null>(null);

  const handleOpen = async (isOpen: boolean) => {
    onOpenChange(isOpen);
    if (isOpen && !shortUrl) {
      try {
        const result = await generateShortUrl.mutateAsync(apiKey.id);
        const baseUrl = window.location.origin;
        setShortUrl(`${baseUrl}/e/${result.shortCode}`);
        setExpiresAt(new Date(result.expiresAt));
      } catch {
        toast.error("Failed to generate setup URL");
      }
    }
  };

  const handleCopy = () => {
    if (shortUrl) {
      navigator.clipboard.writeText(shortUrl);
      toast.success("Copied to clipboard");
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpen}>
      <DialogTrigger asChild>
        <Button variant="ghost" size="icon">
          <ExternalLink className="w-4 h-4" />
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>eReader Setup</DialogTitle>
          <DialogDescription>
            Enter this URL on your eReader's web browser, then bookmark the page.
          </DialogDescription>
        </DialogHeader>
        {generateShortUrl.isPending ? (
          <div className="py-4">Generating URL...</div>
        ) : shortUrl ? (
          <div className="py-4 space-y-4">
            <div className="flex gap-2">
              <Input value={shortUrl} readOnly className="font-mono" />
              <Button variant="outline" onClick={handleCopy}>
                <Copy className="w-4 h-4" />
              </Button>
            </div>
            <p className="text-sm text-muted-foreground">
              This URL expires in 30 minutes. After opening it on your eReader, bookmark the page to access your library anytime.
            </p>
          </div>
        ) : null}
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

**Step 2: Add route to router.tsx**

Add import and route:

```typescript
import { SecuritySettings } from "@/components/pages/SecuritySettings";

// In the routes array, add:
{
  path: "user/security",
  element: <SecuritySettings />,
}
```

**Step 3: Run lint to check for issues**

Run: `yarn lint`
Expected: No errors (or fix any that appear)

**Step 4: Commit**

```bash
git add app/components/pages/SecuritySettings.tsx app/router.tsx
git commit -m "[Frontend] Add Security Settings page with API key management"
```

---

## Task 23: Frontend - Add Security Settings Link to User Popover

**Files:**
- Modify: `app/components/shared/UserPopover.tsx` (or wherever the user menu is)

**Step 1: Find and read the user menu component**

Run: Find the component that shows the user popover/menu (likely in TopNav or UserPopover)

**Step 2: Add Security Settings link**

Add a link to `/user/security` in the user menu, near the existing Settings link.

**Step 3: Commit**

```bash
git add app/components/shared/UserPopover.tsx
git commit -m "[Frontend] Add Security Settings link to user menu"
```

---

## Task 24: Move Password Change from UserSettings

**Files:**
- Modify: `app/components/pages/UserSettings.tsx`

**Step 1: Read current UserSettings**

Run: Read `app/components/pages/UserSettings.tsx`

**Step 2: Remove password change section**

Remove the password change form from UserSettings since it's now in SecuritySettings. Add a link to the Security Settings page instead.

**Step 3: Commit**

```bash
git add app/components/pages/UserSettings.tsx
git commit -m "[Frontend] Move password change to Security Settings"
```

---

## Task 25: Add tygo configuration for apikeys package

**Files:**
- Modify: `tygo.yaml`

**Step 1: Read current tygo config**

Run: Read `tygo.yaml` to understand current structure

**Step 2: Add apikeys package to tygo config**

Add the apikeys package to generate TypeScript types:

```yaml
packages:
  # ... existing packages
  - path: "shisho/pkg/apikeys"
    output_path: "app/types/generated/apikeys.ts"
    type_mappings:
      time.Time: "string"
```

**Step 3: Run tygo to generate types**

Run: `make tygo`
Expected: Types generated successfully

**Step 4: Commit**

```bash
git add tygo.yaml
git commit -m "[Config] Add apikeys to tygo configuration"
```

---

## Task 26: Integration Testing

**Files:** None (manual testing)

**Step 1: Start the development environment**

Run: `make start`

**Step 2: Create an API key via the UI**

1. Navigate to `/user/security`
2. Click "Create API Key"
3. Enter a name and create
4. Verify the key is shown

**Step 3: Test the eReader Setup flow**

1. Click the setup button (external link icon)
2. Copy the short URL
3. Open in a new browser window
4. Verify redirect to eReader UI with libraries listed

**Step 4: Test navigation**

1. Click a library
2. Navigate to All Books, Series, Authors
3. Test pagination
4. Test search

**Step 5: Test download**

1. Click a book to download
2. Verify the file downloads correctly

**Step 6: Commit any fixes**

If any issues found, fix and commit.

---

## Task 27: Run Full Test Suite

**Files:** None

**Step 1: Run all checks**

Run: `make check`
Expected: All tests pass, no lint errors

**Step 2: Fix any issues**

If any tests fail or lint errors, fix and re-run.

**Step 3: Final commit**

```bash
git add -A
git commit -m "[eReader] Complete eReader browser support implementation"
```

---

## Summary

This plan implements the eReader browser support feature in 27 tasks:

1. **Tasks 1-8**: Backend API key system (migrations, models, service, handlers, routes)
2. **Tasks 9-18**: eReader HTML UI package (templates, middleware, handlers for all pages, routes)
3. **Task 19**: Vite proxy configuration
4. **Tasks 20-24**: Frontend (API client, hooks, Security Settings page, navigation)
5. **Task 25**: TypeScript type generation
6. **Tasks 26-27**: Integration testing and verification

Each task follows TDD where appropriate, with frequent commits for easy rollback if needed.
