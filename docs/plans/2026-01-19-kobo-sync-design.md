# Kobo Sync Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement wireless book transfer to Kobo e-readers by impersonating Kobo's sync API, so users can wirelessly sync their Shisho library to Kobo devices.

**Architecture:** A new `pkg/kobo/` package with Echo handlers that impersonate Kobo's store API. The Kobo device's config is modified to point at Shisho instead of `storeapi.kobo.com`. A sync token system using database-persisted sync points enables change detection between syncs. Unhandled requests are proxied to the real Kobo store.

**Tech Stack:** Go/Echo for API handlers, Bun ORM with SQLite for sync state, existing `pkg/kepub/` and `pkg/downloadcache/` for file conversion, `golang.org/x/image/draw` for cover resizing, React/TypeScript/Tanstack Query for frontend setup UI.

---

### Task 1: Database Migration

**Files:**
- Create: `pkg/migrations/20260120000000_create_kobo_sync_tables.go`

**Step 1: Write the migration file**

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			CREATE TABLE kobo_sync_points (
				id TEXT PRIMARY KEY,
				api_key_id TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
				created_at DATETIME NOT NULL,
				completed_at DATETIME
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX idx_kobo_sync_points_api_key ON kobo_sync_points(api_key_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`
			CREATE TABLE kobo_sync_point_books (
				id TEXT PRIMARY KEY,
				sync_point_id TEXT NOT NULL REFERENCES kobo_sync_points(id) ON DELETE CASCADE,
				file_id INTEGER NOT NULL,
				file_hash TEXT NOT NULL,
				file_size INTEGER NOT NULL,
				metadata_hash TEXT NOT NULL,
				synced BOOLEAN NOT NULL DEFAULT FALSE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX idx_sync_point_books_sync_point ON kobo_sync_point_books(sync_point_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX idx_sync_point_books_file ON kobo_sync_point_books(file_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP TABLE IF EXISTS kobo_sync_point_books`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS kobo_sync_points`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

**Step 2: Run migration to verify it works**

Run: `make db:migrate`
Expected: Migration applies successfully with no errors.

**Step 3: Run rollback and re-migrate to verify the down migration**

Run: `make db:rollback && make db:migrate`
Expected: Both rollback and re-migration succeed.

**Step 4: Commit**

```bash
git add pkg/migrations/20260120000000_create_kobo_sync_tables.go
git commit -m "[Kobo] Add database migration for sync points tables"
```

---

### Task 2: Models and Permission Constant

**Files:**
- Create: `pkg/kobo/model.go`
- Modify: `pkg/apikeys/model.go:48` (add permission constant)

**Step 1: Add the `kobo_sync` permission constant to apikeys**

In `pkg/apikeys/model.go`, add after line 48:

```go
// PermissionKoboSync is the permission for Kobo sync API access.
const PermissionKoboSync = "kobo_sync"
```

**Step 2: Create the Kobo models file**

```go
package kobo

import (
	"time"

	"github.com/uptrace/bun"
)

// SyncPoint tracks the state of the library at a sync, per API key.
type SyncPoint struct {
	bun.BaseModel `bun:"table:kobo_sync_points,alias:ksp"`

	ID          string     `bun:"id,pk"`
	APIKeyID    string     `bun:"api_key_id,notnull"`
	CreatedAt   time.Time  `bun:"created_at,notnull"`
	CompletedAt *time.Time `bun:"completed_at"`

	Books []*SyncPointBook `bun:"rel:has-many,join:id=sync_point_id"`
}

// SyncPointBook is a snapshot of a file's state at a sync point.
type SyncPointBook struct {
	bun.BaseModel `bun:"table:kobo_sync_point_books,alias:kspb"`

	ID           string `bun:"id,pk"`
	SyncPointID  string `bun:"sync_point_id,notnull"`
	FileID       int    `bun:"file_id,notnull"`
	FileHash     string `bun:"file_hash,notnull"`
	FileSize     int64  `bun:"file_size,notnull"`
	MetadataHash string `bun:"metadata_hash,notnull"`
	Synced       bool   `bun:"synced,notnull,default:false"`
}

// SyncScope represents the scope of books to sync (parsed from URL).
type SyncScope struct {
	Type      string // "all", "library", "list"
	LibraryID *int
	ListID    *int
}
```

**Step 3: Run tests to verify nothing is broken**

Run: `make test`
Expected: All tests pass.

**Step 4: Commit**

```bash
git add pkg/kobo/model.go pkg/apikeys/model.go
git commit -m "[Kobo] Add sync point models and kobo_sync permission constant"
```

---

### Task 3: Kobo DTO Types (API Response Formats)

**Files:**
- Create: `pkg/kobo/dto.go`

**Step 1: Create the DTO file with Kobo API response types**

```go
package kobo

import "time"

// All types use PascalCase JSON to match Kobo's API expectations.

// SyncResponse is an array of change entries returned by /v1/library/sync.
type SyncResponse []interface{}

// NewEntitlement represents a new or changed book in the sync response.
type NewEntitlement struct {
	NewEntitlement *EntitlementWrapper `json:"NewEntitlement"`
}

// ChangedEntitlement represents a removed book in the sync response.
type ChangedEntitlement struct {
	ChangedEntitlement *EntitlementChangeWrapper `json:"ChangedEntitlement"`
}

// EntitlementWrapper wraps the book entitlement and metadata.
type EntitlementWrapper struct {
	BookEntitlement *BookEntitlement `json:"BookEntitlement"`
	BookMetadata    *BookMetadata    `json:"BookMetadata"`
}

// EntitlementChangeWrapper wraps a change (e.g., removal) to an entitlement.
type EntitlementChangeWrapper struct {
	BookEntitlement *BookEntitlementChange `json:"BookEntitlement"`
}

// BookEntitlement contains the full entitlement info for a book.
type BookEntitlement struct {
	Accessibility     string       `json:"Accessibility"`
	ActivePeriod      *ActivePeriod `json:"ActivePeriod"`
	Created           time.Time    `json:"Created"`
	CrossRevisionId   string       `json:"CrossRevisionId"`
	Id                string       `json:"Id"`
	IsHiddenFromArchive bool       `json:"IsHiddenFromArchive"`
	IsLocked          bool         `json:"IsLocked"`
	IsRemoved         bool         `json:"IsRemoved"`
	LastModified      time.Time    `json:"LastModified"`
	OriginCategory    string       `json:"OriginCategory"`
	RevisionId        string       `json:"RevisionId"`
	Status            string       `json:"Status"`
}

// BookEntitlementChange contains only the fields needed for a change notification.
type BookEntitlementChange struct {
	Id        string `json:"Id"`
	IsRemoved bool   `json:"IsRemoved"`
}

// ActivePeriod indicates when the entitlement became active.
type ActivePeriod struct {
	From time.Time `json:"From"`
}

// BookMetadata contains the metadata for a book visible to the Kobo device.
type BookMetadata struct {
	ContributorRoles []*ContributorRole `json:"ContributorRoles"`
	CoverImageId     string             `json:"CoverImageId"`
	Description      string             `json:"Description"`
	DownloadUrls     []*DownloadURL     `json:"DownloadUrls"`
	EntitlementId    string             `json:"EntitlementId"`
	Language         string             `json:"Language"`
	PublicationDate  string             `json:"PublicationDate"`
	Publisher        *Publisher         `json:"Publisher"`
	Series           *Series           `json:"Series,omitempty"`
	Title            string             `json:"Title"`
}

// ContributorRole represents an author/contributor.
type ContributorRole struct {
	Name string `json:"Name"`
}

// DownloadURL provides the download location for a book file.
type DownloadURL struct {
	Format   string `json:"Format"`
	Platform string `json:"Platform"`
	Size     int64  `json:"Size"`
	Url      string `json:"Url"`
}

// Publisher represents a book publisher.
type Publisher struct {
	Name string `json:"Name"`
}

// Series represents a book series.
type Series struct {
	Name       string  `json:"Name"`
	Number     float64 `json:"Number"`
	NumberFloat float64 `json:"NumberFloat"`
}

// SyncToken is the base64-encoded JSON sent/received in X-Kobo-SyncToken header.
type SyncToken struct {
	LastSyncPointID string `json:"lastSyncPointId"`
}

// DeviceAuthRequest is the body sent by the Kobo to POST /v1/auth/device.
type DeviceAuthRequest struct {
	AffiliateName string `json:"AffiliateName"`
	AppVersion    string `json:"AppVersion"`
	ClientKey     string `json:"ClientKey"`
	DeviceId      string `json:"DeviceId"`
	PlatformId    string `json:"PlatformId"`
}

// DeviceAuthResponse is returned by POST /v1/auth/device.
type DeviceAuthResponse struct {
	AccessToken  string `json:"AccessToken"`
	RefreshToken string `json:"RefreshToken"`
	TokenType    string `json:"TokenType"`
	TrackingId   string `json:"TrackingId"`
	UserKey      string `json:"UserKey"`
}
```

**Step 2: Verify it compiles**

Run: `go build ./pkg/kobo/...`
Expected: No errors.

**Step 3: Commit**

```bash
git add pkg/kobo/dto.go
git commit -m "[Kobo] Add Kobo API DTO types for sync responses"
```

---

### Task 4: Sync Service (Core Business Logic)

**Files:**
- Create: `pkg/kobo/service.go`
- Create: `pkg/kobo/service_test.go`

**Step 1: Write the failing test for sync point creation and change detection**

```go
package kobo

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/database"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	cfg := config.NewForTest()
	db, err := database.New(cfg)
	require.NoError(t, err)
	err = migrations.Run(context.Background(), db.DB)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateSyncPoint(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db.DB)
	ctx := context.Background()

	// Create a sync point with some books
	files := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 1000, MetadataHash: "meta1"},
		{FileID: 2, FileHash: "hash2", FileSize: 2000, MetadataHash: "meta2"},
	}

	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", files)
	require.NoError(t, err)
	assert.NotEmpty(t, sp.ID)
	assert.Equal(t, "api-key-1", sp.APIKeyID)
	assert.NotNil(t, sp.CompletedAt)
	assert.Len(t, sp.Books, 2)
}

func TestDetectChanges_FirstSync(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db.DB)
	ctx := context.Background()

	files := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 1000, MetadataHash: "meta1"},
		{FileID: 2, FileHash: "hash2", FileSize: 2000, MetadataHash: "meta2"},
	}

	changes, err := svc.DetectChanges(ctx, "", files)
	require.NoError(t, err)
	assert.Len(t, changes.Added, 2)
	assert.Empty(t, changes.Removed)
	assert.Empty(t, changes.Changed)
}

func TestDetectChanges_AddedBooks(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db.DB)
	ctx := context.Background()

	// First sync
	files1 := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 1000, MetadataHash: "meta1"},
	}
	sp1, err := svc.CreateSyncPoint(ctx, "api-key-1", files1)
	require.NoError(t, err)

	// Second sync with an added book
	files2 := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 1000, MetadataHash: "meta1"},
		{FileID: 2, FileHash: "hash2", FileSize: 2000, MetadataHash: "meta2"},
	}
	changes, err := svc.DetectChanges(ctx, sp1.ID, files2)
	require.NoError(t, err)
	assert.Len(t, changes.Added, 1)
	assert.Equal(t, 2, changes.Added[0].FileID)
	assert.Empty(t, changes.Removed)
	assert.Empty(t, changes.Changed)
}

func TestDetectChanges_RemovedBooks(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db.DB)
	ctx := context.Background()

	// First sync with 2 books
	files1 := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 1000, MetadataHash: "meta1"},
		{FileID: 2, FileHash: "hash2", FileSize: 2000, MetadataHash: "meta2"},
	}
	sp1, err := svc.CreateSyncPoint(ctx, "api-key-1", files1)
	require.NoError(t, err)

	// Second sync with one removed
	files2 := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 1000, MetadataHash: "meta1"},
	}
	changes, err := svc.DetectChanges(ctx, sp1.ID, files2)
	require.NoError(t, err)
	assert.Empty(t, changes.Added)
	assert.Len(t, changes.Removed, 1)
	assert.Equal(t, 2, changes.Removed[0].FileID)
	assert.Empty(t, changes.Changed)
}

func TestDetectChanges_ChangedBooks(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db.DB)
	ctx := context.Background()

	// First sync
	files1 := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 1000, MetadataHash: "meta1"},
	}
	sp1, err := svc.CreateSyncPoint(ctx, "api-key-1", files1)
	require.NoError(t, err)

	// Second sync with changed hash
	files2 := []ScopedFile{
		{FileID: 1, FileHash: "hash1-v2", FileSize: 1500, MetadataHash: "meta1"},
	}
	changes, err := svc.DetectChanges(ctx, sp1.ID, files2)
	require.NoError(t, err)
	assert.Empty(t, changes.Added)
	assert.Empty(t, changes.Removed)
	assert.Len(t, changes.Changed, 1)
	assert.Equal(t, 1, changes.Changed[0].FileID)
}

func TestDetectChanges_MetadataChange(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db.DB)
	ctx := context.Background()

	// First sync
	files1 := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 1000, MetadataHash: "meta1"},
	}
	sp1, err := svc.CreateSyncPoint(ctx, "api-key-1", files1)
	require.NoError(t, err)

	// Second sync with changed metadata but same file hash
	files2 := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 1000, MetadataHash: "meta1-v2"},
	}
	changes, err := svc.DetectChanges(ctx, sp1.ID, files2)
	require.NoError(t, err)
	assert.Empty(t, changes.Added)
	assert.Empty(t, changes.Removed)
	assert.Len(t, changes.Changed, 1)
}
```

**Step 2: Run tests to verify they fail**

Run: `TZ=America/Chicago CI=true go test ./pkg/kobo/ -v -run TestCreate`
Expected: FAIL (functions don't exist yet)

**Step 3: Write the sync service implementation**

```go
package kobo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// ScopedFile represents a file in the current sync scope with its hashes.
type ScopedFile struct {
	FileID       int
	FileHash     string
	FileSize     int64
	MetadataHash string
}

// SyncChanges contains the detected changes between two sync points.
type SyncChanges struct {
	Added   []ScopedFile
	Removed []ScopedFile
	Changed []ScopedFile
}

// Service provides Kobo sync business logic.
type Service struct {
	db *bun.DB
}

// NewService creates a new Kobo sync service.
func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// CreateSyncPoint creates a new sync point with the given files and marks it complete.
func (svc *Service) CreateSyncPoint(ctx context.Context, apiKeyID string, files []ScopedFile) (*SyncPoint, error) {
	now := time.Now()
	sp := &SyncPoint{
		ID:          uuid.New().String(),
		APIKeyID:    apiKeyID,
		CreatedAt:   now,
		CompletedAt: &now,
	}

	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewInsert().Model(sp).Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		if len(files) > 0 {
			books := make([]*SyncPointBook, len(files))
			for i, f := range files {
				books[i] = &SyncPointBook{
					ID:           uuid.New().String(),
					SyncPointID:  sp.ID,
					FileID:       f.FileID,
					FileHash:     f.FileHash,
					FileSize:     f.FileSize,
					MetadataHash: f.MetadataHash,
					Synced:       false,
				}
			}
			_, err = tx.NewInsert().Model(&books).Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
			sp.Books = books
		}

		return nil
	})

	return sp, errors.WithStack(err)
}

// GetLastSyncPoint retrieves the most recent completed sync point for an API key.
func (svc *Service) GetLastSyncPoint(ctx context.Context, apiKeyID string) (*SyncPoint, error) {
	sp := &SyncPoint{}
	err := svc.db.NewSelect().
		Model(sp).
		Where("api_key_id = ?", apiKeyID).
		Where("completed_at IS NOT NULL").
		Order("created_at DESC").
		Limit(1).
		Relation("Books").
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	return sp, nil
}

// GetSyncPointByID retrieves a sync point by ID.
func (svc *Service) GetSyncPointByID(ctx context.Context, syncPointID string) (*SyncPoint, error) {
	sp := &SyncPoint{}
	err := svc.db.NewSelect().
		Model(sp).
		Where("id = ?", syncPointID).
		Relation("Books").
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	return sp, nil
}

// DetectChanges compares current files against the last sync point.
// If lastSyncPointID is empty, all files are considered new.
func (svc *Service) DetectChanges(ctx context.Context, lastSyncPointID string, currentFiles []ScopedFile) (*SyncChanges, error) {
	changes := &SyncChanges{}

	// First sync: all files are new
	if lastSyncPointID == "" {
		changes.Added = currentFiles
		return changes, nil
	}

	// Get the previous sync point
	prevSP, err := svc.GetSyncPointByID(ctx, lastSyncPointID)
	if err != nil {
		return nil, err
	}
	if prevSP == nil {
		// Previous sync point not found - treat as first sync
		changes.Added = currentFiles
		return changes, nil
	}

	// Build a map of previous files by file ID
	prevFiles := make(map[int]*SyncPointBook, len(prevSP.Books))
	for _, b := range prevSP.Books {
		prevFiles[b.FileID] = b
	}

	// Build a set of current file IDs
	currentFileIDs := make(map[int]bool, len(currentFiles))
	for _, f := range currentFiles {
		currentFileIDs[f.FileID] = true
	}

	// Detect added and changed
	for _, f := range currentFiles {
		prev, exists := prevFiles[f.FileID]
		if !exists {
			changes.Added = append(changes.Added, f)
		} else if prev.FileHash != f.FileHash || prev.MetadataHash != f.MetadataHash {
			changes.Changed = append(changes.Changed, f)
		}
	}

	// Detect removed
	for _, prev := range prevSP.Books {
		if !currentFileIDs[prev.FileID] {
			changes.Removed = append(changes.Removed, ScopedFile{
				FileID:       prev.FileID,
				FileHash:     prev.FileHash,
				FileSize:     prev.FileSize,
				MetadataHash: prev.MetadataHash,
			})
		}
	}

	return changes, nil
}

// GetScopedFiles queries files in scope for the given API key and scope.
func (svc *Service) GetScopedFiles(ctx context.Context, userID int, scope *SyncScope) ([]ScopedFile, error) {
	var files []*models.File

	q := svc.db.NewSelect().
		Model(&files).
		Relation("Book").
		Relation("Book.Authors", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("Person")
		}).
		Relation("Book.BookSeries", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("Series")
		}).
		Relation("Publisher").
		Where("f.file_type IN (?)", bun.In([]string{"epub", "cbz"}))

	switch scope.Type {
	case "all":
		// Get user's accessible library IDs
		var user models.User
		err := svc.db.NewSelect().
			Model(&user).
			Relation("LibraryAccess").
			Where("u.id = ?", userID).
			Scan(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		libraryIDs := user.GetAccessibleLibraryIDs()
		if len(libraryIDs) > 0 {
			q = q.Where("f.library_id IN (?)", bun.In(libraryIDs))
		}
	case "library":
		if scope.LibraryID != nil {
			q = q.Where("f.library_id = ?", *scope.LibraryID)
		}
	case "list":
		if scope.ListID != nil {
			q = q.Join("JOIN list_books lb ON lb.book_id = f.book_id").
				Where("lb.list_id = ?", *scope.ListID)
		}
	}

	err := q.Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	result := make([]ScopedFile, len(files))
	for i, f := range files {
		result[i] = ScopedFile{
			FileID:       f.ID,
			FileHash:     fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d", f.Filepath, f.FilesizeBytes))))[:16],
			FileSize:     f.FilesizeBytes,
			MetadataHash: computeMetadataHash(f),
		}
	}

	return result, nil
}

// computeMetadataHash creates a hash of the metadata fields relevant to the Kobo display.
func computeMetadataHash(file *models.File) string {
	h := sha256.New()
	if file.Book != nil {
		h.Write([]byte(file.Book.Title))
		for _, a := range file.Book.Authors {
			if a.Person != nil {
				h.Write([]byte(a.Person.Name))
			}
		}
	}
	if file.CoverImagePath != nil {
		h.Write([]byte(*file.CoverImagePath))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// ShishoID creates a Kobo-compatible ID prefixed with "shisho-".
func ShishoID(fileID int) string {
	return fmt.Sprintf("shisho-%d", fileID)
}

// ParseShishoID extracts the file ID from a "shisho-{id}" format.
// Returns 0, false if the ID doesn't start with "shisho-".
func ParseShishoID(id string) (int, bool) {
	if !strings.HasPrefix(id, "shisho-") {
		return 0, false
	}
	var fileID int
	_, err := fmt.Sscanf(id, "shisho-%d", &fileID)
	if err != nil {
		return 0, false
	}
	return fileID, true
}
```

**Step 4: Run the tests to verify they pass**

Run: `TZ=America/Chicago CI=true go test ./pkg/kobo/ -v`
Expected: All tests pass.

**Step 5: Commit**

```bash
git add pkg/kobo/service.go pkg/kobo/service_test.go
git commit -m "[Kobo] Add sync service with change detection logic"
```

---

### Task 5: Middleware (API Key Auth + Scope Parsing)

**Files:**
- Create: `pkg/kobo/middleware.go`

**Step 1: Write the middleware**

```go
package kobo

import (
	"context"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

type contextKey string

const (
	contextKeyAPIKey contextKey = "kobo_api_key" //nolint:gosec
	contextKeyScope  contextKey = "kobo_scope"
)

// KoboMiddleware provides authentication and scope middleware for Kobo routes.
type KoboMiddleware struct {
	apiKeyService *apikeys.Service
}

// NewMiddleware creates a new Kobo middleware.
func NewMiddleware(apiKeyService *apikeys.Service) *KoboMiddleware {
	return &KoboMiddleware{apiKeyService: apiKeyService}
}

// APIKeyAuth validates the API key from the URL path and checks for kobo_sync permission.
func (m *KoboMiddleware) APIKeyAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			apiKeyValue := c.Param("apiKey")
			if apiKeyValue == "" {
				return errcodes.Unauthorized("API key required")
			}

			apiKey, err := m.apiKeyService.GetByKey(c.Request().Context(), apiKeyValue)
			if err != nil {
				return errors.WithStack(err)
			}
			if apiKey == nil {
				return errcodes.Unauthorized("Invalid API key")
			}

			if !apiKey.HasPermission(apikeys.PermissionKoboSync) {
				return errcodes.Forbidden("API key lacks Kobo sync permission")
			}

			// Touch last accessed (fire and forget)
			go func() {
				_ = m.apiKeyService.TouchLastAccessed(context.Background(), apiKey.ID)
			}()

			// Store API key in context
			ctx := context.WithValue(c.Request().Context(), contextKeyAPIKey, apiKey)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}

// ScopeParser parses the scope segment from the URL path.
// URL format: /kobo/:apiKey/:scopeType/:scopeId/v1/...
// Scope types: "all", "library", "list"
func (m *KoboMiddleware) ScopeParser() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			scopeType := c.Param("scopeType")
			scopeID := c.Param("scopeId")

			scope := &SyncScope{Type: "all"}

			switch scopeType {
			case "all":
				// No additional parsing needed
			case "library":
				id, err := strconv.Atoi(scopeID)
				if err != nil {
					return errcodes.ValidationError("Invalid library ID in scope")
				}
				scope.Type = "library"
				scope.LibraryID = &id
			case "list":
				id, err := strconv.Atoi(scopeID)
				if err != nil {
					return errcodes.ValidationError("Invalid list ID in scope")
				}
				scope.Type = "list"
				scope.ListID = &id
			default:
				// If scopeType doesn't match known types, treat as "all"
				// This handles the case where the URL has /all/v1/... (scopeType="all", scopeId="v1")
				scope.Type = "all"
			}

			ctx := context.WithValue(c.Request().Context(), contextKeyScope, scope)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}

// GetAPIKeyFromContext retrieves the API key from context.
func GetAPIKeyFromContext(ctx context.Context) *apikeys.APIKey {
	if apiKey, ok := ctx.Value(contextKeyAPIKey).(*apikeys.APIKey); ok {
		return apiKey
	}
	return nil
}

// GetScopeFromContext retrieves the sync scope from context.
func GetScopeFromContext(ctx context.Context) *SyncScope {
	if scope, ok := ctx.Value(contextKeyScope).(*SyncScope); ok {
		return scope
	}
	return &SyncScope{Type: "all"}
}

// stripKoboPrefix strips the /kobo/:apiKey/:scope/... prefix to get the Kobo API path.
func stripKoboPrefix(path string) string {
	// Path format: /kobo/{apiKey}/{scopeType}/{scopeId}/v1/...
	// or: /kobo/{apiKey}/all/v1/...
	parts := strings.SplitN(path, "/v1/", 2)
	if len(parts) == 2 {
		return "/v1/" + parts[1]
	}
	return path
}
```

**Step 2: Verify it compiles**

Run: `go build ./pkg/kobo/...`
Expected: No errors.

**Step 3: Commit**

```bash
git add pkg/kobo/middleware.go
git commit -m "[Kobo] Add API key auth and scope parsing middleware"
```

---

### Task 6: Proxy to Kobo Store

**Files:**
- Create: `pkg/kobo/proxy.go`

**Step 1: Write the proxy implementation**

```go
package kobo

import (
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

var koboStoreClient = &http.Client{
	Timeout: 30 * time.Second,
}

const koboStoreBaseURL = "https://storeapi.kobo.com"

// proxyToKoboStore forwards the request to the real Kobo store API.
func proxyToKoboStore(c echo.Context) error {
	koboPath := stripKoboPrefix(c.Request().URL.Path)
	targetURL := koboStoreBaseURL + koboPath
	if c.Request().URL.RawQuery != "" {
		targetURL += "?" + c.Request().URL.RawQuery
	}

	proxyReq, err := http.NewRequestWithContext(
		c.Request().Context(),
		c.Request().Method,
		targetURL,
		c.Request().Body,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	// Copy relevant headers from device
	headersToForward := []string{
		"Authorization",
		"Content-Type",
		"X-Kobo-SyncToken",
		"User-Agent",
	}
	for _, h := range headersToForward {
		if v := c.Request().Header.Get(h); v != "" {
			proxyReq.Header.Set(h, v)
		}
	}

	resp, err := koboStoreClient.Do(proxyReq)
	if err != nil {
		// If we can't reach Kobo, return a minimal OK response
		return c.JSON(http.StatusOK, map[string]interface{}{})
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, v := range resp.Header {
		for _, val := range v {
			c.Response().Header().Add(k, val)
		}
	}

	c.Response().WriteHeader(resp.StatusCode)
	_, err = io.Copy(c.Response().Writer, resp.Body)
	return errors.WithStack(err)
}
```

**Step 2: Verify it compiles**

Run: `go build ./pkg/kobo/...`
Expected: No errors.

**Step 3: Commit**

```bash
git add pkg/kobo/proxy.go
git commit -m "[Kobo] Add Kobo store proxy for unhandled requests"
```

---

### Task 7: Handlers (Init, Auth, Sync, Download, Cover)

**Files:**
- Create: `pkg/kobo/handlers.go`

**Step 1: Write the handlers**

```go
package kobo

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // Register PNG decoder
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
	"golang.org/x/image/draw"
)

type handler struct {
	service       *Service
	bookService   *books.Service
	downloadCache *downloadcache.Cache
}

func newHandler(service *Service, bookService *books.Service, downloadCache *downloadcache.Cache) *handler {
	return &handler{
		service:       service,
		bookService:   bookService,
		downloadCache: downloadCache,
	}
}

// handleInitialization handles GET /v1/initialization.
// Proxies to Kobo store and injects custom image URL templates.
func (h *handler) handleInitialization(c echo.Context) error {
	koboPath := stripKoboPrefix(c.Request().URL.Path)
	targetURL := koboStoreBaseURL + koboPath

	proxyReq, err := http.NewRequestWithContext(c.Request().Context(), "GET", targetURL, nil)
	if err != nil {
		return errors.WithStack(err)
	}

	// Forward headers
	for _, hdr := range []string{"Authorization", "User-Agent"} {
		if v := c.Request().Header.Get(hdr); v != "" {
			proxyReq.Header.Set(hdr, v)
		}
	}

	resp, err := koboStoreClient.Do(proxyReq)
	if err != nil {
		// Return minimal initialization response if Kobo store is unreachable
		return c.JSON(http.StatusOK, map[string]interface{}{
			"Resources": map[string]interface{}{},
		})
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"Resources": map[string]interface{}{},
		})
	}

	// Inject custom image URLs
	resources, ok := data["Resources"].(map[string]interface{})
	if !ok {
		resources = map[string]interface{}{}
		data["Resources"] = resources
	}

	baseURL := getBaseURL(c)
	resources["image_host"] = baseURL
	resources["image_url_template"] = baseURL + "/v1/books/{ImageId}/thumbnail/{Width}/{Height}/false/image.jpg"
	resources["image_url_quality_template"] = baseURL + "/v1/books/{ImageId}/thumbnail/{Width}/{Height}/{Quality}/{IsGreyscale}/image.jpg"

	return c.JSON(http.StatusOK, data)
}

// handleAuth handles POST /v1/auth/device.
// Returns dummy auth tokens since we use API key auth.
func (h *handler) handleAuth(c echo.Context) error {
	return c.JSON(http.StatusOK, DeviceAuthResponse{
		AccessToken:  "shisho-access-token",
		RefreshToken: "shisho-refresh-token",
		TokenType:    "Bearer",
		TrackingId:   "shisho-tracking",
		UserKey:      "shisho-user",
	})
}

// handleSync handles GET /v1/library/sync.
// Returns changes since the last sync point.
func (h *handler) handleSync(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)
	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		return errcodes.Unauthorized("API key not found")
	}
	scope := GetScopeFromContext(ctx)

	// Parse sync token
	var lastSyncPointID string
	if tokenHeader := c.Request().Header.Get("X-Kobo-SyncToken"); tokenHeader != "" {
		tokenBytes, err := base64.StdEncoding.DecodeString(tokenHeader)
		if err == nil {
			var token SyncToken
			if err := json.Unmarshal(tokenBytes, &token); err == nil {
				lastSyncPointID = token.LastSyncPointID
			}
		}
	}

	// Get current files in scope
	scopedFiles, err := h.service.GetScopedFiles(ctx, apiKey.UserID, scope)
	if err != nil {
		return errors.WithStack(err)
	}

	// Detect changes
	changes, err := h.service.DetectChanges(ctx, lastSyncPointID, scopedFiles)
	if err != nil {
		return errors.WithStack(err)
	}

	// Create new sync point
	sp, err := h.service.CreateSyncPoint(ctx, apiKey.ID, scopedFiles)
	if err != nil {
		return errors.WithStack(err)
	}

	log.Info("kobo sync completed", logger.Data{
		"api_key_id": apiKey.ID,
		"scope":      scope.Type,
		"added":      len(changes.Added),
		"removed":    len(changes.Removed),
		"changed":    len(changes.Changed),
		"total":      len(scopedFiles),
	})

	// Build response
	baseURL := getBaseURL(c)
	response := buildSyncResponse(changes, scopedFiles, baseURL, ctx, h.bookService)

	// Set new sync token
	newToken := SyncToken{LastSyncPointID: sp.ID}
	tokenJSON, _ := json.Marshal(newToken)
	c.Response().Header().Set("X-Kobo-SyncToken", base64.StdEncoding.EncodeToString(tokenJSON))

	return c.JSON(http.StatusOK, response)
}

// handleDownload handles GET /v1/books/:bookId/file/epub.
// Serves files as KePub.
func (h *handler) handleDownload(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)
	bookID := c.Param("bookId")

	fileID, ok := ParseShishoID(bookID)
	if !ok {
		return proxyToKoboStore(c)
	}

	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return errors.WithStack(err)
	}

	book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
	if err != nil {
		return errors.WithStack(err)
	}

	// Find the file with relations from the book's files
	var fileWithRelations *models.File
	for _, f := range book.Files {
		if f.ID == file.ID {
			fileWithRelations = f
			break
		}
	}
	if fileWithRelations == nil {
		fileWithRelations = file
	}

	// Generate KePub
	cachedPath, _, err := h.downloadCache.GetOrGenerateKepub(ctx, book, fileWithRelations)
	if err != nil {
		if errors.Is(err, filegen.ErrKepubNotSupported) {
			log.Warn("kepub not supported for file, serving original", logger.Data{"file_id": fileID})
			return c.File(file.Filepath)
		}
		var genErr *filegen.GenerationError
		if errors.As(err, &genErr) {
			log.Warn("kepub generation failed, serving original", logger.Data{"file_id": fileID, "error": genErr.Message})
			return c.File(file.Filepath)
		}
		return errors.WithStack(err)
	}

	return c.File(cachedPath)
}

// handleCover handles GET /v1/books/:imageId/thumbnail/:w/:h/*.
// Serves resized cover images.
func (h *handler) handleCover(c echo.Context) error {
	ctx := c.Request().Context()
	imageID := c.Param("imageId")

	fileID, ok := ParseShishoID(imageID)
	if !ok {
		return proxyToKoboStore(c)
	}

	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return errors.WithStack(err)
	}
	if file.CoverImagePath == nil || *file.CoverImagePath == "" {
		return errcodes.NotFound("Cover")
	}

	// Get the book to determine the cover path base dir
	book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
	if err != nil {
		return errors.WithStack(err)
	}

	// Determine cover directory (same logic as eReader handler)
	var coverDir string
	if info, statErr := os.Stat(book.Filepath); statErr == nil && !info.IsDir() {
		coverDir = filepath.Dir(book.Filepath)
	} else {
		coverDir = book.Filepath
	}

	coverPath := filepath.Join(coverDir, *file.CoverImagePath)

	// Parse requested dimensions
	widthStr := c.Param("w")
	heightStr := c.Param("h")
	width, _ := strconv.Atoi(widthStr)
	height, _ := strconv.Atoi(heightStr)

	if width == 0 || height == 0 {
		// Serve original if dimensions not specified
		return c.File(coverPath)
	}

	// Open and resize the image
	imgFile, err := os.Open(coverPath)
	if err != nil {
		return errcodes.NotFound("Cover")
	}
	defer imgFile.Close()

	srcImg, _, err := image.Decode(imgFile)
	if err != nil {
		return errors.WithStack(err)
	}

	// Resize maintaining aspect ratio, fitting within requested dimensions
	srcBounds := srcImg.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	// Calculate target dimensions maintaining aspect ratio
	targetW, targetH := fitDimensions(srcW, srcH, width, height)

	dstImg := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.BiLinear.Scale(dstImg, dstImg.Bounds(), srcImg, srcBounds, draw.Over, nil)

	c.Response().Header().Set("Content-Type", "image/jpeg")
	c.Response().Header().Set("Cache-Control", "public, max-age=86400")
	c.Response().WriteHeader(http.StatusOK)
	return jpeg.Encode(c.Response().Writer, dstImg, &jpeg.Options{Quality: 80})
}

// handleMetadata handles GET /v1/library/:bookId/metadata.
// Returns metadata for Shisho books, proxies for unknown books.
func (h *handler) handleMetadata(c echo.Context) error {
	ctx := c.Request().Context()
	bookID := c.Param("bookId")

	fileID, ok := ParseShishoID(bookID)
	if !ok {
		return proxyToKoboStore(c)
	}

	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return errors.WithStack(err)
	}

	book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
	if err != nil {
		return errors.WithStack(err)
	}

	baseURL := getBaseURL(c)
	metadata := buildBookMetadata(book, file, baseURL)
	return c.JSON(http.StatusOK, metadata)
}

// buildSyncResponse creates the array of sync change entries.
func buildSyncResponse(changes *SyncChanges, allFiles []ScopedFile, baseURL string, ctx context.Context, bookService *books.Service) []interface{} {
	var response []interface{}

	// Build a file ID to ScopedFile map for all current files
	fileMap := make(map[int]ScopedFile, len(allFiles))
	for _, f := range allFiles {
		fileMap[f.FileID] = f
	}

	// Added books
	for _, f := range changes.Added {
		entry := buildNewEntitlement(f, baseURL, ctx, bookService)
		if entry != nil {
			response = append(response, entry)
		}
	}

	// Changed books
	for _, f := range changes.Changed {
		entry := buildNewEntitlement(f, baseURL, ctx, bookService)
		if entry != nil {
			response = append(response, entry)
		}
	}

	// Removed books
	for _, f := range changes.Removed {
		response = append(response, &ChangedEntitlement{
			ChangedEntitlement: &EntitlementChangeWrapper{
				BookEntitlement: &BookEntitlementChange{
					Id:        ShishoID(f.FileID),
					IsRemoved: true,
				},
			},
		})
	}

	return response
}

// buildNewEntitlement creates a NewEntitlement sync entry for a file.
func buildNewEntitlement(f ScopedFile, baseURL string, ctx context.Context, bookService *books.Service) *NewEntitlement {
	file, err := bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &f.FileID})
	if err != nil {
		return nil
	}

	book, err := bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
	if err != nil {
		return nil
	}

	metadata := buildBookMetadata(book, file, baseURL)
	now := time.Now()

	return &NewEntitlement{
		NewEntitlement: &EntitlementWrapper{
			BookEntitlement: &BookEntitlement{
				Accessibility:     "Full",
				ActivePeriod:      &ActivePeriod{From: book.CreatedAt},
				Created:           book.CreatedAt,
				CrossRevisionId:   ShishoID(f.FileID),
				Id:                ShishoID(f.FileID),
				IsHiddenFromArchive: false,
				IsLocked:          false,
				IsRemoved:         false,
				LastModified:      now,
				OriginCategory:    "Imported",
				RevisionId:        fmt.Sprintf("%s-%s", ShishoID(f.FileID), f.FileHash[:8]),
				Status:            "Active",
			},
			BookMetadata: metadata,
		},
	}
}

// buildBookMetadata constructs the Kobo BookMetadata from our book/file.
func buildBookMetadata(book *models.Book, file *models.File, baseURL string) *BookMetadata {
	metadata := &BookMetadata{
		EntitlementId: ShishoID(file.ID),
		Title:         book.Title,
		CoverImageId:  ShishoID(file.ID),
		Language:      "en",
		DownloadUrls: []*DownloadURL{
			{
				Format:   "EPUB",
				Platform: "Generic",
				Size:     file.FilesizeBytes,
				Url:      fmt.Sprintf("%s/v1/books/%s/file/epub", baseURL, ShishoID(file.ID)),
			},
		},
	}

	// Authors
	if book.Authors != nil {
		for _, a := range book.Authors {
			if a.Person != nil {
				metadata.ContributorRoles = append(metadata.ContributorRoles, &ContributorRole{
					Name: a.Person.Name,
				})
			}
		}
	}

	// Description
	if book.Description != nil {
		metadata.Description = *book.Description
	}

	// Series
	if len(book.BookSeries) > 0 && book.BookSeries[0].Series != nil {
		metadata.Series = &Series{
			Name:        book.BookSeries[0].Series.Name,
			Number:      book.BookSeries[0].Number,
			NumberFloat: book.BookSeries[0].Number,
		}
	}

	// Publisher
	if file.Publisher != nil {
		metadata.Publisher = &Publisher{Name: file.Publisher.Name}
	}

	// Publication date
	if file.ReleaseDate != nil {
		metadata.PublicationDate = file.ReleaseDate.Format("2006-01-02")
	}

	return metadata
}

// getBaseURL returns the full base URL for the current Kobo scope.
func getBaseURL(c echo.Context) string {
	scheme := "http"
	if c.Request().TLS != nil || c.Request().Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	host := c.Request().Host
	apiKey := c.Param("apiKey")
	scopeType := c.Param("scopeType")
	scopeID := c.Param("scopeId")

	basePath := fmt.Sprintf("/kobo/%s/%s", apiKey, scopeType)
	if scopeID != "" && scopeType != "all" {
		basePath += "/" + scopeID
	}

	return fmt.Sprintf("%s://%s%s", scheme, host, basePath)
}

// fitDimensions calculates target dimensions maintaining aspect ratio.
func fitDimensions(srcW, srcH, maxW, maxH int) (int, int) {
	if srcW <= maxW && srcH <= maxH {
		return srcW, srcH
	}

	ratioW := float64(maxW) / float64(srcW)
	ratioH := float64(maxH) / float64(srcH)

	ratio := ratioW
	if ratioH < ratioW {
		ratio = ratioH
	}

	return int(float64(srcW) * ratio), int(float64(srcH) * ratio)
}
```

**Step 2: Verify it compiles**

Run: `go build ./pkg/kobo/...`
Expected: No errors.

**Step 3: Commit**

```bash
git add pkg/kobo/handlers.go
git commit -m "[Kobo] Add handlers for init, auth, sync, download, and cover endpoints"
```

---

### Task 8: Route Registration

**Files:**
- Create: `pkg/kobo/routes.go`
- Modify: `pkg/server/server.go:87` (add Kobo route registration)

**Step 1: Create the routes file**

```go
package kobo

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers all Kobo sync routes.
func RegisterRoutes(e *echo.Echo, db *bun.DB, downloadCache *downloadcache.Cache) {
	apiKeyService := apikeys.NewService(db)
	bookService := books.NewService(db)
	syncService := NewService(db)

	mw := NewMiddleware(apiKeyService)
	h := newHandler(syncService, bookService, downloadCache)

	// Kobo routes: /kobo/:apiKey/:scopeType/:scopeId/v1/...
	// "all" scope: /kobo/:apiKey/all/v1/...
	koboAll := e.Group("/kobo/:apiKey/all", mw.APIKeyAuth(), mw.ScopeParser())
	registerKoboEndpoints(koboAll, h)

	// "library" scope: /kobo/:apiKey/library/:scopeId/v1/...
	koboLibrary := e.Group("/kobo/:apiKey/library/:scopeId", mw.APIKeyAuth(), mw.ScopeParser())
	registerKoboEndpoints(koboLibrary, h)

	// "list" scope: /kobo/:apiKey/list/:scopeId/v1/...
	koboList := e.Group("/kobo/:apiKey/list/:scopeId", mw.APIKeyAuth(), mw.ScopeParser())
	registerKoboEndpoints(koboList, h)
}

// registerKoboEndpoints registers the Kobo API endpoints on a group.
func registerKoboEndpoints(g *echo.Group, h *handler) {
	g.GET("/v1/initialization", h.handleInitialization)
	g.POST("/v1/auth/device", h.handleAuth)
	g.GET("/v1/library/sync", h.handleSync)
	g.GET("/v1/library/:bookId/metadata", h.handleMetadata)
	g.GET("/v1/books/:bookId/file/epub", h.handleDownload)
	g.GET("/v1/books/:imageId/thumbnail/:w/:h/*", h.handleCover)

	// Catch-all: proxy unhandled requests to Kobo store
	g.Any("/v1/*", func(c echo.Context) error {
		return proxyToKoboStore(c)
	})
}
```

**Step 2: Register Kobo routes in the server**

In `pkg/server/server.go`, after line 87 (the `ereader.RegisterRoutes` call), add:

```go
	// Register Kobo sync routes (API key auth for Kobo device sync)
	kobo.RegisterRoutes(e, db, downloadCache)
```

And add the import at the top:

```go
	"github.com/shishobooks/shisho/pkg/kobo"
```

**Step 3: Verify it compiles**

Run: `go build ./cmd/api/...`
Expected: No errors.

**Step 4: Run all tests**

Run: `make test`
Expected: All tests pass.

**Step 5: Commit**

```bash
git add pkg/kobo/routes.go pkg/server/server.go
git commit -m "[Kobo] Register Kobo sync routes with scope-based URL structure"
```

---

### Task 9: Add `golang.org/x/image` Dependency

**Files:**
- Modify: `go.mod`

**Step 1: Add the dependency**

Run: `go get golang.org/x/image`
Expected: The module is added to go.mod.

**Step 2: Tidy**

Run: `go mod tidy`
Expected: go.sum is updated.

**Step 3: Verify the full build compiles**

Run: `go build ./...`
Expected: No errors.

**Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "[Kobo] Add golang.org/x/image dependency for cover resizing"
```

---

### Task 10: Frontend - Add `kobo_sync` Permission to API Key UI

**Files:**
- Modify: `app/components/pages/SecuritySettings.tsx`
- Modify: `pkg/apikeys/model.go` (already done in Task 2 - just verify the tygo generation)

**Step 1: Run tygo to generate the updated TypeScript types**

Run: `make tygo`
Expected: Types regenerated (or "Nothing to be done" which is fine).

**Step 2: Update the SecuritySettings component to include kobo_sync permission**

In `app/components/pages/SecuritySettings.tsx`, update the imports to include the new permission:

```typescript
import {
  PermissionEReaderBrowser,
  PermissionKoboSync,
  type APIKey,
  type APIKeyShortURL,
} from "@/types/generated/apikeys";
```

**Step 3: Add kobo_sync checkbox to CreateApiKeyDialog**

Add after the `enableEReader` state (around line 207):

```typescript
const [enableKoboSync, setEnableKoboSync] = useState(false);
```

In `handleCreate`, after the eReader permission add:

```typescript
      if (enableKoboSync) {
        await addPermission.mutateAsync({
          id: apiKey.id,
          permission: PermissionKoboSync,
        });
      }
```

Add after the eReader checkbox in the dialog (around line 268):

```tsx
          <div className="flex items-center space-x-2">
            <Checkbox
              checked={enableKoboSync}
              id="enable-kobo-sync"
              onCheckedChange={(checked) => setEnableKoboSync(checked === true)}
            />
            <Label className="cursor-pointer" htmlFor="enable-kobo-sync">
              Enable Kobo wireless sync
            </Label>
          </div>
```

**Step 4: Add kobo_sync checkbox to ApiKeyCard**

In the `ApiKeyCard` component, add below the eReader permission check:

```typescript
  const hasKoboSyncPermission = apiKey.permissions?.some(
    (p) => p?.permission === PermissionKoboSync,
  );
```

Add a handler:

```typescript
  const handleToggleKoboSync = async (checked: boolean) => {
    try {
      if (checked) {
        await addPermission.mutateAsync({
          id: apiKey.id,
          permission: PermissionKoboSync,
        });
      } else {
        await removePermission.mutateAsync({
          id: apiKey.id,
          permission: PermissionKoboSync,
        });
      }
    } catch {
      toast.error("Failed to update permission");
    }
  };
```

Add the checkbox after the eReader checkbox in the card:

```tsx
        <div className="flex items-center space-x-2">
          <Checkbox
            checked={hasKoboSyncPermission}
            disabled={addPermission.isPending || removePermission.isPending}
            id={`kobo-sync-${apiKey.id}`}
            onCheckedChange={handleToggleKoboSync}
          />
          <Label
            className="cursor-pointer text-sm"
            htmlFor={`kobo-sync-${apiKey.id}`}
          >
            Kobo wireless sync
          </Label>
        </div>
```

**Step 5: Verify frontend builds**

Run: `cd app && yarn build`
Expected: Build succeeds with no errors.

**Step 6: Commit**

```bash
git add app/components/pages/SecuritySettings.tsx
git commit -m "[Kobo] Add kobo_sync permission toggle to API key UI"
```

---

### Task 11: Frontend - Kobo Setup Modal

**Files:**
- Modify: `app/components/pages/SecuritySettings.tsx`
- Modify: `app/libraries/api.ts` (add libraries list endpoint if needed)

**Step 1: Add the KoboSetupDialog component to SecuritySettings.tsx**

Add a new component `KoboSetupDialog` after the existing `SetupDialog` component:

```tsx
function KoboSetupDialog({
  apiKey,
  open,
  onOpenChange,
}: {
  apiKey: APIKey;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [scopeType, setScopeType] = useState<"all" | "library" | "list">("all");
  const [scopeId, setScopeId] = useState("");
  const { data: libraries } = useLibraries();
  const { data: lists } = useLists();

  const buildSyncURL = () => {
    const origin = window.location.origin;
    let scopePath: string;
    switch (scopeType) {
      case "library":
        scopePath = `library/${scopeId}`;
        break;
      case "list":
        scopePath = `list/${scopeId}`;
        break;
      default:
        scopePath = "all";
    }
    return `${origin}/kobo/${apiKey.key}/${scopePath}`;
  };

  const syncURL = buildSyncURL();

  const handleCopy = () => {
    navigator.clipboard.writeText(syncURL);
    toast.success("Copied to clipboard");
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogTrigger asChild>
        <Button size="sm" variant="outline">
          Kobo Setup
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Kobo Sync Setup</DialogTitle>
          <DialogDescription>
            Configure your Kobo device to sync books wirelessly from Shisho.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-4">
          {/* Scope Selection */}
          <div className="space-y-2">
            <Label>Sync Scope</Label>
            <div className="space-y-2">
              <label className="flex items-center gap-2">
                <input
                  checked={scopeType === "all"}
                  name="scope"
                  onChange={() => setScopeType("all")}
                  type="radio"
                />
                <span className="text-sm">All Libraries</span>
              </label>
              <label className="flex items-center gap-2">
                <input
                  checked={scopeType === "library"}
                  name="scope"
                  onChange={() => setScopeType("library")}
                  type="radio"
                />
                <span className="text-sm">Specific Library</span>
              </label>
              {scopeType === "library" && libraries && (
                <select
                  className="ml-6 rounded border px-2 py-1 text-sm"
                  onChange={(e) => setScopeId(e.target.value)}
                  value={scopeId}
                >
                  <option value="">Select a library...</option>
                  {libraries.map((lib) => (
                    <option key={lib.id} value={lib.id}>
                      {lib.name}
                    </option>
                  ))}
                </select>
              )}
              <label className="flex items-center gap-2">
                <input
                  checked={scopeType === "list"}
                  name="scope"
                  onChange={() => setScopeType("list")}
                  type="radio"
                />
                <span className="text-sm">Specific List</span>
              </label>
              {scopeType === "list" && lists && (
                <select
                  className="ml-6 rounded border px-2 py-1 text-sm"
                  onChange={(e) => setScopeId(e.target.value)}
                  value={scopeId}
                >
                  <option value="">Select a list...</option>
                  {lists.map((list) => (
                    <option key={list.id} value={list.id}>
                      {list.name}
                    </option>
                  ))}
                </select>
              )}
            </div>
          </div>

          {/* Generated URL */}
          <div className="space-y-2">
            <Label>API Endpoint URL</Label>
            <div className="flex gap-2">
              <Input className="font-mono text-xs" readOnly value={syncURL} />
              <Button onClick={handleCopy} size="sm" variant="outline">
                <Copy className="h-4 w-4" />
              </Button>
            </div>
          </div>

          {/* Setup Instructions */}
          <div className="space-y-2">
            <Label>Setup Instructions</Label>
            <ol className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground">
              <li>Connect your Kobo via USB</li>
              <li>
                Navigate to{" "}
                <code className="rounded bg-muted px-1">.kobo/Kobo/Kobo eReader.conf</code>
              </li>
              <li>
                Find{" "}
                <code className="rounded bg-muted px-1">api_endpoint=https://storeapi.kobo.com</code>
              </li>
              <li>Replace with the URL above</li>
              <li>Eject the Kobo and sync</li>
            </ol>
          </div>
        </div>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

**Step 2: Add the Kobo Setup button to ApiKeyCard**

In the `ApiKeyCard` component, add a `koboSetupDialogOpen` state and render `KoboSetupDialog` next to the existing `SetupDialog` when the API key has `kobo_sync` permission:

```typescript
  const [koboSetupDialogOpen, setKoboSetupDialogOpen] = useState(false);
```

And in the render, after the existing `SetupDialog`:

```tsx
            {hasKoboSyncPermission && (
              <KoboSetupDialog
                apiKey={apiKey}
                onOpenChange={setKoboSetupDialogOpen}
                open={koboSetupDialogOpen}
              />
            )}
```

**Step 3: Add the query hooks for libraries and lists**

Import `useLibraries` from the libraries query hooks and `useLists` from the lists query hooks. These should already exist; verify they are importable. If not, add them.

**Step 4: Verify frontend builds**

Run: `cd app && yarn build`
Expected: Build succeeds.

**Step 5: Commit**

```bash
git add app/components/pages/SecuritySettings.tsx
git commit -m "[Kobo] Add Kobo setup modal with scope selection and instructions"
```

---

### Task 12: Fix Scope Middleware for URL Pattern

The URL pattern needs to handle both:
- `/kobo/:apiKey/all/v1/...` (no scopeId for "all")
- `/kobo/:apiKey/library/:scopeId/v1/...`
- `/kobo/:apiKey/list/:scopeId/v1/...`

This is already handled by the route registration in Task 8 which uses separate route groups for each scope type. Verify the scope parsing works correctly with the route groups.

**Step 1: Write an integration test for the middleware**

Create `pkg/kobo/middleware_test.go`:

```go
package kobo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripKoboPrefix(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/kobo/ak_123/all/v1/library/sync", "/v1/library/sync"},
		{"/kobo/ak_123/library/5/v1/library/sync", "/v1/library/sync"},
		{"/kobo/ak_123/all/v1/initialization", "/v1/initialization"},
		{"/kobo/ak_123/list/3/v1/books/shisho-1/file/epub", "/v1/books/shisho-1/file/epub"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := stripKoboPrefix(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseShishoID(t *testing.T) {
	tests := []struct {
		id       string
		expected int
		ok       bool
	}{
		{"shisho-1", 1, true},
		{"shisho-42", 42, true},
		{"shisho-0", 0, true},
		{"kobo-123", 0, false},
		{"invalid", 0, false},
		{"shisho-", 0, false},
		{"shisho-abc", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			result, ok := ParseShishoID(tt.id)
			assert.Equal(t, tt.expected, result)
			assert.Equal(t, tt.ok, ok)
		})
	}
}

func TestShishoID(t *testing.T) {
	assert.Equal(t, "shisho-1", ShishoID(1))
	assert.Equal(t, "shisho-42", ShishoID(42))
}
```

**Step 2: Run the tests**

Run: `TZ=America/Chicago CI=true go test ./pkg/kobo/ -v -run "TestStrip|TestParse|TestShisho"`
Expected: All tests pass.

**Step 3: Commit**

```bash
git add pkg/kobo/middleware_test.go
git commit -m "[Kobo] Add unit tests for middleware helpers and ID parsing"
```

---

### Task 13: Full Integration Test

**Files:**
- Create: `pkg/kobo/handlers_test.go`

**Step 1: Write integration tests for the sync flow**

```go
package kobo

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncToken_Encode(t *testing.T) {
	token := SyncToken{LastSyncPointID: "test-id-123"}
	tokenJSON, err := json.Marshal(token)
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(tokenJSON)

	// Verify it can be decoded
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)

	var parsed SyncToken
	err = json.Unmarshal(decoded, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "test-id-123", parsed.LastSyncPointID)
}

func TestFitDimensions(t *testing.T) {
	tests := []struct {
		name           string
		srcW, srcH     int
		maxW, maxH     int
		expectW, expectH int
	}{
		{"smaller than max", 100, 150, 200, 300, 100, 150},
		{"width constrained", 400, 300, 200, 300, 200, 150},
		{"height constrained", 300, 600, 300, 400, 200, 400},
		{"both constrained (width wins)", 800, 600, 400, 400, 400, 300},
		{"both constrained (height wins)", 600, 800, 400, 400, 300, 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := fitDimensions(tt.srcW, tt.srcH, tt.maxW, tt.maxH)
			assert.Equal(t, tt.expectW, w, "width")
			assert.Equal(t, tt.expectH, h, "height")
		})
	}
}

func TestBuildBookMetadata(t *testing.T) {
	// This tests that buildBookMetadata doesn't panic with minimal data
	book := &models.Book{
		ID:    1,
		Title: "Test Book",
	}
	file := &models.File{
		ID:            1,
		FilesizeBytes: 1024,
	}

	metadata := buildBookMetadata(book, file, "http://localhost:8080/kobo/key123/all")
	assert.Equal(t, "Test Book", metadata.Title)
	assert.Equal(t, "shisho-1", metadata.EntitlementId)
	assert.Len(t, metadata.DownloadUrls, 1)
	assert.Contains(t, metadata.DownloadUrls[0].Url, "shisho-1")
}
```

Add the import for `models`:

```go
import (
	...
	"github.com/shishobooks/shisho/pkg/models"
)
```

**Step 2: Run all kobo tests**

Run: `TZ=America/Chicago CI=true go test ./pkg/kobo/ -v`
Expected: All tests pass.

**Step 3: Commit**

```bash
git add pkg/kobo/handlers_test.go
git commit -m "[Kobo] Add integration tests for sync token, cover resizing, and metadata"
```

---

### Task 14: Run Full Check Suite

**Step 1: Run `make check`**

Run: `make check`
Expected: All linting, tests, and type checks pass.

**Step 2: Fix any lint issues**

If golangci-lint reports issues, fix them in the relevant files.

**Step 3: Fix any frontend lint issues**

Run: `cd app && yarn lint`
Expected: No errors.

**Step 4: Commit fixes if any**

```bash
git add -A
git commit -m "[Kobo] Fix lint issues"
```

---

### Task 15: Cleanup Old Sync Points (Background Maintenance)

**Files:**
- Modify: `pkg/kobo/service.go` (add cleanup method)

**Step 1: Add a cleanup method to the service**

```go
// CleanupOldSyncPoints removes completed sync points older than the most recent one per API key.
// This prevents the database from growing unbounded.
func (svc *Service) CleanupOldSyncPoints(ctx context.Context, apiKeyID string) error {
	// Keep only the most recent completed sync point per API key
	_, err := svc.db.NewDelete().
		Model((*SyncPoint)(nil)).
		Where("api_key_id = ?", apiKeyID).
		Where("completed_at IS NOT NULL").
		Where("id NOT IN (?)",
			svc.db.NewSelect().
				Model((*SyncPoint)(nil)).
				ColumnExpr("id").
				Where("api_key_id = ?", apiKeyID).
				Where("completed_at IS NOT NULL").
				OrderExpr("created_at DESC").
				Limit(1),
		).
		Exec(ctx)
	return errors.WithStack(err)
}
```

**Step 2: Call cleanup after creating a sync point in handleSync**

In `handlers.go`, after the `CreateSyncPoint` call in `handleSync`, add:

```go
	// Cleanup old sync points (fire and forget)
	go func() {
		_ = h.service.CleanupOldSyncPoints(context.Background(), apiKey.ID)
	}()
```

**Step 3: Write a test for cleanup**

In `service_test.go`:

```go
func TestCleanupOldSyncPoints(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db.DB)
	ctx := context.Background()

	// Create 3 sync points
	files := []ScopedFile{{FileID: 1, FileHash: "h1", FileSize: 100, MetadataHash: "m1"}}
	sp1, err := svc.CreateSyncPoint(ctx, "api-key-1", files)
	require.NoError(t, err)

	sp2, err := svc.CreateSyncPoint(ctx, "api-key-1", files)
	require.NoError(t, err)

	sp3, err := svc.CreateSyncPoint(ctx, "api-key-1", files)
	require.NoError(t, err)

	// Cleanup should keep only the most recent
	err = svc.CleanupOldSyncPoints(ctx, "api-key-1")
	require.NoError(t, err)

	// sp1 and sp2 should be gone
	got1, err := svc.GetSyncPointByID(ctx, sp1.ID)
	require.NoError(t, err)
	assert.Nil(t, got1)

	got2, err := svc.GetSyncPointByID(ctx, sp2.ID)
	require.NoError(t, err)
	assert.Nil(t, got2)

	// sp3 should still exist
	got3, err := svc.GetSyncPointByID(ctx, sp3.ID)
	require.NoError(t, err)
	assert.NotNil(t, got3)
	assert.Equal(t, sp3.ID, got3.ID)
}
```

**Step 4: Run tests**

Run: `TZ=America/Chicago CI=true go test ./pkg/kobo/ -v -run TestCleanup`
Expected: Test passes.

**Step 5: Commit**

```bash
git add pkg/kobo/service.go pkg/kobo/service_test.go pkg/kobo/handlers.go
git commit -m "[Kobo] Add sync point cleanup to prevent unbounded DB growth"
```

---

### Task 16: Final Verification

**Step 1: Run the full test suite**

Run: `make check`
Expected: All checks pass.

**Step 2: Build the binary**

Run: `make build`
Expected: Binary builds successfully.

**Step 3: Verify the migration is included in the migration registry**

Run: `go run ./cmd/api/ --help` (or similar smoke test)
Expected: Binary starts without panics.

---

## Summary of File Changes

### New Files
- `pkg/kobo/model.go` - Sync point models
- `pkg/kobo/dto.go` - Kobo API response types
- `pkg/kobo/service.go` - Sync service (change detection, scope queries)
- `pkg/kobo/service_test.go` - Service tests
- `pkg/kobo/middleware.go` - API key auth + scope parsing
- `pkg/kobo/middleware_test.go` - Middleware tests
- `pkg/kobo/handlers.go` - Endpoint handlers
- `pkg/kobo/handlers_test.go` - Handler tests
- `pkg/kobo/proxy.go` - Kobo store proxy
- `pkg/kobo/routes.go` - Route registration
- `pkg/migrations/20260120000000_create_kobo_sync_tables.go` - DB migration

### Modified Files
- `pkg/apikeys/model.go` - Add `PermissionKoboSync` constant
- `pkg/server/server.go` - Register Kobo routes
- `app/components/pages/SecuritySettings.tsx` - Add kobo_sync UI
- `go.mod` / `go.sum` - Add `golang.org/x/image` dependency
