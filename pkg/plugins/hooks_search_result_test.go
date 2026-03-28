package plugins

import (
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSearchResponse_AllFields(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	val, err := vm.RunString(`({
		results: [{
			title: "The Great Book",
			subtitle: "A Subtitle",
			description: "A detailed description",
			publisher: "Big Publisher",
			imprint: "Imprint Name",
			url: "https://example.com/book",
			coverUrl: "https://example.com/cover.jpg",
			series: "Epic Series",
			seriesNumber: 3.5,
			releaseDate: "2025-01-10",
			authors: [
				{ name: "Author One", role: "writer" },
				{ name: "Author Two", role: "penciller" }
			],
			narrators: ["Narrator A", "Narrator B"],
			genres: ["Fantasy", "Adventure"],
			tags: ["epic", "magic"],
			identifiers: [
				{ type: "isbn_13", value: "9781234567890" },
				{ type: "asin", value: "B00TEST1234" }
			]
		}]
	})`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "test-scope", "test-plugin")
	require.Len(t, resp.Results, 1)

	md := resp.Results[0]
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

	require.NotNil(t, md.ReleaseDate)
	assert.Equal(t, 2025, md.ReleaseDate.Year())
	assert.Equal(t, time.January, md.ReleaseDate.Month())
	assert.Equal(t, 10, md.ReleaseDate.Day())

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

	assert.Equal(t, "test-scope", md.PluginScope)
	assert.Equal(t, "test-plugin", md.PluginID)
}

func TestParseSearchResponse_DateParsing_RFC3339(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	val, err := vm.RunString(`({
		results: [{
			title: "RFC3339 Date Book",
			releaseDate: "2025-01-10T00:00:00Z"
		}]
	})`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "s", "p")
	require.Len(t, resp.Results, 1)

	md := resp.Results[0]
	require.NotNil(t, md.ReleaseDate)
	assert.Equal(t, 2025, md.ReleaseDate.Year())
	assert.Equal(t, time.January, md.ReleaseDate.Month())
	assert.Equal(t, 10, md.ReleaseDate.Day())
}

func TestParseSearchResponse_DateParsing_Invalid(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	val, err := vm.RunString(`({
		results: [{
			title: "Bad Date Book",
			releaseDate: "not-a-date"
		}]
	})`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "s", "p")
	require.Len(t, resp.Results, 1)

	assert.Nil(t, resp.Results[0].ReleaseDate)
}

func TestParseSearchResponse_EmptyResults(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	val, err := vm.RunString(`({ results: [] })`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "s", "p")
	assert.Empty(t, resp.Results)
}

func TestParseSearchResponse_NilInput(t *testing.T) {
	t.Parallel()

	vm := goja.New()

	// nil value
	resp := parseSearchResponse(vm, nil, "s", "p")
	assert.Empty(t, resp.Results)

	// undefined value
	resp = parseSearchResponse(vm, goja.Undefined(), "s", "p")
	assert.Empty(t, resp.Results)

	// null value
	val, err := vm.RunString(`null`)
	require.NoError(t, err)
	resp = parseSearchResponse(vm, val, "s", "p")
	assert.Empty(t, resp.Results)
}
