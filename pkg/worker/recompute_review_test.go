package worker

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestProcessRecomputeReviewJob_PopulatesReviewed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tc := newTestContext(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := tc.db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "T",
		TitleSource:     "filepath",
		SortTitle:       "T",
		SortTitleSource: "filepath",
		AuthorSource:    "filepath",
		Filepath:        "/tmp",
	}
	_, err = tc.db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	main := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/m.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1,
	}
	_, err = tc.db.NewInsert().Model(main).Exec(ctx)
	require.NoError(t, err)
	supp := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/s.pdf",
		FileType:      models.FileTypePDF,
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: 1,
	}
	_, err = tc.db.NewInsert().Model(supp).Exec(ctx)
	require.NoError(t, err)

	job := &models.Job{
		Type:   models.JobTypeRecomputeReview,
		Status: models.JobStatusPending,
		Data:   `{"clear_overrides":false}`,
	}
	_, err = tc.db.NewInsert().Model(job).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, tc.worker.ProcessRecomputeReviewJob(ctx, job, nil))

	var mainReviewed *bool
	require.NoError(t, tc.db.NewSelect().Table("files").Column("reviewed").Where("id = ?", main.ID).Scan(ctx, &mainReviewed))
	require.NotNil(t, mainReviewed)
	require.False(t, *mainReviewed)

	var suppReviewed *bool
	require.NoError(t, tc.db.NewSelect().Table("files").Column("reviewed").Where("id = ?", supp.ID).Scan(ctx, &suppReviewed))
	require.Nil(t, suppReviewed)
}

func TestProcessRecomputeReviewJob_ClearOverrides(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tc := newTestContext(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := tc.db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "T",
		TitleSource:     "filepath",
		SortTitle:       "T",
		SortTitleSource: "filepath",
		AuthorSource:    "filepath",
		Filepath:        "/tmp",
	}
	_, err = tc.db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	override := models.ReviewOverrideReviewed
	file := &models.File{
		LibraryID:      library.ID,
		BookID:         book.ID,
		Filepath:       "/tmp/x.epub",
		FileType:       models.FileTypeEPUB,
		FileRole:       models.FileRoleMain,
		FilesizeBytes:  1,
		ReviewOverride: &override,
	}
	_, err = tc.db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	job := &models.Job{
		Type:   models.JobTypeRecomputeReview,
		Status: models.JobStatusPending,
		Data:   `{"clear_overrides":true}`,
	}
	_, err = tc.db.NewInsert().Model(job).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, tc.worker.ProcessRecomputeReviewJob(ctx, job, nil))

	var got models.File
	require.NoError(t, tc.db.NewSelect().Model(&got).Where("f.id = ?", file.ID).Scan(ctx))
	require.Nil(t, got.ReviewOverride)
	require.Nil(t, got.ReviewOverriddenAt)
	require.NotNil(t, got.Reviewed)
	require.False(t, *got.Reviewed)
}
