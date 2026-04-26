package review

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestRecomputeForFile_Incomplete_SetsFalse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{LibraryID: library.ID, Title: "T", TitleSource: "filepath", Filepath: "/tmp", SortTitle: "T", SortTitleSource: "filepath", AuthorSource: "filepath"}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/x.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, RecomputeForFile(ctx, db, file.ID, Default()))

	var got bool
	err = db.NewSelect().Table("files").Column("reviewed").Where("id = ?", file.ID).Scan(ctx, &got)
	require.NoError(t, err)
	require.False(t, got)
}

func TestRecomputeForFile_OverrideReviewed_ShortCircuits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{LibraryID: library.ID, Title: "T", TitleSource: "filepath", Filepath: "/tmp", SortTitle: "T", SortTitleSource: "filepath", AuthorSource: "filepath"}
	_, err = db.NewInsert().Model(book).Exec(ctx)
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
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// File has no metadata, but override should win
	require.NoError(t, RecomputeForFile(ctx, db, file.ID, Default()))

	var got bool
	err = db.NewSelect().Table("files").Column("reviewed").Where("id = ?", file.ID).Scan(ctx, &got)
	require.NoError(t, err)
	require.True(t, got)
}

func TestRecomputeForFile_Supplement_SetsNull(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{LibraryID: library.ID, Title: "T", TitleSource: "filepath", Filepath: "/tmp", SortTitle: "T", SortTitleSource: "filepath", AuthorSource: "filepath"}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/x.pdf",
		FileType:      models.FileTypePDF,
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: 1,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, RecomputeForFile(ctx, db, file.ID, Default()))

	var got *bool
	err = db.NewSelect().Table("files").Column("reviewed").Where("id = ?", file.ID).Scan(ctx, &got)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestSetOverride_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{LibraryID: library.ID, Title: "T", TitleSource: "filepath", Filepath: "/tmp", SortTitle: "T", SortTitleSource: "filepath", AuthorSource: "filepath"}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/x.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	v := models.ReviewOverrideReviewed
	require.NoError(t, SetOverride(ctx, db, file.ID, &v, Default()))

	var got models.File
	err = db.NewSelect().Model(&got).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, got.ReviewOverride)
	require.Equal(t, "reviewed", *got.ReviewOverride)
	require.NotNil(t, got.ReviewOverriddenAt)
	require.NotNil(t, got.Reviewed)
	require.True(t, *got.Reviewed)

	// Clear
	require.NoError(t, SetOverride(ctx, db, file.ID, nil, Default()))
	got = models.File{}
	err = db.NewSelect().Model(&got).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.Nil(t, got.ReviewOverride)
	require.Nil(t, got.ReviewOverriddenAt)
	// Now driven by completeness — incomplete book → false
	require.False(t, *got.Reviewed)
}
