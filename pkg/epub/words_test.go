package epub

import (
	"context"
	"path/filepath"
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
			Chapter: []byte(`<html><body><h1>One</h1><p>Two <i>three</i> four.</p></body></html>`),
		})
		n, err := WordCount(path)
		require.NoError(t, err)
		assert.Equal(t, 4, n)
	})

	t.Run("declines a book with encrypted content", func(t *testing.T) {
		t.Parallel()
		path := testgen.GenerateEPUB(t, t.TempDir(), "book.kepub.epub", testgen.EPUBOptions{
			Title: "Locked", KoboMarkup: true, KoboDRM: true,
		})
		_, err := WordCount(path)
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
