package mobi

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"html"
	"io"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"golang.org/x/text/encoding/charmap"
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

// UnsupportedError is returned by ReadKF8 for a file it cannot convert.
type UnsupportedError struct {
	Reason string
}

func (e *UnsupportedError) Error() string {
	return "this Kindle book can't be converted: " + e.Reason
}

func unsupported(reason string) error {
	return errors.WithStack(&UnsupportedError{Reason: reason})
}

// KF8 is the content of the KF8 book in a MOBI or AZW3 file, with every
// kindle: reference turned into a relative path, so the documents, styles,
// and resources can sit side by side in one directory of an EPUB.
type KF8 struct {
	// Metadata is what Parse reads from the file, without the cover data.
	Metadata *mediafile.ParsedMetadata
	// Documents are the XHTML documents in reading order.
	Documents []Document
	// Styles are the flows the documents use: stylesheets and SVG images.
	Styles []Document
	// Resources are the images the documents, the styles, or the cover
	// name, in resource order. They are read with ReadResource.
	Resources []*Resource
	// Cover is the cover image, one of Resources, or nil when there is none.
	Cover *Resource
	// TOC is the table of contents.
	TOC []*NavPoint

	db *pdb
	// hrefs maps each kindle:pos location the book or its table of
	// contents names to the href it became.
	hrefs map[string]string
}

// Document is a document or style of a KF8 book.
type Document struct {
	Name      string
	MediaType string
	Content   []byte
}

// Resource is an image of a KF8 book, read from its record when written.
type Resource struct {
	Name      string
	MediaType string
	Size      int64
	record    int
}

// NavPoint is an entry of a KF8 book's table of contents. Href is a
// document's name and, after a #, an id in it.
type NavPoint struct {
	Title    string
	Href     string
	Children []*NavPoint
}

// ReadResource returns a resource's data.
func (k *KF8) ReadResource(r *Resource) ([]byte, error) {
	return k.db.record(r.record, maxImageRecord)
}

// Href returns the href a kindle:pos location (a TOCEntry's Locator, as a
// stored chapter holds it) became, and false for one that names no place
// in the book.
func (k *KF8) Href(locator string) (string, bool) {
	href, ok := k.hrefs[strings.ToUpper(locator)]
	return href, ok
}

// Limits on what a crafted file can make the reader allocate. A text record
// decompresses to at most 4 KiB; the text of the longest books is tens of
// megabytes.
const (
	maxTextRecord = 64 << 10
	maxTextLength = 256 << 20
)

// ReadKF8 reads the KF8 book in a MOBI or AZW3 file: an AZW3, or the KF8
// half of a combo file. It reads the text and indexes and the first bytes of
// each resource record, not the images, which ReadResource reads. It
// declines a DRM-protected file with ErrDRMProtected, and returns an
// UnsupportedError for a file it cannot convert.
func ReadKF8(ctx context.Context, r io.ReaderAt, size int64) (*KF8, error) {
	db, err := readPDB(r, size)
	if err != nil {
		return nil, err
	}
	b, err := readBook(db)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(b.books, func(ref bookRef) bool { return ref.kind == BookKF8 })
	if i < 0 {
		return nil, unsupported("it holds only an older MOBI (KF7) book, which Shisho can't convert yet")
	}
	ref := b.books[i]
	h := ref.header
	switch h.compression {
	case compressionNone, compressionPalmDOC:
	case compressionHuffCDIC:
		return nil, unsupported("its text uses HUFF/CDIC compression, which Shisho can't convert yet")
	}

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

	k := &KF8{Metadata: b.metadata(), db: db, hrefs: map[string]string{}}
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
	embed := func(n uint64) string {
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
				n, _ := strconv.ParseUint(string(sub[3]), 32, 32)
				target = embed(n)
			case sub[4] != nil:
				n, _ := strconv.ParseUint(string(sub[4]), 32, 32)
				if n > 0 && n < uint64(len(styleNames)) {
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
	editTag := func(tag []byte) []byte { return aidAttr.ReplaceAll(rewrite(tag, true), nil) }
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
		return nil, errors.Errorf("image %s is too large", tooLarge)
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

// kindleRef matches a kindle:pos link (group 1), a kindle:embed reference
// (group 3), and a kindle:flow reference (group 4), with any ?mime= that
// follows them.
var kindleRef = regexp.MustCompile(`kindle:(?:(pos:fid:([0-9A-Va-v]+):off:[0-9A-Va-v]+)|embed:([0-9A-Va-v]+)|flow:([0-9A-Va-v]+))(?:\?mime=[A-Za-z0-9/+.-]+)?`)

func partName(i int) string {
	return fmt.Sprintf("part%04d.xhtml", i)
}

func (k *KF8) navPoints(entries []*TOCEntry) []*NavPoint {
	var out []*NavPoint
	for _, e := range entries {
		if e.Title == "" {
			continue
		}
		href, ok := k.Href(e.Locator)
		if !ok {
			href = partName(0)
		}
		out = append(out, &NavPoint{Title: e.Title, Href: href, Children: k.navPoints(e.Children)})
	}
	return out
}

// toUTF8 decodes Windows-1252 text, which a KF8 book should never hold but
// whose header may still say so.
func toUTF8(b []byte, h *header) []byte {
	if h.encoding != encodingCP1252 {
		return b
	}
	if decoded, err := charmap.Windows1252.NewDecoder().Bytes(b); err == nil {
		return decoded
	}
	return b
}

// readText returns the book's text: each text record without its trailing
// entries, decompressed, up to the text length the header gives.
func readText(ctx context.Context, db *pdb, base int, rec []byte, compression uint16) ([]byte, error) {
	be := binary.BigEndian
	length := int64(be.Uint32(rec[4:]))
	count := int(be.Uint16(rec[offTextRecords:]))
	if length > maxTextLength {
		return nil, errors.New("MOBI text is too long")
	}
	if base+count >= db.count() {
		return nil, errors.New("MOBI text records run past the last record")
	}
	flags := headerField(rec, offExtraFlags)
	if flags == nullIndex {
		flags = 0
	}
	flags &= 0xFFFF
	// A record decompresses to about 4 KiB, so a header claiming more text
	// than its records hold does not get its whole length allocated.
	text := make([]byte, 0, min(length, int64(count)*8192))
	for i := base + 1; i <= base+count && int64(len(text)) < length; i++ {
		if err := ctx.Err(); err != nil {
			return nil, errors.WithStack(err)
		}
		data, err := db.record(i, maxTextRecord)
		if err != nil {
			return nil, err
		}
		trail, err := trailingSize(data, flags)
		if err != nil {
			return nil, errors.Wrapf(err, "text record %d", i)
		}
		data = data[:len(data)-trail]
		if compression == compressionPalmDOC {
			if data, err = palmDOCDecompress(data); err != nil {
				return nil, errors.Wrapf(err, "text record %d", i)
			}
		}
		text = append(text, data...)
	}
	if int64(len(text)) < length {
		return nil, errors.New("MOBI text is shorter than its header says")
	}
	return text[:length], nil
}

// trailingSize returns how many bytes of trailing entries end a text
// record. Readers take them off the end from flag bit 1 up, each ending in
// its own size; the multibyte entry (bit 0) comes last, its size in the low
// two bits of its last byte, plus one.
func trailingSize(data []byte, flags uint32) (int, error) {
	n := 0
	for bits := flags >> 1; bits != 0; bits >>= 1 {
		if bits&1 == 0 {
			continue
		}
		size := trailingEntrySize(data[:len(data)-n])
		if size == 0 || n+size > len(data) {
			return 0, errors.New("invalid trailing entry")
		}
		n += size
	}
	if flags&1 != 0 && n < len(data) {
		n += int(data[len(data)-n-1]&0x3) + 1
		if n > len(data) {
			return 0, errors.New("invalid multibyte trailing entry")
		}
	}
	return n, nil
}

// palmDOCDecompress decompresses one PalmDOC text record. Bytes 0x01 to
// 0x08 copy that many literal bytes; 0x80 to 0xBF start a two-byte
// back-reference, 11 bits of distance and 3 bits of length less three;
// 0xC0 and up are a space followed by the byte less 0x80; every other byte
// is itself.
func palmDOCDecompress(in []byte) ([]byte, error) {
	out := make([]byte, 0, 4096)
	for i := 0; i < len(in); {
		c := in[i]
		i++
		switch {
		case c >= 0x01 && c <= 0x08:
			if i+int(c) > len(in) {
				return nil, errors.New("PalmDOC literal runs past the end")
			}
			out = append(out, in[i:i+int(c)]...)
			i += int(c)
		case c >= 0x80 && c <= 0xBF:
			if i >= len(in) {
				return nil, errors.New("PalmDOC back-reference is truncated")
			}
			pair := int(c)<<8 | int(in[i])
			i++
			dist, length := (pair&0x3FFF)>>3, pair&0x7+3
			if dist == 0 || dist > len(out) {
				return nil, errors.New("PalmDOC back-reference points before the start")
			}
			for range length {
				out = append(out, out[len(out)-dist])
			}
		case c >= 0xC0:
			out = append(out, ' ', c^0x80)
		default:
			out = append(out, c)
		}
	}
	return out, nil
}

// readFlows returns each flow's start and end in the text. A book without
// an FDST record has one flow.
func readFlows(db *pdb, base int, rec []byte, textLength int) ([][2]int, error) {
	index := headerField(rec, offFDST)
	if index == nullIndex || index == 0 {
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
		name := t.start + 1
		for name < t.end && !isSpace(doc[name]) && doc[name] != '>' && doc[name] != '/' {
			name++
		}
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

// anchorAt returns where the start tag a position names begins, and its id
// or "" when it has none.
func anchorAt(doc []byte, pos int) (int, string) {
	t, ok := anchorTag(doc, startTags(doc), pos)
	if !ok {
		return -1, ""
	}
	return t.start, t.id
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
	end := 1
	for end < len(tag) && !isSpace(tag[end]) && tag[end] != '>' && tag[end] != '/' {
		end++
	}
	return strings.ToLower(string(tag[1:end]))
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

var aidAttr = regexp.MustCompile(`\s+aid\s*=\s*(?:"[^"]*"|'[^']*')`)

// editDocument returns doc with each start tag passed through editTag and
// the content of each style element through editStyle, leaving text,
// comments, and CDATA alone.
func editDocument(doc []byte, editTag, editStyle func([]byte) []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(doc))
	last := 0
	styleEnd := -1
	eachStartTag(doc, func(start, end int) {
		if start < styleEnd {
			return
		}
		out.Write(doc[last:start])
		out.Write(editTag(doc[start:end]))
		last = end
		if tagName(doc[start:end]) == "style" {
			if styleClose := bytes.Index(bytes.ToLower(doc[end:]), []byte("</style")); styleClose >= 0 {
				out.Write(editStyle(doc[end : end+styleClose]))
				last, styleEnd = end+styleClose, end+styleClose
			}
		}
	})
	out.Write(doc[last:])
	return out.Bytes()
}

// resources is the resource section of a file: the records from the first
// image to the first record that ends it.
type resources struct {
	all []*Resource
	// byRecord maps a record to its Resource, for the image records.
	byRecord map[int]*Resource
	first    int
	count    int
}

// recordsAfterResources open the records that follow a book's resources.
var recordsAfterResources = [][]byte{
	[]byte("FLIS"), []byte("FCIS"), []byte("FDST"), []byte("DATP"), []byte("SRCS"),
	[]byte("CMET"), []byte("BOUNDARY"), {0xE9, 0x8E, 0x0D, 0x0A},
}

// scanResources reads the first bytes of each resource record, so the
// images are known without reading them. A FONT record declines the book.
func scanResources(db *pdb, first uint32) (*resources, error) {
	res := &resources{byRecord: map[int]*Resource{}, first: int(first)}
	if first == nullIndex || int64(first) >= int64(db.count()) {
		return res, nil
	}
	for i := int(first); i < db.count(); i++ {
		head, err := db.prefix(i, 8)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(recordsAfterResources, func(magic []byte) bool { return bytes.HasPrefix(head, magic) }) {
			break
		}
		res.count++
		if bytes.HasPrefix(head, []byte("FONT")) {
			return nil, unsupported("it embeds fonts, which Shisho can't convert yet")
		}
		mimeType := imageMimeType(head)
		if mimeType == "" {
			continue
		}
		r := &Resource{
			Name:      fmt.Sprintf("image%04d%s", res.count, imageExtension(mimeType)),
			MediaType: mimeType,
			Size:      db.offsets[i+1] - db.offsets[i],
			record:    i,
		}
		res.all = append(res.all, r)
		res.byRecord[i] = r
	}
	return res, nil
}

// byEmbed returns the image a kindle:embed number names, counting from 1.
func (r *resources) byEmbed(n uint64) (*Resource, bool) {
	if n == 0 || n > uint64(r.count) { //nolint:gosec // count is a record count
		return nil, false
	}
	res, ok := r.byRecord[r.first+int(n)-1] //nolint:gosec // n is at most count
	return res, ok
}

func (r *resources) named(name string) *Resource {
	for _, res := range r.all {
		if res.Name == name {
			return res
		}
	}
	return nil
}

func imageExtension(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/bmp":
		return ".bmp"
	}
	return ""
}
