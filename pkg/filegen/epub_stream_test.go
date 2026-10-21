package filegen_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/filegen/filegentest"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/ziputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamEPUBEntry is an entry of a fixture EPUB.
type streamEPUBEntry struct {
	name    string
	content []byte
	method  uint16
}

// streamEPUBEntries is a small EPUB 3 book with a nav, an NCX, a cover, and
// a large incompressible image, so preparation reads well under half of it.
func streamEPUBEntries() []streamEPUBEntry {
	plate := make([]byte, 512*1024)
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // test data
	for i := range plate {
		plate[i] = byte(rng.UintN(256))
	}
	chapter := func(n string) []byte {
		return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter ` + n + `</title></head>
<body><h1>Chapter ` + n + `</h1><p>Text of chapter ` + n + `. It has two sentences.</p><img src="images/plate.png"/></body>
</html>`)
	}
	return []streamEPUBEntry{
		{name: "mimetype", content: []byte("application/epub+zip"), method: zip.Store},
		{name: "META-INF/container.xml", method: zip.Deflate, content: []byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`)},
		{name: "OEBPS/content.opf", method: zip.Deflate, content: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Source Title</dc:title>
    <dc:creator id="creator">Source Author</dc:creator>
    <dc:identifier id="bookid">urn:uuid:stream-book</dc:identifier>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="cover" href="images/cover.png" media-type="image/png" properties="cover-image"/>
    <item id="plate" href="images/plate.png" media-type="image/png"/>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="ch2" href="chapter2.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="ncx">
    <itemref idref="ch1"/>
    <itemref idref="ch2"/>
  </spine>
</package>`)},
		{name: "OEBPS/nav.xhtml", method: zip.Deflate, content: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Contents</title></head>
<body><nav epub:type="toc"><ol>
<li><a href="chapter1.xhtml">Old One</a></li>
<li><a href="chapter2.xhtml">Old Two</a></li>
</ol></nav></body>
</html>`)},
		{name: "OEBPS/toc.ncx", method: zip.Deflate, content: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
<head><meta name="dtb:uid" content="urn:uuid:stream-book"/></head>
<docTitle><text>Source Title</text></docTitle>
<navMap>
<navPoint id="n1" playOrder="1"><navLabel><text>Old One</text></navLabel><content src="chapter1.xhtml"/></navPoint>
<navPoint id="n2" playOrder="2"><navLabel><text>Old Two</text></navLabel><content src="chapter2.xhtml"/></navPoint>
</navMap>
</ncx>`)},
		{name: "OEBPS/images/cover.png", method: zip.Store, content: []byte("old cover bytes")},
		{name: "OEBPS/images/plate.png", method: zip.Store, content: plate},
		{name: "OEBPS/chapter1.xhtml", method: zip.Deflate, content: chapter("1")},
		{name: "OEBPS/chapter2.xhtml", method: zip.Deflate, content: chapter("2")},
	}
}

// writeStreamEPUB writes entries as a zip at path.
func writeStreamEPUB(t *testing.T, path string, entries []streamEPUBEntry) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: e.method})
		require.NoError(t, err)
		_, err = w.Write(e.content)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))
}

// streamEPUBBook returns the book and file a download is generated for: new
// metadata, a cover on disk, and chapters, so the OPF, cover, nav, and NCX
// are all rewritten.
func streamEPUBBook(t *testing.T, srcPath string) (*models.Book, *models.File) {
	t.Helper()
	coverName := filepath.Base(srcPath) + ".cover.png"
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(srcPath), coverName), []byte("new cover bytes"), 0o600))
	book := &models.Book{
		Title: "Streamed Title",
		Authors: []*models.Author{
			{SortOrder: 0, Person: &models.Person{Name: "New Author", SortName: "Author, New"}},
		},
		BookSeries: []*models.BookSeries{
			{SortOrder: 0, SeriesNumber: pointerutil.Float64(2), Series: &models.Series{Name: "Stream Series"}},
		},
	}
	file := &models.File{
		FileType:           models.FileTypeEPUB,
		Filepath:           srcPath,
		CoverImageFilename: &coverName,
		CoverMimeType:      pointerutil.String("image/png"),
		Chapters: []*models.Chapter{
			{SortOrder: 0, Title: "First", Href: pointerutil.String("chapter1.xhtml")},
			{SortOrder: 1, Title: "Second", Href: pointerutil.String("chapter2.xhtml")},
		},
	}
	return book, file
}

// checkStreamedEPUB reads output back through the EPUB parser and checks it
// carries the book's metadata, cover, and table of contents, with every
// unchanged entry as it was in the source.
func checkStreamedEPUB(t *testing.T, output []byte, kepub bool) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out.epub")
	require.NoError(t, os.WriteFile(path, output, 0o600))

	parsed, err := epub.Parse(path)
	require.NoError(t, err)
	assert.Equal(t, "Streamed Title", parsed.Title)
	require.Len(t, parsed.Authors, 1)
	assert.Equal(t, "New Author", parsed.Authors[0].Name)
	assert.Equal(t, "Stream Series", parsed.Series)
	assert.Equal(t, []byte("new cover bytes"), parsed.CoverData)
	var titles []string
	for _, ch := range parsed.Chapters {
		titles = append(titles, ch.Title)
	}
	assert.Equal(t, []string{"First", "Second"}, titles)

	zr, err := zip.NewReader(bytes.NewReader(output), int64(len(output)))
	require.NoError(t, err)
	require.NotEmpty(t, zr.File)
	assert.Equal(t, "mimetype", zr.File[0].Name)
	assert.Equal(t, zip.Store, zr.File[0].Method)

	want := map[string][]byte{}
	for _, e := range streamEPUBEntries() {
		want[e.name] = e.content
	}
	for _, f := range zr.File {
		data := readZipEntry(t, f)
		switch {
		case f.Name == "OEBPS/images/plate.png" || f.Name == "META-INF/container.xml":
			assert.Equal(t, want[f.Name], data, f.Name)
		case kepub && strings.HasPrefix(f.Name, "OEBPS/chapter"):
			assert.Contains(t, string(data), `class="koboSpan"`, f.Name)
			assert.Contains(t, string(data), `../kobo.js`, f.Name)
		case !kepub && strings.HasPrefix(f.Name, "OEBPS/chapter"):
			assert.Equal(t, want[f.Name], data, f.Name)
		}
	}
	if kepub {
		assert.Contains(t, string(readZipEntry(t, findZipEntry(t, zr, "kobo.js"))), "function paginate")
		// The KePub OPF transform ran on the rewritten OPF: Shisho's title
		// survives it, and the cover keeps its cover-image property.
		opf := string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/content.opf")))
		assert.Contains(t, opf, "Streamed Title")
		assert.Regexp(t, `<item[^>]+id="cover"[^>]+properties="[^"]*cover-image`, opf)
	}
}

func readZipEntry(t *testing.T, f *zip.File) []byte {
	t.Helper()
	r, err := f.Open()
	require.NoError(t, err)
	defer r.Close()
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	return data
}

func findZipEntry(t *testing.T, zr *zip.Reader, name string) *zip.File {
	t.Helper()
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	require.Failf(t, "entry not found", "%s", name)
	return nil
}

func streamConformanceCase(t *testing.T, gen filegen.StreamGenerator, kepub bool) filegentest.Case {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "source.epub")
	writeStreamEPUB(t, src, streamEPUBEntries())
	book, file := streamEPUBBook(t, src)
	broken := filepath.Join(dir, "broken.epub")
	writeStreamEPUB(t, broken, []streamEPUBEntry{
		{name: "mimetype", content: []byte("application/epub+zip"), method: zip.Store},
		{name: "chapter1.xhtml", content: []byte("<html/>"), method: zip.Deflate},
	})
	return filegentest.Case{
		Generator: gen,
		Source:    src,
		Book:      book,
		File:      file,
		Check:     func(t *testing.T, output []byte) { checkStreamedEPUB(t, output, kepub) },
		Broken:    broken,
	}
}

func TestEPUBGenerator_Conformance(t *testing.T) {
	t.Parallel()
	filegentest.Run(t, streamConformanceCase(t, &filegen.EPUBGenerator{}, false))
}

func TestKepubEPUBGenerator_Conformance(t *testing.T) {
	t.Parallel()
	filegentest.Run(t, streamConformanceCase(t, conversionGenerator(t, models.FileTypeEPUB, models.FileTypeKePub), true))
}

// A KePub delivered as itself goes through the EPUB generator, which
// rewrites only the OPF, nav, NCX, and cover, so the Kobo markup the KePub
// writer added survives and the book's metadata reaches the device.
func TestKePubGenerator_Conformance(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.epub")
	writeStreamEPUB(t, plain, streamEPUBEntries())
	kepubBytes, _, _ := prepareAndWrite(t, conversionGenerator(t, models.FileTypeEPUB, models.FileTypeKePub), plain,
		&models.Book{Title: "Original Title"}, &models.File{FileType: models.FileTypeEPUB, Filepath: plain})
	src := filepath.Join(dir, "source.kepub.epub")
	require.NoError(t, os.WriteFile(src, kepubBytes, 0o600))
	srcZip, err := zip.NewReader(bytes.NewReader(kepubBytes), int64(len(kepubBytes)))
	require.NoError(t, err)

	book, file := streamEPUBBook(t, src)
	file.FileType = models.FileTypeKePub
	broken := filepath.Join(dir, "broken.kepub.epub")
	writeStreamEPUB(t, broken, []streamEPUBEntry{
		{name: "mimetype", content: []byte("application/epub+zip"), method: zip.Store},
		{name: "chapter1.xhtml", content: []byte("<html/>"), method: zip.Deflate},
	})

	gen := streamGenerator(t, models.FileTypeKePub)
	assert.Equal(t, models.FileTypeKePub, gen.(filegen.Generator).SupportedType())
	filegentest.Run(t, filegentest.Case{
		Generator: gen,
		Source:    src,
		Book:      book,
		File:      file,
		Broken:    broken,
		Check: func(t *testing.T, output []byte) {
			path := filepath.Join(t.TempDir(), "out.kepub.epub")
			require.NoError(t, os.WriteFile(path, output, 0o600))
			parsed, err := epub.Parse(path)
			require.NoError(t, err)
			assert.Equal(t, "Streamed Title", parsed.Title)
			require.Len(t, parsed.Authors, 1)
			assert.Equal(t, "New Author", parsed.Authors[0].Name)
			assert.Equal(t, []byte("new cover bytes"), parsed.CoverData)

			zr, err := zip.NewReader(bytes.NewReader(output), int64(len(output)))
			require.NoError(t, err)
			for _, name := range []string{"OEBPS/chapter1.xhtml", "OEBPS/chapter2.xhtml", "kobo.js"} {
				data := readZipEntry(t, findZipEntry(t, zr, name))
				assert.Equal(t, readZipEntry(t, findZipEntry(t, srcZip, name)), data, "%s is copied as it was", name)
			}
			assert.Contains(t, string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/chapter1.xhtml"))), `class="koboSpan"`)
			opf := string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/content.opf")))
			assert.Regexp(t, `<item[^>]+href="[^"]*kobo\.js"`, opf, "the kobo.js manifest item is kept")
			assert.Regexp(t, `<item[^>]+properties="[^"]*scripted`, opf, "content documents stay scripted")
		},
	})
}

// prepareAndWrite runs gen over the EPUB at path.
func prepareAndWrite(t *testing.T, gen filegen.StreamGenerator, path string, book *models.Book, file *models.File) (out []byte, size int64, known bool) {
	t.Helper()
	src, err := filegen.OpenSource(path)
	require.NoError(t, err)
	defer src.Close()
	prepared, err := gen.Prepare(context.Background(), src, book, file)
	require.NoError(t, err)
	size, known = prepared.Size()
	var buf bytes.Buffer
	require.NoError(t, prepared.Write(context.Background(), &buf))
	return buf.Bytes(), size, known
}

// The mimetype entry comes first and uncompressed, wherever the source had
// it, and is added when the source has none.
func TestEPUBGenerator_MimetypeFirst(t *testing.T) {
	t.Parallel()
	moved := streamEPUBEntries()
	moved = append(moved[1:4], append([]streamEPUBEntry{{name: "mimetype", content: []byte("application/epub+zip"), method: zip.Deflate}}, moved[4:]...)...)
	missing := streamEPUBEntries()[1:]

	for name, entries := range map[string][]streamEPUBEntry{"elsewhere": moved, "missing": missing} {
		for genName, gen := range map[string]filegen.StreamGenerator{"epub": &filegen.EPUBGenerator{}, "kepub": conversionGenerator(t, models.FileTypeEPUB, models.FileTypeKePub)} {
			t.Run(genName+"/"+name, func(t *testing.T) {
				t.Parallel()
				src := filepath.Join(t.TempDir(), "source.epub")
				writeStreamEPUB(t, src, entries)
				book, file := streamEPUBBook(t, src)
				out, _, _ := prepareAndWrite(t, gen, src, book, file)

				// A local header at offset 0 naming mimetype, stored, with no
				// extra field, so the bytes at 30..58 read
				// "mimetypeapplication/epub+zip".
				require.Greater(t, len(out), 58)
				assert.Equal(t, uint32(0x04034b50), binary.LittleEndian.Uint32(out[0:4]))
				assert.Equal(t, zip.Store, binary.LittleEndian.Uint16(out[8:10]))
				assert.Equal(t, uint16(0), binary.LittleEndian.Uint16(out[28:30]), "extra field length")
				assert.Equal(t, "mimetypeapplication/epub+zip", string(out[30:58]))

				zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
				require.NoError(t, err)
				count := 0
				for _, f := range zr.File {
					if f.Name == "mimetype" {
						count++
					}
				}
				assert.Equal(t, 1, count, "mimetype written once")
			})
		}
	}
}

// The reported size equals the bytes written when the source's entries use
// data descriptors or carry ZIP64 extra fields, and every entry still reads
// back intact.
func TestEPUBGenerator_SizeMatchesForDataDescriptorAndZIP64Sources(t *testing.T) {
	t.Parallel()
	for name, zip64 := range map[string]bool{"data descriptors": false, "zip64": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := filepath.Join(t.TempDir(), "source.epub")
			require.NoError(t, os.WriteFile(src, rawZip(t, streamEPUBEntries(), zip64), 0o600))
			check, err := zip.OpenReader(src)
			require.NoError(t, err)
			require.NotZero(t, check.File[1].Flags&0x8, "fixture entries use data descriptors")
			require.NoError(t, check.Close())

			book, file := streamEPUBBook(t, src)
			out, size, known := prepareAndWrite(t, &filegen.EPUBGenerator{}, src, book, file)
			require.True(t, known)
			assert.Equal(t, int64(len(out)), size)
			checkStreamedEPUB(t, out, false)
		})
	}
}

// rawZip builds a zip by hand whose entries (all but mimetype) use data
// descriptors, and with zip64 set also carry ZIP64 extra fields with the
// sizes and offset, as some tools write even for small files.
func rawZip(t *testing.T, entries []streamEPUBEntry, zip64 bool) []byte {
	t.Helper()
	var out, central bytes.Buffer
	le := binary.LittleEndian
	u16 := func(b *bytes.Buffer, v uint16) { _ = binary.Write(b, le, v) }
	u32 := func(b *bytes.Buffer, v uint32) { _ = binary.Write(b, le, v) }
	u64 := func(b *bytes.Buffer, v uint64) { _ = binary.Write(b, le, v) }
	for _, e := range entries {
		compressed, err := ziputil.Compress(e.name, e.method, e.content)
		require.NoError(t, err)
		entry := compressed.Data
		offset := uint64(out.Len())
		descriptor := e.name != "mimetype"
		var flags uint16
		if descriptor {
			flags = 0x8
		}
		crc := crc32.ChecksumIEEE(e.content)
		var localExtra, centralExtra bytes.Buffer
		if zip64 {
			u16(&localExtra, 0x0001)
			u16(&localExtra, 16)
			u64(&localExtra, uint64(len(e.content)))
			u64(&localExtra, uint64(len(entry)))
			u16(&centralExtra, 0x0001)
			u16(&centralExtra, 24)
			u64(&centralExtra, uint64(len(e.content)))
			u64(&centralExtra, uint64(len(entry)))
			u64(&centralExtra, offset)
		}
		sizes := func(b *bytes.Buffer, local bool) {
			switch {
			case zip64:
				u32(b, 0xffffffff)
				u32(b, 0xffffffff)
			case local && descriptor:
				u32(b, 0)
				u32(b, 0)
			default:
				u32(b, uint32(len(entry)))
				u32(b, uint32(len(e.content)))
			}
		}

		u32(&out, 0x04034b50)
		u16(&out, 45)
		u16(&out, flags)
		u16(&out, e.method)
		u32(&out, 0) // time and date
		if descriptor {
			u32(&out, 0)
		} else {
			u32(&out, crc)
		}
		sizes(&out, true)
		u16(&out, uint16(len(e.name)))
		u16(&out, uint16(localExtra.Len()))
		out.WriteString(e.name)
		out.Write(localExtra.Bytes())
		out.Write(entry)
		if descriptor {
			u32(&out, 0x08074b50)
			u32(&out, crc)
			if zip64 {
				u64(&out, uint64(len(entry)))
				u64(&out, uint64(len(e.content)))
			} else {
				u32(&out, uint32(len(entry)))
				u32(&out, uint32(len(e.content)))
			}
		}

		u32(&central, 0x02014b50)
		u16(&central, 45)
		u16(&central, 45)
		u16(&central, flags)
		u16(&central, e.method)
		u32(&central, 0)
		u32(&central, crc)
		sizes(&central, false)
		u16(&central, uint16(len(e.name)))
		u16(&central, uint16(centralExtra.Len()))
		u16(&central, 0) // comment
		u16(&central, 0) // disk
		u16(&central, 0) // internal attrs
		u32(&central, 0) // external attrs
		if zip64 {
			u32(&central, 0xffffffff)
		} else {
			u32(&central, uint32(offset))
		}
		central.WriteString(e.name)
		central.Write(centralExtra.Bytes())
	}
	dirOffset := out.Len()
	out.Write(central.Bytes())
	u32(&out, 0x06054b50)
	u16(&out, 0)
	u16(&out, 0)
	u16(&out, uint16(len(entries)))
	u16(&out, uint16(len(entries)))
	u32(&out, uint32(central.Len()))
	u32(&out, uint32(dirOffset))
	u16(&out, 0)
	return out.Bytes()
}

// An entry whose declared size runs past the end of the source is rejected
// while preparing, rather than copied short partway through a download.
func TestEPUBGenerators_RejectEntryPastEndOfSource(t *testing.T) {
	t.Parallel()
	for name, gen := range map[string]filegen.StreamGenerator{"epub": &filegen.EPUBGenerator{}, "kepub": conversionGenerator(t, models.FileTypeEPUB, models.FileTypeKePub)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := filepath.Join(t.TempDir(), "source.epub")
			writeStreamEPUB(t, src, streamEPUBEntries())
			data, err := os.ReadFile(src)
			require.NoError(t, err)
			// Raise plate.png's compressed size in the central directory.
			i := bytes.LastIndex(data, []byte("OEBPS/images/plate.png")) - 46
			require.Equal(t, uint32(0x02014b50), binary.LittleEndian.Uint32(data[i:]))
			binary.LittleEndian.PutUint32(data[i+20:], 10<<20)
			require.NoError(t, os.WriteFile(src, data, 0o600))

			book, file := streamEPUBBook(t, src)
			s, err := filegen.OpenSource(src)
			require.NoError(t, err)
			defer s.Close()
			_, err = gen.Prepare(context.Background(), s, book, file)
			require.Error(t, err)
		})
	}
}
