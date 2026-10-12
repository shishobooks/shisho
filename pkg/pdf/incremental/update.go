package incremental

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/pkg/errors"
)

// maxPageTreeDepth bounds the page tree walk.
const maxPageTreeDepth = 64

// Catalog returns the document catalog and its reference.
func (d *Document) Catalog() (Ref, Dict, error) {
	ref, _ := d.trailer["Root"].(Ref)
	o, err := d.Object(ref)
	if err != nil {
		return Ref{}, nil, errors.Wrap(err, "failed to read the catalog")
	}
	catalog, ok := o.(Dict)
	if !ok {
		return Ref{}, nil, errors.Errorf("catalog %d is not a dictionary", ref.Num)
	}
	return ref, catalog, nil
}

// version returns the PDF version the file declares, such as "1.4": the
// later of its header and the catalog's /Version. It is empty when
// neither can be read.
func (d *Document) version() string {
	header := make([]byte, 16)
	n, _ := d.r.ReadAt(header, 0)
	version := ""
	if i := bytes.Index(header[:n], []byte("%PDF-")); i >= 0 {
		v := header[i+len("%PDF-"):]
		end := bytes.IndexFunc(v, func(r rune) bool { return (r < '0' || r > '9') && r != '.' })
		if end < 0 {
			end = len(v)
		}
		version = string(v[:end])
	}
	if _, catalog, err := d.Catalog(); err == nil {
		if v, ok := catalog["Version"].(Name); ok && compareVersions(string(v), version) > 0 {
			version = string(v)
		}
	}
	return version
}

// compareVersions compares PDF versions of the form major.minor.
func compareVersions(a, b string) int {
	parse := func(v string) (int, int) {
		var major, minor int
		_, _ = fmt.Sscanf(v, "%d.%d", &major, &minor)
		return major, minor
	}
	am, an := parse(a)
	bm, bn := parse(b)
	if am != bm {
		return am - bm
	}
	return an - bn
}

// AtLeastVersion reports whether the file declares version v or later.
func (d *Document) AtLeastVersion(v string) bool {
	return compareVersions(d.version(), v) >= 0
}

// Pages returns a reference to every page, in page order.
func (d *Document) Pages() ([]Ref, error) {
	_, catalog, err := d.Catalog()
	if err != nil {
		return nil, err
	}
	root, ok := catalog["Pages"].(Ref)
	if !ok {
		return nil, errors.New("catalog has no page tree")
	}
	var pages []Ref
	visited := map[Ref]bool{}
	var walk func(ref Ref, depth int) error
	walk = func(ref Ref, depth int) error {
		if depth > maxPageTreeDepth {
			return errors.New("page tree is nested too deeply")
		}
		if visited[ref] {
			return errors.New("page tree has a cycle")
		}
		visited[ref] = true
		o, err := d.Object(ref)
		if err != nil {
			return err
		}
		node, ok := o.(Dict)
		if !ok {
			return errors.Errorf("page tree node %d is not a dictionary", ref.Num)
		}
		kidsObj, err := d.Resolve(node["Kids"])
		if err != nil {
			return err
		}
		kids, isTree := kidsObj.(Array)
		if t, _ := node["Type"].(Name); t == "Page" || !isTree {
			pages = append(pages, ref)
			return nil
		}
		for _, k := range kids {
			kid, ok := k.(Ref)
			if !ok {
				return errors.Errorf("page tree node %d has a kid that is not a reference", ref.Num)
			}
			if err := walk(kid, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, 0); err != nil {
		return nil, err
	}
	return pages, nil
}

// Update collects the objects of an incremental update.
type Update struct {
	doc     *Document
	next    int
	objects map[Ref]Object
	trailer Dict
}

// NewUpdate starts an incremental update to d.
func (d *Document) NewUpdate() *Update {
	return &Update{doc: d, next: d.nextNum, objects: map[Ref]Object{}, trailer: Dict{}}
}

// Reserve returns a reference for a new object, so objects that refer to
// each other can be built before each is Set.
func (u *Update) Reserve() Ref {
	r := Ref{Num: u.next}
	u.next++
	return r
}

// Add adds a new object and returns its reference.
func (u *Update) Add(o Object) Ref {
	r := u.Reserve()
	u.Set(r, o)
	return r
}

// Set sets the object ref refers to: a reserved new object, or a new
// version of an object in the original file.
func (u *Update) Set(ref Ref, o Object) {
	u.objects[ref] = o
}

// SetTrailer sets a trailer entry, such as /Info. /Size, /Prev, /Root, and
// /ID are set from the original file.
func (u *Update) SetTrailer(key Name, o Object) {
	u.trailer[key] = o
}

// Bytes returns the update, to be written right after the original file's
// last byte. Its cross-reference section is the same kind as the
// original's last one and chains back to it.
func (u *Update) Bytes() ([]byte, error) {
	var b bytes.Buffer
	if !u.doc.endsWithEOL {
		b.WriteByte('\n')
	}
	base := u.doc.size

	refs := make([]Ref, 0, len(u.objects))
	for r := range u.objects {
		refs = append(refs, r)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Num < refs[j].Num })

	offsets := map[Ref]int64{}
	for _, r := range refs {
		offsets[r] = base + int64(b.Len())
		fmt.Fprintf(&b, "%d %d obj\n", r.Num, r.Gen)
		if err := writeObject(&b, u.objects[r]); err != nil {
			return nil, errors.Wrapf(err, "object %d", r.Num)
		}
		b.WriteString("\nendobj\n")
	}

	trailer := u.trailer.Clone()
	trailer["Prev"] = Integer(u.doc.startxref)
	trailer["Root"] = u.doc.trailer["Root"]
	if id, ok := u.doc.trailer["ID"]; ok {
		trailer["ID"] = id
	}

	xrefOff := base + int64(b.Len())
	if u.doc.xrefStream {
		self := Ref{Num: u.next}
		refs = append(refs, self)
		offsets[self] = xrefOff
		trailer["Size"] = Integer(self.Num + 1)
		if err := writeXRefStream(&b, self, trailer, refs, offsets); err != nil {
			return nil, err
		}
	} else {
		trailer["Size"] = Integer(u.next)
		writeXRefTable(&b, refs, offsets)
		b.WriteString("trailer\n")
		if err := writeDict(&b, trailer); err != nil {
			return nil, errors.Wrap(err, "trailer")
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "startxref\n%d\n%%%%EOF\n", xrefOff)
	return b.Bytes(), nil
}

// runs groups sorted references into runs of consecutive object numbers,
// the subsections of a cross-reference section.
func runs(refs []Ref) [][]Ref {
	var out [][]Ref
	for i, r := range refs {
		if i == 0 || r.Num != refs[i-1].Num+1 {
			out = append(out, nil)
		}
		out[len(out)-1] = append(out[len(out)-1], r)
	}
	return out
}

func writeXRefTable(b *bytes.Buffer, refs []Ref, offsets map[Ref]int64) {
	b.WriteString("xref\n")
	for _, run := range runs(refs) {
		fmt.Fprintf(b, "%d %d\n", run[0].Num, len(run))
		for _, r := range run {
			fmt.Fprintf(b, "%010d %05d n\r\n", offsets[r], r.Gen)
		}
	}
}

func writeXRefStream(b *bytes.Buffer, self Ref, trailer Dict, refs []Ref, offsets map[Ref]int64) error {
	offWidth := 1
	for _, off := range offsets {
		for off>>(8*offWidth) > 0 {
			offWidth++
		}
	}
	var index Array
	var data []byte
	for _, run := range runs(refs) {
		index = append(index, Integer(run[0].Num), Integer(len(run)))
		for _, r := range run {
			data = append(data, 1)
			for i := offWidth - 1; i >= 0; i-- {
				data = append(data, byte(offsets[r]>>(8*i)))
			}
			data = append(data, byte(r.Gen>>8), byte(r.Gen))
		}
	}
	dict := trailer.Clone()
	dict["Type"] = Name("XRef")
	dict["W"] = Array{Integer(1), Integer(offWidth), Integer(2)}
	dict["Index"] = index
	fmt.Fprintf(b, "%d 0 obj\n", self.Num)
	if err := writeObject(b, &Stream{Dict: dict, Data: data}); err != nil {
		return errors.Wrap(err, "cross-reference stream")
	}
	b.WriteString("\nendobj\n")
	return nil
}
