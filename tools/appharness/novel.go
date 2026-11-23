package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"

	"github.com/pkg/errors"
)

// novelChapters are the reader's fixture's chapters, in the table of
// contents and the spine, before its notes. The flows go to them by title.
var novelChapters = []string{"Chapter One", "Chapter Two", "Chapter Three"}

// novelParagraphs is how many paragraphs each chapter has: several pages
// on a phone, so the flows turn pages within a chapter and across them.
const novelParagraphs = 14

// writeNovel writes the EPUB the reader flows read: three chapters of a few
// pages each, listed in a navigation document so Shisho keeps them as the
// File's chapters. Chapter Two opens with a note reference marked as one,
// whose note lives in its own file, for the footnote popover. Chapter One
// has a picture of black line art on a transparent background, which the
// dark themes put on white.
func writeNovel(path, title, author, series string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = errors.WithStack(cerr)
		}
	}()
	zw := zip.NewWriter(f)
	defer func() {
		if cerr := zw.Close(); cerr != nil && err == nil {
			err = errors.WithStack(cerr)
		}
	}()

	// The mimetype comes first, uncompressed.
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := w.Write([]byte("application/epub+zip")); err != nil {
		return errors.WithStack(err)
	}

	cover, err := pngBytes(solidImage(color.RGBA{R: 40, G: 70, B: 140, A: 255}))
	if err != nil {
		return err
	}
	lineArt, err := pngBytes(lineArtImage())
	if err != nil {
		return err
	}
	entries := []struct {
		name string
		data []byte
	}{
		{"META-INF/container.xml", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`)},
		{"OEBPS/content.opf", []byte(novelOPF(title, author, series))},
		{"OEBPS/nav.xhtml", []byte(novelNav())},
		{"OEBPS/cover.png", cover},
		{"OEBPS/lineart.png", lineArt},
		{"OEBPS/notes.xhtml", []byte(xhtml("Notes", `<h1>Notes</h1>
<aside epub:type="footnote" id="n1"><p>The footnote's text. <a epub:type="backlink" href="chapter2.xhtml#r1">Back</a></p></aside>`))},
	}
	for i, chapter := range novelChapters {
		var body strings.Builder
		fmt.Fprintf(&body, "<h1 id=\"start\">%s</h1>\n", chapter)
		if i == 0 {
			body.WriteString(`<p><img src="lineart.png" alt="A drawing in black lines"/></p>` + "\n")
		}
		for p := 1; p <= novelParagraphs; p++ {
			note := ""
			if i == 1 && p == 1 {
				note = `<a epub:type="noteref" id="r1" href="notes.xhtml#n1">[1]</a>`
			}
			fmt.Fprintf(&body, "<p>%s, paragraph %d.%s %s</p>\n", chapter, p, note, filler)
		}
		entries = append(entries, struct {
			name string
			data []byte
		}{fmt.Sprintf("OEBPS/chapter%d.xhtml", i+1), []byte(xhtml(chapter, body.String()))})
	}
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			return errors.WithStack(err)
		}
		if _, err := w.Write(e.data); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

const filler = "The lamplighter walked the length of the harbor wall each evening, counting the boats that had come " +
	"home and the ones that had not, and writing both numbers in a small book he kept in his coat."

func xhtml(title, body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>` + title + `</title></head>
<body>
` + body + `
</body>
</html>`
}

func novelOPF(title, author, series string) string {
	var manifest, spine strings.Builder
	for i := range novelChapters {
		fmt.Fprintf(&manifest, "    <item id=\"chapter%d\" href=\"chapter%d.xhtml\" media-type=\"application/xhtml+xml\"/>\n", i+1, i+1)
		fmt.Fprintf(&spine, "    <itemref idref=\"chapter%d\"/>\n", i+1)
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>` + title + `</dc:title>
    <dc:creator opf:role="aut">` + author + `</dc:creator>
    <dc:identifier id="bookid">urn:uuid:harness-novel</dc:identifier>
    <dc:language>en</dc:language>
    <meta name="calibre:series" content="` + series + `"/>
    <meta name="calibre:series_index" content="1.0"/>
    <meta name="cover" content="cover-image"/>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="cover-image" href="cover.png" media-type="image/png" properties="cover-image"/>
    <item id="lineart" href="lineart.png" media-type="image/png"/>
` + manifest.String() + `    <item id="notes" href="notes.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
` + spine.String() + `    <itemref idref="notes"/>
  </spine>
</package>`
}

func novelNav() string {
	var items strings.Builder
	for i, chapter := range novelChapters {
		fmt.Fprintf(&items, "      <li><a href=\"chapter%d.xhtml\">%s</a></li>\n", i+1, chapter)
	}
	return xhtml("Contents", `<nav epub:type="toc" id="toc">
    <h1>Contents</h1>
    <ol>
`+items.String()+`      <li><a href="notes.xhtml">Notes</a></li>
    </ol>
  </nav>`)
}

func solidImage(c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 120, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 120; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// lineArtImage is black lines on a transparent background, which would
// vanish on a dark page without the reader's white behind images.
func lineArtImage() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 120))
	black := color.NRGBA{A: 255}
	for x := 10; x < 190; x++ {
		for _, y := range []int{20, 60, 100} {
			img.Set(x, y, black)
			img.Set(x, y+1, black)
		}
	}
	for y := 20; y < 102; y++ {
		img.Set(10, y, black)
		img.Set(189, y, black)
	}
	return img
}

func pngBytes(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, errors.WithStack(err)
	}
	return buf.Bytes(), nil
}

// pictureBookPages is how many pages the fixed-layout fixture has.
const pictureBookPages = 6

// writePictureBook writes a fixed-layout EPUB, which foliate lays out page
// by page rather than reflowing: each page shows its number large and a
// line naming it, on its own color, and the navigation document lists
// every page so Shisho keeps them as the File's chapters. A portrait phone
// shows one page at a time, starting with the first page alone.
func writePictureBook(path, title, author string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = errors.WithStack(cerr)
		}
	}()
	zw := zip.NewWriter(f)
	defer func() {
		if cerr := zw.Close(); cerr != nil && err == nil {
			err = errors.WithStack(cerr)
		}
	}()
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := w.Write([]byte("application/epub+zip")); err != nil {
		return errors.WithStack(err)
	}
	var manifest, spine, nav strings.Builder
	entries := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
	}
	names := []string{"META-INF/container.xml"}
	for i := 1; i <= pictureBookPages; i++ {
		name := fmt.Sprintf("page%d.xhtml", i)
		fmt.Fprintf(&manifest, "    <item id=\"page%d\" href=\"%s\" media-type=\"application/xhtml+xml\"/>\n", i, name)
		fmt.Fprintf(&spine, "    <itemref idref=\"page%d\"/>\n", i)
		fmt.Fprintf(&nav, "      <li><a href=\"%s\">Page %d</a></li>\n", name, i)
		entries["OEBPS/"+name] = fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Page %[1]d</title><meta name="viewport" content="width=600, height=800"/></head>
<body style="margin:0;width:600px;height:800px;overflow:hidden;background:%[2]s;color:#111;font-family:sans-serif;text-align:center">
<h1 style="font-size:160px;margin:120px 0 40px">%[1]d</h1>
<p style="font-size:32px;margin:0 40px">Fixed page %[1]d of the picture book.</p>
</body>
</html>`, i, pageColor(i))
		names = append(names, "OEBPS/"+name)
	}
	entries["OEBPS/nav.xhtml"] = xhtml("Contents", `<nav epub:type="toc"><ol>
`+nav.String()+`    </ol></nav>`)
	entries["OEBPS/content.opf"] = `<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="bookid" prefix="rendition: http://www.idpf.org/vocab/rendition/#">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>` + title + `</dc:title>
    <dc:creator opf:role="aut">` + author + `</dc:creator>
    <dc:identifier id="bookid">urn:uuid:harness-picture-book</dc:identifier>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2026-01-01T00:00:00Z</meta>
    <meta property="rendition:layout">pre-paginated</meta>
    <meta property="rendition:spread">auto</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
` + manifest.String() + `  </manifest>
  <spine>
` + spine.String() + `  </spine>
</package>`
	names = append(names, "OEBPS/nav.xhtml", "OEBPS/content.opf")
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			return errors.WithStack(err)
		}
		if _, err := w.Write([]byte(entries[name])); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// pageColor is a fixed-layout page's own background, so turns are easy to
// see.
func pageColor(page int) string {
	switch page % 4 {
	case 1:
		return "#fde68a"
	case 2:
		return "#bfdbfe"
	case 3:
		return "#bbf7d0"
	default:
		return "#fbcfe8"
	}
}
