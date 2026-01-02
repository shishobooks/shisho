package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessScanJob_EPUBBasic(t *testing.T) {
	tc := newTestContext(t)

	// Create a library with a temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with an EPUB that has no embedded metadata
	// Title and author should come from directory name
	bookDir := testgen.CreateSubDir(t, libraryPath, "[John Doe] My Test Book")
	testgen.GenerateEPUB(t, bookDir, "test.epub", testgen.EPUBOptions{
		// No Title or Authors - should fall back to filepath
		HasCover: true,
	})

	// Run the scan
	err := tc.runScan()
	require.NoError(t, err)

	// Verify the book was created
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	// Title and author come from directory name
	assert.Equal(t, "My Test Book", book.Title)
	assert.Equal(t, models.DataSourceFilepath, book.TitleSource)
	// Author should be extracted WITHOUT brackets
	require.Len(t, book.Authors, 1)
	assert.Equal(t, "John Doe", book.Authors[0].Name)

	// Verify file was created
	files := tc.listFiles()
	require.Len(t, files, 1)
	assert.Equal(t, models.FileTypeEPUB, files[0].FileType)
	assert.NotNil(t, files[0].CoverMimeType)
}

func TestProcessScanJob_EPUBWithSeries(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "Fantasy Book")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:        "The First Book",
		Authors:      []string{"Jane Smith"},
		Series:       "Epic Series",
		SeriesNumber: pointerutil.Float64(1),
		HasCover:     true,
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	assert.Equal(t, "The First Book", book.Title)
	require.NotNil(t, book.SeriesID, "book should have a series ID")
	require.NotNil(t, book.SeriesNumber)
	assert.InDelta(t, 1.0, *book.SeriesNumber, 0.001)

	// Verify series was created
	allSeries := tc.listSeries()
	require.Len(t, allSeries, 1)
	assert.Equal(t, "Epic Series", allSeries[0].Name)
	assert.Equal(t, *book.SeriesID, allSeries[0].ID)
}

func TestProcessScanJob_CBZBasic(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create CBZ without ComicInfo.xml - metadata comes from directory name
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Comic Writer] My Comic")
	testgen.GenerateCBZ(t, bookDir, "comic.cbz", testgen.CBZOptions{
		// No ComicInfo - should fall back to filepath
		HasComicInfo: false,
		PageCount:    5,
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	// Title and author come from directory name
	assert.Equal(t, "My Comic", book.Title)
	assert.Equal(t, models.DataSourceFilepath, book.TitleSource)
	require.Len(t, book.Authors, 1)
	assert.Equal(t, "Comic Writer", book.Authors[0].Name)

	files := tc.listFiles()
	require.Len(t, files, 1)
	assert.Equal(t, models.FileTypeCBZ, files[0].FileType)
}

func TestProcessScanJob_CBZWithMinimalComicInfo(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Test CBZ with minimal ComicInfo - only title, no writer
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Folder Author] Folder Title")
	testgen.GenerateCBZ(t, bookDir, "book.cbz", testgen.CBZOptions{
		Title:        "ComicInfo Title", // Title from ComicInfo takes precedence
		HasComicInfo: true,
		PageCount:    3,
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	// Title comes from ComicInfo, not directory name
	assert.Equal(t, "ComicInfo Title", book.Title)
	assert.Equal(t, models.DataSourceCBZMetadata, book.TitleSource)
	// No writer in ComicInfo, so author comes from directory name
	require.Len(t, book.Authors, 1)
	assert.Equal(t, "Folder Author", book.Authors[0].Name)
}

func TestProcessScanJob_M4BBasic(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)

	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create M4B without embedded metadata - title comes from directory name
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Narrator Name] My Audiobook")
	testgen.GenerateM4B(t, bookDir, "audiobook.m4b", testgen.M4BOptions{
		// No Title/Artist/Album - should fall back to filepath
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	// Title comes from directory name
	assert.Equal(t, "My Audiobook", book.Title)
	assert.Equal(t, models.DataSourceFilepath, book.TitleSource)
	// Author extracted from directory name
	require.Len(t, book.Authors, 1)
	assert.Equal(t, "Narrator Name", book.Authors[0].Name)

	files := tc.listFiles()
	require.Len(t, files, 1)
	assert.Equal(t, models.FileTypeM4B, files[0].FileType)
}

func TestProcessScanJob_UnsupportedExtension(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a file with unsupported extension
	testgen.WriteFile(t, libraryPath, "document.pdf", []byte("not a valid PDF"))
	testgen.WriteFile(t, libraryPath, "readme.txt", []byte("just a text file"))

	err := tc.runScan()
	require.NoError(t, err)

	// No books should be created
	allBooks := tc.listBooks()
	assert.Empty(t, allBooks)
}

func TestProcessScanJob_MimeMismatch(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a file with .epub extension but wrong content
	testgen.WriteFile(t, libraryPath, "fake.epub", []byte("this is not a real epub"))

	err := tc.runScan()
	require.NoError(t, err)

	// No books should be created (mime type won't match)
	allBooks := tc.listBooks()
	assert.Empty(t, allBooks)
}

func TestProcessScanJob_ExistingFileSkipped(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "Test Book")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:    "Original Title",
		Authors:  []string{"Original Author"},
		HasCover: true,
	})

	// First scan
	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	assert.Equal(t, "Original Title", allBooks[0].Title)

	// Second scan - file should be skipped
	err = tc.runScan()
	require.NoError(t, err)

	// Still only one book
	allBooks = tc.listBooks()
	require.Len(t, allBooks, 1)
	files := tc.listFiles()
	require.Len(t, files, 1)
}

func TestProcessScanJob_RootLevelFile(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create EPUB directly in library path (root level)
	testgen.GenerateEPUB(t, libraryPath, "root-book.epub", testgen.EPUBOptions{
		Title:    "Root Level Book",
		Authors:  []string{"Root Author"},
		HasCover: true,
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	assert.Equal(t, "Root Level Book", book.Title)
	// For root-level files, the book path should be the file path itself
	assert.Contains(t, book.Filepath, "root-book.epub")
}

func TestProcessScanJob_DirectoryWithMultipleFiles(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "Multi-Format Book")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:    "Multi-Format Book",
		Authors:  []string{"Author One"},
		HasCover: true,
	})
	testgen.GenerateCBZ(t, bookDir, "book.cbz", testgen.CBZOptions{
		Title:        "Multi-Format Book",
		Writer:       "Author One",
		HasComicInfo: true,
		PageCount:    3,
	})

	err := tc.runScan()
	require.NoError(t, err)

	// Should create one book with two files
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	assert.Equal(t, "Multi-Format Book", book.Title)

	files := tc.listFiles()
	require.Len(t, files, 2)
}

func TestProcessScanJob_CoverExtraction(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "Book With Cover")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:         "Book With Cover",
		Authors:       []string{"Cover Author"},
		HasCover:      true,
		CoverMimeType: "image/png",
	})

	err := tc.runScan()
	require.NoError(t, err)

	// Verify cover file was extracted
	coverPath := filepath.Join(bookDir, "book.epub.cover.png")
	assert.True(t, testgen.FileExists(coverPath), "cover should be extracted")

	files := tc.listFiles()
	require.Len(t, files, 1)
	require.NotNil(t, files[0].CoverMimeType)
	assert.Equal(t, "image/png", *files[0].CoverMimeType)
}

func TestProcessScanJob_ExistingCoverNotOverwritten(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "Book With Existing Cover")

	// Create a pre-existing cover file
	existingCoverContent := []byte("existing cover content")
	existingCoverPath := testgen.WriteFile(t, bookDir, "book.epub.cover.png", existingCoverContent)

	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:         "Book With Existing Cover",
		Authors:       []string{"Author"},
		HasCover:      true,
		CoverMimeType: "image/png",
	})

	err := tc.runScan()
	require.NoError(t, err)

	// Verify the existing cover was not overwritten
	coverData := testgen.ReadFile(t, existingCoverPath)
	assert.Equal(t, existingCoverContent, coverData, "existing cover should not be overwritten")

	files := tc.listFiles()
	require.Len(t, files, 1)
	require.NotNil(t, files[0].CoverSource)
	assert.Equal(t, models.DataSourceExistingCover, *files[0].CoverSource)
}

func TestProcessScanJob_ExistingCoverNotOverwritten_DifferentExtension(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "Book With Different Extension Cover")

	// Create a pre-existing PNG cover file
	existingCoverContent := []byte("existing png cover content")
	existingCoverPath := testgen.WriteFile(t, bookDir, "book.epub.cover.png", existingCoverContent)

	// Create an EPUB with a JPEG cover - should NOT be extracted since PNG exists
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:         "Book With Different Extension Cover",
		Authors:       []string{"Author"},
		HasCover:      true,
		CoverMimeType: "image/jpeg", // Book has JPEG, but PNG exists
	})

	err := tc.runScan()
	require.NoError(t, err)

	// Verify the existing PNG cover was not overwritten
	coverData := testgen.ReadFile(t, existingCoverPath)
	assert.Equal(t, existingCoverContent, coverData, "existing cover should not be overwritten")

	// Verify that no JPEG cover was created
	jpegCoverPath := filepath.Join(bookDir, "book.epub.cover.jpg")
	assert.False(t, testgen.FileExists(jpegCoverPath), "JPEG cover should not be created when PNG exists")

	// Verify the file has existing cover as source
	files := tc.listFiles()
	require.Len(t, files, 1)
	require.NotNil(t, files[0].CoverSource)
	assert.Equal(t, models.DataSourceExistingCover, *files[0].CoverSource)
}

func TestProcessScanJob_ExistingCanonicalCoverNotOverwritten(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "Book With Canonical Cover")

	// Create a pre-existing canonical cover.png file
	existingCoverContent := []byte("user-provided canonical cover")
	existingCoverPath := testgen.WriteFile(t, bookDir, "cover.png", existingCoverContent)

	// Create an EPUB with a JPEG cover
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:         "Book With Canonical Cover",
		Authors:       []string{"Author"},
		HasCover:      true,
		CoverMimeType: "image/jpeg",
	})

	err := tc.runScan()
	require.NoError(t, err)

	// Verify the existing canonical cover was not overwritten
	coverData := testgen.ReadFile(t, existingCoverPath)
	assert.Equal(t, existingCoverContent, coverData, "existing canonical cover should not be overwritten")

	// Verify that no cover.jpg was created
	jpegCoverPath := filepath.Join(bookDir, "cover.jpg")
	assert.False(t, testgen.FileExists(jpegCoverPath), "canonical cover.jpg should not be created when cover.png exists")

	// Verify the book uses the existing cover.png
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	require.NotNil(t, allBooks[0].CoverImagePath)
	assert.Equal(t, "cover.png", *allBooks[0].CoverImagePath)
}

func TestProcessScanJob_ExistingAudiobookCoverNotOverwritten(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "Audiobook With Existing Cover")

	// Create a pre-existing audiobook_cover.png file
	existingCoverContent := []byte("user-provided audiobook cover")
	existingCoverPath := testgen.WriteFile(t, bookDir, "audiobook_cover.png", existingCoverContent)

	// Create an M4B audiobook with a cover
	testgen.GenerateM4B(t, bookDir, "audiobook.m4b", testgen.M4BOptions{
		Title:    "Audiobook With Existing Cover",
		Artist:   "Narrator Name",
		HasCover: true,
	})

	err := tc.runScan()
	require.NoError(t, err)

	// Verify the existing audiobook cover was not overwritten
	coverData := testgen.ReadFile(t, existingCoverPath)
	assert.Equal(t, existingCoverContent, coverData, "existing audiobook cover should not be overwritten")

	// Verify the book uses the existing audiobook_cover.png
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	require.NotNil(t, allBooks[0].CoverImagePath)
	assert.Equal(t, "audiobook_cover.png", *allBooks[0].CoverImagePath)
}

func TestProcessScanJob_VolumeNormalization(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "Comic Volume")
	testgen.GenerateCBZ(t, bookDir, "comic.cbz", testgen.CBZOptions{
		Title:         "My Comic #007",
		Series:        "Comic Series",
		SeriesNumber:  pointerutil.Float64(7),
		HasComicInfo:  true,
		CoverPageType: "FrontCover",
		PageCount:     3,
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	// Title should be normalized from "My Comic #007" to "My Comic v7"
	assert.Equal(t, "My Comic v7", book.Title)
}

func TestProcessScanJob_AuthorFromFilename(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// When EPUB has title but no authors in metadata, author is extracted from directory name
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Famous Author] Great Book Title")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:    "EPUB Title", // Title from EPUB metadata
		Authors:  []string{},   // No authors in metadata
		HasCover: false,
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	// Title comes from EPUB metadata
	assert.Equal(t, "EPUB Title", book.Title)
	assert.Equal(t, models.DataSourceEPUBMetadata, book.TitleSource)
	// Author should be extracted from directory name since no authors in EPUB
	require.Len(t, book.Authors, 1)
	assert.Equal(t, "Famous Author", book.Authors[0].Name)
}

func TestProcessScanJob_MultipleLibraryPaths(t *testing.T) {
	tc := newTestContext(t)

	libraryPath1 := testgen.TempLibraryDir(t)
	libraryPath2 := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath1, libraryPath2})

	// Create books in both paths
	bookDir1 := testgen.CreateSubDir(t, libraryPath1, "Book 1")
	testgen.GenerateEPUB(t, bookDir1, "book1.epub", testgen.EPUBOptions{
		Title:   "Book One",
		Authors: []string{"Author 1"},
	})

	bookDir2 := testgen.CreateSubDir(t, libraryPath2, "Book 2")
	testgen.GenerateEPUB(t, bookDir2, "book2.epub", testgen.EPUBOptions{
		Title:   "Book Two",
		Authors: []string{"Author 2"},
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 2)

	titles := []string{allBooks[0].Title, allBooks[1].Title}
	assert.Contains(t, titles, "Book One")
	assert.Contains(t, titles, "Book Two")
}

func TestProcessScanJob_EmptyLibrary(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	assert.Empty(t, allBooks)
}

func TestProcessScanJob_NestedDirectories(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create nested directory structure
	level1 := testgen.CreateSubDir(t, libraryPath, "Series")
	level2 := testgen.CreateSubDir(t, level1, "Book 1")
	testgen.GenerateEPUB(t, level2, "book.epub", testgen.EPUBOptions{
		Title:   "Nested Book",
		Authors: []string{"Nested Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	assert.Equal(t, "Nested Book", allBooks[0].Title)
}

func TestProcessScanJob_CBZInnerCover(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "Comic With Inner Cover")
	testgen.GenerateCBZ(t, bookDir, "comic.cbz", testgen.CBZOptions{
		Title:         "Comic With Inner Cover",
		HasComicInfo:  true,
		CoverPageType: "InnerCover",
		PageCount:     5,
	})

	err := tc.runScan()
	require.NoError(t, err)

	files := tc.listFiles()
	require.Len(t, files, 1)
	// Cover should still be extracted
	require.NotNil(t, files[0].CoverMimeType)
}

func TestProcessScanJob_EPUBWithJPEGCover(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "Book With JPEG Cover")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:         "Book With JPEG",
		Authors:       []string{"Author"},
		HasCover:      true,
		CoverMimeType: "image/jpeg",
	})

	err := tc.runScan()
	require.NoError(t, err)

	files := tc.listFiles()
	require.Len(t, files, 1)
	require.NotNil(t, files[0].CoverMimeType)
	assert.Equal(t, "image/jpeg", *files[0].CoverMimeType)

	// Verify cover file has .jpg extension
	coverPath := filepath.Join(bookDir, "book.epub.cover.jpg")
	assert.True(t, testgen.FileExists(coverPath), "JPEG cover should be extracted")
}

func TestProcessScanJob_LibraryWalkError(t *testing.T) {
	tc := newTestContext(t)

	// Create a path that doesn't exist
	libraryPath := "/nonexistent/path/to/library"
	tc.createLibrary([]string{libraryPath})

	// Should return an error because path doesn't exist
	err := tc.runScan()
	assert.Error(t, err)
}

func TestProcessScanJob_MetadataUpdateOnRescan(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "Book Dir")
	epubPath := testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:    "Original Title",
		Authors:  []string{"Original Author"},
		HasCover: false,
	})

	// First scan
	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	assert.Equal(t, "Original Title", allBooks[0].Title)

	// Remove the first file and create a new one with updated metadata
	os.Remove(epubPath)
	testgen.GenerateEPUB(t, bookDir, "book2.epub", testgen.EPUBOptions{
		Title:   "Updated Title",
		Authors: []string{"Updated Author"},
	})

	// Second scan
	err = tc.runScan()
	require.NoError(t, err)

	allBooks = tc.listBooks()
	require.Len(t, allBooks, 1)

	// Book should now have two files (the original record is still there,
	// but a new file was added)
	files := tc.listFiles()
	require.Len(t, files, 2)
}

func TestProcessScanJob_RootLevelFileWithCover(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create EPUB directly in library path (root level) with cover
	testgen.GenerateEPUB(t, libraryPath, "root-book.epub", testgen.EPUBOptions{
		Title:         "Root Level Book",
		Authors:       []string{"Root Author"},
		HasCover:      true,
		CoverMimeType: "image/png",
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	assert.Equal(t, "Root Level Book", book.Title)

	// For root-level files, the cover should be saved in the library directory
	// (same directory as the file), not inside the epub file path
	coverPath := filepath.Join(libraryPath, "root-book.epub.cover.png")
	assert.True(t, testgen.FileExists(coverPath), "cover should be extracted to library directory, not inside epub path")

	files := tc.listFiles()
	require.Len(t, files, 1)
	require.NotNil(t, files[0].CoverMimeType)
	assert.Equal(t, "image/png", *files[0].CoverMimeType)
}

func TestProcessScanJob_M4BWithCover(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)

	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "Audiobook With Cover")
	testgen.GenerateM4B(t, bookDir, "audiobook.m4b", testgen.M4BOptions{
		Title:    "Audiobook With Cover",
		Artist:   "Narrator",
		Album:    "Series Name #3",
		HasCover: true,
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	assert.Equal(t, "Audiobook With Cover", book.Title)

	// Check series parsing from album
	require.NotNil(t, book.SeriesID, "book should have a series ID")
	require.NotNil(t, book.SeriesNumber)
	assert.InDelta(t, 3.0, *book.SeriesNumber, 0.001)

	// Verify series was created
	allSeries := tc.listSeries()
	require.Len(t, allSeries, 1)
	assert.Equal(t, "Series Name", allSeries[0].Name)
	assert.Equal(t, *book.SeriesID, allSeries[0].ID)
}

func TestProcessScanJob_OrganizeFileStructure_RootLevelFile(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	// Enable organize_file_structure
	tc.createLibraryWithOptions([]string{libraryPath}, true)

	// Create a root-level EPUB (directly in library path)
	testgen.GenerateEPUB(t, libraryPath, "my-book.epub", testgen.EPUBOptions{
		Title:   "Organized Book",
		Authors: []string{"Test Author"},
	})

	// Verify file exists at root before scan
	originalPath := filepath.Join(libraryPath, "my-book.epub")
	assert.True(t, testgen.FileExists(originalPath), "file should exist at root before scan")

	err := tc.runScan()
	require.NoError(t, err)

	// File should have been moved into an organized folder
	// Expected folder: [Test Author] Organized Book
	organizedFolder := filepath.Join(libraryPath, "[Test Author] Organized Book")
	assert.True(t, testgen.FileExists(organizedFolder), "organized folder should be created")

	// Original file should no longer exist at root
	assert.False(t, testgen.FileExists(originalPath), "original file should be moved from root")

	// File should exist in the organized folder
	organizedFile := filepath.Join(organizedFolder, "[Test Author] Organized Book.epub")
	assert.True(t, testgen.FileExists(organizedFile), "file should exist in organized folder")

	// Verify the book record has the updated path
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	assert.Equal(t, "Organized Book", allBooks[0].Title)
	assert.Equal(t, organizedFolder, allBooks[0].Filepath)

	// Verify file record has updated path
	files := tc.listFiles()
	require.Len(t, files, 1)
	assert.Equal(t, organizedFile, files[0].Filepath)
}

func TestProcessScanJob_OrganizeFileStructure_Disabled(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	// Disable organize_file_structure (default behavior)
	tc.createLibraryWithOptions([]string{libraryPath}, false)

	// Create a root-level EPUB
	testgen.GenerateEPUB(t, libraryPath, "my-book.epub", testgen.EPUBOptions{
		Title:   "Unorganized Book",
		Authors: []string{"Test Author"},
	})

	originalPath := filepath.Join(libraryPath, "my-book.epub")
	assert.True(t, testgen.FileExists(originalPath), "file should exist at root before scan")

	err := tc.runScan()
	require.NoError(t, err)

	// File should remain at root level (not organized)
	assert.True(t, testgen.FileExists(originalPath), "file should remain at root when organize is disabled")

	// No organized folder should be created
	organizedFolder := filepath.Join(libraryPath, "[Test Author] Unorganized Book")
	assert.False(t, testgen.FileExists(organizedFolder), "organized folder should not be created")

	// Verify the book record has the original path (the file path itself for root-level)
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	assert.Equal(t, "Unorganized Book", allBooks[0].Title)
	assert.Equal(t, originalPath, allBooks[0].Filepath)
}

func TestProcessScanJob_OrganizeFileStructure_DirectoryFile_NotMoved(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	// Enable organize_file_structure
	tc.createLibraryWithOptions([]string{libraryPath}, true)

	// Create a file inside a directory (not root-level)
	bookDir := testgen.CreateSubDir(t, libraryPath, "Existing Folder")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:   "Book In Folder",
		Authors: []string{"Author Name"},
	})

	originalPath := filepath.Join(bookDir, "book.epub")
	assert.True(t, testgen.FileExists(originalPath), "file should exist in folder before scan")

	err := tc.runScan()
	require.NoError(t, err)

	// File should remain in its original folder (organize only applies to root-level files)
	assert.True(t, testgen.FileExists(originalPath), "file in directory should not be moved")

	// Verify the book record uses the directory path
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	assert.Equal(t, "Book In Folder", allBooks[0].Title)
	assert.Equal(t, bookDir, allBooks[0].Filepath)
}

func TestProcessScanJob_OrganizeFileStructure_WithCover(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibraryWithOptions([]string{libraryPath}, true)

	// Create a root-level EPUB with cover
	testgen.GenerateEPUB(t, libraryPath, "book-with-cover.epub", testgen.EPUBOptions{
		Title:         "Book With Cover",
		Authors:       []string{"Cover Author"},
		HasCover:      true,
		CoverMimeType: "image/png",
	})

	err := tc.runScan()
	require.NoError(t, err)

	// Verify file was organized
	organizedFolder := filepath.Join(libraryPath, "[Cover Author] Book With Cover")
	assert.True(t, testgen.FileExists(organizedFolder), "organized folder should be created")

	// Cover should also be in the organized folder
	coverPath := filepath.Join(organizedFolder, "[Cover Author] Book With Cover.epub.cover.png")
	assert.True(t, testgen.FileExists(coverPath), "cover should be moved to organized folder")

	// No cover should remain at root
	rootCoverPath := filepath.Join(libraryPath, "book-with-cover.epub.cover.png")
	assert.False(t, testgen.FileExists(rootCoverPath), "cover should not remain at root")
}

func TestProcessScanJob_IsRootLevelFile_MultipleLibraryPaths(t *testing.T) {
	tc := newTestContext(t)

	// Create two library paths
	libraryPath1 := testgen.TempLibraryDir(t)
	libraryPath2 := testgen.TempLibraryDir(t)
	tc.createLibraryWithOptions([]string{libraryPath1, libraryPath2}, false)

	// Create root-level file in first path
	testgen.GenerateEPUB(t, libraryPath1, "root1.epub", testgen.EPUBOptions{
		Title:   "Root Book 1",
		Authors: []string{"Author 1"},
	})

	// Create root-level file in second path
	testgen.GenerateEPUB(t, libraryPath2, "root2.epub", testgen.EPUBOptions{
		Title:   "Root Book 2",
		Authors: []string{"Author 2"},
	})

	// Create directory-based file in first path
	bookDir := testgen.CreateSubDir(t, libraryPath1, "Book Folder")
	testgen.GenerateEPUB(t, bookDir, "dir-book.epub", testgen.EPUBOptions{
		Title:   "Directory Book",
		Authors: []string{"Author 3"},
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 3)

	// Find books by title
	var rootBook1, rootBook2, dirBook *models.Book
	for _, book := range allBooks {
		switch book.Title {
		case "Root Book 1":
			rootBook1 = book
		case "Root Book 2":
			rootBook2 = book
		case "Directory Book":
			dirBook = book
		}
	}

	require.NotNil(t, rootBook1)
	require.NotNil(t, rootBook2)
	require.NotNil(t, dirBook)

	// Root-level books should have file path as book path
	assert.Contains(t, rootBook1.Filepath, "root1.epub")
	assert.Contains(t, rootBook2.Filepath, "root2.epub")

	// Directory-based book should have directory as book path
	assert.Equal(t, bookDir, dirBook.Filepath)
}

func TestProcessScanJob_CleanupOrphanedSeries(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a series directly in the database without any books (orphaned)
	orphanedSeries := &models.Series{
		LibraryID:  1,
		Name:       "Orphaned Series",
		NameSource: models.DataSourceManual,
	}
	err := tc.seriesService.CreateSeries(tc.ctx, orphanedSeries)
	require.NoError(t, err)

	// Create another series that will have a book
	seriesWithBook := &models.Series{
		LibraryID:  1,
		Name:       "Series With Book",
		NameSource: models.DataSourceManual,
	}
	err = tc.seriesService.CreateSeries(tc.ctx, seriesWithBook)
	require.NoError(t, err)

	// Create an EPUB with the second series
	bookDir := testgen.CreateSubDir(t, libraryPath, "Book In Series")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:        "Book In Series",
		Authors:      []string{"Author"},
		Series:       "Series With Book",
		SeriesNumber: pointerutil.Float64(1),
	})

	// Verify both series exist before scan
	allSeries := tc.listSeries()
	require.Len(t, allSeries, 2, "should have 2 series before scan")

	// Run the scan - this should clean up the orphaned series
	err = tc.runScan()
	require.NoError(t, err)

	// Verify only the series with a book remains
	allSeries = tc.listSeries()
	require.Len(t, allSeries, 1, "should have 1 series after scan (orphaned series cleaned up)")
	assert.Equal(t, "Series With Book", allSeries[0].Name)

	// Verify the book was linked to the correct series
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	require.NotNil(t, allBooks[0].SeriesID)
	assert.Equal(t, allSeries[0].ID, *allBooks[0].SeriesID)
}

func TestProcessScanJob_TitleFallbackWhenOnlyBracketsInDirName(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a directory where the name consists ONLY of author brackets
	// After stripping [Author] from "[Author Name]", title would be empty
	// The fix should fall back to the raw filename
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author Name]")
	testgen.GenerateCBZ(t, bookDir, "comic.cbz", testgen.CBZOptions{
		// No title in ComicInfo - relies on filename fallback
		HasComicInfo: false,
		PageCount:    3,
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	// Title should NOT be empty - it should fall back to the file basename "comic"
	assert.NotEmpty(t, book.Title, "title should never be empty")
	assert.Equal(t, "comic", book.Title)
	assert.Equal(t, models.DataSourceFilepath, book.TitleSource)

	// Author should still be extracted from the directory name
	require.Len(t, book.Authors, 1)
	assert.Equal(t, "Author Name", book.Authors[0].Name)
}

func TestProcessScanJob_TitleFallbackWhenOnlyBracketsInDirName_WithNarrator(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Directory name with both author and narrator brackets, no title
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author Name] {Narrator Name}")
	testgen.GenerateEPUB(t, bookDir, "mybook.epub", testgen.EPUBOptions{
		// No title in metadata - relies on filename fallback
		HasCover: false,
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	// Title should NOT be empty - it should fall back to the file basename "mybook"
	assert.NotEmpty(t, book.Title, "title should never be empty")
	assert.Equal(t, "mybook", book.Title)
	assert.Equal(t, models.DataSourceFilepath, book.TitleSource)

	// Author should still be extracted from the directory name
	require.Len(t, book.Authors, 1)
	assert.Equal(t, "Author Name", book.Authors[0].Name)
}

func TestProcessScanJob_TitleFallbackWhenCBZHasEmptyTitle(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a CBZ with ComicInfo.xml that has an empty <Title></Title> element
	// This tests that we correctly fall back to the filename when metadata has empty title
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Akitsuki Itsuki] Test Book")
	testgen.GenerateCBZ(t, bookDir, "comic.cbz", testgen.CBZOptions{
		HasComicInfo:    true,
		ForceEmptyTitle: true, // Creates <Title></Title>
		Writer:          "Akitsuki Itsuki",
		PageCount:       3,
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	// Title should NOT be empty - empty metadata title should not overwrite filepath title
	assert.NotEmpty(t, book.Title, "title should never be empty")
	// The title should come from the directory name (after stripping author brackets)
	assert.Equal(t, "Test Book", book.Title)
	assert.Equal(t, models.DataSourceFilepath, book.TitleSource)
}

func TestProcessScanJob_TitleFallbackRootLevelWithMultipleBrackets(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// This is the exact filename pattern from the user's report:
	// With the non-greedy regex, [Author] and [Other Tag] are both removed,
	// leaving the actual title in the middle
	testgen.GenerateCBZ(t, libraryPath, "[Author] Title [Other Tag].cbz", testgen.CBZOptions{
		HasComicInfo:    true,
		ForceEmptyTitle: true, // ComicInfo has empty title
		Writer:          "Author",
		PageCount:       3,
	})

	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)

	book := allBooks[0]
	// Title should NOT be empty
	assert.NotEmpty(t, book.Title, "title should never be empty")
	// With non-greedy regex, both [Author] and [Other Tag] are stripped,
	// leaving the actual title
	assert.Equal(t, "Title", book.Title)
	assert.Equal(t, models.DataSourceFilepath, book.TitleSource)

	// Author should be extracted from the first bracket pattern
	require.Len(t, book.Authors, 1)
	assert.Equal(t, "Author", book.Authors[0].Name)
}

func TestProcessScanJob_SameNameDifferentExtensions_SeparateCovers(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)

	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with both an EPUB and M4B that have the same base name
	// This tests that each file gets its own cover file (book.epub.cover.jpg vs book.m4b.cover.jpg)
	bookDir := testgen.CreateSubDir(t, libraryPath, "Multi-Format Book")

	// Create book.epub with a cover
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:         "Multi-Format Book",
		Authors:       []string{"Test Author"},
		HasCover:      true,
		CoverMimeType: "image/jpeg",
	})

	// Create book.m4b with a cover
	testgen.GenerateM4B(t, bookDir, "book.m4b", testgen.M4BOptions{
		Title:    "Multi-Format Book",
		Artist:   "Test Author",
		HasCover: true,
	})

	err := tc.runScan()
	require.NoError(t, err)

	// Verify both files were created and belong to the same book
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	assert.Equal(t, "Multi-Format Book", allBooks[0].Title)

	files := tc.listFiles()
	require.Len(t, files, 2, "should have 2 files (epub and m4b)")

	// Verify each file has a cover
	for _, file := range files {
		require.NotNil(t, file.CoverMimeType, "file %s should have a cover", file.Filepath)
	}

	// Verify separate cover files exist for each format
	// The new naming convention is {filename}.cover.{ext} to avoid conflicts
	// EPUB has JPEG cover, M4B has PNG cover
	epubCoverPath := filepath.Join(bookDir, "book.epub.cover.jpg")
	m4bCoverPath := filepath.Join(bookDir, "book.m4b.cover.png")

	assert.True(t, testgen.FileExists(epubCoverPath), "EPUB cover should exist at %s", epubCoverPath)
	assert.True(t, testgen.FileExists(m4bCoverPath), "M4B cover should exist at %s", m4bCoverPath)

	// Verify the covers are different files (not overwritten)
	epubCoverData := testgen.ReadFile(t, epubCoverPath)
	m4bCoverData := testgen.ReadFile(t, m4bCoverPath)

	// The covers should both exist and have content
	assert.NotEmpty(t, epubCoverData, "EPUB cover should have content")
	assert.NotEmpty(t, m4bCoverData, "M4B cover should have content")
}
