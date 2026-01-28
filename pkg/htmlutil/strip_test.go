package htmlutil

import (
	"testing"

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
			expected: "First paragraph\nSecond paragraph",
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
			expected: "The apocalypse will be televised!\nA man. His ex-girlfriend's cat.",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := StripTags(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDecodeHTMLEntities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ampersand",
			input:    "Tom &amp; Jerry",
			expected: "Tom & Jerry",
		},
		{
			name:     "less than greater than",
			input:    "&lt;tag&gt;",
			expected: "<tag>",
		},
		{
			name:     "quotes",
			input:    "&quot;quoted&quot;",
			expected: "\"quoted\"",
		},
		{
			name:     "apostrophe variants",
			input:    "it&#39;s &apos;quoted&apos;",
			expected: "it's 'quoted'",
		},
		{
			name:     "dashes named entities",
			input:    "em&mdash;dash and en&ndash;dash",
			expected: "em\u2014dash and en\u2013dash",
		},
		{
			name:     "dashes numeric entities",
			input:    "em&#8212;dash and en&#8211;dash",
			expected: "em\u2014dash and en\u2013dash",
		},
		{
			name:     "copyright trademark",
			input:    "&copy; 2024 Brand&trade; &reg;",
			expected: "\u00A9 2024 Brand\u2122 \u00AE",
		},
		{
			name:     "numeric entities",
			input:    "&#60;tag&#62; &#38; &#8220;quoted&#8221; &#8216;single&#8217;",
			expected: "<tag> & \u201Cquoted\u201D \u2018single\u2019",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := decodeHTMLEntities(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
