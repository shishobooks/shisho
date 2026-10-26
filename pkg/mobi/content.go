package mobi

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"golang.org/x/text/encoding/charmap"
)

// UnsupportedError is returned by ReadContent for a file it cannot convert.
type UnsupportedError struct {
	Reason string
}

func (e *UnsupportedError) Error() string {
	return "this Kindle book can't be converted: " + e.Reason
}

func unsupported(reason string) error {
	return errors.WithStack(&UnsupportedError{Reason: reason})
}

// Content is the content of the book in a MOBI or AZW3 file, ready to be
// written as an EPUB: every link and reference is a relative path, so the
// documents, styles, fonts, and images can sit side by side in one
// directory.
type Content struct {
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
	// Fonts are the fonts the styles name, decoded from their records.
	Fonts []Document
	// TOC is the table of contents.
	TOC []*NavPoint

	db *pdb
	// hrefs maps each location the book or its table of contents names
	// (kindle:pos or filepos) to the href it became.
	hrefs map[string]string
}

// Document is a document, style, or font of a book.
type Document struct {
	Name      string
	MediaType string
	Content   []byte
}

// Resource is an image of a book, read from its record when written.
type Resource struct {
	Name      string
	MediaType string
	Size      int64
	record    int
}

// NavPoint is an entry of a book's table of contents. Href is a
// document's name and, after a #, an id in it.
type NavPoint struct {
	Title    string
	Href     string
	Children []*NavPoint
}

// ReadResource returns a resource's data.
func (k *Content) ReadResource(r *Resource) ([]byte, error) {
	return k.db.record(r.record, maxImageRecord)
}

// Href returns the href a location (a TOCEntry's Locator, as a stored
// chapter holds it) became, and false for one that names no place in the
// book.
func (k *Content) Href(locator string) (string, bool) {
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

// ReadContent reads the book in a MOBI or AZW3 file: the KF8 book of an
// AZW3 or a combo file, or else the MOBI6 book. It reads the text and
// indexes and the first bytes of each resource record, not the images,
// which ReadResource reads. It declines a DRM-protected file with
// ErrDRMProtected, and returns an UnsupportedError for a file it cannot
// convert.
func ReadContent(ctx context.Context, r io.ReaderAt, size int64) (*Content, error) {
	db, err := readPDB(r, size)
	if err != nil {
		return nil, err
	}
	b, err := readBook(db)
	if err != nil {
		return nil, err
	}
	// The books are listed KF8 first.
	ref := b.books[0]
	if ref.kind == BookKF8 {
		return readKF8(ctx, db, b, ref)
	}
	return readKF7(ctx, db, b, ref)
}

func partName(i int) string {
	return fmt.Sprintf("part%04d.xhtml", i)
}

func (k *Content) navPoints(entries []*TOCEntry) []*NavPoint {
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

// toUTF8 decodes Windows-1252 text, which many MOBI6 books hold, and a KF8
// book's header may still claim.
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
// entries, decompressed, up to the text length the header gives. With all
// set, it is every record's text, and the length only the least it may be:
// KindleGen gives a KF8 book's length as the end of its documents, though
// the records hold its stylesheets after them. A MOBI6 book's length is
// right, and old ones may hold a record past it that does not decompress.
func readText(ctx context.Context, db *pdb, base int, rec []byte, compression uint16, all bool) ([]byte, error) {
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
	var huff *huffDecoder
	if compression == compressionHuffCDIC {
		var err error
		if huff, err = readHuffCDIC(db, base, rec); err != nil {
			return nil, err
		}
	}
	// A record decompresses to about 4 KiB, so a header claiming more text
	// than its records hold does not get its whole length allocated.
	text := make([]byte, 0, min(length, int64(count)*8192))
	for i := base + 1; i <= base+count && (all || int64(len(text)) < length); i++ {
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
		switch compression {
		case compressionPalmDOC:
			data, err = palmDOCDecompress(data)
		case compressionHuffCDIC:
			data, err = huff.decompress(data, maxTextRecord)
		}
		if err != nil {
			return nil, errors.Wrapf(err, "text record %d", i)
		}
		if len(text)+len(data) > maxTextLength {
			return nil, errors.New("MOBI text is too long")
		}
		text = append(text, data...)
	}
	if int64(len(text)) < length {
		return nil, errors.New("MOBI text is shorter than its header says")
	}
	if !all {
		text = text[:length]
	}
	return text, nil
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

// resources is the resource section of a file: the records from the first
// image to the first record that ends it.
type resources struct {
	all []*Resource
	// byRecord maps a record to its Resource, for the image records.
	byRecord map[int]*Resource
	// fonts holds the FONT records.
	fonts map[int]bool
	first int
	count int
}

// recordsAfterResources open the records that follow a book's resources.
var recordsAfterResources = [][]byte{
	[]byte("FLIS"), []byte("FCIS"), []byte("FDST"), []byte("DATP"), []byte("SRCS"),
	[]byte("CMET"), []byte("BOUNDARY"), {0xE9, 0x8E, 0x0D, 0x0A},
}

// scanResources reads the first bytes of each resource record, so the
// images and fonts are known without reading them.
func scanResources(db *pdb, first uint32) (*resources, error) {
	res := &resources{byRecord: map[int]*Resource{}, fonts: map[int]bool{}, first: int(first)}
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
			res.fonts[i] = true
			continue
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

// fontByEmbed returns the FONT record a kindle:embed number names.
func (r *resources) fontByEmbed(n uint64) (int, bool) {
	if n == 0 || n > uint64(r.count) { //nolint:gosec // count is a record count
		return 0, false
	}
	record := r.first + int(n) - 1 //nolint:gosec // n is at most count
	return record, r.fonts[record]
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
