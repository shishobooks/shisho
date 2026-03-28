package books

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func TestDeleteFilesByIDs(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create library and book
	_, book := setupTestLibraryAndBook(t, db)

	// Create two files using CreateFile (which also sets the primary file)
	file1 := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/audiobook1.m4b",
		FilesizeBytes: 1000,
	}
	err := svc.CreateFile(ctx, file1)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/audiobook2.m4b",
		FilesizeBytes: 2000,
	}
	err = svc.CreateFile(ctx, file2)
	require.NoError(t, err)

	// Create persons for narrators
	now := time.Now()
	person1 := &models.Person{
		LibraryID:      book.LibraryID,
		Name:           "Narrator One",
		SortName:       "One, Narrator",
		SortNameSource: models.DataSourceFilepath,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	person2 := &models.Person{
		LibraryID:      book.LibraryID,
		Name:           "Narrator Two",
		SortName:       "Two, Narrator",
		SortNameSource: models.DataSourceFilepath,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err = db.NewInsert().Model(person1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(person2).Exec(ctx)
	require.NoError(t, err)

	// Add narrators to each file (SortOrder must be > 0 due to nullzero constraint)
	narrator1 := &models.Narrator{FileID: file1.ID, PersonID: person1.ID, SortOrder: 1}
	narrator2 := &models.Narrator{FileID: file2.ID, PersonID: person2.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(narrator1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(narrator2).Exec(ctx)
	require.NoError(t, err)

	// Add identifiers to each file
	identifier1 := &models.FileIdentifier{
		FileID:    file1.ID,
		Type:      models.IdentifierTypeISBN13,
		Value:     "9781234567890",
		Source:    models.DataSourceM4BMetadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
	identifier2 := &models.FileIdentifier{
		FileID:    file2.ID,
		Type:      models.IdentifierTypeASIN,
		Value:     "B001234567",
		Source:    models.DataSourceM4BMetadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = db.NewInsert().Model(identifier1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(identifier2).Exec(ctx)
	require.NoError(t, err)

	// Add chapters to each file
	chapter1 := &models.Chapter{FileID: file1.ID, Title: "Ch1", SortOrder: 0, CreatedAt: now, UpdatedAt: now}
	chapter2 := &models.Chapter{FileID: file2.ID, Title: "Ch2", SortOrder: 0, CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(chapter1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(chapter2).Exec(ctx)
	require.NoError(t, err)

	// Batch delete both files
	err = svc.DeleteFilesByIDs(ctx, []int{file1.ID, file2.ID})
	require.NoError(t, err)

	// Verify file records are gone
	var fileCount int
	err = db.NewSelect().TableExpr("files").
		Where("id IN (?)", bun.In([]int{file1.ID, file2.ID})).
		ColumnExpr("count(*)").
		Scan(ctx, &fileCount)
	require.NoError(t, err)
	assert.Equal(t, 0, fileCount, "both file records should be deleted")

	// Verify narrators are gone
	var narratorCount int
	err = db.NewSelect().TableExpr("narrators").
		Where("file_id IN (?)", bun.In([]int{file1.ID, file2.ID})).
		ColumnExpr("count(*)").
		Scan(ctx, &narratorCount)
	require.NoError(t, err)
	assert.Equal(t, 0, narratorCount, "all narrators should be deleted")

	// Verify identifiers are gone
	var identifierCount int
	err = db.NewSelect().TableExpr("file_identifiers").
		Where("file_id IN (?)", bun.In([]int{file1.ID, file2.ID})).
		ColumnExpr("count(*)").
		Scan(ctx, &identifierCount)
	require.NoError(t, err)
	assert.Equal(t, 0, identifierCount, "all identifiers should be deleted")

	// Verify chapters are gone
	var chapterCount int
	err = db.NewSelect().TableExpr("chapters").
		Where("file_id IN (?)", bun.In([]int{file1.ID, file2.ID})).
		ColumnExpr("count(*)").
		Scan(ctx, &chapterCount)
	require.NoError(t, err)
	assert.Equal(t, 0, chapterCount, "all chapters should be deleted")
}

func TestDeleteFilesByIDs_EmptySlice(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// nil slice should be a no-op
	err := svc.DeleteFilesByIDs(ctx, nil)
	require.NoError(t, err)

	// empty slice should be a no-op
	err = svc.DeleteFilesByIDs(ctx, []int{})
	require.NoError(t, err)
}
