package reflowable

import (
	"bytes"
	"slices"
	"unicode/utf8"

	"github.com/shishobooks/shisho/pkg/kepub"
	"golang.org/x/net/html"
)

// node is an element or a text node of a content document, with no
// comments: each reader keeps a different subset of the rest, so a form
// works out its own view of the tree.
type node struct {
	// name is an element's lowercase tag name, and empty for text.
	name     string
	attrs    map[string]string
	parent   *node
	children []*node
	// text is a text node's text, decoded, as the browser's DOM holds it.
	text []rune
	// separate marks a text node crengine keeps apart from the text before
	// it: CDATA, and text after a comment or CDATA. The browser keeps them
	// as nodes too, but a CFI counts them as one stretch of text.
	separate bool
	// start and end are where the node's text starts and ends in its
	// document's body, in code points.
	start, end int
}

func (n *node) isText() bool { return n.name == "" }

func (n *node) id() string { return n.attrs["id"] }

// isKoboSpan reports whether n is one of the spans a KePub wraps text in.
func (n *node) isKoboSpan() bool { return kepub.IsKoboSpan(n.name, n.attrs) }

// isBookWrapper reports whether n is one of the divs a KePub wraps the
// body's content in.
func (n *node) isBookWrapper() bool { return kepub.IsBookWrapper(n.name, n.attrs) }

// elements returns n's element children.
func (n *node) elements() []*node {
	var out []*node
	for _, c := range n.children {
		if !c.isText() {
			out = append(out, c)
		}
	}
	return out
}

// find returns the first element under n, n included, that match accepts.
func (n *node) find(match func(*node) bool) *node {
	if !n.isText() && match(n) {
		return n
	}
	for _, c := range n.children {
		if found := c.find(match); found != nil {
			return found
		}
	}
	return nil
}

// walk calls visit on n and everything under it, in document order.
func (n *node) walk(visit func(*node)) {
	visit(n)
	for _, c := range n.children {
		c.walk(visit)
	}
}

// parse reads a content document, the OPF, or container.xml into a tree
// under a nameless root.
//
// It reads tokens through kepub.EachToken, rather than Go's HTML parser,
// which restructures XHTML that isn't valid HTML, as the browser and
// crengine don't. Sharing the KePub converter's tokenizer setup means a
// document reads the same here as when it was converted: CDATA is read as
// text, and a self-closed tag never starts raw text, so a self-closed
// script or title can't swallow what follows.
func parse(data []byte) (*node, error) {
	root := &node{name: "#document"}
	current := root
	// boundary is set after a comment or CDATA, where crengine starts a new
	// text node.
	boundary := false
	err := kepub.EachToken(data, func(z *html.Tokenizer, tt html.TokenType) {
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			boundary = false
			name, attrs := kepub.TokenAttrs(z)
			el := &node{name: name, parent: current, attrs: attrs}
			current.children = append(current.children, el)
			if tt == html.StartTagToken && !kepub.IsVoidElement(name) {
				current = el
			}
		case html.EndTagToken:
			boundary = false
			name, _ := z.TagName()
			for open := current; open != root; open = open.parent {
				if open.name == string(name) {
					current = open.parent
					break
				}
			}
		case html.TextToken:
			cdata := bytes.HasPrefix(z.Raw(), []byte("<![CDATA["))
			text := []rune(string(z.Text()))
			if len(text) > 0 {
				current.children = append(current.children, &node{parent: current, text: text, separate: cdata || boundary})
			}
			boundary = cdata
		case html.CommentToken:
			boundary = true
		case html.DoctypeToken, html.ErrorToken:
			// EachToken never passes an ErrorToken: it returns the error.
		}
	})
	if err != nil {
		return nil, err
	}
	return root, nil
}

// unwrapKobo removes a KePub's spans and wrapper divs under n, leaving their
// content in their place, as stripping a KePub to EPUB does. Text that ends
// up beside text joins it, as it would reading the stripped document.
func unwrapKobo(n *node) {
	var children []*node
	add := func(c *node) {
		c.parent = n
		if last := len(children) - 1; last >= 0 && c.isText() && children[last].isText() && !c.separate {
			joined := *children[last]
			joined.text = append(slices.Clone(joined.text), c.text...)
			children[last] = &joined
			return
		}
		children = append(children, c)
	}
	for _, c := range n.children {
		if c.isText() {
			add(c)
			continue
		}
		unwrapKobo(c)
		if c.isKoboSpan() || c.isBookWrapper() {
			for _, grandchild := range c.children {
				add(grandchild)
			}
			continue
		}
		add(c)
	}
	n.children = children
}

// measure sets where each node under n starts and ends in the text, starting
// at offset, and returns where n ends.
func measure(n *node, offset int) int {
	n.start = offset
	if n.isText() {
		offset += len(n.text)
	}
	for _, c := range n.children {
		offset = measure(c, offset)
	}
	n.end = offset
	return offset
}

// utf16Len is how many UTF-16 code units the browser counts in text.
func utf16Len(text []rune) int {
	n := 0
	for _, r := range text {
		if r >= 0x10000 && utf8.ValidRune(r) {
			n += 2
		} else {
			n++
		}
	}
	return n
}
