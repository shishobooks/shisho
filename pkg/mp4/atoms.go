package mp4

import (
	"encoding/binary"

	gomp4 "github.com/abema/go-mp4"
)

// MP4 data types used in iTunes metadata atoms.
const (
	DataTypeReserved = 0  // Reserved, should not be used
	DataTypeUTF8     = 1  // UTF-8 text (most common)
	DataTypeUTF16BE  = 2  // UTF-16 big-endian text
	DataTypeJPEG     = 13 // JPEG image data
	DataTypePNG      = 14 // PNG image data
	DataTypeGenre    = 18 // Genre (special text format - the problematic type)
	DataTypeInteger  = 21 // Signed big-endian integer (1, 2, 3, 4, or 8 bytes)
	DataTypeBMP      = 27 // BMP image data
)

// iTunes atom type names (4-byte codes).
// Note: © symbol is encoded as 0xA9 in MacRoman.
var (
	// Standard iTunes metadata atoms.
	AtomTitle    = [4]byte{0xA9, 'n', 'a', 'm'} // ©nam - Title
	AtomArtist   = [4]byte{0xA9, 'A', 'R', 'T'} // ©ART - Artist (author)
	AtomAlbum    = [4]byte{0xA9, 'a', 'l', 'b'} // ©alb - Album
	AtomComposer = [4]byte{0xA9, 'c', 'm', 'p'} // ©cmp - Composer (narrator for audiobooks)
	AtomGenre    = [4]byte{0xA9, 'g', 'e', 'n'} // ©gen - Genre (custom text)
	AtomWriter   = [4]byte{0xA9, 'w', 'r', 't'} // ©wrt - Writer
	AtomComment  = [4]byte{0xA9, 'c', 'm', 't'} // ©cmt - Comment
	AtomYear     = [4]byte{0xA9, 'd', 'a', 'y'} // ©day - Year/Date
	AtomGrouping = [4]byte{0xA9, 'g', 'r', 'p'} // ©grp - Grouping

	// Non-copyright atoms (standard ASCII).
	AtomCover       = [4]byte{'c', 'o', 'v', 'r'} // covr - Cover artwork
	AtomGenreID     = [4]byte{'g', 'n', 'r', 'e'} // gnre - Genre ID (ID3v1 index)
	AtomMediaType   = [4]byte{'s', 't', 'i', 'k'} // stik - Media type (2 = audiobook)
	AtomDescription = [4]byte{'d', 'e', 's', 'c'} // desc - Description
	AtomFreeform    = [4]byte{'-', '-', '-', '-'} // ---- - Freeform/custom atom
)

// Box types for navigation.
var (
	BoxTypeMoov = gomp4.BoxTypeMoov()        // moov - Movie box
	BoxTypeUdta = gomp4.BoxTypeUdta()        // udta - User data box
	BoxTypeMeta = gomp4.BoxTypeMeta()        // meta - Metadata box
	BoxTypeIlst = gomp4.BoxTypeIlst()        // ilst - Item list box
	BoxTypeData = gomp4.BoxTypeData()        // data - Data box
	BoxTypeMvhd = gomp4.BoxTypeMvhd()        // mvhd - Movie header
	BoxTypeTrak = gomp4.BoxTypeTrak()        // trak - Track box
	BoxTypeHdlr = gomp4.BoxTypeHdlr()        // hdlr - Handler box
	BoxTypeMdia = gomp4.BoxTypeMdia()        // mdia - Media box
	BoxTypeChpl = gomp4.StrToBoxType("chpl") // chpl - Chapter list (Nero)
	BoxTypeTref = gomp4.StrToBoxType("tref") // tref - Track reference
	BoxTypeChap = gomp4.StrToBoxType("chap") // chap - Chapter reference
)

// parseDataValue extracts the value from a data atom based on its type.
// The data format is: [1 byte version][3 bytes type][4 bytes locale][...data...].
func parseDataValue(data []byte) (dataType int, value []byte, ok bool) {
	if len(data) < 8 {
		return 0, nil, false
	}

	// First byte is version (usually 0)
	// Next 3 bytes are data type (big-endian)
	dataType = int(data[1])<<16 | int(data[2])<<8 | int(data[3])

	// Next 4 bytes are locale (usually 0)
	// Remaining bytes are the actual value
	value = data[8:]

	return dataType, value, true
}

// parseTextData extracts text from a data atom, handling various data types.
func parseTextData(data []byte) string {
	dataType, value, ok := parseDataValue(data)
	if !ok || len(value) == 0 {
		return ""
	}

	switch dataType {
	case DataTypeUTF8, DataTypeGenre:
		// Both UTF-8 and Genre type 18 contain UTF-8 text
		return string(value)
	case DataTypeUTF16BE:
		// UTF-16 big-endian - convert to string
		if len(value) >= 2 {
			return decodeUTF16BE(value)
		}
	}

	// For unknown types, try to interpret as UTF-8
	return string(value)
}

// parseIntegerData extracts an integer from a data atom.
func parseIntegerData(data []byte) (int64, bool) {
	dataType, value, ok := parseDataValue(data)
	if !ok || dataType != DataTypeInteger {
		return 0, false
	}

	switch len(value) {
	case 1:
		return int64(value[0]), true
	case 2:
		return int64(binary.BigEndian.Uint16(value)), true
	case 4:
		return int64(binary.BigEndian.Uint32(value)), true
	case 8:
		v := binary.BigEndian.Uint64(value)
		// Check for overflow before converting to int64.
		if v > 1<<63-1 {
			return 0, false
		}
		return int64(v), true
	default:
		return 0, false
	}
}

// parseImageData extracts image data and determines the MIME type.
func parseImageData(data []byte) (imageData []byte, mimeType string, ok bool) {
	dataType, value, ok := parseDataValue(data)
	if !ok || len(value) == 0 {
		return nil, "", false
	}

	switch dataType {
	case DataTypeJPEG:
		return value, "image/jpeg", true
	case DataTypePNG:
		return value, "image/png", true
	case DataTypeBMP:
		return value, "image/bmp", true
	default:
		// Try to detect from magic bytes
		return detectImageType(value)
	}
}

// detectImageType attempts to determine image type from magic bytes.
func detectImageType(data []byte) (imageData []byte, mimeType string, ok bool) {
	if len(data) < 4 {
		return nil, "", false
	}

	// JPEG magic bytes: FF D8 FF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return data, "image/jpeg", true
	}

	// PNG magic bytes: 89 50 4E 47 0D 0A 1A 0A
	if len(data) >= 8 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
		return data, "image/png", true
	}

	// BMP magic bytes: 42 4D
	if data[0] == 'B' && data[1] == 'M' {
		return data, "image/bmp", true
	}

	return nil, "", false
}

// decodeUTF16BE decodes UTF-16 big-endian bytes to a string.
func decodeUTF16BE(data []byte) string {
	if len(data) < 2 {
		return ""
	}

	// Check for BOM
	start := 0
	if data[0] == 0xFE && data[1] == 0xFF {
		start = 2 // Skip BOM
	}

	runes := make([]rune, 0, (len(data)-start)/2)
	for i := start; i+1 < len(data); i += 2 {
		r := rune(binary.BigEndian.Uint16(data[i : i+2]))
		if r == 0 {
			break // Null terminator
		}
		runes = append(runes, r)
	}

	return string(runes)
}

// atomTypeEquals checks if an atom type matches a reference type.
func atomTypeEquals(boxType gomp4.BoxType, atomType [4]byte) bool {
	return boxType[0] == atomType[0] &&
		boxType[1] == atomType[1] &&
		boxType[2] == atomType[2] &&
		boxType[3] == atomType[3]
}
