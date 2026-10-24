package filegen

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"html"
	"io"
	"path"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/ziputil"
)

// kindleToEPUB is the Conversion step from MOBI or AZW3 to EPUB, for the
// KF8 book in the file. It reads the book's documents, styles, and table of
// contents into an EPUB held in memory, without its images, and runs the
// EPUB generator's preparation on that, so the book's metadata is applied
// as for any EPUB. The images are copied from the source as they are
// written, after everything else, so the download starts before they are
// read. The size is known up front.
type kindleToEPUB struct {
	fileType string
}

// kindleEPUBDir is the directory of the package in a converted EPUB. Every
// document, style, and image sits in it beside the OPF, so the relative
// hrefs mobi.ReadKF8 gives work as they are.
const kindleEPUBDir = "OEBPS"

func (s kindleToEPUB) prepare(ctx context.Context, in stepInput) (Prepared, error) {
	if in.entries != nil {
		return nil, NewGenerationError(models.FileTypeEPUB, nil, "a Kindle Conversion must read the Original File")
	}
	if err := ctx.Err(); err != nil {
		return nil, NewGenerationError(models.FileTypeEPUB, err, "generation cancelled")
	}
	label := models.FileTypeLabel(s.fileType)
	kf8, err := mobi.ReadKF8(ctx, in.src, in.src.Size())
	if err != nil {
		var unsupported *mobi.UnsupportedError
		switch {
		case errors.As(err, &unsupported), errors.Is(err, mobi.ErrDRMProtected):
			return nil, NewGenerationError(models.FileTypeEPUB, err, err.Error())
		default:
			return nil, NewGenerationError(models.FileTypeEPUB, err, "failed to read "+label+": "+err.Error())
		}
	}

	base, err := kindleEPUBArchive(kf8)
	if err != nil {
		return nil, NewGenerationError(models.FileTypeEPUB, err, "failed to convert "+label+": "+err.Error())
	}
	file := *in.file
	file.Chapters = kindleChapters(kf8, in.file.Chapters)
	plan, err := planEPUB(ctx, bytes.NewReader(base), in.book, &file)
	if err != nil {
		return nil, err
	}

	p := &preparedKindleEPUB{kf8: kf8, plan: plan}
	for _, r := range kf8.Resources {
		if r != kf8.Cover {
			p.images = append(p.images, r)
		}
	}
	return p, nil
}

// compressPlanned returns a planned entry ready to write: rewritten
// content compressed, or an entry of the in-memory EPUB as it is stored
// there.
func compressPlanned(e epubEntry) (*ziputil.Entry, error) {
	if e.source == nil {
		return ziputil.Compress(e.name, rewrittenMethod(e), e.content)
	}
	r, err := e.source.OpenRaw()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &ziputil.Entry{Header: *ziputil.RawHeader(e.source), Data: data}, nil
}

// preparedKindleEPUB is an EPUB ready to be converted from a MOBI or AZW3.
// The entries are compressed and the output measured only once Size or
// Write needs them, since a chain's next step reads entries instead.
type preparedKindleEPUB struct {
	kf8  *mobi.KF8
	plan *epubPlan
	// images are the resources still to be read from the source: all but
	// the cover, which the plan holds.
	images []*mobi.Resource

	once sync.Once
	// compressed holds plan.entries ready to write.
	compressed []*ziputil.Entry
	size       int64
	err        error
}

// ready compresses the planned entries and measures the output.
func (p *preparedKindleEPUB) ready() error {
	p.once.Do(func() {
		for _, e := range p.plan.entries {
			compressed, err := compressPlanned(e)
			if err != nil {
				p.err = errors.Wrap(err, "failed to compress converted entry")
				return
			}
			p.compressed = append(p.compressed, compressed)
		}
		size := &countingWriter{}
		if err := p.write(context.Background(), size, true); err != nil {
			p.err = errors.Wrap(err, "failed to measure destination EPUB")
			return
		}
		p.size = size.n
	})
	return p.err
}

func (p *preparedKindleEPUB) Size() (int64, bool) {
	if p.ready() != nil {
		return 0, false
	}
	return p.size, true
}

func (p *preparedKindleEPUB) Write(ctx context.Context, w io.Writer) error {
	if err := p.ready(); err != nil {
		return NewGenerationError(models.FileTypeEPUB, err, err.Error())
	}
	if err := p.write(ctx, w, false); err != nil {
		return NewGenerationError(models.FileTypeEPUB, err, "failed to write destination EPUB")
	}
	return nil
}

// write writes the EPUB to w. Measuring writes each image's size in zeros
// instead of reading it, which leaves the output the same size: an image is
// stored, and only its checksum depends on its bytes.
func (p *preparedKindleEPUB) write(ctx context.Context, w io.Writer, measure bool) error {
	zw := zip.NewWriter(w)
	for i, e := range p.compressed {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "generation cancelled")
		}
		if err := e.Write(zw); err != nil {
			return err
		}
		if i == 0 {
			// Send the mimetype entry now, so the download starts before
			// the rest is compressed and written.
			if err := zw.Flush(); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	for _, r := range p.images {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "generation cancelled")
		}
		name := path.Join(kindleEPUBDir, r.Name)
		if measure {
			fh := zip.FileHeader{Name: name, Method: zip.Store, CompressedSize64: uint64(r.Size), UncompressedSize64: uint64(r.Size)} //nolint:gosec // a record size is not negative
			body, err := zw.CreateRaw(&fh)
			if err != nil {
				return errors.WithStack(err)
			}
			if err := writeZeros(body, r.Size); err != nil {
				return err
			}
			continue
		}
		data, err := p.kf8.ReadResource(r)
		if err != nil {
			return err
		}
		if int64(len(data)) != r.Size {
			return errors.Errorf("%s changed size while it was read", r.Name)
		}
		entry, err := ziputil.Compress(name, zip.Store, data)
		if err != nil {
			return err
		}
		if err := entry.Write(zw); err != nil {
			return err
		}
	}
	return errors.Wrap(zw.Close(), "failed to finalize destination EPUB")
}

// entries returns the EPUB for the next step of a chain, with each image
// read from the source only as that step converts it.
func (p *preparedKindleEPUB) entries() []kepub.Entry {
	out := make([]kepub.Entry, 0, len(p.plan.entries)+len(p.images))
	for _, e := range p.plan.entries {
		out = append(out, kepub.Entry{Name: e.name, File: e.source, Content: e.content})
	}
	for _, r := range p.images {
		out = append(out, kepub.Entry{Name: path.Join(kindleEPUBDir, r.Name), Load: func() ([]byte, error) {
			return p.kf8.ReadResource(r)
		}})
	}
	return out
}

// kindleChapters returns the stored chapters with each location replaced by
// the href it became in the EPUB, so the EPUB generator matches them to the
// table of contents the way it matches an EPUB's own.
func kindleChapters(kf8 *mobi.KF8, stored []*models.Chapter) []*models.Chapter {
	var convert func([]*models.Chapter) []*models.Chapter
	convert = func(level []*models.Chapter) []*models.Chapter {
		out := make([]*models.Chapter, 0, len(level))
		for _, ch := range level {
			c := *ch
			if ch.Href != nil {
				if href, ok := kf8.Href(*ch.Href); ok {
					c.Href = &href
				}
			}
			c.Children = convert(ch.Children)
			out = append(out, &c)
		}
		return out
	}
	return convert(models.ChapterTree(stored))
}

// kindleEPUBArchive returns an EPUB of the KF8 book's documents, styles,
// table of contents, and cover, with an OPF holding the metadata the file
// carries. Only the cover of its images is in it.
func kindleEPUBArchive(kf8 *mobi.KF8) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, method uint16, content []byte) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = w.Write(content)
		return errors.WithStack(err)
	}
	if err := add("mimetype", zip.Store, []byte(epubMimetype)); err != nil {
		return nil, err
	}
	if err := add("META-INF/container.xml", zip.Deflate, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="`+kindleEPUBDir+`/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>
`)); err != nil {
		return nil, err
	}
	nav, ncx := kindleTOC(kf8)
	files := []mobi.Document{{Name: "content.opf", Content: kindleOPF(kf8)}, {Name: "nav.xhtml", Content: nav}, {Name: "toc.ncx", Content: ncx}}
	files = append(append(files, kf8.Styles...), kf8.Documents...)
	for _, f := range files {
		if err := add(path.Join(kindleEPUBDir, f.Name), zip.Deflate, f.Content); err != nil {
			return nil, err
		}
	}
	if kf8.Cover != nil {
		data, err := kf8.ReadResource(kf8.Cover)
		if err != nil {
			return nil, err
		}
		if err := add(path.Join(kindleEPUBDir, kf8.Cover.Name), zip.Store, data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, errors.WithStack(err)
	}
	return buf.Bytes(), nil
}

// kindleIdentifier returns the identifier of the converted EPUB, a UUID
// derived from the book's documents, so converting the same file twice
// gives the same identifier.
func kindleIdentifier(kf8 *mobi.KF8) string {
	sum := sha256.New()
	for _, d := range kf8.Documents {
		sum.Write(d.Content)
	}
	d := sum.Sum(nil)
	d[6] = d[6]&0x0F | 0x50 // a name-based UUID
	d[8] = d[8]&0x3F | 0x80
	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", d[0:4], d[4:6], d[6:8], d[8:10], d[10:16])
}

// kindleOPF returns an EPUB 3 package document for the KF8 book.
func kindleOPF(kf8 *mobi.KF8) []byte {
	meta := kf8.Metadata
	language := "und"
	if meta.Language != nil && *meta.Language != "" {
		language = *meta.Language
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="bookid">` + kindleIdentifier(kf8) + `</dc:identifier>
`)
	element := func(name, value string) {
		if value != "" {
			b.WriteString("    <dc:" + name + ">" + html.EscapeString(value) + "</dc:" + name + ">\n")
		}
	}
	element("title", meta.Title)
	for _, a := range meta.Authors {
		element("creator", a.Name)
	}
	element("language", language)
	element("publisher", meta.Publisher)
	element("description", meta.Description)
	if meta.ReleaseDate != nil {
		element("date", meta.ReleaseDate.Format("2006-01-02"))
	}
	for _, id := range meta.Identifiers {
		element("identifier", id.Value)
	}
	for _, g := range meta.Genres {
		element("subject", g)
	}
	if kf8.Cover != nil {
		b.WriteString(`    <meta name="cover" content="cover-image"/>` + "\n")
	}
	b.WriteString("  </metadata>\n  <manifest>\n")
	item := func(id, href, mediaType, properties string) {
		b.WriteString(`    <item id="` + id + `" href="` + html.EscapeString(href) + `" media-type="` + mediaType + `"`)
		if properties != "" {
			b.WriteString(` properties="` + properties + `"`)
		}
		b.WriteString("/>\n")
	}
	item("nav", "nav.xhtml", "application/xhtml+xml", "nav")
	item("ncx", "toc.ncx", "application/x-dtbncx+xml", "")
	for i, d := range kf8.Styles {
		item(fmt.Sprintf("flow%d", i+1), d.Name, d.MediaType, "")
	}
	for i, d := range kf8.Documents {
		properties := ""
		if bytes.Contains(d.Content, []byte("<svg")) {
			properties = "svg"
		}
		item(fmt.Sprintf("part%d", i), d.Name, d.MediaType, properties)
	}
	for i, r := range kf8.Resources {
		if r == kf8.Cover {
			item("cover-image", r.Name, r.MediaType, "cover-image")
		} else {
			item(fmt.Sprintf("image%d", i), r.Name, r.MediaType, "")
		}
	}
	b.WriteString("  </manifest>\n  <spine toc=\"ncx\">\n")
	for i := range kf8.Documents {
		b.WriteString(fmt.Sprintf("    <itemref idref=\"part%d\"/>\n", i))
	}
	b.WriteString("  </spine>\n</package>\n")
	return []byte(b.String())
}

// kindleTOC returns the nav document and the NCX for the book's table of
// contents. A book with none gets an entry per document.
func kindleTOC(kf8 *mobi.KF8) ([]byte, []byte) {
	points := kf8.TOC
	if len(points) == 0 {
		for i, d := range kf8.Documents {
			points = append(points, &mobi.NavPoint{Title: fmt.Sprintf("Section %d", i+1), Href: d.Name})
		}
	}
	title := html.EscapeString(kf8.Metadata.Title)

	var nav strings.Builder
	nav.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>` + title + `</title></head>
<body>
<nav epub:type="toc" id="toc">
`)
	var navList func([]*mobi.NavPoint, string)
	navList = func(points []*mobi.NavPoint, indent string) {
		nav.WriteString(indent + "<ol>\n")
		for _, p := range points {
			nav.WriteString(indent + `  <li><a href="` + html.EscapeString(p.Href) + `">` + html.EscapeString(p.Title) + "</a>")
			if len(p.Children) > 0 {
				nav.WriteString("\n")
				navList(p.Children, indent+"    ")
				nav.WriteString(indent + "  ")
			}
			nav.WriteString("</li>\n")
		}
		nav.WriteString(indent + "</ol>\n")
	}
	navList(points, "")
	nav.WriteString("</nav>\n</body>\n</html>\n")

	var ncx strings.Builder
	ncx.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
<head><meta name="dtb:uid" content="` + kindleIdentifier(kf8) + `"/></head>
<docTitle><text>` + title + `</text></docTitle>
<navMap>
`)
	order := 0
	var navMap func([]*mobi.NavPoint)
	navMap = func(points []*mobi.NavPoint) {
		for _, p := range points {
			order++
			ncx.WriteString(fmt.Sprintf(`<navPoint id="np%d" playOrder="%d"><navLabel><text>%s</text></navLabel><content src="%s"/>`,
				order, order, html.EscapeString(p.Title), html.EscapeString(p.Href)))
			navMap(p.Children)
			ncx.WriteString("</navPoint>\n")
		}
	}
	navMap(points)
	ncx.WriteString("</navMap>\n</ncx>\n")
	return []byte(nav.String()), []byte(ncx.String())
}
