package mobi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // decode PNG and WebP covers before re-encoding them as JPEG
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/identifiers"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // see image/png
	"golang.org/x/text/encoding/charmap"
)

// Metadata is what Rewrite writes into a file. A zero field leaves the
// file's own value in place, the way the EPUB generator keeps an OPF value
// Shisho has nothing for. Series, tags, roles, and narrators have no place in
// the format, so there are no fields for them.
type Metadata struct {
	Title       string
	Authors     []string
	Description string
	Publisher   string
	ReleaseDate *time.Time
	Language    string
	// Identifiers replace the file's ISBN and ASIN records when set; nil
	// keeps them.
	Identifiers *Identifiers
	Genres      []string
	// Cover is the new cover image. A JPEG or GIF is written as is; any
	// other image is re-encoded as JPEG, since a MOBI6 reader shows no PNG
	// and a combo file's images belong to its MOBI6 book.
	Cover []byte
}

// Identifiers are the identifiers MOBI can hold.
type Identifiers struct {
	ISBNs []string
	ASIN  string
}

// eofRecord is the record Calibre and KindleGen end a file with.
var eofRecord = []byte{0xE9, 0x8E, 0x0D, 0x0A}

// trailerMagics open the records that follow a book's images. A new cover
// record goes before the first of them, after every existing image, so no
// image a book's text refers to changes its index.
var trailerMagics = []string{"FLIS", "FCIS", "FDST", "DATP", "SRCS", "CMET", "BOUNDARY"}

// isTrailerRecord reports whether a record, given its first bytes and its
// size, is one of the trailer records.
func isTrailerRecord(head []byte, size int64) bool {
	if size == int64(len(eofRecord)) && bytes.Equal(head, eofRecord) {
		return true
	}
	for _, magic := range trailerMagics {
		if bytes.HasPrefix(head, []byte(magic)) {
			return true
		}
	}
	return false
}

// Rewrite copies the MOBI or AZW3 file in src to dst with meta written into
// it. Both header records of a combo file are rewritten. Only header records
// and the cover and thumbnail records change; every other record is copied
// byte for byte.
func Rewrite(src io.ReaderAt, size int64, dst io.Writer, meta *Metadata) error {
	f, err := readPDBFile(src, size)
	if err != nil {
		return err
	}

	rec0, err := f.load(0)
	if err != nil {
		return err
	}
	first, err := parseHeader(rec0)
	if err != nil {
		return err
	}
	if first.encrypted {
		return errors.WithStack(ErrDRMProtected)
	}

	kf8Index := -1
	var kf8 *header
	if !first.isKF8() {
		if index, ok := first.number(exthKF8Boundary); ok && index > 0 && int64(index) < int64(len(f.records)) {
			rec, err := f.load(int(index))
			if err != nil {
				return err
			}
			if h, err := parseHeader(rec); err == nil && h.isKF8() {
				if h.encrypted {
					return errors.WithStack(ErrDRMProtected)
				}
				kf8Index, kf8 = int(index), h
			}
		}
	}

	var newCover *coverRecords
	if meta.Cover != nil {
		cover, err := normalizeCover(meta.Cover)
		if err != nil {
			return err
		}
		if newCover, err = f.placeCover(cover, first, kf8, &kf8Index); err != nil {
			return err
		}
	}

	hasKF8 := first.isKF8() || kf8Index >= 0
	rec, err := rebuildHeaderRecord(f.records[0], first, meta, headerEdits{cover: newCover, kf8Index: kf8Index, kf8URI: hasKF8, countsImages: true})
	if err != nil {
		return err
	}
	f.records[0] = rec
	if kf8Index >= 0 {
		rec, err := rebuildHeaderRecord(f.records[kf8Index], kf8, meta, headerEdits{cover: newCover, kf8Index: -1, kf8URI: true})
		if err != nil {
			return err
		}
		f.records[kf8Index] = rec
	}

	return f.writeTo(dst)
}

// coverRecords is where the cover sits after Rewrite places it: its offset
// from the first image, which the cover and thumbnail EXTH records hold.
type coverRecords struct {
	offset   uint32
	inserted bool
}

// placeCover puts the cover into the file. It replaces the record the file
// names as its cover, and the thumbnail record with a small copy, since a
// Kindle shows the thumbnail in its library. A file with no cover gets a new record after its
// last image, and the record indexes that follow it are shifted; it returns
// the offset to record in the EXTH block then, and nil when the offsets the
// file holds still apply. kf8Index is moved with the KF8 header record.
//
// In a combo file the images belong to the MOBI6 book, so the cover is placed
// through the MOBI6 header and the new record goes in the MOBI6 half.
func (f *pdbFile) placeCover(cover []byte, first, kf8 *header, kf8Index *int) (*coverRecords, error) {
	// end is where the book that owns the images ends.
	end := len(f.records)
	if *kf8Index >= 0 {
		end = *kf8Index
	}
	// Calibre gives a book with no images a first-image index past its end,
	// not the null index.
	firstImage := first.firstImage
	hasImages := firstImage != nullIndex && int64(firstImage) < int64(end)

	if hasImages {
		offset, found := coverOffset(first, kf8)
		if index, ok := f.imageIndex(firstImage, offset, found); ok {
			f.records[index] = cover
			thumbOffset, found := numberIn(exthThumbOffset, first, kf8)
			if thumb, ok := f.imageIndex(firstImage, thumbOffset, found); ok && thumb != index {
				thumbnail, err := thumbnailOf(cover)
				if err != nil {
					return nil, err
				}
				f.records[thumb] = thumbnail
			}
			return nil, nil
		}
	}

	start := int(firstImage)
	if !hasImages {
		start = int(binary.BigEndian.Uint32(f.records[0][offFirstNonBook:]))
	}
	if start <= 0 || start >= end {
		start = 1
	}
	p := end
	for i := start; i < end; i++ {
		if isTrailerRecord(f.head(i), f.size(i)) {
			p = i
			break
		}
	}

	shiftIndexes(f.records[0], 0, p)
	if *kf8Index >= 0 {
		shiftIndexes(f.records[*kf8Index], *kf8Index, p)
		if *kf8Index >= p {
			*kf8Index++
		}
	}
	f.insert(p, cover)

	// A first-image index that named the record now after the cover (a
	// file with no images may name its first trailer record) names the
	// cover instead.
	if !hasImages || int64(firstImage) == int64(p) {
		firstImage = uint32(p) //nolint:gosec // a Palm database has at most 65535 records
		binary.BigEndian.PutUint32(f.records[0][offFirstImage:], firstImage)
	}
	return &coverRecords{offset: uint32(p) - firstImage, inserted: true}, nil //nolint:gosec // as above
}

// coverOffset returns the cover's offset from the first image: the
// cover-offset record of either header, or the kindle:embed URI a KF8 file
// may name it by instead, whose base-32 number counts from 1.
func coverOffset(headers ...*header) (uint32, bool) {
	if offset, ok := numberIn(exthCoverOffset, headers...); ok {
		return offset, true
	}
	for _, h := range headers {
		if h == nil {
			continue
		}
		uri := h.text(exthKF8CoverURI)
		n, err := strconv.ParseUint(strings.TrimPrefix(uri, "kindle:embed:"), 32, 32)
		if strings.HasPrefix(uri, "kindle:embed:") && err == nil && n > 0 {
			return uint32(n - 1), true //nolint:gosec // ParseUint with bitSize 32 keeps n within uint32
		}
	}
	return 0, false
}

// numberIn returns the first numeric EXTH record of a type found in headers.
func numberIn(typ uint32, headers ...*header) (uint32, bool) {
	for _, h := range headers {
		if h == nil {
			continue
		}
		if n, ok := h.number(typ); ok && n != nullIndex {
			return n, true
		}
	}
	return 0, false
}

// imageIndex returns the absolute index of the image at offset from
// firstImage, and false when it names no image record.
func (f *pdbFile) imageIndex(firstImage, offset uint32, ok bool) (int, bool) {
	if !ok {
		return 0, false
	}
	index := int64(firstImage) + int64(offset)
	if index >= int64(len(f.records)) || imageMimeType(f.head(int(index))) == "" {
		return 0, false
	}
	return int(index), true
}

// shiftIndexes adds one to every record index in the header record at base
// that names a record at or after p, the index a record is being inserted
// at. A KF8 header's indexes count from the KF8 header itself, so an insert
// before it moves none of them. The first non-book index marks where a
// region starts, not a record, so a record inserted right at it joins the
// region and the index stays.
func shiftIndexes(rec []byte, base, p int) {
	if p <= base || len(rec) < minHeaderRecord {
		return
	}
	be := binary.BigEndian
	end := min(offMOBIMagic+int(be.Uint32(rec[offHeaderLength:])), len(rec))
	shift := func(off int, strict bool) {
		if off+4 > end {
			return
		}
		v := be.Uint32(rec[off:])
		if v == nullIndex {
			return
		}
		if abs := base + int(v); abs > p || (abs == p && !strict) {
			be.PutUint32(rec[off:], v+1)
		}
	}

	shift(offFirstNonBook, true)
	for _, off := range []int{offFirstImage, offHuffRecord, offFCIS, offFLIS, offSRCS, offNCX} {
		shift(off, false)
	}
	if be.Uint32(rec[offVersion:]) >= 8 {
		for _, off := range []int{offFDST, offFragment, offSkeleton, offDATP, offGuide} {
			shift(off, false)
		}
		return
	}
	// The last content record is the last image. The inserted record is an
	// image placed after every other image, so it becomes the last content
	// record unless that already lies beyond it.
	if offLastContent+2 <= end {
		v := be.Uint16(rec[offLastContent:])
		switch {
		case v == 0xFFFF:
		case base+int(v) >= p:
			be.PutUint16(rec[offLastContent:], v+1)
		default:
			be.PutUint16(rec[offLastContent:], uint16(p-base)) //nolint:gosec // a Palm database has at most 65535 records
		}
	}
}

// Calibre's thumbnails fit in 180 by 240 pixels.
const (
	thumbnailWidth  = 180
	thumbnailHeight = 240
)

// thumbnailOf returns the cover scaled down to fit a thumbnail, as a JPEG,
// or the cover itself when it already fits.
func thumbnailOf(cover []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(cover))
	if err != nil {
		return nil, errors.Wrap(err, "decode cover image")
	}
	if cfg.Width <= thumbnailWidth && cfg.Height <= thumbnailHeight {
		return cover, nil
	}
	img, _, err := image.Decode(bytes.NewReader(cover))
	if err != nil {
		return nil, errors.Wrap(err, "decode cover image")
	}
	scale := min(float64(thumbnailWidth)/float64(cfg.Width), float64(thumbnailHeight)/float64(cfg.Height))
	w := max(1, int(float64(cfg.Width)*scale+0.5))
	h := max(1, int(float64(cfg.Height)*scale+0.5))
	// JPEG has no transparency, so a transparent GIF is flattened onto white.
	thumb := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(thumb, thumb.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(thumb, thumb.Bounds(), img, img.Bounds(), draw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: 90}); err != nil {
		return nil, errors.Wrap(err, "encode thumbnail")
	}
	return buf.Bytes(), nil
}

// normalizeCover returns the cover as a JPEG or GIF.
func normalizeCover(data []byte) ([]byte, error) {
	switch imageMimeType(data) {
	case "image/jpeg", "image/gif":
		return data, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrap(err, "decode cover image")
	}
	// JPEG has no transparency; flatten onto white as a reader would show it.
	flat := image.NewRGBA(img.Bounds())
	draw.Draw(flat, flat.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), img, img.Bounds().Min, draw.Over)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: 90}); err != nil {
		return nil, errors.Wrap(err, "encode cover image")
	}
	return buf.Bytes(), nil
}

// headerEdits are the changes to one header record beyond the metadata.
type headerEdits struct {
	// cover, when set, is a cover Rewrite added, whose offset the cover
	// and thumbnail records must name.
	cover *coverRecords
	// kf8Index, when not negative, is the KF8 header's index for a combo
	// file's MOBI6 header to record.
	kf8Index int
	// kf8URI writes the cover's kindle:embed URI as well.
	kf8URI bool
	// countsImages marks the header whose resource count includes the
	// images: the first one, since a combo file's images are its MOBI6 book's.
	countsImages bool
}

// rebuildHeaderRecord returns the header record with a new EXTH block and
// full name. The header itself is kept, apart from the EXTH flag and the full
// name's offset and length; EXTH records Shisho does not write are kept in
// their order.
func rebuildHeaderRecord(rec []byte, h *header, meta *Metadata, edits headerEdits) ([]byte, error) {
	be := binary.BigEndian
	prefixEnd := int64(offMOBIMagic) + int64(be.Uint32(rec[offHeaderLength:]))
	if prefixEnd < minHeaderRecord || prefixEnd > int64(len(rec)) {
		return nil, errors.New("MOBI header length is invalid")
	}

	var kept []exthRecord
	if be.Uint32(rec[offEXTHFlags:])&exthFlagPresent != 0 {
		records, err := parseEXTHRecords(rec, prefixEnd)
		if err != nil {
			return nil, err
		}
		kept = records
	}

	replaced := make(map[uint32]bool)
	var added []exthRecord
	addText := func(typ uint32, values ...string) {
		replaced[typ] = true
		for _, v := range values {
			added = append(added, exthRecord{typ: typ, data: h.encode(v)})
		}
	}
	addNumber := func(typ, value uint32) {
		replaced[typ] = true
		data := make([]byte, 4)
		be.PutUint32(data, value)
		added = append(added, exthRecord{typ: typ, data: data})
	}

	if meta.Title != "" {
		addText(exthUpdatedTitle, meta.Title)
	}
	if len(meta.Authors) > 0 {
		addText(exthAuthor, meta.Authors...)
	}
	if meta.Description != "" {
		addText(exthDescription, meta.Description)
	}
	if meta.Publisher != "" {
		// The parser prefers the imprint, so an old one would hide the
		// publisher Shisho wrote.
		replaced[exthImprint] = true
		addText(exthPublisher, meta.Publisher)
	}
	if meta.ReleaseDate != nil {
		addText(exthPublishingDate, meta.ReleaseDate.UTC().Format("2006-01-02T15:04:05-07:00"))
	}
	if meta.Language != "" {
		addText(exthLanguage, meta.Language)
	}
	if len(meta.Genres) > 0 {
		addText(exthSubject, meta.Genres...)
	}

	// Calibre writes the book's UUID into the ASIN record and
	// "calibre:<uuid>" into the source record. Those stay unless an ASIN
	// replaces them; an ASIN or ISBN Shisho no longer has goes.
	var dropASIN, dropSourceISBN bool
	if ids := meta.Identifiers; ids != nil {
		addText(exthISBN, ids.ISBNs...)
		dropSourceISBN = true
		if ids.ASIN != "" {
			addText(exthASIN, ids.ASIN)
		} else {
			dropASIN = true
		}
	}

	if edits.cover != nil {
		// Calibre counts the image records; the new cover is one more.
		if count, ok := h.number(exthResourceCount); ok && edits.cover.inserted && edits.countsImages {
			addNumber(exthResourceCount, count+1)
		}
		addNumber(exthCoverOffset, edits.cover.offset)
		addNumber(exthThumbOffset, edits.cover.offset)
		addNumber(exthFakeCover, 0)
		if edits.kf8URI {
			addText(exthKF8CoverURI, kf8EmbedURI(edits.cover.offset))
		}
	}
	if edits.kf8Index >= 0 {
		addNumber(exthKF8Boundary, uint32(edits.kf8Index)) //nolint:gosec // a record index
	}

	exth := make([]exthRecord, 0, len(kept)+len(added))
	for _, r := range kept {
		switch {
		case replaced[r.typ]:
			continue
		case dropASIN && r.typ == exthASIN && identifiers.DetectType(h.decode(r.data), "") == identifiers.TypeASIN:
			continue
		case dropSourceISBN && r.typ == exthSource && strings.HasPrefix(strings.ToLower(h.decode(r.data)), "urn:isbn:"):
			continue
		}
		exth = append(exth, r)
	}
	exth = append(exth, added...)

	nameOffset := int64(be.Uint32(rec[offFullNameOffset:]))
	nameLength := int64(be.Uint32(rec[offFullNameLength:]))
	var name []byte
	padding := 2
	if nameOffset >= prefixEnd && nameOffset+nameLength <= int64(len(rec)) {
		name = rec[nameOffset : nameOffset+nameLength]
		// Keep the zero padding Calibre leaves so a Kindle can rewrite the
		// record in place.
		padding = max(padding, int(int64(len(rec))-nameOffset-nameLength))
	}
	if meta.Title != "" {
		name = h.encode(meta.Title)
	}

	var out bytes.Buffer
	out.Write(rec[:prefixEnd])
	out.Write(encodeEXTH(exth))
	newNameOffset := out.Len()
	out.Write(name)
	padding += (4 - (out.Len()+padding)%4) % 4
	out.Write(make([]byte, padding))

	result := out.Bytes()
	be.PutUint32(result[offEXTHFlags:], be.Uint32(result[offEXTHFlags:])|exthFlagPresent)
	be.PutUint32(result[offFullNameOffset:], uint32(newNameOffset)) //nolint:gosec // a header record is small
	be.PutUint32(result[offFullNameLength:], uint32(len(name)))     //nolint:gosec // as above
	return result, nil
}

// kf8EmbedURI names the image at offset from the first image the way KF8
// does: a four-digit base-32 number counting from 1.
func kf8EmbedURI(offset uint32) string {
	n := strings.ToUpper(strconv.FormatUint(uint64(offset)+1, 32))
	return fmt.Sprintf("kindle:embed:%04s", n)
}

// encodeEXTH writes an EXTH block, padded to a multiple of four bytes.
func encodeEXTH(records []exthRecord) []byte {
	be := binary.BigEndian
	var body bytes.Buffer
	for _, r := range records {
		_ = binary.Write(&body, be, r.typ)
		_ = binary.Write(&body, be, uint32(8+len(r.data))) //nolint:gosec // EXTH values are small
		body.Write(r.data)
	}
	pad := (4 - body.Len()%4) % 4

	out := make([]byte, 12, 12+body.Len()+pad)
	copy(out, "EXTH")
	be.PutUint32(out[4:], uint32(12+body.Len()+pad)) //nolint:gosec // as above
	be.PutUint32(out[8:], uint32(len(records)))      //nolint:gosec // as above
	out = append(out, body.Bytes()...)
	return append(out, make([]byte, pad)...)
}

// encode turns text into the header's encoding. A character Windows-1252
// cannot hold becomes "?".
func (h *header) encode(s string) []byte {
	if h.encoding != encodingCP1252 {
		return []byte(s)
	}
	out := make([]byte, 0, len(s))
	for _, r := range s {
		b, ok := charmap.Windows1252.EncodeRune(r)
		if !ok {
			b = '?'
		}
		out = append(out, b)
	}
	return out
}

// pdbFile is a Palm database being rewritten. Only the records Rewrite
// reads or replaces are held in memory; the rest are copied from the source
// when the file is written, since a book with large images can run to
// hundreds of megabytes.
type pdbFile struct {
	src    io.ReaderAt
	header []byte // the database header, before the record list
	// entries holds each record's attributes and unique ID: the last four
	// bytes of its record list entry.
	entries [][4]byte
	gap     []byte // the bytes between the record list and the first record
	// spans locate each record in the source; an inserted record has none.
	spans []span
	// records holds the records read or replaced; nil means the source's.
	records [][]byte
}

type span struct {
	offset, size int64
}

func readPDBFile(r io.ReaderAt, size int64) (*pdbFile, error) {
	db, err := readPDB(r, size)
	if err != nil {
		return nil, err
	}
	count := db.count()

	head := make([]byte, db.offsets[0])
	if _, err := r.ReadAt(head, 0); err != nil {
		return nil, errors.Wrap(err, "read record list")
	}
	f := &pdbFile{
		src:     r,
		header:  head[:pdbHeaderSize],
		entries: make([][4]byte, count),
		spans:   make([]span, count),
		records: make([][]byte, count),
		gap:     head[pdbHeaderSize+count*pdbRecordEntry:],
	}
	for i := range count {
		copy(f.entries[i][:], head[pdbHeaderSize+i*pdbRecordEntry+4:])
		f.spans[i] = span{offset: db.offsets[i], size: db.offsets[i+1] - db.offsets[i]}
	}
	return f, nil
}

func (f *pdbFile) size(i int) int64 {
	if f.records[i] != nil {
		return int64(len(f.records[i]))
	}
	return f.spans[i].size
}

// load reads a header record into memory so it can be edited.
func (f *pdbFile) load(i int) ([]byte, error) {
	if f.records[i] != nil {
		return f.records[i], nil
	}
	sp := f.spans[i]
	if sp.size > maxHeaderRecord {
		return nil, errors.Errorf("record %d is too large", i)
	}
	data := make([]byte, sp.size)
	if _, err := f.src.ReadAt(data, sp.offset); err != nil {
		return nil, errors.Wrapf(err, "read record %d", i)
	}
	f.records[i] = data
	return data, nil
}

// head returns the first bytes of a record, enough to tell its kind; a
// record that cannot be read has none.
func (f *pdbFile) head(i int) []byte {
	if f.records[i] != nil {
		return f.records[i][:min(len(f.records[i]), 8)]
	}
	sp := f.spans[i]
	data := make([]byte, min(sp.size, 8))
	if _, err := f.src.ReadAt(data, sp.offset); err != nil {
		return nil
	}
	return data
}

// insert puts data in as record p, with a unique ID no other record has.
func (f *pdbFile) insert(p int, data []byte) {
	var maxID uint32
	for _, e := range f.entries {
		maxID = max(maxID, uint32(e[1])<<16|uint32(e[2])<<8|uint32(e[3]))
	}
	id := (maxID + 1) & 0xFFFFFF
	f.entries = slices.Insert(f.entries, p, [4]byte{0, byte(id >> 16), byte(id >> 8), byte(id)})
	f.spans = slices.Insert(f.spans, p, span{})
	f.records = slices.Insert(f.records, p, data)

	seed := binary.BigEndian.Uint32(f.header[pdbUniqueIDSeed:])
	if seed <= id {
		binary.BigEndian.PutUint32(f.header[pdbUniqueIDSeed:], id+1)
	}
}

func (f *pdbFile) writeTo(w io.Writer) error {
	count := len(f.records)
	if count > 0xFFFF {
		return errors.New("MOBI file has too many records")
	}
	be := binary.BigEndian
	header := append([]byte(nil), f.header...)
	be.PutUint16(header[pdbRecordsOffset:], uint16(count))

	list := make([]byte, count*pdbRecordEntry)
	offset := int64(len(header) + len(list) + len(f.gap))
	for i := range f.records {
		if offset > 0xFFFFFFFF {
			return errors.New("MOBI file is too large")
		}
		be.PutUint32(list[i*pdbRecordEntry:], uint32(offset)) //nolint:gosec // checked against 0xFFFFFFFF above
		copy(list[i*pdbRecordEntry+4:], f.entries[i][:])
		offset += f.size(i)
	}

	bw := &errWriter{w: w}
	bw.write(header)
	bw.write(list)
	bw.write(f.gap)
	for i, rec := range f.records {
		if bw.err != nil {
			break
		}
		if rec != nil {
			bw.write(rec)
			continue
		}
		sp := f.spans[i]
		// CopyN fails on a source that ends early, which would leave the
		// record list pointing past the records.
		if _, err := io.CopyN(w, io.NewSectionReader(f.src, sp.offset, sp.size), sp.size); err != nil {
			bw.err = err
		}
	}
	return errors.Wrap(bw.err, "write MOBI file")
}

type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) write(b []byte) {
	if e.err == nil {
		_, e.err = e.w.Write(b)
	}
}
