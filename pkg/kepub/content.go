package kepub

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/pkg/errors"
	"golang.org/x/net/html"
)

// TransformContentWithOptions transforms content with full control over options.
// The scriptPath parameter specifies the relative path to kobo.js from the content file.
// If empty, no script reference is added.
func TransformContentWithOptions(r io.Reader, w io.Writer, counter *SpanCounter, scriptPath string) error {
	input, err := io.ReadAll(r)
	if err != nil {
		return errors.WithStack(err)
	}
	out, err := transformDocument(input, counter, scriptPath)
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return errors.WithStack(err)
}

var (
	// noSpanElements are the elements whose content gets no spans: code,
	// scripts, and markup of other vocabularies, along with the elements
	// the HTML tokenizer reads as raw text, where a span would be text.
	noSpanElements = map[string]bool{
		"script": true, "style": true, "pre": true, "code": true, "svg": true, "math": true,
		"iframe": true, "noembed": true, "noframes": true, "noscript": true, "plaintext": true,
		"textarea": true, "title": true, "xmp": true,
	}
	// paragraphElements are the elements that start a new paragraph of
	// span ids.
	paragraphElements = map[string]bool{
		"p": true, "ol": true, "ul": true, "table": true,
		"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	}
	// headElements are the elements that belong in the head, so in a
	// document without a body start tag, they don't start the body.
	headElements = map[string]bool{
		"base": true, "link": true, "meta": true, "noscript": true, "script": true, "style": true,
		"template": true, "title": true,
	}
)

// transformDocument adds Kobo's markup to a content document: koboSpans
// around the body's text and images, the book-columns and book-inner divs
// around the body's content, and the kobostylehacks style and the script
// at scriptPath at the end of the head. Each is left out where the
// document already has it, so a KePub is not wrapped twice.
//
// Without a body start tag, the body is the content after the head, or
// after the html start tag when there is no head, starting at the first
// text or tag that doesn't belong in the head. The head ends where the body
// starts if it is still open, and where there is no head, one is added
// there for the style and script.
//
// It reads the document as a stream of HTML tokens rather than parsing it
// into a tree, and copies every token it doesn't change as it was. Go's
// HTML parser would restructure markup that is valid XHTML but not valid
// HTML, such as a div inside a p or a self-closed a, and re-render the
// rest.
func transformDocument(data []byte, counter *SpanCounter, scriptPath string) ([]byte, error) {
	has, err := findKoboMarkup(data)
	if err != nil {
		return nil, err
	}
	var headEnd []byte
	if !has.styleHacks {
		headEnd = append(headEnd, koboStyleHacks...)
	}
	if scriptPath != "" && !has.script {
		headEnd = append(headEnd, koboScriptTag(scriptPath)...)
	}
	wrapBody := !has.wrappers
	if has.koboIDs {
		counter.continueAfter(has.lastParagraph)
	}

	// open is an element whose end tag is still to come.
	type open struct {
		name string
		// body is set on the element the body's content is in: the body,
		// or a stand-in where there is no body start tag.
		body     bool
		koboSpan bool
	}
	var stack []open
	openCount := map[string]int{}
	inBody, noSpans, inKoboSpans := 0, 0, 0
	bodyStarted := false
	var out bytes.Buffer
	out.Grow(len(data) + len(data)/2)

	// closeFrom closes stack[i:], innermost first, writing what Kobo's
	// markup adds at the end of each: the style and script at the end of
	// the head, and the wrappers' end tags at the end of the body.
	closeFrom := func(i int) {
		for j := len(stack) - 1; j >= i; j-- {
			o := stack[j]
			openCount[o.name]--
			if o.name == "head" {
				out.Write(headEnd)
				headEnd = nil
			}
			if o.body {
				inBody--
				if wrapBody {
					out.WriteString(bookWrappersEnd)
				}
			}
			if noSpanElements[o.name] {
				noSpans--
			}
			if o.koboSpan {
				inKoboSpans--
			}
		}
		stack = stack[:i]
	}
	// beginBody ends the head, if it is still open, before the body
	// starts, or adds one for the style and script when there is none.
	beginBody := func() {
		bodyStarted = true
		if openCount["head"] > 0 {
			i := len(stack) - 1
			for stack[i].name != "head" {
				i--
			}
			closeFrom(i)
		} else if len(headEnd) > 0 {
			out.WriteString("<head>")
			out.Write(headEnd)
			out.WriteString("</head>")
			headEnd = nil
		}
	}

	err = eachToken(data, func(z *html.Tokenizer, tt html.TokenType) {
		raw := z.Raw()
		isTag := tt == html.StartTagToken || tt == html.SelfClosingTagToken
		var name string
		var attrs map[string]string
		if isTag {
			name, attrs = tokenAttrs(z)
		}
		if !bodyStarted && (len(stack) == 0 || stack[len(stack)-1].name == "html" || stack[len(stack)-1].name == "head") &&
			(isTag && name != "html" && name != "head" && name != "body" && !headElements[name] ||
				tt == html.TextToken && len(bytes.TrimSpace(raw)) > 0) {
			// The content of a document without a body start tag starts.
			beginBody()
			if wrapBody {
				out.WriteString(bookWrappersStart)
			}
			stack = append(stack, open{name: "body", body: true})
			openCount["body"]++
			inBody++
		}
		spans := inBody > 0 && noSpans == 0 && inKoboSpans == 0
		//nolint:exhaustive // Every other token is copied as it was.
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			if name == "head" && tt == html.SelfClosingTagToken && len(headEnd) > 0 {
				// The style and script need the head's end tag.
				out.Write(bytes.TrimRight(bytes.TrimSuffix(raw, []byte("/>")), " \t\n\r\f"))
				out.WriteString(">")
				out.Write(headEnd)
				out.WriteString("</head>")
				headEnd = nil
				return
			}
			startsBody := name == "body" && !bodyStarted
			if startsBody {
				beginBody()
			}
			switch {
			case spans && name == "img":
				counter.incrementParagraph()
				writeKoboSpan(&out, counter, raw)
			default:
				if spans && paragraphElements[name] {
					counter.markParagraphBoundary()
				}
				out.Write(raw)
			}
			if tt == html.SelfClosingTagToken || voidElements[name] {
				return
			}
			o := open{name: name, body: startsBody, koboSpan: isKoboSpan(name, attrs)}
			if o.body {
				inBody++
				if wrapBody {
					out.WriteString(bookWrappersStart)
				}
			}
			if noSpanElements[name] {
				noSpans++
			}
			if o.koboSpan {
				inKoboSpans++
			}
			stack = append(stack, o)
			openCount[name]++
		case html.EndTagToken:
			nameBytes, _ := z.TagName()
			name := string(nameBytes)
			if openCount[name] == 0 {
				// An end tag with no start tag is copied as it was.
				out.Write(raw)
				return
			}
			i := len(stack) - 1
			for stack[i].name != name {
				i--
			}
			closeFrom(i)
			out.Write(raw)
		case html.TextToken:
			if spans && !bytes.HasPrefix(raw, []byte("<![CDATA[")) {
				writeTextSpans(&out, counter, raw)
			} else {
				out.Write(raw)
			}
		default:
			out.Write(raw)
		}
	})
	if err != nil {
		return nil, err
	}
	// Whatever is still open ends with the document.
	closeFrom(0)
	return out.Bytes(), nil
}

// koboMarkup is the Kobo markup a content document already has.
type koboMarkup struct {
	wrappers, styleHacks, script bool
	// koboIDs is set when an element has an id of the form Kobo's spans
	// get, and lastParagraph is the highest paragraph number among them.
	koboIDs       bool
	lastParagraph int
}

// findKoboMarkup returns the Kobo markup data already has.
func findKoboMarkup(data []byte) (koboMarkup, error) {
	var has koboMarkup
	err := eachToken(data, func(z *html.Tokenizer, tt html.TokenType) {
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			return
		}
		name, attrs := tokenAttrs(z)
		switch {
		case isBookWrapper(name, attrs):
			has.wrappers = true
		case isKoboStyleHacks(name, attrs):
			has.styleHacks = true
		case isKoboScript(name, attrs):
			has.script = true
		}
		if id := attrs["id"]; koboSpanIDPattern.MatchString(id) {
			paragraph, err := strconv.Atoi(id[len("kobo."):strings.LastIndexByte(id, '.')])
			if err == nil {
				has.koboIDs = true
				has.lastParagraph = max(has.lastParagraph, paragraph)
			}
		}
	})
	return has, err
}

// writeKoboSpan writes content to out inside a koboSpan with the counter's
// next id.
func writeKoboSpan(out *bytes.Buffer, counter *SpanCounter, content []byte) {
	out.WriteString(koboSpanStart)
	out.WriteString(counter.nextID())
	out.WriteString(koboSpanStartClose)
	out.Write(content)
	out.WriteString(koboSpanEnd)
}

// charRefPattern matches what the HTML tokenizer may read as a character
// reference where an & starts one: a numeric reference, or a run of
// letters and digits that may begin with a named one, either with an
// optional semicolon.
var charRefPattern = regexp.MustCompile(`^&(?:#[xX][0-9a-fA-F]*;?|#[0-9]*;?|[A-Za-z0-9]*;?)`)

// writeTextSpans writes the raw text of a text token to out with each
// segment splitIntoSegments finds wrapped in a koboSpan, unless it is
// whitespace.
//
// Segments are found in the decoded text, as the reader sees it, but
// copied from raw, so entities stay as they were.
func writeTextSpans(out *bytes.Buffer, counter *SpanCounter, raw []byte) {
	decoded, rawAt := decodeText(raw)
	start := 0
	for _, segment := range splitIntoSegments(decoded) {
		end := start + len(segment)
		if rawSegment := raw[rawAt[start]:rawAt[end]]; len(rawSegment) > 0 {
			if shouldWrapText(segment) {
				writeKoboSpan(out, counter, rawSegment)
			} else {
				out.Write(rawSegment)
			}
		}
		start = end
	}
}

// decodeText decodes the raw text of a text token as the tokenizer does,
// and returns, for each offset in the decoded text and its end, the offset
// in raw that a segment boundary there maps to.
//
// raw is decoded a piece at a time: a character reference, a CR or CRLF
// (which the tokenizer reads as LF), or a single byte. A boundary maps to
// the start of the raw piece it falls on, or, inside a piece that decodes
// to more than one byte, to that piece's end, so a reference is never
// split.
func decodeText(raw []byte) (string, []int) {
	var decoded []byte
	rawAt := make([]int, 0, len(raw)+1)
	for i := 0; i < len(raw); {
		n, piece := 1, raw[i:i+1]
		switch raw[i] {
		case '&':
			n = len(charRefPattern.Find(raw[i:]))
			piece = []byte(html.UnescapeString(string(raw[i : i+n])))
		case '\r':
			if i+1 < len(raw) && raw[i+1] == '\n' {
				n = 2
			}
			piece = []byte{'\n'}
		}
		rawAt = append(rawAt, i)
		for range len(piece) - 1 {
			rawAt = append(rawAt, i+n)
		}
		decoded = append(decoded, piece...)
		i += n
	}
	return string(decoded), append(rawAt, len(raw))
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

// continueAfter makes the next span start a paragraph after paragraph,
// unless the counter is past it already, so no new id is one a document
// already has.
func (c *SpanCounter) continueAfter(paragraph int) {
	if c.paragraph <= paragraph {
		c.paragraph = paragraph
		c.sentence = 0
		c.incParaNext = true
	}
}

// incrementParagraph immediately increments the paragraph counter.
// This is called for elements like images that should always start a new paragraph.
func (c *SpanCounter) incrementParagraph() {
	c.paragraph++
	c.sentence = 0
	c.incParaNext = false
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

// voidElements are the HTML elements that never have an end tag.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "param": true, "source": true, "track": true, "wbr": true,
}
