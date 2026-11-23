package main

import (
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reader flows go to the novel's chapters by title, so Shisho has to
// keep them as the File's chapters, with the links the reader resolves.
func TestNovelChaptersAreShishos(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "novel.epub")
	require.NoError(t, writeNovel(path, "The Harness Novel", "Ada Writer", "Harness Series"))

	parsed, err := epub.Parse(path)
	require.NoError(t, err)
	assert.Equal(t, "The Harness Novel", parsed.Title)
	require.Len(t, parsed.Authors, 1)
	assert.Equal(t, "Ada Writer", parsed.Authors[0].Name)
	assert.NotEmpty(t, parsed.CoverData)

	titles := make([]string, 0, len(parsed.Chapters))
	hrefs := make([]string, 0, len(parsed.Chapters))
	for _, chapter := range parsed.Chapters {
		titles = append(titles, chapter.Title)
		require.NotNil(t, chapter.Href)
		hrefs = append(hrefs, *chapter.Href)
	}
	assert.Equal(t, []string{"Chapter One", "Chapter Two", "Chapter Three", "Notes"}, titles)
	assert.Equal(t, []string{"chapter1.xhtml", "chapter2.xhtml", "chapter3.xhtml", "notes.xhtml"}, hrefs)
}

// The fixed-layout fixture is read as one, with a chapter per page.
func TestPictureBookIsFixedLayout(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "picture.epub")
	require.NoError(t, writePictureBook(path, "Harness Picture Book", "Ivy Illustrator"))

	parsed, err := epub.Parse(path)
	require.NoError(t, err)
	assert.Equal(t, "Harness Picture Book", parsed.Title)
	require.Len(t, parsed.Chapters, pictureBookPages)
	assert.Equal(t, "Page 3", parsed.Chapters[2].Title)
}
