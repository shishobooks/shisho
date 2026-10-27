package mobi

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// kf7Cleaner turns the parsed HTML of a MOBI6 book into XHTML a reader and
// epubcheck accept. Elements XHTML has keep the attributes it allows them;
// presentational attributes and elements become styles; Mobipocket's
// elements and elements XHTML lacks give way to their content; elements
// that show nothing in a book, such as scripts and forms, go with theirs.
type kf7Cleaner struct {
	body  *html.Node
	image func(n uint64) string
	// ids maps each id as written to the id it became, and seen holds the
	// ids given out, so each is unique.
	ids  map[string]string
	seen map[string]bool
}

// keptElements are the elements that stay as they are.
var keptElements = setOf(
	"a", "abbr", "address", "article", "aside", "b", "bdi", "bdo", "blockquote", "br", "caption",
	"cite", "code", "col", "colgroup", "dd", "del", "dfn", "div", "dl", "dt", "em", "figcaption",
	"figure", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "i", "img", "ins",
	"kbd", "li", "mark", "nav", "ol", "p", "pre", "q", "rp", "rt", "ruby", "s", "samp", "section",
	"small", "span", "strong", "sub", "sup", "table", "tbody", "td", "tfoot", "th", "thead", "tr",
	"u", "ul", "var", "wbr",
)

// droppedElements go with their content.
var droppedElements = setOf(
	"applet", "audio", "base", "basefont", "button", "embed", "frame", "frameset", "guide", "head",
	"iframe", "input", "link", "meta", "noembed", "noframes", "noscript", "object", "option",
	"param", "reference", "script", "select", "source", "style", "template", "textarea", "title",
	"track", "video",
)

// renamedElements become another element, with the style they had.
var renamedElements = map[string]struct{ name, style string }{
	"acronym":   {"abbr", ""},
	"big":       {"span", "font-size: larger"},
	"center":    {"div", "text-align: center"},
	"dir":       {"ul", ""},
	"font":      {"span", ""},
	"listing":   {"pre", ""},
	"menu":      {"ul", ""},
	"nobr":      {"span", "white-space: nowrap"},
	"plaintext": {"pre", ""},
	"strike":    {"s", ""},
	"tt":        {"span", "font-family: monospace"},
	"xmp":       {"pre", ""},
}

// inlineStyles are the phrasing elements that become a div when they hold
// a block, which they may not, with the style that keeps their look.
var inlineStyles = map[string]string{
	"abbr": "", "b": "font-weight: bold", "bdi": "", "bdo": "", "cite": "font-style: italic",
	"code": "font-family: monospace", "dfn": "font-style: italic", "em": "font-style: italic",
	"i": "font-style: italic", "kbd": "font-family: monospace", "mark": "", "q": "",
	"s": "text-decoration: line-through", "samp": "font-family: monospace", "small": "font-size: smaller",
	"span": "", "strong": "font-weight: bold", "sub": "vertical-align: sub", "sup": "vertical-align: super",
	"u": "text-decoration: underline", "var": "font-style: italic",
}

var blockElements = setOf(
	"address", "article", "aside", "blockquote", "dd", "div", "dl", "dt", "figcaption", "figure",
	"footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "nav", "ol", "p", "pre",
	"section", "table", "ul",
)

// textBlocks are the elements whose width and height Mobipocket reads as a
// first-line indent and a space above.
var textBlocks = setOf("blockquote", "dd", "div", "dt", "h1", "h2", "h3", "h4", "h5", "h6", "li", "ol", "p", "ul")

var tableCells = setOf("caption", "col", "colgroup", "table", "tbody", "td", "tfoot", "th", "thead", "tr")

func setOf(names ...string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

var (
	cssLength = regexp.MustCompile(`^-?(\d+\.?\d*|\.\d+)(em|ex|pt|px|%|in|cm|mm|pc|rem)?$`)
	cssColor  = regexp.MustCompile(`^(#[0-9A-Fa-f]{3}|#[0-9A-Fa-f]{6}|[A-Za-z]{3,20})$`)
	hexColor  = regexp.MustCompile(`^([0-9A-Fa-f]{3}|[0-9A-Fa-f]{6})$`)
	cssFamily = regexp.MustCompile(`^[\w\s,'"-]{1,200}$`)
	digits    = regexp.MustCompile(`^\d{1,9}$`)
	// xmlName is an id XHTML accepts: an XML name without colons.
	xmlName     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
	notNameChar = regexp.MustCompile(`[^A-Za-z0-9_.-]`)
)

// clean cleans the body's content.
func (c *kf7Cleaner) clean(body *html.Node) {
	c.body = body
	c.seen = map[string]bool{}
	c.children(body)
}

func (c *kf7Cleaner) children(n *html.Node) {
	for child := n.FirstChild; child != nil; {
		next := child.NextSibling
		c.node(child)
		child = next
	}
}

func (c *kf7Cleaner) node(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		return
	case html.ElementNode:
	case html.ErrorNode, html.DocumentNode, html.CommentNode, html.DoctypeNode, html.RawNode:
		n.Parent.RemoveChild(n)
		return
	}
	name := n.Data
	switch {
	case n.Namespace != "" || droppedElements[name]:
		n.Parent.RemoveChild(n)
		return
	case name == pageBreak:
		// A page break between the body's children cuts a part; one
		// anywhere else cuts nothing.
		c.children(n)
		if n.Parent == c.body {
			unwrapChildren(n)
		} else {
			unwrap(n)
		}
		return
	}
	c.children(n)

	style := ""
	if r, ok := renamedElements[name]; ok {
		n.Data, style = r.name, r.style
	} else if !keptElements[name] {
		unwrap(n)
		return
	}
	c.attributes(n, name, style)

	switch {
	case n.Data == "img" && attr(n, "src") == "":
		n.Parent.RemoveChild(n)
		return
	case n.Data == "a" && len(n.Attr) == 0 && n.FirstChild == nil, n.Data == "span" && len(n.Attr) == 0:
		unwrap(n)
		return
	case n.Data == "li" && !isList(n.Parent):
		n.Data = "div"
	case n.Data == "ul" || n.Data == "ol":
		fixList(n)
	}
	if s, ok := inlineStyles[n.Data]; ok && holdsBlock(n) {
		n.Data = "div"
		setStyle(n, s)
	}
	if phrasingBlocks[n.Data] {
		unwrapBlocks(n)
	}
}

// phrasingBlocks are the blocks that may hold only phrasing content.
var phrasingBlocks = setOf("h1", "h2", "h3", "h4", "h5", "h6", "p", "pre")

// unwrapBlocks replaces each block in n with its content, which is checked
// in turn.
func unwrapBlocks(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		if c.Type != html.ElementNode || !blockElements[c.Data] {
			c = c.NextSibling
			continue
		}
		next := c.FirstChild
		if next == nil {
			next = c.NextSibling
		}
		unwrap(c)
		c = next
	}
}

// attributes keeps the attributes XHTML allows the element and turns
// presentational ones into its style. name is the element as written,
// style the style it has from that.
func (c *kf7Cleaner) attributes(n *html.Node, name, style string) {
	var decls []string
	if style != "" {
		decls = append(decls, style)
	}
	add := func(prop, val string) {
		if val != "" {
			decls = append(decls, prop+": "+val)
		}
	}
	var kept []html.Attribute
	keep := func(key, val string) { kept = append(kept, html.Attribute{Key: key, Val: val}) }
	authored := ""
	hasID, hasHref, hasAlt := false, false, false
	el := n.Data
	for _, a := range n.Attr {
		val := strings.TrimSpace(a.Val)
		if a.Namespace != "" {
			continue
		}
		switch key := a.Key; {
		case key == "id" || key == "name" && el == "a":
			if !hasID {
				if id := c.id(val); id != "" {
					keep("id", id)
					hasID = true
				}
			}
		case key == "class" || key == "title" || key == "lang":
			if val != "" {
				keep(key, val)
			}
		case key == "dir":
			if val == "ltr" || val == "rtl" || val == "auto" {
				keep(key, val)
			}
		case key == "style":
			authored = val
		case key == "filepos" && el == "a" && !hasHref:
			if n, err := strconv.Atoi(val); err == nil {
				keep("href", "#"+fileposID(n))
				hasHref = true
			}
		case key == "href" && el == "a" && !hasHref:
			if strings.HasPrefix(val, "#") || externalLink(val) {
				keep(key, val)
				hasHref = true
			}
		case (key == "recindex" || key == "hirecindex" || key == "lorecindex") && el == "img":
			if attr(n, "recindex") != "" && key != "recindex" {
				continue
			}
			if n, err := strconv.ParseUint(val, 10, 32); err == nil {
				if src := c.image(n); src != "" {
					keep("src", src)
				}
			}
		case key == "alt" && el == "img" && !hasAlt:
			keep(key, a.Val)
			hasAlt = true
		case (key == "width" || key == "height") && el == "img":
			if digits.MatchString(val) {
				keep(key, val)
			} else {
				add(key, length(val))
			}
		case key == "width" && textBlocks[el]:
			add("text-indent", length(val))
		case key == "height" && textBlocks[el]:
			add("margin-top", length(val))
		case (key == "width" || key == "height") && tableCells[el]:
			add(key, length(val))
		case key == "align" && el == "img":
			switch v := strings.ToLower(val); v {
			case "left", "right":
				add("float", v)
			case "top", "middle", "bottom", "baseline":
				add("vertical-align", v)
			}
		case key == "align" && el != "table":
			switch v := strings.ToLower(val); v {
			case "left", "right", "center", "justify":
				add("text-align", v)
			}
		case key == "valign":
			switch v := strings.ToLower(val); v {
			case "top", "middle", "bottom", "baseline":
				add("vertical-align", v)
			}
		case key == "bgcolor":
			add("background-color", cssColorOf(val))
		case key == "color" && name == "font":
			add("color", cssColorOf(val))
		case key == "size" && name == "font":
			add("font-size", fontSize(val))
		case key == "face" && name == "font":
			if cssFamily.MatchString(val) {
				add("font-family", val)
			}
		case (key == "colspan" || key == "rowspan") && (el == "td" || el == "th"),
			key == "span" && (el == "col" || el == "colgroup"),
			key == "start" && el == "ol",
			key == "value" && el == "li" && n.Parent != nil && n.Parent.Data == "ol":
			if digits.MatchString(val) {
				keep(key, val)
			}
		case key == "type" && el == "ol":
			if len(val) == 1 && strings.ContainsAny(val, "1aAiI") {
				keep(key, val)
			}
		case key == "cite" && (el == "blockquote" || el == "q"):
			if externalLink(val) {
				keep(key, val)
			}
		}
	}
	if authored != "" {
		decls = append(decls, authored)
	}
	if len(decls) > 0 {
		keep("style", strings.Join(decls, "; "))
	}
	if el == "img" && !hasAlt {
		keep("alt", "")
	}
	n.Attr = kept
}

// id returns the id an element written with id gets: the same one, made
// an XML name and unique, or "" when an element before it took it.
func (c *kf7Cleaner) id(id string) string {
	if id == "" {
		return ""
	}
	if _, taken := c.ids[id]; taken {
		return ""
	}
	base := id
	if !xmlName.MatchString(base) {
		base = "id-" + notNameChar.ReplaceAllString(base, "_")
	}
	out := base
	for n := 2; c.seen[out]; n++ {
		out = base + "-" + strconv.Itoa(n)
	}
	c.ids[id] = out
	c.seen[out] = true
	return out
}

func externalLink(href string) bool {
	lower := strings.ToLower(href)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "mailto:")
}

// length returns a CSS length for a Mobipocket width or height, which may
// have no unit, meaning pixels, or "" for one that is not a length.
func length(val string) string {
	m := cssLength.FindStringSubmatch(val)
	switch {
	case m == nil:
		return ""
	case m[2] == "" && strings.Trim(m[1], "0.") != "":
		return val + "px"
	}
	return val
}

func cssColorOf(val string) string {
	if hexColor.MatchString(val) {
		return "#" + val
	}
	if cssColor.MatchString(val) {
		return val
	}
	return ""
}

// fontSize returns the CSS size of a font element's size: 1 to 7, or
// larger or smaller than the text around it.
func fontSize(val string) string {
	sizes := []string{"x-small", "small", "medium", "large", "x-large", "xx-large", "xx-large"}
	switch {
	case strings.HasPrefix(val, "+"):
		return "larger"
	case strings.HasPrefix(val, "-"):
		return "smaller"
	}
	n, err := strconv.Atoi(val)
	if err != nil || n < 1 {
		return ""
	}
	return sizes[min(n, len(sizes))-1]
}

func setStyle(n *html.Node, style string) {
	if style == "" {
		return
	}
	if s := attr(n, "style"); s != "" {
		style += "; " + s
	}
	setAttr(n, "style", style)
}

// isList reports whether n is a list, or an element that becomes one.
func isList(n *html.Node) bool {
	return n != nil && n.Type == html.ElementNode && (n.Data == "ul" || n.Data == "ol" || n.Data == "dir" || n.Data == "menu")
}

func holdsBlock(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && blockElements[c.Data] {
			return true
		}
	}
	return false
}

// fixList gives a list only items: an empty anchor between items moves into
// the item after it, or the one before at the end, and anything else that
// is not whitespace becomes an item.
func fixList(list *html.Node) {
	for c := list.FirstChild; c != nil; {
		next := c.NextSibling
		switch {
		case isItem(c), c.Type == html.TextNode && strings.TrimSpace(c.Data) == "":
		case isPlace(c):
			item := next
			for item != nil && !isItem(item) {
				item = item.NextSibling
			}
			list.RemoveChild(c)
			if item != nil {
				item.InsertBefore(c, item.FirstChild)
			} else if last := lastItem(list); last != nil {
				last.AppendChild(c)
			} else {
				list.AppendChild(wrapItem(c))
			}
		default:
			li := &html.Node{Type: html.ElementNode, Data: "li"}
			list.InsertBefore(li, c)
			list.RemoveChild(c)
			li.AppendChild(c)
		}
		c = next
	}
}

func isItem(n *html.Node) bool {
	return n.Type == html.ElementNode && n.Data == "li"
}

func lastItem(list *html.Node) *html.Node {
	for c := list.LastChild; c != nil; c = c.PrevSibling {
		if isItem(c) {
			return c
		}
	}
	return nil
}

func wrapItem(n *html.Node) *html.Node {
	li := &html.Node{Type: html.ElementNode, Data: "li"}
	li.AppendChild(n)
	return li
}

// unwrap replaces n with its children.
func unwrap(n *html.Node) {
	unwrapChildren(n)
	n.Parent.RemoveChild(n)
}

// unwrapChildren moves n's children to after it.
func unwrapChildren(n *html.Node) {
	for c := n.LastChild; c != nil; c = n.LastChild {
		n.RemoveChild(c)
		n.Parent.InsertBefore(c, n.NextSibling)
	}
}
