# Plugin Refinement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Address four plugin development friction points: type unification, HTML parsing API, cover docs, and test utilities with package rename.

**Architecture:** Collapse `SearchResult` into `ParsedMetadata` throughout the Go backend and TypeScript SDK, add a new `shisho.html` host API backed by `golang.org/x/net/html` + `cascadia`, rename `@shisho/plugin-types` to `@shisho/plugin-sdk` and add a `testing` subpath export with mock factories.

**Tech Stack:** Go (goja, cascadia, x/net/html), TypeScript, pnpm

**Spec:** `docs/superpowers/specs/2026-03-28-plugin-refinement-design.md`

---

### Task 1: Collapse SearchResult into ParsedMetadata (Go)

**Files:**
- Modify: `pkg/plugins/hooks.go:130-191` (delete SearchResult, SearchResultToMetadata, rewrite parseSearchResponse)
- Modify: `pkg/plugins/handler.go:1266,1313,1344-1347,1350` (add EnrichSearchResult, update searchMetadata handler)
- Modify: `pkg/worker/scan_unified.go:2585,2657` (remove SearchResultToMetadata call)

- [ ] **Step 1: Write failing test for parseSearchResponse returning ParsedMetadata**

In `pkg/plugins/hooks_search_result_test.go`, replace all existing tests with one that validates `parseSearchResponse` returns `[]mediafile.ParsedMetadata` instead of `[]SearchResult`. The existing `SearchResultToMetadata` tests are deleted because the function no longer exists.

Replace the entire file contents of `pkg/plugins/hooks_search_result_test.go` with:

```go
package plugins

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSearchResponse_AllFields(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	// Build a JS search response with all fields populated
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

	require.Len(t, md.Authors, 2)
	assert.Equal(t, "Author One", md.Authors[0].Name)
	assert.Equal(t, "writer", md.Authors[0].Role)

	assert.Equal(t, []string{"Narrator A", "Narrator B"}, md.Narrators)
	assert.Equal(t, []string{"Fantasy", "Adventure"}, md.Genres)
	assert.Equal(t, []string{"epic", "magic"}, md.Tags)

	require.Len(t, md.Identifiers, 2)
	assert.Equal(t, "isbn_13", md.Identifiers[0].Type)

	// Server-added fields should be set
	assert.Equal(t, "test-scope", md.PluginScope)
	assert.Equal(t, "test-plugin", md.PluginID)
}

func TestParseSearchResponse_DateParsing_RFC3339(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	val, err := vm.RunString(`({ results: [{ title: "T", releaseDate: "2025-01-10T00:00:00Z" }] })`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "", "")
	require.Len(t, resp.Results, 1)
	require.NotNil(t, resp.Results[0].ReleaseDate)
	assert.Equal(t, 2025, resp.Results[0].ReleaseDate.Year())
}

func TestParseSearchResponse_DateParsing_Invalid(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	val, err := vm.RunString(`({ results: [{ title: "T", releaseDate: "not-a-date" }] })`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "", "")
	require.Len(t, resp.Results, 1)
	assert.Nil(t, resp.Results[0].ReleaseDate)
}

func TestParseSearchResponse_EmptyResults(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	val, err := vm.RunString(`({ results: [] })`)
	require.NoError(t, err)

	resp := parseSearchResponse(vm, val, "", "")
	assert.Empty(t, resp.Results)
}

func TestParseSearchResponse_NilInput(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	resp := parseSearchResponse(vm, nil, "", "")
	assert.Empty(t, resp.Results)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go test ./pkg/plugins/ -run TestParseSearchResponse -v 2>&1 | head -30`

Expected: Compilation errors — `parseSearchResponse` returns `*SearchResponse` with `[]SearchResult`, not `[]ParsedMetadata`. Fields like `md.PluginScope`, `md.ReleaseDate` don't exist on `SearchResult`.

- [ ] **Step 3: Implement type unification in hooks.go**

In `pkg/plugins/hooks.go`:

**Delete** the `SearchResult` struct (lines 130-152), `SearchResultToMetadata` function (lines 159-191), and update `SearchResponse` and `parseSearchResponse`.

Replace lines 130-191 (from `// SearchResult is a single` through the end of `SearchResultToMetadata`) with:

```go
// SearchResponse is the result of a metadata enricher's search() hook.
// Results are ParsedMetadata directly — no intermediate SearchResult type.
// Server-added fields (PluginScope, PluginID) are set by parseSearchResponse.
type SearchResponse struct {
	Results []mediafile.ParsedMetadata
}
```

Then rewrite `parseSearchResponse` (lines 345-433). Replace from `// parseSearchResponse maps` through the closing brace of the function with:

```go
// parseSearchResponse maps a JS search result to SearchResponse.
// Each result is parsed directly into ParsedMetadata. The releaseDate field
// is parsed from "2006-01-02" or RFC3339 format strings into *time.Time.
// PluginScope and PluginID are set on each result for server-side tracking.
func parseSearchResponse(vm *goja.Runtime, val goja.Value, pluginScope, pluginID string) *SearchResponse {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return &SearchResponse{}
	}

	obj := val.ToObject(vm)
	resultsVal := obj.Get("results")
	if resultsVal == nil || goja.IsUndefined(resultsVal) || goja.IsNull(resultsVal) {
		return &SearchResponse{}
	}

	resultsObj := resultsVal.ToObject(vm)
	lengthVal := resultsObj.Get("length")
	if lengthVal == nil || goja.IsUndefined(lengthVal) {
		return &SearchResponse{}
	}
	length := int(lengthVal.ToInteger())

	results := make([]mediafile.ParsedMetadata, 0, length)
	for i := 0; i < length; i++ {
		itemVal := resultsObj.Get(intToString(i))
		if itemVal == nil || goja.IsUndefined(itemVal) || goja.IsNull(itemVal) {
			continue
		}
		itemObj := itemVal.ToObject(vm)

		md := mediafile.ParsedMetadata{
			Title:       getStringField(itemObj, "title"),
			Description: getStringField(itemObj, "description"),
			Publisher:   getStringField(itemObj, "publisher"),
			Subtitle:    getStringField(itemObj, "subtitle"),
			Series:      getStringField(itemObj, "series"),
			Imprint:     getStringField(itemObj, "imprint"),
			URL:         getStringField(itemObj, "url"),
			CoverURL:    getStringField(itemObj, "coverUrl"),
			PluginScope: pluginScope,
			PluginID:    pluginID,
		}

		// seriesNumber -> *float64
		seriesNumVal := itemObj.Get("seriesNumber")
		if seriesNumVal != nil && !goja.IsUndefined(seriesNumVal) && !goja.IsNull(seriesNumVal) {
			f := seriesNumVal.ToFloat()
			md.SeriesNumber = &f
		}

		// releaseDate -> *time.Time (accepts "2006-01-02" or RFC3339)
		releaseDateStr := getStringField(itemObj, "releaseDate")
		if releaseDateStr != "" {
			t, err := time.Parse("2006-01-02", releaseDateStr)
			if err != nil {
				t, err = time.Parse(time.RFC3339, releaseDateStr)
			}
			if err == nil {
				md.ReleaseDate = &t
			}
		}

		// genres -> []string
		genresVal := itemObj.Get("genres")
		if genresVal != nil && !goja.IsUndefined(genresVal) && !goja.IsNull(genresVal) {
			md.Genres = parseStringArray(vm, genresVal)
		}

		// tags -> []string
		tagsVal := itemObj.Get("tags")
		if tagsVal != nil && !goja.IsUndefined(tagsVal) && !goja.IsNull(tagsVal) {
			md.Tags = parseStringArray(vm, tagsVal)
		}

		// narrators -> []string
		narratorsVal := itemObj.Get("narrators")
		if narratorsVal != nil && !goja.IsUndefined(narratorsVal) && !goja.IsNull(narratorsVal) {
			md.Narrators = parseStringArray(vm, narratorsVal)
		}

		// authors -> []ParsedAuthor
		authorsVal := itemObj.Get("authors")
		if authorsVal != nil && !goja.IsUndefined(authorsVal) && !goja.IsNull(authorsVal) {
			md.Authors = parseAuthors(vm, authorsVal)
		}

		// identifiers -> []ParsedIdentifier
		identifiersVal := itemObj.Get("identifiers")
		if identifiersVal != nil && !goja.IsUndefined(identifiersVal) && !goja.IsNull(identifiersVal) {
			md.Identifiers = parseIdentifiers(vm, identifiersVal)
		}

		results = append(results, md)
	}

	return &SearchResponse{Results: results}
}
```

This requires `ParsedMetadata` to have `PluginScope` and `PluginID` fields. Add them to `pkg/mediafile/mediafile.go` in the `ParsedMetadata` struct, after the existing `FieldDataSources` field:

```go
	PluginScope      string            `json:"-"`
	PluginID         string            `json:"-"`
```

These use `json:"-"` because they're server-side tracking fields, never serialized to plugins.

- [ ] **Step 4: Update handler.go searchMetadata to use EnrichSearchResult**

In `pkg/plugins/handler.go`, add the `EnrichSearchResult` type near the other search-related types (before `searchMetadata`, around line 1245):

```go
// EnrichSearchResult wraps ParsedMetadata with server-added fields for the
// search results HTTP response. These fields are never set by plugins.
type EnrichSearchResult struct {
	mediafile.ParsedMetadata
	PluginScope    string   `json:"plugin_scope"`
	PluginID       string   `json:"plugin_id"`
	DisabledFields []string `json:"disabled_fields,omitempty"`
}
```

Then update the `searchMetadata` handler:

Replace `[]SearchResult{}` on line 1266 with `[]EnrichSearchResult{}`.

Replace `var allResults []SearchResult` on line 1313 with `var allResults []EnrichSearchResult`.

Replace the loop body (lines 1344-1347) that sets DisabledFields and appends:

```go
		for _, md := range resp.Results {
			allResults = append(allResults, EnrichSearchResult{
				ParsedMetadata: md,
				PluginScope:    md.PluginScope,
				PluginID:       md.PluginID,
				DisabledFields: disabledFields,
			})
		}
```

- [ ] **Step 5: Update scan_unified.go to remove SearchResultToMetadata**

In `pkg/worker/scan_unified.go`, line 2657 currently reads:

```go
		searchMeta := plugins.SearchResultToMetadata(&firstResult)
```

Replace with:

```go
		searchMeta := &firstResult
```

Since `searchResp.Results` is now `[]mediafile.ParsedMetadata`, `firstResult` is already a `ParsedMetadata`. Taking its address gives us `*mediafile.ParsedMetadata`.

Also update the comment at line 2585 from:
```
// result is converted directly to ParsedMetadata via SearchResultToMetadata.
```
to:
```
// result is used directly as ParsedMetadata (no conversion needed).
```

- [ ] **Step 6: Update hooks_test.go TestSearchResultCarriesAllMetadata**

In `pkg/plugins/hooks_test.go`, the test at line 548 (`TestSearchResultCarriesAllMetadata`) accesses `sr.Title`, `sr.CoverURL`, etc. on a `SearchResult`. Now these are `mediafile.ParsedMetadata` fields.

Replace lines 626-638 (the assertions after `searchResp`) with:

```go
	searchResp, err := manager.RunMetadataSearch(ctx, rt, searchCtx)
	require.NoError(t, err)
	require.Len(t, searchResp.Results, 1)

	md := searchResp.Results[0]
	assert.Equal(t, "Full Title", md.Title)
	assert.Equal(t, "Full description", md.Description)
	assert.Equal(t, []string{"SciFi"}, md.Genres)
	assert.Equal(t, "https://example.com/cover.jpg", md.CoverURL)
	require.Len(t, md.Authors, 1)
	assert.Equal(t, "Full Author", md.Authors[0].Name)
	assert.Equal(t, "writer", md.Authors[0].Role)
	assert.Equal(t, "Full Imprint", md.Imprint)
	assert.Equal(t, "https://example.com/book", md.URL)
```

- [ ] **Step 7: Run all tests to verify**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go test ./pkg/plugins/ -v -count=1 2>&1 | tail -20`

Then: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go test ./pkg/worker/ -run TestScan -v -count=1 2>&1 | tail -20`

Expected: All tests pass. No references to `SearchResult` struct remain (except `EnrichSearchResult` and the unrelated `pkg/search/` types).

- [ ] **Step 8: Commit**

```bash
git add pkg/plugins/hooks.go pkg/plugins/hooks_search_result_test.go pkg/plugins/hooks_test.go pkg/plugins/handler.go pkg/worker/scan_unified.go pkg/mediafile/mediafile.go
git commit -m "[Backend] Collapse SearchResult into ParsedMetadata

Eliminate the duplicate SearchResult type. Search hooks now return
ParsedMetadata directly. Server-added fields (PluginScope, PluginID,
DisabledFields) live on EnrichSearchResult, used only in the HTTP
response layer. Removes SearchResultToMetadata conversion function."
```

---

### Task 2: Add HTML Parsing Host API (Go)

**Files:**
- Create: `pkg/plugins/hostapi_html.go`
- Create: `pkg/plugins/hostapi_html_test.go`
- Modify: `pkg/plugins/hostapi.go:64-67` (register html namespace)
- Modify: `go.mod` (add cascadia dependency)

- [ ] **Step 1: Add cascadia dependency**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go get github.com/andybalholm/cascadia`

- [ ] **Step 2: Write failing test for shisho.html.querySelector**

Create `pkg/plugins/hostapi_html_test.go`:

```go
package plugins

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupHTMLTestVM(t *testing.T) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	shishoObj := vm.NewObject()
	err := vm.Set("shisho", shishoObj)
	require.NoError(t, err)
	err = injectHTMLNamespace(vm, shishoObj)
	require.NoError(t, err)
	return vm
}

func TestHTMLQuerySelector_Basic(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var result = shisho.html.querySelector('<div><p class="intro">Hello</p><p>World</p></div>', 'p.intro');
		result ? { tag: result.tag, text: result.text, cls: result.attributes["class"] } : null;
	`)
	require.NoError(t, err)

	obj := val.ToObject(vm)
	assert.Equal(t, "p", obj.Get("tag").String())
	assert.Equal(t, "Hello", obj.Get("text").String())
	assert.Equal(t, "intro", obj.Get("cls").String())
}

func TestHTMLQuerySelector_MetaTag(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var html = '<html><head><meta name="description" content="A great book"></head><body></body></html>';
		var meta = shisho.html.querySelector(html, 'meta[name="description"]');
		meta ? meta.attributes.content : null;
	`)
	require.NoError(t, err)
	assert.Equal(t, "A great book", val.String())
}

func TestHTMLQuerySelector_NoMatch(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		shisho.html.querySelector('<div>Hello</div>', 'span');
	`)
	require.NoError(t, err)
	assert.True(t, goja.IsNull(val))
}

func TestHTMLQuerySelectorAll_MultipleMatches(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var results = shisho.html.querySelectorAll('<ul><li>A</li><li>B</li><li>C</li></ul>', 'li');
		results.map(function(el) { return el.text; });
	`)
	require.NoError(t, err)

	obj := val.ToObject(vm)
	assert.Equal(t, int64(3), obj.Get("length").ToInteger())
	assert.Equal(t, "A", obj.Get("0").ToObject(vm).Get("0").String()) // goja array
}

func TestHTMLQuerySelector_ScriptContent(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	// The main use case: extracting JSON-LD from script tags
	val, err := vm.RunString(`
		var html = '<html><head><script type="application/ld+json">{"@type":"Book","name":"Test"}</script></head></html>';
		var script = shisho.html.querySelector(html, 'script[type="application/ld+json"]');
		script ? JSON.parse(script.text).name : null;
	`)
	require.NoError(t, err)
	assert.Equal(t, "Test", val.String())
}

func TestHTMLQuerySelector_InnerHTML(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var result = shisho.html.querySelector('<div><b>Bold</b> text</div>', 'div');
		result ? result.innerHTML : null;
	`)
	require.NoError(t, err)
	assert.Equal(t, "<b>Bold</b> text", val.String())
}

func TestHTMLQuerySelector_AttributeSelector(t *testing.T) {
	t.Parallel()
	vm := setupHTMLTestVM(t)

	val, err := vm.RunString(`
		var html = '<meta property="og:title" content="OG Title"><meta property="og:image" content="img.jpg">';
		var el = shisho.html.querySelector(html, 'meta[property="og:title"]');
		el ? el.attributes.content : null;
	`)
	require.NoError(t, err)
	assert.Equal(t, "OG Title", val.String())
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go test ./pkg/plugins/ -run TestHTML -v 2>&1 | head -10`

Expected: Compilation error — `injectHTMLNamespace` is undefined.

- [ ] **Step 4: Implement hostapi_html.go**

Create `pkg/plugins/hostapi_html.go`:

```go
package plugins

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/andybalholm/cascadia"
	"github.com/dop251/goja"
	"golang.org/x/net/html"
)

// htmlElement represents a parsed HTML element.
type htmlElement struct {
	Tag        string
	Attributes map[string]string
	Text       string
	InnerHTML  string
	Children   []*htmlElement
}

// injectHTMLNamespace sets up shisho.html with querySelector and querySelectorAll.
// Uses golang.org/x/net/html for parsing and cascadia for CSS selectors.
func injectHTMLNamespace(vm *goja.Runtime, shishoObj *goja.Object) error {
	htmlObj := vm.NewObject()
	if err := shishoObj.Set("html", htmlObj); err != nil {
		return fmt.Errorf("failed to set shisho.html: %w", err)
	}

	htmlObj.Set("querySelector", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("shisho.html.querySelector: html and selector arguments are required"))
		}
		content := call.Argument(0).String()
		selector := call.Argument(1).String()

		sel, err := cascadia.Parse(selector)
		if err != nil {
			panic(vm.ToValue("shisho.html.querySelector: invalid selector: " + err.Error()))
		}

		doc, err := html.Parse(strings.NewReader(content))
		if err != nil {
			panic(vm.ToValue("shisho.html.querySelector: " + err.Error()))
		}

		match := cascadia.Query(doc, sel)
		if match == nil {
			return goja.Null()
		}

		elem := nodeToHTMLElement(match)
		return htmlElementToGojaValue(vm, elem)
	})

	htmlObj.Set("querySelectorAll", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("shisho.html.querySelectorAll: html and selector arguments are required"))
		}
		content := call.Argument(0).String()
		selector := call.Argument(1).String()

		sel, err := cascadia.Parse(selector)
		if err != nil {
			panic(vm.ToValue("shisho.html.querySelectorAll: invalid selector: " + err.Error()))
		}

		doc, err := html.Parse(strings.NewReader(content))
		if err != nil {
			panic(vm.ToValue("shisho.html.querySelectorAll: " + err.Error()))
		}

		matches := cascadia.QueryAll(doc, sel)
		result := make([]interface{}, len(matches))
		for i, m := range matches {
			elem := nodeToHTMLElement(m)
			result[i] = htmlElementToGojaValue(vm, elem)
		}
		return vm.ToValue(result)
	})

	return nil
}

// nodeToHTMLElement converts an html.Node to our htmlElement struct.
func nodeToHTMLElement(n *html.Node) *htmlElement {
	elem := &htmlElement{
		Attributes: make(map[string]string),
		Children:   make([]*htmlElement, 0),
	}

	if n.Type == html.ElementNode {
		elem.Tag = n.Data
		for _, attr := range n.Attr {
			elem.Attributes[attr.Key] = attr.Val
		}
	}

	elem.Text = extractText(n)
	elem.InnerHTML = renderInnerHTML(n)

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			elem.Children = append(elem.Children, nodeToHTMLElement(child))
		}
	}

	return elem
}

// extractText recursively extracts all text content from a node.
func extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		sb.WriteString(extractText(child))
	}
	return sb.String()
}

// renderInnerHTML renders the inner HTML of a node as a string.
func renderInnerHTML(n *html.Node) string {
	var buf bytes.Buffer
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		html.Render(&buf, child) //nolint:errcheck
	}
	return buf.String()
}

// htmlElementToGojaValue converts an htmlElement to a goja object.
func htmlElementToGojaValue(vm *goja.Runtime, elem *htmlElement) goja.Value {
	obj := vm.NewObject()
	obj.Set("tag", elem.Tag)           //nolint:errcheck
	obj.Set("text", elem.Text)         //nolint:errcheck
	obj.Set("innerHTML", elem.InnerHTML) //nolint:errcheck

	attrs := vm.NewObject()
	for k, v := range elem.Attributes {
		attrs.Set(k, v) //nolint:errcheck
	}
	obj.Set("attributes", attrs) //nolint:errcheck

	children := make([]interface{}, len(elem.Children))
	for i, child := range elem.Children {
		children[i] = htmlElementToGojaValue(vm, child)
	}
	obj.Set("children", vm.ToValue(children)) //nolint:errcheck

	return obj
}
```

- [ ] **Step 5: Register HTML namespace in hostapi.go**

In `pkg/plugins/hostapi.go`, after the XML namespace registration (line 67), add:

```go
	// Set up html namespace
	if err := injectHTMLNamespace(vm, shishoObj); err != nil {
		return err
	}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go test ./pkg/plugins/ -run TestHTML -v -count=1`

Expected: All HTML tests pass.

Then run the full plugin test suite to ensure nothing broke:

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go test ./pkg/plugins/ -v -count=1 2>&1 | tail -20`

- [ ] **Step 7: Commit**

```bash
git add pkg/plugins/hostapi_html.go pkg/plugins/hostapi_html_test.go pkg/plugins/hostapi.go go.mod go.sum
git commit -m "[Backend] Add shisho.html host API with CSS selector support

New host API for HTML parsing using golang.org/x/net/html and cascadia.
Provides querySelector and querySelectorAll with full CSS selector
support, returning elements with tag, attributes, text, innerHTML,
and children. Enables plugins to replace fragile regex scraping."
```

---

### Task 3: Update TypeScript SDK — Type Unification + HTML API

**Files:**
- Modify: `packages/plugin-types/hooks.d.ts` (remove SearchResult, update SearchResponse)
- Modify: `packages/plugin-types/host-api.d.ts` (add HtmlElement, ShishoHTML, update ShishoHostAPI)
- Modify: `packages/plugin-types/metadata.d.ts` (add JSDoc to coverUrl/coverData)

- [ ] **Step 1: Remove SearchResult from hooks.d.ts**

In `packages/plugin-types/hooks.d.ts`, replace the `SearchResult` interface and `SearchResponse` interface (lines 51-74) with:

```typescript
/** Result returned from metadataEnricher.search(). */
export interface SearchResponse {
  results: ParsedMetadata[];
}
```

The `MetadataEnricherHook` (line 129-132) already returns `SearchResponse`, so it now returns `{ results: ParsedMetadata[] }` automatically.

- [ ] **Step 2: Add HTML types to host-api.d.ts**

In `packages/plugin-types/host-api.d.ts`, add after the `ShishoXML` interface (after line 189):

```typescript
/** A parsed HTML element returned by shisho.html queries. */
export interface HtmlElement {
  /** Element tag name (e.g., "div", "meta", "script"). */
  tag: string;
  /** Element attributes as key-value pairs. */
  attributes: Record<string, string>;
  /** Recursive inner text content (all text nodes concatenated). */
  text: string;
  /** Raw inner HTML string. Useful for script tags (JSON-LD) or rich content. */
  innerHTML: string;
  /** Direct child elements. */
  children: HtmlElement[];
}

/** HTML parsing with CSS selector support. */
export interface ShishoHTML {
  /**
   * Find the first element matching a CSS selector.
   * Supports full CSS selector syntax (attribute selectors, combinators, pseudo-classes).
   *
   * @example
   * var meta = shisho.html.querySelector(html, 'meta[name="description"]');
   * var description = meta ? meta.attributes.content : "";
   */
  querySelector(html: string, selector: string): HtmlElement | null;

  /**
   * Find all elements matching a CSS selector.
   *
   * @example
   * var scripts = shisho.html.querySelectorAll(html, 'script[type="application/ld+json"]');
   * var jsonLd = JSON.parse(scripts[0].text);
   */
  querySelectorAll(html: string, selector: string): HtmlElement[];
}
```

Then add `html: ShishoHTML;` to the `ShishoHostAPI` interface (around line 354, after the `xml` line):

```typescript
  html: ShishoHTML;
```

- [ ] **Step 3: Add JSDoc to cover fields in metadata.d.ts**

In `packages/plugin-types/metadata.d.ts`, update the `coverUrl` and `coverData` field comments:

For `coverUrl`:
```typescript
  /**
   * URL to download the cover image from. The server handles downloading and validates
   * the domain against the plugin's httpAccess.domains. This is the recommended way for
   * enricher plugins to provide covers.
   */
  coverUrl?: string;
```

For `coverData`:
```typescript
  /**
   * Raw cover image data as an ArrayBuffer. Use this for file parsers that extract
   * embedded covers, or enrichers that generate/composite images. If both coverData
   * and coverUrl are set, coverData takes precedence (no download occurs).
   */
  coverData?: ArrayBuffer;
```

- [ ] **Step 4: Verify types are consistent**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && cat packages/plugin-types/hooks.d.ts` to confirm SearchResult is removed and SearchResponse uses ParsedMetadata.

- [ ] **Step 5: Commit**

```bash
git add packages/plugin-types/hooks.d.ts packages/plugin-types/host-api.d.ts packages/plugin-types/metadata.d.ts
git commit -m "[Feature] Update plugin SDK types: unify SearchResult, add HTML API

Remove SearchResult interface — search hooks now return ParsedMetadata
directly. Add ShishoHTML and HtmlElement interfaces for the new
shisho.html host API. Add JSDoc to cover fields clarifying coverUrl
vs coverData usage."
```

---

### Task 4: Rename Package to @shisho/plugin-sdk

**Files:**
- Rename: `packages/plugin-types/` → `packages/plugin-sdk/`
- Modify: `packages/plugin-sdk/package.json` (rename, add exports)
- Modify: `packages/plugin-sdk/README.md` (update name)
- Modify: `scripts/release.sh:163,206-209,213` (update path)
- Modify: `.github/workflows/release.yml:165,169` (update path)
- Modify: `.goreleaser.yaml:47` (update npm install reference)

- [ ] **Step 1: Rename the directory**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && git mv packages/plugin-types packages/plugin-sdk`

- [ ] **Step 2: Update package.json**

In `packages/plugin-sdk/package.json`, update the name, homepage, directory, and add exports:

```json
{
  "name": "@shisho/plugin-sdk",
  "version": "0.0.21",
  "description": "TypeScript SDK for Shisho plugin development — types, host API declarations, and test utilities",
  "homepage": "https://github.com/shishobooks/shisho/blob/master/packages/plugin-sdk/README.md",
  "types": "index.d.ts",
  "exports": {
    ".": "./index.d.ts"
  },
  "files": [
    "*.d.ts"
  ],
  "license": "MIT",
  "repository": {
    "type": "git",
    "url": "https://github.com/shishobooks/shisho.git",
    "directory": "packages/plugin-sdk"
  }
}
```

Note: The `"./testing"` export will be added in Task 5 when we create the testing subpath.

- [ ] **Step 3: Update README.md**

In `packages/plugin-sdk/README.md`, replace all occurrences of `@shisho/plugin-types` with `@shisho/plugin-sdk` and `plugin-types` directory references with `plugin-sdk`.

- [ ] **Step 4: Update release.sh**

In `scripts/release.sh`:

Replace line 163:
```bash
    echo "  - packages/plugin-types/package.json -> $VERSION"
```
with:
```bash
    echo "  - packages/plugin-sdk/package.json -> $VERSION"
```

Replace lines 206-207:
```bash
echo "Updating packages/plugin-types/package.json..."
cd packages/plugin-types
```
with:
```bash
echo "Updating packages/plugin-sdk/package.json..."
cd packages/plugin-sdk
```

Replace line 213 (the git add):
```bash
git add CHANGELOG.md package.json packages/plugin-types/package.json
```
with:
```bash
git add CHANGELOG.md package.json packages/plugin-sdk/package.json
```

- [ ] **Step 5: Update release.yml**

In `.github/workflows/release.yml`, replace both occurrences of `./packages/plugin-types` (lines 165, 169) with `./packages/plugin-sdk`.

- [ ] **Step 6: Update .goreleaser.yaml**

In `.goreleaser.yaml`, replace `@shisho/plugin-types` (line 47) with `@shisho/plugin-sdk`.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "[Feature] Rename @shisho/plugin-types to @shisho/plugin-sdk

Rename package directory, update package.json name and exports,
update release script, CI workflow, and goreleaser references."
```

---

### Task 5: Add Test Utilities — @shisho/plugin-sdk/testing

**Files:**
- Create: `packages/plugin-sdk/testing/index.ts`
- Create: `packages/plugin-sdk/testing/tsconfig.json`
- Modify: `packages/plugin-sdk/package.json` (add testing export, files, build script, dependencies)

- [ ] **Step 1: Create the testing module**

Create `packages/plugin-sdk/testing/index.ts`:

```typescript
/**
 * @shisho/plugin-sdk/testing — mock factory for plugin test environments.
 *
 * Usage:
 *   import { createMockShisho } from "@shisho/plugin-sdk/testing";
 *   globalThis.shisho = createMockShisho({ fetch: { ... }, config: { ... } });
 */

export interface MockFetchResponse {
  status?: number;
  body?: string;
  headers?: Record<string, string>;
}

export interface MockShishoOptions {
  /** Route-based fetch mock. Keys are URLs, values are responses. Throws on unmatched URLs. */
  fetch?: Record<string, MockFetchResponse>;
  /** Config key-value pairs returned by shisho.config.get/getAll. */
  config?: Record<string, string>;
  /** Path-based filesystem mock. String values are text content, Buffer values are binary. Directory listings are string arrays. */
  fs?: Record<string, string | Buffer | string[]>;
}

function createMockFetch(routes: Record<string, MockFetchResponse>) {
  return function fetch(url: string, _options?: Record<string, unknown>) {
    const route = routes[url];
    if (!route) {
      throw new Error(
        `[shisho mock] No mock defined for URL: ${url}\nDefined routes: ${Object.keys(routes).join(", ")}`
      );
    }

    const status = route.status ?? 200;
    const body = route.body ?? "";
    const headers = route.headers ?? {};

    return {
      ok: status >= 200 && status < 300,
      status,
      statusText: status === 200 ? "OK" : String(status),
      headers,
      text: function () {
        return body;
      },
      json: function () {
        return JSON.parse(body);
      },
      arrayBuffer: function () {
        const encoder = new TextEncoder();
        return encoder.encode(body).buffer;
      },
    };
  };
}

function createMockConfig(values: Record<string, string>) {
  return {
    get: function (key: string): string | undefined {
      return values[key];
    },
    getAll: function (): Record<string, string> {
      return { ...values };
    },
  };
}

function createMockLog() {
  return {
    debug: function (_msg: string) {},
    info: function (_msg: string) {},
    warn: function (_msg: string) {},
    error: function (_msg: string) {},
  };
}

function createMockUrl() {
  return {
    encodeURIComponent: function (str: string): string {
      return encodeURIComponent(str);
    },
    decodeURIComponent: function (str: string): string {
      return decodeURIComponent(str);
    },
    searchParams: function (params: Record<string, unknown>): string {
      const entries: Array<[string, string]> = [];
      const sortedKeys = Object.keys(params).sort();
      for (const key of sortedKeys) {
        const val = params[key];
        if (Array.isArray(val)) {
          for (const item of val) {
            entries.push([key, String(item)]);
          }
        } else {
          entries.push([key, String(val)]);
        }
      }
      return entries
        .map(function ([k, v]) {
          return encodeURIComponent(k) + "=" + encodeURIComponent(v);
        })
        .join("&");
    },
    parse: function (urlStr: string) {
      const parsed = new URL(urlStr);
      const query: Record<string, string> = {};
      parsed.searchParams.forEach(function (value, key) {
        query[key] = value;
      });
      return {
        protocol: parsed.protocol.replace(":", ""),
        hostname: parsed.hostname,
        port: parsed.port,
        pathname: parsed.pathname,
        search: parsed.search,
        hash: parsed.hash,
        query,
      };
    },
  };
}

function createMockFs(files: Record<string, string | Buffer | string[]>) {
  return {
    readFile: function (path: string): ArrayBuffer {
      const content = files[path];
      if (content === undefined) {
        throw new Error(`[shisho mock] No mock file at path: ${path}`);
      }
      if (Array.isArray(content)) {
        throw new Error(`[shisho mock] Path is a directory, not a file: ${path}`);
      }
      if (typeof content === "string") {
        return new TextEncoder().encode(content).buffer;
      }
      return content.buffer.slice(content.byteOffset, content.byteOffset + content.byteLength);
    },
    readTextFile: function (path: string): string {
      const content = files[path];
      if (content === undefined) {
        throw new Error(`[shisho mock] No mock file at path: ${path}`);
      }
      if (Array.isArray(content)) {
        throw new Error(`[shisho mock] Path is a directory, not a file: ${path}`);
      }
      if (Buffer.isBuffer(content)) {
        return content.toString("utf-8");
      }
      return content;
    },
    writeFile: function (_path: string, _data: ArrayBuffer) {},
    writeTextFile: function (_path: string, _data: string) {},
    exists: function (path: string): boolean {
      return path in files;
    },
    mkdir: function (_path: string) {},
    listDir: function (path: string): string[] {
      const content = files[path];
      if (Array.isArray(content)) {
        return content;
      }
      throw new Error(`[shisho mock] No mock directory at path: ${path}`);
    },
    tempDir: function (): string {
      return "/tmp/shisho-mock-temp";
    },
  };
}

/**
 * Creates a mock shisho global object for plugin testing.
 *
 * @example
 * ```typescript
 * import { createMockShisho } from "@shisho/plugin-sdk/testing";
 *
 * const mockShisho = createMockShisho({
 *   fetch: {
 *     "https://api.example.com/book/123": {
 *       status: 200,
 *       body: JSON.stringify({ title: "Test Book" }),
 *     },
 *   },
 *   config: { api_key: "test-key" },
 * });
 *
 * globalThis.shisho = mockShisho;
 * ```
 */
export function createMockShisho(options: MockShishoOptions = {}) {
  return {
    log: createMockLog(),
    config: createMockConfig(options.config ?? {}),
    http: {
      fetch: createMockFetch(options.fetch ?? {}),
    },
    url: createMockUrl(),
    fs: createMockFs(options.fs ?? {}),
    dataDir: "/tmp/shisho-mock-data",
  };
}
```

Note: `xml` and `html` real implementations in the testing mock are deferred — they require adding library dependencies (e.g., `linkedom` or `cheerio` for HTML, a JS XML parser for XML) to the npm package, which adds complexity to the build. The core mock factory covers the most common plugin testing needs (fetch, config, url, fs, log). XML/HTML real implementations should be a follow-up task tracked on the Notion board.

- [ ] **Step 2: Create tsconfig for the testing subpath**

Create `packages/plugin-sdk/testing/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "declaration": true,
    "outDir": ".",
    "rootDir": ".",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true
  },
  "include": ["index.ts"]
}
```

- [ ] **Step 3: Build the testing module**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement/packages/plugin-sdk/testing && npx tsc -p tsconfig.json`

This should produce `index.js` and `index.d.ts` in the `testing/` directory.

- [ ] **Step 4: Update package.json with testing export**

In `packages/plugin-sdk/package.json`, update the `exports` and `files` fields:

```json
{
  "name": "@shisho/plugin-sdk",
  "version": "0.0.21",
  "description": "TypeScript SDK for Shisho plugin development — types, host API declarations, and test utilities",
  "homepage": "https://github.com/shishobooks/shisho/blob/master/packages/plugin-sdk/README.md",
  "types": "index.d.ts",
  "exports": {
    ".": "./index.d.ts",
    "./testing": {
      "types": "./testing/index.d.ts",
      "import": "./testing/index.js",
      "require": "./testing/index.js"
    }
  },
  "files": [
    "*.d.ts",
    "testing/"
  ],
  "scripts": {
    "build": "cd testing && npx tsc -p tsconfig.json"
  },
  "license": "MIT",
  "repository": {
    "type": "git",
    "url": "https://github.com/shishobooks/shisho.git",
    "directory": "packages/plugin-sdk"
  }
}
```

- [ ] **Step 5: Verify the build output**

Run: `ls -la /Users/robinjoseph/.worktrees/shisho/plugin-refinement/packages/plugin-sdk/testing/`

Expected: `index.ts`, `index.js`, `index.d.ts`, `tsconfig.json` all present.

- [ ] **Step 6: Commit**

```bash
git add packages/plugin-sdk/testing/ packages/plugin-sdk/package.json
git commit -m "[Feature] Add @shisho/plugin-sdk/testing with mock factory

Add createMockShisho() factory for plugin test environments.
Provides route-based fetch mocking, real url implementations,
path-based fs mocking, config map, and silent log no-ops.
Framework-agnostic — works with vitest, jest, or any test runner."
```

---

### Task 6: Update Documentation

**Files:**
- Modify: `website/docs/plugins/development.md` (add HTML API docs, cover guidance, SDK rename, testing docs)
- Modify: `pkg/plugins/CLAUDE.md` (update for type unification, HTML API, SDK rename)

- [ ] **Step 1: Read current plugin development docs**

Run: Read `website/docs/plugins/development.md` to understand the current structure and find insertion points.

- [ ] **Step 2: Update plugin development docs**

In `website/docs/plugins/development.md`, make the following updates:

1. **Replace all `@shisho/plugin-types` references with `@shisho/plugin-sdk`** throughout the file.

2. **Add `shisho.html` section** in the Host APIs section (after shisho.xml):

```markdown
### shisho.html

HTML parsing with full CSS selector support. Use this instead of regex for scraping HTML content.

```javascript
// Find a single element
var meta = shisho.html.querySelector(html, 'meta[name="description"]');
var description = meta ? meta.attributes.content : "";

// Find all matching elements
var items = shisho.html.querySelectorAll(html, '.book-item');

// Extract JSON-LD (common pattern for metadata enrichers)
var scripts = shisho.html.querySelectorAll(html, 'script[type="application/ld+json"]');
if (scripts.length > 0) {
  var jsonLd = JSON.parse(scripts[0].text);
}

// Extract Open Graph data
var ogTitle = shisho.html.querySelector(html, 'meta[property="og:title"]');
var title = ogTitle ? ogTitle.attributes.content : "";
```

Each returned element has:
- `tag` — element tag name (e.g., `"div"`, `"meta"`)
- `attributes` — key-value pairs (e.g., `{ name: "description", content: "..." }`)
- `text` — recursive inner text content
- `innerHTML` — raw inner HTML string
- `children` — child elements
```

3. **Update cover handling guidance** in the metadata enricher section:

```markdown
#### Cover Images

Set `coverUrl` on your search results — the server handles downloading and domain validation automatically. The URL's domain must be in your manifest's `httpAccess.domains` list.

```javascript
return {
  results: [{
    title: "Book Title",
    coverUrl: "https://covers.example.com/book.jpg"
  }]
};
```

For advanced use cases (file parsers extracting embedded covers, or enrichers that generate/composite images), you can set `coverData` as an `ArrayBuffer` instead. If both are set, `coverData` takes precedence.
```

4. **Update SearchResult references** — the search hook now returns `ParsedMetadata` directly:

```markdown
The `search()` hook returns `{ results: ParsedMetadata[] }`. Each result can include any ParsedMetadata field — title, authors, description, coverUrl, genres, identifiers, etc.
```

5. **Add testing section** at the end:

```markdown
## Testing Plugins

The SDK includes a test utilities package at `@shisho/plugin-sdk/testing` that eliminates mock boilerplate.

### Setup

```typescript
import { createMockShisho } from "@shisho/plugin-sdk/testing";

const mockShisho = createMockShisho({
  fetch: {
    "https://api.example.com/search?q=test": {
      status: 200,
      body: JSON.stringify({ results: [{ title: "Test Book" }] }),
    },
  },
  config: {
    api_key: "test-key",
  },
});

globalThis.shisho = mockShisho;
```

### What's Included

| API | Behavior |
|-----|----------|
| `log.*` | Silent no-ops |
| `url.*` | Real implementations (encodeURIComponent, searchParams, parse) |
| `config.*` | Returns values from the config map you provide |
| `http.fetch` | Route-based mock — matches URLs, throws on unmatched |
| `fs.*` | Path-based mock — virtual filesystem from the map you provide |

Unmatched fetch URLs and missing fs paths throw descriptive errors so you know exactly what mock data to add.
```

- [ ] **Step 3: Update pkg/plugins/CLAUDE.md**

Key updates:
- Replace `@shisho/plugin-types` references with `@shisho/plugin-sdk`
- Replace `packages/plugin-types/` paths with `packages/plugin-sdk/`
- Update the "Plugin Types SDK" section to mention the testing subpath
- Add `hostapi_html.go` to the architecture file listing
- Remove references to `SearchResult` and `SearchResultToMetadata` — update to say search hooks return `ParsedMetadata` directly
- Update the "Scan Pipeline Integration" section to remove the `SearchResultToMetadata()` reference

- [ ] **Step 4: Commit**

```bash
git add website/docs/plugins/development.md pkg/plugins/CLAUDE.md
git commit -m "[Docs] Update plugin docs for SDK rename, HTML API, type unification

Update all @shisho/plugin-types references to @shisho/plugin-sdk.
Add shisho.html API documentation with examples. Clarify cover
handling guidance. Document testing utilities. Update CLAUDE.md
for type unification and new host API."
```

---

### Task 7: Run Full Validation

**Files:** None (validation only)

- [ ] **Step 1: Run mise check:quiet**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && mise check:quiet`

Expected: All checks pass (Go tests, Go lint, JS lint).

- [ ] **Step 2: Run mise tygo**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && mise tygo`

This regenerates TypeScript types from Go structs. Since we added `PluginScope`/`PluginID` fields with `json:"-"` tags to `ParsedMetadata`, tygo should skip them (they won't appear in generated types). Verify the generated types don't include these fields.

- [ ] **Step 3: Verify no stale SearchResult references**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && grep -r "SearchResult" pkg/plugins/ --include="*.go" | grep -v "_test.go" | grep -v "EnrichSearchResult" | grep -v CLAUDE`

Expected: No matches. All `SearchResult` references should be gone from production code (only `EnrichSearchResult` remains).

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && grep -r "plugin-types" packages/ scripts/ .github/ .goreleaser.yaml`

Expected: No matches. All references should now point to `plugin-sdk`.

- [ ] **Step 4: Verify no stale references in CLAUDE.md files**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && grep -r "SearchResultToMetadata\|SearchResult[^s]" pkg/plugins/CLAUDE.md`

Expected: No matches.
