package mobi

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"io"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pkg/errors"
)

// Writing an AZW3 file, a KF8 book on its own, from the XHTML documents,
// stylesheets, and images of a book, as the format's specification
// describes it (ADR 0009). Each document is cut into a
// skeleton and fragments, every element in its body gets an aid attribute,
// image, font, and stylesheet references become kindle:embed and
// kindle:flow references, and links between documents become kindle:pos
// references. The text is compressed with PalmDOC into records of 4096
// bytes, each ending in a multibyte trailing entry, followed by the NCX,
// skeleton, fragment, and guide indexes, the images and fonts, and the
// FDST, FLIS, FCIS, and EOF records.

// KF8Book is a book for WriteKF8. Names are paths in one namespace, such
// as an EPUB's zip entry names, which references in the documents and
// stylesheets are resolved against.
type KF8Book struct {
	// Metadata is written to the EXTH block. Its Cover, TOC, and
	// OnTOCSkipped are not used: the cover is one of Images.
	Metadata Metadata
	// Identifier is written as the ASIN records, the identifier the Kindle
	// knows the book by.
	Identifier string
	// Documents are the XHTML documents in reading order.
	Documents []Document
	// Styles are the stylesheets the documents link.
	Styles []Document
	Images []KF8Image
	// Fonts are the embedded fonts the stylesheets name. A font is stored
	// as a FONT resource; data that is no font a Kindle shows is left out.
	Fonts []Document
	// Cover is the name of the image to show as the book's cover, or "".
	Cover string
	// TOC is the table of contents. An Href names a document and, after a
	// #, the id of an element in it.
	TOC []*NavPoint
	// Guide names the book's landmarks, such as where its text starts.
	Guide []GuideRef
	// FixedLayout marks a book whose pages each fill the screen, such as a
	// comic, and OriginalResolution is the size in pixels its pages were
	// made for, as "800x1200".
	FixedLayout        bool
	OriginalResolution string
}

// GuideRef is an entry of a KF8Book's guide: its type, as an EPUB 2
// guide names it ("text", "toc", "cover"), a title, and an Href in the
// form of a NavPoint's.
type GuideRef struct {
	Type, Title, Href string
}

// KF8Image is an image of a KF8Book. It is read only while the file is
// written.
type KF8Image struct {
	Name      string
	MediaType string
	Load      func() ([]byte, error)
}

// ErrNoDocuments is returned by WriteKF8 for a book with no documents.
var ErrNoDocuments = errors.New("the book has no documents")

const (
	// textRecordSize is how much text each text record holds.
	textRecordSize = 4096
	// fragmentSize is the most a document's body content is grouped into
	// one fragment. An element larger than this is cut into fragments
	// inside it.
	fragmentSize = 8192
	// kf8HeaderLength is the MOBI header length of a KF8 book.
	kf8HeaderLength = 264
	// headerPadding follows the full name in the header record, as
	// Calibre leaves it, so a Kindle can rewrite the record in place.
	headerPadding = 1024

	exthDocType = 501
	// exthStartReading holds where the book opens: a place in the text.
	exthStartReading = 116
	// guideTagTitle is the guide index tag holding an entry's title.
	guideTagTitle = 1
	// The records of a fixed-layout book, as Kindle comics carry them. The
	// page size it was made for goes in both resolution records.
	exthFixedLayout           = 122
	exthBookType              = 123
	exthOrientationLock       = 124
	exthOriginalResolutionAlt = 126
	exthZeroGutter            = 127
	exthZeroMargin            = 128
	exthRegionMagnification   = 132
	exthOriginalResolution    = 307
	exthASINAlt               = 504
	ncxTagLength              = 2
	// extraFlagsMBEntry says each text record ends in a multibyte entry
	// alone. The trailing byte sequences that name the index entries a
	// record touches are not written: the flags tell a reader they are
	// absent, and a KF8 book is navigated by its NCX, skeleton, and fragment
	// indexes.
	extraFlagsMBEntry = 0b1
)

// flisRecord and fcisRecord are written as Calibre and KindleGen write
// them; only the FCIS record's text length varies.
var flisRecord = []byte{
	'F', 'L', 'I', 'S', 0, 0, 0, 8, 0, 65, 0, 0, 0, 0, 0, 0,
	0xFF, 0xFF, 0xFF, 0xFF, 0, 1, 0, 3, 0, 0, 0, 3, 0, 0, 0, 1, 0xFF, 0xFF, 0xFF, 0xFF,
}

func fcisRecord(textLength int) []byte {
	r := []byte{
		'F', 'C', 'I', 'S', 0, 0, 0, 20, 0, 0, 0, 16, 0, 0, 0, 1, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32, 0, 0, 0, 8, 0, 1, 0, 1, 0, 0, 0, 0,
	}
	binary.BigEndian.PutUint32(r[20:], uint32(textLength)) //nolint:gosec // checked against the text length limit
	return r
}

// WriteKF8 writes b to w as an AZW3 file. The record list comes first in
// the file and needs every record's offset, so the records are written
// after the space it takes, and it is written last; only the images are
// read while writing, one at a time.
func WriteKF8(ctx context.Context, w io.WriteSeeker, b *KF8Book) error {
	if len(b.Documents) == 0 {
		return errors.WithStack(ErrNoDocuments)
	}
	l := newKF8Layout(b)
	if err := l.layout(ctx); err != nil {
		return err
	}
	return l.write(ctx, w)
}

// kf8Resource is an image or a font stored as a resource record.
type kf8Resource struct {
	image KF8Image
	// record is a font's FONT record, written as it is.
	record []byte
	// mediaType is what the record holds.
	mediaType string
	// convert re-encodes the image as a JPEG, for an image a Kindle can't
	// show and for a cover that is neither a JPEG nor a GIF.
	convert bool
}

// kf8Doc is a document laid out for flow 0.
type kf8Doc struct {
	// out is the document with aid attributes and kindle: references.
	out []byte
	// ids maps each id in the document to where its element starts.
	ids map[string]int
	// first is where an entry naming the document itself points: its
	// body's first element.
	first int
	frags []fragRange
	// links are the kindle:pos placeholders in out, filled in once every
	// document is laid out.
	links []kf8Link
	// start is where the document starts in flow 0, and firstFrag the
	// book-wide number of its first fragment.
	start, firstFrag int
}

// kf8Link is a link to a place in the book: where its placeholder sits in
// a document's out, and the document and id it names.
type kf8Link struct {
	at       int
	name, id string
}

// linkPlaceholder holds a link's place in a document until every document
// is laid out and the place is known. A kindle:pos reference has a fixed
// width, so filling it in moves nothing after it.
const linkPlaceholder = "kindle:pos:fid:0000:off:0000000000"

// maxFragments is one more than the largest fragment number the four base32
// digits of a kindle:pos reference hold.
const maxFragments = 32 * 32 * 32 * 32

// posURI returns the kindle:pos reference to offset off in fragment fid.
func posURI(fid, off int) string {
	return fmt.Sprintf("kindle:pos:fid:%04s:off:%010s", base32(u32(fid)), base32(u32(off)))
}

// fragRange is a fragment of a document: a run of sibling nodes, and the
// aid of the element holding them.
type fragRange struct {
	start, end int
	parent     string
}

type kf8Layout struct {
	book      *KF8Book
	resources []*kf8Resource
	// embeds and fonts map an image's or a font's name to the record it is
	// stored in, counted from 1 as kindle:embed numbers are.
	embeds, fonts map[string]int
	// flows maps a stylesheet's name to its flow number.
	flows map[string]int
	// cover is the record of the cover, counted from 0, or -1.
	cover   int
	nextAid int
	docs    []*kf8Doc
	byName  map[string]int

	text     []byte
	flowEnds []int
	textRecs [][]byte
	ncx      [][]byte
	skel     [][]byte
	frags    [][]byte
	guide    [][]byte
}

func newKF8Layout(b *KF8Book) *kf8Layout {
	l := &kf8Layout{book: b, embeds: map[string]int{}, fonts: map[string]int{}, flows: map[string]int{}, byName: map[string]int{}, cover: -1}
	for _, img := range b.Images {
		r := &kf8Resource{image: img, mediaType: img.MediaType}
		switch img.MediaType {
		case "image/jpeg", "image/png", "image/gif", "image/bmp":
		case "image/svg+xml":
			continue
		default:
			if !strings.HasPrefix(img.MediaType, "image/") {
				continue
			}
			r.mediaType, r.convert = "image/jpeg", true
		}
		l.resources = append(l.resources, r)
		l.embeds[img.Name] = len(l.resources)
		if img.Name == b.Cover {
			l.cover = len(l.resources) - 1
		}
	}
	fontsSize := 0
	for _, f := range b.Fonts {
		// A Kindle shows TrueType and OpenType fonts. Fonts past what a
		// reader takes together are left out, as are fonts too large for a
		// record.
		if !kindleFont(f.Content) || fontsSize+len(f.Content) > maxFontsSize {
			continue
		}
		rec := fontRecord(f.Content)
		if len(rec) > maxImageRecord {
			continue
		}
		fontsSize += len(f.Content)
		l.resources = append(l.resources, &kf8Resource{record: rec})
		l.fonts[f.Name] = len(l.resources)
	}
	if l.cover >= 0 {
		if cover := l.resources[l.cover]; cover.mediaType != "image/jpeg" && cover.mediaType != "image/gif" {
			// Older Kindles show only a JPEG or GIF cover, so the cover gets
			// a JPEG copy, and the documents keep the original.
			l.resources = append(l.resources, &kf8Resource{image: cover.image, mediaType: "image/jpeg", convert: true})
			l.cover = len(l.resources) - 1
		}
	}
	for i, s := range b.Styles {
		l.flows[s.Name] = i + 1
	}
	for i, d := range b.Documents {
		if _, ok := l.byName[d.Name]; !ok {
			l.byName[d.Name] = i
		}
	}
	return l
}

// layout builds the text and the indexes.
func (l *kf8Layout) layout(ctx context.Context) error {
	start, frags := 0, 0
	for _, d := range l.book.Documents {
		if err := ctx.Err(); err != nil {
			return errors.WithStack(err)
		}
		doc := l.document(d)
		doc.start, doc.firstFrag = start, frags
		start += len(doc.out)
		frags += len(doc.frags)
		l.docs = append(l.docs, doc)
	}
	if frags >= maxFragments {
		return errors.New("the book has too many fragments for a Kindle book")
	}
	for _, doc := range l.docs {
		for _, k := range doc.links {
			fid, off := l.place(k.name, k.id)
			copy(doc.out[k.at:], posURI(fid, off))
		}
	}

	var flow0 bytes.Buffer
	var skeletons, fragments []*ncxEntry
	selectors := &cncxWriter{}
	for i, doc := range l.docs {
		last := 0
		for _, f := range doc.frags {
			flow0.Write(doc.out[last:f.start])
			last = f.end
		}
		flow0.Write(doc.out[last:])
		skelLength := flow0.Len() - doc.start
		skeletons = append(skeletons, &ncxEntry{
			ident: []byte(fmt.Sprintf("SKEL%010d", i)),
			// Calibre writes the start and length twice.
			values: map[byte][]uint32{1: {u32(len(doc.frags))}, 6: {u32(doc.start), u32(skelLength), u32(doc.start), u32(skelLength)}},
		})
		inPart := 0
		for _, f := range doc.frags {
			// A fragment goes in at its place in the assembled document,
			// counted as if the fragments before it were already in.
			insert := doc.start + f.start
			selector := selectors.add([]byte("P-//*[@aid='" + f.parent + "']"))
			fragments = append(fragments, &ncxEntry{
				ident:  []byte(fmt.Sprintf("%010d", insert)),
				values: map[byte][]uint32{2: {selector}, 3: {u32(i)}, 4: {u32(len(fragments))}, 6: {u32(inPart), u32(f.end - f.start)}},
			})
			flow0.Write(doc.out[f.start:f.end])
			inPart += f.end - f.start
		}
	}

	l.text = flow0.Bytes()
	l.flowEnds = []int{len(l.text)}
	for _, s := range l.book.Styles {
		l.text = append(l.text, l.rewriteCSS(s.Name, s.Content)...)
		l.flowEnds = append(l.flowEnds, len(l.text))
	}
	if len(l.text) > maxTextLength {
		return errors.New("the book's text is too long for a Kindle book")
	}
	l.textRecs = textRecords(l.text)

	var err error
	if l.skel, err = writeIndex([]tagxEntry{{tag: 1, valuesPerEntry: 1, mask: 0x03}, {tag: 6, valuesPerEntry: 2, mask: 0x0C}}, skeletons, nil); err != nil {
		return err
	}
	fragTags := []tagxEntry{{tag: 2, valuesPerEntry: 1, mask: 0x01}, {tag: 3, valuesPerEntry: 1, mask: 0x02}, {tag: 4, valuesPerEntry: 1, mask: 0x04}, {tag: 6, valuesPerEntry: 2, mask: 0x08}}
	if l.frags, err = writeIndex(fragTags, fragments, selectors.records); err != nil {
		return err
	}
	if len(l.book.TOC) > 0 {
		if l.ncx, err = l.ncxIndex(); err != nil {
			return err
		}
	}
	if l.guide, err = l.guideIndex(); err != nil {
		return err
	}
	return nil
}

// textRecords cuts text into records and compresses each. A record that
// ends inside a character repeats the bytes that finish it, and their
// count, as its multibyte trailing entry; readers drop them.
func textRecords(text []byte) [][]byte {
	var records [][]byte
	for start := 0; start < len(text); start += textRecordSize {
		end := min(start+textRecordSize, len(text))
		rec := palmDOCCompress(text[start:end])
		n := 0
		for end+n < len(text) && n < 3 && text[end+n]&0xC0 == 0x80 {
			n++
		}
		rec = append(rec, text[end:end+n]...)
		records = append(records, append(rec, byte(n)))
	}
	return records
}

// locate returns the fragment and offset a kindle:pos reference names for
// a place in a document: the fragment it is in, or, for a place in the
// skeleton, the start of the next fragment, where the content it opens
// begins.
func (d *kf8Doc) locate(pos int) (int, int) {
	for k, f := range d.frags {
		if pos < f.end || (pos == f.start && f.start == f.end) {
			return d.firstFrag + k, max(0, pos-f.start)
		}
	}
	last := len(d.frags) - 1
	return d.firstFrag + last, pos - d.frags[last].start
}

// target returns the fragment and offset a table of contents href names,
// and the place in the text it is.
func (l *kf8Layout) target(href string) (int, int, int) {
	name, id, _ := strings.Cut(href, "#")
	if unescaped, err := url.PathUnescape(id); err == nil {
		id = unescaped
	}
	fid, off := l.place(name, id)
	return fid, off, l.docs[l.byName[name]].docStartOf(fid) + off
}

// place returns the fragment and offset of the element with id in the
// document called name, or of the document's first element when id is ""
// or names nothing in it.
func (l *kf8Layout) place(name, id string) (int, int) {
	d := l.docs[l.byName[name]]
	pos, ok := d.ids[id]
	if !ok || id == "" {
		pos = d.first
	}
	return d.locate(pos)
}

// docStartOf returns where fragment fid is inserted in the text.
func (d *kf8Doc) docStartOf(fid int) int {
	return d.start + d.frags[fid-d.firstFrag].start
}

// maxLabel caps a table of contents title, so one absurd title can't
// overflow the index record its label is stored in.
const maxLabel = 1024

// navPoints returns the entries of a table of contents a Kindle can show:
// an entry naming a document the book doesn't have is left out, its
// children taking its place, and a long title is cut.
func (l *kf8Layout) navPoints(points []*NavPoint) []*NavPoint {
	var out []*NavPoint
	for _, p := range points {
		children := l.navPoints(p.Children)
		name, _, _ := strings.Cut(p.Href, "#")
		if _, ok := l.byName[name]; !ok {
			out = append(out, children...)
			continue
		}
		out = append(out, &NavPoint{Title: cutLabel(p.Title), Href: p.Href, Children: children})
	}
	return out
}

// cutLabel cuts a title to maxLabel bytes, at a character boundary.
func cutLabel(title string) string {
	if len(title) <= maxLabel {
		return title
	}
	cut := maxLabel
	for cut > 0 && !utf8.RuneStart(title[cut]) {
		cut--
	}
	return title[:cut]
}

// guideIndex returns the guide index, naming each place the book's guide
// gives as a fragment and an offset, or nil for a book with no guide. A
// reference to a document the book doesn't have is left out, and so is a
// second reference of a type. Entries are in order of their type.
func (l *kf8Layout) guideIndex() ([][]byte, error) {
	refs := map[string]GuideRef{}
	for _, g := range l.book.Guide {
		name, _, _ := strings.Cut(g.Href, "#")
		_, ok := l.byName[name]
		if _, taken := refs[g.Type]; !ok || taken || g.Type == "" || len(g.Type) > maxLabel {
			continue
		}
		refs[g.Type] = g
	}
	if len(refs) == 0 {
		return nil, nil
	}
	labels := &cncxWriter{}
	entries := make([]*ncxEntry, 0, len(refs))
	for _, typ := range slices.Sorted(maps.Keys(refs)) {
		g := refs[typ]
		fid, off, _ := l.target(g.Href)
		entries = append(entries, &ncxEntry{ident: []byte(typ), values: map[byte][]uint32{
			guideTagTitle: {labels.add([]byte(cutLabel(g.Title)))},
			ncxTagPosFID:  {u32(fid), u32(off)},
		}})
	}
	tags := []tagxEntry{{tag: guideTagTitle, valuesPerEntry: 1, mask: 0x01}, {tag: ncxTagPosFID, valuesPerEntry: 2, mask: 0x02}}
	return writeIndex(tags, entries, labels.records)
}

// startReading returns where the guide's "text" entry, the start of the
// book's text, is in the text, or false when the guide has none.
func (l *kf8Layout) startReading() (int, bool) {
	for _, g := range l.book.Guide {
		name, _, _ := strings.Cut(g.Href, "#")
		if _, ok := l.byName[name]; ok && g.Type == "text" {
			_, _, pos := l.target(g.Href)
			return pos, true
		}
	}
	return 0, false
}

// ncxIndex returns the NCX index of the table of contents. Entries are
// listed a level at a time, so each entry's children are a contiguous
// range, with identifiers in lowercase hexadecimal as Calibre writes them.
func (l *kf8Layout) ncxIndex() ([][]byte, error) {
	type pending struct {
		point  *NavPoint
		parent int
	}
	type placed struct {
		point                      *NavPoint
		depth, parent, first, last int
		fid, off, pos              int
	}
	var entries []*placed
	toc := l.navPoints(l.book.TOC)
	level := make([]pending, 0, len(toc))
	for _, p := range toc {
		level = append(level, pending{point: p, parent: -1})
	}
	for depth := 0; len(level) > 0 && depth < maxTOCDepth; depth++ {
		start := len(entries)
		for _, p := range level {
			fid, off, pos := l.target(p.point.Href)
			entries = append(entries, &placed{point: p.point, depth: depth, parent: p.parent, first: -1, last: -1, fid: fid, off: off, pos: pos})
		}
		var next []pending
		for i, p := range level {
			if len(p.point.Children) == 0 || depth+1 >= maxTOCDepth {
				continue
			}
			e := entries[start+i]
			e.first = start + len(level) + len(next)
			for _, c := range p.point.Children {
				next = append(next, pending{point: c, parent: start + i})
			}
			e.last = start + len(level) + len(next) - 1
		}
		level = next
	}
	if len(entries) > maxNCXEntries {
		return nil, errors.New("the table of contents has too many entries")
	}

	// An entry's length runs to the next entry in reading order.
	var order []*placed
	var walk func(i int)
	walk = func(i int) {
		order = append(order, entries[i])
		if e := entries[i]; e.first >= 0 {
			for c := e.first; c <= e.last && c < len(entries); c++ {
				walk(c)
			}
		}
	}
	for i, e := range entries {
		if e.depth == 0 {
			walk(i)
		}
	}
	length := map[*placed]int{}
	for i, e := range order {
		end := l.flowEnds[0]
		if i+1 < len(order) && order[i+1].pos > e.pos {
			end = order[i+1].pos
		}
		length[e] = max(0, end-e.pos)
	}

	labels := &cncxWriter{}
	out := make([]*ncxEntry, 0, len(entries))
	for i, e := range entries {
		values := map[byte][]uint32{
			ncxTagPos:    {u32(e.pos)},
			ncxTagLength: {u32(length[e])},
			ncxTagLabel:  {labels.add([]byte(e.point.Title))},
			ncxTagDepth:  {u32(e.depth)},
			ncxTagPosFID: {u32(e.fid), u32(e.off)},
		}
		if e.parent >= 0 {
			values[ncxTagParent] = []uint32{u32(e.parent)}
		}
		if e.first >= 0 {
			values[ncxTagFirstChild] = []uint32{u32(e.first)}
			values[ncxTagLastChild] = []uint32{u32(e.last)}
		}
		out = append(out, &ncxEntry{ident: []byte(fmt.Sprintf("%02x", i)), values: values})
	}
	tags := []tagxEntry{
		{tag: ncxTagPos, valuesPerEntry: 1, mask: 0x01},
		{tag: ncxTagLength, valuesPerEntry: 1, mask: 0x02},
		{tag: ncxTagLabel, valuesPerEntry: 1, mask: 0x04},
		{tag: ncxTagDepth, valuesPerEntry: 1, mask: 0x08},
		{tag: ncxTagParent, valuesPerEntry: 1, mask: 0x10},
		{tag: ncxTagFirstChild, valuesPerEntry: 1, mask: 0x20},
		{tag: ncxTagLastChild, valuesPerEntry: 1, mask: 0x40},
		{tag: ncxTagPosFID, valuesPerEntry: 2, mask: 0x80},
	}
	return writeIndex(tags, out, labels.records)
}

// write writes the file: the records after the space the Palm database
// header and record list take, then those.
func (l *kf8Layout) write(ctx context.Context, w io.WriteSeeker) error {
	textCount := len(l.textRecs)
	ncxAt := 1 + textCount
	skelAt := ncxAt + len(l.ncx)
	fragAt := skelAt + len(l.skel)
	guideAt := fragAt + len(l.frags)
	imageAt := guideAt + len(l.guide)
	fdstAt := imageAt + len(l.resources)
	count := fdstAt + 4 // FDST, FLIS, FCIS, EOF
	if count > 0xFFFF {
		return errors.New("the book has too many records for a Kindle book")
	}

	rec0, err := l.headerRecord(kf8Indexes{
		firstNonBook: ncxAt, ncx: ncxAt, skeleton: skelAt, fragment: fragAt,
		guide: guideAt, firstImage: imageAt, fdst: fdstAt, flis: fdstAt + 1, fcis: fdstAt + 2,
	})
	if err != nil {
		return err
	}
	fdst := make([]byte, 12+8*len(l.flowEnds))
	copy(fdst, "FDST")
	binary.BigEndian.PutUint32(fdst[4:], 12)
	binary.BigEndian.PutUint32(fdst[8:], u32(len(l.flowEnds)))
	start := 0
	for i, end := range l.flowEnds {
		binary.BigEndian.PutUint32(fdst[12+8*i:], u32(start))
		binary.BigEndian.PutUint32(fdst[16+8*i:], u32(end))
		start = end
	}

	listEnd := int64(pdbHeaderSize + count*pdbRecordEntry + 2)
	if _, err := w.Seek(listEnd, io.SeekStart); err != nil {
		return errors.WithStack(err)
	}
	offsets := make([]int64, 0, count)
	at := listEnd
	put := func(rec []byte) error {
		if at+int64(len(rec)) > 0xFFFFFFFF {
			return errors.New("the book is too large for a Kindle book")
		}
		offsets = append(offsets, at)
		_, err := w.Write(rec)
		at += int64(len(rec))
		return errors.WithStack(err)
	}
	records := [][]byte{rec0}
	records = append(records, l.textRecs...)
	records = append(records, l.ncx...)
	records = append(records, l.skel...)
	records = append(records, l.frags...)
	records = append(records, l.guide...)
	for _, rec := range records {
		if err := put(rec); err != nil {
			return err
		}
	}
	for _, r := range l.resources {
		if err := ctx.Err(); err != nil {
			return errors.WithStack(err)
		}
		if r.record != nil {
			if err := put(r.record); err != nil {
				return err
			}
			continue
		}
		data, err := r.image.Load()
		if err != nil {
			return errors.Wrapf(err, "read %s", r.image.Name)
		}
		if r.convert {
			// An image that can't be decoded is stored as it is, so the
			// book still opens with that image missing.
			if jpg, err := normalizeCover(data); err == nil {
				data = jpg
			}
		}
		if err := put(data); err != nil {
			return err
		}
	}
	for _, rec := range [][]byte{fdst, flisRecord, fcisRecord(len(l.text)), eofRecord} {
		if err := put(rec); err != nil {
			return err
		}
	}

	header := make([]byte, pdbHeaderSize, listEnd)
	copy(header, pdbName(l.book.Metadata.Title))
	now := u32(int(time.Now().Unix()))
	binary.BigEndian.PutUint32(header[36:], now) // created
	binary.BigEndian.PutUint32(header[40:], now) // modified
	copy(header[pdbTypeOffset:], "BOOKMOBI")
	binary.BigEndian.PutUint32(header[pdbUniqueIDSeed:], u32(2*count-1))
	binary.BigEndian.PutUint16(header[pdbRecordsOffset:], uint16(count)) //nolint:gosec // checked against 0xFFFF above
	for i, off := range offsets {
		var entry [pdbRecordEntry]byte
		binary.BigEndian.PutUint32(entry[:], uint32(off))        //nolint:gosec // checked against 0xFFFFFFFF in put
		binary.BigEndian.PutUint32(entry[4:], u32(2*i)&0xFFFFFF) // attributes 0, unique ID 2i
		header = append(header, entry[:]...)
	}
	header = append(header, 0, 0)
	if _, err := w.Seek(0, io.SeekStart); err != nil {
		return errors.WithStack(err)
	}
	_, err = w.Write(header)
	return errors.WithStack(err)
}

// kf8Indexes are the record indexes the header names.
type kf8Indexes struct {
	firstNonBook, ncx, skeleton, fragment, guide, firstImage, fdst, flis, fcis int
}

// headerRecord returns the header record: the PalmDOC and MOBI headers, then
// the EXTH block and full name rebuildHeaderRecord writes from the
// metadata, as it does for a rewritten file.
func (l *kf8Layout) headerRecord(at kf8Indexes) ([]byte, error) {
	be := binary.BigEndian
	rec := make([]byte, offMOBIMagic+kf8HeaderLength)
	be.PutUint16(rec[offCompression:], compressionPalmDOC)
	be.PutUint32(rec[4:], u32(len(l.text)))
	be.PutUint16(rec[offTextRecords:], uint16(len(l.textRecs))) //nolint:gosec // the record count is checked against 0xFFFF
	be.PutUint16(rec[10:], textRecordSize)
	copy(rec[offMOBIMagic:], "MOBI")
	be.PutUint32(rec[offHeaderLength:], kf8HeaderLength)
	be.PutUint32(rec[24:], 2) // a book
	be.PutUint32(rec[offTextEncoding:], encodingUTF8)
	id := fnv.New32a()
	id.Write([]byte(l.book.Identifier))
	be.PutUint32(rec[32:], id.Sum32())
	be.PutUint32(rec[offVersion:], 8)
	for off := 40; off < offFirstNonBook; off += 4 {
		be.PutUint32(rec[off:], nullIndex) // orthographic and inflection indexes
	}
	be.PutUint32(rec[offFirstNonBook:], u32(at.firstNonBook))
	be.PutUint32(rec[92:], languageLocale(l.book.Metadata.Language))
	be.PutUint32(rec[104:], 8) // the reader version the book needs
	firstImage := uint32(nullIndex)
	if len(l.resources) > 0 {
		firstImage = u32(at.firstImage)
	}
	be.PutUint32(rec[offFirstImage:], firstImage)
	be.PutUint32(rec[offEXTHFlags:], 0x50) // an EXTH block follows
	be.PutUint32(rec[164:], nullIndex)     // unknown; Calibre writes the null index
	be.PutUint32(rec[168:], nullIndex)     // no DRM
	be.PutUint32(rec[offFDST:], u32(at.fdst))
	be.PutUint32(rec[offFDSTCount:], u32(len(l.flowEnds)))
	be.PutUint32(rec[offFCIS:], u32(at.fcis))
	be.PutUint32(rec[offFCIS+4:], 1)
	be.PutUint32(rec[offFLIS:], u32(at.flis))
	be.PutUint32(rec[offFLIS+4:], 1)
	// Indexes the book has none of, and unknown fields, are the null index.
	for _, off := range []int{offSRCS, 232, 236, offNCX, offFragment, offSkeleton, offDATP, offGuide, 264, 268, 272, 276} {
		be.PutUint32(rec[off:], nullIndex)
	}
	be.PutUint32(rec[offExtraFlags:], extraFlagsMBEntry)
	if len(l.ncx) > 0 {
		be.PutUint32(rec[offNCX:], u32(at.ncx))
	}
	be.PutUint32(rec[offFragment:], u32(at.fragment))
	be.PutUint32(rec[offSkeleton:], u32(at.skeleton))
	if len(l.guide) > 0 {
		be.PutUint32(rec[offGuide:], u32(at.guide))
	}

	count := make([]byte, 4)
	be.PutUint32(count, u32(len(l.resources)))
	exth := []exthRecord{
		{typ: exthResourceCount, data: count},
	}
	if l.book.Identifier != "" {
		// Other writers put the identifier here as well, so a Kindle that
		// reads this record finds the same one.
		exth = append(exth, exthRecord{typ: exthASINAlt, data: []byte(l.book.Identifier)})
	}
	if start, ok := l.startReading(); ok {
		exth = append(exth, exthRecord{typ: exthStartReading, data: binary.BigEndian.AppendUint32(nil, u32(start))})
	}
	if l.book.FixedLayout {
		// The records Kindle comics carry, tested on a Kindle as a set.
		// Without region magnification a Kindle trims each page's blank
		// margins, cutting into pages whose art runs close to them.
		exth = append(exth,
			exthRecord{typ: exthFixedLayout, data: []byte("true")},
			exthRecord{typ: exthBookType, data: []byte("comic")},
			exthRecord{typ: exthOrientationLock, data: []byte("none")},
			exthRecord{typ: exthZeroGutter, data: []byte("true")},
			exthRecord{typ: exthZeroMargin, data: []byte("true")},
			exthRecord{typ: exthRegionMagnification, data: []byte("true")})
		if l.book.OriginalResolution != "" {
			exth = append(exth,
				exthRecord{typ: exthOriginalResolutionAlt, data: []byte(l.book.OriginalResolution)},
				exthRecord{typ: exthOriginalResolution, data: []byte(l.book.OriginalResolution)})
		}
	}
	rec = append(rec, encodeEXTH(exth)...)

	meta := l.book.Metadata
	meta.Cover, meta.TOC, meta.OnTOCSkipped = nil, nil, nil
	ids := &Identifiers{ASIN: l.book.Identifier}
	if meta.Identifiers != nil {
		ids.ISBNs = meta.Identifiers.ISBNs
	}
	meta.Identifiers = ids
	edits := headerEdits{kf8Index: -1, kf8URI: true}
	if l.cover >= 0 {
		edits.cover = &coverRecords{offset: u32(l.cover)}
	}
	rebuilt, err := rebuildHeaderRecord(rec, &header{encoding: encodingUTF8}, &meta, edits)
	if err != nil {
		return nil, err
	}
	return append(rebuilt, make([]byte, headerPadding)...), nil
}

// pdbName is the Palm database name: the title in ASCII letters and
// digits, with underscores for the rest, cut to 31 bytes.
func pdbName(title string) []byte {
	name := make([]byte, 0, 31)
	for _, r := range title {
		if len(name) == 31 {
			break
		}
		if r < 0x80 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			name = append(name, byte(r))
		} else {
			name = append(name, '_')
		}
	}
	if len(name) == 0 {
		name = append(name, "Unknown"...)
	}
	return name
}

// languageLocales are Windows primary language IDs, which a MOBI header's
// locale holds.
var languageLocales = map[string]uint32{
	"ar": 0x01, "ca": 0x03, "zh": 0x04, "cs": 0x05, "da": 0x06, "de": 0x07, "el": 0x08,
	"en": 0x09, "es": 0x0A, "fi": 0x0B, "fr": 0x0C, "he": 0x0D, "hu": 0x0E, "it": 0x10,
	"ja": 0x11, "ko": 0x12, "nl": 0x13, "nb": 0x14, "no": 0x14, "pl": 0x15, "pt": 0x16,
	"ro": 0x18, "ru": 0x19, "sv": 0x1D, "tr": 0x1F, "uk": 0x22, "hi": 0x39,
}

// languageLocale returns the locale for a language tag, English when it is
// not one listed.
func languageLocale(language string) uint32 {
	primary, _, _ := strings.Cut(strings.ToLower(language), "-")
	if locale, ok := languageLocales[primary]; ok {
		return locale
	}
	return languageLocales["en"]
}

func u32(n int) uint32 {
	return uint32(n) //nolint:gosec // sizes, counts, and offsets the writer checks or keeps small
}
