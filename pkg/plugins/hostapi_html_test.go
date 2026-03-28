package plugins

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupHTMLTestVM creates a goja VM with shisho.html injected for testing.
func setupHTMLTestVM(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)
	return rt.vm
}

func TestHTMLQuerySelector_Basic(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var html = '<div class="wrapper"><p class="intro">Hello</p><p class="body">World</p></div>';
		var elem = shisho.html.querySelector(html, "p.intro");
		JSON.stringify({tag: elem.tag, text: elem.text});
	`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"tag":"p","text":"Hello"}`, val.String())
}

func TestHTMLQuerySelector_MetaTag(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var html = '<html><head><meta name="author" content="Jane Austen"><meta name="description" content="A novel"></head></html>';
		var elem = shisho.html.querySelector(html, 'meta[name="author"]');
		elem.attributes.content;
	`)
	require.NoError(t, err)
	assert.Equal(t, "Jane Austen", val.String())
}

func TestHTMLQuerySelector_NoMatch(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var html = '<div><p>Hello</p></div>';
		var result = shisho.html.querySelector(html, "span.missing");
		result === null;
	`)
	require.NoError(t, err)
	assert.True(t, val.ToBoolean())
}

func TestHTMLQuerySelectorAll_MultipleMatches(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var html = '<ul><li>One</li><li>Two</li><li>Three</li></ul>';
		var elems = shisho.html.querySelectorAll(html, "li");
		var texts = [];
		for (var i = 0; i < elems.length; i++) {
			texts.push(elems[i].text);
		}
		JSON.stringify(texts);
	`)
	require.NoError(t, err)
	assert.Equal(t, `["One","Two","Three"]`, val.String())
}

func TestHTMLQuerySelector_ScriptContent(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var html = '<html><head>' +
			'<script type="application/ld+json">{"@type":"Book","name":"Dune","author":"Frank Herbert"}</script>' +
			'</head><body><p>Content</p></body></html>';
		var elem = shisho.html.querySelector(html, 'script[type="application/ld+json"]');
		var data = JSON.parse(elem.text);
		JSON.stringify({type: data["@type"], name: data.name, author: data.author});
	`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"Book","name":"Dune","author":"Frank Herbert"}`, val.String())
}

func TestHTMLQuerySelector_InnerHTML(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var html = '<div id="content"><strong>Bold</strong> and <em>italic</em></div>';
		var elem = shisho.html.querySelector(html, "#content");
		elem.innerHTML;
	`)
	require.NoError(t, err)
	assert.Equal(t, "<strong>Bold</strong> and <em>italic</em>", val.String())
}

func TestHTMLQuerySelector_AttributeSelector(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var html = '<html><head>' +
			'<meta property="og:title" content="My Book">' +
			'<meta property="og:author" content="Author Name">' +
			'</head></html>';
		var elem = shisho.html.querySelector(html, 'meta[property="og:title"]');
		elem.attributes.content;
	`)
	require.NoError(t, err)
	assert.Equal(t, "My Book", val.String())
}
