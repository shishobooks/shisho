package reflowable

import (
	"context"
	"math"
	"net/url"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
)

// KoboSpan is where a Kobo is in a KePub: the id of one of the spans it
// wraps text in, in the content document at Source, the document's path in
// the archive.
type KoboSpan struct {
	Source string
	ID     string
}

// String is span as a Kobo span locator stores it: Source, "#", and ID.
func (s KoboSpan) String() string {
	return s.Source + "#" + s.ID
}

// ParseKoboSpan reads a Kobo span locator String wrote, reporting false for
// one without both a source and an id.
func ParseKoboSpan(stored string) (KoboSpan, bool) {
	i := strings.LastIndex(stored, "#")
	if i <= 0 || i == len(stored)-1 {
		return KoboSpan{}, false
	}
	return KoboSpan{Source: stored[:i], ID: stored[i+1:]}, true
}

// ErrNoKoboSpans is returned for a Kobo span in a book, or a spine item,
// without a KePub's spans.
var ErrNoKoboSpans = errors.New("no Kobo spans here")

// itemFor returns the index of the spine item at source, or -1. A Kobo names
// a chapter by its path in the archive, but a path relative to the package
// document, or escaped as in the manifest, names the same one.
func (b *Book) itemFor(source string) int {
	candidates := []string{source}
	if unescaped, err := url.PathUnescape(source); err == nil && unescaped != source {
		candidates = append(candidates, unescaped)
	}
	for _, c := range candidates {
		for i, it := range b.items {
			if it.path == c {
				return i
			}
		}
	}
	for _, c := range candidates {
		for i, it := range b.items {
			if strings.HasSuffix(it.path, "/"+c) || strings.HasSuffix(c, "/"+it.path) {
				return i
			}
		}
	}
	return -1
}

// FromKoboSpan places the start of span.
func (b *Book) FromKoboSpan(span KoboSpan) (Place, error) {
	i := b.itemFor(span.Source)
	if i < 0 || b.items[i].body == nil {
		return Place{}, errors.Wrapf(ErrNotFound, "%s", span.Source)
	}
	el := b.items[i].body.find(func(n *node) bool { return n.isKoboSpan() && n.id() == span.ID })
	if el == nil {
		return Place{}, errors.Wrapf(ErrNotFound, "span %s in %s", span.ID, span.Source)
	}
	return b.place(i, el.start, true), nil
}

// LocateKoboSpan places span exactly when the book has it, and otherwise by
// the estimate: chapterFraction of the way through its chapter, or the start
// of the chapter when that isn't known, or percentage of the way through the
// book when the chapter isn't there either.
func (b *Book) LocateKoboSpan(span KoboSpan, chapterFraction *float64, percentage float64) Place {
	if p, err := b.FromKoboSpan(span); err == nil {
		return p
	}
	i := b.itemFor(span.Source)
	if i < 0 {
		return b.AtPercentage(percentage)
	}
	fraction := 0.0
	if chapterFraction != nil {
		fraction = max(0, min(1, *chapterFraction))
	}
	return b.place(i, int(math.Round(fraction*float64(b.items[i].length()))), false)
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

// KoboBookmark is a position as a Kobo keeps it: a span, and how far through
// its chapter the position is, from 0 to 1.
type KoboBookmark struct {
	Span            KoboSpan
	ChapterFraction float64
}

// TranslateForKobo returns position, stored for the File at path of
// fileType, as the nearest span in the copy of it delivered as toType, which
// must be a KePub, since only a KePub has spans. It returns
// ErrNotTranslatable as Translate does, and ErrNoKoboSpans for a copy without
// spans.
func TranslateForKobo(ctx context.Context, path, fileType string, position *models.ReadingPosition, toType string) (KoboBookmark, error) {
	to, place, err := translate(ctx, path, fileType, position, toType)
	if err != nil {
		return KoboBookmark{}, err
	}
	span, err := to.KoboSpan(place)
	if err != nil {
		return KoboBookmark{}, err
	}
	return KoboBookmark{Span: span, ChapterFraction: place.Fraction}, nil
}
