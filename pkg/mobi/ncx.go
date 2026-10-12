package mobi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/bits"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// The table of contents of a book is its NCX index: a primary INDX record
// holding the TAGX table, then INDX data records holding the entries, then
// CNCX records holding the labels. Each entry is an identifier, control
// bytes, and tag values; the TAGX table says which control-byte bits mark
// which tags. Parse and Rewrite both read it through readNCX, and Rewrite
// writes it back through ncxIndex.encode.

// NCX tags, as KindleGen and Calibre number them.
const (
	ncxTagPos        = 1  // MOBI6: byte offset of the entry in the text
	ncxTagLabel      = 3  // CNCX offset of the title
	ncxTagDepth      = 4  // 0 at the top level
	ncxTagPosFID     = 6  // KF8: fragment and offset in it
	ncxTagParent     = 21 // index of the parent entry
	ncxTagFirstChild = 22 // index of the first child entry
	ncxTagLastChild  = 23 // index of the last child entry
)

// Offsets into an INDX record header.
const (
	indxHeaderLength = 4
	indxIDXTOffset   = 20
	indxCount        = 24 // data records in the primary, entries in a data record
	indxTotal        = 36 // entries in all data records, in the primary
	indxCNCXCount    = 52
	indxTAGXOffset   = 180
	indxMinHeader    = indxTAGXOffset + 4

	// A record index is 16 bits, and a CNCX offset is a record number
	// times 0x10000 plus an offset into that record.
	maxIndexRecord = 0x10000

	// A crafted index must not cost more than a real one: real tables of
	// contents have hundreds of entries and a few levels, and the tree is
	// walked recursively.
	maxNCXEntries = 50_000
	maxTOCDepth   = 64
)

// tagxEntry is one row of a TAGX table. An end entry closes a control byte.
type tagxEntry struct {
	tag, valuesPerEntry, mask byte
	end                       bool
}

// ncxEntry is one index entry.
type ncxEntry struct {
	ident []byte
	// raw is the entry as written, after the identifier; an entry whose
	// values do not change is written back with it.
	raw    []byte
	values map[byte][]uint32
}

func (e *ncxEntry) value(tag byte) (uint32, bool) {
	if v := e.values[tag]; len(v) > 0 {
		return v[0], true
	}
	return 0, false
}

// ncxIndex is a parsed NCX index.
type ncxIndex struct {
	primary      int // absolute record index of the primary INDX record
	primaryRec   []byte
	controlBytes int
	tags         []tagxEntry
	dataRecs     [][]byte
	// entries lists every entry in index order; recordOf names the data
	// record each one is in.
	entries  []*ncxEntry
	recordOf []int
	cncx     [][]byte
}

// recordSource reads records of a Palm database.
type recordSource interface {
	numRecords() int
	readRecord(i int) ([]byte, error)
}

func (p *pdb) numRecords() int { return p.count() }

func (p *pdb) readRecord(i int) ([]byte, error) { return p.record(i, maxIndexRecord) }

// readNCX reads the NCX index whose primary record is at index primary.
func readNCX(src recordSource, primary int) (*ncxIndex, error) {
	be := binary.BigEndian
	if primary <= 0 || primary >= src.numRecords() {
		return nil, errors.New("NCX index is out of range")
	}
	rec, err := src.readRecord(primary)
	if err != nil {
		return nil, err
	}
	if len(rec) < indxMinHeader || string(rec[:4]) != "INDX" {
		return nil, errors.New("NCX index has no INDX header")
	}
	idx := &ncxIndex{primary: primary, primaryRec: rec}

	tagxStart := int64(be.Uint32(rec[indxTAGXOffset:]))
	if tagxStart == 0 {
		tagxStart = int64(be.Uint32(rec[indxHeaderLength:]))
	}
	if tagxStart+12 > int64(len(rec)) || string(rec[tagxStart:tagxStart+4]) != "TAGX" {
		return nil, errors.New("NCX index has no TAGX table")
	}
	tagxEnd := tagxStart + int64(be.Uint32(rec[tagxStart+4:]))
	if tagxEnd > int64(len(rec)) {
		return nil, errors.New("NCX TAGX table is truncated")
	}
	idx.controlBytes = int(be.Uint32(rec[tagxStart+8:]))
	ends := 0
	for off := tagxStart + 12; off+4 <= tagxEnd; off += 4 {
		t := tagxEntry{tag: rec[off], valuesPerEntry: rec[off+1], mask: rec[off+2], end: rec[off+3]&1 != 0}
		if t.end {
			ends++
		}
		idx.tags = append(idx.tags, t)
	}
	if idx.controlBytes < 1 || ends != idx.controlBytes || !idx.tags[len(idx.tags)-1].end {
		return nil, errors.New("NCX TAGX table does not match its control byte count")
	}

	dataCount := int(be.Uint32(rec[indxCount:]))
	cncxCount := int(be.Uint32(rec[indxCNCXCount:]))
	if primary+1+dataCount+cncxCount > src.numRecords() {
		return nil, errors.New("NCX index runs past the last record")
	}
	for d := range dataCount {
		data, err := src.readRecord(primary + 1 + d)
		if err != nil {
			return nil, err
		}
		offsets, err := idxtOffsets(data)
		if err != nil {
			return nil, err
		}
		idx.dataRecs = append(idx.dataRecs, data)
		for i := 0; i+1 < len(offsets); i++ {
			entry, err := idx.decodeEntry(data[offsets[i]:offsets[i+1]])
			if err != nil {
				return nil, err
			}
			idx.entries = append(idx.entries, entry)
			idx.recordOf = append(idx.recordOf, d)
			if len(idx.entries) > maxNCXEntries {
				return nil, errors.New("NCX index has too many entries")
			}
		}
	}
	for c := range cncxCount {
		data, err := src.readRecord(primary + 1 + dataCount + c)
		if err != nil {
			return nil, err
		}
		idx.cncx = append(idx.cncx, data)
	}
	return idx, nil
}

// idxtOffsets returns the entry offsets an INDX record's IDXT block lists,
// followed by the IDXT offset, where the last entry ends.
func idxtOffsets(rec []byte) ([]int, error) {
	be := binary.BigEndian
	if len(rec) < indxMinHeader || string(rec[:4]) != "INDX" {
		return nil, errors.New("NCX record has no INDX header")
	}
	start := int(be.Uint32(rec[indxIDXTOffset:]))
	count := int(be.Uint32(rec[indxCount:]))
	if start+4+2*count > len(rec) || string(rec[start:start+4]) != "IDXT" {
		return nil, errors.New("NCX record has no IDXT block")
	}
	offsets := make([]int, count+1)
	for i := range count {
		offsets[i] = int(be.Uint16(rec[start+4+2*i:]))
	}
	offsets[count] = start
	for i := range count {
		if offsets[i] < indxMinHeader || offsets[i] >= offsets[i+1] {
			return nil, errors.New("NCX record has an invalid entry offset")
		}
	}
	return offsets, nil
}

// decodeEntry reads one entry: a length-prefixed identifier, the control
// bytes, and then the values of each tag the control bytes mark.
func (idx *ncxIndex) decodeEntry(data []byte) (*ncxEntry, error) {
	identLen := int(data[0])
	if 1+identLen+idx.controlBytes > len(data) {
		return nil, errors.New("NCX entry is truncated")
	}
	e := &ncxEntry{ident: data[1 : 1+identLen], raw: data[1+identLen:], values: map[byte][]uint32{}}
	control := e.raw[:idx.controlBytes]
	rest := e.raw[idx.controlBytes:]

	// A tag's control bits hold its entry count, or, when every bit of a
	// multi-bit mask is set, say that a byte length of its values follows
	// the control bytes instead.
	type present struct {
		tag       tagxEntry
		count     int
		byteCount int
	}
	var tags []present
	cb := 0
	for _, t := range idx.tags {
		if t.end {
			cb++
			continue
		}
		if t.mask == 0 || cb >= len(control) {
			continue
		}
		v := control[cb] & t.mask
		if v == 0 {
			continue
		}
		p := present{tag: t, byteCount: -1}
		if v == t.mask && bits.OnesCount8(t.mask) > 1 {
			n, size := forwardInt(rest)
			if size == 0 {
				return nil, errors.New("NCX entry is truncated")
			}
			p.byteCount = int(n)
			rest = rest[size:]
		} else {
			p.count = int(v >> bits.TrailingZeros8(t.mask))
		}
		tags = append(tags, p)
	}
	for _, p := range tags {
		var values []uint32
		if p.byteCount >= 0 {
			for consumed := 0; consumed < p.byteCount; {
				n, size := forwardInt(rest)
				if size == 0 {
					return nil, errors.New("NCX entry is truncated")
				}
				values = append(values, n)
				rest, consumed = rest[size:], consumed+size
			}
		} else {
			for range p.count * int(p.tag.valuesPerEntry) {
				n, size := forwardInt(rest)
				if size == 0 {
					return nil, errors.New("NCX entry is truncated")
				}
				values = append(values, n)
				rest = rest[size:]
			}
		}
		e.values[p.tag.tag] = values
	}
	return e, nil
}

// encodeEntry writes an entry's values the way decodeEntry reads them.
func (idx *ncxIndex) encodeEntry(e *ncxEntry) ([]byte, error) {
	control := make([]byte, idx.controlBytes)
	var lengths, values []byte
	cb := 0
	for _, t := range idx.tags {
		if t.end {
			cb++
			continue
		}
		v := e.values[t.tag]
		if len(v) == 0 || t.mask == 0 || t.valuesPerEntry == 0 {
			continue
		}
		var encoded []byte
		for _, n := range v {
			encoded = append(encoded, appendForwardInt(nil, n)...)
		}
		count := len(v) / int(t.valuesPerEntry)
		shifted := count << bits.TrailingZeros8(t.mask)
		switch {
		case len(v)%int(t.valuesPerEntry) == 0 && shifted&^int(t.mask) == 0 &&
			(byte(shifted) != t.mask || bits.OnesCount8(t.mask) == 1):
			control[cb] |= byte(shifted)
		case bits.OnesCount8(t.mask) > 1:
			control[cb] |= t.mask
			lengths = appendForwardInt(lengths, uint32(len(encoded))) //nolint:gosec // entry values are small
		default:
			return nil, errors.Errorf("NCX tag %d cannot hold %d values", t.tag, len(v))
		}
		values = append(values, encoded...)
	}
	out := append([]byte{byte(len(e.ident))}, e.ident...)
	out = append(out, control...)
	out = append(out, lengths...)
	return append(out, values...), nil
}

// forwardInt reads a number stored in 7-bit groups, most significant
// first, whose last byte has the high bit set. It returns the number of bytes
// read, or 0 when data ends before the last byte.
func forwardInt(data []byte) (uint32, int) {
	var n uint32
	for i, b := range data {
		if i == 5 {
			return 0, 0
		}
		n = n<<7 | uint32(b&0x7F)
		if b&0x80 != 0 {
			return n, i + 1
		}
	}
	return 0, 0
}

func appendForwardInt(out []byte, n uint32) []byte {
	var groups []byte
	groups = append(groups, byte(n&0x7F)|0x80)
	for n >>= 7; n > 0; n >>= 7 {
		groups = append(groups, byte(n&0x7F))
	}
	for i := len(groups) - 1; i >= 0; i-- {
		out = append(out, groups[i])
	}
	return out
}

// label returns the raw CNCX string at offset, and false when there is
// none.
func (idx *ncxIndex) label(offset uint32) ([]byte, bool) {
	r, pos := int(offset/maxIndexRecord), int(offset%maxIndexRecord)
	if r >= len(idx.cncx) || pos >= len(idx.cncx[r]) {
		return nil, false
	}
	rec := idx.cncx[r]
	n, size := forwardInt(rec[pos:])
	if size == 0 || pos+size+int(n) > len(rec) {
		return nil, false
	}
	return rec[pos+size : pos+size+int(n)], true
}

// cncxEnd returns where the strings of a CNCX record end and its zero
// padding begins.
func cncxEnd(rec []byte) int {
	pos := 0
	for pos < len(rec) {
		n, size := forwardInt(rec[pos:])
		if size == 0 || pos+size+int(n) > len(rec) {
			break
		}
		pos += size + int(n)
	}
	return pos
}

// tocNode is an entry placed in the tree of a table of contents.
type tocNode struct {
	index    int
	children []*tocNode
}

// tree nests the entries. An index with parent tags (KindleGen's, and
// Calibre's KF8 output) nests through them; one without nests by depth in
// index order, and an index where every entry is at depth 0 (Calibre's MOBI6
// output) is flat. A parent must come before its child, which rules out
// cycles, and an entry nested deeper than maxTOCDepth goes to the top level.
func (idx *ncxIndex) tree() []*tocNode {
	nodes := make([]*tocNode, len(idx.entries))
	for i := range nodes {
		nodes[i] = &tocNode{index: i}
	}
	hasParents := false
	for _, e := range idx.entries {
		if _, ok := e.value(ncxTagParent); ok {
			hasParents = true
			break
		}
	}

	var roots []*tocNode
	if hasParents {
		depth := make([]int, len(nodes))
		for i, e := range idx.entries {
			if p, ok := e.value(ncxTagParent); ok && int64(p) < int64(i) && depth[p] < maxTOCDepth-1 {
				depth[i] = depth[p] + 1
				nodes[p].children = append(nodes[p].children, nodes[i])
			} else {
				roots = append(roots, nodes[i])
			}
		}
		return roots
	}

	type level struct {
		depth uint32
		node  *tocNode
	}
	var stack []level
	for i, e := range idx.entries {
		depth, _ := e.value(ncxTagDepth)
		for len(stack) > 0 && (stack[len(stack)-1].depth >= depth || len(stack) >= maxTOCDepth) {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 {
			parent := stack[len(stack)-1].node
			parent.children = append(parent.children, nodes[i])
		} else {
			roots = append(roots, nodes[i])
		}
		stack = append(stack, level{depth: depth, node: nodes[i]})
	}
	return roots
}

// locator names where an entry points, the way links in the book do:
// kindle:pos for KF8, filepos for MOBI6. Shisho only shows it.
func (e *ncxEntry) locator() string {
	if v := e.values[ncxTagPosFID]; len(v) >= 2 {
		return fmt.Sprintf("kindle:pos:fid:%04s:off:%010s", base32(v[0]), base32(v[1]))
	}
	if pos, ok := e.value(ncxTagPos); ok {
		return fmt.Sprintf("filepos:%010d", pos)
	}
	return ""
}

func base32(n uint32) string {
	return strings.ToUpper(strconv.FormatUint(uint64(n), 32))
}

// title returns an entry's label, or "" when it has none.
func (idx *ncxIndex) title(e *ncxEntry, h *header) string {
	offset, ok := e.value(ncxTagLabel)
	if !ok {
		return ""
	}
	raw, ok := idx.label(offset)
	if !ok {
		return ""
	}
	return h.decode(raw)
}

// encode returns the index's records with entries in place of the parsed
// ones: the primary record, then the data records, then the CNCX records.
// Entries stay in the data record they came from, so the record count, and
// every record index in the file, stays the same; a data record whose entries
// were all deleted is written with none. kept[i] is the parsed index of
// entries[i].
func (idx *ncxIndex) encode(entries []*ncxEntry, kept []int, cncx [][]byte) ([][]byte, error) {
	be := binary.BigEndian
	perRecord := make([][]*ncxEntry, len(idx.dataRecs))
	for i, e := range entries {
		d := idx.recordOf[kept[i]]
		perRecord[d] = append(perRecord[d], e)
	}

	out := make([][]byte, 0, 1+len(idx.dataRecs)+len(cncx))
	out = append(out, nil) // the primary, built last
	for d, rec := range idx.dataRecs {
		headerLen := int(be.Uint32(rec[indxHeaderLength:]))
		if headerLen < indxMinHeader || headerLen > len(rec) {
			return nil, errors.New("NCX record header length is invalid")
		}
		var body bytes.Buffer
		offsets := make([]int, 0, len(perRecord[d]))
		for _, e := range perRecord[d] {
			offsets = append(offsets, headerLen+body.Len())
			if e.raw != nil {
				body.WriteByte(byte(len(e.ident)))
				body.Write(e.ident)
				body.Write(e.raw)
				continue
			}
			raw, err := idx.encodeEntry(e)
			if err != nil {
				return nil, err
			}
			body.Write(raw)
		}
		padTo4(&body)
		data := append([]byte(nil), rec[:headerLen]...)
		be.PutUint32(data[indxIDXTOffset:], uint32(headerLen+body.Len())) //nolint:gosec // a record is under 64 KiB
		be.PutUint32(data[indxCount:], uint32(len(offsets)))              //nolint:gosec // as above
		data = append(data, body.Bytes()...)
		data = append(data, idxtBlock(offsets)...)
		if len(data) > maxIndexRecord {
			return nil, errors.New("NCX record is too large")
		}
		out = append(out, data)
	}
	out = append(out, cncx...)

	// The primary record ends with each data record's last identifier and
	// entry count. A data record left empty keeps its old last identifier.
	geometry, err := idxtOffsets(idx.primaryRec)
	if err != nil || len(geometry)-1 != len(idx.dataRecs) {
		return nil, errors.New("NCX primary record has no record geometry")
	}
	start := geometry[0]
	for _, off := range geometry[:len(geometry)-1] {
		start = min(start, off)
	}
	var body bytes.Buffer
	offsets := make([]int, len(idx.dataRecs))
	for d := range idx.dataRecs {
		offsets[d] = start + body.Len()
		ident := idx.lastIdent(d)
		if n := len(perRecord[d]); n > 0 {
			ident = perRecord[d][n-1].ident
		}
		body.WriteByte(byte(len(ident)))
		body.Write(ident)
		_ = binary.Write(&body, be, uint16(len(perRecord[d]))) //nolint:gosec // as above
	}
	padTo4(&body)
	primary := append([]byte(nil), idx.primaryRec[:start]...)
	be.PutUint32(primary[indxIDXTOffset:], uint32(start+body.Len())) //nolint:gosec // as above
	be.PutUint32(primary[indxTotal:], uint32(len(entries)))          //nolint:gosec // as above
	primary = append(primary, body.Bytes()...)
	primary = append(primary, idxtBlock(offsets)...)
	out[0] = primary
	return out, nil
}

// lastIdent returns the identifier of the last parsed entry in data record
// d, or nil when it had none.
func (idx *ncxIndex) lastIdent(d int) []byte {
	var ident []byte
	for i, e := range idx.entries {
		if idx.recordOf[i] == d {
			ident = e.ident
		}
	}
	return ident
}

func idxtBlock(offsets []int) []byte {
	var b bytes.Buffer
	b.WriteString("IDXT")
	for _, off := range offsets {
		_ = binary.Write(&b, binary.BigEndian, uint16(off)) //nolint:gosec // offsets into a record under 64 KiB
	}
	padTo4(&b)
	return b.Bytes()
}

func padTo4(b *bytes.Buffer) {
	b.Write(make([]byte, (4-b.Len()%4)%4))
}
