package reflowable

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"math"
	"net/url"
	"os"
	"path"
	"strings"
	"unicode"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/models"
)

// ErrNotFound is returned for a position that names nothing in the book: a
// spine item it doesn't have, or a form it can't read.
var ErrNotFound = errors.New("position not found in the book")

// Book is one variant of a reflowable File, its content documents as one
// reader sees them: an EPUB as itself, or with a KePub's span markup.
//
// Variants of a File hold the same text, since a KePub only wraps it, so a
// Place's offset means the same in each.
type Book struct {
	items []*item
	// spineSteps is the CFI path to the OPF's spine, and itemSteps each
	// itemref's step within it.
	spineSteps string
	itemSteps  []string
	// opf is the package document, for resolving a CFI's spine steps.
	opf *node
	// itemrefs are the spine's itemrefs, in order.
	itemrefs []*node
}

// item is one spine item.
type item struct {
	// path is the content document's path in the archive.
	path string
	html *node
	body *node
}

func (it *item) length() int {
	if it.body == nil {
		return 0
	}
	return it.body.end
}

// Place is a position in a Book: a spine item, and how far through its text,
// as an offset in code points and as a fraction of the item's text.
//
// An Exact place is where a position resolved to. An estimated place is only
// as precise as its fraction, which other variants and other forms read.
type Place struct {
	// Spine is the 0-based index of the spine item.
	Spine int
	// Offset is how many code points of the item's text come before the
	// place.
	Offset int
	// Fraction is how far through the item's text the place is, from 0 to 1.
	Fraction float64
	Exact    bool
}

// Estimate is p as a spine item and a fraction through its text, which
// every form can be made from, in any variant.
func (p Place) Estimate() Place {
	return Place{Spine: p.Spine, Offset: -1, Fraction: p.Fraction}
}

// Open reads the EPUB or KePub at path as the variant kobo picks: with a
// KePub's span markup, or without. A Book is never changed once read, so the
// few read most recently are kept for the next pull or reader opening the
// same file, until its size or modification time changes.
func Open(ctx context.Context, path string, kobo bool) (*Book, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	key := cacheKey{path: path, size: info.Size(), modified: info.ModTime(), kobo: kobo}
	if book := books.get(key); book != nil {
		return book, nil
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer archive.Close()
	book, err := read(ctx, &archive.Reader, kobo)
	if err != nil {
		return nil, err
	}
	books.put(key, book)
	return book, nil
}

func read(ctx context.Context, archive *zip.Reader, kobo bool) (*Book, error) {
	files := map[string]*zip.File{}
	for _, f := range archive.File {
		files[f.Name] = f
	}
	entry := func(name string) (*node, []byte, error) {
		f, ok := files[name]
		if !ok {
			return nil, nil, errors.Errorf("%s is missing", name)
		}
		r, err := f.Open()
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
		defer r.Close()
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
		tree, err := parse(data)
		return tree, data, err
	}

	container, _, err := entry("META-INF/container.xml")
	if err != nil {
		return nil, err
	}
	rootfile := container.find(func(n *node) bool { return n.name == "rootfile" })
	if rootfile == nil || rootfile.attrs["full-path"] == "" {
		return nil, errors.New("container.xml names no package document")
	}
	opfPath := rootfile.attrs["full-path"]
	opfRoot, _, err := entry(opfPath)
	if err != nil {
		return nil, err
	}
	pkg := opfRoot.find(func(n *node) bool { return n.name == "package" })
	if pkg == nil {
		return nil, errors.New("the package document has no package")
	}
	spine := pkg.find(func(n *node) bool { return n.name == "spine" })
	if spine == nil {
		return nil, errors.New("the package document has no spine")
	}
	hrefs := map[string]string{}
	if manifest := pkg.find(func(n *node) bool { return n.name == "manifest" }); manifest != nil {
		for _, it := range manifest.elements() {
			hrefs[it.attrs["id"]] = it.attrs["href"]
		}
	}

	b := &Book{opf: pkg, spineSteps: elementStep(spine)}
	for _, itemref := range spine.elements() {
		if itemref.name != "itemref" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, errors.WithStack(err)
		}
		b.itemrefs = append(b.itemrefs, itemref)
		b.itemSteps = append(b.itemSteps, elementStep(itemref))
		href := hrefs[itemref.attrs["idref"]]
		if unescaped, err := url.PathUnescape(href); err == nil {
			href = unescaped
		}
		it := &item{path: path.Join(path.Dir(opfPath), strings.SplitN(href, "#", 2)[0])}
		b.items = append(b.items, it)
		doc, data, err := entry(it.path)
		if err != nil {
			// A spine item that's missing still counts, as crengine and
			// foliate count it; it just has no text.
			continue
		}
		it.html, it.body = documentParts(doc)
		if kobo && !hasKoboMarkup(doc) {
			// Kobo markup goes on as Shisho's KePub conversion adds it.
			var converted bytes.Buffer
			if err := kepub.TransformContentWithOptions(bytes.NewReader(data), &converted, kepub.NewSpanCounter(), ""); err != nil {
				return nil, err
			}
			if doc, err = parse(converted.Bytes()); err != nil {
				return nil, err
			}
			it.html, it.body = documentParts(doc)
		}
	}
	if !kobo {
		for _, it := range b.items {
			if it.body != nil {
				unwrapKobo(it.body)
			}
		}
	}
	for _, it := range b.items {
		if it.body != nil {
			measure(it.body, 0)
		}
	}
	return b, nil
}

func documentParts(doc *node) (htmlEl, body *node) {
	htmlEl = doc.find(func(n *node) bool { return n.name == "html" })
	if htmlEl == nil {
		return nil, nil
	}
	return htmlEl, htmlEl.find(func(n *node) bool { return n.name == "body" })
}

func hasKoboMarkup(doc *node) bool {
	return doc.find((*node).isKoboSpan) != nil
}

// place makes the Place at offset in spine item i.
func (b *Book) place(i, offset int, exact bool) Place {
	length := b.items[i].length()
	offset = max(0, min(offset, length))
	fraction := 0.0
	if length > 0 {
		fraction = float64(offset) / float64(length)
	}
	return Place{Spine: i, Offset: offset, Fraction: fraction, Exact: exact}
}

// resolve returns p's spine item and offset in b: the offset an exact place
// holds, or the one its fraction estimates. It returns nil for a spine item b
// doesn't have.
func (b *Book) resolve(p Place) (*item, int) {
	if p.Spine < 0 || p.Spine >= len(b.items) {
		return nil, 0
	}
	it := b.items[p.Spine]
	length := it.length()
	if !p.Exact || p.Offset < 0 {
		return it, int(math.Round(p.Fraction * float64(length)))
	}
	return it, min(p.Offset, length)
}

// AtPercentage estimates the place percentage of the way through the book's
// text.
func (b *Book) AtPercentage(percentage float64) Place {
	total := 0
	for _, it := range b.items {
		total += it.length()
	}
	target := int(math.Round(max(0, min(1, percentage)) * float64(total)))
	last := 0
	for i, it := range b.items {
		if it.length() == 0 {
			continue
		}
		last = i
		if target < it.length() {
			p := b.place(i, target, false)
			return p
		}
		target -= it.length()
	}
	if len(b.items) == 0 {
		return Place{}
	}
	return b.place(last, b.items[last].length(), false)
}

// Locate places a stored position in b: exactly when its form resolves here,
// and otherwise by the estimate, falling back on how far through the book it
// is. A Kobo span is placed by percentage until Kobo positions are stored.
func (b *Book) Locate(locator models.ReadingLocator, percentage float64) Place {
	var p Place
	var err error
	switch {
	case locator.CFI != nil:
		p, err = b.FromCFI(*locator.CFI)
	case locator.XPointer != nil:
		p, err = b.FromXPointer(*locator.XPointer)
	default:
		err = ErrNotFound
	}
	if err != nil {
		return b.AtPercentage(percentage)
	}
	return p
}

// textAt is up to n code points of text from p, with whitespace collapsed.
func (b *Book) textAt(p Place, n int) string {
	it, offset := b.resolve(p)
	if it == nil || it.body == nil {
		return ""
	}
	var out []rune
	space := false
	it.body.walk(func(t *node) {
		if !t.isText() || t.end <= offset {
			return
		}
		for _, r := range t.text[max(0, offset-t.start):] {
			if unicode.IsSpace(r) {
				space = true
				continue
			}
			if space && len(out) > 0 {
				out = append(out, ' ')
			}
			space = false
			out = append(out, r)
		}
	})
	if len(out) > n {
		out = out[:n]
	}
	return string(out)
}
