package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The generated chapter is "Chapter 1" and "This is a test chapter.".
const generatedChapterWords = 7

func TestParse_WordCount(t *testing.T) {
	t.Parallel()

	t.Run("counts the spine's text", func(t *testing.T) {
		t.Parallel()
		path := testgen.GenerateEPUB(t, t.TempDir(), "book.epub", testgen.EPUBOptions{Title: "Book"})
		metadata, err := Parse(path)
		require.NoError(t, err)
		require.NotNil(t, metadata.WordCount)
		assert.Equal(t, generatedChapterWords, *metadata.WordCount)
	})

	t.Run("documents outside the spine don't count", func(t *testing.T) {
		t.Parallel()
		path := testgen.GenerateEPUB(t, t.TempDir(), "book.epub", testgen.EPUBOptions{
			Title:        "Book",
			ExtraEntries: map[string]string{"OEBPS/notes.xhtml": "<html><body><p>Not in the spine at all.</p></body></html>"},
		})
		metadata, err := Parse(path)
		require.NoError(t, err)
		require.NotNil(t, metadata.WordCount)
		assert.Equal(t, generatedChapterWords, *metadata.WordCount)
	})

	t.Run("a book with encrypted content has no count", func(t *testing.T) {
		t.Parallel()
		path := testgen.GenerateEPUB(t, t.TempDir(), "book.epub", testgen.EPUBOptions{
			Title:        "Book",
			ExtraEntries: map[string]string{"META-INF/rights.xml": "<rights/>"},
		})
		metadata, err := Parse(path)
		require.NoError(t, err)
		assert.Nil(t, metadata.WordCount)
	})
}

func TestParseKePub_WordCount(t *testing.T) {
	t.Parallel()

	path := testgen.GenerateEPUB(t, t.TempDir(), "book.kepub.epub", testgen.EPUBOptions{Title: "Book", KoboMarkup: true})
	metadata, err := ParseKePub(path)
	require.NoError(t, err)
	require.NotNil(t, metadata.WordCount)
	assert.Equal(t, generatedChapterWords, *metadata.WordCount, "Kobo's spans and script don't change the count")
}

func TestWordCount(t *testing.T) {
	t.Parallel()

	t.Run("counts as Parse does", func(t *testing.T) {
		t.Parallel()
		path := testgen.GenerateEPUB(t, t.TempDir(), "book.epub", testgen.EPUBOptions{
			Title:   "Book",
			Chapter: []byte(`<html><body><h1>One</h1><p>Two <i>three</i> four five six.</p></body></html>`),
		})
		n, err := WordCount(context.Background(), path)
		require.NoError(t, err)
		assert.Equal(t, 6, n)
	})

	t.Run("declines a book with encrypted content", func(t *testing.T) {
		t.Parallel()
		path := testgen.GenerateEPUB(t, t.TempDir(), "book.kepub.epub", testgen.EPUBOptions{
			Title: "Locked", KoboMarkup: true, KoboDRM: true,
		})
		_, err := WordCount(context.Background(), path)
		assert.ErrorIs(t, err, mediafile.ErrDRMProtected)
	})
}

// Shisho's own KePub conversion writes Kobo's script as a self-closed
// element, as XHTML allows.
func TestParseKePub_WordCountOfConvertedEPUB(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := testgen.GenerateEPUB(t, dir, "book.epub", testgen.EPUBOptions{Title: "Book"})
	dest := filepath.Join(dir, "book.kepub.epub")
	require.NoError(t, kepub.NewConverter().ConvertEPUB(context.Background(), src, dest))

	metadata, err := ParseKePub(dest)
	require.NoError(t, err)
	require.NotNil(t, metadata.WordCount)
	assert.Equal(t, generatedChapterWords, *metadata.WordCount)
}

// openEPUB builds an EPUB in memory whose spine lists idrefs in order, with
// each manifest item a chapter holding text.
func openEPUB(t *testing.T, chapters map[string]string, idrefs ...string) (*zip.Reader, *ParseOPFResult) {
	t.Helper()
	return openEPUBWithMeta(t, "", chapters, idrefs...)
}

// openEPUBWithMeta is openEPUB with meta added to the package's metadata.
func openEPUBWithMeta(t *testing.T, meta string, chapters map[string]string, idrefs ...string) (*zip.Reader, *ParseOPFResult) {
	t.Helper()
	var manifest, spine strings.Builder
	for id := range chapters {
		manifest.WriteString(`<item id="` + id + `" href="` + id + `.xhtml" media-type="application/xhtml+xml"/>`)
	}
	for _, id := range idrefs {
		spine.WriteString(`<itemref idref="` + id + `"/>`)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, body string) {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	write("OEBPS/content.opf", `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>T</dc:title>`+meta+`</metadata><manifest>`+manifest.String()+`</manifest><spine>`+spine.String()+`</spine></package>`)
	for id, text := range chapters {
		write("OEBPS/"+id+".xhtml", `<html><body><p>`+text+`</p></body></html>`)
	}
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	result, err := findOPF(zr)
	require.NoError(t, err)
	return zr, result
}

func TestCountSpine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("a document the spine lists twice counts once", func(t *testing.T) {
		t.Parallel()
		zr, result := openEPUB(t, map[string]string{"a": "one two three four five", "b": "six seven eight nine ten"}, "a", "b", "a", "a")
		n, err := countSpine(ctx, zr, result, maxBookTextBytes)
		require.NoError(t, err)
		assert.Equal(t, 10, n)
	})

	t.Run("a spine naming no document in the book is no count, not zero", func(t *testing.T) {
		t.Parallel()
		zr, result := openEPUB(t, map[string]string{"a": "one"}, "missing")
		_, err := countSpine(ctx, zr, result, maxBookTextBytes)
		assert.Error(t, err)
	})

	t.Run("a book whose text runs past the limit has no count", func(t *testing.T) {
		t.Parallel()
		zr, result := openEPUB(t, map[string]string{"a": strings.Repeat("word ", 40), "b": strings.Repeat("word ", 40)}, "a", "b")
		_, err := countSpine(ctx, zr, result, 300)
		assert.Error(t, err)
	})

	t.Run("a canceled count stops", func(t *testing.T) {
		t.Parallel()
		zr, result := openEPUB(t, map[string]string{"a": "one"}, "a")
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		_, err := countSpine(canceled, zr, result, maxBookTextBytes)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestCountWords_PictureBooks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("a fixed-layout book has no count", func(t *testing.T) {
		t.Parallel()
		zr, result := openEPUBWithMeta(t, `<meta property="rendition:layout">pre-paginated</meta>`,
			map[string]string{"a": strings.Repeat("word ", 50)}, "a")
		_, err := countWords(ctx, zr, result)
		assert.Error(t, err)
	})

	t.Run("a book of pictures with a few words has no count", func(t *testing.T) {
		t.Parallel()
		pages := map[string]string{"copyright": "All rights reserved by the publisher"}
		idrefs := []string{"copyright"}
		for i := range 20 {
			id := "page" + strings.Repeat("i", i+1)
			pages[id] = ""
			idrefs = append(idrefs, id)
		}
		zr, result := openEPUB(t, pages, idrefs...)
		_, err := countWords(ctx, zr, result)
		assert.Error(t, err, "6 words over 21 documents")
	})
}
