package filegen_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/filegen/filegentest"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kindlePlate is a large incompressible image, so preparation reads well
// under half of a Kindle source that shows it.
func kindlePlate() []byte {
	plate := make([]byte, 512*1024)
	rng := rand.New(rand.NewPCG(3, 4)) //nolint:gosec // test data
	for i := range plate {
		plate[i] = byte(rng.UintN(256))
	}
	copy(plate, "\x89PNG\r\n\x1a\n")
	return plate
}

// kindleSource writes a sample book (testgen.KF8Sample or one built on
// it, or testgen.KF7Sample) showing kindlePlate in place of its image.
func kindleSource(t *testing.T, dir, name string, opts testgen.MOBIOptions) (string, []byte) {
	t.Helper()
	plate := kindlePlate()
	if opts.KF8 != nil {
		opts.KF8.Images = [][]byte{plate}
	} else {
		opts.MOBI6.Images = [][]byte{plate}
	}
	return testgen.GenerateMOBI(t, dir, name, opts), plate
}

func huffCDIC(opts testgen.MOBIOptions) testgen.MOBIOptions {
	opts.Compression = testgen.MOBICompressionHuffCDIC
	return opts
}

// kindleBook returns the book and file a download is generated for: new
// metadata and a cover on disk, and chapters stored the way a scan stores
// them, by location, with the second renamed.
func kindleBook(t *testing.T, srcPath, fileType string) (*models.Book, *models.File) {
	t.Helper()
	book, file := streamEPUBBook(t, srcPath)
	file.FileType = fileType
	data, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	tocs, err := mobi.ReadTOC(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	entries := tocs[0].Entries
	require.Len(t, entries, 2)
	file.Chapters = []*models.Chapter{
		{SortOrder: 0, Title: entries[0].Title, Href: pointerutil.String(entries[0].Locator), Children: []*models.Chapter{
			{SortOrder: 0, Title: entries[0].Children[0].Title, Href: pointerutil.String(entries[0].Children[0].Locator)},
		}},
		{SortOrder: 1, Title: "The Second", Href: pointerutil.String(entries[1].Locator)},
	}
	return book, file
}

func TestKindleToEPUB_Conformance(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		fileType string
		opts     testgen.MOBIOptions
		check    func(t *testing.T, output, plate []byte)
	}{
		"AZW3":           {models.FileTypeAZW3, testgen.KF8Sample(testgen.MOBIKindKF8), checkKindleEPUB},
		"combo MOBI":     {models.FileTypeMOBI, testgen.KF8Sample(testgen.MOBIKindCombo), checkKindleEPUB},
		"HUFF/CDIC AZW3": {models.FileTypeAZW3, huffCDIC(testgen.KF8Sample(testgen.MOBIKindKF8)), checkKindleEPUB},
		"embedded fonts": {models.FileTypeAZW3, testgen.KF8FontSample(testgen.MOBIKindKF8), checkFontEPUB},
		"KF7 MOBI":       {models.FileTypeMOBI, testgen.KF7Sample(), checkKF7EPUB},
		"HUFF/CDIC KF7":  {models.FileTypeMOBI, huffCDIC(testgen.KF7Sample()), checkKF7EPUB},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			src, plate := kindleSource(t, dir, "source."+c.fileType, c.opts)
			book, file := kindleBook(t, src, c.fileType)
			broken := testgen.GenerateMOBI(t, dir, "broken."+c.fileType, testgen.MOBIOptions{Kind: testgen.MOBIKindMOBI6, Title: "Locked", Encrypted: true})

			gen := conversionGenerator(t, c.fileType, models.FileTypeEPUB)
			assert.Equal(t, models.FileTypeEPUB, gen.(filegen.Generator).SupportedType())
			filegentest.Run(t, filegentest.Case{
				Generator: gen,
				Source:    src,
				Book:      book,
				File:      file,
				Broken:    broken,
				Check:     func(t *testing.T, output []byte) { c.check(t, output, plate) },
			})
		})
	}
}

// checkConvertedKindle checks what every converted EPUB has: Shisho's
// metadata, the stored chapters with their renamed title, well-formed
// documents, links that resolve, no kindle: reference left, and no
// epubcheck error when epubcheck is installed. It returns the parsed
// chapters and the archive.
func checkConvertedKindle(t *testing.T, output []byte) ([]mediafile.ParsedChapter, *zip.Reader) {
	t.Helper()
	outPath := filepath.Join(t.TempDir(), "out.epub")
	require.NoError(t, os.WriteFile(outPath, output, 0o600))
	parsed, err := epub.Parse(outPath)
	require.NoError(t, err)
	assert.Equal(t, "Streamed Title", parsed.Title)
	require.Len(t, parsed.Authors, 1)
	assert.Equal(t, "New Author", parsed.Authors[0].Name)
	assert.Equal(t, "Stream Series", parsed.Series)
	require.NotNil(t, parsed.Language)
	assert.Equal(t, "en", *parsed.Language, "the source's language is kept")
	assert.Equal(t, []byte("new cover bytes"), parsed.CoverData)
	require.Len(t, parsed.Chapters, 2)
	assert.Equal(t, "Chapter One", parsed.Chapters[0].Title)
	require.Len(t, parsed.Chapters[0].Children, 1)
	assert.Equal(t, "The Second", parsed.Chapters[1].Title, "a renamed chapter keeps its new title")

	zr, err := zip.NewReader(bytes.NewReader(output), int64(len(output)))
	require.NoError(t, err)
	assert.Equal(t, "mimetype", zr.File[0].Name)
	assert.Equal(t, zip.Store, zr.File[0].Method)
	for _, f := range zr.File {
		content := readZipEntry(t, f)
		assert.NotRegexp(t, `["(]kindle:`, string(content), f.Name)
		if strings.HasSuffix(f.Name, ".xhtml") || strings.HasSuffix(f.Name, ".opf") || strings.HasSuffix(f.Name, ".ncx") {
			assertWellFormedXML(t, f.Name, content)
		}
	}
	assertEPUBLinksResolve(t, zr)
	assertKindleEPUBCheckClean(t, outPath)
	return parsed.Chapters, zr
}

// assertKindleEPUBCheckClean runs epubcheck on a converted EPUB when it is
// installed and fails on any error, as assertEPUBCheckClean does for the
// EPUB generator. The fixtures' images are a few bytes of magic number, and
// the cover a few bytes of text, so the errors saying an image is corrupt
// or not its media type are left out.
func assertKindleEPUBCheckClean(t *testing.T, path string) {
	t.Helper()
	if _, err := exec.LookPath("epubcheck"); err != nil {
		return
	}
	out, _ := exec.CommandContext(t.Context(), "epubcheck", path).CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "ERROR(PKG-021)") || strings.HasPrefix(line, "ERROR(OPF-029)") {
			continue
		}
		if strings.HasPrefix(line, "ERROR") || strings.HasPrefix(line, "FATAL") {
			t.Errorf("epubcheck output:\n%s", out)
			return
		}
	}
}

// assertWellFormedXML checks that content parses as XML.
func assertWellFormedXML(t *testing.T, name string, content []byte) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(content))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return
		}
		if !assert.NoError(t, err, "%s is well-formed XML", name) {
			return
		}
	}
}

// checkKindleEPUB checks that an EPUB converted from testgen.KF8Sample
// keeps the source's text, images, and stylesheet.
func checkKindleEPUB(t *testing.T, output, plate []byte) {
	t.Helper()
	chapters, zr := checkConvertedKindle(t, output)
	assert.Equal(t, "part0000.xhtml#ch1", *chapters[0].Href)
	assert.Equal(t, "part0000.xhtml#later", *chapters[0].Children[0].Href)
	assert.Equal(t, "part0001.xhtml#ch2", *chapters[1].Href)

	one := docText(t, readZipEntry(t, findZipEntry(t, zr, "OEBPS/part0000.xhtml")))
	assert.True(t, strings.HasPrefix(one, "Chapter OneRead the next chapter or later on."), one[:80])
	assert.Equal(t, 120, strings.Count(one, "Ünïcödé filler text keeps the chapter long."))
	assert.True(t, strings.HasSuffix(one, "Later in chapter one."))
	two := docText(t, readZipEntry(t, findZipEntry(t, zr, "OEBPS/part0001.xhtml")))
	assert.Equal(t, "Chapter TwoBack to the start. Out to the web. On to the end.A Kindle names images like kindle:embed:0001 inside.The end.", two)

	assert.Equal(t, plate, readZipEntry(t, findZipEntry(t, zr, "OEBPS/image0003.png")), "the image is copied as it was")
	assert.Equal(t, zip.Store, findZipEntry(t, zr, "OEBPS/image0003.png").Method)
	assert.Contains(t, string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/flow0001.css"))), "url(image0003.png)")
	assert.Contains(t, string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/part0000.xhtml"))), `<img src="image0003.png" alt="plate"/>`)

	opf := string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/content.opf")))
	for _, href := range []string{"part0000.xhtml", "part0001.xhtml", "flow0001.css", "image0001.jpg", "image0003.png", "nav.xhtml", "toc.ncx"} {
		assert.Contains(t, opf, `href="`+href+`"`)
	}
}

// checkFontEPUB checks an EPUB converted from testgen.KF8FontSample: what
// checkKindleEPUB checks, and the font copied out, declared in the OPF, and
// named by the stylesheet.
func checkFontEPUB(t *testing.T, output, plate []byte) {
	t.Helper()
	checkKindleEPUB(t, output, plate)
	zr, err := zip.NewReader(bytes.NewReader(output), int64(len(output)))
	require.NoError(t, err)
	assert.Equal(t, testgen.KF8SampleFont, readZipEntry(t, findZipEntry(t, zr, "OEBPS/font0004.ttf")))
	opf := string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/content.opf")))
	assert.Regexp(t, `<item id="[^"]+" href="font0004.ttf" media-type="font/ttf"`, opf)
	assert.Contains(t, string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/flow0001.css"))), `src: url(font0004.ttf);`)
}

// checkKF7EPUB checks an EPUB converted from testgen.KF7Sample: a document
// for each chapter and the printed table of contents, the source's text and
// image, and chapters pointing into the right documents.
func checkKF7EPUB(t *testing.T, output, plate []byte) {
	t.Helper()
	chapters, zr := checkConvertedKindle(t, output)
	assert.Regexp(t, `^part0000\.xhtml#\S+$`, *chapters[0].Href)
	assert.Regexp(t, `^part0000\.xhtml#\S+$`, *chapters[0].Children[0].Href)
	assert.Regexp(t, `^part0001\.xhtml#\S+$`, *chapters[1].Href)

	one := docText(t, readZipEntry(t, findZipEntry(t, zr, "OEBPS/part0000.xhtml")))
	assert.True(t, strings.HasPrefix(one, "Chapter OneRead the next chapter or later on. Café &amp; crème"), one[:80])
	assert.Equal(t, 120, strings.Count(one, "Ünïcödé filler text keeps the chapter long."))
	assert.True(t, strings.HasSuffix(one, "Later in chapter one."))
	two := docText(t, readZipEntry(t, findZipEntry(t, zr, "OEBPS/part0001.xhtml")))
	assert.True(t, strings.HasPrefix(two, "Chapter TwoBack to the start."), two)
	assert.True(t, strings.HasSuffix(two, "The end."), two)
	assert.Equal(t, "Table of ContentsChapter OneChapter Two", docText(t, readZipEntry(t, findZipEntry(t, zr, "OEBPS/part0002.xhtml"))))

	assert.Equal(t, plate, readZipEntry(t, findZipEntry(t, zr, "OEBPS/image0003.png")), "the image is copied as it was")
	assert.Contains(t, string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/part0000.xhtml"))), `<img src="image0003.png"`)
	opf := string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/content.opf")))
	for _, href := range []string{"part0000.xhtml", "part0001.xhtml", "part0002.xhtml", "image0001.jpg", "image0003.png", "nav.xhtml", "toc.ncx"} {
		assert.Contains(t, opf, `href="`+href+`"`)
	}
}

// The identifier of a converted EPUB depends on the book, not on when it
// was converted.
func TestKindleToEPUB_StableIdentifier(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src, _ := kindleSource(t, dir, "book.azw3", testgen.KF8Sample(testgen.MOBIKindKF8))
	gen := conversionGenerator(t, models.FileTypeAZW3, models.FileTypeEPUB)
	file := &models.File{FileType: models.FileTypeAZW3, Filepath: src}
	identifier := func() string {
		out, _, _ := prepareAndWrite(t, gen, src, &models.Book{Title: "Book"}, file)
		zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
		require.NoError(t, err)
		m := regexp.MustCompile(`id="bookid">([^<]+)<`).FindStringSubmatch(string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/content.opf"))))
		require.NotNil(t, m)
		return m[1]
	}
	first := identifier()
	assert.Regexp(t, `^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, first)
	assert.Equal(t, first, identifier())
}

// A file the conversion doesn't support fails before anything is written,
// with an error saying why, never with a broken EPUB.
func TestKindleToEPUB_DeclinesUnsupported(t *testing.T) {
	t.Parallel()
	drm := testgen.KF8Sample(testgen.MOBIKindKF8)
	drm.Encrypted = true

	for name, c := range map[string]struct {
		opts testgen.MOBIOptions
		want string
	}{
		"DRM": {drm, "DRM"},
		"huge image": {func() testgen.MOBIOptions {
			o := testgen.KF8Sample(testgen.MOBIKindKF8)
			o.KF8.Images = [][]byte{append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64<<20)...)}
			return o
		}(), "image too large"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := testgen.GenerateMOBI(t, t.TempDir(), "book.azw3", c.opts)
			src, err := filegen.OpenSource(path)
			require.NoError(t, err)
			defer src.Close()
			for _, to := range []string{models.FileTypeEPUB, models.FileTypeKePub} {
				gen := conversionGenerator(t, models.FileTypeAZW3, to)
				_, err = gen.Prepare(context.Background(), src, &models.Book{Title: "Book"}, &models.File{FileType: models.FileTypeAZW3})
				require.ErrorIs(t, err, filegen.ErrUnsupportedContent, "the web routes answer it with the reason")
				var genErr *filegen.GenerationError
				require.ErrorAs(t, err, &genErr)
				assert.Equal(t, to, genErr.FileType)
				assert.Contains(t, genErr.Message, c.want)
			}
		})
	}
}

// A Kindle file reaches KePub through EPUB: the KePub has Shisho's
// metadata, Kobo markup in its chapters, and the image and any font copied
// from the source.
func TestKindleToKePub(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		fileType string
		opts     testgen.MOBIOptions
	}{
		"AZW3":           {models.FileTypeAZW3, testgen.KF8Sample(testgen.MOBIKindKF8)},
		"combo MOBI":     {models.FileTypeMOBI, testgen.KF8Sample(testgen.MOBIKindCombo)},
		"embedded fonts": {models.FileTypeAZW3, testgen.KF8FontSample(testgen.MOBIKindKF8)},
		"KF7 MOBI":       {models.FileTypeMOBI, testgen.KF7Sample()},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			src, plate := kindleSource(t, dir, "book."+c.fileType, c.opts)
			book, file := kindleBook(t, src, c.fileType)
			out, _, _ := prepareAndWrite(t, conversionGenerator(t, c.fileType, models.FileTypeKePub), src, book, file)

			outPath := filepath.Join(t.TempDir(), "out.kepub.epub")
			require.NoError(t, os.WriteFile(outPath, out, 0o600))
			parsed, err := epub.Parse(outPath)
			require.NoError(t, err)
			assert.Equal(t, "Streamed Title", parsed.Title)
			assert.Equal(t, []byte("new cover bytes"), parsed.CoverData)

			zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
			require.NoError(t, err)
			for _, f := range zr.File {
				if strings.HasSuffix(f.Name, ".xhtml") {
					assertWellFormedXML(t, f.Name, readZipEntry(t, f))
				}
			}
			assert.Contains(t, string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/part0000.xhtml"))), `class="koboSpan"`)
			assert.Equal(t, plate, readZipEntry(t, findZipEntry(t, zr, "OEBPS/image0003.png")))
			if c.opts.KF8 != nil && len(c.opts.KF8.Fonts) > 0 {
				assert.Equal(t, testgen.KF8SampleFont, readZipEntry(t, findZipEntry(t, zr, "OEBPS/font0004.ttf")))
			}
			findZipEntry(t, zr, "kobo.js")
			assertEPUBLinksResolve(t, zr)
		})
	}
}
