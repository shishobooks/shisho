package mobi

import (
	"bytes"
	"fmt"
	"html"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Laying out a document for a KF8 book: its image, font, and stylesheet
// references become kindle: references, its links into the book
// placeholders for kindle:pos references, every element in its body gets
// an aid, and its body is cut into fragments.

// xnode is a node of a document's body: an element, or a run of text, a
// comment, or anything else that is not an element.
type xnode struct {
	name       string
	start, end int
	aid        string
	element    bool
	children   []*xnode
}

// document lays out one document: references rewritten, aids added, and
// its body cut into fragments.
func (l *kf8Layout) document(d Document) *kf8Doc {
	src := d.Content
	doc := &kf8Doc{ids: map[string]int{}}
	var out bytes.Buffer
	out.Grow(len(src) + len(src)/8)
	root := &xnode{element: true}
	stack := []*xnode{root}
	var body *xnode
	bodyContent := -1
	inBody := false
	leaf := func(start int) {
		if out.Len() > start {
			top := stack[len(stack)-1]
			top.children = append(top.children, &xnode{start: start, end: out.Len()})
		}
	}
	// skipTo copies src from i through the end of the first sep after it.
	skipTo := func(i int, sep string) int {
		end := bytes.Index(src[i:], []byte(sep))
		if end < 0 {
			end = len(src)
		} else {
			end += i + len(sep)
		}
		start := out.Len()
		out.Write(src[i:end])
		leaf(start)
		return end
	}

	for i := 0; i < len(src); {
		if src[i] != '<' {
			end := bytes.IndexByte(src[i+1:], '<')
			if end < 0 {
				end = len(src)
			} else {
				end += i + 1
			}
			start := out.Len()
			out.Write(src[i:end])
			leaf(start)
			i = end
			continue
		}
		switch {
		case bytes.HasPrefix(src[i:], []byte("<!--")):
			i = skipTo(i, "-->")
		case bytes.HasPrefix(src[i:], []byte("<![CDATA[")):
			i = skipTo(i, "]]>")
		case bytes.HasPrefix(src[i:], []byte("<?")):
			i = skipTo(i, "?>")
		case bytes.HasPrefix(src[i:], []byte("<!")):
			i = skipTo(i, ">")
		case i+2 < len(src) && src[i+1] == '/' && isStartTag(src, i+1):
			end := tagEnd(src, i+1)
			name := tagName(src[i+1 : end])
			k := len(stack) - 1
			for k > 0 && stack[k].name != name {
				k--
			}
			start := out.Len()
			out.Write(src[i:end])
			if k == 0 {
				leaf(start)
				i = end
				continue
			}
			// Elements left open inside the one closed end where it does.
			for _, open := range stack[k+1:] {
				open.end = start
			}
			stack[k].end = out.Len()
			for _, closed := range stack[k:] {
				if closed == body {
					inBody = false
				}
			}
			stack = stack[:k]
			i = end
		case isStartTag(src, i):
			end := tagEnd(src, i)
			raw := src[i:end]
			name := tagName(raw)
			tag, links := l.editTag(d.Name, name, raw)
			n := &xnode{name: name, element: true}
			if name == "body" && body == nil {
				body, inBody = n, true
			}
			if inBody {
				n.aid = strings.ToUpper(strconv.FormatUint(uint64(l.nextAid), 32)) //nolint:gosec // a count
				l.nextAid++
				// The aid goes in before every attribute, so it moves each
				// placeholder by its length.
				before := len(tag)
				tag = withAid(tag, n.aid)
				for k := range links {
					links[k].at += len(tag) - before
				}
			}
			selfClosed := bytes.HasSuffix(raw, []byte("/>"))
			if selfClosed && !voidElements[name] {
				// A Kindle reads the document as HTML, where "<a/>" opens an
				// element that takes in what follows it, so it gets an end
				// tag.
				// tag may still be src's bytes, so it is copied first.
				tag = append(bytes.Clone(bytes.TrimRight(tag[:len(tag)-2], " \t\n\r\f")), '>')
				tag = append(tag, "</"+string(raw[1:nameEnd(raw)])+">"...)
			}
			n.start = out.Len()
			for _, k := range links {
				k.at += n.start
				doc.links = append(doc.links, k)
			}
			out.Write(tag)
			if id := tagID(tag); id != "" {
				if _, taken := doc.ids[id]; !taken {
					doc.ids[id] = n.start
				}
			}
			top := stack[len(stack)-1]
			top.children = append(top.children, n)
			i = end
			switch {
			// A void element written without "/>" still has no content.
			case selfClosed || voidElements[name]:
				n.end = out.Len()
				if n == body {
					inBody = false
					bodyContent = n.end
				}
			case name == "script" || name == "style":
				// Their content is text, even where it looks like markup.
				closeAt := indexFold(src[i:], "</"+name)
				if closeAt < 0 {
					closeAt = len(src) - i
				}
				content := src[i : i+closeAt]
				if name == "style" {
					content = l.rewriteCSS(d.Name, content)
				}
				out.Write(content)
				i += closeAt
				if i < len(src) {
					closeEnd := tagEnd(src, i+1)
					out.Write(src[i:closeEnd])
					i = closeEnd
				}
				n.end = out.Len()
			default:
				stack = append(stack, n)
				if n == body {
					bodyContent = out.Len()
				}
			}
		default:
			start := out.Len()
			out.WriteByte('<')
			leaf(start)
			i++
		}
	}
	for _, open := range stack[1:] {
		open.end = out.Len()
	}
	doc.out = out.Bytes()

	if body == nil {
		// No element has an aid for the selector to name.
		doc.frags = []fragRange{{start: len(doc.out), end: len(doc.out)}}
	} else {
		fragmentChildren(body, &doc.frags)
		if len(doc.frags) == 0 {
			doc.frags = []fragRange{{start: bodyContent, end: bodyContent, parent: body.aid}}
		}
	}
	doc.first = doc.frags[0].start
	if body != nil {
		for _, c := range body.children {
			if c.element {
				doc.first = c.start
				break
			}
		}
	}
	return doc
}

// fragmentChildren cuts n's content into fragments: runs of its children of
// up to fragmentSize bytes together, except that a larger element is cut
// into fragments inside it, its own tags staying in the skeleton.
func fragmentChildren(n *xnode, frags *[]fragRange) {
	var cur *fragRange
	for _, c := range n.children {
		size := c.end - c.start
		if c.element && size > fragmentSize && len(c.children) > 0 {
			cur = nil
			fragmentChildren(c, frags)
			continue
		}
		if cur == nil || cur.end-cur.start+size > fragmentSize {
			*frags = append(*frags, fragRange{start: c.start, end: c.end, parent: n.aid})
			cur = &(*frags)[len(*frags)-1]
			continue
		}
		cur.end = c.end
	}
}

// indexFold returns where s first appears in b, comparing ASCII letters in
// either case, or -1.
func indexFold(b []byte, s string) int {
	for i := 0; i+len(s) <= len(b); i++ {
		if asciiEqualFold(b[i:i+len(s)], s) {
			return i
		}
	}
	return -1
}

// withAid returns a start tag with an aid attribute right after its name.
func withAid(tag []byte, aid string) []byte {
	end := nameEnd(tag)
	out := make([]byte, 0, len(tag)+len(aid)+8)
	out = append(out, tag[:end]...)
	out = append(out, ` aid="`...)
	out = append(out, aid...)
	out = append(out, '"')
	return append(out, tag[end:]...)
}

// tagAttr is an attribute of a start tag: its name, lowercased, and where
// its value sits in the tag, or -1 for one with no value. start includes
// the whitespace before the attribute.
type tagAttr struct {
	name                 string
	start, end           int
	valueStart, valueEnd int
}

// tagAttrs returns the attributes of a start tag.
func tagAttrs(tag []byte) []tagAttr {
	var attrs []tagAttr
	i := nameEnd(tag)
	for i < len(tag) {
		start := i
		for i < len(tag) && isSpace(tag[i]) {
			i++
		}
		nameStart := i
		for i < len(tag) && !isSpace(tag[i]) && tag[i] != '=' && tag[i] != '>' && tag[i] != '/' {
			i++
		}
		if i == nameStart {
			i++
			continue
		}
		a := tagAttr{name: strings.ToLower(string(tag[nameStart:i])), start: start, valueStart: -1, valueEnd: -1}
		j := i
		for j < len(tag) && isSpace(tag[j]) {
			j++
		}
		if j < len(tag) && tag[j] == '=' {
			j++
			for j < len(tag) && isSpace(tag[j]) {
				j++
			}
			if j < len(tag) && (tag[j] == '"' || tag[j] == '\'') {
				quote := tag[j]
				end := bytes.IndexByte(tag[j+1:], quote)
				if end < 0 {
					end = len(tag) - j - 1
				}
				a.valueStart, a.valueEnd = j+1, j+1+end
				i = min(len(tag), j+2+end)
			} else {
				a.valueStart = j
				// An unquoted value ends before the "/>" closing the tag.
				for j < len(tag) && !isSpace(tag[j]) && tag[j] != '>' && !bytes.HasPrefix(tag[j:], []byte("/>")) {
					j++
				}
				a.valueEnd, i = j, j
			}
		}
		a.end = i
		attrs = append(attrs, a)
	}
	return attrs
}

// editTag returns a start tag with its image and stylesheet references
// rewritten to kindle: references, its links into the book replaced by
// linkPlaceholder, and any aid attribute removed. It also returns the
// links, each at its placeholder in the tag returned.
func (l *kf8Layout) editTag(from, name string, tag []byte) ([]byte, []kf8Link) {
	attrs := tagAttrs(tag)
	var out []byte
	var links []kf8Link
	last := 0
	for _, a := range attrs {
		var value []byte
		switch {
		case a.name == "aid":
			out = append(out, tag[last:a.start]...)
			last = a.end
			continue
		case a.valueStart < 0:
			continue
		case a.name == "src" || (name == "image" && (a.name == "href" || a.name == "xlink:href")):
			value = l.embedRef(from, string(tag[a.valueStart:a.valueEnd]))
		case name == "link" && a.name == "href":
			value = l.flowRef(from, string(tag[a.valueStart:a.valueEnd]))
		case (name == "a" || name == "area") && (a.name == "href" || a.name == "xlink:href"):
			if target, id, ok := l.linkTarget(from, string(tag[a.valueStart:a.valueEnd])); ok {
				value = []byte(linkPlaceholder)
				links = append(links, kf8Link{at: len(out) + a.valueStart - last, name: target, id: id})
			}
		case a.name == "style":
			if v := l.rewriteCSS(from, tag[a.valueStart:a.valueEnd]); !bytes.Equal(v, tag[a.valueStart:a.valueEnd]) {
				value = v
			}
		}
		if value == nil {
			continue
		}
		out = append(out, tag[last:a.valueStart]...)
		out = append(out, value...)
		last = a.valueEnd
	}
	if out == nil {
		return tag, nil
	}
	return append(out, tag[last:]...), links
}

// linkTarget returns the document and id a link from the document at from
// names, or false when it names no document of the book. The id is "" for
// a link to a whole document.
func (l *kf8Layout) linkTarget(from, ref string) (string, string, bool) {
	ref = strings.TrimSpace(html.UnescapeString(ref))
	u, err := url.Parse(ref)
	if err != nil || u.Scheme != "" || u.Host != "" || strings.HasPrefix(u.Path, "/") || ref == "" {
		return "", "", false
	}
	name := from
	if u.Path != "" {
		name = path.Join(path.Dir(from), u.Path)
	}
	if _, ok := l.byName[name]; !ok {
		return "", "", false
	}
	return name, u.Fragment, true
}

// embedRef returns the kindle:embed reference for an image reference from
// the document at from, or nil when it names no image of the book.
func (l *kf8Layout) embedRef(from, ref string) []byte {
	name, ok := resolveRef(from, html.UnescapeString(ref))
	if !ok {
		return nil
	}
	n, ok := l.embeds[name]
	if !ok {
		return nil
	}
	return []byte(kf8EmbedURI(u32(n-1)) + "?mime=" + l.resources[n-1].mediaType)
}

// flowRef returns the kindle:flow reference for a stylesheet reference, or
// nil when it names no stylesheet of the book.
func (l *kf8Layout) flowRef(from, ref string) []byte {
	name, ok := resolveRef(from, html.UnescapeString(ref))
	if !ok {
		return nil
	}
	n, ok := l.flows[name]
	if !ok {
		return nil
	}
	return []byte(fmt.Sprintf("kindle:flow:%04s?mime=text/css", base32(u32(n))))
}

// fontRef returns the kindle:embed reference for a font reference, or nil
// when it names no font the book stores.
func (l *kf8Layout) fontRef(from, ref string) []byte {
	name, ok := resolveRef(from, ref)
	if !ok {
		return nil
	}
	n, ok := l.fonts[name]
	if !ok {
		return nil
	}
	return []byte(kf8EmbedURI(u32(n - 1)))
}

var (
	// cssURL matches a url() and its reference, in double quotes (group 1),
	// single quotes (2), the &quot; of a style attribute (3), or none (4).
	cssURL = regexp.MustCompile(`url\(\s*(?:"([^"]*)"|'([^']*)'|&quot;(.*?)&quot;|([^'")\s]*))\s*\)`)
	// cssImport matches an @import that names its stylesheet with a string
	// rather than url().
	cssImport = regexp.MustCompile(`(@import\s+)(?:"([^"]*)"|'([^']*)')`)
)

// rewriteCSS returns CSS from the stylesheet or document at from with
// each url() naming an image or a font of the book turned into a
// kindle:embed reference, and each naming a stylesheet, as @import does,
// into a kindle:flow reference. A Kindle reads a kindle: reference only
// unquoted, as KindleGen writes it, so a rewritten url() loses its quotes,
// and an @import naming its stylesheet with a string gets a url().
func (l *kf8Layout) rewriteCSS(from string, css []byte) []byte {
	css = cssURL.ReplaceAllFunc(css, func(m []byte) []byte {
		sub := cssURL.FindSubmatch(m)
		ref := string(bytes.Join(sub[1:], nil))
		value := l.embedRef(from, ref)
		if value == nil {
			value = l.fontRef(from, ref)
		}
		if value == nil {
			value = l.flowRef(from, ref)
		}
		if value == nil {
			return m
		}
		return slices.Concat([]byte("url("), value, []byte(")"))
	})
	return cssImport.ReplaceAllFunc(css, func(m []byte) []byte {
		sub := cssImport.FindSubmatch(m)
		value := l.flowRef(from, string(sub[2])+string(sub[3]))
		if value == nil {
			return m
		}
		return slices.Concat(sub[1], []byte("url("), value, []byte(")"))
	})
}

// resolveRef returns the name a relative reference in the file at from
// names, and
// false for a reference to another place, such as a URL or a fragment of
// the same document.
func resolveRef(from, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "#") {
		return "", false
	}
	u, err := url.Parse(ref)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" || strings.HasPrefix(u.Path, "/") {
		return "", false
	}
	return path.Join(path.Dir(from), u.Path), true
}
