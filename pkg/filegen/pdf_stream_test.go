package filegen_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pkg/errors"
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
	require.NoError(t, api.OptimizeFile(classic, out, conf))

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
			broken := filepath.Join(dir, "broken.pdf")
			require.NoError(t, os.WriteFile(broken, []byte("%PDF-1.4\nnot really a PDF\n"), 0o600))
			book, file := pdfTestBook()
			filegentest.Run(t, filegentest.Case{
				Generator: &filegen.PDFGenerator{},
				Source:    writeSourcePDF(t, dir, kind),
				Book:      book,
				File:      file,
				Check:     checkPDFOutput,
				Broken:    broken,
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

func TestPDFGenerator_EncryptedServedAsOriginal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := writeSourcePDF(t, dir, classicXRef)
	encrypted := filepath.Join(dir, "encrypted.pdf")
	pdf.EnsurePdfcpuInit()
	require.NoError(t, api.EncryptFile(srcPath, encrypted, model.NewAESConfiguration("", "owner", 256)))

	book, file := pdfTestBook()
	_, _, err := generatePDF(t, encrypted, book, file)
	require.Error(t, err)
	var genErr *filegen.GenerationError
	require.ErrorAs(t, err, &genErr, "a GenerationError makes device routes serve the Original File")
	assert.ErrorIs(t, err, incremental.ErrEncrypted)
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
	assert.True(t, errors.Is(err, incremental.ErrUnreadable))
}
