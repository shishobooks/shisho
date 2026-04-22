package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestYAML_Parse_Scalars(t *testing.T) {
	t.Parallel()
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	require.NoError(t, InjectHostAPIs(rt, cfg))

	val, err := rt.vm.RunString(`
		JSON.stringify({
			str: shisho.yaml.parse("hello"),
			num: shisho.yaml.parse("42"),
			bool: shisho.yaml.parse("true"),
		});
	`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"str":"hello","num":42,"bool":true}`, val.String())
}

func TestYAML_Parse_Mapping(t *testing.T) {
	t.Parallel()
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	require.NoError(t, InjectHostAPIs(rt, cfg))

	val, err := rt.vm.RunString(`
		var doc = shisho.yaml.parse("title: My Book\nauthor: Alice\npages: 100");
		JSON.stringify({title: doc.title, author: doc.author, pages: doc.pages});
	`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"title":"My Book","author":"Alice","pages":100}`, val.String())
}

func TestYAML_Parse_NestedAndSequence(t *testing.T) {
	t.Parallel()
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	require.NoError(t, InjectHostAPIs(rt, cfg))

	val, err := rt.vm.RunString(`
		var doc = shisho.yaml.parse(
			"book:\n" +
			"  title: My Book\n" +
			"  authors:\n" +
			"    - Alice\n" +
			"    - Bob\n"
		);
		JSON.stringify({
			title: doc.book.title,
			authors: doc.book.authors,
			count: doc.book.authors.length,
		});
	`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"title":"My Book","authors":["Alice","Bob"],"count":2}`, val.String())
}

func TestYAML_Parse_Invalid(t *testing.T) {
	t.Parallel()
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	require.NoError(t, InjectHostAPIs(rt, cfg))

	_, err := rt.vm.RunString(`shisho.yaml.parse("title: My Book\n  unexpected: indent");`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shisho.yaml.parse")
}

func TestYAML_Parse_MissingArg(t *testing.T) {
	t.Parallel()
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	require.NoError(t, InjectHostAPIs(rt, cfg))

	_, err := rt.vm.RunString(`shisho.yaml.parse();`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content argument is required")
}

func TestYAML_Stringify_Object(t *testing.T) {
	t.Parallel()
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	require.NoError(t, InjectHostAPIs(rt, cfg))

	val, err := rt.vm.RunString(`shisho.yaml.stringify({title: "My Book", pages: 100});`)
	require.NoError(t, err)
	// Round-trip via parse to avoid asserting against yaml.v3's exact formatting.
	roundtrip, err := rt.vm.RunString(`
		var s = shisho.yaml.stringify({title: "My Book", pages: 100});
		var back = shisho.yaml.parse(s);
		JSON.stringify({title: back.title, pages: back.pages});
	`)
	require.NoError(t, err)
	assert.NotEmpty(t, val.String())
	assert.JSONEq(t, `{"title":"My Book","pages":100}`, roundtrip.String())
}

func TestYAML_Stringify_MissingArg(t *testing.T) {
	t.Parallel()
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	require.NoError(t, InjectHostAPIs(rt, cfg))

	_, err := rt.vm.RunString(`shisho.yaml.stringify();`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "value argument is required")
}
