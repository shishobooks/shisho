package filegen_test

import (
	"archive/zip"
	"bytes"
	"context"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/filegen/filegentest"
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

// kindleSource writes testgen.KF8Sample, showing kindlePlate, as a file of
// the given kind.
func kindleSource(t *testing.T, dir, name string, kind testgen.MOBIKind) (string, []byte) {
	t.Helper()
	opts := testgen.KF8Sample(kind)
	plate := kindlePlate()
	opts.KF8.Images = [][]byte{plate}
	return testgen.GenerateMOBI(t, dir, name, opts), plate
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
		kind     testgen.MOBIKind
	}{
		"AZW3":       {models.FileTypeAZW3, testgen.MOBIKindKF8},
		"combo MOBI": {models.FileTypeMOBI, testgen.MOBIKindCombo},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			src, plate := kindleSource(t, dir, "source."+c.fileType, c.kind)
			book, file := kindleBook(t, src, c.fileType)
			broken := testgen.GenerateMOBI(t, dir, "broken."+c.fileType, testgen.MOBIOptions{Kind: testgen.MOBIKindMOBI6, Title: "Old"})

			gen := conversionGenerator(t, c.fileType, models.FileTypeEPUB)
			assert.Equal(t, models.FileTypeEPUB, gen.(filegen.Generator).SupportedType())
			filegentest.Run(t, filegentest.Case{
				Generator: gen,
				Source:    src,
				Book:      book,
				File:      file,
				Broken:    broken,
				Check:     func(t *testing.T, output []byte) { checkKindleEPUB(t, output, plate) },
			})
		})
	}
}

// checkKindleEPUB checks that a converted EPUB parses with Shisho's
// metadata and chapters, keeps the source's text and images, and that its
// links resolve.
func checkKindleEPUB(t *testing.T, output, plate []byte) {
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
	assert.Equal(t, "part0000.xhtml#ch1", *parsed.Chapters[0].Href)
	require.Len(t, parsed.Chapters[0].Children, 1)
	assert.Equal(t, "part0000.xhtml#later", *parsed.Chapters[0].Children[0].Href)
	assert.Equal(t, "The Second", parsed.Chapters[1].Title, "a renamed chapter keeps its new title")
	assert.Equal(t, "part0001.xhtml#ch2", *parsed.Chapters[1].Href)

	zr, err := zip.NewReader(bytes.NewReader(output), int64(len(output)))
	require.NoError(t, err)
	assert.Equal(t, "mimetype", zr.File[0].Name)
	assert.Equal(t, zip.Store, zr.File[0].Method)

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
	assertEPUBLinksResolve(t, zr)

	opf := string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/content.opf")))
	for _, href := range []string{"part0000.xhtml", "part0001.xhtml", "flow0001.css", "image0001.jpg", "image0003.png", "nav.xhtml", "toc.ncx"} {
		assert.Contains(t, opf, `href="`+href+`"`)
	}
	for _, f := range zr.File {
		assert.NotRegexp(t, `["(]kindle:`, string(readZipEntry(t, f)), f.Name)
	}
}

// The identifier of a converted EPUB depends on the book, not on when it
// was converted.
func TestKindleToEPUB_StableIdentifier(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src, _ := kindleSource(t, dir, "book.azw3", testgen.MOBIKindKF8)
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
	font := testgen.KF8Sample(testgen.MOBIKindKF8)
	font.KF8.Font = true
	huff := testgen.KF8Sample(testgen.MOBIKindKF8)
	huff.Compression = testgen.MOBICompressionHuffCDIC
	drm := testgen.KF8Sample(testgen.MOBIKindKF8)
	drm.Encrypted = true

	for name, c := range map[string]struct {
		opts testgen.MOBIOptions
		want string
	}{
		"KF7 only":      {testgen.MOBIOptions{Kind: testgen.MOBIKindMOBI6, Title: "Old"}, "older MOBI (KF7) book"},
		"HUFF/CDIC":     {huff, "HUFF/CDIC compression"},
		"embedded font": {font, "embeds fonts"},
		"DRM":           {drm, "DRM"},
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

// An AZW3 reaches KePub through EPUB: the KePub has Shisho's metadata, Kobo
// markup in its chapters, and the image copied from the source.
func TestKindleToKePub(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src, plate := kindleSource(t, dir, "book.azw3", testgen.MOBIKindKF8)
	book, file := kindleBook(t, src, models.FileTypeAZW3)
	out, _, _ := prepareAndWrite(t, conversionGenerator(t, models.FileTypeAZW3, models.FileTypeKePub), src, book, file)

	outPath := filepath.Join(t.TempDir(), "out.kepub.epub")
	require.NoError(t, os.WriteFile(outPath, out, 0o600))
	parsed, err := epub.Parse(outPath)
	require.NoError(t, err)
	assert.Equal(t, "Streamed Title", parsed.Title)
	assert.Equal(t, []byte("new cover bytes"), parsed.CoverData)

	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	require.NoError(t, err)
	assert.Contains(t, string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/part0000.xhtml"))), `class="koboSpan"`)
	assert.Equal(t, plate, readZipEntry(t, findZipEntry(t, zr, "OEBPS/image0003.png")))
	findZipEntry(t, zr, "kobo.js")
	assertEPUBLinksResolve(t, zr)
}
