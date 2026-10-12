// Package incremental reads just enough of a PDF to append an incremental
// update to it, and writes that update. It parses the end of the file (the
// cross-reference sections and trailers), then only the objects a caller
// asks for, so preparing an update never reads the page contents. The
// original bytes stay untouched: the update is written after them, which
// keeps a digital signature over those bytes valid.
package incremental

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"unicode/utf16"

	"github.com/pkg/errors"
)

// Object is a PDF object: Null, Bool, Integer, Real, Name, String, Array,
// Dict, Ref, or *Stream.
type Object interface{}

// Null is the PDF null object.
type Null struct{}

// Bool is a PDF boolean.
type Bool bool

// Integer is a PDF integer.
type Integer int64

// Real is a PDF real number, kept as written so it is copied exactly.
type Real string

// Name is a PDF name, without the leading slash.
type Name string

// String is a PDF string's bytes, after escapes and hex are decoded.
type String []byte

// Array is a PDF array.
type Array []Object

// Dict is a PDF dictionary. A key whose value is null is absent.
type Dict map[Name]Object

// Ref is an indirect reference.
type Ref struct {
	Num int
	Gen int
}

// Stream is a stream object. Data is the stream as stored, still encoded
// by any filters its dictionary names. The writer sets /Length itself.
type Stream struct {
	Dict Dict
	Data []byte
}

// Clone returns a shallow copy of d, so a new version of an object can be
// built without changing the one that was read.
func (d Dict) Clone() Dict {
	c := make(Dict, len(d)+4)
	for k, v := range d {
		c[k] = v
	}
	return c
}

// TextString encodes s as a PDF text string: the bytes themselves when s
// is printable ASCII, which PDFDocEncoding shares, and UTF-16BE with a byte
// order mark otherwise.
func TextString(s string) String {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			ascii = false
			break
		}
	}
	if ascii {
		return String(s)
	}
	units := utf16.Encode([]rune(s))
	b := make([]byte, 2, 2+2*len(units))
	b[0], b[1] = 0xfe, 0xff
	for _, u := range units {
		b = append(b, byte(u>>8), byte(u))
	}
	return String(b)
}

// Text decodes a PDF text string: UTF-16BE when it starts with a byte
// order mark, otherwise PDFDocEncoding, read as Latin-1 (the two differ
// only in rarely used punctuation).
func (s String) Text() string {
	if len(s) >= 2 && s[0] == 0xfe && s[1] == 0xff {
		units := make([]uint16, 0, (len(s)-2)/2)
		for i := 2; i+1 < len(s); i += 2 {
			units = append(units, uint16(s[i])<<8|uint16(s[i+1]))
		}
		return string(utf16.Decode(units))
	}
	if len(s) >= 3 && s[0] == 0xef && s[1] == 0xbb && s[2] == 0xbf {
		return string(s[3:])
	}
	runes := make([]rune, len(s))
	for i, c := range s {
		runes[i] = rune(c)
	}
	return string(runes)
}

// writeObject serializes o. A *Stream is only valid as the value of an
// indirect object, and gets its /Length from its data.
func writeObject(b *bytes.Buffer, o Object) error {
	switch v := o.(type) {
	case nil, Null:
		b.WriteString("null")
	case Bool:
		if v {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case Integer:
		b.WriteString(strconv.FormatInt(int64(v), 10))
	case Real:
		b.WriteString(string(v))
	case Name:
		writeName(b, v)
	case String:
		writeString(b, v)
	case Ref:
		fmt.Fprintf(b, "%d %d R", v.Num, v.Gen)
	case Array:
		b.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				b.WriteByte(' ')
			}
			if err := writeObject(b, item); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case Dict:
		return writeDict(b, v)
	case *Stream:
		d := v.Dict.Clone()
		d["Length"] = Integer(len(v.Data))
		if err := writeDict(b, d); err != nil {
			return err
		}
		b.WriteString("\nstream\n")
		b.Write(v.Data)
		b.WriteString("\nendstream")
	default:
		return errors.Errorf("cannot write %T as a PDF object", o)
	}
	return nil
}

func writeDict(b *bytes.Buffer, d Dict) error {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	b.WriteString("<<")
	for _, k := range keys {
		v := d[Name(k)]
		if _, ok := v.(*Stream); ok {
			return errors.Errorf("dictionary entry /%s is a stream, which must be indirect", k)
		}
		writeName(b, Name(k))
		b.WriteByte(' ')
		if err := writeObject(b, v); err != nil {
			return err
		}
	}
	b.WriteString(">>")
	return nil
}

func writeName(b *bytes.Buffer, n Name) {
	b.WriteByte('/')
	for i := 0; i < len(n); i++ {
		c := n[i]
		if c < 0x21 || c > 0x7e || c == '#' || isDelimiter(c) {
			fmt.Fprintf(b, "#%02X", c)
		} else {
			b.WriteByte(c)
		}
	}
}

// writeString writes a literal string, escaping what a reader would
// otherwise take as syntax.
func writeString(b *bytes.Buffer, s String) {
	b.WriteByte('(')
	for _, c := range s {
		switch c {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if c < 0x20 || c > 0x7e {
				fmt.Fprintf(b, "\\%03o", c)
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte(')')
}
