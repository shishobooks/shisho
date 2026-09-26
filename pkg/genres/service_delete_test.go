package genres

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// createGenreBook inserts a Book with the given genre_source that carries
// every listed Genre, and returns its ID.
func createGenreBook(t *testing.T, db *bun.DB, lib *models.Library, source *string, genreIDs ...int) int {
	t.Helper()
	ctx := context.Background()

	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		GenreSource:     source,
		Filepath:        t.TempDir(),
	}
	_, err := db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	for _, genreID := range genreIDs {
		_, err = db.NewInsert().Model(&models.BookGenre{BookID: book.ID, GenreID: genreID}).Exec(ctx)
		require.NoError(t, err)
	}
	return book.ID
}

func retrieveGenreBook(t *testing.T, db *bun.DB, bookID int) (*models.Book, []int) {
	t.Helper()
	ctx := context.Background()
	book := &models.Book{}
	require.NoError(t, db.NewSelect().Model(book).Where("b.id = ?", bookID).Scan(ctx))
	var genreIDs []int
	require.NoError(t, db.NewSelect().Model((*models.BookGenre)(nil)).Column("genre_id").Where("book_id = ?", bookID).Order("genre_id").Scan(ctx, &genreIDs))
	return book, genreIDs
}

// Deleting a shared Genre is a deliberate user action, so every Book that
// carried it gets a manual genre_source, whatever its prior source and even
// when other Genres remain. An ordinary Scan then leaves the collection
// alone instead of re-creating the Genre from a sidecar or the file
// (ADR 0006).
func TestDeleteGenre_StampsManualGenreSourceOnEveryAffectedBook(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	deleted := &models.Genre{LibraryID: lib.ID, Name: "Deleted"}
	require.NoError(t, svc.CreateGenre(ctx, deleted))
	kept := &models.Genre{LibraryID: lib.ID, Name: "Kept"}
	require.NoError(t, svc.CreateGenre(ctx, kept))

	priorSources := []*string{
		testgen.StringPtr(models.DataSourceManual),
		testgen.StringPtr(models.DataSourceSidecar),
		testgen.StringPtr("plugin:test/enricher"),
		testgen.StringPtr(models.DataSourcePlugin),
		testgen.StringPtr(models.DataSourceM4BMetadata),
		testgen.StringPtr(models.DataSourceFilepath),
		nil,
	}
	var emptied []int
	for _, source := range priorSources {
		emptied = append(emptied, createGenreBook(t, db, lib, source, deleted.ID))
	}
	pluginSource := testgen.StringPtr("plugin:test/enricher")
	partial := createGenreBook(t, db, lib, pluginSource, deleted.ID, kept.ID)
	untouched := createGenreBook(t, db, lib, pluginSource, kept.ID)

	require.NoError(t, svc.DeleteGenre(ctx, deleted.ID))

	for i, bookID := range emptied {
		book, genreIDs := retrieveGenreBook(t, db, bookID)
		assert.Empty(t, genreIDs, "prior source %d: the join row is removed", i)
		if assert.NotNil(t, book.GenreSource, "prior source %d: genre_source is stamped", i) {
			assert.Equal(t, models.DataSourceManual, *book.GenreSource, "prior source %d", i)
		}
	}

	book, genreIDs := retrieveGenreBook(t, db, partial)
	assert.Equal(t, []int{kept.ID}, genreIDs, "the remaining Genre is kept")
	if assert.NotNil(t, book.GenreSource) {
		assert.Equal(t, models.DataSourceManual, *book.GenreSource, "a partially changed collection is stamped too")
	}

	book, genreIDs = retrieveGenreBook(t, db, untouched)
	assert.Equal(t, []int{kept.ID}, genreIDs)
	assert.Equal(t, pluginSource, book.GenreSource, "Books without the deleted Genre keep their source")

	_, err := svc.RetrieveGenre(ctx, RetrieveGenreOptions{ID: &deleted.ID})
	require.Error(t, err, "the Genre itself is deleted")
}

// A Genre that no Book carries deletes cleanly and touches no Book.
func TestDeleteGenre_Unused_TouchesNoBooks(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	unused := &models.Genre{LibraryID: lib.ID, Name: "Unused"}
	require.NoError(t, svc.CreateGenre(ctx, unused))
	other := &models.Genre{LibraryID: lib.ID, Name: "Other"}
	require.NoError(t, svc.CreateGenre(ctx, other))
	pluginSource := testgen.StringPtr("plugin:test/enricher")
	bookID := createGenreBook(t, db, lib, pluginSource, other.ID)

	require.NoError(t, svc.DeleteGenre(ctx, unused.ID))

	book, genreIDs := retrieveGenreBook(t, db, bookID)
	assert.Equal(t, []int{other.ID}, genreIDs)
	assert.Equal(t, pluginSource, book.GenreSource)
}

// Merging re-points Books at the target. It is not a clear, so the Books
// keep their existing source.
func TestMergeGenres_KeepsBookSources(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	source := &models.Genre{LibraryID: lib.ID, Name: "Source"}
	require.NoError(t, svc.CreateGenre(ctx, source))
	target := &models.Genre{LibraryID: lib.ID, Name: "Target"}
	require.NoError(t, svc.CreateGenre(ctx, target))

	pluginSource := testgen.StringPtr("plugin:test/enricher")
	moved := createGenreBook(t, db, lib, pluginSource, source.ID)
	both := createGenreBook(t, db, lib, testgen.StringPtr(models.DataSourceM4BMetadata), source.ID, target.ID)

	require.NoError(t, svc.MergeGenres(ctx, target.ID, source.ID))

	book, genreIDs := retrieveGenreBook(t, db, moved)
	assert.Equal(t, []int{target.ID}, genreIDs)
	assert.Equal(t, pluginSource, book.GenreSource)

	book, genreIDs = retrieveGenreBook(t, db, both)
	assert.Equal(t, []int{target.ID}, genreIDs)
	assert.Equal(t, testgen.StringPtr(models.DataSourceM4BMetadata), book.GenreSource)
}
