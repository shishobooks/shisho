package mobi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readTOCBytes(t *testing.T, data []byte) []TOC {
	t.Helper()
	tocs, err := ReadTOC(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	return tocs
}

// tocTitles returns entries as nested titles, like chapterTitles.
func tocTitles(entries []*TOCEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		s := e.Title
		if len(e.Children) > 0 {
			s += "[" + strings.Join(tocTitles(e.Children), ", ") + "]"
		}
		out = append(out, s)
	}
	return out
}

// indexOf returns the Index of the entry titled title.
func indexOf(t *testing.T, entries []*TOCEntry, title string) int {
	t.Helper()
	var find func([]*TOCEntry) int
	find = func(entries []*TOCEntry) int {
		for _, e := range entries {
			if e.Title == title {
				return e.Index
			}
			if i := find(e.Children); i >= 0 {
				return i
			}
		}
		return -1
	}
	i := find(entries)
	require.GreaterOrEqual(t, i, 0, "no entry titled %q", title)
	return i
}

// editByTitle renames and deletes entries of each TOC by title.
func editByTitle(t *testing.T, tocs []TOC, renamed map[string]string, deleted ...string) map[Book]*TOCEdit {
	t.Helper()
	edits := map[Book]*TOCEdit{}
	for _, toc := range tocs {
		edit := &TOCEdit{Renamed: map[int]string{}, Deleted: map[int]bool{}}
		for from, to := range renamed {
			edit.Renamed[indexOf(t, toc.Entries, from)] = to
		}
		for _, title := range deleted {
			edit.Deleted[indexOf(t, toc.Entries, title)] = true
		}
		edits[toc.Book] = edit
	}
	return edits
}

// checkChildRanges asserts that the entries are numbered in order and that
// every entry's child range names exactly the entries whose parent it is.
func checkChildRanges(t *testing.T, data []byte, base int) {
	t.Helper()
	db := openPDB(t, data)
	h := headerAt(t, db, base)
	idx, err := readNCX(db, base+int(h.ncx))
	require.NoError(t, err)
	require.Equal(t, len(idx.entries), int(binary.BigEndian.Uint32(idx.primaryRec[indxTotal:])))
	for i, e := range idx.entries {
		assert.Equal(t, fmt.Sprintf("%02X", i), string(e.ident), "entry %d identifier", i)
		var children []int
		for j, c := range idx.entries {
			if p, ok := c.value(ncxTagParent); ok && int(p) == i {
				children = append(children, j)
			}
		}
		first, hasFirst := e.value(ncxTagFirstChild)
		last, hasLast := e.value(ncxTagLastChild)
		if len(children) == 0 {
			assert.False(t, hasFirst || hasLast, "entry %d has a child range but no children", i)
			continue
		}
		assert.Equal(t, children[0], int(first), "entry %d first child", i)
		assert.Equal(t, children[len(children)-1], int(last), "entry %d last child", i)
	}
}

func TestRewrite_TOCRoundTrip(t *testing.T) {
	t.Parallel()

	want := []string{"Part One[First Chapter]", "Part Two[Chapter 3]", "The End"}
	for name, kind := range map[string]testgen.MOBIKind{
		"MOBI6": testgen.MOBIKindMOBI6,
		"KF8":   testgen.MOBIKindKF8,
		"combo": testgen.MOBIKindCombo,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opts := calibreOptions(kind)
			opts.TOC = nestedTOC
			src := testgen.BuildMOBI(t, opts)
			tocs := readTOCBytes(t, src)
			if kind == testgen.MOBIKindCombo {
				require.Len(t, tocs, 2)
				assert.Equal(t, BookKF8, tocs[0].Book)
				assert.Equal(t, BookMOBI6, tocs[1].Book)
			} else {
				require.Len(t, tocs, 1)
			}

			// Deleting Chapter 2 takes Scene 2.1 with it.
			edits := editByTitle(t, tocs, map[string]string{"Chapter 1": "First Chapter", "Epilogue": "The End"}, "Chapter 2")
			var out bytes.Buffer
			require.NoError(t, Rewrite(bytes.NewReader(src), int64(len(src)), &out, &Metadata{TOC: edits}))

			after := readTOCBytes(t, out.Bytes())
			require.Len(t, after, len(tocs))
			for _, toc := range after {
				assert.Equal(t, want, tocTitles(toc.Entries), "book %d", toc.Book)
			}
			assert.Equal(t, want, chapterTitles(parseBytes(t, out.Bytes()).Chapters))

			checkChildRanges(t, out.Bytes(), 0)
			if kind == testgen.MOBIKindCombo {
				kf8, _ := headerAt(t, openPDB(t, out.Bytes()), 0).number(exthKF8Boundary)
				checkChildRanges(t, out.Bytes(), int(kf8))
			}
		})
	}
}

func TestRewrite_TOCKeepsLocators(t *testing.T) {
	t.Parallel()

	opts := calibreOptions(testgen.MOBIKindKF8)
	opts.TOC = nestedTOC
	src := testgen.BuildMOBI(t, opts)
	before := readTOCBytes(t, src)[0].Entries

	edits := editByTitle(t, readTOCBytes(t, src), nil, "Part One")
	_, out := rewriteBuilt(t, opts, &Metadata{TOC: edits})
	after := readTOCBytes(t, out)[0].Entries
	require.Len(t, after, 2)
	assert.Equal(t, before[1].Locator, after[0].Locator)
	assert.Equal(t, before[2].Locator, after[1].Locator)
}

func TestRewrite_TOCComboWithFlatMOBI6Half(t *testing.T) {
	t.Parallel()

	// Calibre flattens the MOBI6 half of a combo file, so a nested KF8
	// entry and its children are separate entries there.
	opts := calibreOptions(testgen.MOBIKindCombo)
	opts.TOC = nestedTOC
	opts.FlatMOBI6TOC = true
	src := testgen.BuildMOBI(t, opts)
	tocs := readTOCBytes(t, src)
	edits := map[Book]*TOCEdit{
		BookKF8: {Deleted: map[int]bool{indexOf(t, tocs[0].Entries, "Part Two"): true}},
		BookMOBI6: {Deleted: map[int]bool{
			indexOf(t, tocs[1].Entries, "Part Two"):  true,
			indexOf(t, tocs[1].Entries, "Chapter 3"): true,
		}},
	}
	_, out := rewriteBuilt(t, opts, &Metadata{TOC: edits})
	after := readTOCBytes(t, out)
	assert.Equal(t, []string{"Part One[Chapter 1, Chapter 2[Scene 2.1]]", "Epilogue"}, tocTitles(after[0].Entries))
	assert.Equal(t, []string{"Part One", "Chapter 1", "Chapter 2", "Scene 2.1", "Epilogue"}, tocTitles(after[1].Entries))
}

// textTrailer returns the extra data flags of the book at base and the end
// of its first text record.
func textTrailer(t *testing.T, data []byte, base int) (uint16, []byte) {
	t.Helper()
	db := openPDB(t, data)
	rec, err := db.record(base, maxHeaderRecord)
	require.NoError(t, err)
	text, err := db.record(base+1, maxHeaderRecord)
	require.NoError(t, err)
	return uint16(binary.BigEndian.Uint32(rec[offExtraFlags:])), text[len(text)-5:] //nolint:gosec // the flags are the low 16 bits
}

func TestRewrite_TOCDeletionStripsTBS(t *testing.T) {
	t.Parallel()

	opts := calibreOptions(testgen.MOBIKindCombo)
	opts.TOC = nestedTOC
	src := testgen.BuildMOBI(t, opts)
	kf8, _ := headerAt(t, openPDB(t, src), 0).number(exthKF8Boundary)
	for _, base := range []int{0, int(kf8)} {
		flags, tail := textTrailer(t, src, base)
		assert.Equal(t, uint16(0b11), flags)
		assert.Equal(t, []byte{0x00, 0x86, 0x80, 0x07, 0x84}, tail)
	}

	// A rename keeps every entry's number, so the TBS still holds.
	_, renamed := rewriteBuilt(t, opts, &Metadata{TOC: editByTitle(t, readTOCBytes(t, src), map[string]string{"Epilogue": "End"})})
	for _, base := range []int{0, int(kf8)} {
		flags, tail := textTrailer(t, renamed, base)
		assert.Equal(t, uint16(0b11), flags)
		assert.Equal(t, []byte{0x00, 0x86, 0x80, 0x07, 0x84}, tail)
	}

	// A deletion renumbers entries; the TBS goes and the multibyte byte
	// stays.
	_, deleted := rewriteBuilt(t, opts, &Metadata{TOC: editByTitle(t, readTOCBytes(t, src), nil, "Epilogue")})
	for _, base := range []int{0, int(kf8)} {
		flags, tail := textTrailer(t, deleted, base)
		assert.Equal(t, uint16(0b01), flags)
		assert.Equal(t, byte(0x00), tail[len(tail)-1])
		assert.NotEqual(t, []byte{0x86, 0x80, 0x07, 0x84}, tail[1:])
	}
}

func TestRewrite_TOCDeletingEverythingLeavesIndex(t *testing.T) {
	t.Parallel()

	opts := calibreOptions(testgen.MOBIKindKF8)
	opts.TOC = nestedTOC
	src := testgen.BuildMOBI(t, opts)
	edits := editByTitle(t, readTOCBytes(t, src), nil, "Part One", "Part Two", "Epilogue")
	_, out := rewriteBuilt(t, opts, &Metadata{TOC: edits})
	assert.Equal(t, nestedTitles, tocTitles(readTOCBytes(t, out)[0].Entries))
}

func TestRewrite_TOCWithCoverInserted(t *testing.T) {
	t.Parallel()

	// Inserting a cover record shifts the records after it, the KF8 half
	// of a combo file included; the edit must land in the shifted index.
	opts := calibreOptions(testgen.MOBIKindCombo)
	opts.HasCover = false
	opts.TOC = nestedTOC
	src := testgen.BuildMOBI(t, opts)
	edits := editByTitle(t, readTOCBytes(t, src), map[string]string{"Part Two": "Second Part"}, "Epilogue")
	_, out := rewriteBuilt(t, opts, &Metadata{Cover: testImage(t, 20, 30, "image/jpeg"), TOC: edits})
	meta := parseBytes(t, out)
	assert.NotEmpty(t, meta.CoverData)
	for _, toc := range readTOCBytes(t, out) {
		assert.Equal(t, []string{"Part One[Chapter 1, Chapter 2[Scene 2.1]]", "Second Part[Chapter 3]"}, tocTitles(toc.Entries))
	}
}

func TestRewrite_TOCCP1252(t *testing.T) {
	t.Parallel()

	opts := calibreOptions(testgen.MOBIKindMOBI6)
	opts.CP1252 = true
	opts.TOC = []testgen.MOBITOCEntry{{Title: "One"}}
	src := testgen.BuildMOBI(t, opts)
	_, out := rewriteBuilt(t, opts, &Metadata{TOC: editByTitle(t, readTOCBytes(t, src), map[string]string{"One": "Café"})})
	assert.Equal(t, []string{"Café"}, tocTitles(readTOCBytes(t, out)[0].Entries))
}

func TestRewrite_NoTOCEditKeepsIndexRecords(t *testing.T) {
	t.Parallel()

	opts := calibreOptions(testgen.MOBIKindCombo)
	opts.TOC = nestedTOC
	src, out := rewriteBuilt(t, opts, &Metadata{Title: "Other"})
	srcDB, outDB := openPDB(t, src), openPDB(t, out)
	require.Equal(t, srcDB.count(), outDB.count())
	kf8, _ := headerAt(t, srcDB, 0).number(exthKF8Boundary)
	for i := 1; i < srcDB.count(); i++ {
		if i == int(kf8) {
			continue
		}
		a, err := srcDB.record(i, maxImageRecord)
		require.NoError(t, err)
		b, err := outDB.record(i, maxImageRecord)
		require.NoError(t, err)
		assert.Equal(t, a, b, "record %d", i)
	}
}

func TestNCXEntryEncoding(t *testing.T) {
	t.Parallel()

	// A two-bit mask holds counts 1 and 2; a full mask means a byte length
	// of the values follows the control bytes.
	idx := &ncxIndex{controlBytes: 1, tags: []tagxEntry{
		{tag: 1, valuesPerEntry: 1, mask: 0x03},
		{tag: 2, valuesPerEntry: 1, mask: 0x04},
		{end: true},
	}}
	for _, values := range []map[byte][]uint32{
		{1: {5}},
		{1: {5, 300}, 2: {7}},
		{1: {1, 2, 3, 200000}},
		{2: {0}},
	} {
		raw, err := idx.encodeEntry(&ncxEntry{ident: []byte("01"), values: values})
		require.NoError(t, err)
		e, err := idx.decodeEntry(raw)
		require.NoError(t, err)
		assert.Equal(t, values, e.values)
		assert.Equal(t, []byte("01"), e.ident)
	}

	_, err := idx.encodeEntry(&ncxEntry{values: map[byte][]uint32{2: {1, 2}}})
	assert.Error(t, err, "a one-bit mask holds a single value")
}

func TestIdentScheme(t *testing.T) {
	t.Parallel()

	entries := func(idents ...string) []*ncxEntry {
		out := make([]*ncxEntry, len(idents))
		for i, id := range idents {
			out[i] = &ncxEntry{ident: []byte(id)}
		}
		return out
	}
	idents := func(scheme func(int) []byte, ns ...int) []string {
		out := make([]string, len(ns))
		for i, n := range ns {
			out[i] = string(scheme(n))
		}
		return out
	}

	// Calibre's KF8 identifiers are lowercase; its MOBI6 ones uppercase
	// with an even number of digits.
	kf8 := identScheme(entries("00", "01", "02", "03", "04", "05", "06", "07", "08", "09", "0a", "0b"))
	require.NotNil(t, kf8)
	assert.Equal(t, []string{"03", "0a"}, idents(kf8, 3, 10))
	mobi6 := identScheme(entries("00", "01", "02", "03", "04", "05", "06", "07", "08", "09", "0A"))
	require.NotNil(t, mobi6)
	assert.Equal(t, []string{"0A", "0100"}, idents(mobi6, 10, 256))

	assert.Nil(t, identScheme(entries("a", "b")), "identifiers that are not entry numbers stay")
	assert.Nil(t, identScheme(entries("00", "02")))
}

func TestRewrite_TOCAcrossSeveralRecords(t *testing.T) {
	t.Parallel()

	// Two entries per data record and three labels per CNCX record. In
	// level order the index is Part One, Part Two | Epilogue, Chapter 1 |
	// Chapter 2, Chapter 3 | Scene 2.1, so deleting Epilogue and Chapter 1
	// empties the second data record, and the rename goes into the last CNCX
	// record.
	for name, kind := range map[string]testgen.MOBIKind{"KF8": testgen.MOBIKindKF8, "combo": testgen.MOBIKindCombo} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opts := calibreOptions(kind)
			opts.TOC = nestedTOC
			opts.NCXEntriesPerRecord = 2
			opts.NCXLabelsPerCNCX = 3
			src := testgen.BuildMOBI(t, opts)
			assert.Equal(t, nestedTitles, chapterTitles(parseBytes(t, src).Chapters))

			edits := editByTitle(t, readTOCBytes(t, src), map[string]string{"Scene 2.1": "The Scene"}, "Epilogue", "Chapter 1")
			_, out := rewriteBuilt(t, opts, &Metadata{TOC: edits})
			want := []string{"Part One[Chapter 2[The Scene]]", "Part Two[Chapter 3]"}
			for _, toc := range readTOCBytes(t, out) {
				assert.Equal(t, want, tocTitles(toc.Entries), "book %d", toc.Book)
			}
			checkChildRanges(t, out, 0)
		})
	}
}

func TestParse_TOCDepthIsCapped(t *testing.T) {
	t.Parallel()

	// A chain nested past maxTOCDepth must not recurse without bound.
	deep := []testgen.MOBITOCEntry{{Title: "Leaf"}}
	for i := range maxTOCDepth + 10 {
		deep = []testgen.MOBITOCEntry{{Title: fmt.Sprintf("Level %d", i), Children: deep}}
	}
	opts := calibreOptions(testgen.MOBIKindKF8)
	opts.TOC = deep
	meta := parseBytes(t, testgen.BuildMOBI(t, opts))

	depth := 0
	for level := meta.Chapters; len(level) > 0; level = level[0].Children {
		depth++
	}
	assert.Equal(t, maxTOCDepth, depth)
}

// swapTAGXRows moves the TAGX end row of the book's NCX index ahead of the
// tag rows, so a tag row follows the last end row.
func swapTAGXRows(t *testing.T, data []byte) []byte {
	t.Helper()
	db := openPDB(t, data)
	tagx := db.offsets[int(headerAt(t, db, 0).ncx)] + indxHeaderLen
	require.Equal(t, []byte("TAGX"), data[tagx:tagx+4])
	first := tagx + 12
	last := tagx + int64(binary.BigEndian.Uint32(data[tagx+4:])) - 4
	require.Equal(t, byte(1), data[last+3], "the last row is the end row")
	out := slices.Clone(data)
	copy(out[first:], data[last:last+4])
	copy(out[last:], data[first:first+4])
	return out
}

// indxHeaderLen is the INDX header length mobigen writes, where its TAGX
// table starts.
const indxHeaderLen = 192

func TestReadTOC_RejectsTAGXEndingInATag(t *testing.T) {
	t.Parallel()

	opts := calibreOptions(testgen.MOBIKindKF8)
	opts.TOC = nestedTOC
	src := swapTAGXRows(t, testgen.BuildMOBI(t, opts))
	assert.Empty(t, readTOCBytes(t, src))
	assert.Empty(t, parseBytes(t, src).Chapters)

	// Rewrite leaves such an index alone, says why, and writes the rest.
	var skipped []*TOCSkipError
	var out bytes.Buffer
	err := Rewrite(bytes.NewReader(src), int64(len(src)), &out, &Metadata{
		Title:        "Still Written",
		TOC:          map[Book]*TOCEdit{BookKF8: {Renamed: map[int]string{0: "X"}}},
		OnTOCSkipped: func(e *TOCSkipError) { skipped = append(skipped, e) },
	})
	require.NoError(t, err)
	require.Len(t, skipped, 1)
	assert.Equal(t, BookKF8, skipped[0].Book)
	assert.Equal(t, "Still Written", parseBytes(t, out.Bytes()).Title)
}

func TestAppendLabel(t *testing.T) {
	t.Parallel()

	label := func(offset uint32) *ncxEntry {
		return &ncxEntry{values: map[byte][]uint32{ncxTagLabel: {offset}}}
	}

	// Zero padding after the last string is replaced.
	idx := &ncxIndex{entries: []*ncxEntry{label(0)}}
	cncx := [][]byte{{0x83, 'a', 'b', 'c', 0, 0, 0, 0}}
	offset, err := idx.appendLabel(cncx, []byte("de"))
	require.NoError(t, err)
	assert.Equal(t, uint32(4), offset)
	assert.Equal(t, []byte{0x83, 'a', 'b', 'c', 0x82, 'd', 'e', 0}, cncx[0])

	// Bytes after the last string that are not padding stay put.
	_, err = idx.appendLabel([][]byte{{0x83, 'a', 'b', 'c', 0, 1}}, []byte("x"))
	require.Error(t, err)

	// So does a label something points at past the last string.
	idx = &ncxIndex{entries: []*ncxEntry{label(0), label(6)}}
	_, err = idx.appendLabel([][]byte{{0x83, 'a', 'b', 'c', 0, 0, 0, 0}}, []byte("x"))
	require.Error(t, err)

	_, err = (&ncxIndex{}).appendLabel(nil, []byte("x"))
	require.Error(t, err, "an index with no CNCX record has nowhere for a label")
}

func FuzzRewriteTOC(f *testing.F) {
	for _, kind := range []testgen.MOBIKind{testgen.MOBIKindMOBI6, testgen.MOBIKindKF8, testgen.MOBIKindCombo} {
		opts := calibreOptions(kind)
		opts.TOC = nestedTOC
		opts.NCXEntriesPerRecord = 3
		f.Add(testgen.BuildMOBI(&testing.T{}, opts))
	}
	f.Fuzz(func(_ *testing.T, data []byte) {
		tocs, err := ReadTOC(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return
		}
		edits := map[Book]*TOCEdit{}
		for _, toc := range tocs {
			edits[toc.Book] = &TOCEdit{Renamed: map[int]string{0: "Renamed"}, Deleted: map[int]bool{1: true}}
		}
		_ = Rewrite(bytes.NewReader(data), int64(len(data)), io.Discard, &Metadata{TOC: edits})
	})
}
