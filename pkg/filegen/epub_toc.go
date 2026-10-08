package filegen

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/url"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/models"
)

// The table of contents is rewritten by splicing bytes into the source
// document rather than by marshaling it again: encoding/xml cannot write the
// XHTML nav document back without mangling its namespaces, and splicing
// leaves everything a chapter edit does not touch exactly as written.
//
// A scan stores the chapters of one document, the primary: the nav document
// when it has entries, otherwise the NCX (pkg/epub opf.go). The Chapters tab
// only renames and deletes, so the stored chapters are aligned to the
// primary's entries to find which were renamed or deleted, and the same
// edits are carried to the matching entries of the other document. Entries
// with no counterpart are left alone, so an NCX richer than the nav keeps
// its extra entries. A stored tree that is not the primary's entries with
// some renamed or deleted (chapters added, reordered, or supplied by a
// sidecar or plugin) leaves both documents as written.

const ncxMediaType = "application/x-dtbncx+xml"

// maxAlignStates caps the alignment table so a huge table of contents with
// thousands of deletions cannot exhaust memory.
const maxAlignStates = 4_000_000

// tocPaths returns the zip entry names of the EPUB 3 navigation document and
// the NCX, or "" for the ones the package does not have. The NCX is the item
// the spine names, falling back to any item with the NCX media type.
func tocPaths(pkg *opfPackage, opfPath string) (navPath, ncxPath string) {
	for _, item := range pkg.Manifest.Items {
		properties, _ := item.Attrs.get("", "properties")
		if navPath == "" && slices.Contains(strings.Fields(properties), "nav") {
			navPath = opfEntryPath(opfPath, item.Href)
		}
		if (pkg.Spine.Toc != "" && item.ID == pkg.Spine.Toc) || (ncxPath == "" && item.MediaType == ncxMediaType) {
			ncxPath = opfEntryPath(opfPath, item.Href)
		}
	}
	return navPath, ncxPath
}

// editedChapter is a stored chapter reduced to what the table of contents
// shows.
type editedChapter struct {
	title    string
	href     string
	children []editedChapter
}

// chapterTree returns the file's chapters as a tree. A file loaded from the
// database lists every chapter in one flat slice, nested ones included, so
// the tree is rebuilt from ParentID.
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

	var build func(level []*models.Chapter) []editedChapter
	build = func(level []*models.Chapter) []editedChapter {
		level = slices.Clone(level)
		sort.SliceStable(level, func(i, j int) bool { return level[i].SortOrder < level[j].SortOrder })
		out := make([]editedChapter, 0, len(level))
		for _, ch := range level {
			c := editedChapter{title: ch.Title, children: build(byParent[ch.ID])}
			if ch.Href != nil {
				c.href = *ch.Href
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

// child returns the last child named name, the one xml.Unmarshal keeps for a
// single-valued field.
func (n *xmlNode) child(name string) *xmlNode {
	for i := len(n.children) - 1; i >= 0; i-- {
		if n.children[i].name == name {
			return n.children[i]
		}
	}
	return nil
}

// attr returns the last attribute named local in any namespace, as
// xml.Unmarshal does for an attribute field without a namespace.
func (n *xmlNode) attr(local string) string {
	for i := len(n.attrs) - 1; i >= 0; i-- {
		if n.attrs[i].Name.Local == local {
			return n.attrs[i].Value
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
	resolved string   // href as a package path, comparable across documents
	label    *xmlNode // the element whose text is the title
	list     *xmlNode // the nav ol holding children; nil for NCX
	children []*tocEntry
}

// isHeading reports whether e is a nav heading, a span that the nav spec
// requires to be followed by a list.
func (e *tocEntry) isHeading() bool {
	return e.label != nil && e.label.name == "span"
}

// resolveHref turns a document-relative href into a package path, keeping
// any fragment.
func resolveHref(docPath, href string) string {
	if href == "" {
		return ""
	}
	ref, fragment, hasFragment := strings.Cut(href, "#")
	if unescaped, err := url.PathUnescape(ref); err == nil {
		ref = unescaped
	}
	if ref == "" {
		ref = docPath
	} else {
		ref = path.Join(path.Dir(docPath), ref)
	}
	if hasFragment {
		return ref + "#" + fragment
	}
	return ref
}

// navEntries returns the entries of the first toc nav that has a list,
// mirroring pkg/epub parseNavDocument.
func navEntries(root *xmlNode, docPath string) []*tocEntry {
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
			return navListEntries(ol, docPath)
		}
	}
	return nil
}

func navListEntries(ol *xmlNode, docPath string) []*tocEntry {
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
		e.resolved = resolveHref(docPath, e.href)
		if e.list = li.child("ol"); e.list != nil {
			e.children = navListEntries(e.list, docPath)
		}
		entries = append(entries, e)
	}
	return entries
}

// ncxEntries returns the navMap's entries, mirroring pkg/epub parseNCX.
func ncxEntries(root *xmlNode, docPath string) []*tocEntry {
	if root.name != "ncx" {
		return nil
	}
	navMap := root.child("navMap")
	if navMap == nil {
		return nil
	}
	return ncxPointEntries(navMap, docPath)
}

func ncxPointEntries(parent *xmlNode, docPath string) []*tocEntry {
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
		e.resolved = resolveHref(docPath, e.href)
		e.children = ncxPointEntries(point, docPath)
		entries = append(entries, e)
	}
	return entries
}

// tocDoc is a nav document or NCX read for rewriting.
type tocDoc struct {
	path    string
	data    []byte
	entries []*tocEntry
}

func readTOCDoc(docPath string, data []byte, entriesOf func(*xmlNode, string) []*tocEntry) *tocDoc {
	if data == nil {
		return nil
	}
	root, err := parseXMLNodes(data)
	if err != nil {
		return nil
	}
	return &tocDoc{path: docPath, data: data, entries: entriesOf(root, docPath)}
}

// hasChapters reports whether the parser gets any chapter from doc.
func (doc *tocDoc) hasChapters() bool {
	if doc == nil {
		return false
	}
	return slices.ContainsFunc(doc.entries, func(e *tocEntry) bool { return e.title != "" })
}

// tocChanges records how a document's entries were edited: entries in
// renamed get a new title, and entries in deleted are removed with their
// descendants.
type tocChanges struct {
	renamed map[*tocEntry]string
	deleted map[*tocEntry]bool
}

func newTOCChanges() tocChanges {
	return tocChanges{renamed: map[*tocEntry]string{}, deleted: map[*tocEntry]bool{}}
}

func (c tocChanges) empty() bool {
	return len(c.renamed) == 0 && len(c.deleted) == 0
}

// tocMatch pairs a primary entry with the stored chapter it became.
type tocMatch struct {
	entry    *tocEntry
	chapter  editedChapter
	children []tocMatch
}

// alignTOC matches stored chapters to the primary's entries and returns the
// edits that turn one into the other. Entries left unmatched were deleted.
// A chapter saved with an empty title keeps the entry's label.
func alignTOC(entries []*tocEntry, chapters []editedChapter) (tocChanges, bool) {
	changes := newTOCChanges()
	matches, _, ok := align(entries, chapters)
	if !ok {
		return changes, false
	}
	var record func(entries []*tocEntry, matches []tocMatch)
	record = func(entries []*tocEntry, matches []tocMatch) {
		matched := make(map[*tocEntry]bool, len(matches))
		for _, m := range matches {
			matched[m.entry] = true
			title := strings.TrimSpace(m.chapter.title)
			if title != "" && title != m.entry.title {
				changes.renamed[m.entry] = title
			}
			record(m.entry.children, m.children)
		}
		for _, e := range entries {
			if e.title != "" && !matched[e] {
				changes.deleted[e] = true
			}
		}
	}
	record(entries, matches)
	return changes, true
}

// align finds the best order-preserving match of every chapter to a titled
// entry: same href or same title, with children that align in turn. It
// prefers equal hrefs, then equal titles, so deleting one of two similar
// entries removes the right one. Edits only delete, so chapter j can only
// match one of entries j to j+(n-m), and the table covers just that band:
// linear when nothing was deleted.
func align(all []*tocEntry, chapters []editedChapter) ([]tocMatch, int, bool) {
	// Entries without a title never became chapters.
	var entries []*tocEntry
	for _, e := range all {
		if e.title != "" {
			entries = append(entries, e)
		}
	}
	n, m := len(entries), len(chapters)
	if m > n {
		return nil, 0, false
	}
	k := n - m
	if (m+1)*(k+1) > maxAlignStates {
		return nil, 0, false
	}

	// best[j][d] aligns the first j chapters with the first j+d entries.
	// A negative score means they cannot align; matched means entry j+d-1
	// matched chapter j-1, with children as its children's alignment.
	type state struct {
		score    int
		matched  bool
		children []tocMatch
	}
	best := make([][]state, m+1)
	for j := range best {
		best[j] = make([]state, k+1)
		for d := range best[j] {
			if j > 0 {
				best[j][d].score = -1
			}
		}
	}
	for j := 1; j <= m; j++ {
		ch := chapters[j-1]
		for d := 0; d <= k; d++ {
			s := &best[j][d]
			if d > 0 {
				s.score = best[j][d-1].score
			}
			prev := best[j-1][d].score
			if prev < 0 {
				continue
			}
			e := entries[j-1+d]
			sameHref, sameTitle := e.href == ch.href, e.title == ch.title
			if !sameHref && !sameTitle {
				continue
			}
			children, score, ok := align(e.children, ch.children)
			if !ok {
				continue
			}
			if sameHref {
				score += 2
			}
			if sameTitle {
				score++
			}
			if prev+score > s.score {
				*s = state{score: prev + score, matched: true, children: children}
			}
		}
	}
	if best[m][k].score < 0 {
		return nil, 0, false
	}

	matches := make([]tocMatch, m)
	for j, d := m, k; j > 0; {
		s := best[j][d]
		if !s.matched {
			d--
			continue
		}
		matches[j-1] = tocMatch{entry: entries[j-1+d], chapter: chapters[j-1], children: s.children}
		j--
	}
	return matches, best[m][k].score, true
}

// carryChanges maps the primary's changes onto doc's entries. An entry
// follows the primary entry with the same package path and title, else the
// only one with its title, else the only one with its path. Entries with no
// counterpart are left alone.
func carryChanges(primary []*tocEntry, changes tocChanges, doc *tocDoc) tocChanges {
	type key struct{ resolved, title string }
	byKey := map[key][]*tocEntry{}
	byTitle := map[string][]*tocEntry{}
	byPath := map[string][]*tocEntry{}
	gone := map[*tocEntry]bool{}
	var index func(entries []*tocEntry, deleted bool)
	index = func(entries []*tocEntry, deleted bool) {
		for _, e := range entries {
			if e.title == "" {
				continue
			}
			gone[e] = deleted || changes.deleted[e]
			k := key{e.resolved, e.title}
			byKey[k] = append(byKey[k], e)
			byTitle[e.title] = append(byTitle[e.title], e)
			if e.resolved != "" {
				byPath[e.resolved] = append(byPath[e.resolved], e)
			}
			index(e.children, gone[e])
		}
	}
	index(primary, false)

	counterpart := func(e *tocEntry) *tocEntry {
		if found := byKey[key{e.resolved, e.title}]; len(found) > 0 {
			return found[0]
		}
		if found := byTitle[e.title]; len(found) == 1 {
			return found[0]
		}
		if found := byPath[e.resolved]; e.resolved != "" && len(found) == 1 {
			return found[0]
		}
		return nil
	}

	carried := newTOCChanges()
	var walk func(entries []*tocEntry)
	walk = func(entries []*tocEntry) {
		for _, e := range entries {
			if e.title == "" {
				walk(e.children)
				continue
			}
			if p := counterpart(e); p != nil {
				if gone[p] {
					carried.deleted[e] = true
					continue
				}
				if title, ok := changes.renamed[p]; ok && title != e.title {
					carried.renamed[e] = title
				}
			}
			walk(e.children)
		}
	}
	walk(doc.entries)
	return carried
}

// byteEdit replaces source bytes [start, end) with text.
type byteEdit struct {
	start, end int64
	text       []byte
}

// tocEdits returns the byte edits that apply changes to entries, and how
// many of entries remain. A nav list left with no items is removed, since an
// empty ol is invalid, and so is a heading left with nothing under it.
func tocEdits(data []byte, entries []*tocEntry, changes tocChanges) ([]byteEdit, int, error) {
	var edits []byteEdit
	remaining := 0
	for _, e := range entries {
		if changes.deleted[e] {
			edits = append(edits, removal(data, e.node))
			continue
		}

		children, childrenLeft, err := tocEdits(data, e.children, changes)
		if err != nil {
			return nil, 0, err
		}
		if e.list != nil && len(e.children) > 0 && childrenLeft == 0 {
			if e.isHeading() {
				edits = append(edits, removal(data, e.node))
				continue
			}
			children = []byteEdit{removal(data, e.list)}
		}

		remaining++
		edits = append(edits, children...)
		if title, ok := changes.renamed[e]; ok && e.label != nil {
			var escaped bytes.Buffer
			if err := xml.EscapeText(&escaped, []byte(title)); err != nil {
				return nil, 0, err
			}
			edits = append(edits, byteEdit{start: e.label.innerStart, end: e.label.innerEnd, text: escaped.Bytes()})
		}
	}
	return edits, remaining, nil
}

// removal deletes an element along with the whitespace that indents it.
func removal(data []byte, n *xmlNode) byteEdit {
	start := n.start
	for start > 0 && strings.ContainsRune(" \t\r\n", rune(data[start-1])) {
		start--
	}
	return byteEdit{start: start, end: n.end}
}

// applyTOCChanges returns doc's bytes with changes applied, or nil when
// there is nothing to change or no entry would be left.
func applyTOCChanges(doc *tocDoc, changes tocChanges) ([]byte, error) {
	if changes.empty() {
		return nil, nil
	}
	edits, remaining, err := tocEdits(doc.data, doc.entries, changes)
	if err != nil || remaining == 0 {
		return nil, err
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out bytes.Buffer
	var pos int64
	for _, e := range edits {
		out.Write(doc.data[pos:e.start])
		out.Write(e.text)
		pos = e.end
	}
	out.Write(doc.data[pos:])
	return out.Bytes(), nil
}

// rewriteTOCs applies the file's chapter edits to the nav document and NCX,
// given their paths and source bytes (nil when absent). It returns the
// rewritten documents by path; a document missing from the result is copied
// as written.
func rewriteTOCs(ctx context.Context, navPath string, navData []byte, ncxPath string, ncxData []byte, stored []*models.Chapter) map[string][]byte {
	chapters := chapterTree(stored)
	if len(chapters) == 0 {
		return nil
	}

	nav := readTOCDoc(navPath, navData, navEntries)
	ncx := readTOCDoc(ncxPath, ncxData, ncxEntries)
	primary, secondary := nav, ncx
	if !nav.hasChapters() {
		primary, secondary = ncx, nil
	}
	if primary == nil {
		return nil
	}

	log := logger.FromContext(ctx)
	changes, ok := alignTOC(primary.entries, chapters)
	if !ok {
		log.Warn("EPUB chapters do not match its table of contents, leaving it as written", logger.Data{
			"category": "epub_toc_write",
			"toc":      primary.path,
		})
		return nil
	}

	out := map[string][]byte{}
	write := func(doc *tocDoc, changes tocChanges) {
		data, err := applyTOCChanges(doc, changes)
		if err != nil {
			log.Warn("failed to write EPUB chapter edits, leaving the table of contents as written", logger.Data{
				"category": "epub_toc_write",
				"toc":      doc.path,
				"error":    err.Error(),
			})
		}
		if data != nil {
			out[doc.path] = data
		}
	}
	write(primary, changes)
	if secondary != nil && !changes.empty() {
		write(secondary, carryChanges(primary.entries, changes, secondary))
	}
	return out
}
