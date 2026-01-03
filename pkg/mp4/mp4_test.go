package mp4_test

import (
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/mp4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParse_DataType18Genre tests that parsing works for M4B files that have
// genre metadata stored with data type 18 (a format that dhowden/tag doesn't handle).
// This test should FAIL with the current dhowden/tag implementation and PASS once
// we implement the new go-mp4 based parser.
func TestParse_DataType18Genre(t *testing.T) {
	dir := testgen.TempDir(t, "mp4-type18-*")

	// Generate a synthetic M4B with data type 18 genre
	path := testgen.GenerateM4BWithType18Genre(t, dir, "test.m4b", testgen.M4BType18Options{
		Title: "Test Audiobook",
		Genre: "Fantasy",
	})

	// This should NOT error - the current dhowden/tag parser fails here with:
	// "invalid content type: 18"
	metadata, err := mp4.Parse(path)
	require.NoError(t, err, "Parse should not error on data type 18 genre")

	// Verify the title was parsed correctly
	assert.Equal(t, "Test Audiobook", metadata.Title)
}

// TestParse_BasicMetadata tests basic metadata extraction using ffmpeg-generated M4B.
// This test should pass with both the old and new implementations.
func TestParse_BasicMetadata(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-basic-*")

	path := testgen.GenerateM4B(t, dir, "test.m4b", testgen.M4BOptions{
		Title:    "My Test Book",
		Artist:   "Author Name",
		Album:    "Test Series #3",
		Composer: "Narrator Name",
		Genre:    "Science Fiction",
		Duration: 1.0,
	})

	metadata, err := mp4.Parse(path)
	require.NoError(t, err)

	assert.Equal(t, "My Test Book", metadata.Title)
	assert.Contains(t, metadata.Authors, "Author Name")
	assert.Contains(t, metadata.Narrators, "Narrator Name")
}

// TestParse_WithCover tests that cover images are extracted correctly.
func TestParse_WithCover(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-cover-*")

	path := testgen.GenerateM4B(t, dir, "test.m4b", testgen.M4BOptions{
		Title:    "Book With Cover",
		Artist:   "Cover Author",
		HasCover: true,
		Duration: 1.0,
	})

	metadata, err := mp4.Parse(path)
	require.NoError(t, err)

	assert.Equal(t, "Book With Cover", metadata.Title)
	assert.NotEmpty(t, metadata.CoverData, "Cover data should be present")
	assert.NotEmpty(t, metadata.CoverMimeType, "Cover mime type should be set")
}

// TestParse_SeriesExtraction tests series name and number parsing from album field.
func TestParse_SeriesExtraction(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-series-*")

	path := testgen.GenerateM4B(t, dir, "test.m4b", testgen.M4BOptions{
		Title:    "Book Title",
		Artist:   "Author",
		Album:    "Dungeon Crawler Carl #7",
		Duration: 1.0,
	})

	metadata, err := mp4.Parse(path)
	require.NoError(t, err)

	assert.Equal(t, "Dungeon Crawler Carl", metadata.Series)
	require.NotNil(t, metadata.SeriesNumber)
	assert.InDelta(t, 7.0, *metadata.SeriesNumber, 0.001)
}

// TestParseFull_Duration tests that duration is extracted correctly.
func TestParseFull_Duration(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-duration-*")

	path := testgen.GenerateM4B(t, dir, "test.m4b", testgen.M4BOptions{
		Title:    "Duration Test",
		Duration: 2.5, // 2.5 seconds
	})

	metadata, err := mp4.ParseFull(path)
	require.NoError(t, err)

	// Duration should be approximately 2.5 seconds (allow for encoding variations)
	assert.InDelta(t, 2.5, metadata.Duration.Seconds(), 0.5)
	assert.Positive(t, metadata.Bitrate)
}

// TestWrite_Roundtrip tests that writing metadata and re-reading produces the same values.
func TestWrite_Roundtrip(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-write-*")

	// Generate initial M4B
	path := testgen.GenerateM4B(t, dir, "test.m4b", testgen.M4BOptions{
		Title:    "Original Title",
		Artist:   "Original Author",
		Duration: 1.0,
	})

	// Parse to get the full metadata
	original, err := mp4.ParseFull(path)
	require.NoError(t, err)
	assert.Equal(t, "Original Title", original.Title)

	// Modify and write
	original.Title = "Modified Title"
	original.Authors = []string{"New Author"}

	err = mp4.Write(path, original, mp4.WriteOptions{CreateBackup: true})
	require.NoError(t, err)

	// Verify backup was created
	assert.FileExists(t, path+".bak")

	// Re-read and verify
	modified, err := mp4.ParseFull(path)
	require.NoError(t, err)
	assert.Equal(t, "Modified Title", modified.Title)
	assert.Contains(t, modified.Authors, "New Author")
}
