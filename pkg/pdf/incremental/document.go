package incremental

import (
	"bytes"
	"compress/zlib"
	"io"

	"github.com/pkg/errors"
)

var (
	// ErrCannotDecrypt is returned by Open for an encrypted PDF that needs
	// a password to open, or that is encrypted other than by the standard
	// security handler.
	ErrCannotDecrypt = errors.New("PDF is encrypted in a way Shisho cannot open")
	// ErrUnreadable wraps every failure to read the cross-reference data
	// Open needs.
	ErrUnreadable = errors.New("PDF cross-reference data cannot be read")
)

const (
	// tailSize is how much of the end of the file is searched for
	// startxref. The specification puts it in the last 1024 bytes; some
	// writers leave junk after %%EOF.
	tailSize = 4096
	// maxSections bounds the /Prev chain.
	maxSections = 4096
	// maxDecoded bounds a decoded stream, so a hostile file cannot expand
	// into all of memory.
	maxDecoded = 256 << 20
	// maxEntries bounds the cross-reference entries read from all
	// sections. Real books have thousands of objects; a tiny stream can
	// otherwise inflate into hundreds of millions of entries.
	maxEntries = 4_000_000
	// maxWindow bounds how much is read to parse one object or
	// cross-reference table, so an unterminated object cannot pull the
	// whole file into memory.
	maxWindow = 64 << 20
	// maxObjStreamCache bounds the decoded object streams kept for reuse.
	maxObjStreamCache = 64 << 20
	// maxRefDepth bounds nested Object calls.
	maxRefDepth = 32
	// firstWindow is how much is read to parse an object; the read grows
	// when the object is larger.
	firstWindow = 1 << 10
)

type entryKind int

const (
	entryFree entryKind = iota
	entryInUse
	entryCompressed
)

// xrefEntry locates an object: at offset for entryInUse, or at position
// index in the object stream numbered stream for entryCompressed.
type xrefEntry struct {
	kind   entryKind
	offset int64
	gen    int
	stream int
	index  int
}

// Document is a PDF opened for an incremental update.
type Document struct {
	r    io.ReaderAt
	size int64

	entries map[int]xrefEntry
	trailer Dict
	// startxref is the offset of the last cross-reference section, which
	// the update's section chains back to.
	startxref int64
	// xrefStream is whether the last section is a cross-reference stream,
	// so the update writes the same kind.
	xrefStream bool
	// nextNum is the first unused object number.
	nextNum int
	// endsWithEOL is whether the file ends with an end-of-line marker.
	endsWithEOL bool

	objStreams map[int]*objStream
	// objStreamBytes is the decoded size of the cached object streams.
	objStreamBytes int
	// entryCount counts the cross-reference entries read, against
	// maxEntries.
	entryCount int
	// sec decrypts the objects of an encrypted PDF; nil when it is not
	// encrypted.
	sec *security
	// depth counts nested Object calls (a stream's /Length can be a
	// reference), so a reference cycle fails instead of recursing forever.
	depth int
}

type objStream struct {
	data    []byte
	offsets []int
	nums    []int
}

// Open reads the cross-reference sections and trailers of the PDF in r.
// An encrypted PDF that opens without a password is decrypted as it is
// read; one that needs a password returns an error wrapping
// ErrCannotDecrypt. It returns an error wrapping ErrUnreadable when the
// cross-reference data cannot be read.
func Open(r io.ReaderAt, size int64) (*Document, error) {
	d := &Document{
		r:          r,
		size:       size,
		entries:    map[int]xrefEntry{},
		trailer:    Dict{},
		objStreams: map[int]*objStream{},
	}
	if err := d.readXRef(); err != nil {
		return nil, errors.Wrap(ErrUnreadable, err.Error())
	}
	if enc, ok := d.trailer["Encrypt"]; ok {
		sec, err := d.openSecurity(enc)
		if err != nil {
			return nil, errors.Wrap(ErrCannotDecrypt, err.Error())
		}
		d.sec = sec
	}
	if _, ok := d.trailer["Root"].(Ref); !ok {
		return nil, errors.Wrap(ErrUnreadable, "trailer has no /Root")
	}
	return d, nil
}

// Trailer returns the merged trailer: each key from the newest section
// that has it.
func (d *Document) Trailer() Dict {
	return d.trailer
}

func (d *Document) readXRef() error {
	tailLen := min(int64(tailSize), d.size)
	tail := make([]byte, tailLen)
	if _, err := d.r.ReadAt(tail, d.size-tailLen); err != nil && !errors.Is(err, io.EOF) {
		return errors.WithStack(err)
	}
	if n := len(tail); n > 0 {
		d.endsWithEOL = tail[n-1] == '\n' || tail[n-1] == '\r'
	}
	i := bytes.LastIndex(tail, []byte("startxref"))
	if i < 0 {
		return errors.New("no startxref")
	}
	p := &parser{buf: tail[i+len("startxref"):], final: true}
	off, err := p.object()
	if err != nil {
		return errors.Wrap(err, "startxref offset")
	}
	start, ok := off.(Integer)
	if !ok || start < 0 || int64(start) >= d.size {
		return errors.Errorf("invalid startxref offset %v", off)
	}
	d.startxref = int64(start)

	maxNum := 0
	seen := map[int64]bool{}
	next := d.startxref
	for i := 0; ; i++ {
		if i >= maxSections {
			return errors.New("too many cross-reference sections")
		}
		if seen[next] {
			return errors.New("cross-reference sections loop")
		}
		seen[next] = true
		trailer, isStream, err := d.readSection(next, &maxNum)
		if err != nil {
			return err
		}
		if i == 0 {
			d.xrefStream = isStream
			if size, ok := trailer["Size"].(Integer); ok && int(size) > maxNum {
				maxNum = int(size) - 1
			}
		}
		for _, k := range []Name{"Root", "Info", "ID", "Encrypt"} {
			if _, have := d.trailer[k]; !have {
				if v, ok := trailer[k]; ok {
					d.trailer[k] = v
				}
			}
		}
		prev, ok := trailer["Prev"].(Integer)
		if !ok {
			break
		}
		next = int64(prev)
	}
	d.nextNum = maxNum + 1
	return nil
}

// readSection reads the cross-reference section at off, adding entries
// for objects no newer section has, and returns its trailer.
func (d *Document) readSection(off int64, maxNum *int) (Dict, bool, error) {
	if off < 0 || off >= d.size {
		return nil, false, errors.Errorf("cross-reference offset %d is outside the file", off)
	}
	var table []tableEntry
	var trailer Dict
	err := d.parseAt(off, func(p *parser) error {
		p.skipSpace()
		if !bytes.HasPrefix(p.buf[p.pos:], []byte("xref")) {
			table = nil
			return nil
		}
		var err error
		table, trailer, err = parseTable(p, d.countEntries)
		return err
	})
	if err != nil {
		return nil, false, errors.Wrapf(err, "cross-reference table at %d", off)
	}

	if trailer == nil {
		// Not a table, so it must be a cross-reference stream.
		entries, dict, err := d.readXRefStream(off)
		if err != nil {
			return nil, false, err
		}
		d.addEntries(entries, maxNum, nil)
		return dict, true, nil
	}

	// A hybrid file lists the objects in its object streams only in the
	// stream named by /XRefStm, and marks them free in the table, so the
	// stream's entries go in before the table's free ones.
	d.addEntries(table, maxNum, func(e tableEntry) bool { return e.kind != entryFree })
	if stm, ok := trailer["XRefStm"].(Integer); ok {
		entries, _, err := d.readXRefStream(int64(stm))
		if err != nil {
			return nil, false, err
		}
		d.addEntries(entries, maxNum, nil)
	}
	d.addEntries(table, maxNum, func(e tableEntry) bool { return e.kind == entryFree })
	return trailer, false, nil
}

type tableEntry struct {
	num int
	xrefEntry
}

// countEntries charges n entries against maxEntries before they are read.
func (d *Document) countEntries(n int64) error {
	if n < 0 || n > maxEntries || d.entryCount+int(n) > maxEntries {
		return errors.New("too many cross-reference entries")
	}
	d.entryCount += int(n)
	return nil
}

func (d *Document) addEntries(entries []tableEntry, maxNum *int, keep func(tableEntry) bool) {
	for _, e := range entries {
		if keep != nil && !keep(e) {
			continue
		}
		if e.num > *maxNum {
			*maxNum = e.num
		}
		if _, have := d.entries[e.num]; !have {
			d.entries[e.num] = e.xrefEntry
		}
	}
}

// parseTable reads a classic cross-reference table and its trailer.
func parseTable(p *parser, count func(n int64) error) ([]tableEntry, Dict, error) {
	p.pos += len("xref")
	var entries []tableEntry
	for {
		o, err := p.object()
		if err != nil {
			return nil, nil, err
		}
		if k, ok := o.(keyword); ok && k == "trailer" {
			t, err := p.object()
			if err != nil {
				return nil, nil, err
			}
			trailer, ok := t.(Dict)
			if !ok {
				return nil, nil, errors.New("trailer is not a dictionary")
			}
			return entries, trailer, nil
		}
		start, ok := o.(Integer)
		if !ok {
			return nil, nil, errors.Errorf("expected a subsection start, got %v", o)
		}
		c, err := p.object()
		if err != nil {
			return nil, nil, err
		}
		n, ok := c.(Integer)
		if !ok || n < 0 {
			return nil, nil, errors.Errorf("invalid subsection count %v", c)
		}
		if err := count(int64(n)); err != nil {
			return nil, nil, err
		}
		for i := 0; i < int(n); i++ {
			offTok, err := p.object()
			if err != nil {
				return nil, nil, err
			}
			genTok, err := p.object()
			if err != nil {
				return nil, nil, err
			}
			kindTok, err := p.object()
			if err != nil {
				return nil, nil, err
			}
			off, ok1 := offTok.(Integer)
			gen, ok2 := genTok.(Integer)
			kind, ok3 := kindTok.(keyword)
			if !ok1 || !ok2 || !ok3 || (kind != "n" && kind != "f") {
				return nil, nil, errors.Errorf("invalid cross-reference entry %v %v %v", offTok, genTok, kindTok)
			}
			e := tableEntry{num: int(start) + i, xrefEntry: xrefEntry{offset: int64(off), gen: int(gen)}}
			if kind == "n" {
				e.kind = entryInUse
			}
			entries = append(entries, e)
		}
	}
}

// readXRefStream reads the cross-reference stream at off.
func (d *Document) readXRefStream(off int64) ([]tableEntry, Dict, error) {
	_, o, err := d.indirectAt(off)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cross-reference stream at %d", off)
	}
	s, ok := o.(*Stream)
	if !ok {
		return nil, nil, errors.Errorf("object at %d is not a cross-reference stream", off)
	}
	if t, _ := s.Dict["Type"].(Name); t != "XRef" {
		return nil, nil, errors.Errorf("stream at %d is not a cross-reference stream", off)
	}
	data, err := d.decode(s)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cross-reference stream at %d", off)
	}
	w, ok := s.Dict["W"].(Array)
	if !ok || len(w) != 3 {
		return nil, nil, errors.New("cross-reference stream has an invalid /W")
	}
	widths := make([]int, len(w))
	rowLen := 0
	for i, v := range w {
		n, ok := v.(Integer)
		if !ok || n < 0 || n > 8 {
			return nil, nil, errors.New("cross-reference stream has an invalid /W")
		}
		widths[i] = int(n)
		rowLen += int(n)
	}
	if rowLen == 0 {
		return nil, nil, errors.New("cross-reference stream has an empty /W")
	}
	index, ok := s.Dict["Index"].(Array)
	if !ok {
		size, ok := s.Dict["Size"].(Integer)
		if !ok {
			return nil, nil, errors.New("cross-reference stream has no /Size")
		}
		index = Array{Integer(0), size}
	}
	if len(index)%2 != 0 {
		return nil, nil, errors.New("cross-reference stream has an invalid /Index")
	}

	var entries []tableEntry
	row := 0
	for i := 0; i < len(index); i += 2 {
		start, ok1 := index[i].(Integer)
		count, ok2 := index[i+1].(Integer)
		if !ok1 || !ok2 || start < 0 || count < 0 {
			return nil, nil, errors.New("cross-reference stream has an invalid /Index")
		}
		if err := d.countEntries(int64(count)); err != nil {
			return nil, nil, err
		}
		for j := 0; j < int(count); j++ {
			if (row+1)*rowLen > len(data) {
				return nil, nil, errors.New("cross-reference stream is shorter than its /Index")
			}
			b := data[row*rowLen:]
			row++
			f1 := int64(1)
			if widths[0] > 0 {
				f1 = readField(b[:widths[0]])
			}
			f2 := readField(b[widths[0] : widths[0]+widths[1]])
			f3 := readField(b[widths[0]+widths[1] : rowLen])
			e := tableEntry{num: int(start) + j}
			switch f1 {
			case 0:
				e.kind = entryFree
			case 1:
				e.kind = entryInUse
				e.offset = f2
				e.gen = int(f3)
			case 2:
				e.kind = entryCompressed
				e.stream = int(f2)
				e.index = int(f3)
			default:
				// Unknown types are references to the null object.
				e.kind = entryFree
			}
			entries = append(entries, e)
		}
	}
	return entries, s.Dict, nil
}

func readField(b []byte) int64 {
	var v int64
	for _, c := range b {
		v = v<<8 | int64(c)
	}
	return v
}

// parseAt runs fn over the file from off, growing the window it reads
// until fn has the bytes it needs.
func (d *Document) parseAt(off int64, fn func(p *parser) error) error {
	if off < 0 || off >= d.size {
		return errors.Errorf("offset %d is outside the file", off)
	}
	window := int64(firstWindow)
	for {
		n := min(window, d.size-off)
		buf := make([]byte, n)
		if _, err := d.r.ReadAt(buf, off); err != nil && !errors.Is(err, io.EOF) {
			return errors.WithStack(err)
		}
		err := fn(&parser{buf: buf, final: off+n >= d.size})
		if !errors.Is(err, errShort) {
			return err
		}
		if n >= maxWindow {
			return errors.Errorf("object at offset %d is too large", off)
		}
		window = min(window*4, maxWindow)
	}
}

// indirectAt reads the indirect object at off. A stream's data is read
// from the file but left encoded.
func (d *Document) indirectAt(off int64) (Ref, Object, error) {
	var ref Ref
	var obj Object
	var dataOff int64 = -1
	err := d.parseAt(off, func(p *parser) error {
		dataOff = -1
		num, err := p.integer()
		if err != nil {
			return err
		}
		gen, err := p.integer()
		if err != nil {
			return err
		}
		p.skipSpace()
		kw, err := p.regular()
		if err != nil {
			return err
		}
		if kw != "obj" {
			return errors.Errorf("no object at offset %d", off)
		}
		ref = Ref{Num: int(num), Gen: int(gen)}
		obj, err = p.object()
		if err != nil {
			return err
		}
		if _, ok := obj.(keyword); ok {
			return errors.Errorf("object %d has no value", num)
		}
		dict, ok := obj.(Dict)
		if !ok {
			return nil
		}
		p.skipSpace()
		save := p.pos
		kw, err = p.regular()
		if err != nil {
			return err
		}
		if kw != "stream" {
			p.pos = save
			return nil
		}
		// The keyword is followed by CRLF or LF; tolerate a bare CR.
		if p.pos < len(p.buf) && p.buf[p.pos] == '\r' {
			p.pos++
			if p.pos == len(p.buf) && !p.final {
				// The LF that may follow is past the window.
				return errShort
			}
		}
		if p.pos < len(p.buf) && p.buf[p.pos] == '\n' {
			p.pos++
		}
		dataOff = off + int64(p.pos)
		obj = &Stream{Dict: dict}
		return nil
	})
	if err != nil {
		return Ref{}, nil, err
	}
	if s, ok := obj.(*Stream); ok {
		length, err := d.resolveInt(s.Dict["Length"])
		if err != nil {
			return Ref{}, nil, errors.Wrapf(err, "length of stream %d", ref.Num)
		}
		if length < 0 || dataOff+length > d.size || length > maxDecoded {
			return Ref{}, nil, errors.Errorf("stream %d has an invalid length %d", ref.Num, length)
		}
		s.Data = make([]byte, length)
		if _, err := d.r.ReadAt(s.Data, dataOff); err != nil && !errors.Is(err, io.EOF) {
			return Ref{}, nil, errors.WithStack(err)
		}
	}
	return ref, obj, nil
}

func (p *parser) integer() (int64, error) {
	p.skipSpace()
	tok, err := p.regular()
	if err != nil {
		return 0, err
	}
	n, ok := parseInt(tok)
	if !ok {
		return 0, errors.Errorf("expected an integer, got %q", tok)
	}
	return n, nil
}

func (d *Document) resolveInt(o Object) (int64, error) {
	v, err := d.Resolve(o)
	if err != nil {
		return 0, err
	}
	n, ok := v.(Integer)
	if !ok {
		return 0, errors.Errorf("expected an integer, got %T", v)
	}
	return int64(n), nil
}

// Resolve returns the object o refers to when it is a reference, and o
// otherwise. A missing dictionary entry (nil) resolves to Null.
func (d *Document) Resolve(o Object) (Object, error) {
	switch v := o.(type) {
	case nil:
		return Null{}, nil
	case Ref:
		return d.Object(v)
	}
	return o, nil
}

// Object returns the object ref refers to, or Null when the file has no
// such object, as the specification requires.
func (d *Document) Object(ref Ref) (Object, error) {
	if d.depth > maxRefDepth {
		return nil, errors.New("PDF references nested too deeply")
	}
	d.depth++
	defer func() { d.depth-- }()
	e, ok := d.entries[ref.Num]
	if !ok {
		return Null{}, nil
	}
	switch e.kind {
	case entryInUse:
		got, obj, err := d.indirectAt(e.offset)
		if err != nil {
			return nil, err
		}
		if got.Num != ref.Num {
			return nil, errors.Errorf("object %d is listed at offset %d, which holds object %d", ref.Num, e.offset, got.Num)
		}
		if d.sec != nil {
			return d.sec.crypt(got, obj, d.sec.decrypt)
		}
		return obj, nil
	case entryCompressed:
		return d.compressed(ref.Num, e)
	case entryFree:
	}
	return Null{}, nil
}

// compressed returns object num from its object stream.
func (d *Document) compressed(num int, e xrefEntry) (Object, error) {
	stm, ok := d.objStreams[e.stream]
	if !ok {
		se, ok := d.entries[e.stream]
		if !ok || se.kind != entryInUse {
			return nil, errors.Errorf("object stream %d is missing", e.stream)
		}
		var err error
		if stm, err = d.loadObjStream(e.stream, se.offset); err != nil {
			return nil, errors.Wrapf(err, "object stream %d", e.stream)
		}
		if d.objStreamBytes+len(stm.data) > maxObjStreamCache {
			d.objStreams = map[int]*objStream{}
			d.objStreamBytes = 0
		}
		d.objStreams[e.stream] = stm
		d.objStreamBytes += len(stm.data)
	}
	i := e.index
	if i < 0 || i >= len(stm.offsets) || stm.nums[i] != num {
		// Some writers get the index wrong; find the object by number.
		i = -1
		for j, n := range stm.nums {
			if n == num {
				i = j
				break
			}
		}
		if i < 0 {
			return nil, errors.Errorf("object %d is not in object stream %d", num, e.stream)
		}
	}
	p := &parser{buf: stm.data, pos: stm.offsets[i], final: true}
	return p.object()
}

func (d *Document) loadObjStream(num int, off int64) (*objStream, error) {
	got, o, err := d.indirectAt(off)
	if err != nil {
		return nil, err
	}
	if d.sec != nil {
		if o, err = d.sec.crypt(got, o, d.sec.decrypt); err != nil {
			return nil, err
		}
	}
	s, ok := o.(*Stream)
	if !ok || got.Num != num {
		return nil, errors.New("not a stream")
	}
	n, ok1 := s.Dict["N"].(Integer)
	first, ok2 := s.Dict["First"].(Integer)
	if !ok1 || !ok2 || n < 0 || first < 0 {
		return nil, errors.New("invalid /N or /First")
	}
	data, err := d.decode(s)
	if err != nil {
		return nil, err
	}
	if int(first) > len(data) {
		return nil, errors.New("/First is past the end of the stream")
	}
	stm := &objStream{data: data}
	p := &parser{buf: data[:first], final: true}
	for i := 0; i < int(n); i++ {
		objNum, err := p.integer()
		if err != nil {
			return nil, err
		}
		objOff, err := p.integer()
		if err != nil {
			return nil, err
		}
		if objOff < 0 || int(first)+int(objOff) > len(data) {
			return nil, errors.New("object offset is past the end of the stream")
		}
		stm.nums = append(stm.nums, int(objNum))
		stm.offsets = append(stm.offsets, int(first)+int(objOff))
	}
	return stm, nil
}

// decode returns a stream's data with its filters undone. Only Flate,
// with or without a PNG predictor, is supported: it is what writers use
// for cross-reference and object streams.
func (d *Document) decode(s *Stream) ([]byte, error) {
	filters, err := d.Resolve(s.Dict["Filter"])
	if err != nil {
		return nil, err
	}
	params, err := d.Resolve(s.Dict["DecodeParms"])
	if err != nil {
		return nil, err
	}
	var names []Object
	var paramList []Object
	switch f := filters.(type) {
	case Null:
		return s.Data, nil
	case Name:
		names = Array{f}
		paramList = Array{params}
	case Array:
		names = f
		if pa, ok := params.(Array); ok {
			paramList = pa
		}
	default:
		return nil, errors.Errorf("invalid /Filter %v", filters)
	}
	data := s.Data
	for i, n := range names {
		if n != Name("FlateDecode") {
			return nil, errors.Errorf("unsupported filter %v", n)
		}
		var p Dict
		if i < len(paramList) {
			v, err := d.Resolve(paramList[i])
			if err != nil {
				return nil, err
			}
			p, _ = v.(Dict)
		}
		if data, err = inflate(data); err != nil {
			return nil, err
		}
		if data, err = unpredict(data, p); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func inflate(data []byte) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer zr.Close()
	out, err := io.ReadAll(io.LimitReader(zr, maxDecoded+1))
	// Writers often leave out the checksum or the final block; what
	// inflated is still the stream.
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, errors.WithStack(err)
	}
	if len(out) > maxDecoded {
		return nil, errors.New("stream is too large")
	}
	return out, nil
}

// unpredict undoes a PNG predictor (/Predictor 10 to 15).
func unpredict(data []byte, params Dict) ([]byte, error) {
	if params == nil {
		return data, nil
	}
	predictor, _ := params["Predictor"].(Integer)
	if predictor <= 1 {
		return data, nil
	}
	if predictor < 10 {
		return nil, errors.Errorf("unsupported predictor %d", predictor)
	}
	colors, bpc, columns := Integer(1), Integer(8), Integer(1)
	if v, ok := params["Colors"].(Integer); ok {
		colors = v
	}
	if v, ok := params["BitsPerComponent"].(Integer); ok {
		bpc = v
	}
	if v, ok := params["Columns"].(Integer); ok {
		columns = v
	}
	if colors < 1 || colors > 32 || bpc < 1 || bpc > 16 || columns < 1 || columns > 1<<20 {
		return nil, errors.New("invalid predictor parameters")
	}
	bpp := int(max((colors*bpc+7)/8, 1))
	rowLen := int((colors*bpc*columns + 7) / 8)
	prev := make([]byte, rowLen)
	out := make([]byte, 0, len(data))
	for i := 0; i+1+rowLen <= len(data); i += rowLen + 1 {
		kind := data[i]
		row := append([]byte(nil), data[i+1:i+1+rowLen]...)
		for j := range row {
			var left, up, upLeft byte
			if j >= bpp {
				left = row[j-bpp]
				upLeft = prev[j-bpp]
			}
			up = prev[j]
			switch kind {
			case 0:
			case 1:
				row[j] += left
			case 2:
				row[j] += up
			case 3:
				row[j] += byte((int(left) + int(up)) / 2)
			case 4:
				row[j] += paeth(left, up, upLeft)
			default:
				return nil, errors.Errorf("invalid PNG filter type %d", kind)
			}
		}
		out = append(out, row...)
		prev = row
	}
	return out, nil
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	}
	return c
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
