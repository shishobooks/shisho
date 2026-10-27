package mobigen

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"

	"github.com/pkg/errors"
	"golang.org/x/text/encoding/charmap"
)

// MOBI6Content is the text of a MOBI6 (KF7) book: one HTML stream, as
// Calibre and KindleGen write it, where links and the table of contents
// name places by their byte offset in the text. Text refers to places and
// images with placeholders the builder resolves:
//
//   - {{NAME}} marks a place and is removed
//   - filepos:NAME becomes filepos=0000001234, the offset of place NAME, as
//     a link names it
//   - recindex:N becomes recindex="0000K", the resource number of Images[N]
//
// A TOCEntry's Target names a place.
type MOBI6Content struct {
	Text string
	// Images are resource records after the cover and thumbnail.
	Images [][]byte
}

var (
	mobi6Place   = regexp.MustCompile(`\{\{([A-Za-z0-9_-]+)\}\}`)
	mobi6Link    = regexp.MustCompile(`filepos:([A-Za-z0-9_-]+)`)
	mobi6Image   = regexp.MustCompile(`recindex:(\d+)`)
	mobi6Pending = []byte("filepos=__________")
)

// mobi6Layout is a MOBI6 book's text and the offset of each place in it.
type mobi6Layout struct {
	text   []byte
	places map[string]uint32
}

// layoutMOBI6 builds the book's text, encoded as Windows-1252 when cp1252
// is set. firstEmbed is the resource number of Images[0].
func layoutMOBI6(c *MOBI6Content, firstEmbed int, cp1252 bool) (*mobi6Layout, error) {
	text := []byte(c.Text)
	if cp1252 {
		var err error
		if text, err = charmap.Windows1252.NewEncoder().Bytes(text); err != nil {
			return nil, errors.Wrap(err, "encode MOBI6 text as Windows-1252")
		}
	}
	var imageErr error
	text = mobi6Image.ReplaceAllFunc(text, func(m []byte) []byte {
		n, _ := strconv.Atoi(string(mobi6Image.FindSubmatch(m)[1]))
		if n >= len(c.Images) {
			imageErr = errors.Errorf("no image %d", n)
		}
		return []byte(fmt.Sprintf(`recindex="%05d"`, firstEmbed+n))
	})
	if imageErr != nil {
		return nil, imageErr
	}
	// Links take the length they will have once resolved, so the offsets
	// of the places after them are right.
	var links []string
	text = mobi6Link.ReplaceAllFunc(text, func(m []byte) []byte {
		links = append(links, string(mobi6Link.FindSubmatch(m)[1]))
		return mobi6Pending
	})
	l := &mobi6Layout{places: map[string]uint32{}}
	var out bytes.Buffer
	last := 0
	for _, m := range mobi6Place.FindAllSubmatchIndex(text, -1) {
		out.Write(text[last:m[0]])
		l.places[string(text[m[2]:m[3]])] = u32(out.Len())
		last = m[1]
	}
	out.Write(text[last:])
	l.text = out.Bytes()
	for _, name := range links {
		at := bytes.Index(l.text, mobi6Pending)
		place, ok := l.places[name]
		if !ok {
			return nil, errors.Errorf("no place %q", name)
		}
		copy(l.text[at:], fmt.Sprintf("filepos=%010d", place))
	}
	return l, nil
}
