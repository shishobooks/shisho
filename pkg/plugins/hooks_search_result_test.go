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
			],
			language: "en-US",
			abridged: true
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

	require.NotNil(t, md.Language)
	assert.Equal(t, "en-US", *md.Language)
	require.NotNil(t, md.Abridged)
	assert.True(t, *md.Abridged)
}

func TestParseSearchResponse_LanguageNormalized(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	val, err := vm.RunString(`({
		results: [{
			title: "Book",
			language: "eng"
		}]
	})`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "s", "p")
	require.Len(t, resp.Results, 1)

	md := resp.Results[0]
	require.NotNil(t, md.Language)
	assert.Equal(t, "en", *md.Language)
}

func TestParseSearchResponse_LanguageInvalid(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	val, err := vm.RunString(`({
		results: [{
			title: "Book",
			language: "not-a-language"
		}]
	})`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "s", "p")
	require.Len(t, resp.Results, 1)

	md := resp.Results[0]
	assert.Nil(t, md.Language)
}

func TestParseSearchResponse_AbridgedFalse(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	val, err := vm.RunString(`({
		results: [{
			title: "Book",
			abridged: false
		}]
	})`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "s", "p")
	require.Len(t, resp.Results, 1)

	md := resp.Results[0]
	require.NotNil(t, md.Abridged)
	assert.False(t, *md.Abridged)
}

func TestParseSearchResponse_AbridgedAbsent(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	val, err := vm.RunString(`({
		results: [{
			title: "Book"
		}]
	})`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "s", "p")
	require.Len(t, resp.Results, 1)

	md := resp.Results[0]
	assert.Nil(t, md.Abridged)
	assert.Nil(t, md.Language)
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

func TestParseSearchResponse_Confidence(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	val, err := vm.RunString(`({ results: [
		{ title: "High", confidence: 0.95 },
		{ title: "Low", confidence: 0.3 },
		{ title: "None" }
	] })`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "", "")
	require.Len(t, resp.Results, 3)

	require.NotNil(t, resp.Results[0].Confidence)
	assert.InDelta(t, 0.95, *resp.Results[0].Confidence, 0.001)

	require.NotNil(t, resp.Results[1].Confidence)
	assert.InDelta(t, 0.3, *resp.Results[1].Confidence, 0.001)

	assert.Nil(t, resp.Results[2].Confidence)
}

func TestParseSearchResponse_DescriptionHTMLStripped(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	val, err := vm.RunString(`({
		results: [{
			title: "Test Book",
			description: "<p>First paragraph.</p><p>Second paragraph.</p>"
		}]
	})`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "s", "p")
	require.Len(t, resp.Results, 1)

	// Description should have HTML stripped, with paragraph breaks preserved
	assert.Equal(t, "First paragraph.\n\nSecond paragraph.", resp.Results[0].Description)
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

func TestParseSearchResponse_CoverPage(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	val, err := vm.RunString(`({ results: [{ title: "x", coverPage: 3 }] })`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "test", "plugin-id")

	require.Len(t, resp.Results, 1)
	require.NotNil(t, resp.Results[0].CoverPage)
	assert.Equal(t, 3, *resp.Results[0].CoverPage)
}

func TestParseSearchResponse_CoverPage_MissingOrNull(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	val, err := vm.RunString(`({ results: [{ title: "a" }, { title: "b", coverPage: null }] })`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "test", "plugin-id")

	require.Len(t, resp.Results, 2)
	assert.Nil(t, resp.Results[0].CoverPage)
	assert.Nil(t, resp.Results[1].CoverPage)
}

func TestParseSearchResponse_CoverPage_Invalid(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	// Negative, NaN, and Infinity values must be rejected at parse time so
	// they don't propagate to the apply path and cause broken previews.
	val, err := vm.RunString(`({ results: [
		{ title: "neg", coverPage: -1 },
		{ title: "nan", coverPage: NaN },
		{ title: "inf", coverPage: Infinity },
		{ title: "zero", coverPage: 0 },
	] })`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "test", "plugin-id")

	require.Len(t, resp.Results, 4)
	assert.Nil(t, resp.Results[0].CoverPage, "negative coverPage should be rejected")
	assert.Nil(t, resp.Results[1].CoverPage, "NaN coverPage should be rejected")
	assert.Nil(t, resp.Results[2].CoverPage, "Infinity coverPage should be rejected")
	require.NotNil(t, resp.Results[3].CoverPage, "0 is a valid coverPage")
	assert.Equal(t, 0, *resp.Results[3].CoverPage)
}
