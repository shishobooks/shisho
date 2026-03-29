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

func TestHTMLParse_Basic(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var doc = shisho.html.parse('<div class="wrapper"><p>Hello</p></div>');
		doc !== null && doc !== undefined && doc.__node !== null;
	`)
	require.NoError(t, err)
	assert.True(t, val.ToBoolean())
}

func TestHTMLQuerySelector_Basic(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var doc = shisho.html.parse('<div class="wrapper"><p class="intro">Hello</p><p class="body">World</p></div>');
		var elem = shisho.html.querySelector(doc, "p.intro");
		JSON.stringify({tag: elem.tag, text: elem.text});
	`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"tag":"p","text":"Hello"}`, val.String())
}

func TestHTMLQuerySelector_MetaTag(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var doc = shisho.html.parse('<html><head><meta name="author" content="Jane Austen"><meta name="description" content="A novel"></head></html>');
		var elem = shisho.html.querySelector(doc, 'meta[name="author"]');
		elem.attributes.content;
	`)
	require.NoError(t, err)
	assert.Equal(t, "Jane Austen", val.String())
}

func TestHTMLQuerySelector_NoMatch(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var doc = shisho.html.parse('<div><p>Hello</p></div>');
		var result = shisho.html.querySelector(doc, "span.missing");
		result === null;
	`)
	require.NoError(t, err)
	assert.True(t, val.ToBoolean())
}

func TestHTMLQuerySelectorAll_MultipleMatches(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var doc = shisho.html.parse('<ul><li>One</li><li>Two</li><li>Three</li></ul>');
		var elems = shisho.html.querySelectorAll(doc, "li");
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
		var doc = shisho.html.parse('<html><head>' +
			'<script type="application/ld+json">{"@type":"Book","name":"Dune","author":"Frank Herbert"}</script>' +
			'</head><body><p>Content</p></body></html>');
		var elem = shisho.html.querySelector(doc, 'script[type="application/ld+json"]');
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
		var doc = shisho.html.parse('<div id="content"><strong>Bold</strong> and <em>italic</em></div>');
		var elem = shisho.html.querySelector(doc, "#content");
		elem.innerHTML;
	`)
	require.NoError(t, err)
	assert.Equal(t, "<strong>Bold</strong> and <em>italic</em>", val.String())
}

func TestHTMLQuerySelector_AttributeSelector(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var doc = shisho.html.parse('<html><head>' +
			'<meta property="og:title" content="My Book">' +
			'<meta property="og:author" content="Author Name">' +
			'</head></html>');
		var elem = shisho.html.querySelector(doc, 'meta[property="og:title"]');
		elem.attributes.content;
	`)
	require.NoError(t, err)
	assert.Equal(t, "My Book", val.String())
}

func TestHTMLQuerySelector_OnChildElement(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	// Query a parsed doc, get a child element, then query *that* element
	val, err := vm.RunString(`
		var doc = shisho.html.parse('<div><section><p class="a">First</p><p class="b">Second</p></section><p class="a">Third</p></div>');
		var section = shisho.html.querySelector(doc, "section");
		var p = shisho.html.querySelector(section, "p.b");
		p.text;
	`)
	require.NoError(t, err)
	assert.Equal(t, "Second", val.String())
}

func TestHTMLParse_InvalidDoc(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	// querySelector on a non-parsed object should panic with descriptive error
	_, err := vm.RunString(`
		shisho.html.querySelector({tag: "div"}, "p");
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "use shisho.html.parse() first")
}
