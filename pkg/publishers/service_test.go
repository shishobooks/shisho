package publishers

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
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

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func createTestLibrary(t *testing.T, db *bun.DB) *models.Library {
	t.Helper()
	lib := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(t, err)
	return lib
}

func TestFindOrCreatePublisher_PrimaryNameMatch(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	pub := &models.Publisher{LibraryID: lib.ID, Name: "Penguin Random House"}
	err := svc.CreatePublisher(ctx, pub)
	require.NoError(t, err)

	found, err := svc.FindOrCreatePublisher(ctx, "Penguin Random House", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, pub.ID, found.ID)
}

func TestFindOrCreatePublisher_AliasMatch(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	pub := &models.Publisher{LibraryID: lib.ID, Name: "Penguin Random House"}
	err := svc.CreatePublisher(ctx, pub)
	require.NoError(t, err)

	_, err = db.NewRaw(
		"INSERT INTO publisher_aliases (created_at, publisher_id, name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), pub.ID, "PRH", lib.ID,
	).Exec(ctx)
	require.NoError(t, err)

	found, err := svc.FindOrCreatePublisher(ctx, "PRH", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, pub.ID, found.ID)
	assert.Equal(t, "Penguin Random House", found.Name)
}

func TestFindOrCreatePublisher_NoMatch_CreatesNew(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	found, err := svc.FindOrCreatePublisher(ctx, "HarperCollins", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, "HarperCollins", found.Name)
	assert.Equal(t, lib.ID, found.LibraryID)
}

func TestSetParent_Success(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	parent := &models.Publisher{LibraryID: lib.ID, Name: "Penguin Random House"}
	err := svc.CreatePublisher(ctx, parent)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Dutton"}
	err = svc.CreatePublisher(ctx, child)
	require.NoError(t, err)

	err = svc.SetParent(ctx, child.ID, &parent.ID)
	require.NoError(t, err)

	// Retrieve and verify
	updated, err := svc.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &child.ID})
	require.NoError(t, err)
	require.NotNil(t, updated.ParentID)
	assert.Equal(t, parent.ID, *updated.ParentID)
}

func TestSetParent_ClearParent(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	parent := &models.Publisher{LibraryID: lib.ID, Name: "Penguin Random House"}
	err := svc.CreatePublisher(ctx, parent)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Dutton", ParentID: &parent.ID}
	err = svc.CreatePublisher(ctx, child)
	require.NoError(t, err)

	// Clear parent
	err = svc.SetParent(ctx, child.ID, nil)
	require.NoError(t, err)

	updated, err := svc.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &child.ID})
	require.NoError(t, err)
	assert.Nil(t, updated.ParentID)
}

func TestSetParent_DirectCycleRejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	pubA := &models.Publisher{LibraryID: lib.ID, Name: "A"}
	err := svc.CreatePublisher(ctx, pubA)
	require.NoError(t, err)

	pubB := &models.Publisher{LibraryID: lib.ID, Name: "B"}
	err = svc.CreatePublisher(ctx, pubB)
	require.NoError(t, err)

	// A -> B
	err = svc.SetParent(ctx, pubA.ID, &pubB.ID)
	require.NoError(t, err)

	// B -> A would create cycle
	err = svc.SetParent(ctx, pubB.ID, &pubA.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestSetParent_DeeperCycleRejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	pubA := &models.Publisher{LibraryID: lib.ID, Name: "A"}
	err := svc.CreatePublisher(ctx, pubA)
	require.NoError(t, err)

	pubB := &models.Publisher{LibraryID: lib.ID, Name: "B"}
	err = svc.CreatePublisher(ctx, pubB)
	require.NoError(t, err)

	pubC := &models.Publisher{LibraryID: lib.ID, Name: "C"}
	err = svc.CreatePublisher(ctx, pubC)
	require.NoError(t, err)

	// Chain: A -> B -> C
	err = svc.SetParent(ctx, pubA.ID, &pubB.ID)
	require.NoError(t, err)

	err = svc.SetParent(ctx, pubB.ID, &pubC.ID)
	require.NoError(t, err)

	// C -> A would create cycle A->B->C->A
	err = svc.SetParent(ctx, pubC.ID, &pubA.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestSetParent_SelfReferenceRejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	pub := &models.Publisher{LibraryID: lib.ID, Name: "A"}
	err := svc.CreatePublisher(ctx, pub)
	require.NoError(t, err)

	err = svc.SetParent(ctx, pub.ID, &pub.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestGetAncestors_ReturnsOrderedChain(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	root := &models.Publisher{LibraryID: lib.ID, Name: "Root"}
	err := svc.CreatePublisher(ctx, root)
	require.NoError(t, err)

	middle := &models.Publisher{LibraryID: lib.ID, Name: "Middle", ParentID: &root.ID}
	err = svc.CreatePublisher(ctx, middle)
	require.NoError(t, err)

	leaf := &models.Publisher{LibraryID: lib.ID, Name: "Leaf", ParentID: &middle.ID}
	err = svc.CreatePublisher(ctx, leaf)
	require.NoError(t, err)

	ancestors, err := svc.GetAncestors(ctx, leaf.ID)
	require.NoError(t, err)
	require.Len(t, ancestors, 2)
	// Ordered from immediate parent to root
	assert.Equal(t, "Middle", ancestors[0].Name)
	assert.Equal(t, "Root", ancestors[1].Name)
}

func TestGetAncestors_RootReturnsEmpty(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	root := &models.Publisher{LibraryID: lib.ID, Name: "Root"}
	err := svc.CreatePublisher(ctx, root)
	require.NoError(t, err)

	ancestors, err := svc.GetAncestors(ctx, root.ID)
	require.NoError(t, err)
	assert.Empty(t, ancestors)
}

func TestGetDescendantIDs_ReturnsAllDescendants(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	root := &models.Publisher{LibraryID: lib.ID, Name: "Root"}
	err := svc.CreatePublisher(ctx, root)
	require.NoError(t, err)

	childA := &models.Publisher{LibraryID: lib.ID, Name: "ChildA", ParentID: &root.ID}
	err = svc.CreatePublisher(ctx, childA)
	require.NoError(t, err)

	childB := &models.Publisher{LibraryID: lib.ID, Name: "ChildB", ParentID: &root.ID}
	err = svc.CreatePublisher(ctx, childB)
	require.NoError(t, err)

	grandchild := &models.Publisher{LibraryID: lib.ID, Name: "Grandchild", ParentID: &childA.ID}
	err = svc.CreatePublisher(ctx, grandchild)
	require.NoError(t, err)

	ids, err := svc.GetDescendantIDs(ctx, root.ID)
	require.NoError(t, err)
	assert.Len(t, ids, 3)
	assert.Contains(t, ids, childA.ID)
	assert.Contains(t, ids, childB.ID)
	assert.Contains(t, ids, grandchild.ID)
}

func TestGetDescendantIDs_LeafReturnsEmpty(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	pub := &models.Publisher{LibraryID: lib.ID, Name: "Leaf"}
	err := svc.CreatePublisher(ctx, pub)
	require.NoError(t, err)

	ids, err := svc.GetDescendantIDs(ctx, pub.ID)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestListPublishers_ExcludeIDs(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	pubA := &models.Publisher{LibraryID: lib.ID, Name: "A"}
	err := svc.CreatePublisher(ctx, pubA)
	require.NoError(t, err)

	pubB := &models.Publisher{LibraryID: lib.ID, Name: "B"}
	err = svc.CreatePublisher(ctx, pubB)
	require.NoError(t, err)

	pubC := &models.Publisher{LibraryID: lib.ID, Name: "C"}
	err = svc.CreatePublisher(ctx, pubC)
	require.NoError(t, err)

	results, err := svc.ListPublishers(ctx, ListPublishersOptions{
		LibraryID:  &lib.ID,
		ExcludeIDs: []int{pubA.ID, pubC.ID},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "B", results[0].Name)
}

func TestListPublishers_SearchMatchesAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)
	searchSvc := search.NewService(db)

	lib := createTestLibrary(t, db)

	pub := &models.Publisher{LibraryID: lib.ID, Name: "Penguin Random House"}
	err := svc.CreatePublisher(ctx, pub)
	require.NoError(t, err)

	_, err = db.NewRaw(
		"INSERT INTO publisher_aliases (created_at, publisher_id, name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), pub.ID, "PRH", lib.ID,
	).Exec(ctx)
	require.NoError(t, err)

	err = searchSvc.IndexPublisher(ctx, pub)
	require.NoError(t, err)

	searchStr := "PRH"
	results, err := svc.ListPublishers(ctx, ListPublishersOptions{
		LibraryID: &lib.ID,
		Search:    &searchStr,
	})
	require.NoError(t, err)
	require.Len(t, results, 1, "Should find publisher by alias 'PRH'")
	assert.Equal(t, "Penguin Random House", results[0].Name)
}
