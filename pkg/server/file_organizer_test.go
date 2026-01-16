package server

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// testContext holds all the dependencies needed for testing the file organizer.
type testContext struct {
	t              *testing.T
	ctx            context.Context
	db             *bun.DB
	worker         *worker.Worker
	bookService    *books.Service
	libraryService *libraries.Service
	personService  *people.Service
	fileOrganizer  *fileOrganizer
}

// newTestContext creates a new test context with an in-memory SQLite database
// and all necessary services initialized.
func newTestContext(t *testing.T) *testContext {
	t.Helper()

	// Create in-memory SQLite database
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())

	// Run migrations
	_, err = migrations.BringUpToDate(context.Background(), db)
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Create services
	bookService := books.NewService(db)
	libraryService := libraries.NewService(db)
	personService := people.NewService(db)

	// Create worker for scanning
	cfg := &config.Config{
		WorkerProcesses:           1,
		SupplementExcludePatterns: []string{".*", ".DS_Store", "Thumbs.db", "desktop.ini"},
	}
	w := worker.New(cfg, db)

	// Create file organizer
	fo := &fileOrganizer{
		db:             db,
		bookService:    bookService,
		libraryService: libraryService,
	}

	// Create context with logger
	ctx := logger.New().WithContext(context.Background())

	tc := &testContext{
		t:              t,
		ctx:            ctx,
		db:             db,
		worker:         w,
		bookService:    bookService,
		libraryService: libraryService,
		personService:  personService,
		fileOrganizer:  fo,
	}

	t.Cleanup(func() {
		db.Close()
	})

	return tc
}

// createLibraryWithOrganize creates a test library with OrganizeFileStructure enabled.
func (tc *testContext) createLibraryWithOrganize(paths []string) *models.Library {
	tc.t.Helper()

	libraryPaths := make([]*models.LibraryPath, len(paths))
	for i, p := range paths {
		libraryPaths[i] = &models.LibraryPath{
			Filepath: p,
		}
	}

	library := &models.Library{
		Name:                  "Test Library",
		OrganizeFileStructure: true,
		CoverAspectRatio:      "book",
		LibraryPaths:          libraryPaths,
	}

	err := tc.libraryService.CreateLibrary(tc.ctx, library)
	if err != nil {
		tc.t.Fatalf("failed to create library: %v", err)
	}

	return library
}

// runScan executes the scan job for all libraries.
func (tc *testContext) runScan() error {
	log := logger.FromContext(tc.ctx)
	jobLogService := joblogs.NewService(tc.db)
	jobLog := jobLogService.NewJobLogger(tc.ctx, 0, log)
	return tc.worker.ProcessScanJob(tc.ctx, nil, jobLog)
}

// listBooks returns all books in the database with authors loaded.
func (tc *testContext) listBooks() []*models.Book {
	tc.t.Helper()

	allBooks, err := tc.bookService.ListBooks(tc.ctx, books.ListBooksOptions{})
	if err != nil {
		tc.t.Fatalf("failed to list books: %v", err)
	}
	return allBooks
}

// listFiles returns all files in the database.
func (tc *testContext) listFiles() []*models.File {
	tc.t.Helper()

	files, err := tc.bookService.ListFiles(tc.ctx, books.ListFilesOptions{})
	if err != nil {
		tc.t.Fatalf("failed to list files: %v", err)
	}
	return files
}

func TestFileOrganizer_OrganizeBookFiles_AuthorNameChange(t *testing.T) {
	tc := newTestContext(t)

	// Create a temp library directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibraryWithOrganize([]string{libraryPath})

	// Create a book directory with original author name
	originalAuthor := "Original Author"
	bookDir := testgen.CreateSubDir(t, libraryPath, "["+originalAuthor+"] Test Book")

	// Generate an EPUB in the directory
	testgen.GenerateEPUB(t, bookDir, "["+originalAuthor+"] Test Book.epub", testgen.EPUBOptions{
		Title:   "Test Book",
		Authors: []string{originalAuthor},
	})

	// Run initial scan
	err := tc.runScan()
	require.NoError(t, err)

	// Verify initial state
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	book := allBooks[0]

	require.Len(t, book.Authors, 1)
	require.NotNil(t, book.Authors[0].Person)
	assert.Equal(t, originalAuthor, book.Authors[0].Person.Name)

	originalBookPath := book.Filepath
	assert.Contains(t, originalBookPath, "[Original Author]")
	assert.True(t, testgen.FileExists(originalBookPath), "original book directory should exist")

	files := tc.listFiles()
	require.Len(t, files, 1)
	originalFilePath := files[0].Filepath
	assert.Contains(t, originalFilePath, "[Original Author]")
	assert.True(t, testgen.FileExists(originalFilePath), "original file should exist")

	// Get the person and update their name
	person := book.Authors[0].Person
	newAuthor := "New Author Name"
	person.Name = newAuthor
	err = tc.personService.UpdatePerson(tc.ctx, person, people.UpdatePersonOptions{
		Columns: []string{"name"},
	})
	require.NoError(t, err)

	// Now call OrganizeBookFiles to reorganize
	err = tc.fileOrganizer.OrganizeBookFiles(tc.ctx, book.ID)
	require.NoError(t, err)

	// Verify the folder was renamed
	expectedNewBookDir := filepath.Join(libraryPath, "[New Author Name] Test Book")
	assert.True(t, testgen.FileExists(expectedNewBookDir), "new book directory should exist")
	assert.False(t, testgen.FileExists(originalBookPath), "original book directory should not exist")

	// Verify the file was renamed
	expectedNewFilePath := filepath.Join(expectedNewBookDir, "[New Author Name] Test Book.epub")
	assert.True(t, testgen.FileExists(expectedNewFilePath), "new file should exist")
	assert.False(t, testgen.FileExists(originalFilePath), "original file should not exist")

	// Verify database was updated
	updatedBook, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	assert.Equal(t, expectedNewBookDir, updatedBook.Filepath)

	updatedFiles := tc.listFiles()
	require.Len(t, updatedFiles, 1)
	assert.Equal(t, expectedNewFilePath, updatedFiles[0].Filepath)
}

func TestFileOrganizer_OrganizeBookFiles_DisabledLibrary(t *testing.T) {
	tc := newTestContext(t)

	// Create a temp library directory with OrganizeFileStructure DISABLED
	libraryPath := testgen.TempLibraryDir(t)

	libraryPaths := []*models.LibraryPath{{Filepath: libraryPath}}
	library := &models.Library{
		Name:                  "Test Library",
		OrganizeFileStructure: false, // Disabled
		CoverAspectRatio:      "book",
		LibraryPaths:          libraryPaths,
	}
	err := tc.libraryService.CreateLibrary(tc.ctx, library)
	require.NoError(t, err)

	// Create a book directory
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Original Author] Test Book")
	testgen.GenerateEPUB(t, bookDir, "[Original Author] Test Book.epub", testgen.EPUBOptions{
		Title:   "Test Book",
		Authors: []string{"Original Author"},
	})

	// Run initial scan
	err = tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	book := allBooks[0]
	originalBookPath := book.Filepath

	files := tc.listFiles()
	require.Len(t, files, 1)
	originalFilePath := files[0].Filepath

	// Update person name
	person := book.Authors[0].Person
	person.Name = "New Author Name"
	err = tc.personService.UpdatePerson(tc.ctx, person, people.UpdatePersonOptions{
		Columns: []string{"name"},
	})
	require.NoError(t, err)

	// Call OrganizeBookFiles - should not reorganize because library has it disabled
	err = tc.fileOrganizer.OrganizeBookFiles(tc.ctx, book.ID)
	require.NoError(t, err)

	// Verify files were NOT renamed (because OrganizeFileStructure is disabled)
	assert.True(t, testgen.FileExists(originalBookPath), "original book directory should still exist")
	assert.True(t, testgen.FileExists(originalFilePath), "original file should still exist")

	// New paths should not exist
	expectedNewBookDir := filepath.Join(libraryPath, "[New Author Name] Test Book")
	assert.False(t, testgen.FileExists(expectedNewBookDir), "new book directory should not exist")
}

func TestFileOrganizer_RenameNarratedFile_NarratorNameChange(t *testing.T) {
	tc := newTestContext(t)

	// Create a temp library directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibraryWithOrganize([]string{libraryPath})

	// Create a book directory with author and narrator
	originalAuthor := "Test Author"
	originalNarrator := "Original Narrator"
	bookDir := testgen.CreateSubDir(t, libraryPath, "["+originalAuthor+"] Audiobook Title")

	// Generate an M4B with narrator
	testgen.GenerateM4B(t, bookDir, "["+originalAuthor+"] Audiobook Title {"+originalNarrator+"}.m4b", testgen.M4BOptions{
		Title:    "Audiobook Title",
		Artist:   originalAuthor,
		Composer: originalNarrator, // Narrator is stored in Composer field
	})

	// Run initial scan
	err := tc.runScan()
	require.NoError(t, err)

	// Verify initial state
	files := tc.listFiles()
	require.Len(t, files, 1)
	file := files[0]

	require.Len(t, file.Narrators, 1)
	require.NotNil(t, file.Narrators[0].Person)
	assert.Equal(t, originalNarrator, file.Narrators[0].Person.Name)

	originalFilePath := file.Filepath
	assert.Contains(t, originalFilePath, "{Original Narrator}")
	assert.True(t, testgen.FileExists(originalFilePath), "original file should exist")

	// Update the narrator's name
	narratorPerson := file.Narrators[0].Person
	newNarrator := "New Narrator Name"
	narratorPerson.Name = newNarrator
	err = tc.personService.UpdatePerson(tc.ctx, narratorPerson, people.UpdatePersonOptions{
		Columns: []string{"name"},
	})
	require.NoError(t, err)

	// Call RenameNarratedFile to rename the file
	newPath, err := tc.fileOrganizer.RenameNarratedFile(tc.ctx, file.ID)
	require.NoError(t, err)

	// Verify the file was renamed
	assert.Contains(t, newPath, "{New Narrator Name}")
	assert.True(t, testgen.FileExists(newPath), "new file should exist")
	assert.False(t, testgen.FileExists(originalFilePath), "original file should not exist")

	// Verify database was updated
	updatedFile, err := tc.bookService.RetrieveFile(tc.ctx, books.RetrieveFileOptions{ID: &file.ID})
	require.NoError(t, err)
	assert.Equal(t, newPath, updatedFile.Filepath)
}

func TestFileOrganizer_GetLibraryOrganizeSetting(t *testing.T) {
	tc := newTestContext(t)

	// Create library with OrganizeFileStructure enabled
	libraryPath := testgen.TempLibraryDir(t)
	library := tc.createLibraryWithOrganize([]string{libraryPath})

	enabled, err := tc.fileOrganizer.GetLibraryOrganizeSetting(tc.ctx, library.ID)
	require.NoError(t, err)
	assert.True(t, enabled)

	// Create another library with OrganizeFileStructure disabled
	libraryPath2 := testgen.TempLibraryDir(t)
	libraryPaths := []*models.LibraryPath{{Filepath: libraryPath2}}
	library2 := &models.Library{
		Name:                  "Disabled Library",
		OrganizeFileStructure: false,
		CoverAspectRatio:      "book",
		LibraryPaths:          libraryPaths,
	}
	err = tc.libraryService.CreateLibrary(tc.ctx, library2)
	require.NoError(t, err)

	enabled2, err := tc.fileOrganizer.GetLibraryOrganizeSetting(tc.ctx, library2.ID)
	require.NoError(t, err)
	assert.False(t, enabled2)
}

func TestFileOrganizer_OrganizeBookFiles_CoverImagePathUpdated(t *testing.T) {
	tc := newTestContext(t)

	// Create a temp library directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibraryWithOrganize([]string{libraryPath})

	// Create a book directory with original author name
	originalAuthor := "Original Author"
	bookDir := testgen.CreateSubDir(t, libraryPath, "["+originalAuthor+"] Test Book With Cover")

	// Generate an EPUB with a cover in the directory
	testgen.GenerateEPUB(t, bookDir, "["+originalAuthor+"] Test Book With Cover.epub", testgen.EPUBOptions{
		Title:    "Test Book With Cover",
		Authors:  []string{originalAuthor},
		HasCover: true,
	})

	// Run initial scan
	err := tc.runScan()
	require.NoError(t, err)

	// Verify initial state - file should have a cover
	files := tc.listFiles()
	require.Len(t, files, 1)
	file := files[0]

	// Verify cover was extracted
	require.NotNil(t, file.CoverImagePath, "CoverImagePath should be set after scan")
	originalCoverPath := *file.CoverImagePath
	assert.Contains(t, originalCoverPath, "[Original Author]", "original cover path should contain original author name")

	// Verify the cover file exists on disk
	originalCoverFullPath := filepath.Join(bookDir, originalCoverPath)
	assert.True(t, testgen.FileExists(originalCoverFullPath), "original cover file should exist at %s", originalCoverFullPath)

	// Get the book and update the author's name
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	book := allBooks[0]

	require.Len(t, book.Authors, 1)
	person := book.Authors[0].Person
	newAuthor := "New Author Name"
	person.Name = newAuthor
	err = tc.personService.UpdatePerson(tc.ctx, person, people.UpdatePersonOptions{
		Columns: []string{"name"},
	})
	require.NoError(t, err)

	// Call OrganizeBookFiles to reorganize
	err = tc.fileOrganizer.OrganizeBookFiles(tc.ctx, book.ID)
	require.NoError(t, err)

	// Verify the file was renamed
	expectedNewBookDir := filepath.Join(libraryPath, "[New Author Name] Test Book With Cover")
	expectedNewFilePath := filepath.Join(expectedNewBookDir, "[New Author Name] Test Book With Cover.epub")
	assert.True(t, testgen.FileExists(expectedNewFilePath), "new file should exist at %s", expectedNewFilePath)

	// Verify the cover file was renamed on disk
	expectedNewCoverFilename := "[New Author Name] Test Book With Cover.epub.cover.png"
	expectedNewCoverFullPath := filepath.Join(expectedNewBookDir, expectedNewCoverFilename)
	assert.True(t, testgen.FileExists(expectedNewCoverFullPath), "new cover file should exist at %s", expectedNewCoverFullPath)

	// THIS IS THE BUG: CoverImagePath in database should be updated to match the new filename
	updatedFile, err := tc.bookService.RetrieveFile(tc.ctx, books.RetrieveFileOptions{ID: &file.ID})
	require.NoError(t, err)

	require.NotNil(t, updatedFile.CoverImagePath, "CoverImagePath should still be set")
	assert.Equal(t, expectedNewCoverFilename, *updatedFile.CoverImagePath,
		"CoverImagePath should be updated to match the new filename")
}
