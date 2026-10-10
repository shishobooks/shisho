package mobi

import (
	"encoding/binary"
	"strings"
	"unicode/utf8"

	"github.com/pkg/errors"
	"golang.org/x/text/encoding/charmap"
)

// Offsets into a header record (record 0 of a book). The record opens with
// the 16-byte PalmDOC header, and the MOBI header follows it.
const (
	offCompression    = 0
	offEncryption     = 12
	offMOBIMagic      = 16
	offHeaderLength   = 20
	offTextEncoding   = 28
	offVersion        = 36
	offFirstNonBook   = 80
	offFullNameOffset = 84
	offFullNameLength = 88
	offFirstImage     = 108
	offHuffRecord     = 112
	offEXTHFlags      = 128
	minHeaderRecord   = offEXTHFlags + 4

	// Record indexes past the EXTH flags. A MOBI6 header has the last
	// content record (a uint16) where a KF8 header has the FDST record, and
	// only a KF8 header has the fragment, skeleton, DATP, and guide indexes.
	offFDST        = 192
	offLastContent = 194
	offFCIS        = 200
	offFLIS        = 208
	offSRCS        = 224
	offNCX         = 244
	offFragment    = 248
	offSkeleton    = 252
	offDATP        = 256
	offGuide       = 260

	exthFlagPresent = 0x40
	nullIndex       = 0xFFFFFFFF
)

// Compression types in the PalmDOC header. Metadata and covers sit outside
// the text records, so reading them never decompresses text; any other value
// means a corrupt or unknown file.
const (
	compressionNone     = 1
	compressionPalmDOC  = 2
	compressionHuffCDIC = 17480
)

const (
	encodingCP1252 = 1252
	encodingUTF8   = 65001
)

// EXTH record types.
const (
	exthAuthor         = 100
	exthPublisher      = 101
	exthImprint        = 102
	exthDescription    = 103
	exthISBN           = 104
	exthSubject        = 105
	exthPublishingDate = 106
	exthSource         = 112
	exthASIN           = 113
	exthKF8Boundary    = 121
	exthResourceCount  = 125
	exthCoverOffset    = 201
	exthThumbOffset    = 202
	exthFakeCover      = 203
	exthKF8CoverURI    = 129
	exthUpdatedTitle   = 503
	exthLanguage       = 524
)

// header is the part of a book's header record the parser uses.
type header struct {
	compression uint16
	encrypted   bool
	version     uint32
	encoding    uint32
	fullName    string
	firstImage  uint32
	exth        map[uint32][][]byte
}

func parseHeader(rec []byte) (*header, error) {
	if len(rec) < minHeaderRecord || string(rec[offMOBIMagic:offMOBIMagic+4]) != "MOBI" {
		return nil, errors.New("record has no MOBI header")
	}
	be := binary.BigEndian
	h := &header{
		compression: be.Uint16(rec[offCompression:]),
		encrypted:   be.Uint16(rec[offEncryption:]) != 0,
		version:     be.Uint32(rec[offVersion:]),
		encoding:    be.Uint32(rec[offTextEncoding:]),
		firstImage:  be.Uint32(rec[offFirstImage:]),
	}

	nameOffset := int64(be.Uint32(rec[offFullNameOffset:]))
	nameLength := int64(be.Uint32(rec[offFullNameLength:]))
	if nameOffset+nameLength <= int64(len(rec)) {
		h.fullName = h.decode(rec[nameOffset : nameOffset+nameLength])
	}

	headerLength := int64(be.Uint32(rec[offHeaderLength:]))
	if be.Uint32(rec[offEXTHFlags:])&exthFlagPresent != 0 {
		exth, err := parseEXTH(rec, offMOBIMagic+headerLength)
		if err != nil {
			return nil, err
		}
		h.exth = exth
	}
	return h, nil
}

// exthRecord is one EXTH record, in file order.
type exthRecord struct {
	typ  uint32
	data []byte
}

// parseEXTH reads the EXTH block at start into a map of each record type's
// values.
func parseEXTH(rec []byte, start int64) (map[uint32][][]byte, error) {
	records, err := parseEXTHRecords(rec, start)
	if err != nil {
		return nil, err
	}
	exth := make(map[uint32][][]byte)
	for _, r := range records {
		exth[r.typ] = append(exth[r.typ], r.data)
	}
	return exth, nil
}

// parseEXTHRecords reads the EXTH block at start: "EXTH", the block length,
// the record count, and then each record's type, length (including its
// 8-byte header), and data.
func parseEXTHRecords(rec []byte, start int64) ([]exthRecord, error) {
	be := binary.BigEndian
	if start < 0 || start+12 > int64(len(rec)) || string(rec[start:start+4]) != "EXTH" {
		return nil, errors.New("EXTH block is missing")
	}
	count := be.Uint32(rec[start+8:])
	var records []exthRecord
	pos := start + 12
	for i := uint32(0); i < count; i++ {
		if pos+8 > int64(len(rec)) {
			return nil, errors.New("EXTH block is truncated")
		}
		typ := be.Uint32(rec[pos:])
		length := int64(be.Uint32(rec[pos+4:]))
		if length < 8 || pos+length > int64(len(rec)) {
			return nil, errors.Errorf("EXTH record %d has an invalid length", typ)
		}
		records = append(records, exthRecord{typ: typ, data: rec[pos+8 : pos+length]})
		pos += length
	}
	return records, nil
}

// decode turns header or EXTH text into a trimmed UTF-8 string.
func (h *header) decode(b []byte) string {
	var s string
	if h.encoding == encodingCP1252 {
		decoded, err := charmap.Windows1252.NewDecoder().Bytes(b)
		if err != nil {
			return ""
		}
		s = string(decoded)
	} else {
		s = strings.ToValidUTF8(string(b), string(utf8.RuneError))
	}
	return strings.TrimSpace(strings.Trim(s, "\x00"))
}

// texts returns every non-empty text value of an EXTH record type.
func (h *header) texts(typ uint32) []string {
	var values []string
	for _, b := range h.exth[typ] {
		if s := h.decode(b); s != "" {
			values = append(values, s)
		}
	}
	return values
}

// text returns the first non-empty text value of an EXTH record type.
func (h *header) text(typ uint32) string {
	if values := h.texts(typ); len(values) > 0 {
		return values[0]
	}
	return ""
}

// number returns a numeric EXTH record, and false when it is absent.
func (h *header) number(typ uint32) (uint32, bool) {
	values := h.exth[typ]
	if len(values) == 0 || len(values[0]) != 4 {
		return 0, false
	}
	return binary.BigEndian.Uint32(values[0]), true
}

func (h *header) isKF8() bool {
	return h.version >= 8
}

func (h *header) checkCompression() error {
	switch h.compression {
	case compressionNone, compressionPalmDOC, compressionHuffCDIC:
		return nil
	}
	return errors.Errorf("unsupported MOBI compression type %d", h.compression)
}
