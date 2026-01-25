package books

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetrieveBook_LoadsChaptersForEachFile(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library and book
	_, book := setupTestLibraryAndBook(t, db)

	// Create a test file for the book
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/audiobook.m4b",
		FilesizeBytes: 1000,
	}
	_, err := db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create chapters for the file directly in the database
	// Insert in order with sort_order 0, 1, 2 to test ordering
	now := time.Now()
	chaptersData := []*models.Chapter{
		{FileID: file.ID, SortOrder: 0, Title: "Chapter 1", StartTimestampMs: ptrInt64(0), CreatedAt: now, UpdatedAt: now},
		{FileID: file.ID, SortOrder: 1, Title: "Chapter 2", StartTimestampMs: ptrInt64(30000), CreatedAt: now, UpdatedAt: now},
		{FileID: file.ID, SortOrder: 2, Title: "Chapter 3", StartTimestampMs: ptrInt64(60000), CreatedAt: now, UpdatedAt: now},
	}
	for _, ch := range chaptersData {
		_, err := db.NewInsert().Model(ch).Exec(ctx)
		require.NoError(t, err)
	}

	// Call RetrieveBook
	bookSvc := NewService(db)
	retrievedBook, err := bookSvc.RetrieveBook(ctx, RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Assert file.Chapters is populated
	require.Len(t, retrievedBook.Files, 1, "Book should have one file")
	require.NotNil(t, retrievedBook.Files[0].Chapters, "File should have chapters loaded")
	require.Len(t, retrievedBook.Files[0].Chapters, 3, "File should have 3 chapters")

	// Assert chapters are ordered by sort_order (0, 1, 2)
	assert.Equal(t, "Chapter 1", retrievedBook.Files[0].Chapters[0].Title, "First chapter by sort_order")
	assert.Equal(t, "Chapter 2", retrievedBook.Files[0].Chapters[1].Title, "Second chapter by sort_order")
	assert.Equal(t, "Chapter 3", retrievedBook.Files[0].Chapters[2].Title, "Third chapter by sort_order")
}

func TestRetrieveBook_LoadsNestedChaptersViaChildren(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library and book
	_, book := setupTestLibraryAndBook(t, db)

	// Create a test file for the book
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/audiobook.m4b",
		FilesizeBytes: 1000,
	}
	_, err := db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create a parent chapter (no parent_id)
	now := time.Now()
	parentChapter := &models.Chapter{
		FileID:           file.ID,
		SortOrder:        0,
		Title:            "Part 1",
		StartTimestampMs: ptrInt64(0),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_, err = db.NewInsert().Model(parentChapter).Exec(ctx)
	require.NoError(t, err)

	// Create child chapters with parent_id pointing to parent
	childChapters := []*models.Chapter{
		{
			FileID:           file.ID,
			ParentID:         &parentChapter.ID,
			SortOrder:        0,
			Title:            "Chapter 1",
			StartTimestampMs: ptrInt64(0),
			CreatedAt:        now,
			UpdatedAt:        now,
		},
		{
			FileID:           file.ID,
			ParentID:         &parentChapter.ID,
			SortOrder:        1,
			Title:            "Chapter 2",
			StartTimestampMs: ptrInt64(30000),
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}
	for _, ch := range childChapters {
		_, err := db.NewInsert().Model(ch).Exec(ctx)
		require.NoError(t, err)
	}

	// Call RetrieveBook
	bookSvc := NewService(db)
	retrievedBook, err := bookSvc.RetrieveBook(ctx, RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Assert file has chapters loaded
	require.Len(t, retrievedBook.Files, 1, "Book should have one file")
	require.NotNil(t, retrievedBook.Files[0].Chapters, "File should have chapters loaded")

	// Find the parent chapter (the one with no ParentID)
	var loadedParent *models.Chapter
	for _, ch := range retrievedBook.Files[0].Chapters {
		if ch.ParentID == nil && ch.Title == "Part 1" {
			loadedParent = ch
			break
		}
	}
	require.NotNil(t, loadedParent, "Parent chapter should be found in file.Chapters")

	// Assert parent chapter's Children field is populated
	require.NotNil(t, loadedParent.Children, "Parent chapter should have Children loaded")
	require.Len(t, loadedParent.Children, 2, "Parent should have 2 child chapters")

	// Assert Children have correct data and are ordered by sort_order
	assert.Equal(t, "Chapter 1", loadedParent.Children[0].Title, "First child by sort_order")
	assert.Equal(t, "Chapter 2", loadedParent.Children[1].Title, "Second child by sort_order")

	// Assert child chapters have correct parent reference
	assert.Equal(t, parentChapter.ID, *loadedParent.Children[0].ParentID, "First child has correct parent_id")
	assert.Equal(t, parentChapter.ID, *loadedParent.Children[1].ParentID, "Second child has correct parent_id")
}

func TestRetrieveBookByFilePath_LoadsChaptersForEachFile(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library and book
	library, book := setupTestLibraryAndBook(t, db)

	// Create a test file for the book
	testFilePath := "/test/audiobook.m4b"
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      testFilePath,
		FilesizeBytes: 1000,
	}
	_, err := db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create chapters for the file directly in the database
	// Insert in order with sort_order 0, 1, 2 to test ordering
	now := time.Now()
	chaptersData := []*models.Chapter{
		{FileID: file.ID, SortOrder: 0, Title: "Chapter 1", StartTimestampMs: ptrInt64(0), CreatedAt: now, UpdatedAt: now},
		{FileID: file.ID, SortOrder: 1, Title: "Chapter 2", StartTimestampMs: ptrInt64(30000), CreatedAt: now, UpdatedAt: now},
		{FileID: file.ID, SortOrder: 2, Title: "Chapter 3", StartTimestampMs: ptrInt64(60000), CreatedAt: now, UpdatedAt: now},
	}
	for _, ch := range chaptersData {
		_, err := db.NewInsert().Model(ch).Exec(ctx)
		require.NoError(t, err)
	}

	// Call RetrieveBookByFilePath
	bookSvc := NewService(db)
	retrievedBook, err := bookSvc.RetrieveBookByFilePath(ctx, testFilePath, library.ID)
	require.NoError(t, err)

	// Assert file.Chapters is populated
	require.Len(t, retrievedBook.Files, 1, "Book should have one file")
	require.NotNil(t, retrievedBook.Files[0].Chapters, "File should have chapters loaded")
	require.Len(t, retrievedBook.Files[0].Chapters, 3, "File should have 3 chapters")

	// Assert chapters are ordered by sort_order (0, 1, 2)
	assert.Equal(t, "Chapter 1", retrievedBook.Files[0].Chapters[0].Title, "First chapter by sort_order")
	assert.Equal(t, "Chapter 2", retrievedBook.Files[0].Chapters[1].Title, "Second chapter by sort_order")
	assert.Equal(t, "Chapter 3", retrievedBook.Files[0].Chapters[2].Title, "Third chapter by sort_order")
}

func TestRetrieveBookByFilePath_LoadsNestedChapters(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library and book
	library, book := setupTestLibraryAndBook(t, db)

	// Create a test file for the book
	testFilePath := "/test/audiobook.m4b"
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      testFilePath,
		FilesizeBytes: 1000,
	}
	_, err := db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create a parent chapter (no parent_id)
	now := time.Now()
	parentChapter := &models.Chapter{
		FileID:           file.ID,
		SortOrder:        0,
		Title:            "Part 1",
		StartTimestampMs: ptrInt64(0),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_, err = db.NewInsert().Model(parentChapter).Exec(ctx)
	require.NoError(t, err)

	// Create child chapters with parent_id pointing to parent
	childChapters := []*models.Chapter{
		{
			FileID:           file.ID,
			ParentID:         &parentChapter.ID,
			SortOrder:        0,
			Title:            "Chapter 1",
			StartTimestampMs: ptrInt64(0),
			CreatedAt:        now,
			UpdatedAt:        now,
		},
		{
			FileID:           file.ID,
			ParentID:         &parentChapter.ID,
			SortOrder:        1,
			Title:            "Chapter 2",
			StartTimestampMs: ptrInt64(30000),
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}
	for _, ch := range childChapters {
		_, err := db.NewInsert().Model(ch).Exec(ctx)
		require.NoError(t, err)
	}

	// Call RetrieveBookByFilePath
	bookSvc := NewService(db)
	retrievedBook, err := bookSvc.RetrieveBookByFilePath(ctx, testFilePath, library.ID)
	require.NoError(t, err)

	// Assert file has chapters loaded
	require.Len(t, retrievedBook.Files, 1, "Book should have one file")
	require.NotNil(t, retrievedBook.Files[0].Chapters, "File should have chapters loaded")

	// Find the parent chapter (the one with no ParentID)
	var loadedParent *models.Chapter
	for _, ch := range retrievedBook.Files[0].Chapters {
		if ch.ParentID == nil && ch.Title == "Part 1" {
			loadedParent = ch
			break
		}
	}
	require.NotNil(t, loadedParent, "Parent chapter should be found in file.Chapters")

	// Assert parent chapter's Children field is populated
	require.NotNil(t, loadedParent.Children, "Parent chapter should have Children loaded")
	require.Len(t, loadedParent.Children, 2, "Parent should have 2 child chapters")

	// Assert Children have correct data and are ordered by sort_order
	assert.Equal(t, "Chapter 1", loadedParent.Children[0].Title, "First child by sort_order")
	assert.Equal(t, "Chapter 2", loadedParent.Children[1].Title, "Second child by sort_order")

	// Assert child chapters have correct parent reference
	assert.Equal(t, parentChapter.ID, *loadedParent.Children[0].ParentID, "First child has correct parent_id")
	assert.Equal(t, parentChapter.ID, *loadedParent.Children[1].ParentID, "Second child has correct parent_id")
}

// ptrInt64 is a helper to create a pointer to an int64.
func ptrInt64(v int64) *int64 {
	return &v
}
