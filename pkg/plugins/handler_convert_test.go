package plugins

import (
	"math"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertFieldsToMetadata_StringFields(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"title":       "My Book",
		"subtitle":    "A Subtitle",
		"description": "A description",
		"publisher":   "Publisher Co",
		"imprint":     "Imprint Name",
		"url":         "https://example.com",
		"series":      "Great Series",
		"cover_url":   "https://example.com/cover.jpg",
	}

	md := convertFieldsToMetadata(fields)

	assert.Equal(t, "My Book", md.Title)
	assert.Equal(t, "A Subtitle", md.Subtitle)
	assert.Equal(t, "A description", md.Description)
	assert.Equal(t, "Publisher Co", md.Publisher)
	assert.Equal(t, "Imprint Name", md.Imprint)
	assert.Equal(t, "https://example.com", md.URL)
	assert.Equal(t, "Great Series", md.Series)
	assert.Equal(t, "https://example.com/cover.jpg", md.CoverURL)
}

func TestConvertFieldsToMetadata_Authors(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"authors": []any{
			map[string]any{"name": "Author One", "role": "writer"},
			map[string]any{"name": "Author Two", "role": "penciller"},
		},
	}

	md := convertFieldsToMetadata(fields)

	require.Len(t, md.Authors, 2)
	assert.Equal(t, mediafile.ParsedAuthor{Name: "Author One", Role: "writer"}, md.Authors[0])
	assert.Equal(t, mediafile.ParsedAuthor{Name: "Author Two", Role: "penciller"}, md.Authors[1])
}

func TestConvertFieldsToMetadata_StringArrays(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"narrators": []any{"Narrator A", "Narrator B"},
		"genres":    []any{"Fantasy", "Sci-Fi"},
		"tags":      []any{"epic", "adventure"},
	}

	md := convertFieldsToMetadata(fields)

	assert.Equal(t, []string{"Narrator A", "Narrator B"}, md.Narrators)
	assert.Equal(t, []string{"Fantasy", "Sci-Fi"}, md.Genres)
	assert.Equal(t, []string{"epic", "adventure"}, md.Tags)
}

func TestConvertFieldsToMetadata_Identifiers(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"identifiers": []any{
			map[string]any{"type": "isbn_13", "value": "9781234567890"},
			map[string]any{"type": "asin", "value": "B00TEST1234"},
		},
	}

	md := convertFieldsToMetadata(fields)

	require.Len(t, md.Identifiers, 2)
	assert.Equal(t, mediafile.ParsedIdentifier{Type: "isbn_13", Value: "9781234567890"}, md.Identifiers[0])
	assert.Equal(t, mediafile.ParsedIdentifier{Type: "asin", Value: "B00TEST1234"}, md.Identifiers[1])
}

func TestConvertFieldsToMetadata_SeriesNumber(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"series_number": float64(2.5),
	}

	md := convertFieldsToMetadata(fields)

	require.NotNil(t, md.SeriesNumber)
	assert.InDelta(t, 2.5, *md.SeriesNumber, 0.001)
}

func TestConvertFieldsToMetadata_ReleaseDate_DateOnly(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"release_date": "2025-06-15",
	}

	md := convertFieldsToMetadata(fields)

	require.NotNil(t, md.ReleaseDate)
	assert.Equal(t, 2025, md.ReleaseDate.Year())
	assert.Equal(t, time.June, md.ReleaseDate.Month())
	assert.Equal(t, 15, md.ReleaseDate.Day())
}

func TestConvertFieldsToMetadata_ReleaseDate_RFC3339(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"release_date": "2025-06-15T00:00:00Z",
	}

	md := convertFieldsToMetadata(fields)

	require.NotNil(t, md.ReleaseDate)
	assert.Equal(t, 2025, md.ReleaseDate.Year())
	assert.Equal(t, time.June, md.ReleaseDate.Month())
	assert.Equal(t, 15, md.ReleaseDate.Day())
}

func TestConvertFieldsToMetadata_ReleaseDate_Invalid(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"release_date": "not-a-date",
	}

	md := convertFieldsToMetadata(fields)

	assert.Nil(t, md.ReleaseDate)
}

func TestConvertFieldsToMetadata_EmptyFields(t *testing.T) {
	t.Parallel()
	fields := map[string]any{}

	md := convertFieldsToMetadata(fields)

	require.NotNil(t, md)
	assert.Empty(t, md.Title)
	assert.Empty(t, md.Subtitle)
	assert.Empty(t, md.Description)
	assert.Empty(t, md.Publisher)
	assert.Empty(t, md.Imprint)
	assert.Empty(t, md.URL)
	assert.Empty(t, md.Series)
	assert.Empty(t, md.CoverURL)
	assert.Nil(t, md.SeriesNumber)
	assert.Nil(t, md.Authors)
	assert.Nil(t, md.Narrators)
	assert.Nil(t, md.Genres)
	assert.Nil(t, md.Tags)
	assert.Nil(t, md.Identifiers)
	assert.Nil(t, md.ReleaseDate)
}

func TestConvertFieldsToMetadata_WrongTypes(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"title":         123,       // int instead of string
		"subtitle":      true,      // bool instead of string
		"series_number": "not-num", // string instead of float64
		"authors":       "not-a-slice",
		"narrators":     42,
		"genres":        false,
		"tags":          map[string]any{},
		"identifiers":   "nope",
		"release_date":  999,
	}

	md := convertFieldsToMetadata(fields)

	require.NotNil(t, md)
	assert.Empty(t, md.Title)
	assert.Empty(t, md.Subtitle)
	assert.Nil(t, md.SeriesNumber)
	assert.Nil(t, md.Authors)
	assert.Nil(t, md.Narrators)
	assert.Nil(t, md.Genres)
	assert.Nil(t, md.Tags)
	assert.Nil(t, md.Identifiers)
	assert.Nil(t, md.ReleaseDate)
}

func TestConvertFieldsToMetadata_EmptyStringValues(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"title":    "",
		"subtitle": "",
	}

	md := convertFieldsToMetadata(fields)

	assert.Empty(t, md.Title)
	assert.Empty(t, md.Subtitle)
}

func TestConvertFieldsToMetadata_AuthorsWithMissingName(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"authors": []any{
			map[string]any{"role": "writer"},                         // missing name
			map[string]any{"name": "", "role": "writer"},             // empty name
			map[string]any{"name": "Valid Author", "role": "writer"}, // valid
		},
	}

	md := convertFieldsToMetadata(fields)

	// Authors with missing or empty names should be skipped
	require.Len(t, md.Authors, 1)
	assert.Equal(t, "Valid Author", md.Authors[0].Name)
}

func TestConvertFieldsToMetadata_IdentifiersWithMissingTypeOrValue(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"identifiers": []any{
			map[string]any{"type": "isbn_13"},                           // missing value
			map[string]any{"value": "9781234567890"},                    // missing type
			map[string]any{"type": "", "value": "9781234567890"},        // empty type
			map[string]any{"type": "isbn_13", "value": ""},              // empty value
			map[string]any{"type": "isbn_13", "value": "9781234567890"}, // valid
		},
	}

	md := convertFieldsToMetadata(fields)

	// Only identifiers with both type and value populated should be included
	require.Len(t, md.Identifiers, 1)
	assert.Equal(t, "isbn_13", md.Identifiers[0].Type)
	assert.Equal(t, "9781234567890", md.Identifiers[0].Value)
}

func TestConvertFieldsToMetadata_Language(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"language": "en-US",
	}

	md := convertFieldsToMetadata(fields)

	require.NotNil(t, md.Language)
	assert.Equal(t, "en-US", *md.Language)
}

func TestConvertFieldsToMetadata_LanguageNormalized(t *testing.T) {
	t.Parallel()
	// ISO 639-2/T three-letter code should be normalized to BCP 47
	fields := map[string]any{
		"language": "eng",
	}

	md := convertFieldsToMetadata(fields)

	require.NotNil(t, md.Language)
	assert.Equal(t, "en", *md.Language)
}

func TestConvertFieldsToMetadata_LanguageInvalid(t *testing.T) {
	t.Parallel()
	// Invalid tags should be dropped (nil)
	fields := map[string]any{
		"language": "not-a-language",
	}

	md := convertFieldsToMetadata(fields)

	assert.Nil(t, md.Language)
}

func TestConvertFieldsToMetadata_LanguageEmpty(t *testing.T) {
	t.Parallel()
	// Empty string should be ignored (nil)
	fields := map[string]any{
		"language": "",
	}

	md := convertFieldsToMetadata(fields)

	assert.Nil(t, md.Language)
}

func TestConvertFieldsToMetadata_AbridgedTrue(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"abridged": true,
	}

	md := convertFieldsToMetadata(fields)

	require.NotNil(t, md.Abridged)
	assert.True(t, *md.Abridged)
}

func TestConvertFieldsToMetadata_AbridgedFalse(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"abridged": false,
	}

	md := convertFieldsToMetadata(fields)

	require.NotNil(t, md.Abridged)
	assert.False(t, *md.Abridged)
}

func TestConvertFieldsToMetadata_CoverPage(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"cover_page": float64(4), // JSON numbers decode to float64
	}

	md := convertFieldsToMetadata(fields)

	require.NotNil(t, md.CoverPage)
	assert.Equal(t, 4, *md.CoverPage)
}

func TestConvertFieldsToMetadata_CoverPage_Missing(t *testing.T) {
	t.Parallel()
	md := convertFieldsToMetadata(map[string]any{})
	assert.Nil(t, md.CoverPage)
}

func TestConvertFieldsToMetadata_CoverPage_Invalid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		value float64
	}{
		{"negative", -1},
		{"NaN", math.NaN()},
		{"positive infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			md := convertFieldsToMetadata(map[string]any{"cover_page": tc.value})
			assert.Nil(t, md.CoverPage, "invalid coverPage %v should be rejected", tc.value)
		})
	}
}

func TestConvertFieldsToMetadata_AbridgedMissing(t *testing.T) {
	t.Parallel()
	// Absent key should result in nil
	fields := map[string]any{
		"title": "My Book",
	}

	md := convertFieldsToMetadata(fields)

	assert.Nil(t, md.Abridged)
}

func TestConvertFieldsToMetadata_SeriesNumberUnit(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"series_number_unit": "chapter",
	}

	md := convertFieldsToMetadata(fields)

	require.NotNil(t, md.SeriesNumberUnit)
	assert.Equal(t, "chapter", *md.SeriesNumberUnit)
}

func TestConvertFieldsToMetadata_SeriesNumberUnitVolume(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"series_number_unit": "volume",
	}

	md := convertFieldsToMetadata(fields)

	require.NotNil(t, md.SeriesNumberUnit)
	assert.Equal(t, "volume", *md.SeriesNumberUnit)
}

func TestConvertFieldsToMetadata_SeriesNumberUnitInvalid(t *testing.T) {
	t.Parallel()
	// Invalid values must be dropped (nil), not passed through
	fields := map[string]any{
		"series_number_unit": "bogus",
	}

	md := convertFieldsToMetadata(fields)

	assert.Nil(t, md.SeriesNumberUnit)
}

func TestConvertFieldsToMetadata_SeriesNumberUnitMissing(t *testing.T) {
	t.Parallel()
	// Absent key should result in nil
	fields := map[string]any{
		"series_number": float64(3),
	}

	md := convertFieldsToMetadata(fields)

	assert.Nil(t, md.SeriesNumberUnit)
}
