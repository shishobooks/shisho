package mobigen

import (
	"bytes"
	"encoding/binary"
	"sort"
)

// HUFF/CDIC text is a stream of Huffman codes, each naming a phrase of the
// dictionary the CDIC records hold. A phrase is stored as it is or, when
// its flag bit is clear, itself HUFF/CDIC compressed, which readers expand
// on first use.
//
// The encoder here is simple but covers both code paths a reader takes: the
// first huffReserved 8-bit prefixes start 12-bit codes, found through the
// HUFF record's second table, and every other prefix is a whole 8-bit code,
// found through its first table. Indexes 0 to 255-huffReserved are 8-bit
// codes and the rest are 12-bit codes. The dictionary is every single byte
// followed by huffPhrases, so the codes for byte 0xFF and the phrases are 12
// bits long, and two CDIC records are needed to hold it.

// huffPhrases are the dictionary's multibyte phrases. Those marked
// compressed are stored HUFF/CDIC compressed themselves.
var huffPhrases = []struct {
	text       string
	compressed bool
}{
	{"</p><p>", true},
	{"Chapter ", false},
	{"the ", true},
	{" filler text keeps the chapter long.", true},
	{"filepos=", false},
	{"Ünïcödé", false},
}

const (
	huffReserved   = 1
	huffMaxCode12  = 255 + 15*huffReserved
	huffCDICBits   = 8
	huffDictionary = 256
)

// huffCode returns the code of dictionary index i and its length in bits.
func huffCode(i int) (uint32, int) {
	if i <= 255-huffReserved {
		return u32(255 - i), 8
	}
	return u32(huffMaxCode12 - i), 12
}

// huffEncode returns text as HUFF/CDIC codes, padded with zero bits to a
// whole byte. With phrases set, it uses the longest phrase that matches at
// each place; without, only single bytes. Zero padding is always fewer bits
// than a code whose prefix is 0, which is a 12-bit code, so readers stop
// before it.
func huffEncode(text []byte, phrases bool) []byte {
	order := make([]int, len(huffPhrases))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return len(huffPhrases[order[a]].text) > len(huffPhrases[order[b]].text) })

	var out []byte
	var acc uint64
	bits := 0
	put := func(code uint32, n int) {
		acc = acc<<n | uint64(code)
		bits += n
		for bits >= 8 {
			out = append(out, byte(acc>>(bits-8)))
			bits -= 8
		}
	}
	for len(text) > 0 {
		index, size := int(text[0]), 1
		if phrases {
			for _, p := range order {
				if bytes.HasPrefix(text, []byte(huffPhrases[p].text)) {
					index, size = huffDictionary+p, len(huffPhrases[p].text)
					break
				}
			}
		}
		put(huffCode(index))
		text = text[size:]
	}
	if bits > 0 {
		put(0, 8-bits)
	}
	return out
}

// huffRecord returns the HUFF record: its header, the 256 entries of the
// table read by a code's first 8 bits, and the minimum and maximum code of
// each code length from 1 to 32. An entry holds a code length in its low 5
// bits, a flag saying the code is that long in bit 7, and the maximum code
// of that length above them; a maximum code is the code whose phrase has
// index 0, so a code's phrase is the maximum less the code.
func huffRecord() []byte {
	be := binary.BigEndian
	r := make([]byte, 24+256*4+64*4)
	copy(r, "HUFF")
	be.PutUint32(r[4:], 24)
	be.PutUint32(r[8:], 24)
	be.PutUint32(r[12:], 24+256*4)
	// An entry that is not a whole code gives the shortest length its
	// codes may have, which is more than 8.
	for prefix := range 256 {
		entry := uint32(9)
		if prefix >= huffReserved {
			entry = 8 | 0x80 | 255<<8
		}
		be.PutUint32(r[24+4*prefix:], entry)
	}
	limits := r[24+256*4:]
	// A 12-bit code is below the minimum code of each shorter length, so a
	// reader lengthens it to 12, where the minimum is 0.
	for length := 9; length < 12; length++ {
		be.PutUint32(limits[8*(length-1):], huffReserved<<(length-8))
	}
	be.PutUint32(limits[8*(12-1)+4:], huffMaxCode12)
	return r
}

// cdicRecords returns the CDIC records holding the dictionary, 1<<huffCDICBits
// phrases each: a header with the dictionary's phrase count and the bits,
// each phrase's offset after the header, and the phrases, each a length
// with the high bit set when it is stored as it is.
func cdicRecords() [][]byte {
	type phrase struct {
		data    []byte
		literal bool
	}
	var dict []phrase
	for b := range huffDictionary {
		dict = append(dict, phrase{data: []byte{byte(b)}, literal: true})
	}
	for _, p := range huffPhrases {
		if p.compressed {
			dict = append(dict, phrase{data: huffEncode([]byte(p.text), false)})
		} else {
			dict = append(dict, phrase{data: []byte(p.text), literal: true})
		}
	}

	be := binary.BigEndian
	var records [][]byte
	for start := 0; start < len(dict); start += 1 << huffCDICBits {
		group := dict[start:min(start+1<<huffCDICBits, len(dict))]
		var body bytes.Buffer
		offsets := make([]byte, 2*len(group))
		for i, p := range group {
			be.PutUint16(offsets[2*i:], u16(len(offsets)+body.Len()))
			length := u16(len(p.data))
			if p.literal {
				length |= 0x8000
			}
			_ = binary.Write(&body, be, length)
			body.Write(p.data)
		}
		r := make([]byte, 16, 16+len(offsets)+body.Len())
		copy(r, "CDIC")
		be.PutUint32(r[4:], 16)
		be.PutUint32(r[8:], u32(len(dict)))
		be.PutUint32(r[12:], huffCDICBits)
		r = append(r, offsets...)
		records = append(records, append(r, body.Bytes()...))
	}
	return records
}
