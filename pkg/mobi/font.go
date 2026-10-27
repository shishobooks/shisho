package mobi

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/pkg/errors"
)

// A FONT resource record holds an embedded font: "FONT", the font's size,
// flags, where the font's data starts, and the length and start of an XOR
// key. Flag bit 0 says the data is zlib compressed, and bit 1 that its
// first 1040 bytes, as stored, are XORed with the key.
const (
	fontFlagZlib       = 0b01
	fontFlagObfuscated = 0b10
	fontObfuscatedLen  = 1040
	fontHeaderLen      = 24

	// maxFontsSize caps the book's decoded fonts together; embedded fonts
	// are rarely more than a few megabytes each.
	maxFontsSize = 64 << 20
)

// errFontsTooLarge is returned by readFont for a font larger than its limit.
var errFontsTooLarge = errors.New("MOBI fonts are too large")

// readFont returns the font in FONT record i, which is resource number n.
// It refuses a font larger than limit with errFontsTooLarge.
func readFont(db *pdb, i, n int, limit int64) (Document, error) {
	rec, err := db.record(i, maxImageRecord)
	if err != nil {
		return Document{}, err
	}
	if len(rec) < fontHeaderLen || string(rec[:4]) != "FONT" {
		return Document{}, errors.New("MOBI FONT record is invalid")
	}
	be := binary.BigEndian
	size, flags, start := int64(be.Uint32(rec[4:])), be.Uint32(rec[8:]), int64(be.Uint32(rec[12:]))
	keyLen, keyStart := int64(be.Uint32(rec[16:])), int64(be.Uint32(rec[20:]))
	if start > int64(len(rec)) {
		return Document{}, errors.New("MOBI FONT record is truncated")
	}
	if size > limit || flags&fontFlagZlib == 0 && int64(len(rec))-start > limit {
		return Document{}, errors.WithStack(errFontsTooLarge)
	}
	data := bytes.Clone(rec[start:])
	if flags&fontFlagObfuscated != 0 {
		if keyLen == 0 || keyStart+keyLen > int64(len(rec)) {
			return Document{}, errors.New("MOBI FONT record's key is out of range")
		}
		key := rec[keyStart : keyStart+keyLen]
		for n := range min(len(data), fontObfuscatedLen) {
			data[n] ^= key[n%len(key)]
		}
	}
	if flags&fontFlagZlib != 0 {
		zr, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return Document{}, errors.Wrap(err, "MOBI FONT record")
		}
		data, err = io.ReadAll(io.LimitReader(zr, size+1))
		if err != nil {
			return Document{}, errors.Wrap(err, "MOBI FONT record")
		}
		if int64(len(data)) != size {
			return Document{}, errors.New("MOBI FONT record is not the size it says")
		}
	}
	mediaType, ext := fontType(data)
	if mediaType == "" {
		return Document{}, errors.New("MOBI FONT record holds no font Shisho knows")
	}
	return Document{Name: fontName(n, ext), MediaType: mediaType, Content: data}, nil
}

// fontType returns the media type and file extension of a font, or ""
// for data that is no font EPUB readers take.
func fontType(data []byte) (string, string) {
	switch {
	case bytes.HasPrefix(data, []byte("\x00\x01\x00\x00")), bytes.HasPrefix(data, []byte("true")):
		return "font/ttf", ".ttf"
	case bytes.HasPrefix(data, []byte("OTTO")):
		return "font/otf", ".otf"
	case bytes.HasPrefix(data, []byte("wOFF")):
		return "font/woff", ".woff"
	case bytes.HasPrefix(data, []byte("wOF2")):
		return "font/woff2", ".woff2"
	}
	return "", ""
}

// fontName is the file name of the font that is resource number n.
func fontName(n int, ext string) string {
	return fmt.Sprintf("font%04d%s", n, ext)
}

// kindleFont reports whether data is a font a Kindle shows: TrueType or
// OpenType. A Kindle takes no WOFF.
func kindleFont(data []byte) bool {
	mediaType, _ := fontType(data)
	return mediaType == "font/ttf" || mediaType == "font/otf"
}

// fontRecord returns a FONT record holding font, zlib compressed and not
// obfuscated, so it has no key. Compressing into memory can't fail.
func fontRecord(font []byte) []byte {
	rec := make([]byte, fontHeaderLen, fontHeaderLen+len(font))
	copy(rec, "FONT")
	be := binary.BigEndian
	be.PutUint32(rec[4:], u32(len(font)))
	be.PutUint32(rec[8:], fontFlagZlib)
	be.PutUint32(rec[12:], fontHeaderLen)
	be.PutUint32(rec[20:], fontHeaderLen) // the key, of length 0, would start here
	buf := bytes.NewBuffer(rec)
	zw := zlib.NewWriter(buf)
	_, _ = zw.Write(font)
	_ = zw.Close()
	return buf.Bytes()
}
