package tags

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// createTagBook inserts a Book with the given tag_source that carries
// every listed Tag, and returns its ID.
func createTagBook(t *testing.T, db *bun.DB, lib *models.Library, source *string, tagIDs ...int) int {
	t.Helper()
	ctx := context.Background()

	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		TagSource:       source,
		Filepath:        t.TempDir(),
	}
	_, err := db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	for _, tagID := range tagIDs {
		_, err = db.NewInsert().Model(&models.BookTag{BookID: book.ID, TagID: tagID}).Exec(ctx)
		require.NoError(t, err)
	}
	return book.ID
}

func retrieveTagBook(t *testing.T, db *bun.DB, bookID int) (*models.Book, []int) {
	t.Helper()
	ctx := context.Background()
	book := &models.Book{}
	require.NoError(t, db.NewSelect().Model(book).Where("b.id = ?", bookID).Scan(ctx))
	var tagIDs []int
	require.NoError(t, db.NewSelect().Model((*models.BookTag)(nil)).Column("tag_id").Where("book_id = ?", bookID).Order("tag_id").Scan(ctx, &tagIDs))
	return book, tagIDs
}

// Deleting a shared Tag is a deliberate user action, so every Book that
// carried it gets a manual tag_source, whatever its prior source and even
// when other Tags remain. An ordinary Scan then leaves the collection
// alone instead of re-creating the Tag from a sidecar or the file
// (ADR 0006).
func TestDeleteTag_StampsManualTagSourceOnEveryAffectedBook(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	deleted := &models.Tag{LibraryID: lib.ID, Name: "Deleted"}
	require.NoError(t, svc.CreateTag(ctx, deleted))
	kept := &models.Tag{LibraryID: lib.ID, Name: "Kept"}
	require.NoError(t, svc.CreateTag(ctx, kept))

	priorSources := []*string{
		testgen.StringPtr(models.DataSourceManual),
		testgen.StringPtr(models.DataSourceSidecar),
		testgen.StringPtr("plugin:test/enricher"),
		testgen.StringPtr(models.DataSourcePlugin),
		testgen.StringPtr(models.DataSourceEPUBMetadata),
		testgen.StringPtr(models.DataSourceFilepath),
		nil,
	}
	var emptied []int
	for _, source := range priorSources {
		emptied = append(emptied, createTagBook(t, db, lib, source, deleted.ID))
	}
	pluginSource := testgen.StringPtr("plugin:test/enricher")
	partial := createTagBook(t, db, lib, pluginSource, deleted.ID, kept.ID)
	untouched := createTagBook(t, db, lib, pluginSource, kept.ID)

	require.NoError(t, svc.DeleteTag(ctx, deleted.ID))

	for i, bookID := range emptied {
		book, tagIDs := retrieveTagBook(t, db, bookID)
		assert.Empty(t, tagIDs, "prior source %d: the join row is removed", i)
		if assert.NotNil(t, book.TagSource, "prior source %d: tag_source is stamped", i) {
			assert.Equal(t, models.DataSourceManual, *book.TagSource, "prior source %d", i)
		}
	}

	book, tagIDs := retrieveTagBook(t, db, partial)
	assert.Equal(t, []int{kept.ID}, tagIDs, "the remaining Tag is kept")
	if assert.NotNil(t, book.TagSource) {
		assert.Equal(t, models.DataSourceManual, *book.TagSource, "a partially changed collection is stamped too")
	}

	book, tagIDs = retrieveTagBook(t, db, untouched)
	assert.Equal(t, []int{kept.ID}, tagIDs)
	assert.Equal(t, pluginSource, book.TagSource, "Books without the deleted Tag keep their source")

	_, err := svc.RetrieveTag(ctx, RetrieveTagOptions{ID: &deleted.ID})
	require.Error(t, err, "the Tag itself is deleted")
}

// A Tag that no Book carries deletes cleanly and touches no Book.
func TestDeleteTag_Unused_TouchesNoBooks(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	unused := &models.Tag{LibraryID: lib.ID, Name: "Unused"}
	require.NoError(t, svc.CreateTag(ctx, unused))
	other := &models.Tag{LibraryID: lib.ID, Name: "Other"}
	require.NoError(t, svc.CreateTag(ctx, other))
	pluginSource := testgen.StringPtr("plugin:test/enricher")
	bookID := createTagBook(t, db, lib, pluginSource, other.ID)

	require.NoError(t, svc.DeleteTag(ctx, unused.ID))

	book, tagIDs := retrieveTagBook(t, db, bookID)
	assert.Equal(t, []int{other.ID}, tagIDs)
	assert.Equal(t, pluginSource, book.TagSource)
}

// Merging re-points Books at the target. It is not a clear, so the Books
// keep their existing source.
func TestMergeTags_KeepsBookSources(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	source := &models.Tag{LibraryID: lib.ID, Name: "Source"}
	require.NoError(t, svc.CreateTag(ctx, source))
	target := &models.Tag{LibraryID: lib.ID, Name: "Target"}
	require.NoError(t, svc.CreateTag(ctx, target))

	pluginSource := testgen.StringPtr("plugin:test/enricher")
	moved := createTagBook(t, db, lib, pluginSource, source.ID)
	both := createTagBook(t, db, lib, testgen.StringPtr(models.DataSourceEPUBMetadata), source.ID, target.ID)

	require.NoError(t, svc.MergeTags(ctx, target.ID, source.ID))

	book, tagIDs := retrieveTagBook(t, db, moved)
	assert.Equal(t, []int{target.ID}, tagIDs)
	assert.Equal(t, pluginSource, book.TagSource)

	book, tagIDs = retrieveTagBook(t, db, both)
	assert.Equal(t, []int{target.ID}, tagIDs)
	assert.Equal(t, testgen.StringPtr(models.DataSourceEPUBMetadata), book.TagSource)
}
