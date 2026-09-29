package books

import (
	"context"
	"fmt"
	"testing"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// insertBookMarkedReviewed inserts a Book with one EPUB File that is stored
// as reviewed even though the Book meets none of the default criteria, so a
// recompute flips it to not reviewed.
func insertBookMarkedReviewed(ctx context.Context, t *testing.T, db *bun.DB, lib *models.Library, title string) int {
	t.Helper()
	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           title,
		TitleSource:     models.DataSourceManual,
		SortTitle:       title,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err := db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	reviewed := true
	file := &models.File{
		LibraryID:     lib.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      fmt.Sprintf("%s/%s.epub", book.Filepath, title),
		FilesizeBytes: 1,
		Reviewed:      &reviewed,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)
	return file.ID
}

func fileReviewed(ctx context.Context, t *testing.T, db *bun.DB, fileID int) bool {
	t.Helper()
	var file models.File
	require.NoError(t, db.NewSelect().Model(&file).Where("f.id = ?", fileID).Scan(ctx))
	require.NotNil(t, file.Reviewed)
	return *file.Reviewed
}

// RecomputeReviewedForBooks refreshes every listed Book and leaves the rest
// alone.
func TestRecomputeReviewedForBooks_RecomputesOnlyListedBooks(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	lib := &models.Library{Name: "Library", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)

	first := insertBookMarkedReviewed(ctx, t, db, lib, "First")
	second := insertBookMarkedReviewed(ctx, t, db, lib, "Second")
	unlisted := insertBookMarkedReviewed(ctx, t, db, lib, "Unlisted")
	var bookIDs []int
	for _, fileID := range []int{first, second} {
		var file models.File
		require.NoError(t, db.NewSelect().Model(&file).Where("f.id = ?", fileID).Scan(ctx))
		bookIDs = append(bookIDs, file.BookID)
	}

	NewService(db).WithAppSettings(appsettings.NewService(db)).RecomputeReviewedForBooks(ctx, bookIDs)

	assert.False(t, fileReviewed(ctx, t, db, first))
	assert.False(t, fileReviewed(ctx, t, db, second))
	assert.True(t, fileReviewed(ctx, t, db, unlisted), "a Book that was not listed is not recomputed")
}
