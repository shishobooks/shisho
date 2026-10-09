package testgen

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

// MOBI layout constants, matching what Calibre 9 writes.
const (
	mobiHeaderLengthMOBI6 = 232
	mobiHeaderLengthKF8   = 264
	mobiTextRecordSize    = 4096
	mobiNullIndex         = 0xFFFFFFFF
	mobiEncodingUTF8      = 65001
	mobiEncodingCP1252    = 1252
	mobiCompressionNone   = 1
	mobiCompressionPalm   = 2
	mobiCompressionHuff   = 17480
	mobiEncryptionMobi    = 2
	mobiEXTHFlags         = 0x50
	mobiEXTHFlagsCombo    = 0x850
)

// GenerateMOBI writes a MOBI, AZW3, or combo file shaped like Calibre's
// output and returns its path.
func GenerateMOBI(t *testing.T, dir, filename string, opts MOBIOptions) string {
	t.Helper()
	return WriteFile(t, dir, filename, BuildMOBI(t, opts))
}

// BuildMOBI returns the bytes of a MOBI, AZW3, or combo file shaped like
// Calibre's output: a Palm database whose record 0 holds the PalmDOC header,
// the MOBI header, the EXTH block, and the full name, followed by text
// records, image records, and the FLIS, FCIS, and EOF records. A combo file
// appends a BOUNDARY record and a KF8 book that shares the MOBI6 book's
// images, so its own first-image index points at no image.
func BuildMOBI(t *testing.T, opts MOBIOptions) []byte {
	t.Helper()

	var images [][]byte
	if opts.HasCover {
		mime := opts.CoverMimeType
		if mime == "" {
			mime = "image/jpeg"
		}
		cover := generateImage(t, mime)
		images = [][]byte{cover, cover} // cover, then thumbnail
	}

	var records [][]byte
	switch opts.Kind {
	case MOBIKindMOBI6:
		records = buildMOBIBook(t, opts, 6, 0, images, -1)
	case MOBIKindKF8:
		records = buildMOBIBook(t, opts, 8, 0, images, -1)
	case MOBIKindCombo:
		mobi6Opts := opts
		if opts.MOBI6Title != "" {
			mobi6Opts.Title = opts.MOBI6Title
			mobi6Opts.FullName = opts.MOBI6Title
		}
		// The MOBI6 book's record count does not depend on the boundary
		// index it stores, so build it once to learn where the KF8 header
		// lands, then again with that index.
		probe := buildMOBIBook(t, mobi6Opts, 6, 0, images, 0)
		kf8Index := len(probe) + 1
		records = buildMOBIBook(t, mobi6Opts, 6, 0, images, kf8Index)
		records = append(records, []byte("BOUNDARY"))
		records = append(records, buildMOBIBook(t, opts, 8, kf8Index, nil, -1)...)
	default:
		t.Fatalf("unknown MOBI kind %d", opts.Kind)
	}
	records = append(records, []byte{0xE9, 0x8E, 0x0D, 0x0A}) // EOF

	return buildPalmDatabase(pdbName(opts), records)
}

// buildMOBIBook returns the records of one book: header record, text
// records, the HUFF and CDIC records when compressed that way, the images,
// and then FDST (KF8 only), FLIS, and FCIS. base is the absolute index of the
// header record. kf8Index, when not negative, is written as the KF8 boundary
// record (121) of a combo file's MOBI6 header.
func buildMOBIBook(t *testing.T, opts MOBIOptions, version uint32, base int, images [][]byte, kf8Index int) [][]byte {
	t.Helper()

	text := []byte("<html><head></head><body><h1>Chapter One</h1><p>Hello world.</p></body></html>")
	textRecords := [][]byte{compressMOBIText(text, opts.Compression)}

	var after [][]byte
	var huffIndex, huffCount uint32 = 0, 0
	if opts.Compression == MOBICompressionHuffCDIC {
		huffIndex = u32(1 + len(textRecords))
		huffCount = 2
		after = append(after, huffRecord(), cdicRecord())
	}

	firstNonBook := 1 + len(textRecords) + len(after)
	firstImage := uint32(mobiNullIndex)
	if len(images) > 0 {
		firstImage = u32(base + firstNonBook)
	} else if version == 8 && base > 0 {
		// The images of a combo file belong to the MOBI6 book. Calibre's
		// KF8 header still names a first image, at an index that holds no
		// image whether read as absolute or as relative to the KF8 header.
		firstImage = uint32(base)
	}
	after = append(after, images...)
	if version == 8 {
		after = append(after, fdstRecord(len(text)))
	}
	after = append(after, flisRecord(), fcisRecord(len(text)))

	header := buildMOBIHeaderRecord(t, opts, mobiHeaderSpec{
		version:      version,
		textLength:   u32(len(text)),
		textRecords:  u16(len(textRecords)),
		firstNonBook: u32(firstNonBook),
		firstImage:   firstImage,
		hasImages:    len(images) > 0 || (version == 8 && base > 0 && opts.HasCover),
		huffIndex:    huffIndex,
		huffCount:    huffCount,
		kf8Index:     kf8Index,
	})

	records := [][]byte{header}
	records = append(records, textRecords...)
	return append(records, after...)
}

type mobiHeaderSpec struct {
	version      uint32
	textLength   uint32
	textRecords  uint16
	firstNonBook uint32
	firstImage   uint32
	hasImages    bool
	huffIndex    uint32
	huffCount    uint32
	kf8Index     int
}

func buildMOBIHeaderRecord(t *testing.T, opts MOBIOptions, spec mobiHeaderSpec) []byte {
	t.Helper()

	encode := func(s string) []byte {
		if !opts.CP1252 {
			return []byte(s)
		}
		b, err := charmap.Windows1252.NewEncoder().Bytes([]byte(s))
		if err != nil {
			t.Fatalf("failed to encode %q as Windows-1252: %v", s, err)
		}
		return b
	}

	headerLength := uint32(mobiHeaderLengthMOBI6)
	if spec.version == 8 || spec.kf8Index >= 0 {
		headerLength = mobiHeaderLengthKF8
	}

	exth := buildEXTH(opts, spec, encode)
	fullName := opts.FullName
	if fullName == "" {
		fullName = opts.Title
	}
	fullNameBytes := encode(fullName)

	var compression uint16
	switch opts.Compression {
	case MOBICompressionPalmDOC:
		compression = mobiCompressionPalm
	case MOBICompressionNone:
		compression = mobiCompressionNone
	case MOBICompressionHuffCDIC:
		compression = mobiCompressionHuff
	}
	encoding := uint32(mobiEncodingUTF8)
	if opts.CP1252 {
		encoding = mobiEncodingCP1252
	}

	h := make([]byte, 16+headerLength)
	be := binary.BigEndian
	// PalmDOC header.
	be.PutUint16(h[0:], compression)
	be.PutUint32(h[4:], spec.textLength)
	be.PutUint16(h[8:], spec.textRecords)
	be.PutUint16(h[10:], mobiTextRecordSize)
	if opts.Encrypted {
		be.PutUint16(h[12:], mobiEncryptionMobi)
	}
	// MOBI header.
	copy(h[16:], "MOBI")
	be.PutUint32(h[20:], headerLength)
	be.PutUint32(h[24:], 2) // book
	be.PutUint32(h[28:], encoding)
	be.PutUint32(h[32:], 0x2A2B2C2D) // unique ID
	be.PutUint32(h[36:], spec.version)
	for off := 40; off < 80; off += 4 {
		be.PutUint32(h[off:], mobiNullIndex) // orthographic and inflection indexes
	}
	be.PutUint32(h[80:], spec.firstNonBook)
	be.PutUint32(h[84:], u32(len(h)+len(exth)))
	be.PutUint32(h[88:], u32(len(fullNameBytes)))
	be.PutUint32(h[92:], 9) // locale: English
	be.PutUint32(h[104:], spec.version)
	be.PutUint32(h[108:], spec.firstImage)
	be.PutUint32(h[112:], spec.huffIndex)
	be.PutUint32(h[116:], spec.huffCount)
	exthFlags := uint32(mobiEXTHFlags)
	if spec.kf8Index >= 0 {
		exthFlags = mobiEXTHFlagsCombo
	}
	be.PutUint32(h[128:], exthFlags)
	be.PutUint32(h[164:], mobiNullIndex)
	if opts.Encrypted {
		// Retail files point at a DRM voucher block in record 0.
		be.PutUint32(h[168:], u32(len(h)+len(exth)))
		be.PutUint32(h[172:], 1)
		be.PutUint32(h[176:], 48)
		be.PutUint32(h[180:], 1)
	} else {
		be.PutUint32(h[168:], mobiNullIndex)
	}
	be.PutUint16(h[192:], 1)
	be.PutUint16(h[194:], spec.textRecords)
	be.PutUint32(h[196:], 1)
	be.PutUint32(h[224:], mobiNullIndex)
	be.PutUint32(h[232:], mobiNullIndex)
	be.PutUint32(h[236:], mobiNullIndex)
	be.PutUint32(h[244:], mobiNullIndex) // no NCX index
	if headerLength == mobiHeaderLengthKF8 {
		for off := 248; off < 280; off += 4 {
			be.PutUint32(h[off:], mobiNullIndex)
		}
	}

	var rec bytes.Buffer
	rec.Write(h)
	rec.Write(exth)
	rec.Write(fullNameBytes)
	// Calibre pads record 0 with zeros so Kindles can rewrite it in place.
	rec.Write(make([]byte, 2+(4-len(fullNameBytes)%4)%4+1024))
	return rec.Bytes()
}

// buildEXTH writes the EXTH block in the record order Calibre uses.
func buildEXTH(opts MOBIOptions, spec mobiHeaderSpec, encode func(string) []byte) []byte {
	type record struct {
		typ  uint32
		data []byte
	}
	var recs []record
	addString := func(typ uint32, value string) {
		if value != "" {
			recs = append(recs, record{typ, encode(value)})
		}
	}
	addUint32 := func(typ uint32, value uint32) {
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, value)
		recs = append(recs, record{typ, b})
	}

	addString(503, opts.Title)
	addString(524, opts.Language)
	for _, author := range opts.Authors {
		addString(100, author)
	}
	addString(104, opts.ISBN)
	addString(103, opts.Description)
	addString(101, opts.Publisher)
	addString(102, opts.Imprint)
	for _, subject := range opts.Subjects {
		addString(105, subject)
	}
	addString(113, opts.ASIN)
	addString(112, opts.Source)
	addString(501, "EBOK")
	addString(106, opts.PublishingDate)
	addUint32(204, 202) // creator software: Calibre
	if spec.hasImages {
		addUint32(201, 0) // cover offset
		addUint32(203, 0) // no fake cover
		addUint32(202, 1) // thumbnail offset
		addString(129, "kindle:embed:0001")
	}
	if spec.kf8Index >= 0 {
		addUint32(121, u32(spec.kf8Index))
	}
	if opts.Encrypted {
		addString(208, "watermark")
	}

	var body bytes.Buffer
	for _, r := range recs {
		_ = binary.Write(&body, binary.BigEndian, r.typ)
		_ = binary.Write(&body, binary.BigEndian, u32(8+len(r.data)))
		body.Write(r.data)
	}
	pad := (4 - body.Len()%4) % 4

	var out bytes.Buffer
	out.WriteString("EXTH")
	_ = binary.Write(&out, binary.BigEndian, u32(12+body.Len()+pad))
	_ = binary.Write(&out, binary.BigEndian, u32(len(recs)))
	out.Write(body.Bytes())
	out.Write(make([]byte, pad))
	return out.Bytes()
}

// compressMOBIText returns one text record. PalmDOC output uses only the
// literal forms of the format, which any decoder reads; HUFF/CDIC output is
// opaque bytes, since only the header's compression field matters to tests.
func compressMOBIText(text []byte, compression MOBICompression) []byte {
	switch compression {
	case MOBICompressionNone:
		return append([]byte(nil), text...)
	case MOBICompressionHuffCDIC:
		return bytes.Repeat([]byte{0xA5}, len(text)/2)
	case MOBICompressionPalmDOC:
	}
	var out []byte
	for _, b := range text {
		if b == 0 || (b >= 0x09 && b <= 0x7F) {
			out = append(out, b)
		} else {
			out = append(out, 1, b)
		}
	}
	return out
}

func huffRecord() []byte {
	r := make([]byte, 24+256*4+64*4)
	copy(r, "HUFF")
	binary.BigEndian.PutUint32(r[4:], 24)
	binary.BigEndian.PutUint32(r[8:], 24)
	binary.BigEndian.PutUint32(r[12:], 24+256*4)
	return r
}

func cdicRecord() []byte {
	r := make([]byte, 16)
	copy(r, "CDIC")
	binary.BigEndian.PutUint32(r[4:], 16)
	return r
}

func fdstRecord(textLength int) []byte {
	r := make([]byte, 20)
	copy(r, "FDST")
	binary.BigEndian.PutUint32(r[4:], 12)
	binary.BigEndian.PutUint32(r[8:], 1)
	binary.BigEndian.PutUint32(r[16:], u32(textLength))
	return r
}

func flisRecord() []byte {
	return []byte{
		'F', 'L', 'I', 'S', 0, 0, 0, 8, 0, 65, 0, 0, 0, 0, 0, 0,
		0xFF, 0xFF, 0xFF, 0xFF, 0, 1, 0, 3, 0, 0, 0, 3, 0, 0, 0, 1, 0xFF, 0xFF, 0xFF, 0xFF,
	}
}

func fcisRecord(textLength int) []byte {
	r := []byte{
		'F', 'C', 'I', 'S', 0, 0, 0, 20, 0, 0, 0, 16, 0, 0, 0, 1, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32, 0, 0, 0, 8, 0, 1, 0, 1, 0, 0, 0, 0,
	}
	binary.BigEndian.PutUint32(r[20:], u32(textLength))
	return r
}

// buildPalmDatabase wraps records in a Palm database with type BOOKMOBI.
func buildPalmDatabase(name string, records [][]byte) []byte {
	const headerSize = 78
	listSize := 8*len(records) + 2 // record list plus two bytes of gap

	var out bytes.Buffer
	nameField := make([]byte, 32)
	copy(nameField[:31], name)
	out.Write(nameField)
	out.Write(make([]byte, 28)) // attributes, version, dates, modification number, app info, sort info
	out.WriteString("BOOKMOBI")
	_ = binary.Write(&out, binary.BigEndian, u32(2*len(records)-1)) // unique ID seed
	_ = binary.Write(&out, binary.BigEndian, uint32(0))             // next record list
	_ = binary.Write(&out, binary.BigEndian, u16(len(records)))

	offset := headerSize + listSize
	for i, rec := range records {
		_ = binary.Write(&out, binary.BigEndian, u32(offset))
		_ = binary.Write(&out, binary.BigEndian, u32(2*i)) // attributes 0, unique ID 2*i
		offset += len(rec)
	}
	out.Write([]byte{0, 0})
	for _, rec := range records {
		out.Write(rec)
	}
	return out.Bytes()
}

// pdbName is the Palm database name Calibre writes: the title with spaces
// replaced by underscores, cut to 31 bytes.
func pdbName(opts MOBIOptions) string {
	name := opts.FullName
	if name == "" {
		name = opts.Title
	}
	if name == "" {
		name = "Unknown"
	}
	name = strings.ReplaceAll(name, " ", "_")
	if len(name) > 31 {
		name = name[:31]
	}
	return name
}

// u32 and u16 narrow sizes, counts, and indexes, which stay small in a
// generated test file.
func u32(n int) uint32 { return uint32(n) } //nolint:gosec // test files are far below 4 GiB

func u16(n int) uint16 { return uint16(n) } //nolint:gosec // test files have few records
