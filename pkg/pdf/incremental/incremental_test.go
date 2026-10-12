package incremental

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_Objects(t *testing.T) {
	t.Parallel()
	p := &parser{final: true, buf: []byte(`<< /Name#20Space (a\(b\)\n\101\
c) /Hex <48 6 9> /Arr [1 -2.5 3 0 R true null /N] /Nested << /K 7 >> /Gone null >>`)}
	o, err := p.object()
	require.NoError(t, err)
	assert.Equal(t, Dict{
		"Name Space": String("a(b)\nAc"),
		"Hex":        String("Hi"),
		"Arr":        Array{Integer(1), Real("-2.5"), Ref{Num: 3}, Bool(true), Null{}, Name("N")},
		"Nested":     Dict{"K": Integer(7)},
	}, o)
}

func TestParser_ShortBufferAsksForMore(t *testing.T) {
	t.Parallel()
	// An integer at the end of a window might be the start of a reference.
	p := &parser{buf: []byte("[1 0")}
	_, err := p.object()
	require.ErrorIs(t, err, errShort)

	p = &parser{buf: []byte("[1 0"), final: true}
	_, err = p.object()
	require.Error(t, err)
	assert.NotErrorIs(t, err, errShort)
}

func TestWriteObject_RoundTrips(t *testing.T) {
	t.Parallel()
	in := Dict{
		"S":   String("needs (escaping) \\ \r\n\x01"),
		"N":   Name("with space/slash#"),
		"A":   Array{Integer(-4), Real("0.5"), Ref{Num: 9, Gen: 2}, Bool(false), Null{}},
		"Sub": Dict{"T": TextString("Café ✓")},
	}
	var b bytes.Buffer
	require.NoError(t, writeObject(&b, in))
	out, err := (&parser{buf: b.Bytes(), final: true}).object()
	require.NoError(t, err)
	assert.Equal(t, in, out)
	assert.Equal(t, "Café ✓", out.(Dict)["Sub"].(Dict)["T"].(String).Text())
}

func TestTextString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, String("plain"), TextString("plain"))
	assert.Equal(t, []byte{0xfe, 0xff, 0, 'C', 0, 0xe9}, []byte(TextString("Cé")))
	assert.Equal(t, "Cé", TextString("Cé").Text())
	assert.Equal(t, "Cé", String("C\xe9").Text(), "PDFDocEncoding reads as Latin-1")
}

// pdfBuilder writes objects and records their offsets.
type pdfBuilder struct {
	b       bytes.Buffer
	offsets map[int]int
}

func newPDF() *pdfBuilder {
	p := &pdfBuilder{offsets: map[int]int{}}
	p.b.WriteString("%PDF-1.5\n")
	return p
}

func (p *pdfBuilder) obj(num int, body string) {
	p.offsets[num] = p.b.Len()
	fmt.Fprintf(&p.b, "%d 0 obj\n%s\nendobj\n", num, body)
}

func (p *pdfBuilder) stream(num int, dict string, data []byte) {
	p.offsets[num] = p.b.Len()
	fmt.Fprintf(&p.b, "%d 0 obj\n<< %s /Length %d >>\nstream\n", num, dict, len(data))
	p.b.Write(data)
	p.b.WriteString("\nendstream\nendobj\n")
}

func deflate(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, err := w.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return b.Bytes()
}

// A hybrid file lists objects in object streams only in the stream named
// by its trailer's /XRefStm, and marks them free in its table.
func TestOpen_HybridFile(t *testing.T) {
	t.Parallel()
	p := newPDF()
	p.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	header := "2 0 3 50 "
	objs := "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"
	objs += strings.Repeat(" ", 50-len(objs)) + "<< /Type /Page /Parent 2 0 R >>"
	p.stream(4, fmt.Sprintf("/Type /ObjStm /N 2 /First %d /Filter /FlateDecode", len(header)), deflate(t, []byte(header+objs)))

	// The cross-reference stream: W [1 2 1], entries for objects 2 and 3.
	rows := []byte{2, 0, 4, 0, 2, 0, 4, 1}
	p.stream(5, "/Type /XRef /Size 6 /Index [2 2] /W [1 2 1]", rows)

	xref := p.b.Len()
	p.b.WriteString("xref\n0 6\n0000000000 65535 f\r\n")
	fmt.Fprintf(&p.b, "%010d 00000 n\r\n", p.offsets[1])
	p.b.WriteString("0000000000 00000 f\r\n0000000000 00000 f\r\n")
	fmt.Fprintf(&p.b, "%010d 00000 n\r\n", p.offsets[4])
	fmt.Fprintf(&p.b, "%010d 00000 n\r\n", p.offsets[5])
	fmt.Fprintf(&p.b, "trailer\n<< /Size 6 /Root 1 0 R /XRefStm %d >>\nstartxref\n%d\n%%%%EOF", p.offsets[5], xref)

	doc, err := Open(bytes.NewReader(p.b.Bytes()), int64(p.b.Len()))
	require.NoError(t, err)
	pages, err := doc.Pages()
	require.NoError(t, err)
	assert.Equal(t, []Ref{{Num: 3}}, pages)
	assert.False(t, doc.xrefStream, "the last section is a table")
	assert.False(t, doc.endsWithEOL)

	// The update starts on a new line and chains back with a table.
	u := doc.NewUpdate()
	info := u.Add(Dict{"Title": String("T")})
	u.SetTrailer("Info", info)
	appended, err := u.Bytes()
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(appended, []byte("\n6 0 obj\n")), "%q", appended)
	assert.Contains(t, string(appended), fmt.Sprintf("/Prev %d", xref))
	assert.NotContains(t, string(appended), "XRefStm")

	full := append(append([]byte{}, p.b.Bytes()...), appended...)
	doc, err = Open(bytes.NewReader(full), int64(len(full)))
	require.NoError(t, err)
	o, err := doc.Resolve(doc.Trailer()["Info"])
	require.NoError(t, err)
	assert.Equal(t, Dict{"Title": String("T")}, o)
	pages, err = doc.Pages()
	require.NoError(t, err)
	assert.Equal(t, []Ref{{Num: 3}}, pages)
}

// A cross-reference stream with a PNG predictor, as most writers produce.
func TestOpen_XRefStreamWithPredictor(t *testing.T) {
	t.Parallel()
	p := newPDF()
	p.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	p.obj(2, "<< /Type /Pages /Kids [] /Count 0 >>")
	xref := p.b.Len()
	// Rows of W [1 2 1], each PNG Up-filtered (type 2) against the last.
	rows := [][]byte{{0, 0, 0, 255}, {1, 0, byte(p.offsets[1]), 0}, {1, 0, byte(p.offsets[2]), 0}, {1, 0, byte(xref), 0}}
	require.Less(t, xref, 256, "offsets fit one byte in this test")
	var raw []byte
	prev := make([]byte, 4)
	for _, r := range rows {
		raw = append(raw, 2)
		for i := range r {
			raw = append(raw, r[i]-prev[i])
		}
		prev = r
	}
	p.stream(3, "/Type /XRef /Size 4 /Root 1 0 R /W [1 2 1] /Filter /FlateDecode /DecodeParms << /Predictor 12 /Columns 4 >>", deflate(t, raw))
	fmt.Fprintf(&p.b, "startxref\n%d\n%%%%EOF\n", xref)

	doc, err := Open(bytes.NewReader(p.b.Bytes()), int64(p.b.Len()))
	require.NoError(t, err)
	assert.True(t, doc.xrefStream)
	ref, catalog, err := doc.Catalog()
	require.NoError(t, err)
	assert.Equal(t, Ref{Num: 1}, ref)
	assert.Equal(t, Name("Catalog"), catalog["Type"])
	assert.Equal(t, 4, doc.nextNum)
}

func TestOpen_Failures(t *testing.T) {
	t.Parallel()
	classic := func(trailer string) []byte {
		p := newPDF()
		p.obj(1, "<< /Type /Catalog >>")
		xref := p.b.Len()
		fmt.Fprintf(&p.b, "xref\n0 2\n0000000000 65535 f\r\n%010d 00000 n\r\ntrailer\n%s\nstartxref\n%d\n%%%%EOF\n", p.offsets[1], trailer, xref)
		return p.b.Bytes()
	}
	open := func(data []byte) error {
		_, err := Open(bytes.NewReader(data), int64(len(data)))
		return err
	}

	require.NoError(t, open(classic("<< /Size 2 /Root 1 0 R >>")))
	require.ErrorIs(t, open(classic("<< /Size 2 /Root 1 0 R /Encrypt << /Filter /Standard >> >>")), ErrEncrypted)
	require.ErrorIs(t, open(classic("<< /Size 2 >>")), ErrUnreadable, "no /Root")
	require.ErrorIs(t, open(classic("<< /Size 2 /Root 1 0 R /Prev 9 >>")), ErrUnreadable, "a /Prev pointing at nothing")
	require.ErrorIs(t, open([]byte("%PDF-1.4\nno cross-reference data")), ErrUnreadable)
}

// A stream whose /Length refers to itself fails instead of recursing
// forever.
func TestObject_ReferenceCycle(t *testing.T) {
	t.Parallel()
	p := newPDF()
	p.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	p.offsets[2] = p.b.Len()
	p.b.WriteString("2 0 obj\n<< /Length 2 0 R >>\nstream\nabc\nendstream\nendobj\n")
	xref := p.b.Len()
	fmt.Fprintf(&p.b, "xref\n0 3\n0000000000 65535 f\r\n%010d 00000 n\r\n%010d 00000 n\r\ntrailer\n<< /Size 3 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", p.offsets[1], p.offsets[2], xref)

	doc, err := Open(bytes.NewReader(p.b.Bytes()), int64(p.b.Len()))
	require.NoError(t, err)
	_, err = doc.Pages()
	require.Error(t, err)
}

// xrefStreamPDF returns a PDF whose only cross-reference section is a
// stream with the given dictionary entries and (already encoded) data.
func xrefStreamPDF(t *testing.T, dict string, data []byte) []byte {
	t.Helper()
	p := newPDF()
	p.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	xref := p.b.Len()
	p.stream(3, "/Type /XRef /Root 1 0 R "+dict, data)
	fmt.Fprintf(&p.b, "startxref\n%d\n%%%%EOF\n", xref)
	return p.b.Bytes()
}

// A tiny cross-reference stream that inflates to millions of entries is
// refused instead of filling memory.
func TestOpen_RefusesTooManyEntries(t *testing.T) {
	t.Parallel()
	rows := maxEntries + 1
	data := xrefStreamPDF(t, fmt.Sprintf("/Size %d /W [1 0 0] /Filter /FlateDecode", rows), deflate(t, make([]byte, rows)))
	_, err := Open(bytes.NewReader(data), int64(len(data)))
	require.ErrorIs(t, err, ErrUnreadable)
}

// Absurd predictor parameters are refused, not multiplied into an
// overflowing row length.
func TestOpen_RefusesHugePredictorParameters(t *testing.T) {
	t.Parallel()
	data := xrefStreamPDF(t, "/Size 2 /W [1 1 1] /Filter /FlateDecode /DecodeParms << /Predictor 12 /Colors 2147483648 /BitsPerComponent 2147483648 /Columns 2 >>", deflate(t, make([]byte, 8)))
	_, err := Open(bytes.NewReader(data), int64(len(data)))
	require.ErrorIs(t, err, ErrUnreadable)
}

// An object stream whose header gives a negative offset fails cleanly.
func TestObject_NegativeObjectStreamOffset(t *testing.T) {
	t.Parallel()
	p := newPDF()
	p.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	p.stream(4, "/Type /ObjStm /N 1 /First 6", []byte("2 -50 << >>"))
	xref := p.b.Len()
	rows := []byte{1, 0, byte(p.offsets[1]), 0, 2, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, byte(p.offsets[4]), 0}
	require.Less(t, p.offsets[4], 256)
	p.stream(5, "/Type /XRef /Root 1 0 R /Size 5 /Index [1 4] /W [1 2 1]", rows)
	fmt.Fprintf(&p.b, "startxref\n%d\n%%%%EOF\n", xref)

	doc, err := Open(bytes.NewReader(p.b.Bytes()), int64(p.b.Len()))
	require.NoError(t, err)
	_, err = doc.Object(Ref{Num: 2})
	require.Error(t, err)
}

// "stream" followed by CRLF still finds the data when the CR is the last
// byte of the first read window.
func TestIndirectAt_StreamCRLFAcrossWindow(t *testing.T) {
	t.Parallel()
	rows := []byte{0, 0, 0, 255, 1, 0, 9, 0, 1, 0, 0, 0}
	p := newPDF()
	p.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	xref := p.b.Len()
	head := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Root 1 0 R /Size 3 /W [1 2 1] /Length %d ", len(rows))
	pad := firstWindow - len(head) - len(">>\nstream\r")
	require.Positive(t, pad)
	p.b.WriteString(head + strings.Repeat(" ", pad) + ">>\nstream\r\n")
	require.Equal(t, byte('\r'), p.b.Bytes()[xref+firstWindow-1], "the CR is the window's last byte")
	rows[6] = byte(p.offsets[1])
	rows[10] = byte(xref >> 8)
	rows[11] = byte(xref)
	p.b.Write(rows)
	fmt.Fprintf(&p.b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", xref)

	doc, err := Open(bytes.NewReader(p.b.Bytes()), int64(p.b.Len()))
	require.NoError(t, err)
	_, catalog, err := doc.Catalog()
	require.NoError(t, err)
	assert.Equal(t, Name("Catalog"), catalog["Type"])
}

func TestAtLeastVersion(t *testing.T) {
	t.Parallel()
	doc := func(header, catalog string) *Document {
		p := &pdfBuilder{offsets: map[int]int{}}
		p.b.WriteString(header + "\n")
		p.obj(1, catalog)
		xref := p.b.Len()
		fmt.Fprintf(&p.b, "xref\n0 2\n0000000000 65535 f\r\n%010d 00000 n\r\ntrailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", p.offsets[1], xref)
		d, err := Open(bytes.NewReader(p.b.Bytes()), int64(p.b.Len()))
		require.NoError(t, err)
		return d
	}
	assert.False(t, doc("%PDF-1.3", "<< /Type /Catalog >>").AtLeastVersion("1.4"))
	assert.True(t, doc("%PDF-1.4", "<< /Type /Catalog >>").AtLeastVersion("1.4"))
	assert.True(t, doc("%PDF-1.3", "<< /Type /Catalog /Version /1.7 >>").AtLeastVersion("1.4"), "the catalog's /Version overrides an older header")
	assert.True(t, doc("%PDF-2.0", "<< /Type /Catalog >>").AtLeastVersion("1.4"))
}

// Open and the reads a download makes never panic, whatever the bytes.
func FuzzOpen(f *testing.F) {
	p := newPDF()
	p.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	p.obj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	p.obj(3, "<< /Type /Page /Parent 2 0 R >>")
	xref := p.b.Len()
	fmt.Fprintf(&p.b, "xref\n0 4\n0000000000 65535 f\r\n%010d 00000 n\r\n%010d 00000 n\r\n%010d 00000 n\r\ntrailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", p.offsets[1], p.offsets[2], p.offsets[3], xref)
	f.Add(p.b.Bytes())
	f.Fuzz(func(_ *testing.T, data []byte) {
		doc, err := Open(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return
		}
		_, _, _ = doc.Catalog()
		_, _ = doc.Pages()
		_ = doc.AtLeastVersion("1.4")
		_, _ = doc.NewUpdate().Bytes()
	})
}
