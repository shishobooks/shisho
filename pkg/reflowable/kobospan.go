package reflowable

import (
	"github.com/pkg/errors"
)

// KoboSpan is where a Kobo is in a KePub: the id of one of the spans it
// wraps text in, in the content document at Source, the document's path in
// the archive.
type KoboSpan struct {
	Source string
	ID     string
}

// ErrNoKoboSpans is returned for a Kobo span in a book, or a spine item,
// without a KePub's spans.
var ErrNoKoboSpans = errors.New("no Kobo spans here")

// FromKoboSpan places the start of span.
func (b *Book) FromKoboSpan(span KoboSpan) (Place, error) {
	for i, it := range b.items {
		if it.path != span.Source || it.body == nil {
			continue
		}
		el := it.body.find(func(n *node) bool { return n.isKoboSpan() && n.id() == span.ID })
		if el == nil {
			return Place{}, errors.Wrapf(ErrNotFound, "span %s in %s", span.ID, span.Source)
		}
		return b.place(i, el.start, true), nil
	}
	return Place{}, errors.Wrapf(ErrNotFound, "%s", span.Source)
}

// KoboSpan is the span at p: the one holding it, or the next after it, or
// at the end, the last.
func (b *Book) KoboSpan(p Place) (KoboSpan, error) {
	it, offset := b.resolve(p)
	if it == nil || it.body == nil {
		return KoboSpan{}, ErrNoKoboSpans
	}
	var found, last *node
	it.body.walk(func(n *node) {
		if found != nil || !n.isKoboSpan() {
			return
		}
		last = n
		if offset < n.end {
			found = n
		}
	})
	if found == nil {
		found = last
	}
	if found == nil {
		return KoboSpan{}, ErrNoKoboSpans
	}
	return KoboSpan{Source: it.path, ID: found.id()}, nil
}
