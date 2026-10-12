package mobi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// calibreOptions mirrors what Calibre 9 writes for a book with full metadata.
func calibreOptions(kind testgen.MOBIKind) testgen.MOBIOptions {
	return testgen.MOBIOptions{
		Kind:           kind,
		Title:          "The Test Book",
		Authors:        []string{"Doe, Jane", "John Smith"},
		Description:    "<p>A <b>great</b> book.</p>",
		Publisher:      "Big Pub",
		PublishingDate: "2021-03-04T06:00:00+00:00",
		Language:       "fr",
		ISBN:           "9780306406157",
		Source:         "calibre:57b1e498-e4f8-4ab9-8838-9a87118f9a2b",
		ASIN:           "57b1e498-e4f8-4ab9-8838-9a87118f9a2b",
		Subjects:       []string{"Fantasy;Adventure", "Epic"},
		HasCover:       true,
	}
}

func parseBuilt(t *testing.T, opts testgen.MOBIOptions) (*mediafile.ParsedMetadata, error) {
	t.Helper()
	path := testgen.GenerateMOBI(t, t.TempDir(), "book.mobi", opts)
	return Parse(path)
}

func TestParse_MetadataMapping(t *testing.T) {
	t.Parallel()

	for name, kind := range map[string]testgen.MOBIKind{
		"MOBI6": testgen.MOBIKindMOBI6,
		"KF8":   testgen.MOBIKindKF8,
		"combo": testgen.MOBIKindCombo,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			meta, err := parseBuilt(t, calibreOptions(kind))
			require.NoError(t, err)

			assert.Equal(t, "The Test Book", meta.Title)
			assert.Equal(t, []mediafile.ParsedAuthor{{Name: "Jane Doe"}, {Name: "John Smith"}}, meta.Authors)
			assert.Equal(t, "A great book.", meta.Description)
			assert.Equal(t, "Big Pub", meta.Publisher)
			require.NotNil(t, meta.ReleaseDate)
			assert.Equal(t, time.Date(2021, 3, 4, 6, 0, 0, 0, time.UTC), meta.ReleaseDate.UTC())
			require.NotNil(t, meta.Language)
			assert.Equal(t, "fr", *meta.Language)
			assert.Equal(t, []mediafile.ParsedIdentifier{{Type: "isbn_13", Value: "9780306406157"}}, meta.Identifiers,
				"Calibre's UUID in the ASIN and source records is not an identifier")
			assert.Equal(t, []string{"Fantasy", "Adventure", "Epic"}, meta.Genres)
			assert.Equal(t, models.DataSourceMOBIMetadata, meta.DataSource)
			assert.Equal(t, "image/jpeg", meta.CoverMimeType)
			assert.NotEmpty(t, meta.CoverData)

			assert.Empty(t, meta.Series)
			assert.Nil(t, meta.SeriesNumber)
			assert.Empty(t, meta.Tags)
			assert.Empty(t, meta.Narrators)
		})
	}
}

func TestParse_TitleFallsBackToFullName(t *testing.T) {
	t.Parallel()

	meta, err := parseBuilt(t, testgen.MOBIOptions{FullName: "Header Title"})
	require.NoError(t, err)
	assert.Equal(t, "Header Title", meta.Title)

	meta, err = parseBuilt(t, testgen.MOBIOptions{Title: "Updated Title", FullName: "Header Title"})
	require.NoError(t, err)
	assert.Equal(t, "Updated Title", meta.Title, "the updated-title record wins")
}

func TestParse_AuthorNames(t *testing.T) {
	t.Parallel()

	meta, err := parseBuilt(t, testgen.MOBIOptions{
		Title: "Book",
		Authors: []string{
			"Doe, Jane",
			"  Le Guin ,  Ursula K.  ",
			"Sammy Davis, Jr.",
			"King, Martin Luther, Jr.",
			"Plain Name",
			"Doe, Jane",
			"",
		},
	})
	require.NoError(t, err)

	names := make([]string, 0, len(meta.Authors))
	for _, a := range meta.Authors {
		names = append(names, a.Name)
		assert.Empty(t, a.Role, "MOBI authors carry no role")
	}
	assert.Equal(t, []string{
		"Jane Doe",
		"Ursula K. Le Guin",
		"Sammy Davis, Jr.",
		"King, Martin Luther, Jr.",
		"Plain Name",
	}, names)
}

func TestParse_ImprintOverridesPublisher(t *testing.T) {
	t.Parallel()

	meta, err := parseBuilt(t, testgen.MOBIOptions{Title: "Book", Publisher: "Parent House", Imprint: "Small Imprint"})
	require.NoError(t, err)
	assert.Equal(t, "Small Imprint", meta.Publisher)
}

func TestParse_Identifiers(t *testing.T) {
	t.Parallel()

	meta, err := parseBuilt(t, testgen.MOBIOptions{
		Title:  "Book",
		Source: "urn:isbn:0-306-40615-2",
		ASIN:   "B00ABCDEFG",
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []mediafile.ParsedIdentifier{
		{Type: "isbn_10", Value: "0-306-40615-2"},
		{Type: "asin", Value: "B00ABCDEFG"},
	}, meta.Identifiers)

	meta, err = parseBuilt(t, testgen.MOBIOptions{
		Title:  "Book",
		ISBN:   "978-0-306-40615-7",
		Source: "urn:isbn:9780306406157",
	})
	require.NoError(t, err)
	assert.Equal(t, []mediafile.ParsedIdentifier{{Type: "isbn_13", Value: "978-0-306-40615-7"}}, meta.Identifiers,
		"the same ISBN from both records is read once")

	meta, err = parseBuilt(t, testgen.MOBIOptions{Title: "Book", ISBN: "not an isbn"})
	require.NoError(t, err)
	assert.Empty(t, meta.Identifiers)
}

func TestParse_ReleaseDateFormats(t *testing.T) {
	t.Parallel()

	tests := map[string]time.Time{
		"2021-03-04T06:00:00+00:00": time.Date(2021, 3, 4, 6, 0, 0, 0, time.UTC),
		"2021-03-04T06:00:00Z":      time.Date(2021, 3, 4, 6, 0, 0, 0, time.UTC),
		"2021-03-04":                time.Date(2021, 3, 4, 0, 0, 0, 0, time.UTC),
		"2021":                      time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	for value, want := range tests {
		meta, err := parseBuilt(t, testgen.MOBIOptions{Title: "Book", PublishingDate: value})
		require.NoError(t, err)
		require.NotNil(t, meta.ReleaseDate, value)
		assert.Equal(t, want, meta.ReleaseDate.UTC(), value)
	}

	meta, err := parseBuilt(t, testgen.MOBIOptions{Title: "Book", PublishingDate: "someday"})
	require.NoError(t, err)
	assert.Nil(t, meta.ReleaseDate)
}

func TestParse_Language(t *testing.T) {
	t.Parallel()

	meta, err := parseBuilt(t, testgen.MOBIOptions{Title: "Book", Language: "en-us"})
	require.NoError(t, err)
	require.NotNil(t, meta.Language)
	assert.Equal(t, "en-US", *meta.Language)

	meta, err = parseBuilt(t, testgen.MOBIOptions{Title: "Book"})
	require.NoError(t, err)
	assert.Nil(t, meta.Language)
}

func TestParse_ComboReadsKF8Half(t *testing.T) {
	t.Parallel()

	opts := calibreOptions(testgen.MOBIKindCombo)
	opts.MOBI6Title = "Old Half Title"
	meta, err := parseBuilt(t, opts)
	require.NoError(t, err)
	assert.Equal(t, "The Test Book", meta.Title)
	assert.NotEmpty(t, meta.CoverData, "the cover comes from the images the two halves share")
}

func TestParse_Cover(t *testing.T) {
	t.Parallel()

	opts := calibreOptions(testgen.MOBIKindKF8)
	opts.CoverMimeType = "image/png"
	meta, err := parseBuilt(t, opts)
	require.NoError(t, err)
	assert.Equal(t, "image/png", meta.CoverMimeType)
	assert.Equal(t, []byte("\x89PNG\r\n\x1a\n"), meta.CoverData[:8])

	for name, kind := range map[string]testgen.MOBIKind{
		"MOBI6": testgen.MOBIKindMOBI6,
		"KF8":   testgen.MOBIKindKF8,
		"combo": testgen.MOBIKindCombo,
	} {
		opts := calibreOptions(kind)
		opts.HasCover = false
		meta, err := parseBuilt(t, opts)
		require.NoError(t, err, name)
		assert.Empty(t, meta.CoverData, name)
		assert.Empty(t, meta.CoverMimeType, name)
	}
}

func TestParse_Compression(t *testing.T) {
	t.Parallel()

	for name, compression := range map[string]testgen.MOBICompression{
		"none":     testgen.MOBICompressionNone,
		"PalmDOC":  testgen.MOBICompressionPalmDOC,
		"HUFFCDIC": testgen.MOBICompressionHuffCDIC,
	} {
		opts := calibreOptions(testgen.MOBIKindMOBI6)
		opts.Compression = compression
		meta, err := parseBuilt(t, opts)
		require.NoError(t, err, name)
		assert.Equal(t, "The Test Book", meta.Title, name)
		assert.NotEmpty(t, meta.CoverData, name)
	}
}

func TestParse_CP1252(t *testing.T) {
	t.Parallel()

	meta, err := parseBuilt(t, testgen.MOBIOptions{
		Title:   "Café Société",
		Authors: []string{"Brontë, Anne"},
		CP1252:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, "Café Société", meta.Title)
	assert.Equal(t, "Anne Brontë", meta.Authors[0].Name)
}

func TestParse_DRMProtected(t *testing.T) {
	t.Parallel()

	for name, kind := range map[string]testgen.MOBIKind{
		"MOBI6": testgen.MOBIKindMOBI6,
		"KF8":   testgen.MOBIKindKF8,
		"combo": testgen.MOBIKindCombo,
	} {
		opts := calibreOptions(kind)
		opts.Encrypted = true
		_, err := parseBuilt(t, opts)
		require.Error(t, err, name)
		assert.ErrorIs(t, err, ErrDRMProtected, name)
	}
}

func TestParse_NotMOBI(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	testgen.GenerateEPUB(t, dir, "book.epub", testgen.EPUBOptions{Title: "EPUB"})
	require.NoError(t, os.Rename(filepath.Join(dir, "book.epub"), filepath.Join(dir, "book.mobi")))

	_, err := Parse(filepath.Join(dir, "book.mobi"))
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrDRMProtected))
}

func TestParse_Truncated(t *testing.T) {
	t.Parallel()

	data := testgen.BuildMOBI(t, calibreOptions(testgen.MOBIKindMOBI6))
	for _, size := range []int{10, 78, 100, 300, len(data) / 2} {
		path := testgen.WriteFile(t, t.TempDir(), "book.mobi", data[:size])
		_, err := Parse(path)
		assert.Error(t, err, "size %d", size)
	}
}

// nestedTOC is a table of contents three levels deep.
var nestedTOC = []testgen.MOBITOCEntry{
	{Title: "Part One", Children: []testgen.MOBITOCEntry{
		{Title: "Chapter 1"},
		{Title: "Chapter 2", Children: []testgen.MOBITOCEntry{{Title: "Scene 2.1"}}},
	}},
	{Title: "Part Two", Children: []testgen.MOBITOCEntry{{Title: "Chapter 3"}}},
	{Title: "Epilogue"},
}

// chapterTitles returns the chapters as nested titles: "Title" for a leaf,
// "Title[child, ...]" for a chapter with children.
func chapterTitles(chapters []mediafile.ParsedChapter) []string {
	out := make([]string, 0, len(chapters))
	for _, ch := range chapters {
		s := ch.Title
		if len(ch.Children) > 0 {
			s += "[" + strings.Join(chapterTitles(ch.Children), ", ") + "]"
		}
		out = append(out, s)
	}
	return out
}

var nestedTitles = []string{"Part One[Chapter 1, Chapter 2[Scene 2.1]]", "Part Two[Chapter 3]", "Epilogue"}

func TestParse_Chapters(t *testing.T) {
	t.Parallel()

	for name, kind := range map[string]testgen.MOBIKind{
		"MOBI6": testgen.MOBIKindMOBI6,
		"KF8":   testgen.MOBIKindKF8,
		"combo": testgen.MOBIKindCombo,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opts := calibreOptions(kind)
			opts.TOC = nestedTOC
			meta, err := parseBuilt(t, opts)
			require.NoError(t, err)
			assert.Equal(t, nestedTitles, chapterTitles(meta.Chapters))
			for _, ch := range meta.Chapters {
				require.NotNil(t, ch.Href, ch.Title)
				if kind == testgen.MOBIKindMOBI6 {
					assert.Regexp(t, `^filepos:\d{10}$`, *ch.Href)
				} else {
					assert.Regexp(t, `^kindle:pos:fid:[0-9A-V]{4}:off:[0-9A-V]{10}$`, *ch.Href)
				}
			}
		})
	}
}

func TestParse_ChaptersFlatMOBI6(t *testing.T) {
	t.Parallel()

	// Calibre writes a MOBI6 index flat, every entry at depth 0.
	opts := calibreOptions(testgen.MOBIKindMOBI6)
	opts.TOC = nestedTOC
	opts.FlatMOBI6TOC = true
	meta, err := parseBuilt(t, opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"Part One", "Chapter 1", "Chapter 2", "Scene 2.1", "Part Two", "Chapter 3", "Epilogue"}, chapterTitles(meta.Chapters))
}

func TestParse_ChaptersComboPrefersKF8Half(t *testing.T) {
	t.Parallel()

	opts := calibreOptions(testgen.MOBIKindCombo)
	opts.TOC = nestedTOC
	opts.FlatMOBI6TOC = true
	meta, err := parseBuilt(t, opts)
	require.NoError(t, err)
	assert.Equal(t, nestedTitles, chapterTitles(meta.Chapters))

	// A KF8 half with no index leaves the MOBI6 half's.
	opts = calibreOptions(testgen.MOBIKindCombo)
	opts.MOBI6TOC = []testgen.MOBITOCEntry{{Title: "Old One"}, {Title: "Old Two"}}
	meta, err = parseBuilt(t, opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"Old One", "Old Two"}, chapterTitles(meta.Chapters))
}

func TestParse_ChaptersCP1252(t *testing.T) {
	t.Parallel()

	opts := calibreOptions(testgen.MOBIKindMOBI6)
	opts.CP1252 = true
	opts.TOC = []testgen.MOBITOCEntry{{Title: "Café"}}
	meta, err := parseBuilt(t, opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"Café"}, chapterTitles(meta.Chapters))
}

func TestParse_NoTOC(t *testing.T) {
	t.Parallel()

	meta, err := parseBuilt(t, calibreOptions(testgen.MOBIKindKF8))
	require.NoError(t, err)
	assert.Empty(t, meta.Chapters)
}
