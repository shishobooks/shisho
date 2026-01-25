package kobo

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/migrations"
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

func TestCreateSyncPoint(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	files := []ScopedFile{
		{FileID: 1, FileHash: "abc123", FileSize: 1024, MetadataHash: "meta1"},
		{FileID: 2, FileHash: "def456", FileSize: 2048, MetadataHash: "meta2"},
	}

	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", files)
	require.NoError(t, err)

	assert.NotEmpty(t, sp.ID, "SyncPoint should have a generated ID")
	assert.Equal(t, "api-key-1", sp.APIKeyID)
	assert.NotNil(t, sp.CompletedAt, "SyncPoint should be marked as complete")
	assert.Len(t, sp.Books, 2, "SyncPoint should have 2 books")

	// Verify books have correct data.
	booksByFileID := make(map[int]*SyncPointBook)
	for _, b := range sp.Books {
		booksByFileID[b.FileID] = b
	}
	assert.Equal(t, "abc123", booksByFileID[1].FileHash)
	assert.Equal(t, int64(1024), booksByFileID[1].FileSize)
	assert.Equal(t, "meta1", booksByFileID[1].MetadataHash)
	assert.Equal(t, "def456", booksByFileID[2].FileHash)
	assert.Equal(t, int64(2048), booksByFileID[2].FileSize)
	assert.Equal(t, "meta2", booksByFileID[2].MetadataHash)
}

func TestCreateSyncPoint_Empty(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", []ScopedFile{})
	require.NoError(t, err)

	assert.NotEmpty(t, sp.ID)
	assert.Equal(t, "api-key-1", sp.APIKeyID)
	assert.NotNil(t, sp.CompletedAt)
	assert.Empty(t, sp.Books)
}

func TestDetectChanges_FirstSync(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	currentFiles := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 2, FileHash: "hash2", FileSize: 200, MetadataHash: "meta2"},
		{FileID: 3, FileHash: "hash3", FileSize: 300, MetadataHash: "meta3"},
	}

	changes, err := svc.DetectChanges(ctx, "", currentFiles)
	require.NoError(t, err)

	assert.Len(t, changes.Added, 3, "First sync should mark all files as Added")
	assert.Empty(t, changes.Removed, "First sync should have no Removed")
	assert.Empty(t, changes.Changed, "First sync should have no Changed")

	// Verify the added files match input.
	addedIDs := make(map[int]bool)
	for _, f := range changes.Added {
		addedIDs[f.FileID] = true
	}
	assert.True(t, addedIDs[1])
	assert.True(t, addedIDs[2])
	assert.True(t, addedIDs[3])
}

func TestDetectChanges_AddedBooks(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create initial sync point with 2 files.
	prevFiles := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 2, FileHash: "hash2", FileSize: 200, MetadataHash: "meta2"},
	}
	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", prevFiles)
	require.NoError(t, err)

	// Current files include a new file (ID=3).
	currentFiles := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 2, FileHash: "hash2", FileSize: 200, MetadataHash: "meta2"},
		{FileID: 3, FileHash: "hash3", FileSize: 300, MetadataHash: "meta3"},
	}

	changes, err := svc.DetectChanges(ctx, sp.ID, currentFiles)
	require.NoError(t, err)

	assert.Len(t, changes.Added, 1, "Should detect 1 added book")
	assert.Equal(t, 3, changes.Added[0].FileID)
	assert.Equal(t, "hash3", changes.Added[0].FileHash)
	assert.Empty(t, changes.Removed)
	assert.Empty(t, changes.Changed)
}

func TestDetectChanges_RemovedBooks(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create initial sync point with 3 files.
	prevFiles := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 2, FileHash: "hash2", FileSize: 200, MetadataHash: "meta2"},
		{FileID: 3, FileHash: "hash3", FileSize: 300, MetadataHash: "meta3"},
	}
	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", prevFiles)
	require.NoError(t, err)

	// Current files are missing file ID=2.
	currentFiles := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 3, FileHash: "hash3", FileSize: 300, MetadataHash: "meta3"},
	}

	changes, err := svc.DetectChanges(ctx, sp.ID, currentFiles)
	require.NoError(t, err)

	assert.Empty(t, changes.Added)
	assert.Len(t, changes.Removed, 1, "Should detect 1 removed book")
	assert.Equal(t, 2, changes.Removed[0].FileID)
	assert.Equal(t, "hash2", changes.Removed[0].FileHash)
	assert.Empty(t, changes.Changed)
}

func TestDetectChanges_ChangedBooks(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create initial sync point.
	prevFiles := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 2, FileHash: "hash2", FileSize: 200, MetadataHash: "meta2"},
	}
	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", prevFiles)
	require.NoError(t, err)

	// File ID=2 has a different FileHash (file content changed).
	currentFiles := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 2, FileHash: "hash2-updated", FileSize: 250, MetadataHash: "meta2"},
	}

	changes, err := svc.DetectChanges(ctx, sp.ID, currentFiles)
	require.NoError(t, err)

	assert.Empty(t, changes.Added)
	assert.Empty(t, changes.Removed)
	assert.Len(t, changes.Changed, 1, "Should detect 1 changed book")
	assert.Equal(t, 2, changes.Changed[0].FileID)
	assert.Equal(t, "hash2-updated", changes.Changed[0].FileHash)
}

func TestDetectChanges_MetadataChange(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create initial sync point.
	prevFiles := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 2, FileHash: "hash2", FileSize: 200, MetadataHash: "meta2"},
	}
	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", prevFiles)
	require.NoError(t, err)

	// File ID=1 has same FileHash but different MetadataHash.
	currentFiles := []ScopedFile{
		{FileID: 1, FileHash: "hash1", FileSize: 100, MetadataHash: "meta1-updated"},
		{FileID: 2, FileHash: "hash2", FileSize: 200, MetadataHash: "meta2"},
	}

	changes, err := svc.DetectChanges(ctx, sp.ID, currentFiles)
	require.NoError(t, err)

	assert.Empty(t, changes.Added)
	assert.Empty(t, changes.Removed)
	assert.Len(t, changes.Changed, 1, "Should detect metadata change")
	assert.Equal(t, 1, changes.Changed[0].FileID)
	assert.Equal(t, "meta1-updated", changes.Changed[0].MetadataHash)
	assert.Equal(t, "hash1", changes.Changed[0].FileHash, "FileHash should remain the same")
}

func TestShishoID(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "shisho-42", ShishoID(42))
	assert.Equal(t, "shisho-1", ShishoID(1))
	assert.Equal(t, "shisho-0", ShishoID(0))
}

func TestParseShishoID(t *testing.T) {
	t.Parallel()
	id, ok := ParseShishoID("shisho-42")
	assert.True(t, ok)
	assert.Equal(t, 42, id)

	id, ok = ParseShishoID("shisho-1")
	assert.True(t, ok)
	assert.Equal(t, 1, id)

	// Invalid cases.
	_, ok = ParseShishoID("invalid-42")
	assert.False(t, ok)

	_, ok = ParseShishoID("shisho-")
	assert.False(t, ok)

	_, ok = ParseShishoID("shisho-abc")
	assert.False(t, ok)

	_, ok = ParseShishoID("")
	assert.False(t, ok)
}

func TestCleanupOldSyncPoints(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	svc := NewService(db)
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

	// sp1 and sp2 should be gone (sql.ErrNoRows)
	_, err = svc.GetSyncPointByID(ctx, sp1.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, sql.ErrNoRows))

	_, err = svc.GetSyncPointByID(ctx, sp2.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, sql.ErrNoRows))

	// sp3 should still exist
	got3, err := svc.GetSyncPointByID(ctx, sp3.ID)
	require.NoError(t, err)
	assert.NotNil(t, got3)
	assert.Equal(t, sp3.ID, got3.ID)
}
