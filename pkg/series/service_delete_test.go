package series

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func createSeriesDeleteLibrary(t *testing.T, db *bun.DB) *models.Library {
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

func createNamedSeries(t *testing.T, svc *Service, lib *models.Library, name string) *models.Series {
	t.Helper()
	s := &models.Series{LibraryID: lib.ID, Name: name, NameSource: models.DataSourceManual}
	require.NoError(t, svc.CreateSeries(context.Background(), s))
	return s
}

// createSeriesBook inserts a Book with the given series_source that is a
// member of every listed Series, in order, and returns its ID.
func createSeriesBook(t *testing.T, db *bun.DB, lib *models.Library, source *string, seriesIDs ...int) int {
	t.Helper()
	ctx := context.Background()

	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		SeriesSource:    source,
		Filepath:        t.TempDir(),
	}
	_, err := db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	for i, seriesID := range seriesIDs {
		_, err = db.NewInsert().Model(&models.BookSeries{BookID: book.ID, SeriesID: seriesID, SortOrder: i + 1}).Exec(ctx)
		require.NoError(t, err)
	}
	return book.ID
}

func retrieveSeriesBook(t *testing.T, db *bun.DB, bookID int) (*models.Book, []int) {
	t.Helper()
	ctx := context.Background()
	book := &models.Book{}
	require.NoError(t, db.NewSelect().Model(book).Where("b.id = ?", bookID).Scan(ctx))
	var seriesIDs []int
	require.NoError(t, db.NewSelect().Model((*models.BookSeries)(nil)).Column("series_id").Where("book_id = ?", bookID).Order("sort_order").Scan(ctx, &seriesIDs))
	return book, seriesIDs
}

// Deleting a shared Series is a deliberate user action, so every member Book
// gets a manual series_source, whatever its prior source and even when it
// stays in other Series. An ordinary Scan then leaves the memberships alone
// instead of re-creating the Series from a sidecar or the file (ADR 0006).
func TestDeleteSeries_StampsManualSeriesSourceOnEveryMemberBook(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createSeriesDeleteLibrary(t, db)
	deleted := createNamedSeries(t, svc, lib, "Deleted")
	kept := createNamedSeries(t, svc, lib, "Kept")

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
		emptied = append(emptied, createSeriesBook(t, db, lib, source, deleted.ID))
	}
	pluginSource := testgen.StringPtr("plugin:test/enricher")
	partial := createSeriesBook(t, db, lib, pluginSource, deleted.ID, kept.ID)
	untouched := createSeriesBook(t, db, lib, pluginSource, kept.ID)

	affected, err := svc.DeleteSeries(ctx, deleted.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, append(append([]int{}, emptied...), partial), affected)

	for i, bookID := range emptied {
		book, seriesIDs := retrieveSeriesBook(t, db, bookID)
		assert.Empty(t, seriesIDs, "prior source %d: the membership is removed", i)
		if assert.NotNil(t, book.SeriesSource, "prior source %d: series_source is stamped", i) {
			assert.Equal(t, models.DataSourceManual, *book.SeriesSource, "prior source %d", i)
		}
	}

	book, seriesIDs := retrieveSeriesBook(t, db, partial)
	assert.Equal(t, []int{kept.ID}, seriesIDs, "the remaining membership is kept")
	if assert.NotNil(t, book.SeriesSource) {
		assert.Equal(t, models.DataSourceManual, *book.SeriesSource, "a partially changed collection is stamped too")
	}

	book, seriesIDs = retrieveSeriesBook(t, db, untouched)
	assert.Equal(t, []int{kept.ID}, seriesIDs)
	assert.Equal(t, pluginSource, book.SeriesSource, "Books outside the deleted Series keep their source")

	_, err = svc.RetrieveSeries(ctx, RetrieveSeriesOptions{ID: &deleted.ID})
	require.Error(t, err, "the Series itself is deleted")
}

// A Series with no member Books deletes cleanly and touches no Book.
func TestDeleteSeries_Unused_TouchesNoBooks(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createSeriesDeleteLibrary(t, db)
	unused := createNamedSeries(t, svc, lib, "Unused")
	other := createNamedSeries(t, svc, lib, "Other")
	pluginSource := testgen.StringPtr("plugin:test/enricher")
	bookID := createSeriesBook(t, db, lib, pluginSource, other.ID)

	affected, err := svc.DeleteSeries(ctx, unused.ID)
	require.NoError(t, err)
	assert.Empty(t, affected)

	book, seriesIDs := retrieveSeriesBook(t, db, bookID)
	assert.Equal(t, []int{other.ID}, seriesIDs)
	assert.Equal(t, pluginSource, book.SeriesSource)
}

// Merging re-points memberships at the target. It is not a clear, so the
// Books keep their existing source.
func TestMergeSeries_KeepsBookSources(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createSeriesDeleteLibrary(t, db)
	source := createNamedSeries(t, svc, lib, "Source")
	target := createNamedSeries(t, svc, lib, "Target")
	pluginSource := testgen.StringPtr("plugin:test/enricher")
	bookID := createSeriesBook(t, db, lib, pluginSource, source.ID)

	_, err := svc.MergeSeries(ctx, target.ID, source.ID)
	require.NoError(t, err)

	book, seriesIDs := retrieveSeriesBook(t, db, bookID)
	assert.Equal(t, []int{target.ID}, seriesIDs)
	assert.Equal(t, pluginSource, book.SeriesSource)
}
