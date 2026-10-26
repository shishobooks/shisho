package mobi

import (
	"bytes"
	"encoding/binary"

	"github.com/pkg/errors"
)

// HUFF/CDIC text is a stream of Huffman codes, read most significant bit
// first, each naming a phrase of a dictionary. The book's header names its
// HUFF record, which holds the code tables, followed by CDIC records, which
// hold the dictionary. A phrase is stored as it is, or, when the high bit
// of its length is clear, HUFF/CDIC compressed itself; it is expanded the
// first time a code names it.
//
// A code is read from the next 32 bits. The first table, indexed by the top
// 8 bits, gives a code length, whether the code is known to be that long,
// and the maximum code of that length. When it is not, the code is as long
// as the shortest length whose minimum code, from the second table, is not
// above it, and that length's maximum code is used. The phrase's index is
// the maximum code less the code, both taken at the code's length.

// huffCode is an entry of a HUFF record's first table, with its maximum
// code shifted to the top of 32 bits and the bits below it set.
type huffCode struct {
	length   int
	terminal bool
	max      uint64
}

// huffPhrase is a dictionary phrase. A phrase not yet expanded is still
// compressed; expanding marks one being expanded, so a phrase that names
// itself is found.
type huffPhrase struct {
	data      []byte
	expanded  bool
	expanding bool
}

type huffDecoder struct {
	codes [256]huffCode
	// min and max hold each code length's minimum and maximum code,
	// shifted as huffCode.max is.
	min, max [33]uint64
	phrases  []huffPhrase
	// expanded counts the bytes of the phrases expanded so far.
	expanded int
}

const (
	// maxHuffDepth caps how deep phrases may nest. Real dictionaries nest a
	// few levels; each level costs a stack frame.
	maxHuffDepth = 32
	// maxHuffPhrases caps the dictionary, which real books keep to tens of
	// thousands of phrases.
	maxHuffPhrases = 1 << 20
	// maxHuffExpanded caps the bytes of all expanded phrases, which are
	// kept: each is short in a real dictionary, but a crafted one could
	// give every text record its own chain of long phrases.
	maxHuffExpanded = 64 << 20
)

// readHuffCDIC reads the HUFF and CDIC records of the book whose header
// record, at base, is rec.
func readHuffCDIC(db *pdb, base int, rec []byte) (*huffDecoder, error) {
	be := binary.BigEndian
	first, count := be.Uint32(rec[offHuffRecord:]), be.Uint32(rec[offHuffCount:])
	if count < 2 || int64(base)+int64(first)+int64(count) > int64(db.count()) {
		return nil, errors.New("MOBI HUFF/CDIC records are out of range")
	}
	huff, err := db.record(base+int(first), maxIndexRecord)
	if err != nil {
		return nil, err
	}
	d := &huffDecoder{}
	if err := d.readTables(huff); err != nil {
		return nil, err
	}
	for i := 1; i < int(count); i++ {
		cdic, err := db.record(base+int(first)+i, maxIndexRecord)
		if err != nil {
			return nil, err
		}
		if err := d.readPhrases(cdic); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func (d *huffDecoder) readTables(rec []byte) error {
	be := binary.BigEndian
	if len(rec) < 16 || !bytes.Equal(rec[:8], []byte("HUFF\x00\x00\x00\x18")) {
		return errors.New("MOBI HUFF record is invalid")
	}
	codes, limits := int64(be.Uint32(rec[8:])), int64(be.Uint32(rec[12:]))
	if codes+256*4 > int64(len(rec)) || limits+64*4 > int64(len(rec)) {
		return errors.New("MOBI HUFF record is truncated")
	}
	for i := range d.codes {
		v := be.Uint32(rec[codes+4*int64(i):])
		c := huffCode{length: int(v & 0x1F), terminal: v&0x80 != 0}
		// A code of 8 bits or fewer is whole in the 8 bits that index the
		// table, so it is always known to be that long.
		if c.length == 0 || c.length <= 8 && !c.terminal {
			return errors.New("MOBI HUFF record has an invalid code")
		}
		c.max = (uint64(v>>8)+1)<<(32-c.length) - 1
		d.codes[i] = c
	}
	for length := 1; length <= 32; length++ {
		at := limits + 8*int64(length-1)
		d.min[length] = uint64(be.Uint32(rec[at:])) << (32 - length)
		d.max[length] = (uint64(be.Uint32(rec[at+4:]))+1)<<(32-length) - 1
	}
	return nil
}

// readPhrases adds a CDIC record's phrases to the dictionary. A record
// holds 1<<bits phrases, or fewer when the dictionary's phrase count is
// reached; each is a 16-bit length, its high bit set when the phrase is
// stored as it is, and the phrase.
func (d *huffDecoder) readPhrases(rec []byte) error {
	be := binary.BigEndian
	if len(rec) < 16 || !bytes.Equal(rec[:8], []byte("CDIC\x00\x00\x00\x10")) {
		return errors.New("MOBI CDIC record is invalid")
	}
	total, bits := int64(be.Uint32(rec[8:])), be.Uint32(rec[12:])
	if total > maxHuffPhrases || bits > 16 {
		return errors.New("MOBI CDIC record is too large")
	}
	n := min(int64(1)<<bits, total-int64(len(d.phrases)))
	if n < 0 || 16+2*n > int64(len(rec)) {
		return errors.New("MOBI CDIC record is truncated")
	}
	for i := range n {
		at := 16 + int(be.Uint16(rec[16+2*i:]))
		if at+2 > len(rec) {
			return errors.New("MOBI CDIC phrase runs past its record")
		}
		length := be.Uint16(rec[at:])
		end := at + 2 + int(length&0x7FFF)
		if end > len(rec) {
			return errors.New("MOBI CDIC phrase runs past its record")
		}
		d.phrases = append(d.phrases, huffPhrase{data: rec[at+2 : end], expanded: length&0x8000 != 0})
	}
	return nil
}

// decompress decompresses one text record, refusing output longer than
// limit.
func (d *huffDecoder) decompress(data []byte, limit int) ([]byte, error) {
	var out []byte
	if err := d.expand(&out, data, 0, limit); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *huffDecoder) expand(out *[]byte, data []byte, depth, limit int) error {
	if depth > maxHuffDepth {
		return errors.New("MOBI HUFF/CDIC phrases nest too deep")
	}
	left := 8 * len(data)
	// Eight zero bytes past the end let every 32-bit window be read
	// whole; the bit count stops decoding before them.
	padded := append(data[:len(data):len(data)], make([]byte, 8)...)
	for pos := 0; ; {
		at := pos / 8
		window := uint64(padded[at])<<32 | uint64(binary.BigEndian.Uint32(padded[at+1:]))
		code := window >> (8 - pos%8) & 0xFFFFFFFF
		c := d.codes[code>>24]
		length, maxCode := c.length, c.max
		if !c.terminal {
			for length < 32 && code < d.min[length] {
				length++
			}
			maxCode = d.max[length]
		}
		if left -= length; left < 0 {
			return nil
		}
		pos += length
		if maxCode < code {
			return errors.New("MOBI HUFF/CDIC code is out of range")
		}
		index := (maxCode - code) >> (32 - length)
		if index >= uint64(len(d.phrases)) {
			return errors.New("MOBI HUFF/CDIC code names no phrase")
		}
		p := &d.phrases[index]
		if !p.expanded {
			if p.expanding {
				return errors.New("MOBI HUFF/CDIC phrase contains itself")
			}
			p.expanding = true
			var phrase []byte
			if err := d.expand(&phrase, p.data, depth+1, limit); err != nil {
				return err
			}
			if d.expanded += len(phrase); d.expanded > maxHuffExpanded {
				return errors.New("MOBI HUFF/CDIC phrases expand too large")
			}
			p.data, p.expanded, p.expanding = phrase, true, false
		}
		if len(*out)+len(p.data) > limit {
			return errors.New("MOBI HUFF/CDIC text record is too long")
		}
		*out = append(*out, p.data...)
	}
}
