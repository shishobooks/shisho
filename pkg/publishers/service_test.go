package publishers

import (
	"context"
	"database/sql"
	"fmt"
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

func TestMergePublishers_TargetIsChildOfSource_NoSelfReference(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	// Create hierarchy: source -> target (target is child of source)
	source := &models.Publisher{LibraryID: lib.ID, Name: "Source"}
	err := svc.CreatePublisher(ctx, source)
	require.NoError(t, err)

	target := &models.Publisher{LibraryID: lib.ID, Name: "Target", ParentID: &source.ID}
	err = svc.CreatePublisher(ctx, target)
	require.NoError(t, err)

	// Also give source another child to verify it gets re-parented to target
	sibling := &models.Publisher{LibraryID: lib.ID, Name: "Sibling", ParentID: &source.ID}
	err = svc.CreatePublisher(ctx, sibling)
	require.NoError(t, err)

	// Merge source into target
	err = svc.MergePublishers(ctx, target.ID, source.ID)
	require.NoError(t, err)

	// Target must NOT have a self-reference (parent_id must not be target.ID)
	updated, err := svc.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &target.ID})
	require.NoError(t, err)
	if updated.ParentID != nil {
		assert.NotEqual(t, target.ID, *updated.ParentID, "target must not have self-reference")
	}
	// Target's parent should be nil (it was child of source, source is deleted)
	assert.Nil(t, updated.ParentID, "target parent_id should be cleared since source is deleted")

	// Sibling should now be re-parented to target
	updatedSibling, err := svc.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &sibling.ID})
	require.NoError(t, err)
	require.NotNil(t, updatedSibling.ParentID)
	assert.Equal(t, target.ID, *updatedSibling.ParentID, "sibling should be re-parented to target")

	// Source should be deleted
	_, err = svc.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &source.ID})
	require.Error(t, err)
}

func TestCleanupOrphanedPublishers_PreservesParents(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	// Create a parent publisher with no files but with a child
	parent := &models.Publisher{LibraryID: lib.ID, Name: "Parent"}
	err := svc.CreatePublisher(ctx, parent)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Child", ParentID: &parent.ID}
	err = svc.CreatePublisher(ctx, child)
	require.NoError(t, err)

	// Create a truly orphaned publisher (no files, no children)
	orphan := &models.Publisher{LibraryID: lib.ID, Name: "Orphan"}
	err = svc.CreatePublisher(ctx, orphan)
	require.NoError(t, err)

	// Run cleanup
	deletedIDs, err := svc.CleanupOrphanedPublishers(ctx)
	require.NoError(t, err)

	// Orphan and child (no files, no children) should be deleted
	assert.Contains(t, deletedIDs, orphan.ID, "orphan with no files and no children should be deleted")
	assert.Contains(t, deletedIDs, child.ID, "child with no files and no children should be deleted")

	// Parent should be preserved because it has a child
	assert.NotContains(t, deletedIDs, parent.ID, "parent with children should be preserved")

	// Verify parent still exists
	_, err = svc.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &parent.ID})
	require.NoError(t, err)
}

func TestSetParent_CrossLibraryRejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib1 := createTestLibrary(t, db)

	// Create a second library
	lib2 := &models.Library{
		Name:                     "Second Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib2).Exec(ctx)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib1.ID, Name: "Child"}
	err = svc.CreatePublisher(ctx, child)
	require.NoError(t, err)

	parent := &models.Publisher{LibraryID: lib2.ID, Name: "Parent In Other Library"}
	err = svc.CreatePublisher(ctx, parent)
	require.NoError(t, err)

	// Attempt to set parent from a different library
	err = svc.SetParent(ctx, child.ID, &parent.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "same library")
}

func TestSetParent_ZeroParentIDRejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	pub := &models.Publisher{LibraryID: lib.ID, Name: "Publisher"}
	err := svc.CreatePublisher(ctx, pub)
	require.NoError(t, err)

	// parent_id: 0 should be rejected
	zero := 0
	err = svc.SetParent(ctx, pub.ID, &zero)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid parent")
}

func TestSetParent_NegativeParentIDRejected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	pub := &models.Publisher{LibraryID: lib.ID, Name: "Publisher"}
	err := svc.CreatePublisher(ctx, pub)
	require.NoError(t, err)

	// parent_id: -1 should be rejected
	neg := -1
	err = svc.SetParent(ctx, pub.ID, &neg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid parent")
}

func TestValidateNoCycle_NonExistentParentReturnsError(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	pub := &models.Publisher{LibraryID: lib.ID, Name: "Publisher"}
	err := svc.CreatePublisher(ctx, pub)
	require.NoError(t, err)

	// proposedParentID that doesn't exist
	err = svc.ValidateNoCycle(ctx, pub.ID, 99999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parent publisher not found")
}

func createTestFile(t *testing.T, db *bun.DB, lib *models.Library, publisherID int, filepath string) {
	t.Helper()
	ctx := context.Background()

	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err := db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     lib.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      filepath,
		FilesizeBytes: 1,
		PublisherID:   &publisherID,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)
}

func TestGetFileCount_IncludesDescendantFiles(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	// Create hierarchy: root -> child -> grandchild
	root := &models.Publisher{LibraryID: lib.ID, Name: "Root"}
	err := svc.CreatePublisher(ctx, root)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Child", ParentID: &root.ID}
	err = svc.CreatePublisher(ctx, child)
	require.NoError(t, err)

	grandchild := &models.Publisher{LibraryID: lib.ID, Name: "Grandchild", ParentID: &child.ID}
	err = svc.CreatePublisher(ctx, grandchild)
	require.NoError(t, err)

	// Create files for each publisher
	createTestFile(t, db, lib, root.ID, "/tmp/root-file.epub")
	createTestFile(t, db, lib, child.ID, "/tmp/child-file.epub")
	createTestFile(t, db, lib, grandchild.ID, "/tmp/grandchild-file.epub")

	// Root should count files from self + child + grandchild = 3
	count, err := svc.GetFileCount(ctx, root.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Child should count files from self + grandchild = 2
	count, err = svc.GetFileCount(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Grandchild (leaf) should count only its own files = 1
	count, err = svc.GetFileCount(ctx, grandchild.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestGetFilesPaginated_IncludesDescendantFiles(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	// Create hierarchy: root -> child
	root := &models.Publisher{LibraryID: lib.ID, Name: "Root"}
	err := svc.CreatePublisher(ctx, root)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Child", ParentID: &root.ID}
	err = svc.CreatePublisher(ctx, child)
	require.NoError(t, err)

	// Create files
	createTestFile(t, db, lib, root.ID, "/tmp/root-file.epub")
	createTestFile(t, db, lib, child.ID, "/tmp/child-file.epub")

	// Root paginated query should return both files
	files, total, err := svc.GetFilesPaginated(ctx, root.ID, 50, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, files, 2)

	// Child paginated query should return only child's file
	files, total, err = svc.GetFilesPaginated(ctx, child.ID, 50, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, files, 1)
}

func TestGetFilesPaginated_PaginatesCorrectlyWithDescendants(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	// Create hierarchy: root -> child
	root := &models.Publisher{LibraryID: lib.ID, Name: "Root"}
	err := svc.CreatePublisher(ctx, root)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Child", ParentID: &root.ID}
	err = svc.CreatePublisher(ctx, child)
	require.NoError(t, err)

	// Create 3 files total (1 root, 2 child)
	createTestFile(t, db, lib, root.ID, "/tmp/a-root.epub")
	createTestFile(t, db, lib, child.ID, "/tmp/b-child1.epub")
	createTestFile(t, db, lib, child.ID, "/tmp/c-child2.epub")

	// Page 1 of size 2 should return 2 items, total 3
	files, total, err := svc.GetFilesPaginated(ctx, root.ID, 2, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, files, 2)

	// Page 2 of size 2 should return 1 item, total 3
	files, total, err = svc.GetFilesPaginated(ctx, root.ID, 2, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, files, 1)
}

func TestGetFiles_IncludesDescendantFiles(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	// Create hierarchy: root -> child
	root := &models.Publisher{LibraryID: lib.ID, Name: "Root"}
	err := svc.CreatePublisher(ctx, root)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Child", ParentID: &root.ID}
	err = svc.CreatePublisher(ctx, child)
	require.NoError(t, err)

	createTestFile(t, db, lib, root.ID, "/tmp/root-file.epub")
	createTestFile(t, db, lib, child.ID, "/tmp/child-file.epub")

	// Root GetFiles should return both files
	files, err := svc.GetFiles(ctx, root.ID)
	require.NoError(t, err)
	assert.Len(t, files, 2)

	// Child GetFiles should return only child's file
	files, err = svc.GetFiles(ctx, child.ID)
	require.NoError(t, err)
	assert.Len(t, files, 1)
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

// createFilesForPublisher creates a book and the specified number of files
// associated with the given publisher.
func createFilesForPublisher(t *testing.T, db *bun.DB, lib *models.Library, publisherID int, count int) {
	t.Helper()
	ctx := context.Background()

	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err := db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	for i := 0; i < count; i++ {
		file := &models.File{
			LibraryID:     lib.ID,
			BookID:        book.ID,
			FileType:      models.FileTypeEPUB,
			FileRole:      models.FileRoleMain,
			Filepath:      fmt.Sprintf("/tmp/pub%d_file%d.epub", publisherID, i),
			FilesizeBytes: 1,
			PublisherID:   &publisherID,
		}
		_, err = db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)
	}
}

func TestGetChildren_ReturnsDirectChildrenWithFileCount(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	parent := &models.Publisher{LibraryID: lib.ID, Name: "Parent"}
	err := svc.CreatePublisher(ctx, parent)
	require.NoError(t, err)

	childA := &models.Publisher{LibraryID: lib.ID, Name: "ChildA", ParentID: &parent.ID}
	err = svc.CreatePublisher(ctx, childA)
	require.NoError(t, err)

	childB := &models.Publisher{LibraryID: lib.ID, Name: "ChildB", ParentID: &parent.ID}
	err = svc.CreatePublisher(ctx, childB)
	require.NoError(t, err)

	// Grandchild should NOT appear in children of parent
	grandchild := &models.Publisher{LibraryID: lib.ID, Name: "Grandchild", ParentID: &childA.ID}
	err = svc.CreatePublisher(ctx, grandchild)
	require.NoError(t, err)

	// Create files for children
	createFilesForPublisher(t, db, lib, childA.ID, 3)
	createFilesForPublisher(t, db, lib, childB.ID, 1)

	children, err := svc.GetChildren(ctx, parent.ID)
	require.NoError(t, err)
	require.Len(t, children, 2)

	// Children should be ordered by name
	assert.Equal(t, "ChildA", children[0].Name)
	assert.Equal(t, 3, children[0].FileCount)
	assert.Equal(t, "ChildB", children[1].Name)
	assert.Equal(t, 1, children[1].FileCount)
}

func TestGetChildren_NoChildren(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	leaf := &models.Publisher{LibraryID: lib.ID, Name: "Leaf"}
	err := svc.CreatePublisher(ctx, leaf)
	require.NoError(t, err)

	children, err := svc.GetChildren(ctx, leaf.ID)
	require.NoError(t, err)
	assert.Empty(t, children)
}

func TestGetDescendantFileCount_CountsAllDescendantFiles(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	root := &models.Publisher{LibraryID: lib.ID, Name: "Root"}
	err := svc.CreatePublisher(ctx, root)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Child", ParentID: &root.ID}
	err = svc.CreatePublisher(ctx, child)
	require.NoError(t, err)

	grandchild := &models.Publisher{LibraryID: lib.ID, Name: "Grandchild", ParentID: &child.ID}
	err = svc.CreatePublisher(ctx, grandchild)
	require.NoError(t, err)

	// Create files at different levels
	createFilesForPublisher(t, db, lib, child.ID, 2)
	createFilesForPublisher(t, db, lib, grandchild.ID, 3)

	count, err := svc.GetDescendantFileCount(ctx, root.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, count) // 2 + 3

	// Child's descendants have 3 files (grandchild only)
	count, err = svc.GetDescendantFileCount(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Grandchild has no descendants
	count, err = svc.GetDescendantFileCount(ctx, grandchild.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestGetDescendantPublisherCount_CountsAllDescendantPublishers(t *testing.T) {
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

	count, err := svc.GetDescendantPublisherCount(ctx, root.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, count) // childA + childB + grandchild

	count, err = svc.GetDescendantPublisherCount(ctx, childA.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count) // grandchild only

	count, err = svc.GetDescendantPublisherCount(ctx, childB.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, count) // no children
}
