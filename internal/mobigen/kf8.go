package mobigen

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// KF8Content is the text of a KF8 book, laid out as Calibre and KindleGen
// lay it out: each part is a skeleton with its body cut out into fragments
// stored after it, the parts are followed by the CSS flows, and a skeleton
// index and a fragment index say how to put them back together.
//
// Parts refer to the rest of the book with placeholders the builder
// resolves:
//
//   - kindle:embed:img:N names Images[N]
//   - kindle:pos:id:ID names the element with id="ID", or with
//     data-anchor="ID" for an element that has no id, as a link and as a
//     TOCEntry's Target do
//
// A part links style n (counting from 1) as kindle:flow:000n?mime=text/css.
type KF8Content struct {
	Parts  []KF8Part
	Styles []string
	// Images are resource records after the cover and thumbnail.
	Images [][]byte
	// Font adds a FONT resource record, as a book with an embedded font has.
	Font bool
}

// KF8Part is one XHTML document. Head and Tail are its skeleton, before and
// after the body content; empty, they are a plain XHTML head and body.
// Fragments are the body content, in the order they are inserted.
type KF8Part struct {
	Head, Tail string
	Fragments  []string
}

const (
	defaultKF8Head = `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Part</title><link href="kindle:flow:0001?mime=text/css" rel="stylesheet" type="text/css"/></head><body aid="0">`
	defaultKF8Tail = `</body></html>`

	// posPlaceholder has the length of a resolved kindle:pos link, so
	// positions found with it in place stay right once it is resolved.
	posPlaceholder = "kindle:pos:fid:____:off:__________"
)

var (
	embedRef = regexp.MustCompile(`kindle:embed:img:(\d+)`)
	posRef   = regexp.MustCompile(`kindle:pos:id:([A-Za-z0-9_.-]+)`)
)

// kf8Layout is a KF8 book's text and the indexes that describe it.
type kf8Layout struct {
	text      []byte
	flowEnds  []int // end of each flow in text; flow 0 is the parts
	skeletons []kf8Skeleton
	fragments []kf8Fragment
	// targets maps an element id to the fragment it is in and its offset
	// from the fragment's start.
	targets map[string][2]uint32
}

type kf8Skeleton struct {
	start, length, fragments int
}

type kf8Fragment struct {
	insert, part, length int
}

// layoutKF8 builds the book's text. firstEmbed is the kindle:embed number of
// Images[0].
func layoutKF8(c *KF8Content, firstEmbed int) (*kf8Layout, error) {
	resolve := func(s string) (string, error) {
		var err error
		s = embedRef.ReplaceAllStringFunc(s, func(m string) string {
			n, _ := strconv.Atoi(embedRef.FindStringSubmatch(m)[1])
			if n >= len(c.Images) {
				err = errors.Errorf("no image %d", n)
			}
			return fmt.Sprintf("kindle:embed:%04s?mime=image/png", strings.ToUpper(strconv.FormatInt(int64(firstEmbed+n), 32)))
		})
		return s, err
	}
	l := &kf8Layout{targets: map[string][2]uint32{}}
	type pending struct {
		at int
		id string
	}
	var links []pending
	var flow0 bytes.Buffer
	for p, part := range c.Parts {
		head, tail := part.Head, part.Tail
		if head == "" && tail == "" {
			head, tail = defaultKF8Head, defaultKF8Tail
		}
		var err error
		if head, err = resolve(head); err != nil {
			return nil, err
		}
		frags := make([]string, len(part.Fragments))
		for i, f := range part.Fragments {
			if frags[i], err = resolve(f); err != nil {
				return nil, err
			}
		}

		start := flow0.Len()
		flow0.WriteString(head)
		flow0.WriteString(tail)
		l.skeletons = append(l.skeletons, kf8Skeleton{start: start, length: len(head) + len(tail), fragments: len(frags)})
		// Every fragment goes in where the head ends, after the ones before
		// it, at a position counted as if the part were assembled.
		insert := start + len(head)
		for _, f := range frags {
			fid := len(l.fragments)
			fragStart := flow0.Len()
			ids := posRef.FindAllStringSubmatch(f, -1)
			f = posRef.ReplaceAllString(f, posPlaceholder)
			at := 0
			for _, id := range ids {
				at += strings.Index(f[at:], posPlaceholder)
				links = append(links, pending{at: fragStart + at, id: id[1]})
				at += len(posPlaceholder)
			}
			for _, m := range regexp.MustCompile(`<[^>]*\s(?:id|data-anchor)="([^"]+)"`).FindAllStringSubmatchIndex(f, -1) {
				l.targets[f[m[2]:m[3]]] = [2]uint32{u32(fid), u32(m[0])}
			}
			flow0.WriteString(f)
			l.fragments = append(l.fragments, kf8Fragment{insert: insert, part: p, length: len(f)})
			insert += len(f)
		}
	}

	text := flow0.Bytes()
	for _, link := range links {
		target, ok := l.targets[link.id]
		if !ok {
			return nil, errors.Errorf("no element with id %q", link.id)
		}
		resolved := fmt.Sprintf("kindle:pos:fid:%04s:off:%010s", base32(target[0]), base32(target[1]))
		copy(text[link.at:], resolved)
	}
	if bytes.Contains(text, []byte("_____")) {
		return nil, errors.New("a kindle:pos placeholder was left unresolved")
	}

	l.text = append([]byte(nil), text...)
	l.flowEnds = []int{len(l.text)}
	for _, s := range c.Styles {
		s, err := resolve(s)
		if err != nil {
			return nil, err
		}
		l.text = append(l.text, s...)
		l.flowEnds = append(l.flowEnds, len(l.text))
	}
	return l, nil
}

func base32(n uint32) string {
	return strings.ToUpper(strconv.FormatUint(uint64(n), 32))
}

// fdst returns the FDST record listing the flows.
func (l *kf8Layout) fdst() []byte {
	r := make([]byte, 12+8*len(l.flowEnds))
	copy(r, "FDST")
	binary.BigEndian.PutUint32(r[4:], 12)
	binary.BigEndian.PutUint32(r[8:], u32(len(l.flowEnds)))
	start := 0
	for i, end := range l.flowEnds {
		binary.BigEndian.PutUint32(r[12+8*i:], u32(start))
		binary.BigEndian.PutUint32(r[16+8*i:], u32(end))
		start = end
	}
	return r
}

// skeletonIndex returns the skeleton index: an entry per part naming its
// fragment count (tag 1) and its start and length in the text (tag 6).
// Calibre writes the start and length twice.
func (l *kf8Layout) skeletonIndex() [][]byte {
	entries := make([]indexEntry, len(l.skeletons))
	for i, s := range l.skeletons {
		entries[i] = indexEntry{
			ident:  []byte(fmt.Sprintf("SKEL%010d", i)),
			values: map[byte][]uint32{1: {u32(s.fragments)}, 6: {u32(s.start), u32(s.length), u32(s.start), u32(s.length)}},
		}
	}
	return indexRecords([]ncxTag{{1, 1, 0x03}, {6, 2, 0x0C}}, entries, nil)
}

// fragmentIndex returns the fragment index: an entry per fragment, named by
// its insert position, with the CNCX offset of its selector (tag 2), its
// part (tag 3), its sequence number (tag 4), and its offset in the part's
// stored fragments and its length (tag 6).
func (l *kf8Layout) fragmentIndex() [][]byte {
	var cncx bytes.Buffer
	entries := make([]indexEntry, len(l.fragments))
	offsetInPart := 0
	for i, f := range l.fragments {
		if i == 0 || l.fragments[i-1].part != f.part {
			offsetInPart = 0
		}
		selector := []byte("P-//*[@aid='0']")
		label := cncx.Len()
		cncx.Write(forwardInt(u32(len(selector))))
		cncx.Write(selector)
		entries[i] = indexEntry{
			ident: []byte(fmt.Sprintf("%010d", f.insert)),
			values: map[byte][]uint32{
				2: {u32(label)},
				3: {u32(f.part)},
				4: {u32(i)},
				6: {u32(offsetInPart), u32(f.length)},
			},
		}
		offsetInPart += f.length
	}
	return indexRecords([]ncxTag{{2, 1, 0x01}, {3, 1, 0x02}, {4, 1, 0x04}, {6, 2, 0x08}}, entries, [][]byte{cncx.Bytes()})
}

// indexEntry is one entry of a generated index.
type indexEntry struct {
	ident  []byte
	values map[byte][]uint32
}

// indexRecords returns an index in one data record: the primary INDX
// record, the data record, and the CNCX records. A tag's values are written
// with its control bits holding their count over its values per entry.
func indexRecords(tags []ncxTag, entries []indexEntry, cncx [][]byte) [][]byte {
	be := binary.BigEndian
	var body bytes.Buffer
	offsets := make([]int, 0, len(entries))
	for _, e := range entries {
		offsets = append(offsets, indxHeaderLength+body.Len())
		var control byte
		var data []byte
		for _, t := range tags {
			v, ok := e.values[t.tag]
			if !ok {
				continue
			}
			shift := 0
			for t.mask>>shift&1 == 0 {
				shift++
			}
			control |= byte(len(v)/int(t.values)) << shift
			for _, n := range v {
				data = append(data, forwardInt(n)...)
			}
		}
		body.WriteByte(byte(len(e.ident)))
		body.Write(e.ident)
		body.WriteByte(control)
		body.Write(data)
	}
	padTo4(&body)
	data := indxHeader()
	be.PutUint32(data[12:], 1)
	be.PutUint32(data[20:], u32(indxHeaderLength+body.Len()))
	be.PutUint32(data[24:], u32(len(offsets)))
	be.PutUint32(data[28:], mobiNullIndex)
	data = append(data, body.Bytes()...)
	data = append(data, idxt(offsets)...)

	var tagx bytes.Buffer
	tagx.WriteString("TAGX")
	_ = binary.Write(&tagx, be, u32(12+4*(len(tags)+1)))
	_ = binary.Write(&tagx, be, uint32(1))
	for _, t := range tags {
		tagx.Write([]byte{t.tag, t.values, t.mask, 0})
	}
	tagx.Write([]byte{0, 0, 0, 1})
	var geometry bytes.Buffer
	geometry.Write(tagx.Bytes())
	geometryOffset := indxHeaderLength + geometry.Len()
	var last []byte
	if len(entries) > 0 {
		last = entries[len(entries)-1].ident
	}
	geometry.WriteByte(byte(len(last)))
	geometry.Write(last)
	_ = binary.Write(&geometry, be, u16(len(entries)))
	padTo4(&geometry)

	primary := indxHeader()
	be.PutUint32(primary[16:], 2)
	be.PutUint32(primary[20:], u32(indxHeaderLength+geometry.Len()))
	be.PutUint32(primary[24:], 1)
	be.PutUint32(primary[28:], mobiEncodingUTF8)
	be.PutUint32(primary[36:], u32(len(entries)))
	be.PutUint32(primary[52:], u32(len(cncx)))
	be.PutUint32(primary[180:], indxHeaderLength)
	primary = append(primary, geometry.Bytes()...)
	primary = append(primary, idxt([]int{geometryOffset})...)

	records := [][]byte{primary, data}
	for _, rec := range cncx {
		b := bytes.NewBuffer(append([]byte(nil), rec...))
		padTo4(b)
		records = append(records, b.Bytes())
	}
	return records
}

// splitTextRecords cuts text into records of at most size bytes, as the
// text of every MOBI book is stored.
func splitTextRecords(text []byte, size int) [][]byte {
	var out [][]byte
	for len(text) > size {
		out = append(out, text[:size])
		text = text[size:]
	}
	return append(out, text)
}
