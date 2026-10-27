package filegen_test

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func azw3TestImage(t *testing.T, mimeType string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 30, 40))
	for y := range 40 {
		for x := range 30 {
			img.Set(x, y, color.RGBA{200, uint8(x * 8), uint8(y * 6), 255}) //nolint:gosec // small test values
		}
	}
	var buf bytes.Buffer
	if mimeType == "image/png" {
		require.NoError(t, png.Encode(&buf, img))
	} else {
		require.NoError(t, jpeg.Encode(&buf, img, nil))
	}
	return buf.Bytes()
}

// writeAZW3SourceEPUB writes an EPUB 3 with two chapters, a stylesheet, an
// image in the first chapter, a cover, and a nav document.
func writeAZW3SourceEPUB(t *testing.T, path string) []byte {
	t.Helper()
	return writeAZW3SourceEPUBWithCover(t, path, "cover.jpg")
}

// writeAZW3SourceEPUBWithCover is writeAZW3SourceEPUB with the cover image
// stored under coverName, which the OPF names percent-encoded.
func writeAZW3SourceEPUBWithCover(t *testing.T, path, coverName string) []byte {
	t.Helper()
	pic := azw3TestImage(t, "image/png")
	entries := []struct {
		name string
		data []byte
	}{
		{"mimetype", []byte("application/epub+zip")},
		{"META-INF/container.xml", []byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)},
		{"OEBPS/content.opf", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Title In File</dc:title>
    <dc:creator>Author In File</dc:creator>
    <dc:identifier id="bookid">urn:uuid:azw3-source</dc:identifier>
    <dc:language>en</dc:language>
    <dc:publisher>File Publisher</dc:publisher>
    <meta name="cover" content="cover"/>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="css" href="styles/book.css" media-type="text/css"/>
    <item id="c1" href="text/one.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="text/two.xhtml" media-type="application/xhtml+xml"/>
    <item id="pic" href="images/pic.png" media-type="image/png"/>
    <item id="cover" href="images/` + url.PathEscape(coverName) + `" media-type="image/jpeg" properties="cover-image"/>
  </manifest>
  <spine><itemref idref="c1"/><itemref idref="c2"/></spine>
</package>`)},
		{"OEBPS/nav.xhtml", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>Contents</title></head>
<body><nav epub:type="toc"><ol>
<li><a href="text/one.xhtml">Chapter One</a></li>
<li><a href="text/two.xhtml#second">Chapter Two</a></li>
</ol></nav></body></html>`)},
		{"OEBPS/styles/book.css", []byte(`p.boxed { border: 1px solid black; }`)},
		{"OEBPS/text/one.xhtml", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>One</title><link rel="stylesheet" href="../styles/book.css"/></head>
<body><h1>Chapter One</h1><p class="boxed">This is chapter one.</p><p><img src="../images/pic.png" alt=""/></p></body></html>`)},
		{"OEBPS/text/two.xhtml", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Two</title></head>
<body><h1 id="second">Chapter Two</h1><p>This is chapter two.</p></body></html>`)},
		{"OEBPS/images/pic.png", pic},
		{"OEBPS/images/" + coverName, azw3TestImage(t, "image/jpeg")},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		method := zip.Deflate
		if e.name == "mimetype" {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: method})
		require.NoError(t, err)
		_, err = w.Write(e.data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))
	return pic
}

// azw3Book returns the book and file an AZW3 is generated for: Shisho's
// own title, author, and cover, and the second chapter renamed.
func azw3Book(t *testing.T, srcPath string, fileID int) (*models.Book, *models.File, []byte) {
	t.Helper()
	cover := azw3TestImage(t, "image/jpeg")
	coverName := filepath.Base(srcPath) + ".cover.jpg"
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(srcPath), coverName), cover, 0o600))
	book := &models.Book{
		ID:    fileID,
		Title: "Shisho Title",
		Authors: []*models.Author{
			{SortOrder: 0, Person: &models.Person{Name: "New Author", SortName: "Author, New"}},
		},
	}
	file := &models.File{
		ID:                 fileID,
		BookID:             fileID,
		FileType:           models.FileTypeEPUB,
		Filepath:           srcPath,
		CoverImageFilename: &coverName,
		CoverMimeType:      pointerutil.String("image/jpeg"),
		Chapters: []*models.Chapter{
			{SortOrder: 0, Title: "Chapter One", Href: pointerutil.String("text/one.xhtml")},
			{SortOrder: 1, Title: "The Second", Href: pointerutil.String("text/two.xhtml#second")},
		},
	}
	return book, file, cover
}

// generateAZW3 builds the file delivered as AZW3 and returns its path.
func generateAZW3(t *testing.T, fileType, srcPath string, book *models.Book, file *models.File) string {
	t.Helper()
	gen, err := filegen.GeneratorFor(fileType, models.FileTypeAZW3)
	require.NoError(t, err)
	assert.Equal(t, models.FileTypeAZW3, gen.SupportedType())
	_, streams := gen.(filegen.StreamGenerator)
	assert.False(t, streams, "an AZW3 is built in full before it is sent")
	dest := filepath.Join(t.TempDir(), "out.azw3")
	require.NoError(t, gen.Generate(context.Background(), srcPath, dest, book, file))
	return dest
}

func TestEPUBToAZW3_ReadsBack(t *testing.T) {
	t.Parallel()
	src := filepath.Join(t.TempDir(), "book.epub")
	pic := writeAZW3SourceEPUB(t, src)
	book, file, cover := azw3Book(t, src, 7)
	out := generateAZW3(t, models.FileTypeEPUB, src, book, file)

	meta, err := mobi.Parse(out)
	require.NoError(t, err)
	assert.Equal(t, "Shisho Title", meta.Title)
	require.Len(t, meta.Authors, 1)
	assert.Equal(t, "New Author", meta.Authors[0].Name)
	assert.Equal(t, "File Publisher", meta.Publisher, "a field Shisho has no value for keeps the file's own")
	require.NotNil(t, meta.Language)
	assert.Equal(t, "en", *meta.Language)
	assert.Equal(t, cover, meta.CoverData)
	var titles []string
	for _, ch := range meta.Chapters {
		titles = append(titles, ch.Title)
	}
	assert.Equal(t, []string{"Chapter One", "The Second"}, titles)

	f, err := os.Open(out)
	require.NoError(t, err)
	defer f.Close()
	info, err := f.Stat()
	require.NoError(t, err)
	content, err := mobi.ReadContent(context.Background(), f, info.Size())
	require.NoError(t, err)
	require.Len(t, content.Documents, 2)
	assert.Contains(t, string(content.Documents[0].Content), `<p class="boxed">This is chapter one.</p>`)
	assert.Contains(t, string(content.Documents[0].Content), `href="flow0001.css"`)
	assert.Contains(t, string(content.Documents[1].Content), `This is chapter two.`)
	require.Len(t, content.Styles, 1)
	assert.Contains(t, string(content.Styles[0].Content), "border: 1px solid black")
	var images [][]byte
	for _, r := range content.Resources {
		data, err := content.ReadResource(r)
		require.NoError(t, err)
		images = append(images, data)
	}
	assert.Contains(t, images, pic)
	require.Len(t, content.TOC, 2)
	assert.Equal(t, "part0001.xhtml#second", content.TOC[1].Href)
}

// A KePub, a CBZ, and a MOBI reach AZW3 through EPUB.
func TestConversionChainsToAZW3_ReadBack(t *testing.T) {
	t.Parallel()
	for fileType, sample := range chainSamples(t) {
		if fileType == models.FileTypeEPUB || fileType == models.FileTypeAZW3 {
			continue
		}
		t.Run(fileType, func(t *testing.T) {
			t.Parallel()
			out := generateAZW3(t, fileType, sample, &models.Book{Title: "Chained"}, &models.File{ID: 1, FileType: fileType})
			meta, err := mobi.Parse(out)
			require.NoError(t, err)
			assert.Equal(t, "Chained", meta.Title)
			f, err := os.Open(out)
			require.NoError(t, err)
			defer f.Close()
			info, err := f.Stat()
			require.NoError(t, err)
			content, err := mobi.ReadContent(context.Background(), f, info.Size())
			require.NoError(t, err)
			assert.NotEmpty(t, content.Documents)
		})
	}
}

// An EPUB whose package lists no documents has nothing to convert, and says
// so.
func TestEPUBToAZW3_DeclinesABookWithNoDocuments(t *testing.T) {
	t.Parallel()
	src := filepath.Join(t.TempDir(), "empty.epub")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range map[string]string{
		"mimetype":          "application/epub+zip",
		"OEBPS/content.opf": `<?xml version="1.0"?><package version="3.0" xmlns="http://www.idpf.org/2007/opf"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Empty</dc:title></metadata><manifest/><spine/></package>`,
	} {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(data))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(src, buf.Bytes(), 0o600))

	gen, err := filegen.GeneratorFor(models.FileTypeEPUB, models.FileTypeAZW3)
	require.NoError(t, err)
	err = gen.Generate(context.Background(), src, filepath.Join(t.TempDir(), "out.azw3"), &models.Book{Title: "Empty"}, &models.File{ID: 1, FileType: models.FileTypeEPUB})
	require.ErrorIs(t, err, filegen.ErrUnsupportedContent)
	var genErr *filegen.GenerationError
	require.ErrorAs(t, err, &genErr)
	assert.Equal(t, models.FileTypeAZW3, genErr.FileType)
	assert.Contains(t, genErr.Message, "no documents")
}

// A cover the OPF names with a percent-encoded href is still the cover.
func TestEPUBToAZW3_FindsAnEscapedCover(t *testing.T) {
	t.Parallel()
	src := filepath.Join(t.TempDir(), "book.epub")
	writeAZW3SourceEPUBWithCover(t, src, "cover art.jpg")
	out := generateAZW3(t, models.FileTypeEPUB, src, &models.Book{Title: "Escaped"}, &models.File{ID: 1, FileType: models.FileTypeEPUB})
	meta, err := mobi.Parse(out)
	require.NoError(t, err)
	assert.Equal(t, azw3TestImage(t, "image/jpeg"), meta.CoverData)
}
