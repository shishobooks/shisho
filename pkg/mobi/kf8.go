package mobi

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"html"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// A KF8 book's text is one stream, cut into records and compressed. The
// FDST record splits it into flows: flow 0 holds every XHTML document, and
// the flows after it hold stylesheets and SVG images. Each document is
// stored as a skeleton, the document with its body content cut out, followed
// by that content in fragments; the skeleton index gives each skeleton's
// place in flow 0 and its fragment count, and the fragment index gives each
// fragment's insert position, counted as if the documents before it and the
// fragments before it in its own document were already assembled. Links
// name places as a fragment and an offset into it (kindle:pos), images as a
// resource number (kindle:embed), and flows by number (kindle:flow).

// readKF8 reads the KF8 book ref of a file: an AZW3, or the KF8 half of a
// combo file.
func readKF8(ctx context.Context, db *pdb, b *book, ref bookRef) (*Content, error) {
	h := ref.header
	rec, err := db.record(ref.base, maxHeaderRecord)
	if err != nil {
		return nil, err
	}
	text, err := readText(ctx, db, ref.base, rec, h.compression)
	if err != nil {
		return nil, err
	}
	flows, err := readFlows(db, ref.base, rec, len(text))
	if err != nil {
		return nil, err
	}
	parts, err := readParts(ctx, db, ref.base, rec, text[flows[0][0]:flows[0][1]])
	if err != nil {
		return nil, err
	}

	k := &Content{Metadata: b.metadata(), db: db, hrefs: map[string]string{}}
	res, err := scanResources(db, b.firstImage)
	if err != nil {
		return nil, err
	}

	// Find every place the documents and the table of contents name, and
	// give the element there an id when it has none.
	locators := map[string]bool{}
	for _, p := range parts.docs {
		eachStartTag(p, func(start, end int) {
			for _, m := range kindleRef.FindAllSubmatch(p[start:end], -1) {
				if m[1] != nil {
					locators[strings.ToUpper(string(m[0]))] = true
				}
			}
		})
	}
	tocs := readTOCs(db, []bookRef{ref})
	if len(tocs) > 0 {
		var walk func([]*TOCEntry)
		walk = func(entries []*TOCEntry) {
			for _, e := range entries {
				if e.Locator != "" {
					locators[strings.ToUpper(e.Locator)] = true
				}
				walk(e.Children)
			}
		}
		walk(tocs[0].Entries)
	}
	k.hrefs = parts.anchor(locators)

	used := map[int]bool{}
	tooLarge := ""
	fonts := &fontsRead{names: map[int]string{}}
	embed := func(n uint64) string {
		r, ok := res.byEmbed(n)
		if !ok {
			return k.font(res, fonts, n)
		}
		if r.Size > maxImageRecord {
			tooLarge = r.Name
		}
		used[r.record] = true
		return r.Name
	}
	styleNames := make([]string, len(flows))
	for f := 1; f < len(flows); f++ {
		content := text[flows[f][0]:flows[f][1]]
		doc := Document{Name: fmt.Sprintf("flow%04d.css", f), MediaType: "text/css"}
		if t := bytes.TrimSpace(content); bytes.HasPrefix(t, []byte("<svg")) || bytes.HasPrefix(t, []byte("<?xml")) {
			doc = Document{Name: fmt.Sprintf("flow%04d.svg", f), MediaType: "image/svg+xml"}
		}
		styleNames[f] = doc.Name
		k.Styles = append(k.Styles, doc)
	}
	// rewrite turns kindle: references into relative paths, escaped for
	// markup when escape is set. A reference that names nothing in the book
	// is left as written.
	rewrite := func(content []byte, escape bool) []byte {
		return kindleRef.ReplaceAllFunc(content, func(m []byte) []byte {
			sub := kindleRef.FindSubmatch(m)
			target := ""
			switch {
			case sub[1] != nil:
				target = k.hrefs[strings.ToUpper(string(m))]
			case sub[3] != nil:
				if n, err := strconv.ParseUint(string(sub[3]), 32, 32); err == nil {
					target = embed(n)
				}
			case sub[4] != nil:
				if n, err := strconv.ParseUint(string(sub[4]), 32, 32); err == nil && n > 0 && n < uint64(len(styleNames)) {
					target = styleNames[n]
				}
			}
			switch {
			case target == "":
				return m
			case escape:
				return []byte(html.EscapeString(target))
			}
			return []byte(target)
		})
	}
	editTag := func(tag []byte) []byte { return withXHTMLNamespace(kindleAttrs.ReplaceAll(rewrite(tag, true), nil)) }
	editStyle := func(css []byte) []byte { return rewrite(css, false) }
	for f, style := range k.Styles {
		content := text[flows[f+1][0]:flows[f+1][1]]
		if style.MediaType == "image/svg+xml" {
			content = editDocument(content, editTag, editStyle)
		} else {
			content = rewrite(content, false)
		}
		k.Styles[f].Content = toUTF8(content, h)
	}
	for i, p := range parts.docs {
		k.Documents = append(k.Documents, Document{
			Name:      partName(i),
			MediaType: "application/xhtml+xml",
			Content:   toUTF8(editDocument(p, editTag, editStyle), h),
		})
	}
	if n, ok := b.coverEmbed(); ok {
		if name := embed(n); name != "" {
			k.Cover = res.named(name)
		}
	}
	if tooLarge != "" {
		return nil, unsupported("it has an image too large to copy (" + tooLarge + ")")
	}
	if fonts.tooLarge {
		return nil, unsupported("its fonts are too large to copy")
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

// fontsRead is what Content.font has read: the name of each font record,
// "" for one left out, their size together, and whether a font was left out
// for making them too large.
type fontsRead struct {
	names    map[int]string
	size     int64
	tooLarge bool
}

// font returns the file name of the font a kindle:embed number names,
// adding it to the book's fonts the first time, or "" when it names no
// font or one that can't be read, which is left out.
func (k *Content) font(res *resources, read *fontsRead, n uint64) string {
	record, ok := res.fontByEmbed(n)
	if !ok {
		return ""
	}
	if name, ok := read.names[record]; ok {
		return name
	}
	font, err := readFont(k.db, record, int(n), maxFontsSize-read.size) //nolint:gosec // n is at most the resource count
	name := ""
	switch {
	case err == nil:
		name = font.Name
		k.Fonts = append(k.Fonts, font)
		read.size += int64(len(font.Content))
	case errors.Is(err, errFontsTooLarge):
		read.tooLarge = true
	}
	read.names[record] = name
	return name
}

// kindleRef matches a kindle:pos link (group 1), a kindle:embed reference
// (group 3), and a kindle:flow reference (group 4), with any ?mime= that
// follows them.
var kindleRef = regexp.MustCompile(`kindle:(?:(pos:fid:([0-9A-Va-v]+):off:[0-9A-Va-v]+)|embed:([0-9A-Va-v]+)|flow:([0-9A-Va-v]+))(?:\?mime=[A-Za-z0-9/+.-]+)?`)

// readFlows returns each flow's start and end in the text. A book without
// an FDST record has one flow.
func readFlows(db *pdb, base int, rec []byte, textLength int) ([][2]int, error) {
	// KindleGen writes no FDST record for a book of one flow, and leaves a
	// MOBI6 header's first and last text records where its index goes,
	// with a flow count of 1.
	index := headerField(rec, offFDST)
	if count := headerField(rec, offFDSTCount); index == nullIndex || index == 0 || count <= 1 {
		return [][2]int{{0, textLength}}, nil
	}
	data, err := db.record(base+int(index), maxIndexRecord)
	if err != nil {
		return nil, err
	}
	be := binary.BigEndian
	if len(data) < 12 || string(data[:4]) != "FDST" {
		return nil, errors.New("MOBI FDST record is invalid")
	}
	start, count := int64(be.Uint32(data[4:])), int64(be.Uint32(data[8:]))
	if count == 0 || start+8*count > int64(len(data)) {
		return nil, errors.New("MOBI FDST record is truncated")
	}
	flows := make([][2]int, count)
	for i := range flows {
		s, e := int64(be.Uint32(data[start+8*int64(i):])), int64(be.Uint32(data[start+8*int64(i)+4:]))
		if s > e || e > int64(textLength) {
			return nil, errors.New("MOBI flow runs past the end of the text")
		}
		flows[i] = [2]int{int(s), int(e)}
	}
	return flows, nil
}

// kf8Parts is a KF8 book's assembled documents and what links into them
// are resolved against.
type kf8Parts struct {
	docs [][]byte
	// starts holds where each document starts in the coordinates insert
	// positions use, in increasing order.
	starts []int
	// inserts holds each fragment's insert position.
	inserts []int
}

// kf8Skeleton and kf8Fragment are entries of the skeleton and fragment
// indexes.
type kf8Skeleton struct {
	fragments, start, length int
}

type kf8Fragment struct {
	insert, length int
}

// maxInsertWork caps the bytes assembling the documents may move. A
// fragment goes in after the ones before it, so a real book moves little
// more than each skeleton's tail per fragment; a crafted index that inserts
// every fragment at the start of a large document would move far more.
const maxInsertWork = 256 << 20

// readParts reads the skeleton and fragment indexes and assembles the
// documents in flow 0 from them.
func readParts(ctx context.Context, db *pdb, base int, rec []byte, flow []byte) (*kf8Parts, error) {
	skelIndex, fragIndex := headerField(rec, offSkeleton), headerField(rec, offFragment)
	if skelIndex == nullIndex || fragIndex == nullIndex {
		return nil, unsupported("it has no skeleton or fragment index, so its documents can't be found")
	}
	skelIdx, err := readNCX(db, base+int(skelIndex))
	if err != nil {
		return nil, errors.Wrap(err, "read skeleton index")
	}
	fragIdx, err := readNCX(db, base+int(fragIndex))
	if err != nil {
		return nil, errors.Wrap(err, "read fragment index")
	}
	skels := make([]kf8Skeleton, 0, len(skelIdx.entries))
	for i, e := range skelIdx.entries {
		count, _ := e.value(1)
		geometry := e.values[6]
		if len(geometry) < 2 {
			return nil, errors.Errorf("skeleton %d has no position", i)
		}
		skels = append(skels, kf8Skeleton{fragments: int(count), start: int(geometry[0]), length: int(geometry[1])})
	}
	frags := make([]kf8Fragment, 0, len(fragIdx.entries))
	for i, e := range fragIdx.entries {
		insert, err := strconv.Atoi(string(e.ident))
		geometry := e.values[6]
		if err != nil || insert < 0 || len(geometry) < 2 {
			return nil, errors.Errorf("fragment %d is invalid", i)
		}
		frags = append(frags, kf8Fragment{insert: insert, length: int(geometry[1])})
	}
	return assembleParts(ctx, skels, frags, flow)
}

// assembleParts puts each document back together: its skeleton, with each
// of its fragments, stored after it, inserted in turn. The skeletons must
// follow one another, each starting where the fragments of the one before
// end, so the documents together are no larger than flow.
func assembleParts(ctx context.Context, skels []kf8Skeleton, frags []kf8Fragment, flow []byte) (*kf8Parts, error) {
	p := &kf8Parts{}
	next, at, work := 0, 0, 0
	for i, s := range skels {
		if err := ctx.Err(); err != nil {
			return nil, errors.WithStack(err)
		}
		if s.start != at || s.length < 0 || s.length > len(flow)-at || s.fragments > len(frags)-next {
			return nil, errors.Errorf("skeleton %d is not where the text says it is", i)
		}
		doc := slices.Clone(flow[at : at+s.length])
		at += s.length
		for range s.fragments {
			f := frags[next]
			next++
			local := f.insert - s.start
			if f.length < 0 || f.length > len(flow)-at || local < 0 || local > len(doc) {
				return nil, errors.Errorf("fragment %d runs past its document", next-1)
			}
			if work += len(doc) - local; work > maxInsertWork {
				return nil, errors.New("the book's fragments are out of order")
			}
			doc = slices.Insert(doc, local, flow[at:at+f.length]...)
			at += f.length
			p.inserts = append(p.inserts, f.insert)
		}
		p.docs = append(p.docs, doc)
		p.starts = append(p.starts, s.start)
	}
	if len(p.docs) == 0 {
		return nil, unsupported("it has no documents")
	}
	return p, nil
}

// anchor finds the element each kindle:pos location names, gives it an id
// when it has none, and returns the href of each location that names one.
// A location inside a document but at no element names the document.
func (p *kf8Parts) anchor(locators map[string]bool) map[string]string {
	hrefs := map[string]string{}
	tags := make([][]startTag, len(p.docs))
	// added maps a document's tags that get a new id to that id.
	added := make([]map[int]string, len(p.docs))
	taken := make([]map[string]bool, len(p.docs))
	for loc := range locators {
		m := posRef.FindStringSubmatch(loc)
		if m == nil {
			continue
		}
		fid, err1 := strconv.ParseUint(m[1], 32, 32)
		off, err2 := strconv.ParseUint(m[2], 32, 32)
		if err1 != nil || err2 != nil || fid >= uint64(len(p.inserts)) {
			continue
		}
		pos := p.inserts[fid] + int(off) //nolint:gosec // off fits in 32 bits
		doc := sort.SearchInts(p.starts, pos+1) - 1
		if doc < 0 || pos >= p.starts[doc]+len(p.docs[doc]) {
			continue
		}
		if tags[doc] == nil {
			tags[doc] = startTags(p.docs[doc])
			added[doc] = map[int]string{}
			taken[doc] = map[string]bool{}
			for _, t := range tags[doc] {
				taken[doc][t.id] = true
			}
		}
		t, ok := anchorTag(p.docs[doc], tags[doc], pos-p.starts[doc])
		switch {
		case !ok:
			hrefs[loc] = partName(doc)
			continue
		case t.id == "" && added[doc][t.start] == "":
			id := fmt.Sprintf("kindle-pos-%d-%d", doc, t.start)
			for n := 2; taken[doc][id]; n++ {
				id = fmt.Sprintf("kindle-pos-%d-%d-%d", doc, t.start, n)
			}
			taken[doc][id] = true
			added[doc][t.start] = id
		}
		id := t.id
		if id == "" {
			id = added[doc][t.start]
		}
		hrefs[loc] = partName(doc) + "#" + id
	}
	for d := range p.docs {
		if len(added[d]) > 0 {
			p.docs[d] = withIDs(p.docs[d], tags[d], added[d])
		}
	}
	return hrefs
}

// withIDs returns doc with the given ids added to the start tags at their
// keys, replacing an empty id attribute a tag already has.
func withIDs(doc []byte, tags []startTag, ids map[int]string) []byte {
	var out bytes.Buffer
	out.Grow(len(doc) + 32*len(ids))
	last := 0
	for _, t := range tags {
		id, ok := ids[t.start]
		if !ok {
			continue
		}
		name := t.start + nameEnd(doc[t.start:t.end])
		out.Write(doc[last:name])
		out.WriteString(` id="` + html.EscapeString(id) + `"`)
		out.Write(idAttr.ReplaceAll(doc[name:t.end], nil))
		last = t.end
	}
	out.Write(doc[last:])
	return out.Bytes()
}

var posRef = regexp.MustCompile(`^KINDLE:POS:FID:([0-9A-V]+):OFF:([0-9A-V]+)$`)

// startTag is a start tag of a document: where it begins and ends, and its
// id, decoded, or "" when it has none or an empty one.
type startTag struct {
	start, end int
	id         string
}

// maxTagLength caps how far a start tag is scanned for its end, so a quote
// left open does not make every later tag scan to the end of the document.
const maxTagLength = 64 << 10

// startTags returns the start tags of doc in order, skipping comments and
// CDATA sections.
func startTags(doc []byte) []startTag {
	var tags []startTag
	eachStartTag(doc, func(start, end int) {
		tags = append(tags, startTag{start: start, end: end, id: tagID(doc[start:end])})
	})
	return tags
}

// eachStartTag calls fn with the bounds of each start tag of doc, skipping
// comments and CDATA sections.
func eachStartTag(doc []byte, fn func(start, end int)) {
	for i := 0; i < len(doc); i++ {
		if doc[i] != '<' {
			continue
		}
		switch {
		case bytes.HasPrefix(doc[i:], []byte("<!--")):
			end := bytes.Index(doc[i:], []byte("-->"))
			if end < 0 {
				return
			}
			i += end + 2
		case bytes.HasPrefix(doc[i:], []byte("<![CDATA[")):
			end := bytes.Index(doc[i:], []byte("]]>"))
			if end < 0 {
				return
			}
			i += end + 2
		case isStartTag(doc, i):
			end := tagEnd(doc, i)
			fn(i, end)
			i = end - 1
		}
	}
}

// maxNesting caps how many enclosing elements anchorTag looks through for
// an id.
const maxNesting = 16

// anchorTag returns the start tag a position names: the last one beginning
// at or before it, which is the tag the position is in, or else the element
// whose content holds it. A tag with no id that opens its parent's content,
// with only whitespace before it, names the same place as its parent, so the
// parent is returned when it has an id: Kindle books point a chapter's entry
// at the first element inside the element with the chapter's id.
func anchorTag(doc []byte, tags []startTag, pos int) (startTag, bool) {
	i := sort.Search(len(tags), func(i int) bool { return tags[i].start > pos }) - 1
	if i < 0 {
		return startTag{}, false
	}
	for child, n := i, 0; tags[child].id == "" && child > 0 && n < maxNesting; child, n = child-1, n+1 {
		parent := tags[child-1]
		between := doc[parent.end:tags[child].start]
		if len(bytes.TrimLeft(between, " \t\n\r\f")) > 0 || bytes.HasSuffix(doc[parent.start:parent.end], []byte("/>")) {
			break
		}
		if parent.id != "" {
			return parent, true
		}
	}
	return tags[i], true
}

func isStartTag(doc []byte, at int) bool {
	return at+1 < len(doc) && (doc[at+1] >= 'a' && doc[at+1] <= 'z' || doc[at+1] >= 'A' && doc[at+1] <= 'Z')
}

// tagEnd returns the index just past the start tag at at, skipping quoted
// attribute values, or where scanning gave up.
func tagEnd(doc []byte, at int) int {
	var quote byte
	limit := min(len(doc), at+maxTagLength)
	for i := at + 1; i < limit; i++ {
		switch c := doc[i]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return i + 1
		}
	}
	return limit
}

var idAttr = regexp.MustCompile(`\sid\s*=\s*(?:"([^"]*)"|'([^']*)')`)

// tagID returns a start tag's id, with its character references decoded.
func tagID(tag []byte) string {
	m := idAttr.FindSubmatch(tag)
	if m == nil {
		return ""
	}
	return html.UnescapeString(string(m[1]) + string(m[2]))
}

// tagName returns a start tag's element name, in lower case.
func tagName(tag []byte) string {
	return strings.ToLower(string(tag[1:nameEnd(tag)]))
}

// nameEnd returns where a start tag's element name ends.
func nameEnd(tag []byte) int {
	end := 1
	for end < len(tag) && !isSpace(tag[end]) && tag[end] != '>' && tag[end] != '/' {
		end++
	}
	return end
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// kindleAttrs matches the attributes KindleGen adds for Kindles alone: aid,
// which the fragment index's selectors name, and data-AmznPageBreak and
// the like, which XHTML rejects for their capitals.
var kindleAttrs = regexp.MustCompile(`\s+(?:aid|data-Amzn[A-Za-z]*)\s*=\s*(?:"[^"]*"|'[^']*')`)

// withXHTMLNamespace returns an html start tag with the XHTML namespace,
// which KindleGen leaves out of a book built from HTML, and any other tag
// as it is.
func withXHTMLNamespace(tag []byte) []byte {
	if tagName(tag) != "html" || xmlnsAttr.Match(tag) {
		return tag
	}
	end := nameEnd(tag)
	return slices.Concat(tag[:end], []byte(` xmlns="http://www.w3.org/1999/xhtml"`), tag[end:])
}

var xmlnsAttr = regexp.MustCompile(`\sxmlns\s*=`)

// editDocument returns doc with each start tag passed through editTag and
// the content of each style element through editStyle, leaving text,
// comments, and CDATA alone.
func editDocument(doc []byte, editTag, editStyle func([]byte) []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(doc))
	last := 0
	styleEnd := -1
	closes := styleEndTags(doc)
	eachStartTag(doc, func(start, end int) {
		if start < styleEnd {
			return
		}
		out.Write(doc[last:start])
		out.Write(editTag(doc[start:end]))
		last = end
		if tagName(doc[start:end]) != "style" || bytes.HasSuffix(doc[start:end], []byte("/>")) {
			return
		}
		for len(closes) > 0 && closes[0] < end {
			closes = closes[1:]
		}
		if len(closes) > 0 {
			out.Write(editStyle(doc[end:closes[0]]))
			last, styleEnd = closes[0], closes[0]
		}
	})
	out.Write(doc[last:])
	return out.Bytes()
}

// styleEndTags returns where each "</style" in doc begins, in any letter
// case, in order. It compares bytes, so the positions are doc's own, which
// lowercasing a copy would not keep for text that isn't ASCII.
func styleEndTags(doc []byte) []int {
	const tag = "</style"
	var out []int
	for i := 0; ; i++ {
		at := bytes.Index(doc[i:], []byte("</"))
		if at < 0 || i+at+len(tag) > len(doc) {
			return out
		}
		i += at
		if asciiEqualFold(doc[i:i+len(tag)], tag) {
			out = append(out, i)
		}
	}
}

// asciiEqualFold reports whether b is s, with ASCII letters compared in
// either case and every other byte compared as is.
func asciiEqualFold(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := range b {
		c := b[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != s[i] {
			return false
		}
	}
	return true
}
