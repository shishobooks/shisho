package filegen_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/filegen/filegentest"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdf"
	"github.com/shishobooks/shisho/pkg/pdf/incremental"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pdfSourceKind is how a test source PDF stores its cross-reference data.
type pdfSourceKind string

const (
	classicXRef   pdfSourceKind = "classic cross-reference table"
	xrefStream    pdfSourceKind = "cross-reference stream"
	objectStreams pdfSourceKind = "compressed object streams"
)

var pdfSourceKinds = []pdfSourceKind{classicXRef, xrefStream, objectStreams}

// writeSourcePDF writes a 5-page PDF of the given kind, mostly page
// content, and returns its path.
func writeSourcePDF(t *testing.T, dir string, kind pdfSourceKind) string {
	t.Helper()
	classic := filepath.Join(dir, "classic.pdf")
	filegen.WriteTestPDFWithContent(t, classic, 5, map[string]string{
		"Title":    "Original Title",
		"Author":   "Original Author",
		"Creator":  "Original Creator",
		"Keywords": "old, words",
	}, 200_000)
	if kind == classicXRef {
		return classic
	}

	pdf.EnsurePdfcpuInit()
	conf := model.NewDefaultConfiguration()
	conf.WriteXRefStream = true
	conf.WriteObjectStream = kind == objectStreams
	out := filepath.Join(dir, strings.ReplaceAll(string(kind), " ", "-")+".pdf")
	require.NoError(t, api.OptimizeFile(context.Background(), classic, out, conf, nil))

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Contains(t, string(data), "/XRef", "the source must use a cross-reference stream")
	if kind == objectStreams {
		require.Contains(t, string(data), "/ObjStm", "the source must use object streams")
	} else {
		require.NotContains(t, string(data), "/ObjStm")
	}
	return out
}

// writeBrokenPDF writes a file whose preparation fails and returns its
// path.
func writeBrokenPDF(t *testing.T, dir string) string {
	t.Helper()
	broken := filepath.Join(dir, "broken.pdf")
	require.NoError(t, os.WriteFile(broken, []byte("%PDF-1.4\nnot really a PDF\n"), 0o600))
	return broken
}

func pdfTestBook() (*models.Book, *models.File) {
	description := "A book about things"
	language := "fr"
	name := "File Title ✓"
	p0, p2, p4 := 0, 2, 4
	book := &models.Book{
		Title:       "Book Title",
		Description: &description,
		Authors: []*models.Author{
			{SortOrder: 1, Person: &models.Person{Name: "Second Author"}},
			{SortOrder: 0, Person: &models.Person{Name: "First Author"}},
		},
		BookTags: []*models.BookTag{
			{Tag: &models.Tag{Name: "Fantasy"}},
			{Tag: &models.Tag{Name: "Adventure"}},
		},
	}
	file := &models.File{
		FileType: models.FileTypePDF,
		Name:     &name,
		Language: &language,
		Chapters: []*models.Chapter{
			{Title: "Opening", SortOrder: 0, StartPage: &p0},
			{Title: "Middle", SortOrder: 1, StartPage: &p2, Children: []*models.Chapter{
				{Title: "Inside", SortOrder: 0, StartPage: &p2},
			}},
			{Title: "Ending", SortOrder: 2, StartPage: &p4},
		},
	}
	return book, file
}

// checkPDFOutput reads output back through the PDF parser and checks it
// carries the book's metadata and bookmarks.
func checkPDFOutput(t *testing.T, output []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out.pdf")
	require.NoError(t, os.WriteFile(path, output, 0o600))

	meta, err := pdf.Parse(path)
	require.NoError(t, err)
	assert.Equal(t, "File Title ✓", meta.Title)
	require.Len(t, meta.Authors, 2)
	assert.Equal(t, "First Author", meta.Authors[0].Name)
	assert.Equal(t, "Second Author", meta.Authors[1].Name)
	assert.Equal(t, "A book about things", meta.Description)
	assert.Equal(t, []string{"Fantasy", "Adventure"}, meta.Tags)
	require.NotNil(t, meta.Language)
	assert.Equal(t, "fr", *meta.Language)
	require.Len(t, meta.Chapters, 4)
	for i, want := range []struct {
		title string
		page  int
	}{{"Opening", 0}, {"Middle", 2}, {"Inside", 2}, {"Ending", 4}} {
		assert.Equal(t, want.title, meta.Chapters[i].Title)
		require.NotNil(t, meta.Chapters[i].StartPage)
		assert.Equal(t, want.page, *meta.Chapters[i].StartPage)
	}
}

func generatePDF(t *testing.T, srcPath string, book *models.Book, file *models.File) ([]byte, int64, error) {
	t.Helper()
	src, err := filegen.OpenSource(srcPath)
	require.NoError(t, err)
	defer src.Close()
	prepared, err := (&filegen.PDFGenerator{}).Prepare(context.Background(), src, book, file)
	if err != nil {
		return nil, 0, err
	}
	size, ok := prepared.Size()
	require.True(t, ok, "a PDF's size is known up front")
	var buf bytes.Buffer
	require.NoError(t, prepared.Write(context.Background(), &buf))
	return buf.Bytes(), size, nil
}

func TestPDFGenerator_Conformance(t *testing.T) {
	t.Parallel()
	for _, kind := range pdfSourceKinds {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			book, file := pdfTestBook()
			filegentest.Run(t, filegentest.Case{
				Generator: &filegen.PDFGenerator{},
				Source:    writeSourcePDF(t, dir, kind),
				Book:      book,
				File:      file,
				Check:     checkPDFOutput,
				Broken:    writeBrokenPDF(t, dir),
			})
		})
	}
}

// The original bytes are kept as they are, with the update after them, so
// a signature over them stays valid.
func TestPDFGenerator_OriginalIsPrefix(t *testing.T) {
	t.Parallel()
	for _, kind := range pdfSourceKinds {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			srcPath := writeSourcePDF(t, t.TempDir(), kind)
			original, err := os.ReadFile(srcPath)
			require.NoError(t, err)
			book, file := pdfTestBook()

			out, size, err := generatePDF(t, srcPath, book, file)
			require.NoError(t, err)
			assert.Equal(t, int64(len(out)), size)
			require.Greater(t, len(out), len(original))
			assert.True(t, bytes.HasPrefix(out, original), "the output must start with the original bytes")
		})
	}
}

// The update's cross-reference section is the same kind as the source's.
func TestPDFGenerator_KeepsCrossReferenceKind(t *testing.T) {
	t.Parallel()
	for _, kind := range pdfSourceKinds {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			srcPath := writeSourcePDF(t, t.TempDir(), kind)
			original, err := os.ReadFile(srcPath)
			require.NoError(t, err)
			book, file := pdfTestBook()

			out, _, err := generatePDF(t, srcPath, book, file)
			require.NoError(t, err)
			appended := string(out[len(original):])
			if kind == classicXRef {
				assert.Contains(t, appended, "\nxref\n")
				assert.Contains(t, appended, "trailer")
			} else {
				assert.Contains(t, appended, "/Type /XRef")
				assert.NotContains(t, appended, "\nxref\n")
			}
		})
	}
}

// Updating an already updated PDF follows the chain of cross-reference
// sections, and the newest metadata wins.
func TestPDFGenerator_UpdatesAnUpdatedPDF(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := writeSourcePDF(t, dir, objectStreams)
	first, _, err := generatePDF(t, srcPath, &models.Book{Title: "First Pass"}, &models.File{FileType: models.FileTypePDF})
	require.NoError(t, err)
	updated := filepath.Join(dir, "updated.pdf")
	require.NoError(t, os.WriteFile(updated, first, 0o600))

	book, file := pdfTestBook()
	out, _, err := generatePDF(t, updated, book, file)
	require.NoError(t, err)
	checkPDFOutput(t, out)
}

// The update writes an XMP metadata stream matching the info dictionary.
func TestPDFGenerator_WritesXMP(t *testing.T) {
	t.Parallel()
	srcPath := writeSourcePDF(t, t.TempDir(), classicXRef)
	book, file := pdfTestBook()
	out, _, err := generatePDF(t, srcPath, book, file)
	require.NoError(t, err)

	doc, err := incremental.Open(bytes.NewReader(out), int64(len(out)))
	require.NoError(t, err)
	_, catalog, err := doc.Catalog()
	require.NoError(t, err)
	o, err := doc.Resolve(catalog["Metadata"])
	require.NoError(t, err)
	stream, ok := o.(*incremental.Stream)
	require.True(t, ok, "the catalog's /Metadata must be a stream")
	assert.Equal(t, incremental.Name("XML"), stream.Dict["Subtype"])
	xmp := string(stream.Data)
	assert.Contains(t, xmp, "File Title ✓")
	assert.Contains(t, xmp, "<rdf:li>First Author</rdf:li>")
	assert.Contains(t, xmp, "<rdf:li>Second Author</rdf:li>")
	assert.Contains(t, xmp, "A book about things")
	assert.Contains(t, xmp, "<rdf:li>Fantasy</rdf:li>")
	assert.Contains(t, xmp, "<rdf:li>fr</rdf:li>")
}

// Info entries Shisho does not set are kept.
func TestPDFGenerator_KeepsOtherInfoEntries(t *testing.T) {
	t.Parallel()
	srcPath := writeSourcePDF(t, t.TempDir(), objectStreams)
	out, _, err := generatePDF(t, srcPath, &models.Book{Title: "New"}, &models.File{FileType: models.FileTypePDF})
	require.NoError(t, err)

	doc, err := incremental.Open(bytes.NewReader(out), int64(len(out)))
	require.NoError(t, err)
	o, err := doc.Resolve(doc.Trailer()["Info"])
	require.NoError(t, err)
	info, ok := o.(incremental.Dict)
	require.True(t, ok)
	assert.Equal(t, "New", info["Title"].(incremental.String).Text())
	assert.Equal(t, "Original Creator", info["Creator"].(incremental.String).Text())
	assert.Equal(t, "old, words", info["Keywords"].(incremental.String).Text(), "keywords stay when the book has no tags")
}

// restrictedCrypto is how a test source is encrypted with the standard
// security handler.
type restrictedCrypto struct {
	name string
	conf func(userPW string) *model.Configuration
}

var restrictedCryptos = []restrictedCrypto{
	{"RC4 40-bit", func(pw string) *model.Configuration { return model.NewRC4Configuration(pw, "owner", 40) }},
	{"RC4 128-bit", func(pw string) *model.Configuration { return model.NewRC4Configuration(pw, "owner", 128) }},
	{"AES-128", func(pw string) *model.Configuration { return model.NewAESConfiguration(pw, "owner", 128) }},
	{"AES-256", func(pw string) *model.Configuration { return model.NewAESConfiguration(pw, "owner", 256) }},
}

// writeEncryptedPDF writes a source of the given kind encrypted with c and
// the user password userPW, allowing only printing, and returns its path.
// An empty userPW makes it permission-restricted: it opens without a
// password.
func writeEncryptedPDF(t *testing.T, dir string, kind pdfSourceKind, c restrictedCrypto, userPW string) string {
	t.Helper()
	srcPath := writeSourcePDF(t, dir, kind)
	pdf.EnsurePdfcpuInit()
	conf := c.conf(userPW)
	conf.Permissions = model.PermissionsPrint
	conf.WriteXRefStream = kind != classicXRef
	conf.WriteObjectStream = kind == objectStreams
	out := filepath.Join(dir, "encrypted.pdf")
	require.NoError(t, api.EncryptFile(context.Background(), srcPath, out, conf))
	return out
}

// checkKeepsEncryption checks that output, generated from original, still
// uses the original's encryption and permissions, and that its update is
// encrypted: no author name appears in it as plain text unless the
// document metadata is left unencrypted.
func checkKeepsEncryption(t *testing.T, original, output []byte, cleartextMetadata bool) {
	t.Helper()
	require.True(t, bytes.HasPrefix(output, original), "the output must start with the original bytes")

	origDoc, err := incremental.Open(bytes.NewReader(original), int64(len(original)))
	require.NoError(t, err)
	outDoc, err := incremental.Open(bytes.NewReader(output), int64(len(output)))
	require.NoError(t, err)
	require.NotNil(t, origDoc.Trailer()["Encrypt"])
	assert.Equal(t, origDoc.Trailer()["Encrypt"], outDoc.Trailer()["Encrypt"])
	assert.Equal(t, origDoc.Trailer()["ID"], outDoc.Trailer()["ID"])

	pdf.EnsurePdfcpuInit()
	origPerms, err := api.GetPermissions(context.Background(), bytes.NewReader(original), model.NewDefaultConfiguration())
	require.NoError(t, err)
	outPerms, err := api.GetPermissions(context.Background(), bytes.NewReader(output), model.NewDefaultConfiguration())
	require.NoError(t, err)
	require.NotNil(t, origPerms)
	require.NotNil(t, outPerms, "the output must still be encrypted")
	assert.Equal(t, *origPerms, *outPerms)

	appended := string(output[len(original):])
	assert.NotContains(t, appended, "(First Author", "strings in the update must be encrypted")
	if cleartextMetadata {
		assert.Contains(t, appended, "<rdf:li>First Author</rdf:li>", "the metadata stream stays unencrypted when the file says so")
	} else {
		assert.NotContains(t, appended, "First Author", "the metadata stream must be encrypted")
	}

	// The update reads back decrypted.
	_, catalog, err := outDoc.Catalog()
	require.NoError(t, err)
	o, err := outDoc.Resolve(catalog["Metadata"])
	require.NoError(t, err)
	stream, ok := o.(*incremental.Stream)
	require.True(t, ok, "the catalog's /Metadata must be a stream")
	assert.Contains(t, string(stream.Data), "<rdf:li>First Author</rdf:li>")
}

// A permission-restricted PDF opens without a password, so it gets
// Shisho's metadata in an update encrypted the same way as the original.
func TestPDFGenerator_PermissionRestricted(t *testing.T) {
	t.Parallel()
	for _, c := range restrictedCryptos {
		for _, kind := range pdfSourceKinds {
			t.Run(c.name+"/"+string(kind), func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				srcPath := writeEncryptedPDF(t, dir, kind, c, "")
				original, err := os.ReadFile(srcPath)
				require.NoError(t, err)
				if kind == objectStreams {
					require.Contains(t, string(original), "/ObjStm", "the source must use object streams")
				}
				book, file := pdfTestBook()
				filegentest.Run(t, filegentest.Case{
					Generator: &filegen.PDFGenerator{},
					Source:    srcPath,
					Book:      book,
					File:      file,
					Check: func(t *testing.T, output []byte) {
						checkPDFOutput(t, output)
						checkKeepsEncryption(t, original, output, false)
					},
					Broken: writeBrokenPDF(t, dir),
				})
			})
		}
	}
}

// Revisions and options pdfcpu cannot write (its RC4 128-bit is revision
// 4 and its AES-256 is revision 5), made with qpdf from a 5-page PDF
// written by writeTestPDFWithContent:
//
//	qpdf --allow-weak-crypto --encrypt --user-password= --owner-password=owner --bits=128 --use-aes=n --print=none --modify=none -- in.pdf restricted-rc4-128-r3.pdf
//	qpdf --encrypt --user-password= --owner-password=owner --bits=128 --use-aes=y --cleartext-metadata --print=none -- in.pdf restricted-aes-128-cleartext-metadata.pdf
//	qpdf --object-streams=generate --encrypt --user-password= --owner-password=owner --bits=256 --print=none -- in.pdf restricted-aes-256-r6-object-streams.pdf
func TestPDFGenerator_PermissionRestrictedFixtures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		file              string
		cleartextMetadata bool
	}{
		{file: "restricted-rc4-128-r3.pdf"},
		{file: "restricted-aes-128-cleartext-metadata.pdf", cleartextMetadata: true},
		{file: "restricted-aes-256-r6-object-streams.pdf"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			srcPath := filepath.Join("testdata", tc.file)
			original, err := os.ReadFile(srcPath)
			require.NoError(t, err)
			book, file := pdfTestBook()
			out, size, err := generatePDF(t, srcPath, book, file)
			require.NoError(t, err)
			assert.Equal(t, int64(len(out)), size)
			checkPDFOutput(t, out)
			checkKeepsEncryption(t, original, out, tc.cleartextMetadata)
		})
	}
}

// A PDF that needs a password to open cannot be read, so device routes
// serve the Original File.
func TestPDFGenerator_PasswordRequiredServedAsOriginal(t *testing.T) {
	t.Parallel()
	for _, c := range restrictedCryptos {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			encrypted := writeEncryptedPDF(t, t.TempDir(), classicXRef, c, "user")
			book, file := pdfTestBook()
			_, _, err := generatePDF(t, encrypted, book, file)
			require.Error(t, err)
			var genErr *filegen.GenerationError
			require.ErrorAs(t, err, &genErr, "a GenerationError makes device routes serve the Original File")
			require.ErrorIs(t, err, incremental.ErrCannotDecrypt)
			assert.ErrorIs(t, err, filegen.ErrCannotDecrypt, "the web UI routes answer this with a message")
		})
	}
}

func TestPDFGenerator_UnreadableXRefServedAsOriginal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := writeSourcePDF(t, dir, classicXRef)
	data, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	i := bytes.LastIndex(data, []byte("startxref"))
	require.Positive(t, i)
	broken := append(append([]byte{}, data[:i]...), []byte("startxref\n12\n%%EOF\n")...)
	brokenPath := filepath.Join(dir, "broken.pdf")
	require.NoError(t, os.WriteFile(brokenPath, broken, 0o600))

	book, file := pdfTestBook()
	_, _, err = generatePDF(t, brokenPath, book, file)
	require.Error(t, err)
	var genErr *filegen.GenerationError
	require.ErrorAs(t, err, &genErr)
	assert.ErrorIs(t, err, incremental.ErrUnreadable)
}

// Chapters on a PDF whose page tree is empty are dropped instead of
// pointing at pages that do not exist.
func TestPDFGenerator_EmptyPageTreeSkipsBookmarks(t *testing.T) {
	t.Parallel()
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	catalog := b.Len()
	b.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	pages := b.Len()
	b.WriteString("2 0 obj\n<< /Type /Pages /Kids [] /Count 0 >>\nendobj\n")
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 3\n0000000000 65535 f\r\n%010d 00000 n\r\n%010d 00000 n\r\ntrailer\n<< /Size 3 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", catalog, pages, xref)
	srcPath := filepath.Join(t.TempDir(), "empty.pdf")
	require.NoError(t, os.WriteFile(srcPath, b.Bytes(), 0o600))

	book, file := pdfTestBook()
	out, _, err := generatePDF(t, srcPath, book, file)
	require.NoError(t, err)
	assert.NotContains(t, string(out[b.Len():]), "/Outlines")
}

// A PDF older than 1.4 is declared 1.4 by the update, which adds the
// /Metadata and /Lang entries that version introduced.
func TestPDFGenerator_RaisesVersionForMetadata(t *testing.T) {
	t.Parallel()
	srcPath := writeSourcePDF(t, t.TempDir(), classicXRef)
	data, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(data, []byte("%PDF-1.4")))
	copy(data, "%PDF-1.3")
	require.NoError(t, os.WriteFile(srcPath, data, 0o600))

	book, file := pdfTestBook()
	out, _, err := generatePDF(t, srcPath, book, file)
	require.NoError(t, err)
	doc, err := incremental.Open(bytes.NewReader(out), int64(len(out)))
	require.NoError(t, err)
	_, catalog, err := doc.Catalog()
	require.NoError(t, err)
	assert.Equal(t, incremental.Name("1.4"), catalog["Version"])
	checkPDFOutput(t, out)
}
