package plugins

import (
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchResultToMetadata_AllFieldsPopulated(t *testing.T) {
	t.Parallel()
	seriesNum := 3.5
	sr := &SearchResult{
		Title:        "The Great Book",
		Subtitle:     "A Subtitle",
		Description:  "A detailed description",
		Publisher:    "Big Publisher",
		Imprint:      "Imprint Name",
		URL:          "https://example.com/book",
		CoverURL:     "https://example.com/cover.jpg",
		Series:       "Epic Series",
		SeriesNumber: &seriesNum,
		Authors: []mediafile.ParsedAuthor{
			{Name: "Author One", Role: "writer"},
			{Name: "Author Two", Role: "penciller"},
		},
		Narrators: []string{"Narrator A", "Narrator B"},
		Genres:    []string{"Fantasy", "Adventure"},
		Tags:      []string{"epic", "magic"},
		Identifiers: []mediafile.ParsedIdentifier{
			{Type: "isbn_13", Value: "9781234567890"},
			{Type: "asin", Value: "B00TEST1234"},
		},
		ReleaseDate: "2025-01-10",
		ImageURL:    "https://example.com/image.jpg",
	}

	md := SearchResultToMetadata(sr)

	assert.Equal(t, "The Great Book", md.Title)
	assert.Equal(t, "A Subtitle", md.Subtitle)
	assert.Equal(t, "A detailed description", md.Description)
	assert.Equal(t, "Big Publisher", md.Publisher)
	assert.Equal(t, "Imprint Name", md.Imprint)
	assert.Equal(t, "https://example.com/book", md.URL)
	assert.Equal(t, "https://example.com/cover.jpg", md.CoverURL)
	assert.Equal(t, "Epic Series", md.Series)
	require.NotNil(t, md.SeriesNumber)
	assert.InDelta(t, 3.5, *md.SeriesNumber, 0.001)

	require.Len(t, md.Authors, 2)
	assert.Equal(t, "Author One", md.Authors[0].Name)
	assert.Equal(t, "writer", md.Authors[0].Role)
	assert.Equal(t, "Author Two", md.Authors[1].Name)
	assert.Equal(t, "penciller", md.Authors[1].Role)

	assert.Equal(t, []string{"Narrator A", "Narrator B"}, md.Narrators)
	assert.Equal(t, []string{"Fantasy", "Adventure"}, md.Genres)
	assert.Equal(t, []string{"epic", "magic"}, md.Tags)

	require.Len(t, md.Identifiers, 2)
	assert.Equal(t, "isbn_13", md.Identifiers[0].Type)
	assert.Equal(t, "9781234567890", md.Identifiers[0].Value)
	assert.Equal(t, "asin", md.Identifiers[1].Type)
	assert.Equal(t, "B00TEST1234", md.Identifiers[1].Value)

	require.NotNil(t, md.ReleaseDate)
	assert.Equal(t, 2025, md.ReleaseDate.Year())
	assert.Equal(t, time.January, md.ReleaseDate.Month())
	assert.Equal(t, 10, md.ReleaseDate.Day())
}

func TestSearchResultToMetadata_DateParsing_DateOnly(t *testing.T) {
	t.Parallel()
	sr := &SearchResult{
		ReleaseDate: "2025-01-10",
	}

	md := SearchResultToMetadata(sr)

	require.NotNil(t, md.ReleaseDate)
	assert.Equal(t, 2025, md.ReleaseDate.Year())
	assert.Equal(t, time.January, md.ReleaseDate.Month())
	assert.Equal(t, 10, md.ReleaseDate.Day())
}

func TestSearchResultToMetadata_DateParsing_RFC3339(t *testing.T) {
	t.Parallel()
	sr := &SearchResult{
		ReleaseDate: "2025-01-10T00:00:00Z",
	}

	md := SearchResultToMetadata(sr)

	require.NotNil(t, md.ReleaseDate)
	assert.Equal(t, 2025, md.ReleaseDate.Year())
	assert.Equal(t, time.January, md.ReleaseDate.Month())
	assert.Equal(t, 10, md.ReleaseDate.Day())
}

func TestSearchResultToMetadata_DateParsing_Invalid(t *testing.T) {
	t.Parallel()
	sr := &SearchResult{
		ReleaseDate: "not-a-date",
	}

	md := SearchResultToMetadata(sr)

	assert.Nil(t, md.ReleaseDate)
}

func TestSearchResultToMetadata_EmptyResult(t *testing.T) {
	t.Parallel()
	sr := &SearchResult{}

	md := SearchResultToMetadata(sr)

	require.NotNil(t, md)
	assert.Empty(t, md.Title)
	assert.Empty(t, md.Subtitle)
	assert.Empty(t, md.Description)
	assert.Empty(t, md.Publisher)
	assert.Empty(t, md.Imprint)
	assert.Empty(t, md.URL)
	assert.Empty(t, md.CoverURL)
	assert.Empty(t, md.Series)
	assert.Nil(t, md.SeriesNumber)
	assert.Nil(t, md.Authors)
	assert.Nil(t, md.Narrators)
	assert.Nil(t, md.Genres)
	assert.Nil(t, md.Tags)
	assert.Nil(t, md.Identifiers)
	assert.Nil(t, md.ReleaseDate)
}

func TestSearchResultToMetadata_ImageURL_DoesNotFallbackToCoverURL(t *testing.T) {
	t.Parallel()
	// The ImageURL → CoverURL fallback happens in parseSearchResponse, not
	// in SearchResultToMetadata. So when CoverURL is empty and ImageURL is
	// set, the resulting ParsedMetadata.CoverURL should remain empty.
	sr := &SearchResult{
		ImageURL: "https://example.com/image.jpg",
		CoverURL: "",
	}

	md := SearchResultToMetadata(sr)

	assert.Empty(t, md.CoverURL, "CoverURL should be empty because the ImageURL fallback is in parseSearchResponse, not SearchResultToMetadata")
}
