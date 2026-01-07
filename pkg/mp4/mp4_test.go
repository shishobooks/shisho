package mp4_test

import (
	"testing"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/mediafile"
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
	require.Len(t, metadata.Authors, 1)
	assert.Equal(t, "Author Name", metadata.Authors[0].Name)
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
	original.Authors = []mediafile.ParsedAuthor{{Name: "New Author"}}

	err = mp4.Write(path, original, mp4.WriteOptions{CreateBackup: true})
	require.NoError(t, err)

	// Verify backup was created
	assert.FileExists(t, path+".bak")

	// Re-read and verify
	modified, err := mp4.ParseFull(path)
	require.NoError(t, err)
	assert.Equal(t, "Modified Title", modified.Title)
	require.Len(t, modified.Authors, 1)
	assert.Equal(t, "New Author", modified.Authors[0].Name)
}

// TestWrite_Subtitle tests subtitle roundtrip through freeform atom.
func TestWrite_Subtitle(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-subtitle-*")

	// Generate initial M4B
	path := testgen.GenerateM4B(t, dir, "test.m4b", testgen.M4BOptions{
		Title:    "Main Title",
		Duration: 1.0,
	})

	// Parse and add subtitle
	meta, err := mp4.ParseFull(path)
	require.NoError(t, err)
	meta.Subtitle = "A Compelling Subtitle"

	err = mp4.Write(path, meta, mp4.WriteOptions{})
	require.NoError(t, err)

	// Re-read and verify subtitle was written
	modified, err := mp4.ParseFull(path)
	require.NoError(t, err)
	assert.Equal(t, "A Compelling Subtitle", modified.Subtitle)
}

// TestWrite_NarratorAtoms tests that narrators are written to both ©nrt and ©cmp.
func TestWrite_NarratorAtoms(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-narrator-*")

	// Generate initial M4B without narrators
	path := testgen.GenerateM4B(t, dir, "test.m4b", testgen.M4BOptions{
		Title:    "Narrator Test",
		Duration: 1.0,
	})

	// Parse and set narrators
	meta, err := mp4.ParseFull(path)
	require.NoError(t, err)
	meta.Narrators = []string{"John Smith", "Jane Doe"}

	err = mp4.Write(path, meta, mp4.WriteOptions{})
	require.NoError(t, err)

	// Re-read and verify narrators
	modified, err := mp4.ParseFull(path)
	require.NoError(t, err)
	require.Len(t, modified.Narrators, 2)
	assert.Equal(t, "John Smith", modified.Narrators[0])
	assert.Equal(t, "Jane Doe", modified.Narrators[1])
}

// TestWrite_SeriesFormatting tests album formatting from series info.
func TestWrite_SeriesFormatting(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)

	tests := []struct {
		name          string
		series        string
		seriesNumber  *float64
		expectedAlbum string
	}{
		{"integer number", "Test Series", pointerutil.Float64(1), "Test Series #1"},
		{"decimal number", "Test Series", pointerutil.Float64(1.5), "Test Series #1.5"},
		{"no number", "Test Series", nil, "Test Series"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := testgen.TempDir(t, "mp4-series-format-*")

			// Generate M4B
			path := testgen.GenerateM4B(t, dir, "test.m4b", testgen.M4BOptions{
				Title:    "Series Test",
				Duration: 1.0,
			})

			// Parse, set series, and write
			meta, err := mp4.ParseFull(path)
			require.NoError(t, err)
			meta.Series = tc.series
			meta.SeriesNumber = tc.seriesNumber

			err = mp4.Write(path, meta, mp4.WriteOptions{})
			require.NoError(t, err)

			// Re-read and verify album was formatted correctly
			modified, err := mp4.ParseFull(path)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedAlbum, modified.Album)
		})
	}
}

// TestWriteToFile_AtomicWrite tests that WriteToFile creates a new file atomically.
func TestWriteToFile_AtomicWrite(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-writeto-*")

	// Generate source M4B
	srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
		Title:    "Source Title",
		Duration: 1.0,
	})

	destPath := dir + "/dest.m4b"

	// Parse source and modify
	meta, err := mp4.ParseFull(srcPath)
	require.NoError(t, err)
	meta.Title = "Destination Title"

	// Write to new file
	err = mp4.WriteToFile(srcPath, destPath, meta)
	require.NoError(t, err)

	// Verify destination file exists
	assert.FileExists(t, destPath)

	// Verify no temp file remains
	assert.NoFileExists(t, destPath+".tmp")

	// Verify source is unchanged
	source, err := mp4.ParseFull(srcPath)
	require.NoError(t, err)
	assert.Equal(t, "Source Title", source.Title)

	// Verify destination has modified metadata
	dest, err := mp4.ParseFull(destPath)
	require.NoError(t, err)
	assert.Equal(t, "Destination Title", dest.Title)
}

// TestWrite_PreservesMetadata tests that unmodified fields are preserved.
func TestWrite_PreservesMetadata(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-preserve-*")

	// Generate M4B with various metadata
	path := testgen.GenerateM4B(t, dir, "test.m4b", testgen.M4BOptions{
		Title:    "Original Title",
		Artist:   "Original Author",
		Composer: "Original Narrator",
		Genre:    "Fantasy",
		Duration: 1.0,
	})

	// Parse and only modify title
	meta, err := mp4.ParseFull(path)
	require.NoError(t, err)
	meta.Title = "Modified Title"

	err = mp4.Write(path, meta, mp4.WriteOptions{})
	require.NoError(t, err)

	// Re-read and verify other fields are preserved
	modified, err := mp4.ParseFull(path)
	require.NoError(t, err)
	assert.Equal(t, "Modified Title", modified.Title)
	require.Len(t, modified.Authors, 1)
	assert.Equal(t, "Original Author", modified.Authors[0].Name)
	assert.Contains(t, modified.Narrators, "Original Narrator")
	assert.Equal(t, "Fantasy", modified.Genre)
}

// TestWrite_PreservesUnknownAtoms tests that unknown/unrecognized atoms are preserved.
// This ensures tags like album_artist, copyright, date, etc. that we don't explicitly
// handle are still preserved when writing files.
func TestWrite_PreservesUnknownAtoms(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-unknown-atoms-*")

	const (
		expectedCopyright   = "©2024 Test Publisher"
		expectedAlbumArtist = "Test Album Artist"
	)

	// Generate M4B with metadata that includes atoms we treat as "unknown"
	// (album_artist is stored as aART, copyright as cprt - both are unknown atoms)
	path := testgen.GenerateM4B(t, dir, "test.m4b", testgen.M4BOptions{
		Title:       "Test Book",
		Artist:      "Test Author",
		Duration:    1.0,
		Copyright:   expectedCopyright,
		AlbumArtist: expectedAlbumArtist,
	})

	// Parse the file
	original, err := mp4.ParseFull(path)
	require.NoError(t, err)
	assert.Equal(t, "Test Book", original.Title)

	// Verify we captured unknown atoms
	assert.NotEmpty(t, original.UnknownAtoms, "Should have captured unknown atoms (aART, cprt)")

	// Find the specific unknown atoms we expect and verify their data
	var foundAlbumArtist, foundCopyright []byte
	for _, atom := range original.UnknownAtoms {
		atomType := string(atom.Type[:])
		if atomType == "aART" {
			foundAlbumArtist = atom.Data
		}
		if atomType == "cprt" {
			foundCopyright = atom.Data
		}
	}
	assert.NotNil(t, foundAlbumArtist, "Should have captured aART (album_artist) atom")
	assert.NotNil(t, foundCopyright, "Should have captured cprt (copyright) atom")

	// Modify the title and write back
	original.Title = "Modified Title"
	destPath := dir + "/modified.m4b"
	err = mp4.WriteToFile(path, destPath, original)
	require.NoError(t, err)

	// Re-read the modified file
	modified, err := mp4.ParseFull(destPath)
	require.NoError(t, err)

	// Verify title was modified
	assert.Equal(t, "Modified Title", modified.Title)

	// Verify unknown atoms were preserved with same data
	assert.Len(t, modified.UnknownAtoms, len(original.UnknownAtoms),
		"Unknown atoms count should be preserved")

	// Verify specific atoms are still present with the same raw data
	var modifiedAlbumArtist, modifiedCopyright []byte
	for _, atom := range modified.UnknownAtoms {
		atomType := string(atom.Type[:])
		if atomType == "aART" {
			modifiedAlbumArtist = atom.Data
		}
		if atomType == "cprt" {
			modifiedCopyright = atom.Data
		}
	}
	assert.NotNil(t, modifiedAlbumArtist, "aART atom should be preserved after write")
	assert.NotNil(t, modifiedCopyright, "cprt atom should be preserved after write")

	// Verify the raw atom data is identical (byte-for-byte preservation)
	assert.Equal(t, foundAlbumArtist, modifiedAlbumArtist,
		"aART atom data should be byte-for-byte identical")
	assert.Equal(t, foundCopyright, modifiedCopyright,
		"cprt atom data should be byte-for-byte identical")

	// Also verify using ffprobe that the actual tag values are correct
	tags := testgen.GetM4BTags(t, destPath)
	assert.Equal(t, expectedAlbumArtist, tags["album_artist"],
		"album_artist value should be preserved")
	assert.Equal(t, expectedCopyright, tags["copyright"],
		"copyright value should be preserved")
}

// TestWrite_PreservesCommentAndYear tests that comment and year fields are preserved.
func TestWrite_PreservesCommentAndYear(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-comment-year-*")

	// Generate M4B with comment and date
	path := testgen.GenerateM4B(t, dir, "test.m4b", testgen.M4BOptions{
		Title:    "Test Book",
		Duration: 1.0,
		Comment:  "This is a test comment with detailed description.",
		Date:     "2024",
	})

	// Parse the file
	original, err := mp4.ParseFull(path)
	require.NoError(t, err)
	assert.Equal(t, "This is a test comment with detailed description.", original.Comment)
	assert.Equal(t, "2024", original.Year)

	// Modify title and write to new file
	original.Title = "Modified Title"
	destPath := dir + "/modified.m4b"
	err = mp4.WriteToFile(path, destPath, original)
	require.NoError(t, err)

	// Re-read and verify comment and year are preserved
	modified, err := mp4.ParseFull(destPath)
	require.NoError(t, err)
	assert.Equal(t, "Modified Title", modified.Title)
	assert.Equal(t, "This is a test comment with detailed description.", modified.Comment)
	assert.Equal(t, "2024", modified.Year)
}
