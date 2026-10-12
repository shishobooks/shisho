package mobi

import (
	"encoding/binary"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
)

// Book names one of the books a file holds: a MOBI6 file holds a MOBI6
// book, an AZW3 file a KF8 book, and a combo file one of each.
type Book int

const (
	BookMOBI6 Book = iota
	BookKF8
)

// TOC is the table of contents of one book in a file.
type TOC struct {
	Book    Book
	Entries []*TOCEntry
}

// TOCEntry is an entry of a table of contents. Index is its position in
// the book's NCX index, which a TOCEdit names it by. An entry with no title
// never becomes a chapter.
type TOCEntry struct {
	Index    int
	Title    string
	Locator  string
	Children []*TOCEntry
}

// TOCEdit renames and deletes entries of a book's table of contents, by
// Index. Deleting an entry deletes the entries nested under it.
type TOCEdit struct {
	Renamed map[int]string
	Deleted map[int]bool
}

// TOCSkipError says why Rewrite left a book's table of contents as written
// instead of applying its TOCEdit.
type TOCSkipError struct {
	Book Book
	Err  error
}

func (e *TOCSkipError) Error() string { return e.Err.Error() }

// bookRef is a book in the file and the index of its header record.
type bookRef struct {
	kind   Book
	base   int
	header *header
}

// ReadTOC returns the tables of contents of the books in a MOBI or AZW3
// file that have a titled entry, in the order Parse prefers them: KF8 first.
// Parse reads chapters from the first.
func ReadTOC(r io.ReaderAt, size int64) ([]TOC, error) {
	db, err := readPDB(r, size)
	if err != nil {
		return nil, err
	}
	b, err := readBook(db)
	if err != nil {
		return nil, err
	}
	return readTOCs(db, b.books), nil
}

// readTOCs reads the table of contents of each book. A book whose index
// cannot be read has none, as a scan should still import the file.
func readTOCs(src recordSource, books []bookRef) []TOC {
	var tocs []TOC
	for _, b := range books {
		if b.header.ncx == nullIndex {
			continue
		}
		idx, err := readNCX(src, b.base+int(b.header.ncx))
		if err != nil {
			continue
		}
		entries := tocEntries(idx, b.header, idx.tree())
		if slices.ContainsFunc(entries, func(e *TOCEntry) bool { return e.Title != "" }) {
			tocs = append(tocs, TOC{Book: b.kind, Entries: entries})
		}
	}
	return tocs
}

func tocEntries(idx *ncxIndex, h *header, nodes []*tocNode) []*TOCEntry {
	out := make([]*TOCEntry, 0, len(nodes))
	for _, n := range nodes {
		e := idx.entries[n.index]
		out = append(out, &TOCEntry{
			Index:    n.index,
			Title:    idx.title(e, h),
			Locator:  e.locator(),
			Children: tocEntries(idx, h, n.children),
		})
	}
	return out
}

// parsedChapters turns entries into chapters. An entry with no title is
// left out with the entries under it, as the EPUB parser leaves out a nav
// entry with no label.
func parsedChapters(entries []*TOCEntry) []mediafile.ParsedChapter {
	var out []mediafile.ParsedChapter
	for _, e := range entries {
		if e.Title == "" {
			continue
		}
		ch := mediafile.ParsedChapter{Title: e.Title, Children: parsedChapters(e.Children)}
		if e.Locator != "" {
			locator := e.Locator
			ch.Href = &locator
		}
		out = append(out, ch)
	}
	return out
}

func (f *pdbFile) numRecords() int { return len(f.records) }

func (f *pdbFile) readRecord(i int) ([]byte, error) { return f.load(i) }

// applyTOCEdit writes edit into the NCX index of the book whose header
// record is at base. An edit that would delete every entry is left out, as
// the EPUB generator leaves out one that would empty its table of contents.
// It returns why an edit was left out when the index cannot take it: it
// cannot be read, or it has no room for a new label. The index is then left
// as written.
//
// Deleting entries renumbers the rest, which leaves stale the trailing byte
// sequences (TBS) that name the entries each text record touches, so they
// are removed from the text records along with the flag that announces them;
// a file without them is valid, and many older files have none.
func (f *pdbFile) applyTOCEdit(base int, h *header, edit *TOCEdit) error {
	ncx := headerField(f.records[base], offNCX)
	if ncx == nullIndex {
		return nil
	}
	idx, err := readNCX(f, base+int(ncx))
	if err != nil {
		return err
	}

	gone := make([]bool, len(idx.entries))
	var mark func(nodes []*tocNode, parentGone bool)
	mark = func(nodes []*tocNode, parentGone bool) {
		for _, n := range nodes {
			gone[n.index] = parentGone || edit.Deleted[n.index]
			mark(n.children, gone[n.index])
		}
	}
	mark(idx.tree(), false)
	if !slices.Contains(gone, false) {
		return nil
	}

	entries := slices.Clone(idx.entries)
	cncx := slices.Clone(idx.cncx)
	for i, title := range edit.Renamed {
		if i < 0 || i >= len(entries) || gone[i] || title == "" {
			continue
		}
		offset, err := idx.appendLabel(cncx, h.encode(title))
		if err != nil {
			return err
		}
		entries[i] = withValues(entries[i], map[byte][]uint32{ncxTagLabel: {offset}})
	}

	// kept lists the surviving entries' old indexes; before[i] counts the
	// survivors before old index i, which is the new index of a survivor.
	var kept []int
	before := make([]int, len(entries)+1)
	for i := range entries {
		before[i+1] = before[i]
		if !gone[i] {
			kept = append(kept, i)
			before[i+1]++
		}
	}
	ident := identScheme(idx.entries)
	survivors := make([]*ncxEntry, 0, len(kept))
	for n, i := range kept {
		e := renumber(entries[i], gone, before)
		if ident != nil && n != i {
			renumbered := *e
			renumbered.ident = ident(n)
			e = &renumbered
		}
		survivors = append(survivors, e)
	}

	var stripped map[int][]byte
	if len(kept) < len(entries) {
		if stripped, err = f.strippedTBS(base); err != nil {
			return err
		}
	}
	records, err := idx.encode(survivors, kept, cncx)
	if err != nil {
		return err
	}
	for k, rec := range records {
		f.records[idx.primary+k] = rec
	}
	if stripped != nil {
		for i, rec := range stripped {
			f.records[i] = rec
		}
		flags := binary.BigEndian.Uint32(f.records[base][offExtraFlags:])
		binary.BigEndian.PutUint32(f.records[base][offExtraFlags:], flags&^extraFlagTBS)
	}
	return nil
}

// withValues returns a copy of e with the given tag values replaced; a
// nil value removes the tag.
func withValues(e *ncxEntry, values map[byte][]uint32) *ncxEntry {
	out := &ncxEntry{ident: e.ident, values: make(map[byte][]uint32, len(e.values))}
	for tag, v := range e.values {
		out.values[tag] = v
	}
	for tag, v := range values {
		if v == nil {
			delete(out.values, tag)
		} else {
			out.values[tag] = v
		}
	}
	return out
}

// renumber returns e with its parent and child range naming the entries'
// new indexes, given which entries are gone and how many survivors come
// before each old index. A child range keeps the surviving entries it held
// and goes when none is left.
func renumber(e *ncxEntry, gone []bool, before []int) *ncxEntry {
	n := int64(len(gone))
	changes := map[byte][]uint32{}
	if p, ok := e.value(ncxTagParent); ok {
		switch {
		case int64(p) >= n || gone[p]:
			changes[ncxTagParent] = nil
		case before[p] != int(p):
			changes[ncxTagParent] = []uint32{uint32(before[p])} //nolint:gosec // an entry index
		}
	}
	first, hasFirst := e.value(ncxTagFirstChild)
	last, hasLast := e.value(ncxTagLastChild)
	if hasFirst || hasLast {
		if !hasFirst {
			first = last
		}
		if !hasLast {
			last = first
		}
		lo, hi := int64(first), min(int64(last), n-1)
		switch {
		case lo > hi || before[hi+1] == before[lo]:
			changes[ncxTagFirstChild], changes[ncxTagLastChild] = nil, nil
		case before[lo] != int(first) || before[hi+1]-1 != int(last):
			if hasFirst {
				changes[ncxTagFirstChild] = []uint32{uint32(before[lo])} //nolint:gosec // an entry index
			}
			if hasLast {
				changes[ncxTagLastChild] = []uint32{uint32(before[hi+1] - 1)} //nolint:gosec // an entry index
			}
		}
	}
	if len(changes) == 0 {
		return e
	}
	return withValues(e, changes)
}

// identScheme returns how the index numbers its entries' identifiers, so
// renumbered entries can follow it: Calibre's reader rejects an index whose
// last identifier is not the entry count less one. Calibre writes
// hexadecimal, lowercase in KF8 and uppercase with an even number of digits in
// MOBI6, padded to two digits. It returns nil when the identifiers are not
// entry numbers; they are then kept as written, which leaves them in order
// with gaps where entries were deleted.
func identScheme(entries []*ncxEntry) func(n int) []byte {
	if len(entries) == 0 {
		return nil
	}
	width := len(entries[0].ident)
	even, upper := true, true
	for i, e := range entries {
		s := string(e.ident)
		n, err := strconv.ParseUint(s, 16, 32)
		if err != nil || n != uint64(i) || len(s) < width { //nolint:gosec // i is an entry index
			return nil
		}
		even = even && len(s)%2 == 0
		if strings.ToLower(s) != s {
			upper = true
		} else if strings.ToUpper(s) != s {
			upper = false
		}
	}
	return func(n int) []byte {
		s := strconv.FormatUint(uint64(n), 16) //nolint:gosec // an entry index
		if upper {
			s = strings.ToUpper(s)
		}
		for len(s) < width || (even && len(s)%2 != 0) {
			s = "0" + s
		}
		return []byte(s)
	}
}

// appendLabel adds a label to the last CNCX record, after its last string,
// and returns its CNCX offset. Old labels stay, since entries may share them.
// It fails when there is no CNCX record, or no room in the last one, or when
// something other than zero padding follows its last string or a label points
// there, which appending would overwrite.
func (idx *ncxIndex) appendLabel(cncx [][]byte, label []byte) (uint32, error) {
	if len(cncx) == 0 {
		return 0, errors.New("NCX index has no CNCX record for a new label")
	}
	last := len(cncx) - 1
	end := cncxEnd(cncx[last])
	if slices.ContainsFunc(cncx[last][end:], func(b byte) bool { return b != 0 }) {
		return 0, errors.New("NCX CNCX record has data after its last label")
	}
	for _, e := range idx.entries {
		if offset, ok := e.value(ncxTagLabel); ok && int(offset/maxIndexRecord) == last && int(offset%maxIndexRecord) >= end {
			return 0, errors.New("NCX label points past the last CNCX label")
		}
	}
	rec := appendForwardInt(slices.Clone(cncx[last][:end]), uint32(len(label))) //nolint:gosec // a label is short
	rec = append(rec, label...)
	rec = append(rec, make([]byte, (4-len(rec)%4)%4)...)
	if len(rec) > maxIndexRecord {
		return 0, errors.New("NCX CNCX record has no room for a new label")
	}
	cncx[last] = rec
	return uint32(last*maxIndexRecord + end), nil //nolint:gosec // under 16 bits of records
}

// extraFlagTBS marks text records that end with a trailing byte sequence.
const extraFlagTBS = 0b10

// strippedTBS returns the text records of the book at base without their
// trailing byte sequences, or nil when they have none. The sequence is the
// outermost trailing entry (readers take entries off the end from flag bit 1
// up), ending in its own size as a backward-encoded number.
func (f *pdbFile) strippedTBS(base int) (map[int][]byte, error) {
	rec := f.records[base]
	flags := headerField(rec, offExtraFlags)
	if flags == nullIndex || flags&extraFlagTBS == 0 {
		return nil, nil
	}
	count := int(binary.BigEndian.Uint16(rec[offTextRecords:]))
	if base+count >= len(f.records) {
		return nil, errors.New("MOBI text records run past the last record")
	}
	out := make(map[int][]byte, count)
	for i := base + 1; i <= base+count; i++ {
		data, err := f.load(i)
		if err != nil {
			return nil, err
		}
		size := trailingEntrySize(data)
		if size == 0 || size > len(data) {
			return nil, errors.Errorf("text record %d has an invalid trailing byte sequence", i)
		}
		out[i] = data[:len(data)-size]
	}
	return out, nil
}

// trailingEntrySize reads the size a trailing entry ends with: 7-bit
// groups read backward from the last byte, until a byte with the high bit
// set.
func trailingEntrySize(data []byte) int {
	n, shift := 0, 0
	for i := len(data) - 1; i >= 0 && shift < 28; i-- {
		b := data[i]
		n |= int(b&0x7F) << shift
		shift += 7
		if b&0x80 != 0 {
			return n
		}
	}
	return 0
}
