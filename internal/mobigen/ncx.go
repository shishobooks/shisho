package mobigen

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/pkg/errors"
	"golang.org/x/text/encoding/charmap"
)

// NCX tags, as KindleGen and Calibre number them.
const (
	ncxTagPos        = 1
	ncxTagLength     = 2
	ncxTagLabel      = 3
	ncxTagDepth      = 4
	ncxTagPosFID     = 6
	ncxTagParent     = 21
	ncxTagFirstChild = 22
	ncxTagLastChild  = 23

	indxHeaderLength = 192
)

// ncxTag is one TAGX entry: a tag, its values per entry, and its bit in the
// control byte.
type ncxTag struct {
	tag, values, mask byte
}

// ncxEntry is one generated index entry. Every value but the label and
// the child range is a placeholder: nothing reads the text at a position.
type ncxEntry struct {
	title                 string
	depth                 int
	parent                int // -1 at the top level
	firstChild, lastChild int // -1 without children
	order                 int // position in reading order
	target                string
}

// ncxRecords returns the NCX index of a book: the primary INDX record, the
// data records, and the CNCX records. perRecord and perCNCX, when positive,
// cap the entries in a data record and the labels in a CNCX record, so a
// test can get the multi-record layout a large book has. Entries are listed a
// level at a time, as KindleGen and Calibre's KF8 output do, so each entry's
// children are a contiguous range; a flat index lists them in reading order
// at depth 0, as Calibre's MOBI6 output does.
// targets, for KF8 content, maps an entry's Target to its fragment and
// offset; without it, targets are ignored.
func ncxRecords(toc []TOCEntry, kf8, flat, cp1252 bool, perRecord, perCNCX int, targets map[string][2]uint32) ([][]byte, error) {
	entries := levelOrder(toc)
	if flat {
		entries = readingOrder(toc)
	}
	if perRecord <= 0 {
		perRecord = len(entries)
	}
	if perCNCX <= 0 {
		perCNCX = len(entries)
	}

	tags := []ncxTag{{ncxTagPos, 1, 0x01}, {ncxTagLength, 1, 0x02}, {ncxTagLabel, 1, 0x04}, {ncxTagDepth, 1, 0x08}}
	if !flat {
		tags = append(tags, ncxTag{ncxTagParent, 1, 0x10}, ncxTag{ncxTagFirstChild, 1, 0x20}, ncxTag{ncxTagLastChild, 1, 0x40})
	}
	if kf8 {
		tags = append(tags, ncxTag{ncxTagPosFID, 2, 0x80})
	}

	// A label's CNCX offset is its record number times 0x10000 plus its
	// offset in that record.
	var cncxRecords []*bytes.Buffer
	labels := make([]int, len(entries))
	for i, e := range entries {
		label := []byte(e.title)
		if cp1252 {
			var err error
			if label, err = charmap.Windows1252.NewEncoder().Bytes(label); err != nil {
				return nil, errors.Wrapf(err, "encode %q as Windows-1252", e.title)
			}
		}
		if i%perCNCX == 0 {
			cncxRecords = append(cncxRecords, &bytes.Buffer{})
		}
		rec := cncxRecords[len(cncxRecords)-1]
		labels[i] = (len(cncxRecords)-1)*0x10000 + rec.Len()
		rec.Write(forwardInt(uint32(len(label)))) //nolint:gosec // test labels are short
		rec.Write(label)
	}

	be := binary.BigEndian
	var dataRecords [][]byte
	type geometryEntry struct {
		lastIdent []byte
		count     int
	}
	var geometryEntries []geometryEntry
	for start := 0; start < len(entries); start += perRecord {
		var body bytes.Buffer
		var offsets []int
		var lastIdent []byte
		for i := start; i < min(start+perRecord, len(entries)); i++ {
			e := entries[i]
			offsets = append(offsets, indxHeaderLength+body.Len())
			lastIdent = []byte(fmt.Sprintf("%02X", i))
			values := map[byte][]uint32{
				ncxTagPos:    {u32(e.order * 10)},
				ncxTagLength: {10},
				ncxTagLabel:  {u32(labels[i])},
				ncxTagDepth:  {u32(e.depth)},
				ncxTagPosFID: {u32(e.order), 0},
			}
			if e.target != "" && targets != nil {
				target, ok := targets[e.target]
				if !ok {
					return nil, errors.Errorf("no element with id %q", e.target)
				}
				values[ncxTagPosFID] = target[:]
			}
			if e.parent >= 0 {
				values[ncxTagParent] = []uint32{u32(e.parent)}
			}
			if e.firstChild >= 0 {
				values[ncxTagFirstChild] = []uint32{u32(e.firstChild)}
				values[ncxTagLastChild] = []uint32{u32(e.lastChild)}
			}
			var control byte
			var data []byte
			for _, t := range tags {
				if v, ok := values[t.tag]; ok {
					control |= t.mask
					for _, n := range v {
						data = append(data, forwardInt(n)...)
					}
				}
			}
			body.WriteByte(byte(len(lastIdent)))
			body.Write(lastIdent)
			body.WriteByte(control)
			body.Write(data)
		}
		padTo4(&body)

		data := indxHeader()
		be.PutUint32(data[12:], 1)
		be.PutUint32(data[20:], u32(indxHeaderLength+body.Len()))
		be.PutUint32(data[24:], u32(len(offsets)))
		be.PutUint32(data[28:], mobiNullIndex)
		data = append(data, body.Bytes()...)
		data = append(data, idxt(offsets)...)
		dataRecords = append(dataRecords, data)
		geometryEntries = append(geometryEntries, geometryEntry{lastIdent, len(offsets)})
	}

	var tagx bytes.Buffer
	tagx.WriteString("TAGX")
	_ = binary.Write(&tagx, be, u32(12+4*(len(tags)+1)))
	_ = binary.Write(&tagx, be, uint32(1)) // control byte count
	for _, t := range tags {
		tagx.Write([]byte{t.tag, t.values, t.mask, 0})
	}
	tagx.Write([]byte{0, 0, 0, 1})

	// The primary record ends with each data record's last identifier and
	// entry count, which a reader can search by identifier.
	var geometry bytes.Buffer
	geometry.Write(tagx.Bytes())
	geometryOffsets := make([]int, 0, len(geometryEntries))
	for _, g := range geometryEntries {
		geometryOffsets = append(geometryOffsets, indxHeaderLength+geometry.Len())
		geometry.WriteByte(byte(len(g.lastIdent)))
		geometry.Write(g.lastIdent)
		_ = binary.Write(&geometry, be, u16(g.count))
	}
	padTo4(&geometry)

	primary := indxHeader()
	be.PutUint32(primary[16:], 2)
	be.PutUint32(primary[20:], u32(indxHeaderLength+geometry.Len()))
	be.PutUint32(primary[24:], u32(len(dataRecords)))
	be.PutUint32(primary[28:], mobiEncodingUTF8)
	be.PutUint32(primary[36:], u32(len(entries)))
	be.PutUint32(primary[52:], u32(len(cncxRecords)))
	be.PutUint32(primary[180:], indxHeaderLength)
	primary = append(primary, geometry.Bytes()...)
	primary = append(primary, idxt(geometryOffsets)...)

	records := append([][]byte{primary}, dataRecords...)
	for _, rec := range cncxRecords {
		padTo4(rec)
		records = append(records, rec.Bytes())
	}
	return records, nil
}

func indxHeader() []byte {
	h := make([]byte, indxHeaderLength)
	copy(h, "INDX")
	binary.BigEndian.PutUint32(h[4:], indxHeaderLength)
	binary.BigEndian.PutUint32(h[32:], mobiNullIndex)
	return h
}

// idxt returns an IDXT block listing the entry offsets, padded to four
// bytes.
func idxt(offsets []int) []byte {
	var b bytes.Buffer
	b.WriteString("IDXT")
	for _, off := range offsets {
		_ = binary.Write(&b, binary.BigEndian, u16(off))
	}
	padTo4(&b)
	return b.Bytes()
}

func padTo4(b *bytes.Buffer) {
	b.Write(make([]byte, (4-b.Len()%4)%4))
}

// forwardInt encodes n in 7-bit groups, most significant first, with the
// high bit set on the last byte.
func forwardInt(n uint32) []byte {
	out := []byte{byte(n&0x7F) | 0x80}
	for n >>= 7; n > 0; n >>= 7 {
		out = append([]byte{byte(n & 0x7F)}, out...)
	}
	return out
}

// levelOrder lists the entries a level at a time, each level's entries
// grouped under their parents in order.
func levelOrder(toc []TOCEntry) []ncxEntry {
	order := map[*TOCEntry]int{}
	n := 0
	var number func(entries []TOCEntry)
	number = func(entries []TOCEntry) {
		for i := range entries {
			order[&entries[i]] = n
			n++
			number(entries[i].Children)
		}
	}
	number(toc)

	var out []ncxEntry
	level := make([]*TOCEntry, 0, len(toc))
	parents := make([]int, 0, len(toc))
	for i := range toc {
		level = append(level, &toc[i])
		parents = append(parents, -1)
	}
	for depth := 0; len(level) > 0; depth++ {
		start := len(out)
		for i, e := range level {
			out = append(out, ncxEntry{title: e.Title, depth: depth, parent: parents[i], firstChild: -1, lastChild: -1, order: order[e], target: e.Target})
		}
		var next []*TOCEntry
		var nextParents []int
		nextIndex := len(out)
		for i, e := range level {
			if len(e.Children) == 0 {
				continue
			}
			out[start+i].firstChild = nextIndex
			out[start+i].lastChild = nextIndex + len(e.Children) - 1
			for j := range e.Children {
				next = append(next, &e.Children[j])
				nextParents = append(nextParents, start+i)
			}
			nextIndex += len(e.Children)
		}
		level, parents = next, nextParents
	}
	return out
}

// readingOrder lists every entry in reading order at depth 0.
func readingOrder(toc []TOCEntry) []ncxEntry {
	var out []ncxEntry
	var walk func(entries []TOCEntry)
	walk = func(entries []TOCEntry) {
		for _, e := range entries {
			out = append(out, ncxEntry{title: e.Title, parent: -1, firstChild: -1, lastChild: -1, order: len(out)})
			walk(e.Children)
		}
	}
	walk(toc)
	return out
}
