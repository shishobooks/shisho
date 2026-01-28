package plugins

import (
	"encoding/json"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupURLNamespace(t *testing.T) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	shishoObj := vm.NewObject()
	err := vm.Set("shisho", shishoObj)
	require.NoError(t, err)
	err = injectURLNamespace(vm, shishoObj)
	require.NoError(t, err)
	return vm
}

func TestURLEncodeURIComponent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple string",
			input:    "hello world",
			expected: "hello+world",
		},
		{
			name:     "special characters",
			input:    "foo=bar&baz=qux",
			expected: "foo%3Dbar%26baz%3Dqux",
		},
		{
			name:     "unicode characters",
			input:    "日本語",
			expected: "%E6%97%A5%E6%9C%AC%E8%AA%9E",
		},
		{
			name:     "already safe characters",
			input:    "abc123",
			expected: "abc123",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "url with query",
			input:    "https://example.com?foo=bar",
			expected: "https%3A%2F%2Fexample.com%3Ffoo%3Dbar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vm := setupURLNamespace(t)
			val, err := vm.RunString(`shisho.url.encodeURIComponent("` + tt.input + `")`)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, val.String())
		})
	}
}

func TestURLDecodeURIComponent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple encoded string",
			input:    "hello+world",
			expected: "hello world",
		},
		{
			name:     "percent encoded",
			input:    "hello%20world",
			expected: "hello world",
		},
		{
			name:     "special characters",
			input:    "foo%3Dbar%26baz%3Dqux",
			expected: "foo=bar&baz=qux",
		},
		{
			name:     "unicode characters",
			input:    "%E6%97%A5%E6%9C%AC%E8%AA%9E",
			expected: "日本語",
		},
		{
			name:     "already decoded",
			input:    "abc123",
			expected: "abc123",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vm := setupURLNamespace(t)
			val, err := vm.RunString(`shisho.url.decodeURIComponent("` + tt.input + `")`)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, val.String())
		})
	}
}

func TestURLDecodeURIComponent_InvalidInput(t *testing.T) {
	t.Parallel()
	vm := setupURLNamespace(t)
	_, err := vm.RunString(`shisho.url.decodeURIComponent("%zz")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shisho.url.decodeURIComponent")
}

func TestURLSearchParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "simple object",
			script:   `shisho.url.searchParams({foo: "bar", baz: "qux"})`,
			expected: "baz=qux&foo=bar", // sorted alphabetically
		},
		{
			name:     "single key",
			script:   `shisho.url.searchParams({query: "search term"})`,
			expected: "query=search+term",
		},
		{
			name:     "special characters in values",
			script:   `shisho.url.searchParams({url: "https://example.com"})`,
			expected: "url=https%3A%2F%2Fexample.com",
		},
		{
			name:     "numeric values",
			script:   `shisho.url.searchParams({page: 1, limit: 10})`,
			expected: "limit=10&page=1",
		},
		{
			name:     "empty object",
			script:   `shisho.url.searchParams({})`,
			expected: "",
		},
		{
			name:     "array values",
			script:   `shisho.url.searchParams({tags: ["a", "b", "c"]})`,
			expected: "tags=a&tags=b&tags=c",
		},
		{
			name:     "null and undefined values skipped",
			script:   `shisho.url.searchParams({foo: "bar", skip: null, also: undefined, baz: "qux"})`,
			expected: "baz=qux&foo=bar",
		},
		{
			name:     "boolean values",
			script:   `shisho.url.searchParams({active: true, deleted: false})`,
			expected: "active=true&deleted=false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vm := setupURLNamespace(t)
			val, err := vm.RunString(tt.script)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, val.String())
		})
	}
}

func TestURLSearchParams_MissingArgument(t *testing.T) {
	t.Parallel()
	vm := setupURLNamespace(t)
	_, err := vm.RunString(`shisho.url.searchParams()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "object argument is required")
}

func TestURLParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		url      string
		expected map[string]interface{}
	}{
		{
			name: "full URL with all components",
			url:  "https://user:pass@example.com:8080/path/to/resource?foo=bar&baz=qux#section",
			expected: map[string]interface{}{
				"protocol": "https",
				"host":     "example.com:8080",
				"hostname": "example.com",
				"port":     "8080",
				"pathname": "/path/to/resource",
				"search":   "?foo=bar&baz=qux",
				"hash":     "#section",
				"username": "user",
				"password": "pass",
			},
		},
		{
			name: "simple URL",
			url:  "https://example.com/path",
			expected: map[string]interface{}{
				"protocol": "https",
				"host":     "example.com",
				"hostname": "example.com",
				"port":     "",
				"pathname": "/path",
				"search":   "",
				"hash":     "",
				"username": "",
				"password": "",
			},
		},
		{
			name: "URL with query only",
			url:  "https://api.example.com/search?q=test",
			expected: map[string]interface{}{
				"protocol": "https",
				"host":     "api.example.com",
				"hostname": "api.example.com",
				"port":     "",
				"pathname": "/search",
				"search":   "?q=test",
				"hash":     "",
				"username": "",
				"password": "",
			},
		},
		{
			name: "URL with hash only",
			url:  "https://example.com/page#anchor",
			expected: map[string]interface{}{
				"protocol": "https",
				"host":     "example.com",
				"hostname": "example.com",
				"port":     "",
				"pathname": "/page",
				"search":   "",
				"hash":     "#anchor",
				"username": "",
				"password": "",
			},
		},
		{
			name: "URL without path",
			url:  "https://example.com",
			expected: map[string]interface{}{
				"protocol": "https",
				"host":     "example.com",
				"hostname": "example.com",
				"port":     "",
				"pathname": "",
				"search":   "",
				"hash":     "",
				"username": "",
				"password": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vm := setupURLNamespace(t)

			script := `JSON.stringify(shisho.url.parse("` + tt.url + `"))`
			val, err := vm.RunString(script)
			require.NoError(t, err)

			var result map[string]interface{}
			err = json.Unmarshal([]byte(val.String()), &result)
			require.NoError(t, err)

			assert.Equal(t, tt.url, result["href"])
			for key, expected := range tt.expected {
				assert.Equal(t, expected, result[key], "mismatch for key %s", key)
			}
		})
	}
}

func TestURLParse_Query(t *testing.T) {
	t.Parallel()
	vm := setupURLNamespace(t)

	script := `
		var parsed = shisho.url.parse("https://example.com/search?q=test&page=1&sort=desc");
		JSON.stringify(parsed.query);
	`
	val, err := vm.RunString(script)
	require.NoError(t, err)

	var query map[string]interface{}
	err = json.Unmarshal([]byte(val.String()), &query)
	require.NoError(t, err)

	assert.Equal(t, "test", query["q"])
	assert.Equal(t, "1", query["page"])
	assert.Equal(t, "desc", query["sort"])
}

func TestURLParse_QueryMultipleValues(t *testing.T) {
	t.Parallel()
	vm := setupURLNamespace(t)

	script := `
		var parsed = shisho.url.parse("https://example.com/search?tag=a&tag=b&tag=c");
		JSON.stringify(parsed.query);
	`
	val, err := vm.RunString(script)
	require.NoError(t, err)

	var query map[string]interface{}
	err = json.Unmarshal([]byte(val.String()), &query)
	require.NoError(t, err)

	tags, ok := query["tag"].([]interface{})
	require.True(t, ok, "expected tag to be an array")
	assert.Equal(t, []interface{}{"a", "b", "c"}, tags)
}

func TestURLParse_MissingArgument(t *testing.T) {
	t.Parallel()
	vm := setupURLNamespace(t)
	_, err := vm.RunString(`shisho.url.parse()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "url argument is required")
}

func TestURLEncodeURIComponent_MissingArgument(t *testing.T) {
	t.Parallel()
	vm := setupURLNamespace(t)
	_, err := vm.RunString(`shisho.url.encodeURIComponent()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "string argument is required")
}

func TestURLDecodeURIComponent_MissingArgument(t *testing.T) {
	t.Parallel()
	vm := setupURLNamespace(t)
	_, err := vm.RunString(`shisho.url.decodeURIComponent()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "string argument is required")
}

// TestURLRoundTrip verifies that encode/decode are inverse operations.
func TestURLRoundTrip(t *testing.T) {
	t.Parallel()
	vm := setupURLNamespace(t)

	script := `
		var original = "hello world & goodbye=world";
		var encoded = shisho.url.encodeURIComponent(original);
		var decoded = shisho.url.decodeURIComponent(encoded);
		decoded === original;
	`
	val, err := vm.RunString(script)
	require.NoError(t, err)
	assert.True(t, val.ToBoolean())
}

// TestURLSearchParamsWithParse verifies searchParams and parse work together.
func TestURLSearchParamsWithParse(t *testing.T) {
	t.Parallel()
	vm := setupURLNamespace(t)

	script := `
		var params = shisho.url.searchParams({query: "test", page: 1});
		var url = "https://api.example.com/search?" + params;
		var parsed = shisho.url.parse(url);
		JSON.stringify({
			fullUrl: url,
			query: parsed.query.query,
			page: parsed.query.page
		});
	`
	val, err := vm.RunString(script)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(val.String()), &result)
	require.NoError(t, err)

	assert.Equal(t, "https://api.example.com/search?page=1&query=test", result["fullUrl"])
	assert.Equal(t, "test", result["query"])
	assert.Equal(t, "1", result["page"])
}
