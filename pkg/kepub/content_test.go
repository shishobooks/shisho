package kepub

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformContent(t *testing.T) {
	t.Parallel()
	t.Run("adds wrapper divs to body content", func(t *testing.T) {
		input := `<html><head></head><body><p>Hello</p></body></html>`
		var buf bytes.Buffer

		err := TransformContent(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, `id="book-columns"`)
		assert.Contains(t, output, `id="book-inner"`)
	})

	t.Run("wraps text in koboSpan elements", func(t *testing.T) {
		input := `<html><head></head><body><p>Hello world</p></body></html>`
		var buf bytes.Buffer

		err := TransformContent(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, `class="koboSpan"`)
		assert.Contains(t, output, `kobo.`)
	})

	t.Run("generates unique span IDs for each text node", func(t *testing.T) {
		input := `<html><head></head><body>
			<p>First paragraph.</p>
			<p>Second paragraph.</p>
		</body></html>`
		var buf bytes.Buffer

		err := TransformContent(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
		// Each paragraph should increment the paragraph counter
		assert.Contains(t, output, `id="kobo.1.1"`)
		assert.Contains(t, output, `id="kobo.2.1"`)
	})

	t.Run("preserves original text content", func(t *testing.T) {
		input := `<html><head></head><body><p>Original text content here.</p></body></html>`
		var buf bytes.Buffer

		err := TransformContent(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Original text content here.")
	})

	t.Run("skips script and style elements", func(t *testing.T) {
		input := `<html><head><style>body { color: red; }</style></head><body>
			<script>alert("test");</script>
			<p>Visible text</p>
		</body></html>`
		var buf bytes.Buffer

		err := TransformContent(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
		// Script content should not be wrapped
		assert.Contains(t, output, `<script>alert("test");</script>`)
		// Style content should not be wrapped
		assert.Contains(t, output, `color: red`)
		// But paragraph text should be wrapped
		assert.Contains(t, output, `<span class="koboSpan"`)
	})

	t.Run("handles nested elements correctly", func(t *testing.T) {
		input := `<html><head></head><body><p>Text with <strong>bold</strong> and <em>italic</em>.</p></body></html>`
		var buf bytes.Buffer

		err := TransformContent(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
		// Original structure should be preserved
		assert.Contains(t, output, "<strong>")
		assert.Contains(t, output, "</strong>")
		assert.Contains(t, output, "<em>")
		assert.Contains(t, output, "</em>")
	})

	t.Run("handles empty body", func(t *testing.T) {
		input := `<html><head></head><body></body></html>`
		var buf bytes.Buffer

		err := TransformContent(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, `id="book-columns"`)
		assert.Contains(t, output, `id="book-inner"`)
	})

	t.Run("handles whitespace-only text nodes", func(t *testing.T) {
		input := `<html><head></head><body>
			<p>   </p>
			<p>Real content</p>
		</body></html>`
		var buf bytes.Buffer

		err := TransformContent(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
		// Should contain wrapped real content
		assert.Contains(t, output, "Real content")
		assert.Contains(t, output, `class="koboSpan"`)
	})

	t.Run("preserves special characters", func(t *testing.T) {
		input := `<html><head></head><body><p>Hello &amp; goodbye &lt;world&gt;</p></body></html>`
		var buf bytes.Buffer

		err := TransformContent(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
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
		var buf bytes.Buffer

		err := TransformContent(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
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

func TestTransformContentBytes(t *testing.T) {
	t.Parallel()
	t.Run("transforms content correctly", func(t *testing.T) {
		input := []byte(`<html><head></head><body><p>Test content</p></body></html>`)

		output, err := TransformContentBytes(input)
		require.NoError(t, err)

		assert.Contains(t, string(output), `id="book-columns"`)
		assert.Contains(t, string(output), `class="koboSpan"`)
	})
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

func TestTransformContentWithCounter(t *testing.T) {
	t.Parallel()
	t.Run("maintains counter across multiple calls", func(t *testing.T) {
		counter := NewSpanCounter()

		// First file
		input1 := `<html><head></head><body><p>First file content.</p></body></html>`
		var buf1 bytes.Buffer
		err := TransformContentWithCounter(strings.NewReader(input1), &buf1, counter)
		require.NoError(t, err)

		// Second file - should continue from where first left off
		input2 := `<html><head></head><body><p>Second file content.</p></body></html>`
		var buf2 bytes.Buffer
		err = TransformContentWithCounter(strings.NewReader(input2), &buf2, counter)
		require.NoError(t, err)

		output1 := buf1.String()
		output2 := buf2.String()

		// First file should have lower IDs
		assert.Contains(t, output1, `id="kobo.1.1"`)

		// Second file should have higher IDs (paragraph 2+)
		assert.Contains(t, output2, `id="kobo.2.1"`)
		// And should NOT have kobo.1.1
		assert.NotContains(t, output2, `id="kobo.1.1"`)
	})
}
