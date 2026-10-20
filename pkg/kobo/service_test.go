package kobo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// newSyncPointTestDB returns testdb.New with the Device the sync point
// tests reference.
func newSyncPointTestDB(t *testing.T) *bun.DB {
	t.Helper()
	db := testdb.New(t)
	// Insert a test Device that all sync point tests reference
	insertTestDevice(t, db, "api-key-1")
	return db
}

// insertTestDevice creates a user and Device for FK-valid test data.
func insertTestDevice(t *testing.T, db *bun.DB, keyID string) {
	t.Helper()

	// Get admin role ID (seeded by migrations)
	var roleID int
	err := db.QueryRow("SELECT id FROM roles WHERE name = 'admin'").Scan(&roleID)
	require.NoError(t, err)

	// Insert a user (use the keyID as a suffix so the helper can be called
	// more than once per DB without colliding on the unique username).
	username := "_device_owner_" + keyID
	_, err = db.Exec("INSERT INTO users (username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?)",
		username, "fakehash", roleID, true)
	require.NoError(t, err)

	var userID int
	err = db.QueryRow("SELECT id FROM users WHERE username = ?", username).Scan(&userID)
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO devices (id, user_id, name, device_type, credential, created_at, updated_at) VALUES (?, ?, ?, 'kobo', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		keyID, userID, "test-device", "ak_test_"+keyID)
	require.NoError(t, err)
}

func TestCreateSyncPoint(t *testing.T) {
	t.Parallel()
	db := newSyncPointTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	files := []ScopedFile{
		{FileID: 1, EntryID: "shisho-1", FileHash: "abc123", FileSize: 1024, MetadataHash: "meta1"},
		{FileID: 2, EntryID: "shisho-2", FileHash: "def456", FileSize: 2048, MetadataHash: "meta2"},
	}

	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", files)
	require.NoError(t, err)

	assert.NotEmpty(t, sp.ID, "SyncPoint should have a generated ID")
	assert.Equal(t, "api-key-1", sp.DeviceID)
	assert.Nil(t, sp.CompletedAt, "SyncPoint should be created as in-progress")
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
	db := newSyncPointTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", []ScopedFile{})
	require.NoError(t, err)

	assert.NotEmpty(t, sp.ID)
	assert.Equal(t, "api-key-1", sp.DeviceID)
	assert.Nil(t, sp.CompletedAt, "SyncPoint should be created as in-progress")
	assert.Empty(t, sp.Books)
}

func TestGetSyncPointByID_RejectsOtherTenant(t *testing.T) {
	t.Parallel()
	db := newSyncPointTestDB(t)
	insertTestDevice(t, db, "api-key-2")
	ctx := context.Background()
	svc := NewService(db)

	// Sync point owned by api-key-1.
	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", []ScopedFile{
		{FileID: 1, EntryID: "shisho-1", FileHash: "h", FileSize: 1, MetadataHash: "m"},
	})
	require.NoError(t, err)

	// api-key-2 must not be able to read it even with the correct UUID.
	_, err = svc.GetSyncPointByID(ctx, "api-key-2", sp.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, sql.ErrNoRows),
		"GetSyncPointByID must return ErrNoRows when device_id does not match the row owner")

	// MarkSyncPointCompleted from the wrong owner must be a no-op (the WHERE
	// clause matches no rows, so completed_at stays nil for the real owner).
	require.NoError(t, svc.MarkSyncPointCompleted(ctx, "api-key-2", sp.ID))
	got, err := svc.GetSyncPointByID(ctx, "api-key-1", sp.ID)
	require.NoError(t, err)
	assert.Nil(t, got.CompletedAt,
		"MarkSyncPointCompleted from a non-owner must not stamp completed_at on the row")
}

func TestMarkSyncPointCompleted(t *testing.T) {
	t.Parallel()
	db := newSyncPointTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", []ScopedFile{
		{FileID: 1, EntryID: "shisho-1", FileHash: "h", FileSize: 1, MetadataHash: "m"},
	})
	require.NoError(t, err)
	require.Nil(t, sp.CompletedAt)

	require.NoError(t, svc.MarkSyncPointCompleted(ctx, "api-key-1", sp.ID))

	got, err := svc.GetSyncPointByID(ctx, "api-key-1", sp.ID)
	require.NoError(t, err)
	require.NotNil(t, got.CompletedAt, "CompletedAt should be set after MarkSyncPointCompleted")

	// GetLastSyncPoint filters on completed_at IS NOT NULL — it should now find this point.
	last, err := svc.GetLastSyncPoint(ctx, "api-key-1")
	require.NoError(t, err)
	assert.Equal(t, sp.ID, last.ID)
}

func TestDetectChanges_FirstSync(t *testing.T) {
	t.Parallel()
	db := newSyncPointTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	currentFiles := []ScopedFile{
		{FileID: 1, EntryID: "shisho-1", FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 2, EntryID: "shisho-2", FileHash: "hash2", FileSize: 200, MetadataHash: "meta2"},
		{FileID: 3, EntryID: "shisho-3", FileHash: "hash3", FileSize: 300, MetadataHash: "meta3"},
	}

	changes, err := svc.DetectChanges(ctx, "api-key-1", "", currentFiles)
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
	db := newSyncPointTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create initial sync point with 2 files.
	prevFiles := []ScopedFile{
		{FileID: 1, EntryID: "shisho-1", FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 2, EntryID: "shisho-2", FileHash: "hash2", FileSize: 200, MetadataHash: "meta2"},
	}
	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", prevFiles)
	require.NoError(t, err)

	// Current files include a new file (ID=3).
	currentFiles := []ScopedFile{
		{FileID: 1, EntryID: "shisho-1", FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 2, EntryID: "shisho-2", FileHash: "hash2", FileSize: 200, MetadataHash: "meta2"},
		{FileID: 3, EntryID: "shisho-3", FileHash: "hash3", FileSize: 300, MetadataHash: "meta3"},
	}

	changes, err := svc.DetectChanges(ctx, "api-key-1", sp.ID, currentFiles)
	require.NoError(t, err)

	assert.Len(t, changes.Added, 1, "Should detect 1 added book")
	assert.Equal(t, 3, changes.Added[0].FileID)
	assert.Equal(t, "hash3", changes.Added[0].FileHash)
	assert.Empty(t, changes.Removed)
	assert.Empty(t, changes.Changed)
}

func TestDetectChanges_RemovedBooks(t *testing.T) {
	t.Parallel()
	db := newSyncPointTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create initial sync point with 3 files.
	prevFiles := []ScopedFile{
		{FileID: 1, EntryID: "shisho-1", FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 2, EntryID: "shisho-2", FileHash: "hash2", FileSize: 200, MetadataHash: "meta2"},
		{FileID: 3, EntryID: "shisho-3", FileHash: "hash3", FileSize: 300, MetadataHash: "meta3"},
	}
	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", prevFiles)
	require.NoError(t, err)

	// Current files are missing file ID=2.
	currentFiles := []ScopedFile{
		{FileID: 1, EntryID: "shisho-1", FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 3, EntryID: "shisho-3", FileHash: "hash3", FileSize: 300, MetadataHash: "meta3"},
	}

	changes, err := svc.DetectChanges(ctx, "api-key-1", sp.ID, currentFiles)
	require.NoError(t, err)

	assert.Empty(t, changes.Added)
	assert.Len(t, changes.Removed, 1, "Should detect 1 removed book")
	assert.Equal(t, 2, changes.Removed[0].FileID)
	assert.Equal(t, "hash2", changes.Removed[0].FileHash)
	assert.Empty(t, changes.Changed)
}

func TestDetectChanges_ChangedBooks(t *testing.T) {
	t.Parallel()
	db := newSyncPointTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create initial sync point.
	prevFiles := []ScopedFile{
		{FileID: 1, EntryID: "shisho-1", FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 2, EntryID: "shisho-2", FileHash: "hash2", FileSize: 200, MetadataHash: "meta2"},
	}
	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", prevFiles)
	require.NoError(t, err)

	// File ID=2 has a different FileHash (file content changed).
	currentFiles := []ScopedFile{
		{FileID: 1, EntryID: "shisho-1", FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 2, EntryID: "shisho-2", FileHash: "hash2-updated", FileSize: 250, MetadataHash: "meta2"},
	}

	changes, err := svc.DetectChanges(ctx, "api-key-1", sp.ID, currentFiles)
	require.NoError(t, err)

	assert.Empty(t, changes.Added)
	assert.Empty(t, changes.Removed)
	assert.Len(t, changes.Changed, 1, "Should detect 1 changed book")
	assert.Equal(t, 2, changes.Changed[0].FileID)
	assert.Equal(t, "hash2-updated", changes.Changed[0].FileHash)
}

func TestDetectChanges_MetadataChange(t *testing.T) {
	t.Parallel()
	db := newSyncPointTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create initial sync point.
	prevFiles := []ScopedFile{
		{FileID: 1, EntryID: "shisho-1", FileHash: "hash1", FileSize: 100, MetadataHash: "meta1"},
		{FileID: 2, EntryID: "shisho-2", FileHash: "hash2", FileSize: 200, MetadataHash: "meta2"},
	}
	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", prevFiles)
	require.NoError(t, err)

	// File ID=1 has same FileHash but different MetadataHash.
	currentFiles := []ScopedFile{
		{FileID: 1, EntryID: "shisho-1", FileHash: "hash1", FileSize: 100, MetadataHash: "meta1-updated"},
		{FileID: 2, EntryID: "shisho-2", FileHash: "hash2", FileSize: 200, MetadataHash: "meta2"},
	}

	changes, err := svc.DetectChanges(ctx, "api-key-1", sp.ID, currentFiles)
	require.NoError(t, err)

	assert.Empty(t, changes.Added)
	assert.Empty(t, changes.Removed)
	assert.Len(t, changes.Changed, 1, "Should detect metadata change")
	assert.Equal(t, 1, changes.Changed[0].FileID)
	assert.Equal(t, "meta1-updated", changes.Changed[0].MetadataHash)
	assert.Equal(t, "hash1", changes.Changed[0].FileHash, "FileHash should remain the same")
}

func TestScopedFilesFromSnapshot_RoundTrip(t *testing.T) {
	t.Parallel()
	books := []*SyncPointBook{
		{FileID: 1, EntryID: "shisho-1", FileHash: "h1", FileSize: 100, MetadataHash: "m1"},
		{FileID: 2, EntryID: "shisho-2", FileHash: "h2", FileSize: 200, MetadataHash: "m2"},
	}
	out := ScopedFilesFromSnapshot(books)
	require.Len(t, out, 2)
	assert.Equal(t, 1, out[0].FileID)
	assert.Equal(t, "h1", out[0].FileHash)
	assert.Equal(t, int64(100), out[0].FileSize)
	assert.Equal(t, "m1", out[0].MetadataHash)
	assert.Equal(t, 2, out[1].FileID)
}

func TestCleanupOldSyncPoints_SkipsInProgress(t *testing.T) {
	t.Parallel()
	db := newSyncPointTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	files := []ScopedFile{{FileID: 1, EntryID: "shisho-1", FileHash: "h1", FileSize: 100, MetadataHash: "m1"}}

	// One completed, one in-progress (e.g. an in-flight pagination).
	completed, err := svc.CreateSyncPoint(ctx, "api-key-1", files)
	require.NoError(t, err)
	require.NoError(t, svc.MarkSyncPointCompleted(ctx, "api-key-1", completed.ID))

	inProgress, err := svc.CreateSyncPoint(ctx, "api-key-1", files)
	require.NoError(t, err)

	// Cleanup must not delete the in-progress point even though it's not the
	// "most recent completed" — it represents a paginated sync the device is
	// actively walking through.
	require.NoError(t, svc.CleanupOldSyncPoints(ctx, "api-key-1"))

	got, err := svc.GetSyncPointByID(ctx, "api-key-1", inProgress.ID)
	require.NoError(t, err)
	assert.NotNil(t, got)
}

func TestCleanupOldSyncPoints(t *testing.T) {
	t.Parallel()
	db := newSyncPointTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create 3 sync points and mark all completed (cleanup only touches completed points).
	files := []ScopedFile{{FileID: 1, EntryID: "shisho-1", FileHash: "h1", FileSize: 100, MetadataHash: "m1"}}
	sp1, err := svc.CreateSyncPoint(ctx, "api-key-1", files)
	require.NoError(t, err)
	require.NoError(t, svc.MarkSyncPointCompleted(ctx, "api-key-1", sp1.ID))

	sp2, err := svc.CreateSyncPoint(ctx, "api-key-1", files)
	require.NoError(t, err)
	require.NoError(t, svc.MarkSyncPointCompleted(ctx, "api-key-1", sp2.ID))

	sp3, err := svc.CreateSyncPoint(ctx, "api-key-1", files)
	require.NoError(t, err)
	require.NoError(t, svc.MarkSyncPointCompleted(ctx, "api-key-1", sp3.ID))

	// Cleanup should keep only the most recent
	err = svc.CleanupOldSyncPoints(ctx, "api-key-1")
	require.NoError(t, err)

	// sp1 and sp2 should be gone (sql.ErrNoRows)
	_, err = svc.GetSyncPointByID(ctx, "api-key-1", sp1.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, sql.ErrNoRows))

	_, err = svc.GetSyncPointByID(ctx, "api-key-1", sp2.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, sql.ErrNoRows))

	// sp3 should still exist
	got3, err := svc.GetSyncPointByID(ctx, "api-key-1", sp3.ID)
	require.NoError(t, err)
	assert.NotNil(t, got3)
	assert.Equal(t, sp3.ID, got3.ID)
}

// A book without a cover keeps the CoverImageId suffix it had before cover
// times were tracked: the metadata hash prefix alone.
func TestCoverCacheKey(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "0123abcd", coverCacheKey("0123abcd99999999", nil))
	assert.Equal(t, "abc", coverCacheKey("abc", nil))

	mtime := time.Unix(1700000000, 0)
	withCover := coverCacheKey("0123abcd99999999", &mtime)
	assert.Len(t, withCover, 8)
	assert.NotEqual(t, "0123abcd", withCover)
	later := mtime.Add(time.Microsecond)
	assert.NotEqual(t, withCover, coverCacheKey("0123abcd99999999", &later))
}

// The hash input for a file whose Kobo title is its book's title must not
// change, or every synced book would go out as changed after an upgrade.
func TestComputeMetadataHash_UnchangedWhenTitleIsBookTitle(t *testing.T) {
	t.Parallel()
	cover := "cover.jpg"
	book := &models.Book{
		Title:   "Alpha",
		Authors: []*models.Author{{Person: &models.Person{Name: "Ann Author"}}},
	}
	sum := sha256.Sum256([]byte("Alpha\x00Ann Author\x00cover.jpg"))
	want := hex.EncodeToString(sum[:])[:16]

	sameName := "Alpha"
	empty := ""
	for _, name := range []*string{nil, &empty, &sameName} {
		file := &models.File{Name: name, CoverImageFilename: &cover}
		assert.Equal(t, want, computeMetadataHash(book, file))
	}

	other := "Alpha: Special Edition"
	assert.NotEqual(t, want, computeMetadataHash(book, &models.File{Name: &other, CoverImageFilename: &cover}))
}

func TestComputeMetadataHash_NamedFileWithoutBookKeepsName(t *testing.T) {
	t.Parallel()
	a, b := "First", "Second"
	assert.NotEqual(t,
		computeMetadataHash(nil, &models.File{Name: &a}),
		computeMetadataHash(nil, &models.File{Name: &b}))
}

// The delivered File Type is part of what decides whether an entry changed.
func TestDetectChanges_DeliveredTypeChange(t *testing.T) {
	t.Parallel()
	db := newSyncPointTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	prev := []ScopedFile{{FileID: 1, EntryID: "shisho-1", FileHash: "h", MetadataHash: "m", DeliveredFileType: models.FileTypeKePub}}
	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", prev)
	require.NoError(t, err)

	curr := []ScopedFile{{FileID: 1, EntryID: "shisho-1", FileHash: "h", MetadataHash: "m", DeliveredFileType: models.FileTypeEPUB}}
	changes, err := svc.DetectChanges(ctx, "api-key-1", sp.ID, curr)
	require.NoError(t, err)
	assert.Len(t, changes.Changed, 1)
}

// An entry handed to another File is a change under the same entry id,
// whatever the hashes say.
func TestDetectChanges_HandedOverEntry(t *testing.T) {
	t.Parallel()
	db := newSyncPointTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	prev := []ScopedFile{{FileID: 1, EntryID: "shisho-1", FileHash: "h", MetadataHash: "m", DeliveredFileType: models.FileTypeKePub}}
	sp, err := svc.CreateSyncPoint(ctx, "api-key-1", prev)
	require.NoError(t, err)

	curr := []ScopedFile{{FileID: 2, EntryID: "shisho-1", FileHash: "h", MetadataHash: "m", DeliveredFileType: models.FileTypeKePub}}
	changes, err := svc.DetectChanges(ctx, "api-key-1", sp.ID, curr)
	require.NoError(t, err)
	require.Len(t, changes.Changed, 1)
	assert.Empty(t, changes.Added)
	assert.Empty(t, changes.Removed)
}
