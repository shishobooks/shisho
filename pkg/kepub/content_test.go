package kepub

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func TestTransformDocument(t *testing.T) {
	t.Parallel()
	t.Run("adds wrapper divs to body content", func(t *testing.T) {
		input := `<html><head></head><body><p>Hello</p></body></html>`
		out, err := transformDocument([]byte(input), NewSpanCounter(), "")
		require.NoError(t, err)

		output := string(out)
		assert.Contains(t, output, `id="book-columns"`)
		assert.Contains(t, output, `id="book-inner"`)
	})

	t.Run("wraps text in koboSpan elements", func(t *testing.T) {
		input := `<html><head></head><body><p>Hello world</p></body></html>`
		out, err := transformDocument([]byte(input), NewSpanCounter(), "")
		require.NoError(t, err)

		output := string(out)
		assert.Contains(t, output, `class="koboSpan"`)
		assert.Contains(t, output, `kobo.`)
	})

	t.Run("generates unique span IDs for each text node", func(t *testing.T) {
		input := `<html><head></head><body>
			<p>First paragraph.</p>
			<p>Second paragraph.</p>
		</body></html>`
		out, err := transformDocument([]byte(input), NewSpanCounter(), "")
		require.NoError(t, err)

		output := string(out)
		// Each paragraph should increment the paragraph counter
		assert.Contains(t, output, `id="kobo.1.1"`)
		assert.Contains(t, output, `id="kobo.2.1"`)
	})

	t.Run("preserves original text content", func(t *testing.T) {
		input := `<html><head></head><body><p>Original text content here.</p></body></html>`
		out, err := transformDocument([]byte(input), NewSpanCounter(), "")
		require.NoError(t, err)

		output := string(out)
		assert.Contains(t, output, "Original text content here.")
	})

	t.Run("skips script and style elements", func(t *testing.T) {
		input := `<html><head><style>body { color: red; }</style></head><body>
			<script>alert("test");</script>
			<p>Visible text</p>
		</body></html>`
		out, err := transformDocument([]byte(input), NewSpanCounter(), "")
		require.NoError(t, err)

		output := string(out)
		// Script content should not be wrapped
		assert.Contains(t, output, `<script>alert("test");</script>`)
		// Style content should not be wrapped
		assert.Contains(t, output, `color: red`)
		// But paragraph text should be wrapped
		assert.Contains(t, output, `<span class="koboSpan"`)
	})

	t.Run("handles nested elements correctly", func(t *testing.T) {
		input := `<html><head></head><body><p>Text with <strong>bold</strong> and <em>italic</em>.</p></body></html>`
		out, err := transformDocument([]byte(input), NewSpanCounter(), "")
		require.NoError(t, err)

		output := string(out)
		// Original structure should be preserved
		assert.Contains(t, output, "<strong>")
		assert.Contains(t, output, "</strong>")
		assert.Contains(t, output, "<em>")
		assert.Contains(t, output, "</em>")
	})

	t.Run("handles empty body", func(t *testing.T) {
		input := `<html><head></head><body></body></html>`
		out, err := transformDocument([]byte(input), NewSpanCounter(), "")
		require.NoError(t, err)

		output := string(out)
		assert.Contains(t, output, `id="book-columns"`)
		assert.Contains(t, output, `id="book-inner"`)
	})

	t.Run("handles whitespace-only text nodes", func(t *testing.T) {
		input := `<html><head></head><body>
			<p>   </p>
			<p>Real content</p>
		</body></html>`
		out, err := transformDocument([]byte(input), NewSpanCounter(), "")
		require.NoError(t, err)

		output := string(out)
		// Should contain wrapped real content
		assert.Contains(t, output, "Real content")
		assert.Contains(t, output, `class="koboSpan"`)
	})

	t.Run("preserves special characters", func(t *testing.T) {
		input := `<html><head></head><body><p>Hello &amp; goodbye &lt;world&gt;</p></body></html>`
		out, err := transformDocument([]byte(input), NewSpanCounter(), "")
		require.NoError(t, err)

		output := string(out)
		// HTML entities should be preserved or properly encoded
		assert.Contains(t, output, "&amp;")
		assert.Contains(t, output, "&lt;")
		assert.Contains(t, output, "&gt;")
	})

	t.Run("resets sentence counter for each paragraph", func(t *testing.T) {
		input := `<html><head></head><body>
			<p>First sentence. Second sentence.</p>
			<p>Third sentence.</p>
		</body></html>`
		out, err := transformDocument([]byte(input), NewSpanCounter(), "")
		require.NoError(t, err)

		output := string(out)
		// First paragraph: kobo.1.1, kobo.1.2 (two sentences)
		// Second paragraph: kobo.2.1
		assert.Contains(t, output, `id="kobo.1.1"`)
		assert.Contains(t, output, `id="kobo.1.2"`)
		assert.Contains(t, output, `id="kobo.2.1"`)
	})
}

func TestSplitIntoSegments(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single sentence without punctuation",
			input:    "Hello world",
			expected: []string{"Hello world"},
		},
		{
			name:     "single sentence with period",
			input:    "Hello world.",
			expected: []string{"Hello world."},
		},
		{
			name:     "two sentences with period",
			input:    "First sentence. Second sentence.",
			expected: []string{"First sentence.", " ", "Second sentence."},
		},
		{
			name:     "sentence with question mark",
			input:    "What is this? This is a test.",
			expected: []string{"What is this?", " ", "This is a test."},
		},
		{
			name:     "sentence with exclamation mark",
			input:    "Wow! That is amazing!",
			expected: []string{"Wow!", " ", "That is amazing!"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "splits on newlines (multiline)",
			input:    "First line\nSecond line",
			expected: []string{"First line", "\n", "Second line"},
		},
		{
			name:     "splits on colon",
			input:    "Title: content here.",
			expected: []string{"Title:", " ", "content here."},
		},
		{
			name:     "preserves trailing text after last sentence",
			input:    "First. Second and more",
			expected: []string{"First.", " ", "Second and more"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitIntoSegments(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSpanCounter(t *testing.T) {
	t.Parallel()
	t.Run("generates sequential IDs", func(t *testing.T) {
		counter := NewSpanCounter()

		id1 := counter.nextID()
		id2 := counter.nextID()
		id3 := counter.nextID()

		assert.Equal(t, "kobo.0.1", id1)
		assert.Equal(t, "kobo.0.2", id2)
		assert.Equal(t, "kobo.0.3", id3)
	})

	t.Run("deferred paragraph increment on markParagraphBoundary", func(t *testing.T) {
		counter := NewSpanCounter()

		counter.nextID() // kobo.0.1
		counter.nextID() // kobo.0.2
		counter.markParagraphBoundary()
		id := counter.nextID()

		// Paragraph should increment when nextID is called after markParagraphBoundary
		assert.Equal(t, "kobo.1.1", id)
	})

	t.Run("immediate paragraph increment on incrementParagraph", func(t *testing.T) {
		counter := NewSpanCounter()

		counter.nextID() // kobo.0.1
		counter.incrementParagraph()
		id := counter.nextID()

		// incrementParagraph immediately increments, then nextID adds sentence
		assert.Equal(t, "kobo.1.1", id)
	})

	t.Run("markParagraphBoundary only increments once", func(t *testing.T) {
		counter := NewSpanCounter()

		counter.markParagraphBoundary()
		counter.markParagraphBoundary() // Second call should not double increment
		id1 := counter.nextID()
		id2 := counter.nextID()

		assert.Equal(t, "kobo.1.1", id1)
		assert.Equal(t, "kobo.1.2", id2) // Still in same paragraph
	})
}

func TestTransformDocument_SharedCounter(t *testing.T) {
	t.Parallel()
	t.Run("maintains counter across multiple calls", func(t *testing.T) {
		counter := NewSpanCounter()

		// First file
		input1 := `<html><head></head><body><p>First file content.</p></body></html>`
		out1, err := transformDocument([]byte(input1), counter, "")
		require.NoError(t, err)

		// Second file - should continue from where first left off
		input2 := `<html><head></head><body><p>Second file content.</p></body></html>`
		out2, err := transformDocument([]byte(input2), counter, "")
		require.NoError(t, err)

		output1 := string(out1)
		output2 := string(out2)

		// First file should have lower IDs
		assert.Contains(t, output1, `id="kobo.1.1"`)

		// Second file should have higher IDs (paragraph 2+)
		assert.Contains(t, output2, `id="kobo.2.1"`)
		// And should NOT have kobo.1.1
		assert.NotContains(t, output2, `id="kobo.1.1"`)
	})
}

// fidelityBody is chapter markup an HTML parser would restructure or
// re-render: a div inside a p, a pre's leading newline, case-sensitive and
// self-closed SVG, entities, single-quoted and uppercase attributes, a
// self-closed anchor, CDATA, and a comment holding a tag.
const fidelityBody = `<p>a<div>b</div>c</p>
<pre>
line</pre>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><linearGradient id="g"/><path d="M0 0"/></svg>
<p class='q'>Tom &amp; Jerry&#160;<br/><a id="x"/>after</p>
<P CLASS="u">Upper</P>
<![CDATA[ x > y ]]>
<!-- a <span class="koboSpan"> comment -->`

// fidelityDoc is an XHTML document with head's markup after its title and
// body as its body's content.
func fidelityDoc(head, body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>T</title>` + head + `</head>
<body>` + body + `</body>
</html>`
}

const koboStyleHacksTag = `<style type="text/css" id="kobostylehacks">div#book-inner { margin-top: 0; margin-bottom: 0; }</style>`

// Only the KePub markup is added: everything else in a document is copied
// as it was.
func TestTransformDocument_KeepsOtherMarkupAsItWas(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	require.NoError(t, TransformContentWithOptions(strings.NewReader(fidelityDoc("", fidelityBody)), &out, NewSpanCounter(), "../kobo.js"))

	want := fidelityDoc(
		koboStyleHacksTag+`<script type="text/javascript" src="../kobo.js"/>`,
		`<div id="book-columns"><div id="book-inner">`+
			`<p><span class="koboSpan" id="kobo.1.1">a</span><div><span class="koboSpan" id="kobo.1.2">b</span></div><span class="koboSpan" id="kobo.1.3">c</span></p>
<pre>
line</pre>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><linearGradient id="g"/><path d="M0 0"/></svg>
<p class='q'><span class="koboSpan" id="kobo.2.1">Tom &amp; Jerry&#160;</span><br/><a id="x"/><span class="koboSpan" id="kobo.2.2">after</span></p>
<P CLASS="u"><span class="koboSpan" id="kobo.3.1">Upper</span></P>
<![CDATA[ x > y ]]>
<!-- a <span class="koboSpan"> comment -->`+
			`</div></div>`)
	assert.Equal(t, want, out.String())
}

// Segments are found in the decoded text, and each is copied with its
// entities as they were, never split.
func TestTransformDocument_SplitsAroundEntities(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "an entity stays whole inside a segment",
			in:   `<p>A&amp;B. C&#x2019;s</p>`,
			want: `<p><span class="koboSpan" id="kobo.1.1">A&amp;B.</span> <span class="koboSpan" id="kobo.1.2">C&#x2019;s</span></p>`,
		},
		{
			name: "an entity for punctuation ends a sentence",
			in:   `<p>One&#46; Two</p>`,
			want: `<p><span class="koboSpan" id="kobo.1.1">One&#46;</span> <span class="koboSpan" id="kobo.1.2">Two</span></p>`,
		},
		{
			name: "an entity for a space separates sentences and is not wrapped",
			in:   `<p>One.&#32;Two</p>`,
			want: `<p><span class="koboSpan" id="kobo.1.1">One.</span>&#32;<span class="koboSpan" id="kobo.1.2">Two</span></p>`,
		},
		{
			name: "a no-break space is wrapped, as an entity or a character",
			in:   "<p>&nbsp;</p><p>&#160;</p><p> </p><p> </p>",
			want: "<p><span class=\"koboSpan\" id=\"kobo.1.1\">&nbsp;</span></p><p><span class=\"koboSpan\" id=\"kobo.2.1\">&#160;</span></p><p><span class=\"koboSpan\" id=\"kobo.3.1\"> </span></p><p> </p>",
		},
		{
			name: "a carriage return ends a line as a newline does",
			in:   "<p>One\r\nTwo &amp three</p>",
			want: "<p><span class=\"koboSpan\" id=\"kobo.1.1\">One</span>\r\n<span class=\"koboSpan\" id=\"kobo.1.2\">Two &amp three</span></p>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, err := transformDocument([]byte(`<html><head></head><body>`+tt.in+`</body></html>`), NewSpanCounter(), "")
			require.NoError(t, err)
			want := `<html><head>` + koboStyleHacksTag + `</head><body><div id="book-columns"><div id="book-inner">` + tt.want + `</div></div></body></html>`
			assert.Equal(t, want, string(out))
		})
	}
}

// decodeText decodes text as the HTML tokenizer does.
func TestDecodeText_MatchesTokenizer(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"plain",
		"&amp; &amp &ampx &AMP; &notit; &notin; &bogus; & &#",
		"&#46;&#x2E;&#X2e &#12a &#x; &#xZ &#1114112; &#0;",
		"a\r\nb\rc\n\rd",
		"café &eacute; &NotEqualTilde;",
	} {
		z := html.NewTokenizer(strings.NewReader(raw))
		require.Equal(t, html.TextToken, z.Next(), raw)
		decoded, rawAt := decodeText([]byte(raw))
		assert.Equal(t, string(z.Text()), decoded, raw)
		assert.Len(t, rawAt, len(decoded)+1, raw)
	}
}

// Images start a paragraph and get a span of their own, and nothing in a
// skipped element is wrapped.
func TestTransformDocument_ImagesAndSkippedElements(t *testing.T) {
	t.Parallel()
	in := fidelityDoc("", `<h1>Head</h1><p>Text <img src="a.png" alt="A"/> more</p><code>x()</code><math><mi>x</mi></math><p>End</p>`)
	out, err := transformDocument([]byte(in), NewSpanCounter(), "")
	require.NoError(t, err)
	want := fidelityDoc(koboStyleHacksTag,
		`<div id="book-columns"><div id="book-inner"><h1><span class="koboSpan" id="kobo.1.1">Head</span></h1><p><span class="koboSpan" id="kobo.2.1">Text </span><span class="koboSpan" id="kobo.3.1"><img src="a.png" alt="A"/></span> <span class="koboSpan" id="kobo.3.2">more</span></p><code>x()</code><math><mi>x</mi></math><p><span class="koboSpan" id="kobo.4.1">End</span></p></div></div>`)
	assert.Equal(t, want, string(out))
}

// Elements HTML lets hold only text get no spans: a time with no datetime
// attribute, whose text is the date, and an option. A time with a datetime
// attribute holds phrasing content, so its text is wrapped.
func TestTransformDocument_TextOnlyElements(t *testing.T) {
	t.Parallel()
	in := fidelityDoc("", `<p>In <time>1900</time>, <time datetime="1993">later</time>.</p><select><option>One</option></select>`)
	out, err := transformDocument([]byte(in), NewSpanCounter(), "")
	require.NoError(t, err)
	want := fidelityDoc(koboStyleHacksTag,
		`<div id="book-columns"><div id="book-inner"><p><span class="koboSpan" id="kobo.1.1">In </span><time>1900</time><span class="koboSpan" id="kobo.1.2">, </span><time datetime="1993"><span class="koboSpan" id="kobo.1.3">later</span></time><span class="koboSpan" id="kobo.1.4">.</span></p><select><option>One</option></select></div></div>`)
	assert.Equal(t, want, string(out))
}

// Transforming a KePub again changes nothing: its spans, wrappers, style
// hacks, and script stay as they were, without a second copy.
func TestTransformDocument_Idempotent(t *testing.T) {
	t.Parallel()
	var first bytes.Buffer
	require.NoError(t, TransformContentWithOptions(strings.NewReader(fidelityDoc("", fidelityBody)), &first, NewSpanCounter(), "../kobo.js"))
	var second bytes.Buffer
	require.NoError(t, TransformContentWithOptions(bytes.NewReader(first.Bytes()), &second, NewSpanCounter(), "../kobo.js"))
	assert.Equal(t, first.String(), second.String())
}

// transform runs transformDocument on in with a new counter and the script
// at ../kobo.js.
func transform(t *testing.T, in string) string {
	t.Helper()
	out, err := transformDocument([]byte(in), NewSpanCounter(), "../kobo.js")
	require.NoError(t, err)
	return string(out)
}

const koboHeadEnd = koboStyleHacksTag + `<script type="text/javascript" src="../kobo.js"/>`

// Without a body start tag, the content after the head, or after the html
// start tag when there is no head, or the whole of a fragment, is the body:
// its text gets spans and the wrappers go around it.
func TestTransformDocument_NoBodyTag(t *testing.T) {
	t.Parallel()
	const p = `<p><span class="koboSpan" id="kobo.1.1">Hi.</span></p>`
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "content after the head",
			in:   "<html><head><title>T</title></head>\n<p>Hi.</p>\n</html>",
			want: "<html><head><title>T</title>" + koboHeadEnd + "</head>\n" + bookWrappersStart + p + "\n" + bookWrappersEnd + "</html>",
		},
		{
			name: "content after the head without an html end tag",
			in:   "<html><head></head><p>Hi.</p>",
			want: "<html><head>" + koboHeadEnd + "</head>" + bookWrappersStart + p + bookWrappersEnd,
		},
		{
			name: "a body end tag ends it",
			in:   "<html><head></head><p>Hi.</p></body></html>",
			want: "<html><head>" + koboHeadEnd + "</head>" + bookWrappersStart + p + bookWrappersEnd + "</body></html>",
		},
		{
			name: "no head",
			in:   "<html><p>Hi.</p></html>",
			want: "<html><head>" + koboHeadEnd + "</head>" + bookWrappersStart + p + bookWrappersEnd + "</html>",
		},
		{
			name: "head elements before the content stay out of it",
			in:   `<html><title>T</title><link rel="stylesheet" href="s.css"/><p>Hi.</p></html>`,
			want: `<html><title>T</title><link rel="stylesheet" href="s.css"/><head>` + koboHeadEnd + "</head>" + bookWrappersStart + p + bookWrappersEnd + "</html>",
		},
		{
			name: "a fragment",
			in:   "<p>Hi.</p>",
			want: "<head>" + koboHeadEnd + "</head>" + bookWrappersStart + p + bookWrappersEnd,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, transform(t, tt.in))
			assert.Equal(t, tt.want, transform(t, tt.want), "transforming again")
		})
	}
}

// The style and script go at the end of the head even when it isn't closed
// before the body, and into a head of their own when there is none.
func TestTransformDocument_HeadNotClosedOrMissing(t *testing.T) {
	t.Parallel()
	const body = "<body>" + bookWrappersStart + `<p><span class="koboSpan" id="kobo.1.1">Hi.</span></p>` + bookWrappersEnd + "</body>"
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no head end tag",
			in:   "<html><head><title>T</title><body><p>Hi.</p></body></html>",
			want: "<html><head><title>T</title>" + koboHeadEnd + body + "</html>",
		},
		{
			name: "no head end tag and no html end tag",
			in:   "<html><head><title>T</title><body><p>Hi.</p></body>",
			want: "<html><head><title>T</title>" + koboHeadEnd + body,
		},
		{
			name: "no head",
			in:   "<html><body><p>Hi.</p></body></html>",
			want: "<html><head>" + koboHeadEnd + "</head>" + body + "</html>",
		},
		{
			name: "a self-closed head",
			in:   `<html><head lang="en" /><body><p>Hi.</p></body></html>`,
			want: `<html><head lang="en">` + koboHeadEnd + "</head>" + body + "</html>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, transform(t, tt.in))
		})
	}
}

// Text already inside a koboSpan is left alone and other text is wrapped,
// with ids after every Kobo id the document has.
func TestTransformDocument_WrapsTextOutsideExistingSpans(t *testing.T) {
	t.Parallel()
	in := `<html><head></head><body><p><span class="koboSpan" id="kobo.3.2">Old.</span> Tail.</p><p>New.</p><p><span class="koboSpan" id="kobo.1.1"><img src="a.png"/></span><img src="b.png"/></p></body></html>`
	want := `<html><head>` + koboHeadEnd + `</head><body>` + bookWrappersStart +
		`<p><span class="koboSpan" id="kobo.3.2">Old.</span> <span class="koboSpan" id="kobo.4.1">Tail.</span></p>` +
		`<p><span class="koboSpan" id="kobo.5.1">New.</span></p>` +
		`<p><span class="koboSpan" id="kobo.1.1"><img src="a.png"/></span><span class="koboSpan" id="kobo.6.1"><img src="b.png"/></span></p>` +
		bookWrappersEnd + `</body></html>`
	assert.Equal(t, want, transform(t, in))
}

// Either wrapper div means the wrappers are there already.
func TestTransformDocument_BookInnerAlone(t *testing.T) {
	t.Parallel()
	in := `<html><head></head><body><div id="book-inner"><p>Hi.</p></div></body></html>`
	want := `<html><head>` + koboHeadEnd + `</head><body><div id="book-inner"><p><span class="koboSpan" id="kobo.1.1">Hi.</span></p></div></body></html>`
	assert.Equal(t, want, transform(t, in))
}

// A noscript in the head is part of the head, not the start of the body.
func TestTransformDocument_NoscriptInHead(t *testing.T) {
	t.Parallel()
	in := `<html><head><noscript><link rel="stylesheet" href="n.css"/></noscript></head><body><p>Hi.</p></body></html>`
	want := `<html><head><noscript><link rel="stylesheet" href="n.css"/></noscript>` + koboHeadEnd + `</head><body>` + bookWrappersStart + `<p><span class="koboSpan" id="kobo.1.1">Hi.</span></p>` + bookWrappersEnd + `</body></html>`
	assert.Equal(t, want, transform(t, in))
}
