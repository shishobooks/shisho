package kepub

import (
	"bytes"
	"fmt"
	"io"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// TransformContent transforms HTML/XHTML content for KePub compatibility.
// It adds Kobo span wrappers around text nodes and wrapper divs around body content.
// This creates a new span counter, so span IDs will start from kobo.1.1.
// For converting multiple files in one book, use TransformContentWithCounter instead.
func TransformContent(r io.Reader, w io.Writer) error {
	return TransformContentWithCounter(r, w, NewSpanCounter())
}

// TransformContentWithCounter transforms content using the provided span counter.
// This allows maintaining unique span IDs across multiple files in a book.
// NOTE: Per Kobo's implementation, paragraph counters should be per-file, starting at 1.
func TransformContentWithCounter(r io.Reader, w io.Writer, counter *SpanCounter) error {
	return TransformContentWithOptions(r, w, counter, "")
}

// TransformContentWithOptions transforms content with full control over options.
// The scriptPath parameter specifies the relative path to kobo.js from the content file.
// If empty, no script reference is added.
// This function preserves XHTML format by pre/post processing around html.Parse.
func TransformContentWithOptions(r io.Reader, w io.Writer, counter *SpanCounter, scriptPath string) error {
	// Read all input first
	input, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	// Pre-process to extract XHTML-specific elements
	xhtml := NewXHTMLProcessor()
	preprocessed := xhtml.PreProcess(input)

	// Parse the HTML content
	doc, err := html.Parse(bytes.NewReader(preprocessed))
	if err != nil {
		return err
	}

	// Only transform the body element - never touch head/title
	body := findElement(doc, atom.Body)
	if body != nil {
		transformNode(body, counter)
	}

	// Add Kobo style hacks to head
	addKoboStyleHacks(doc)

	// Add kobo.js script reference to head (if path provided)
	if scriptPath != "" {
		addKoboScriptRef(doc, scriptPath)
	}

	// Add wrapper divs to body
	addBodyWrappers(doc)

	// Render to HTML5 format
	var htmlBuf bytes.Buffer
	if err := html.Render(&htmlBuf, doc); err != nil {
		return err
	}

	// Post-process to restore XHTML format
	result := xhtml.PostProcess(htmlBuf.Bytes())
	_, err = w.Write(result)
	return err
}

// SpanCounter tracks span IDs for unique identification across content files.
// Use NewSpanCounter to create one, and pass the same counter to all content
// files in a book to ensure globally unique span IDs.
type SpanCounter struct {
	paragraph   int
	sentence    int
	incParaNext bool // Deferred paragraph increment, only applied when creating a span
}

// NewSpanCounter creates a new span counter starting from 0.
func NewSpanCounter() *SpanCounter {
	return &SpanCounter{}
}

// nextID returns the next span ID and increments counters.
// If incParaNext is set, it increments paragraph first and resets sentence.
func (c *SpanCounter) nextID() string {
	if c.incParaNext {
		c.paragraph++
		c.sentence = 0
		c.incParaNext = false
	}
	c.sentence++
	return fmt.Sprintf("kobo.%d.%d", c.paragraph, c.sentence)
}

// markParagraphBoundary marks that the next span should start a new paragraph.
// This is called when encountering block elements like p, ol, ul, table, headings.
func (c *SpanCounter) markParagraphBoundary() {
	c.incParaNext = true
}

// incrementParagraph immediately increments the paragraph counter.
// This is called for elements like images that should always start a new paragraph.
func (c *SpanCounter) incrementParagraph() {
	c.paragraph++
	c.sentence = 0
	c.incParaNext = false
}

// transformNode recursively transforms nodes, adding Kobo spans to text nodes.
func transformNode(n *html.Node, counter *SpanCounter) {
	// Skip certain elements that shouldn't have spans
	if n.Type == html.ElementNode {
		//nolint:exhaustive // We only need to handle specific block elements
		switch n.DataAtom {
		case atom.Script, atom.Style, atom.Pre, atom.Code, atom.Svg, atom.Math:
			return // Don't transform these elements
		// Block elements that mark paragraph boundaries (kepubify pattern)
		case atom.P, atom.Ol, atom.Ul, atom.Table, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
			counter.markParagraphBoundary()
		default:
			// Other elements: continue processing children
		}
	}

	// Process child nodes
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling

		if c.Type == html.TextNode && shouldWrapText(c.Data) {
			// Wrap text node in Kobo spans
			wrapTextNode(n, c, counter)
		} else if c.Type == html.ElementNode && c.DataAtom == atom.Img {
			// Images immediately increment paragraph and get wrapped in spans
			counter.incrementParagraph()
			wrapImageInSpan(n, c, counter)
		} else {
			transformNode(c, counter)
		}

		c = next
	}
}

// shouldWrapText returns true if the text contains meaningful content to wrap.
// Note: NBSP (U+00A0) is considered meaningful content and should be wrapped,
// as Kobo uses it for position tracking.
func shouldWrapText(text string) bool {
	// Skip if empty
	if text == "" {
		return false
	}
	// Check if it contains any non-whitespace or NBSP characters
	for _, r := range text {
		// NBSP (U+00A0) should be wrapped
		if r == '\u00A0' {
			return true
		}
		// Regular non-whitespace should be wrapped
		if !unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

// wrapTextNode wraps a text node in Kobo span elements.
// Each sentence gets its own span with a unique ID.
func wrapTextNode(parent *html.Node, textNode *html.Node, counter *SpanCounter) {
	text := textNode.Data

	// Split text into sentences
	sentences := splitIntoSegments(text)
	if len(sentences) == 0 {
		return
	}

	// Create a document fragment to hold the new nodes
	var newNodes []*html.Node

	for _, sentence := range sentences {
		if !shouldWrapText(sentence) {
			// Preserve whitespace-only segments as plain text (but not NBSP)
			newNodes = append(newNodes, &html.Node{
				Type: html.TextNode,
				Data: sentence,
			})
			continue
		}

		// Create span wrapper
		span := &html.Node{
			Type:     html.ElementNode,
			DataAtom: atom.Span,
			Data:     "span",
			Attr: []html.Attribute{
				{Key: "class", Val: "koboSpan"},
				{Key: "id", Val: counter.nextID()},
			},
		}

		// Add text content to span
		span.AppendChild(&html.Node{
			Type: html.TextNode,
			Data: sentence,
		})

		newNodes = append(newNodes, span)
	}

	// Replace original text node with new nodes
	for _, newNode := range newNodes {
		parent.InsertBefore(newNode, textNode)
	}
	parent.RemoveChild(textNode)
}

// wrapImageInSpan wraps an image element in a Kobo span.
// This is required by Kobo for proper progress tracking around images.
func wrapImageInSpan(parent *html.Node, imgNode *html.Node, counter *SpanCounter) {
	// Create span wrapper
	span := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Span,
		Data:     "span",
		Attr: []html.Attribute{
			{Key: "class", Val: "koboSpan"},
			{Key: "id", Val: counter.nextID()},
		},
	}

	// Insert span at image's position, then move image into span
	parent.InsertBefore(span, imgNode)
	parent.RemoveChild(imgNode)
	span.AppendChild(imgNode)
}

// splitIntoSegments splits text into segments for Kobo span wrapping.
// This matches Calibre's approach of splitting on both sentence boundaries AND line breaks.
// Each segment becomes a separate koboSpan for fine-grained position tracking.
func splitIntoSegments(text string) []string {
	if text == "" {
		return nil
	}

	var result []string
	currentPos := 0

	for currentPos < len(text) {
		// Skip leading whitespace and add as separate segment
		wsStart := currentPos
		for currentPos < len(text) && isWhitespace(rune(text[currentPos])) {
			currentPos++
		}
		if currentPos > wsStart {
			result = append(result, text[wsStart:currentPos])
		}

		if currentPos >= len(text) {
			break
		}

		// Find the end of this segment - either sentence punctuation+whitespace or newline
		segStart := currentPos
		segEnd := -1

		for i := currentPos; i < len(text); i++ {
			ch := text[i]

			// Check for sentence-ending punctuation followed by whitespace
			if ch == '.' || ch == '!' || ch == '?' || ch == ':' {
				// Found punctuation - look for optional quotes then whitespace
				j := i + 1
				for j < len(text) && isQuote(rune(text[j])) {
					j++
				}
				// If followed by whitespace or end of text, this is a segment boundary
				if j >= len(text) || isWhitespace(rune(text[j])) {
					segEnd = j
					break
				}
			}

			// Check for newline (segment ends at line boundary)
			if ch == '\n' {
				segEnd = i
				break
			}
		}

		// If no boundary found, take rest of text
		if segEnd == -1 {
			segEnd = len(text)
		}

		// Add the segment if non-empty
		if segEnd > segStart {
			result = append(result, text[segStart:segEnd])
		}

		currentPos = segEnd
	}

	return result
}

// isWhitespace returns true if the rune is a whitespace character.
func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// isQuote returns true if the rune is a quote character.
func isQuote(r rune) bool {
	return r == '\'' || r == '"' ||
		r == '\u201c' || r == '\u201d' || // " "
		r == '\u2018' || r == '\u2019' || // ' '
		r == '\u2026' // …
}

// addKoboStyleHacks adds the Kobo style hacks stylesheet to the document head.
// This is required by Kobo for proper margin handling.
func addKoboStyleHacks(doc *html.Node) {
	head := findElement(doc, atom.Head)
	if head == nil {
		return
	}

	// Create style element with Kobo hacks
	style := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Style,
		Data:     "style",
		Attr: []html.Attribute{
			{Key: "type", Val: "text/css"},
			{Key: "id", Val: "kobostylehacks"},
		},
	}

	// Add the CSS content
	style.AppendChild(&html.Node{
		Type: html.TextNode,
		Data: "div#book-inner { margin-top: 0; margin-bottom: 0; }",
	})

	// Append to head
	head.AppendChild(style)
}

// addKoboScriptRef adds a reference to kobo.js in the document head.
// This JavaScript file handles pagination and progress tracking on Kobo devices.
// The scriptPath should be relative to the content file's location.
func addKoboScriptRef(doc *html.Node, scriptPath string) {
	head := findElement(doc, atom.Head)
	if head == nil {
		return
	}

	// Create script element referencing kobo.js
	script := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Script,
		Data:     "script",
		Attr: []html.Attribute{
			{Key: "type", Val: "text/javascript"},
			{Key: "src", Val: scriptPath},
		},
	}

	// Append to head
	head.AppendChild(script)
}

// addBodyWrappers adds the Kobo wrapper divs around body content.
// Wraps body content with: div#book-columns > div#book-inner > original content.
func addBodyWrappers(doc *html.Node) {
	body := findElement(doc, atom.Body)
	if body == nil {
		return
	}

	// Create wrapper divs
	bookColumns := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Div,
		Data:     "div",
		Attr: []html.Attribute{
			{Key: "id", Val: "book-columns"},
		},
	}

	bookInner := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Div,
		Data:     "div",
		Attr: []html.Attribute{
			{Key: "id", Val: "book-inner"},
		},
	}

	// Move all body children to book-inner
	for c := body.FirstChild; c != nil; {
		next := c.NextSibling
		body.RemoveChild(c)
		bookInner.AppendChild(c)
		c = next
	}

	// Build the hierarchy
	bookColumns.AppendChild(bookInner)
	body.AppendChild(bookColumns)
}

// findElement finds the first element with the given atom in the document.
func findElement(n *html.Node, a atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == a {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElement(c, a); found != nil {
			return found
		}
	}
	return nil
}

// TransformContentBytes is a convenience function that transforms content from bytes.
func TransformContentBytes(input []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := TransformContent(bytes.NewReader(input), &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
