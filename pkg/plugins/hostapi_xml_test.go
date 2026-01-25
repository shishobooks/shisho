package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXML_Parse_CreatesDocument(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<root><child>text</child></root>');
		JSON.stringify({tag: doc.tag, namespace: doc.namespace});
	`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"tag":"root","namespace":""}`, val.String())
}

func TestXML_Parse_NestedElements(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<root><parent><child>inner</child></parent></root>');
		doc.children[0].children[0].tag;
	`)
	require.NoError(t, err)
	assert.Equal(t, "child", val.String())
}

func TestXML_Parse_TextContentConcatenation(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	// Text nodes around child elements get concatenated
	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<root>Hello <b>World</b> End</root>');
		doc.text;
	`)
	require.NoError(t, err)
	// The root's direct text content is "Hello " + " End" (text nodes not inside <b>)
	assert.Equal(t, "Hello  End", val.String())
}

func TestXML_Parse_Attributes(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<root id="123" class="main"></root>');
		JSON.stringify({id: doc.attributes.id, cls: doc.attributes["class"]});
	`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"123","cls":"main"}`, val.String())
}

func TestXML_Parse_NamespacedElements(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<package xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>My Book</dc:title></package>');
		var title = doc.children[0];
		JSON.stringify({tag: title.tag, ns: title.namespace, text: title.text});
	`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"tag":"title","ns":"http://purl.org/dc/elements/1.1/","text":"My Book"}`, val.String())
}

func TestXML_QuerySelector_FindsByTagName(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<root><a>first</a><b>second</b><a>third</a></root>');
		var result = shisho.xml.querySelector(doc, "b");
		result.text;
	`)
	require.NoError(t, err)
	assert.Equal(t, "second", val.String())
}

func TestXML_QuerySelector_FindsByNamespacePrefix(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<package xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Book Title</dc:title><dc:creator>Author</dc:creator></package>');
		var result = shisho.xml.querySelector(doc, "dc|creator", {"dc": "http://purl.org/dc/elements/1.1/"});
		result.text;
	`)
	require.NoError(t, err)
	assert.Equal(t, "Author", val.String())
}

func TestXML_QuerySelector_ReturnsNullForNoMatch(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<root><child>text</child></root>');
		var result = shisho.xml.querySelector(doc, "nonexistent");
		result === null;
	`)
	require.NoError(t, err)
	assert.True(t, val.ToBoolean())
}

func TestXML_QuerySelectorAll_ReturnsMultipleMatches(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<root><item>A</item><other>X</other><item>B</item><item>C</item></root>');
		var results = shisho.xml.querySelectorAll(doc, "item");
		var texts = [];
		for (var i = 0; i < results.length; i++) {
			texts.push(results[i].text);
		}
		JSON.stringify(texts);
	`)
	require.NoError(t, err)
	assert.Equal(t, `["A","B","C"]`, val.String())
}

func TestXML_QuerySelectorAll_ReturnsEmptyArrayForNoMatch(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<root><child>text</child></root>');
		var results = shisho.xml.querySelectorAll(doc, "nonexistent");
		results.length;
	`)
	require.NoError(t, err)
	assert.Equal(t, int64(0), val.ToInteger())
}

func TestXML_QuerySelector_NamespaceQualifiedMultipleNS(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var xmlContent = '<package xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">' +
			'<dc:title>My Book</dc:title>' +
			'<opf:meta>some meta</opf:meta>' +
			'<dc:creator>Author Name</dc:creator>' +
			'</package>';
		var doc = shisho.xml.parse(xmlContent);
		var namespaces = {
			"dc": "http://purl.org/dc/elements/1.1/",
			"opf": "http://www.idpf.org/2007/opf"
		};
		var dcTitle = shisho.xml.querySelector(doc, "dc|title", namespaces);
		var opfMeta = shisho.xml.querySelector(doc, "opf|meta", namespaces);
		JSON.stringify({title: dcTitle.text, meta: opfMeta.text});
	`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"title":"My Book","meta":"some meta"}`, val.String())
}

func TestXML_QuerySelector_UnqualifiedMatchesAnyNamespace(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<package xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Namespaced</dc:title></package>');
		// Unqualified "title" should match <dc:title> (matches by local name regardless of namespace)
		var result = shisho.xml.querySelector(doc, "title");
		result.text;
	`)
	require.NoError(t, err)
	assert.Equal(t, "Namespaced", val.String())
}

func TestXML_QuerySelectorAll_DeepNested(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<root><a><item>1</item><b><item>2</item></b></a><item>3</item></root>');
		var results = shisho.xml.querySelectorAll(doc, "item");
		var texts = [];
		for (var i = 0; i < results.length; i++) {
			texts.push(results[i].text);
		}
		JSON.stringify(texts);
	`)
	require.NoError(t, err)
	assert.Equal(t, `["1","2","3"]`, val.String())
}

func TestXML_QuerySelector_MatchesRoot(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<root><child>text</child></root>');
		var result = shisho.xml.querySelector(doc, "root");
		result.tag;
	`)
	require.NoError(t, err)
	assert.Equal(t, "root", val.String())
}

func TestXML_Parse_EmptyElement(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<root/>');
		JSON.stringify({tag: doc.tag, text: doc.text, children: doc.children.length});
	`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"tag":"root","text":"","children":0}`, val.String())
}

func TestXML_Parse_InvalidXML(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	_, err = rt.vm.RunString(`shisho.xml.parse("not xml at all < > &")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shisho.xml.parse")
}

func TestXML_Parse_NoArgument(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	_, err = rt.vm.RunString(`shisho.xml.parse()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content argument is required")
}

func TestXML_QuerySelector_NoArguments(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	_, err = rt.vm.RunString(`shisho.xml.querySelector()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "doc and selector arguments are required")
}

func TestXML_QuerySelectorAll_NoArguments(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	_, err = rt.vm.RunString(`shisho.xml.querySelectorAll()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "doc and selector arguments are required")
}

func TestXML_FunctionsExist(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	funcs := []string{"parse", "querySelector", "querySelectorAll"}
	for _, fn := range funcs {
		val, err := rt.vm.RunString(`typeof shisho.xml.` + fn)
		require.NoError(t, err, "checking typeof shisho.xml.%s", fn)
		assert.Equal(t, "function", val.String(), "shisho.xml.%s should be a function", fn)
	}
}

func TestXML_QuerySelector_NamespaceMismatch(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	// Looking for dc|title with wrong namespace URI should not match
	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<package xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Book</dc:title></package>');
		var result = shisho.xml.querySelector(doc, "dc|title", {"dc": "http://wrong.namespace/"});
		result === null;
	`)
	require.NoError(t, err)
	assert.True(t, val.ToBoolean())
}

func TestXML_Parse_MultipleChildren(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<root><a/><b/><c/></root>');
		var tags = [];
		for (var i = 0; i < doc.children.length; i++) {
			tags.push(doc.children[i].tag);
		}
		JSON.stringify(tags);
	`)
	require.NoError(t, err)
	assert.Equal(t, `["a","b","c"]`, val.String())
}

func TestXML_Parse_ChildTextContent(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`
		var doc = shisho.xml.parse('<root><b>World</b></root>');
		doc.children[0].text;
	`)
	require.NoError(t, err)
	assert.Equal(t, "World", val.String())
}
