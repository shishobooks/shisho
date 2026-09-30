package htmlutil

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStripTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "plain text no tags",
			input:    "Hello world",
			expected: "Hello world",
		},
		{
			name:     "simple paragraph",
			input:    "<p>Hello world</p>",
			expected: "Hello world",
		},
		{
			name:     "multiple paragraphs",
			input:    "<p>First paragraph</p><p>Second paragraph</p>",
			expected: "First paragraph\n\nSecond paragraph",
		},
		{
			name:     "div with content",
			input:    "<div>Content here</div>",
			expected: "Content here",
		},
		{
			name:     "nested tags",
			input:    "<p><strong>Bold</strong> and <em>italic</em></p>",
			expected: "Bold and italic",
		},
		{
			name:     "br tags",
			input:    "Line one<br>Line two<br/>Line three<br />Line four",
			expected: "Line one\nLine two\nLine three\nLine four",
		},
		{
			name:     "tags with attributes",
			input:    `<p style="font-weight: 600">Styled text</p>`,
			expected: "Styled text",
		},
		{
			name:     "complex html from screenshot",
			input:    `<div><p style="font-weight: 600">The apocalypse <em>will</em> be televised!</p><p>A man. His ex-girlfriend's cat.</p></div>`,
			expected: "The apocalypse will be televised!\n\nA man. His ex-girlfriend's cat.",
		},
		{
			name:     "html entities",
			input:    "Tom &amp; Jerry &mdash; the classic",
			expected: "Tom & Jerry \u2014 the classic",
		},
		{
			name:     "multiple spaces collapsed",
			input:    "Too    many    spaces",
			expected: "Too many spaces",
		},
		{
			name:     "list items",
			input:    "<ul><li>Item one</li><li>Item two</li></ul>",
			expected: "Item one\nItem two",
		},
		{
			name:     "headings",
			input:    "<h1>Title</h1><p>Content</p>",
			expected: "Title\nContent",
		},
		{
			name:     "nbsp entity",
			input:    "Hello&nbsp;world",
			expected: "Hello world",
		},
		{
			name:     "quotes entities",
			input:    "&ldquo;Hello&rdquo; said the &lsquo;man&rsquo;",
			expected: "\u201CHello\u201D said the \u2018man\u2019",
		},
		{
			name:     "self-closing tags",
			input:    "Text <img src='test.jpg'/> more text",
			expected: "Text more text",
		},
		{
			name:     "preserves content between inline tags",
			input:    "This is <strong>very</strong> important",
			expected: "This is very important",
		},
		{
			name:     "preserves double newlines in plain text",
			input:    "First paragraph\n\nSecond paragraph",
			expected: "First paragraph\n\nSecond paragraph",
		},
		{
			name:     "preserves multiple paragraph breaks in plain text",
			input:    "Paragraph one\n\nParagraph two\n\nParagraph three",
			expected: "Paragraph one\n\nParagraph two\n\nParagraph three",
		},
		{
			name:     "collapses three or more newlines to double",
			input:    "First\n\n\nSecond\n\n\n\nThird",
			expected: "First\n\nSecond\n\nThird",
		},
		{
			name:     "html paragraphs produce double newlines",
			input:    "<p>First paragraph</p><p>Second paragraph</p><p>Third paragraph</p>",
			expected: "First paragraph\n\nSecond paragraph\n\nThird paragraph",
		},
		{
			name:     "escaped less-than in prose decodes",
			input:    "<p>a &lt; b</p>",
			expected: "a < b",
		},
		{
			name:     "escaped comparison operators in prose decode",
			input:    "a &lt; b and c &gt; d",
			expected: "a < b and c > d",
		},
		{
			name:     "entity-encoded tag is removed",
			input:    "&lt;img src=x onerror=alert(1)&gt;",
			expected: "",
		},
		{
			name:     "entity-encoded script keeps only its text",
			input:    "Before &lt;script&gt;alert(1)&lt;/script&gt; after",
			expected: "Before alert(1) after",
		},
		{
			name:     "numeric-entity-encoded tag is removed",
			input:    "x&#60;b&#62;bold&#60;/b&#62;y",
			expected: "xboldy",
		},
		{
			name:     "double-encoded tag decodes one level to inert text",
			input:    "&amp;lt;b&amp;gt;x",
			expected: "&lt;b&gt;x",
		},
		{
			name:     "hex-encoded tag is removed",
			input:    "a&#x3C;img src=x onerror=alert(1)&#x3E;b",
			expected: "ab",
		},
		{
			name:     "zero-padded decimal and uppercase entities are removed",
			input:    "a&#060;b&#062;b&LT;i&GT;c",
			expected: "abc",
		},
		{
			name:     "numeric and named entities decode",
			input:    "&copy; 2024 &#8220;quoted&#8221; &#39;it&apos;s&#39;",
			expected: "\u00A9 2024 \u201Cquoted\u201D 'it's'",
		},
		{
			name:     "removing a tag cannot join the pieces of another",
			input:    "&lt;&lt;b&gt;img src=x onerror=alert(1)&gt;",
			expected: "img src=x onerror=alert(1)>",
		},
		{
			name:     "unclosed entity-encoded tag cannot open",
			input:    "<p>&lt;img src=x onerror=alert(1)</p><p>next</p>",
			expected: "img src=x onerror=alert(1)\n\nnext",
		},
		{
			name:     "unclosed raw tag cannot open",
			input:    "a <b",
			expected: "a b",
		},
		{
			name:     "entity-encoded paragraphs keep their breaks",
			input:    "&lt;p&gt;First&lt;/p&gt;&lt;p&gt;Second&lt;br&gt;line&lt;/p&gt;",
			expected: "First\n\nSecond\nline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := StripTags(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Nested encoded tags are handled in linear time: a loop that strips one level
// per pass takes seconds on this input.
func TestStripTags_NestedEncodedTagsAreLinear(t *testing.T) {
	t.Parallel()
	const depth = 20000
	input := strings.Repeat("&lt;", depth) + "b&gt;" + strings.Repeat("b&gt;", depth)

	start := time.Now()
	result := StripTags(input)
	elapsed := time.Since(start)

	assert.NotRegexp(t, `<[A-Za-z/!?]`, result)
	assert.Less(t, elapsed, 2*time.Second)
}
