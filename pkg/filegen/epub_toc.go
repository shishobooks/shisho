package filegen

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/shishobooks/shisho/pkg/models"
)

// The table of contents is rewritten by splicing bytes into the source
// document rather than by marshaling it again: encoding/xml cannot write the
// XHTML nav document back without mangling its namespaces, and splicing
// leaves everything a chapter edit does not touch exactly as written.
//
// The stored chapters are the parsed table of contents (pkg/epub nav.go)
// after the Chapters tab renamed or deleted entries. Adding and reordering
// are not supported, so a stored tree that is not the source's entries with
// some renamed or deleted leaves the document as written.

const ncxMediaType = "application/x-dtbncx+xml"

// tocPaths returns the zip entry names of the EPUB 3 navigation document and
// the NCX, or "" for the ones the package does not have.
func tocPaths(pkg *opfPackage, opfPath string) (navPath, ncxPath string) {
	for _, item := range pkg.Manifest.Items {
		properties, _ := item.Attrs.get("", "properties")
		if navPath == "" && hasField(properties, "nav") {
			navPath = opfEntryPath(opfPath, item.Href)
		}
	}
	for _, item := range pkg.Manifest.Items {
		if pkg.Spine.Toc != "" && item.ID == pkg.Spine.Toc {
			return navPath, opfEntryPath(opfPath, item.Href)
		}
	}
	for _, item := range pkg.Manifest.Items {
		if item.MediaType == ncxMediaType {
			return navPath, opfEntryPath(opfPath, item.Href)
		}
	}
	return navPath, ""
}

func hasField(s, field string) bool {
	for _, f := range strings.Fields(s) {
		if f == field {
			return true
		}
	}
	return false
}

// editedChapter is a stored chapter reduced to what the table of contents
// shows.
type editedChapter struct {
	title    string
	href     string
	children []editedChapter
}

// chapterTree returns the file's chapters as a tree. A file loaded from the
// database lists every chapter in one flat slice, nested ones included, with
// only direct children attached, so the tree is rebuilt from ParentID.
func chapterTree(chapters []*models.Chapter) []editedChapter {
	byParent := map[int][]*models.Chapter{}
	var roots []*models.Chapter
	for _, ch := range chapters {
		if ch.ParentID == nil {
			roots = append(roots, ch)
		} else {
			byParent[*ch.ParentID] = append(byParent[*ch.ParentID], ch)
		}
	}
	flat := len(byParent) > 0

	var build func(level []*models.Chapter) []editedChapter
	build = func(level []*models.Chapter) []editedChapter {
		level = append([]*models.Chapter(nil), level...)
		sort.SliceStable(level, func(i, j int) bool { return level[i].SortOrder < level[j].SortOrder })
		out := make([]editedChapter, 0, len(level))
		for _, ch := range level {
			c := editedChapter{title: ch.Title}
			if ch.Href != nil {
				c.href = *ch.Href
			}
			if flat {
				c.children = build(byParent[ch.ID])
			} else {
				c.children = build(ch.Children)
			}
			out = append(out, c)
		}
		return out
	}
	return build(roots)
}

// xmlNode is an element of a parsed document with the byte offsets of its
// tags, so it can be replaced or removed in the source bytes.
type xmlNode struct {
	name       string
	attrs      []xml.Attr
	start, end int64 // the whole element
	innerStart int64 // just after the start tag
	innerEnd   int64 // just before the end tag
	text       strings.Builder
	children   []*xmlNode
}

func (n *xmlNode) child(name string) *xmlNode {
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

func (n *xmlNode) attr(local string) string {
	for _, a := range n.attrs {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// parseXMLNodes reads data into an element tree. It fails on the same
// malformed input xml.Unmarshal rejects, so a document the parser could not
// read is never rewritten.
func parseXMLNodes(data []byte) (*xmlNode, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	var root *xmlNode
	var stack []*xmlNode
	for {
		offset := d.InputOffset()
		tok, err := d.Token()
		if err != nil {
			if root != nil && len(stack) == 0 && errors.Is(err, io.EOF) {
				return root, nil
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &xmlNode{name: t.Name.Local, attrs: t.Attr, start: offset, innerStart: d.InputOffset()}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, n)
			} else if root == nil {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			n := stack[len(stack)-1]
			n.innerEnd = offset
			n.end = d.InputOffset()
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write(t)
			}
		}
	}
}

// tocEntry is one entry of a table of contents: a nav li or an NCX
// navPoint.
type tocEntry struct {
	node     *xmlNode
	title    string   // as the parser reads it; "" when the parser skips the entry
	href     string   // as the parser reads it
	label    *xmlNode // the element whose text is the title
	list     *xmlNode // the nav ol holding children; nil for NCX
	children []*tocEntry
}

// navEntries returns the entries of the first toc nav that has a list,
// mirroring pkg/epub parseNavDocument.
func navEntries(root *xmlNode) []*tocEntry {
	if root.name != "html" {
		return nil
	}
	body := root.child("body")
	if body == nil {
		return nil
	}
	for _, nav := range body.children {
		if nav.name != "nav" || nav.attr("type") != "toc" {
			continue
		}
		if ol := nav.child("ol"); ol != nil {
			return navListEntries(ol)
		}
	}
	return nil
}

func navListEntries(ol *xmlNode) []*tocEntry {
	var entries []*tocEntry
	for _, li := range ol.children {
		if li.name != "li" {
			continue
		}
		e := &tocEntry{node: li}
		if a := li.child("a"); a != nil {
			e.label = a
			e.href = a.attr("href")
		} else {
			e.label = li.child("span")
		}
		if e.label != nil {
			e.title = strings.TrimSpace(e.label.text.String())
		}
		if e.list = li.child("ol"); e.list != nil {
			e.children = navListEntries(e.list)
		}
		entries = append(entries, e)
	}
	return entries
}

// ncxEntries returns the navMap's entries, mirroring pkg/epub parseNCX.
func ncxEntries(root *xmlNode) []*tocEntry {
	if root.name != "ncx" {
		return nil
	}
	navMap := root.child("navMap")
	if navMap == nil {
		return nil
	}
	return ncxPointEntries(navMap)
}

func ncxPointEntries(parent *xmlNode) []*tocEntry {
	var entries []*tocEntry
	for _, point := range parent.children {
		if point.name != "navPoint" {
			continue
		}
		e := &tocEntry{node: point}
		if navLabel := point.child("navLabel"); navLabel != nil {
			if e.label = navLabel.child("text"); e.label != nil {
				e.title = strings.TrimSpace(e.label.text.String())
			}
		}
		if content := point.child("content"); content != nil {
			e.href = content.attr("src")
		}
		e.children = ncxPointEntries(point)
		entries = append(entries, e)
	}
	return entries
}

// tocMatch pairs a source entry with the stored chapter it became.
type tocMatch struct {
	entry    *tocEntry
	chapter  editedChapter
	children []tocMatch
}

// alignTOC matches stored chapters to the parser's entries, in order. Every
// chapter must match an entry with the same href or the same title (it may
// have been renamed, and the nav and NCX can disagree on hrefs), and their
// children must align in turn. Entries left unmatched were deleted. Among
// alignments it prefers the one with the most equal hrefs and titles, so
// deleting one of two similar entries removes the right one.
func alignTOC(entries []*tocEntry, chapters []editedChapter) ([]tocMatch, bool) {
	matches, _, ok := align(entries, chapters)
	return matches, ok
}

func align(all []*tocEntry, chapters []editedChapter) ([]tocMatch, int, bool) {
	// Entries without a title never became chapters.
	var entries []*tocEntry
	for _, e := range all {
		if e.title != "" {
			entries = append(entries, e)
		}
	}
	n, m := len(entries), len(chapters)

	type pair struct {
		matches []tocMatch
		score   int
		ok      bool
	}
	pairs := make([][]*pair, n)
	pairFor := func(i, j int) *pair {
		if pairs[i] == nil {
			pairs[i] = make([]*pair, m)
		}
		if p := pairs[i][j]; p != nil {
			return p
		}
		e, ch := entries[i], chapters[j]
		p := &pair{}
		sameHref, sameTitle := e.href == ch.href, e.title == ch.title
		if sameHref || sameTitle {
			p.matches, p.score, p.ok = align(e.children, ch.children)
			if sameHref {
				p.score++
			}
			if sameTitle {
				p.score++
			}
		}
		pairs[i][j] = p
		return p
	}

	// best[i][j] is the best score aligning the first i entries with the
	// first j chapters, or -1 when they cannot align.
	best := make([][]int, n+1)
	for i := range best {
		best[i] = make([]int, m+1)
		for j := 1; j <= m; j++ {
			best[i][j] = -1
		}
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			best[i][j] = best[i-1][j]
			if best[i-1][j-1] < 0 {
				continue
			}
			if p := pairFor(i-1, j-1); p.ok && best[i-1][j-1]+p.score > best[i][j] {
				best[i][j] = best[i-1][j-1] + p.score
			}
		}
	}
	if best[n][m] < 0 {
		return nil, 0, false
	}

	matches := make([]tocMatch, m)
	for i, j := n, m; j > 0; i-- {
		if best[i][j] == best[i-1][j] {
			continue
		}
		p := pairFor(i-1, j-1)
		matches[j-1] = tocMatch{entry: entries[i-1], chapter: chapters[j-1], children: p.matches}
		j--
	}
	return matches, best[n][m], true
}

// byteEdit replaces source bytes [start, end) with text.
type byteEdit struct {
	start, end int64
	text       []byte
}

// tocEdits returns the edits that turn entries into the matched chapters:
// renamed labels get the new title, and unmatched entries are removed. A nav
// list left with no items is removed with them, since an empty ol is invalid.
func tocEdits(data []byte, entries []*tocEntry, matches []tocMatch, list *xmlNode) []byteEdit {
	matched := make(map[*tocEntry]tocMatch, len(matches))
	for _, m := range matches {
		matched[m.entry] = m
	}

	if list != nil && len(matches) == 0 && len(entries) > 0 {
		allDeleted := true
		for _, e := range entries {
			if e.title == "" {
				allDeleted = false
			}
		}
		if allDeleted {
			return []byteEdit{removal(data, list)}
		}
	}

	var edits []byteEdit
	for _, e := range entries {
		if e.title == "" {
			continue
		}
		m, ok := matched[e]
		if !ok {
			edits = append(edits, removal(data, e.node))
			continue
		}
		if m.chapter.title != e.title {
			var title bytes.Buffer
			_ = xml.EscapeText(&title, []byte(m.chapter.title))
			edits = append(edits, byteEdit{start: e.label.innerStart, end: e.label.innerEnd, text: title.Bytes()})
		}
		edits = append(edits, tocEdits(data, e.children, m.children, e.list)...)
	}
	return edits
}

// removal deletes an element along with the whitespace that indents it.
func removal(data []byte, n *xmlNode) byteEdit {
	start := n.start
	for start > 0 && strings.ContainsRune(" \t\r\n", rune(data[start-1])) {
		start--
	}
	return byteEdit{start: start, end: n.end}
}

func applyEdits(data []byte, edits []byteEdit) []byte {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out bytes.Buffer
	var pos int64
	for _, e := range edits {
		out.Write(data[pos:e.start])
		out.Write(e.text)
		pos = e.end
	}
	out.Write(data[pos:])
	return out.Bytes()
}

// rewriteTOC applies chapter edits to a nav document or NCX. It returns data
// unchanged when there are no stored chapters, the document cannot be read,
// or the chapters are not the document's entries with some renamed or
// deleted.
func rewriteTOC(data []byte, chapters []editedChapter, entriesOf func(*xmlNode) []*tocEntry) []byte {
	if len(chapters) == 0 {
		return data
	}
	root, err := parseXMLNodes(data)
	if err != nil {
		return data
	}
	entries := entriesOf(root)
	matches, ok := alignTOC(entries, chapters)
	if !ok {
		return data
	}
	edits := tocEdits(data, entries, matches, nil)
	if len(edits) == 0 {
		return data
	}
	return applyEdits(data, edits)
}
