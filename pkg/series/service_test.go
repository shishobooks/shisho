package series

import (
	"context"
	"database/sql"
	"testing"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
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

// TestDeleteSeries_RemovesBookSeriesAndFlipsReviewed is a regression test for
// the bug where soft-deleting a series left orphan book_series rows behind,
// so the Reviewed-flag completeness check (len(book.BookSeries) > 0) would
// still consider the book to have a series even though no UI surfaces it.
//
// After deletion the join rows must be gone and a reviewed-recompute must
// flip the file back to reviewed=false when `series` is a required field.
func TestDeleteSeries_RemovesBookSeriesAndFlipsReviewed(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceManual,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/tmp/test.epub",
		FilesizeBytes: 1,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	s := &models.Series{
		LibraryID:      library.ID,
		Name:           "Test Series",
		NameSource:     models.DataSourceManual,
		SortName:       "Test Series",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(s).Exec(ctx)
	require.NoError(t, err)

	bs := &models.BookSeries{BookID: book.ID, SeriesID: s.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(bs).Exec(ctx)
	require.NoError(t, err)

	// Configure review criteria so that `series` is the only thing keeping the
	// file reviewed=true. Then recompute and confirm reviewed=true.
	settings := appsettings.NewService(db)
	require.NoError(t, review.Save(ctx, settings, review.Criteria{
		BookFields:  []string{review.FieldSeries},
		AudioFields: nil,
	}))

	bookSvc := books.NewService(db).WithAppSettings(settings)
	bookSvc.RecomputeReviewedForBook(ctx, book.ID)

	var pre models.File
	require.NoError(t, db.NewSelect().Model(&pre).Where("f.id = ?", file.ID).Scan(ctx))
	require.NotNil(t, pre.Reviewed)
	require.True(t, *pre.Reviewed, "file should be reviewed when its only required field (series) is satisfied")

	// Delete the series. DeleteSeries should hard-delete the join rows and
	// return the affected book IDs so the caller can recompute.
	svc := NewService(db)
	affected, err := svc.DeleteSeries(ctx, s.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{book.ID}, affected)

	// book_series rows for this series must be gone.
	count, err := db.NewSelect().Model((*models.BookSeries)(nil)).Where("series_id = ?", s.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "book_series rows pointing at the deleted series must be removed")

	// Recompute (mirrors what the handler will do for each affected book).
	bookSvc.RecomputeReviewedForBook(ctx, book.ID)

	var post models.File
	require.NoError(t, db.NewSelect().Model(&post).Where("f.id = ?", file.ID).Scan(ctx))
	require.NotNil(t, post.Reviewed)
	assert.False(t, *post.Reviewed, "file should flip back to unreviewed once the series is gone")
}
