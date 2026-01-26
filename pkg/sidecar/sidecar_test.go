package sidecar

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileSidecarFromModel_Name(t *testing.T) {
	t.Parallel()
	name := "Custom File Name"
	file := &models.File{
		Name: &name,
	}

	sidecar := FileSidecarFromModel(file)

	assert.NotNil(t, sidecar.Name)
	assert.Equal(t, "Custom File Name", *sidecar.Name)
}

func TestFileSidecarFromModel_NilName(t *testing.T) {
	t.Parallel()
	file := &models.File{
		Name: nil,
	}

	sidecar := FileSidecarFromModel(file)

	assert.Nil(t, sidecar.Name)
}

func TestFileSidecarFromModel_WithChapters(t *testing.T) {
	t.Parallel()
	page1 := 0
	page2 := 10
	file := &models.File{
		Chapters: []*models.Chapter{
			{Title: "Chapter 1", StartPage: &page1},
			{Title: "Chapter 2", StartPage: &page2},
		},
	}

	sidecar := FileSidecarFromModel(file)

	assert.Len(t, sidecar.Chapters, 2)
	assert.Equal(t, "Chapter 1", sidecar.Chapters[0].Title)
	assert.Equal(t, 0, *sidecar.Chapters[0].StartPage)
	assert.Equal(t, "Chapter 2", sidecar.Chapters[1].Title)
	assert.Equal(t, 10, *sidecar.Chapters[1].StartPage)
}

func TestFileSidecarFromModel_WithNestedChapters(t *testing.T) {
	t.Parallel()
	href1 := "part1.xhtml"
	href2 := "ch1.xhtml"
	href3 := "ch2.xhtml"
	file := &models.File{
		Chapters: []*models.Chapter{
			{
				Title: "Part 1",
				Href:  &href1,
				Children: []*models.Chapter{
					{Title: "Chapter 1", Href: &href2},
					{Title: "Chapter 2", Href: &href3},
				},
			},
		},
	}

	sidecar := FileSidecarFromModel(file)

	assert.Len(t, sidecar.Chapters, 1)
	assert.Equal(t, "Part 1", sidecar.Chapters[0].Title)
	assert.Len(t, sidecar.Chapters[0].Children, 2)
	assert.Equal(t, "Chapter 1", sidecar.Chapters[0].Children[0].Title)
	assert.Equal(t, "Chapter 2", sidecar.Chapters[0].Children[1].Title)
}

func TestFileSidecarFromModel_NoChapters(t *testing.T) {
	t.Parallel()
	file := &models.File{
		Chapters: nil,
	}

	sidecar := FileSidecarFromModel(file)

	assert.Nil(t, sidecar.Chapters)
}

func TestChaptersFromModels_Empty(t *testing.T) {
	t.Parallel()
	result := ChaptersFromModels(nil)
	assert.Nil(t, result)

	result = ChaptersFromModels([]*models.Chapter{})
	assert.Nil(t, result)
}

func TestChaptersFromModels_CBZ(t *testing.T) {
	t.Parallel()
	page1 := 0
	page2 := 10
	chapters := []*models.Chapter{
		{Title: "Chapter 1", StartPage: &page1},
		{Title: "Chapter 2", StartPage: &page2},
	}

	result := ChaptersFromModels(chapters)

	assert.Len(t, result, 2)
	assert.Equal(t, "Chapter 1", result[0].Title)
	assert.Equal(t, 0, *result[0].StartPage)
	assert.Nil(t, result[0].StartTimestampMs)
	assert.Nil(t, result[0].Href)

	assert.Equal(t, "Chapter 2", result[1].Title)
	assert.Equal(t, 10, *result[1].StartPage)
}

func TestChaptersFromModels_M4B(t *testing.T) {
	t.Parallel()
	ts1 := int64(0)
	ts2 := int64(60000)
	chapters := []*models.Chapter{
		{Title: "Chapter 1", StartTimestampMs: &ts1},
		{Title: "Chapter 2", StartTimestampMs: &ts2},
	}

	result := ChaptersFromModels(chapters)

	assert.Len(t, result, 2)
	assert.Equal(t, "Chapter 1", result[0].Title)
	assert.Equal(t, int64(0), *result[0].StartTimestampMs)
	assert.Nil(t, result[0].StartPage)

	assert.Equal(t, "Chapter 2", result[1].Title)
	assert.Equal(t, int64(60000), *result[1].StartTimestampMs)
}

func TestChaptersFromModels_EPUB(t *testing.T) {
	t.Parallel()
	href1 := "chapter1.xhtml"
	href2 := "chapter2.xhtml"
	chapters := []*models.Chapter{
		{Title: "Chapter 1", Href: &href1},
		{Title: "Chapter 2", Href: &href2},
	}

	result := ChaptersFromModels(chapters)

	assert.Len(t, result, 2)
	assert.Equal(t, "Chapter 1", result[0].Title)
	assert.Equal(t, "chapter1.xhtml", *result[0].Href)
	assert.Nil(t, result[0].StartPage)
	assert.Nil(t, result[0].StartTimestampMs)
}

func TestChaptersFromModels_Nested(t *testing.T) {
	t.Parallel()
	href1 := "part1.xhtml"
	href2 := "chapter1.xhtml"
	href3 := "chapter2.xhtml"

	chapters := []*models.Chapter{
		{
			Title: "Part 1",
			Href:  &href1,
			Children: []*models.Chapter{
				{Title: "Chapter 1", Href: &href2},
				{Title: "Chapter 2", Href: &href3},
			},
		},
	}

	result := ChaptersFromModels(chapters)

	assert.Len(t, result, 1)
	assert.Equal(t, "Part 1", result[0].Title)
	assert.Len(t, result[0].Children, 2)
	assert.Equal(t, "Chapter 1", result[0].Children[0].Title)
	assert.Equal(t, "Chapter 2", result[0].Children[1].Title)
}

func TestChaptersToModels_Empty(t *testing.T) {
	t.Parallel()
	result := ChaptersToModels(nil)
	assert.Nil(t, result)

	result = ChaptersToModels([]ChapterMetadata{})
	assert.Nil(t, result)
}

func TestChaptersToModels_CBZ(t *testing.T) {
	t.Parallel()
	page1 := 0
	page2 := 10
	chapters := []ChapterMetadata{
		{Title: "Chapter 1", StartPage: &page1},
		{Title: "Chapter 2", StartPage: &page2},
	}

	result := ChaptersToModels(chapters)

	assert.Len(t, result, 2)
	assert.Equal(t, "Chapter 1", result[0].Title)
	assert.Equal(t, 0, *result[0].StartPage)
	assert.Nil(t, result[0].StartTimestampMs)
	assert.Nil(t, result[0].Href)

	assert.Equal(t, "Chapter 2", result[1].Title)
	assert.Equal(t, 10, *result[1].StartPage)
}

func TestChaptersToModels_M4B(t *testing.T) {
	t.Parallel()
	ts1 := int64(0)
	ts2 := int64(60000)
	chapters := []ChapterMetadata{
		{Title: "Chapter 1", StartTimestampMs: &ts1},
		{Title: "Chapter 2", StartTimestampMs: &ts2},
	}

	result := ChaptersToModels(chapters)

	assert.Len(t, result, 2)
	assert.Equal(t, "Chapter 1", result[0].Title)
	assert.Equal(t, int64(0), *result[0].StartTimestampMs)

	assert.Equal(t, "Chapter 2", result[1].Title)
	assert.Equal(t, int64(60000), *result[1].StartTimestampMs)
}

func TestChaptersToModels_Nested(t *testing.T) {
	t.Parallel()
	href1 := "part1.xhtml"
	href2 := "chapter1.xhtml"

	chapters := []ChapterMetadata{
		{
			Title: "Part 1",
			Href:  &href1,
			Children: []ChapterMetadata{
				{Title: "Chapter 1", Href: &href2},
			},
		},
	}

	result := ChaptersToModels(chapters)

	assert.Len(t, result, 1)
	assert.Equal(t, "Part 1", result[0].Title)
	assert.Len(t, result[0].Children, 1)
	assert.Equal(t, "Chapter 1", result[0].Children[0].Title)
	assert.Equal(t, "chapter1.xhtml", *result[0].Children[0].Href)
}

func TestChaptersRoundTrip_CBZ(t *testing.T) {
	t.Parallel()
	page1 := 0
	page2 := 15
	original := []*models.Chapter{
		{Title: "Chapter 1", StartPage: &page1},
		{Title: "Chapter 2", StartPage: &page2},
	}

	// Model -> Sidecar -> Model
	sidecarChapters := ChaptersFromModels(original)
	roundTrip := ChaptersToModels(sidecarChapters)

	assert.Len(t, roundTrip, 2)
	assert.Equal(t, original[0].Title, roundTrip[0].Title)
	assert.Equal(t, *original[0].StartPage, *roundTrip[0].StartPage)
	assert.Equal(t, original[1].Title, roundTrip[1].Title)
	assert.Equal(t, *original[1].StartPage, *roundTrip[1].StartPage)
}

func TestChaptersRoundTrip_M4B(t *testing.T) {
	t.Parallel()
	ts1 := int64(0)
	ts2 := int64(120000)
	original := []*models.Chapter{
		{Title: "Introduction", StartTimestampMs: &ts1},
		{Title: "Main Content", StartTimestampMs: &ts2},
	}

	// Model -> Sidecar -> Model
	sidecarChapters := ChaptersFromModels(original)
	roundTrip := ChaptersToModels(sidecarChapters)

	assert.Len(t, roundTrip, 2)
	assert.Equal(t, original[0].Title, roundTrip[0].Title)
	assert.Equal(t, *original[0].StartTimestampMs, *roundTrip[0].StartTimestampMs)
	assert.Equal(t, original[1].Title, roundTrip[1].Title)
	assert.Equal(t, *original[1].StartTimestampMs, *roundTrip[1].StartTimestampMs)
}

func TestChaptersRoundTrip_EPUB_Nested(t *testing.T) {
	t.Parallel()
	href1 := "part1.xhtml"
	href2 := "ch1.xhtml"
	href3 := "ch2.xhtml"

	original := []*models.Chapter{
		{
			Title: "Part 1",
			Href:  &href1,
			Children: []*models.Chapter{
				{Title: "Chapter 1", Href: &href2},
				{Title: "Chapter 2", Href: &href3},
			},
		},
	}

	// Model -> Sidecar -> Model
	sidecarChapters := ChaptersFromModels(original)
	roundTrip := ChaptersToModels(sidecarChapters)

	assert.Len(t, roundTrip, 1)
	assert.Equal(t, original[0].Title, roundTrip[0].Title)
	assert.Equal(t, *original[0].Href, *roundTrip[0].Href)
	assert.Len(t, roundTrip[0].Children, 2)
	assert.Equal(t, original[0].Children[0].Title, roundTrip[0].Children[0].Title)
	assert.Equal(t, *original[0].Children[0].Href, *roundTrip[0].Children[0].Href)
}

// =============================================================================
// Sidecar Writing Tests
// =============================================================================

func TestWriteFileSidecar(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.cbz")

	page1 := 0
	page2 := 10
	s := &FileSidecar{
		URL: strPtr("https://example.com"),
		Chapters: []ChapterMetadata{
			{Title: "Chapter 1", StartPage: &page1},
			{Title: "Chapter 2", StartPage: &page2},
		},
	}

	err := WriteFileSidecar(filePath, s)
	require.NoError(t, err)

	// Read it back and verify
	readBack, err := ReadFileSidecar(filePath)
	require.NoError(t, err)
	require.NotNil(t, readBack)

	assert.Equal(t, CurrentVersion, readBack.Version)
	assert.Equal(t, "https://example.com", *readBack.URL)
	assert.Len(t, readBack.Chapters, 2)
	assert.Equal(t, "Chapter 1", readBack.Chapters[0].Title)
	assert.Equal(t, 0, *readBack.Chapters[0].StartPage)
	assert.Equal(t, "Chapter 2", readBack.Chapters[1].Title)
	assert.Equal(t, 10, *readBack.Chapters[1].StartPage)
}

func TestWriteFileSidecarFromModel(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "audiobook.m4b")

	ts1 := int64(0)
	ts2 := int64(60000)
	file := &models.File{
		Filepath: filePath,
		URL:      strPtr("https://example.com/audiobook"),
		Chapters: []*models.Chapter{
			{Title: "Introduction", StartTimestampMs: &ts1},
			{Title: "Chapter 1", StartTimestampMs: &ts2},
		},
	}

	err := WriteFileSidecarFromModel(file)
	require.NoError(t, err)

	// Verify file exists at correct path
	sidecarPath := FileSidecarPath(filePath)
	_, err = os.Stat(sidecarPath)
	require.NoError(t, err)

	// Read it back and verify chapters are included
	readBack, err := ReadFileSidecar(filePath)
	require.NoError(t, err)
	require.NotNil(t, readBack)

	assert.Equal(t, "https://example.com/audiobook", *readBack.URL)
	assert.Len(t, readBack.Chapters, 2)
	assert.Equal(t, "Introduction", readBack.Chapters[0].Title)
	assert.Equal(t, int64(0), *readBack.Chapters[0].StartTimestampMs)
	assert.Equal(t, "Chapter 1", readBack.Chapters[1].Title)
	assert.Equal(t, int64(60000), *readBack.Chapters[1].StartTimestampMs)
}

func TestWriteFileSidecarFromModel_WithNestedChapters(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "book.epub")

	href1 := "part1.xhtml"
	href2 := "ch1.xhtml"
	href3 := "ch2.xhtml"
	file := &models.File{
		Filepath: filePath,
		Chapters: []*models.Chapter{
			{
				Title: "Part 1",
				Href:  &href1,
				Children: []*models.Chapter{
					{Title: "Chapter 1", Href: &href2},
					{Title: "Chapter 2", Href: &href3},
				},
			},
		},
	}

	err := WriteFileSidecarFromModel(file)
	require.NoError(t, err)

	// Read it back and verify nested structure
	readBack, err := ReadFileSidecar(filePath)
	require.NoError(t, err)
	require.NotNil(t, readBack)

	assert.Len(t, readBack.Chapters, 1)
	assert.Equal(t, "Part 1", readBack.Chapters[0].Title)
	assert.Len(t, readBack.Chapters[0].Children, 2)
	assert.Equal(t, "Chapter 1", readBack.Chapters[0].Children[0].Title)
	assert.Equal(t, "ch1.xhtml", *readBack.Chapters[0].Children[0].Href)
}

func TestWriteBookSidecar(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	bookPath := filepath.Join(tmpDir, "mybook.epub")

	s := &BookSidecar{
		Title:       "Test Book",
		Description: strPtr("A test description"),
		Authors: []AuthorMetadata{
			{Name: "John Doe", SortName: "Doe, John"},
		},
	}

	err := WriteBookSidecar(bookPath, s)
	require.NoError(t, err)

	// Read it back and verify
	readBack, err := ReadBookSidecar(bookPath)
	require.NoError(t, err)
	require.NotNil(t, readBack)

	assert.Equal(t, CurrentVersion, readBack.Version)
	assert.Equal(t, "Test Book", readBack.Title)
	assert.Equal(t, "A test description", *readBack.Description)
	assert.Len(t, readBack.Authors, 1)
	assert.Equal(t, "John Doe", readBack.Authors[0].Name)
}

func TestWriteBookSidecarFromModel(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	bookPath := filepath.Join(tmpDir, "mybook.epub")

	book := &models.Book{
		Filepath:    bookPath,
		Title:       "Model Book",
		Description: strPtr("Description from model"),
		Authors: []*models.Author{
			{Person: &models.Person{Name: "Jane Smith", SortName: "Smith, Jane"}, SortOrder: 0},
		},
	}

	err := WriteBookSidecarFromModel(book)
	require.NoError(t, err)

	// Read it back and verify
	readBack, err := ReadBookSidecar(bookPath)
	require.NoError(t, err)
	require.NotNil(t, readBack)

	assert.Equal(t, "Model Book", readBack.Title)
	assert.Equal(t, "Description from model", *readBack.Description)
	assert.Len(t, readBack.Authors, 1)
	assert.Equal(t, "Jane Smith", readBack.Authors[0].Name)
}

func TestWriteFileSidecar_SetsVersion(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.cbz")

	// Create sidecar with version 0 (unset)
	s := &FileSidecar{
		Version: 0,
	}

	err := WriteFileSidecar(filePath, s)
	require.NoError(t, err)

	// Read back and verify version was set
	readBack, err := ReadFileSidecar(filePath)
	require.NoError(t, err)
	assert.Equal(t, CurrentVersion, readBack.Version)
}

func strPtr(s string) *string {
	return &s
}

// =============================================================================
// BookSidecarPath Tests
// =============================================================================

func TestBookSidecarPath_ExistingDirectory(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	bookDir := filepath.Join(tmpDir, "MyBook")
	require.NoError(t, os.MkdirAll(bookDir, 0755))

	// For an existing directory, sidecar should be inside the directory
	result := BookSidecarPath(bookDir)

	assert.Equal(t, filepath.Join(bookDir, "MyBook.metadata.json"), result)
}

func TestBookSidecarPath_ExistingFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "MyBook.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("test"), 0644))

	// For an existing file, sidecar should be alongside it without the .epub extension
	result := BookSidecarPath(filePath)

	assert.Equal(t, filepath.Join(tmpDir, "MyBook.metadata.json"), result)
}

func TestBookSidecarPath_NonExistentDirectoryPath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	// This is the key bug case: a path that represents a FUTURE directory (not yet created)
	// The path ends with a directory name (no extension), simulating what happens during
	// scanning when book.Filepath is set to the expected organized folder before it exists.
	futureBookDir := filepath.Join(tmpDir, "MyBook")

	// The sidecar should be inside the future directory, not alongside it
	result := BookSidecarPath(futureBookDir)

	// BUG: Currently returns /tmp/xxx/MyBook.metadata.json (sibling)
	// EXPECTED: /tmp/xxx/MyBook/MyBook.metadata.json (inside the directory)
	assert.Equal(t, filepath.Join(futureBookDir, "MyBook.metadata.json"), result)
}

func TestBookSidecarPath_NonExistentDirectoryPath_WithTrailingSlash(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	// Path with trailing slash clearly indicates it's meant to be a directory
	futureBookDir := filepath.Join(tmpDir, "MyBook") + string(filepath.Separator)

	result := BookSidecarPath(futureBookDir)

	// Should be inside the directory
	assert.Equal(t, filepath.Join(tmpDir, "MyBook", "MyBook.metadata.json"), result)
}

func TestWriteBookSidecar_DirectoryCreatedBeforeWrite(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	// This simulates the scan scenario with OrganizeFileStructure enabled:
	// The caller creates the directory before writing the sidecar
	bookDir := filepath.Join(tmpDir, "MyBook")
	require.NoError(t, os.MkdirAll(bookDir, 0755))

	s := &BookSidecar{
		Title: "My Book",
	}

	err := WriteBookSidecar(bookDir, s)
	require.NoError(t, err)

	// Verify the sidecar was written to the correct location INSIDE the directory
	expectedPath := filepath.Join(bookDir, "MyBook.metadata.json")
	_, err = os.Stat(expectedPath)
	require.NoError(t, err, "sidecar should exist at %s", expectedPath)

	// Read it back and verify
	readBack, err := ReadBookSidecar(bookDir)
	require.NoError(t, err)
	require.NotNil(t, readBack)
	assert.Equal(t, "My Book", readBack.Title)
}

func TestWriteBookSidecar_NonExistentDirectory_FailsGracefully(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	// When the directory doesn't exist and caller doesn't create it,
	// the write should fail (caller's responsibility to create dir)
	futureBookDir := filepath.Join(tmpDir, "NonExistentBook")

	s := &BookSidecar{
		Title: "My Book",
	}

	// Writing should fail because the directory doesn't exist
	err := WriteBookSidecar(futureBookDir, s)
	require.Error(t, err, "write should fail when directory doesn't exist")
}
