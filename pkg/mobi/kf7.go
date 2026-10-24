package mobi

import (
	"bytes"
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// A MOBI6 (KF7) book's text is one HTML stream written for Mobipocket
// readers, not XHTML: attribute values without quotes, void elements with
// end tags, self-closed elements that HTML reads as open, presentational
// attributes, font elements, and Mobipocket's own elements, such as
// mbp:pagebreak between chapters. Links name places by their byte offset in
// the text (filepos), and so do the table of contents and the guide; images
// are named by resource number (recindex).
//
// The text is read in four steps. An empty anchor with an id is put at each
// offset something names, while the offsets are still the file's. The text
// is then parsed as HTML, which, unlike KF8 content, it is: Go's HTML parser
// closes what is left open and nests what is misnested, which a rewrite of
// the tokens could not do. The tree is cleaned into elements and attributes
// XHTML allows, with presentation moved into styles, and cut into a
// document at each page break between chapters.

// readKF7 reads the MOBI6 book ref of a file.
func readKF7(ctx context.Context, db *pdb, b *book, ref bookRef) (*Content, error) {
	h := ref.header
	rec, err := db.record(ref.base, maxHeaderRecord)
	if err != nil {
		return nil, err
	}
	text, err := readText(ctx, db, ref.base, rec, h.compression)
	if err != nil {
		return nil, err
	}
	res, err := scanResources(db, b.firstImage)
	if err != nil {
		return nil, err
	}
	k := &Content{Metadata: b.metadata(), db: db, hrefs: map[string]string{}}

	places := fileposPlaces(text)
	tocs := readTOCs(db, []bookRef{ref})
	locators := map[string]int{}
	if len(tocs) > 0 {
		var walk func([]*TOCEntry)
		walk = func(entries []*TOCEntry) {
			for _, e := range entries {
				if pos, ok := fileposLocator(e.Locator); ok {
					places[pos] = true
					locators[strings.ToUpper(e.Locator)] = pos
				}
				walk(e.Children)
			}
		}
		walk(tocs[0].Entries)
	}
	text = toUTF8(withFileposAnchors(text, places, h.encoding == encodingUTF8), h)

	used := map[int]bool{}
	tooLarge := ""
	image := func(n uint64) string {
		r, ok := res.byEmbed(n)
		if !ok {
			return ""
		}
		if r.Size > maxImageRecord {
			tooLarge = r.Name
		}
		used[r.record] = true
		return r.Name
	}
	docs, ids, err := kf7Documents(text, k.Metadata.Title, image)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, unsupported("it has no text")
	}
	for i, doc := range docs {
		k.Documents = append(k.Documents, Document{Name: partName(i), MediaType: "application/xhtml+xml", Content: doc})
	}
	for locator, pos := range locators {
		if part, ok := ids[fileposID(pos)]; ok {
			k.hrefs[locator] = partName(part) + "#" + fileposID(pos)
		}
	}

	if n, ok := b.coverEmbed(); ok {
		if name := image(n); name != "" {
			k.Cover = res.named(name)
		}
	}
	if tooLarge != "" {
		return nil, unsupported("it has an image too large to copy (" + tooLarge + ")")
	}
	for _, r := range res.all {
		if used[r.record] {
			k.Resources = append(k.Resources, r)
		}
	}
	if len(tocs) > 0 {
		k.TOC = k.navPoints(tocs[0].Entries)
	}
	return k, nil
}

// maxPlaces caps how many places a book's text may name.
const maxPlaces = 1 << 20

var fileposAttr = regexp.MustCompile(`(?i)\sfilepos\s*=\s*["']?\s*(\d{1,10})`)

// fileposPlaces returns the offsets the text's tags name with filepos
// attributes.
func fileposPlaces(text []byte) map[int]bool {
	places := map[int]bool{}
	eachStartTag(text, func(start, end int) {
		for _, m := range fileposAttr.FindAllSubmatch(text[start:end], -1) {
			if n, err := strconv.Atoi(string(m[1])); err == nil && len(places) < maxPlaces {
				places[n] = true
			}
		}
	})
	return places
}

// fileposLocator returns the offset a table of contents entry's filepos
// locator names.
func fileposLocator(locator string) (int, bool) {
	digits, ok := strings.CutPrefix(locator, "filepos:")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	return n, err == nil && n >= 0
}

func fileposID(pos int) string {
	return "filepos" + strconv.Itoa(pos)
}

// withFileposAnchors returns text with an empty anchor whose id names the
// offset at each place. A place is moved back to where an anchor can go
// without changing the text: out of a tag, a character reference, or a
// UTF-8 character, to their start, and out of a link's text, to the
// link's start, since the anchor would end the link. A place past the end
// moves to the end, and one before the body's start tag to just after it,
// as an anchor in the head would start the body there.
func withFileposAnchors(text []byte, places map[int]bool, utf8 bool) []byte {
	sorted := make([]int, 0, len(places))
	for pos := range places {
		sorted = append(sorted, pos)
	}
	slices.Sort(sorted)
	body := 0
	eachStartTag(text, func(start, end int) {
		if body == 0 && tagName(text[start:end]) == "body" {
			body = end
		}
	})
	var out bytes.Buffer
	out.Grow(len(text) + 32*len(sorted))
	// The text is scanned once, up to each place in turn, keeping where
	// the tag, the character reference, and the link the scan is in
	// started, or -1 outside one.
	last, scanned := 0, 0
	tag, ref, link, linkTag := -1, -1, -1, -1
	for _, pos := range sorted {
		at := max(min(pos, len(text)), body)
		for ; scanned < at; scanned++ {
			switch c := text[scanned]; {
			case c == '<':
				tag, ref = scanned, -1
				switch {
				case startsTag(text[scanned:], "<a"):
					linkTag = scanned
				case startsTag(text[scanned:], "</a"):
					link = -1
				}
			case c == '>' && tag >= 0:
				if linkTag == tag && text[scanned-1] != '/' {
					link = linkTag
				}
				tag = -1
			case c == '&':
				ref = scanned
			case c == ';' || isSpace(c):
				ref = -1
			}
		}
		switch {
		case tag >= 0:
			at = tag
		case ref >= 0 && at-ref <= maxCharRef:
			at = ref
		}
		if link >= 0 && at-link <= maxLinkText {
			at = link
		}
		for utf8 && at > 0 && at < len(text) && text[at]&0xC0 == 0x80 {
			at--
		}
		at = max(at, last)
		out.Write(text[last:at])
		out.WriteString(`<a id="` + fileposID(pos) + `"></a>`)
		last = at
	}
	out.Write(text[last:])
	return out.Bytes()
}

// maxCharRef and maxLinkText cap how far back a place moves to a character
// reference's or a link's start, so a stray & or a link left open does not
// move every place after it.
const (
	maxCharRef  = 32
	maxLinkText = 4096
)

// startsTag reports whether b starts with the tag name prefix, such as
// "<a" or "</a", in any letter case, followed by the end of the name.
func startsTag(b []byte, prefix string) bool {
	return len(b) > len(prefix) && asciiEqualFold(b[:len(prefix)], prefix) &&
		(isSpace(b[len(prefix)]) || b[len(prefix)] == '>' || b[len(prefix)] == '/')
}

// maxKF7Tags caps the elements of a MOBI6 book's text, which is parsed into
// a tree held whole. The longest real books have a few hundred thousand.
const maxKF7Tags = 2 << 20

// kf7Documents turns a MOBI6 book's HTML into XHTML documents, one for each
// part the page breaks between chapters cut it into, and returns which
// document each id is in. image returns the file name of the image a
// recindex names, or "" when there is none; a nil image names none.
func kf7Documents(text []byte, title string, image func(n uint64) string) ([][]byte, map[string]int, error) {
	if image == nil {
		image = func(uint64) string { return "" }
	}
	tokens, tags := kf7Tokens(text)
	if tags > maxKF7Tags {
		return nil, nil, unsupported("its text has too many elements to convert")
	}
	// The parser fails only on elements nested deeper than it allows,
	// which also bounds how deep the tree is cleaned and written
	// recursively.
	root, err := html.Parse(bytes.NewReader(tokens))
	if err != nil {
		return nil, nil, unsupported("its text is nested too deeply to convert")
	}
	c := &kf7Cleaner{image: image, ids: map[string]string{}}
	var head, body *html.Node
	for n := range root.Descendants() {
		switch {
		case n.Type != html.ElementNode:
		case n.Data == "head" && head == nil:
			head = n
		case n.Data == "body" && body == nil:
			body = n
		}
	}
	var styles []string
	if head != nil {
		for n := range head.Descendants() {
			if n.Type == html.ElementNode && n.Data == "style" && n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				styles = append(styles, n.FirstChild.Data)
			}
		}
	}
	if body == nil {
		return nil, nil, nil
	}
	hoistPageBreaks(body)
	c.clean(body)

	parts := splitAtPageBreaks(body)
	ids := map[string]int{}
	for i, part := range parts {
		eachNode(part, func(n *html.Node) {
			if id := attr(n, "id"); id != "" {
				ids[id] = i
			}
		})
	}
	docs := make([][]byte, 0, len(parts))
	for _, part := range parts {
		eachNode(part, func(n *html.Node) { resolveLink(n, ids, c.ids) })
		docs = append(docs, xhtmlDocument(title, styles, part))
	}
	return docs, ids, nil
}

// kf7Tokens returns the text with what HTML parses differently from the way
// Mobipocket readers read it rewritten: a self-closed element other than a
// void one, which HTML leaves open, becomes a start and an end tag, and an
// end tag of a void element, which HTML reads as another (a </br> is a
// second line break), is dropped. A doctype comes first, so the parser
// does not read the text the way old browsers did, which puts tables inside
// paragraphs. It also returns how many tags start elements.
func kf7Tokens(text []byte) ([]byte, int) {
	var out bytes.Buffer
	out.Grow(len(text) + 64)
	out.WriteString("<!DOCTYPE html>")
	z := html.NewTokenizer(bytes.NewReader(text))
	tags := 0
	for {
		tt := z.Next()
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			tags++
		}
		switch tt {
		case html.ErrorToken:
			return out.Bytes(), tags
		case html.DoctypeToken:
			continue
		case html.SelfClosingTagToken:
			name, _ := z.TagName()
			raw := z.Raw()
			// A self-closed raw text element, such as <title/>, would
			// otherwise read everything after it as its text.
			z.NextIsNotRawText()
			if voidElements[string(name)] {
				out.Write(raw)
				continue
			}
			open := bytes.TrimRight(bytes.TrimSuffix(raw, []byte(">")), "/ \t\r\n\f")
			out.Write(open)
			out.WriteString("></" + string(name) + ">")
		case html.EndTagToken:
			name, _ := z.TagName()
			if !voidElements[string(name)] {
				out.Write(z.Raw())
			}
		case html.TextToken, html.StartTagToken, html.CommentToken:
			out.Write(z.Raw())
		}
	}
}

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "param": true, "source": true, "track": true, "wbr": true,
}

func attr(n *html.Node, key string) string {
	if n.Type != html.ElementNode {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == key && a.Namespace == "" {
			return a.Val
		}
	}
	return ""
}

func setAttr(n *html.Node, key, val string) {
	for i, a := range n.Attr {
		if a.Key == key && a.Namespace == "" {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

func removeAttr(n *html.Node, key string) {
	n.Attr = slices.DeleteFunc(n.Attr, func(a html.Attribute) bool { return a.Key == key })
}

// resolveLink points a link to a place in the book at the document the
// place is in, and drops a link to no place. renamed maps an id as written
// to the id it became.
func resolveLink(n *html.Node, ids map[string]int, renamed map[string]string) {
	if n.Type != html.ElementNode || n.Data != "a" {
		return
	}
	href := attr(n, "href")
	id, ok := strings.CutPrefix(href, "#")
	if !ok {
		return
	}
	if to, ok := renamed[id]; ok {
		id = to
	}
	part, ok := ids[id]
	if !ok {
		removeAttr(n, "href")
		return
	}
	setAttr(n, "href", partName(part)+"#"+id)
}

// splitAtPageBreaks cuts the body's content into parts at the page breaks
// among its children, dropping the page breaks. Empty anchors just before a
// page break name the start of the next part, so they move into it. A part
// with no text or image joins the part before it, or the one after when it
// is first.
func splitAtPageBreaks(body *html.Node) [][]*html.Node {
	var parts [][]*html.Node
	var current []*html.Node
	cut := func() {
		split := len(current)
		for split > 0 && isPlace(current[split-1]) {
			split--
		}
		part, carried := current[:split], slices.Clone(current[split:])
		switch {
		case hasContent(part):
			parts = append(parts, part)
		case len(parts) > 0:
			parts[len(parts)-1] = append(parts[len(parts)-1], part...)
		default:
			carried = append(part, carried...)
		}
		current = carried
	}
	for n := body.FirstChild; n != nil; {
		next := n.NextSibling
		body.RemoveChild(n)
		if isPageBreak(n) {
			cut()
		} else {
			current = append(current, n)
		}
		n = next
	}
	switch {
	case hasContent(current) || len(parts) == 0 && len(current) > 0:
		parts = append(parts, current)
	case len(parts) > 0:
		parts[len(parts)-1] = append(parts[len(parts)-1], current...)
	}
	if len(parts) == 1 && !hasContent(parts[0]) {
		return nil
	}
	return parts
}

const pageBreak = "mbp:pagebreak"

// pageBreakWrappers are the elements a page break between chapters may be
// inside, which old books wrap their whole text in.
var pageBreakWrappers = setOf("b", "big", "blockquote", "center", "div", "em", "font", "i", "p", "section", "small", "span", "strong", "u")

// hoistPageBreaks moves each page break inside only wrapper elements up to
// the body, splitting each wrapper in two around it, so it cuts the text
// into parts. The second half of a wrapper has its attributes but its id.
func hoistPageBreaks(body *html.Node) {
	var breaks []*html.Node
	for n := range body.Descendants() {
		if isPageBreak(n) && n.Parent != body {
			breaks = append(breaks, n)
		}
	}
	for _, pb := range breaks {
		inWrappers := true
		for p := pb.Parent; p != body && inWrappers; p = p.Parent {
			inWrappers = p.Type == html.ElementNode && pageBreakWrappers[p.Data]
		}
		if !inWrappers {
			continue
		}
		for pb.Parent != body {
			parent := pb.Parent
			if pb.NextSibling != nil {
				rest := &html.Node{Type: html.ElementNode, Data: parent.Data, DataAtom: parent.DataAtom}
				rest.Attr = slices.DeleteFunc(slices.Clone(parent.Attr), func(a html.Attribute) bool { return a.Key == "id" })
				for c := pb.NextSibling; c != nil; {
					next := c.NextSibling
					parent.RemoveChild(c)
					rest.AppendChild(c)
					c = next
				}
				parent.Parent.InsertBefore(rest, parent.NextSibling)
			}
			parent.RemoveChild(pb)
			parent.Parent.InsertBefore(pb, parent.NextSibling)
		}
	}
}

func isPageBreak(n *html.Node) bool {
	return n.Type == html.ElementNode && n.Data == pageBreak
}

// isPlace reports whether n is an empty anchor with an id, or whitespace.
func isPlace(n *html.Node) bool {
	if n.Type == html.TextNode {
		return strings.TrimSpace(n.Data) == ""
	}
	return n.Type == html.ElementNode && n.Data == "a" && n.FirstChild == nil && attr(n, "id") != ""
}

// hasContent reports whether nodes hold text or an image.
func hasContent(nodes []*html.Node) bool {
	found := false
	eachNode(nodes, func(n *html.Node) {
		found = found || n.Type == html.TextNode && strings.TrimSpace(n.Data) != "" || n.Type == html.ElementNode && n.Data == "img"
	})
	return found
}

// eachNode calls fn with each of nodes and their descendants, in order.
func eachNode(nodes []*html.Node, fn func(*html.Node)) {
	for _, n := range nodes {
		fn(n)
		for d := range n.Descendants() {
			fn(d)
		}
	}
}

// xhtmlDocument returns an XHTML document holding nodes as its body.
func xhtmlDocument(title string, styles []string, nodes []*html.Node) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n" + `<!DOCTYPE html>` + "\n")
	b.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml"><head><title>`)
	writeText(&b, title, false)
	b.WriteString(`</title>`)
	for _, s := range styles {
		b.WriteString(`<style type="text/css">`)
		writeText(&b, s, true)
		b.WriteString(`</style>`)
	}
	b.WriteString(`</head><body>`)
	for _, n := range nodes {
		writeNode(&b, n)
	}
	b.WriteString(`</body></html>`)
	return b.Bytes()
}

func writeNode(b *bytes.Buffer, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		writeText(b, n.Data, false)
	case html.ElementNode:
		b.WriteString("<" + n.Data)
		for _, a := range n.Attr {
			b.WriteString(" " + a.Key + `="`)
			writeText(b, a.Val, false)
			b.WriteString(`"`)
		}
		if voidElements[n.Data] {
			b.WriteString("/>")
			return
		}
		b.WriteString(">")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			writeNode(b, c)
		}
		b.WriteString("</" + n.Data + ">")
	case html.ErrorNode, html.DocumentNode, html.CommentNode, html.DoctypeNode, html.RawNode:
	}
}

// writeText writes s escaped for XML, without the characters XML forbids.
// Style text keeps its > signs, which child selectors use.
func writeText(b *bytes.Buffer, s string, style bool) {
	for _, r := range strings.ToValidUTF8(s, "\uFFFD") {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>' && !style:
			b.WriteString("&gt;")
		case r == '"' && !style:
			b.WriteString("&quot;")
		case r < 0x20 && r != '\t' && r != '\n' && r != '\r', r == 0xFFFE, r == 0xFFFF:
		default:
			b.WriteRune(r)
		}
	}
}
