package main

import (
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/cbz"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The flow turns the comic to page 4 of 8, and the manga is there to try
// reading right to left, so Shisho has to read them that way.
func TestComicsAreReadAsTheyreMade(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	comic := filepath.Join(dir, "comic.cbz")
	require.NoError(t, writeComic(comic, "Harness Comic", "Pat Penciller", comicPrinted))
	parsed, err := cbz.Parse(comic)
	require.NoError(t, err)
	assert.Equal(t, "Harness Comic", parsed.Title)
	require.NotNil(t, parsed.PageCount)
	assert.Equal(t, 8, *parsed.PageCount)
	require.NotNil(t, parsed.ReadingDirection)
	assert.Equal(t, models.ReadingDirectionLTR, *parsed.ReadingDirection)

	manga := filepath.Join(dir, "manga.cbz")
	require.NoError(t, writeComic(manga, "Harness Manga", "Pat Penciller", comicManga))
	parsed, err = cbz.Parse(manga)
	require.NoError(t, err)
	require.NotNil(t, parsed.ReadingDirection)
	assert.Equal(t, models.ReadingDirectionRTL, *parsed.ReadingDirection)
}

// The manual's outline has to become chapters starting on its pages, to try
// the page reader's contents with.
func TestManualOutlineIsItsChapters(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "manual.pdf")
	require.NoError(t, writeManual(path, "Harness Manual"))

	parsed, err := pdf.Parse(path)
	require.NoError(t, err)
	require.NotNil(t, parsed.PageCount)
	assert.Equal(t, manualPages, *parsed.PageCount)

	outline, err := pdf.ExtractOutline(path)
	require.NoError(t, err)
	require.Len(t, outline, len(manualChapters))
	for i, chapter := range manualChapters {
		assert.Equal(t, chapter.Title, outline[i].Title)
		assert.Equal(t, chapter.Page, outline[i].StartPage)
	}
}
