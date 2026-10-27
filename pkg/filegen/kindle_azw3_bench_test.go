package filegen_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/require"
)

// Large-book sizes: 60 chapters of about 25 KB of text each, about the
// length of a 600-page novel, and 200 photos of about 600 KB. The cover is a
// PNG, which an AZW3 carries a JPEG copy of, so the build re-encodes it.
const (
	largeBookChapters = 60
	largeBookImages   = 200
)

// writeLargeBookEPUB writes an EPUB the size of a long illustrated book:
// chapters with footnotes and links between them, a stylesheet, and many
// photos.
func writeLargeBookEPUB(b *testing.B, path string) {
	b.Helper()
	rng := rand.New(rand.NewPCG(7, 8)) //nolint:gosec // test data
	img := image.NewRGBA(image.Rect(0, 0, 1000, 1400))
	for y := range 1400 {
		for x := range 1000 {
			img.Set(x, y, color.RGBA{uint8(rng.UintN(256)), uint8(x), uint8(y), 255}) //nolint:gosec // test data
		}
	}
	var photo, cover bytes.Buffer
	require.NoError(b, jpeg.Encode(&photo, img, &jpeg.Options{Quality: 80}))
	require.NoError(b, png.Encode(&cover, img))

	var manifest, spine, nav strings.Builder
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, data []byte, method uint16) {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		require.NoError(b, err)
		_, err = w.Write(data)
		require.NoError(b, err)
	}
	add("mimetype", []byte("application/epub+zip"), zip.Store)
	add("META-INF/container.xml", []byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`), zip.Deflate)
	add("OEBPS/style.css", []byte(`p { margin: 0; text-indent: 1em; } img { max-width: 100%; } .note { font-size: 0.8em; }`), zip.Deflate)
	add("OEBPS/images/cover.png", cover.Bytes(), zip.Store)
	manifest.WriteString(`<item id="cover" href="images/cover.png" media-type="image/png" properties="cover-image"/>`)
	for i := range largeBookImages {
		add(fmt.Sprintf("OEBPS/images/p%03d.jpg", i), photo.Bytes(), zip.Store)
		fmt.Fprintf(&manifest, `<item id="i%d" href="images/p%03d.jpg" media-type="image/jpeg"/>`, i, i)
	}
	for c := range largeBookChapters {
		var doc strings.Builder
		fmt.Fprintf(&doc, `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>Chapter %d</title><link rel="stylesheet" href="style.css"/></head>
<body><h1 id="c%d">Chapter %d</h1>
`, c, c, c)
		for p := range 120 {
			fmt.Fprintf(&doc, `<p id="p%d">Paragraph %d of chapter %d tells more of the story, with enough words in it to fill a few lines of a page.`, p, p, c)
			if p%20 == 0 {
				fmt.Fprintf(&doc, `<a epub:type="noteref" href="ch%03d.xhtml#n%d">%d</a> See <a href="ch%03d.xhtml#c%d">the next chapter</a>.`, c, p, p, (c+1)%largeBookChapters, (c+1)%largeBookChapters)
			}
			doc.WriteString("</p>\n")
			if img := c*largeBookImages/largeBookChapters + p/40; p%40 == 39 && img < largeBookImages {
				fmt.Fprintf(&doc, `<p><img src="images/p%03d.jpg" alt=""/></p>`+"\n", img)
			}
		}
		for p := 0; p < 120; p += 20 {
			fmt.Fprintf(&doc, `<aside id="n%d" epub:type="footnote" class="note">Note %d.</aside>`+"\n", p, p)
		}
		doc.WriteString("</body></html>")
		add(fmt.Sprintf("OEBPS/ch%03d.xhtml", c), []byte(doc.String()), zip.Deflate)
		fmt.Fprintf(&manifest, `<item id="c%d" href="ch%03d.xhtml" media-type="application/xhtml+xml"/>`, c, c)
		fmt.Fprintf(&spine, `<itemref idref="c%d"/>`, c)
		fmt.Fprintf(&nav, `<li><a href="ch%03d.xhtml#c%d">Chapter %d</a></li>`, c, c, c)
	}
	add("OEBPS/nav.xhtml", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>Contents</title></head><body><nav epub:type="toc"><ol>`+nav.String()+`</ol></nav></body></html>`), zip.Deflate)
	add("OEBPS/content.opf", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Large Book</dc:title><dc:identifier id="id">urn:uuid:large-book</dc:identifier><dc:language>en</dc:language></metadata>
<manifest><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="css" href="style.css" media-type="text/css"/>`+manifest.String()+`</manifest><spine>`+spine.String()+`</spine></package>`), zip.Deflate)
	require.NoError(b, zw.Close())
	require.NoError(b, os.WriteFile(path, buf.Bytes(), 0o600))
}

// BenchmarkEPUBToAZW3_LargeBook times building the AZW3 of a long
// illustrated book, the wait before a Kindle's browser gets its first byte
// on a first download. Run it on the hardware to measure with
//
//	go test ./pkg/filegen/ -run '^$' -bench EPUBToAZW3_LargeBook -benchtime 5x
func BenchmarkEPUBToAZW3_LargeBook(b *testing.B) {
	dir := b.TempDir()
	src := filepath.Join(dir, "large.epub")
	writeLargeBookEPUB(b, src)
	info, err := os.Stat(src)
	require.NoError(b, err)
	gen, err := filegen.GeneratorFor(models.FileTypeEPUB, models.FileTypeAZW3)
	require.NoError(b, err)
	dest := filepath.Join(dir, "large.azw3")
	b.SetBytes(info.Size())
	for b.Loop() {
		require.NoError(b, gen.Generate(context.Background(), src, dest, &models.Book{Title: "Large Book"}, &models.File{ID: 1, FileType: models.FileTypeEPUB}))
	}
	out, err := os.Stat(dest)
	require.NoError(b, err)
	b.ReportMetric(float64(out.Size())/(1<<20), "MB-out")
}
