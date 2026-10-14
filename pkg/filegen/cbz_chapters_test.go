package filegen

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/pkg/cbz"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cbzEntryNames(t *testing.T, path string) []string {
	t.Helper()
	r, err := zip.OpenReader(path)
	require.NoError(t, err)
	defer r.Close()
	names := make([]string, len(r.File))
	for i, f := range r.File {
		names[i] = f.Name
	}
	return names
}

func generateCBZWithChapters(t *testing.T, pages []testPage, chapters []*models.Chapter) string {
	t.Helper()
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.cbz")
	createTestCBZ(t, srcPath, testCBZOptions{title: "Comic", pages: pages})
	destPath := filepath.Join(tmpDir, "dest.cbz")
	file := &models.File{FileType: models.FileTypeCBZ, Chapters: chapters}
	require.NoError(t, (&CBZGenerator{}).Generate(context.Background(), srcPath, destPath, &models.Book{Title: "Comic"}, file))
	return destPath
}

func smallPages(names ...string) []testPage {
	pages := make([]testPage, len(names))
	for i, name := range names {
		format := "jpeg"
		if filepath.Ext(name) == ".png" {
			format = "png"
		}
		pages[i] = testPage{filename: name, width: 10, height: 10, format: format}
	}
	return pages
}

func TestCBZGenerator_Chapters(t *testing.T) {
	t.Parallel()

	t.Run("writes chapters as folders that scan back as the chapters", func(t *testing.T) {
		t.Parallel()
		destPath := generateCBZWithChapters(t,
			smallPages("p1.jpg", "p2.png", "p3.jpg", "p4.jpg", "p5.jpg"),
			[]*models.Chapter{
				{Title: "Chapter 2", StartPage: pointerutil.Int(3), SortOrder: 1},
				{Title: "Chapter 1", StartPage: pointerutil.Int(1), SortOrder: 0},
			})

		assert.Equal(t, []string{
			"ComicInfo.xml",
			"0001.jpg",
			"Chapter 1/0002.jpg",
			"Chapter 1/0003.jpg",
			"Chapter 2/0004.jpg",
			"Chapter 2/0005.jpg",
		}, cbzEntryNames(t, destPath))

		parsed, err := cbz.Parse(destPath)
		require.NoError(t, err)
		require.Len(t, parsed.Chapters, 2)
		assert.Equal(t, "Chapter 1", parsed.Chapters[0].Title)
		assert.Equal(t, 1, *parsed.Chapters[0].StartPage)
		assert.Equal(t, "Chapter 2", parsed.Chapters[1].Title)
		assert.Equal(t, 3, *parsed.Chapters[1].StartPage)
	})

	t.Run("round-trips titles that need numbering", func(t *testing.T) {
		t.Parallel()
		destPath := generateCBZWithChapters(t,
			smallPages("p1.jpg", "p2.jpg", "p3.jpg"),
			[]*models.Chapter{
				{Title: "Prologue", StartPage: pointerutil.Int(0), SortOrder: 0},
				{Title: "Chapter 1", StartPage: pointerutil.Int(1), SortOrder: 1},
				{Title: "Epilogue", StartPage: pointerutil.Int(2), SortOrder: 2},
			})

		parsed, err := cbz.Parse(destPath)
		require.NoError(t, err)
		require.Len(t, parsed.Chapters, 3)
		assert.Equal(t, "Prologue", parsed.Chapters[0].Title)
		assert.Equal(t, "Chapter 1", parsed.Chapters[1].Title)
		assert.Equal(t, "Epilogue", parsed.Chapters[2].Title)
	})

	t.Run("writes only top-level chapters", func(t *testing.T) {
		t.Parallel()
		destPath := generateCBZWithChapters(t,
			smallPages("p1.jpg", "p2.jpg", "p3.jpg"),
			[]*models.Chapter{
				{ID: 1, Title: "Part 1", StartPage: pointerutil.Int(0), SortOrder: 0},
				{ID: 2, ParentID: pointerutil.Int(1), Title: "Section", StartPage: pointerutil.Int(1), SortOrder: 0},
				{ID: 3, Title: "Part 2", StartPage: pointerutil.Int(2), SortOrder: 1},
			})

		assert.Equal(t, []string{
			"ComicInfo.xml",
			"Part 1/0001.jpg",
			"Part 1/0002.jpg",
			"Part 2/0003.jpg",
		}, cbzEntryNames(t, destPath))
	})

	t.Run("keeps the archive's names when there are no chapters", func(t *testing.T) {
		t.Parallel()
		destPath := generateCBZWithChapters(t, smallPages("Ch 1/a.jpg", "Ch 2/b.jpg"), nil)
		assert.Equal(t, []string{"ComicInfo.xml", "Ch 1/a.jpg", "Ch 2/b.jpg"}, cbzEntryNames(t, destPath))
	})

	t.Run("keeps the archive's names when its folders hold the chapters", func(t *testing.T) {
		t.Parallel()
		destPath := generateCBZWithChapters(t,
			smallPages("Ch 1/a.jpg", "Ch 2/b.jpg"),
			[]*models.Chapter{
				{Title: "Ch 1", StartPage: pointerutil.Int(0), SortOrder: 0},
				{Title: "Ch 2", StartPage: pointerutil.Int(1), SortOrder: 1},
			})
		assert.Equal(t, []string{"ComicInfo.xml", "Ch 1/a.jpg", "Ch 2/b.jpg"}, cbzEntryNames(t, destPath))
	})

	t.Run("drops the old folder entries when it rewrites the folders", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{title: "Comic", pages: smallPages("Old/a.jpg", "Old/b.jpg")})
		addZipEntries(t, srcPath, "Old/", "Old/notes.txt")

		destPath := filepath.Join(tmpDir, "dest.cbz")
		file := &models.File{FileType: models.FileTypeCBZ, Chapters: []*models.Chapter{
			{Title: "New 1", StartPage: pointerutil.Int(0), SortOrder: 0},
			{Title: "New 2", StartPage: pointerutil.Int(1), SortOrder: 1},
		}}
		require.NoError(t, (&CBZGenerator{}).Generate(context.Background(), srcPath, destPath, &models.Book{Title: "Comic"}, file))

		assert.Equal(t, []string{
			"ComicInfo.xml",
			"Old/notes.txt",
			"New 1/0001.jpg",
			"New 2/0002.jpg",
		}, cbzEntryNames(t, destPath))
	})
}

func TestCBZGenerator_ChaptersFromPagesOutOfZipOrder(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.cbz")
	// Widths tell the pages apart: p1 is 10 wide, p2 20, p10 30.
	createTestCBZ(t, srcPath, testCBZOptions{title: "Comic", pages: []testPage{
		{filename: "p10.jpg", width: 30, height: 10, format: "jpeg"},
		{filename: "p2.jpg", width: 20, height: 10, format: "jpeg"},
		{filename: "p1.jpg", width: 10, height: 10, format: "jpeg"},
	}})
	destPath := filepath.Join(tmpDir, "dest.cbz")
	file := &models.File{FileType: models.FileTypeCBZ, CoverPage: pointerutil.Int(1), Chapters: []*models.Chapter{
		{Title: "A", StartPage: pointerutil.Int(0), SortOrder: 0},
		{Title: "B", StartPage: pointerutil.Int(2), SortOrder: 1},
	}}
	require.NoError(t, (&CBZGenerator{}).Generate(context.Background(), srcPath, destPath, &models.Book{Title: "Comic"}, file))

	for name, width := range map[string]int{"A/0001.jpg": 10, "A/0002.jpg": 20, "B/0003.jpg": 30} {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(readFileFromCBZ(t, destPath, name)))
		require.NoError(t, err)
		assert.Equal(t, width, cfg.Width, name)
	}
	parsed, err := cbz.Parse(destPath)
	require.NoError(t, err)
	require.NotNil(t, parsed.CoverPage)
	assert.Equal(t, 1, *parsed.CoverPage)
}

func TestCBZGenerator_ChaptersDropMacOSImages(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.cbz")
	createTestCBZ(t, srcPath, testCBZOptions{title: "Comic", pages: smallPages("Old/a.jpg", "Old/b.jpg")})
	// KOReader counts any image file as a page, so these left in the old
	// folder would add a stray chapter.
	addZipEntries(t, srcPath, "Old/._a.jpg", "__MACOSX/Old/._b.jpg")

	destPath := filepath.Join(tmpDir, "dest.cbz")
	file := &models.File{FileType: models.FileTypeCBZ, Chapters: []*models.Chapter{
		{Title: "New 1", StartPage: pointerutil.Int(0), SortOrder: 0},
		{Title: "New 2", StartPage: pointerutil.Int(1), SortOrder: 1},
	}}
	require.NoError(t, (&CBZGenerator{}).Generate(context.Background(), srcPath, destPath, &models.Book{Title: "Comic"}, file))

	assert.Equal(t, []string{"ComicInfo.xml", "New 1/0001.jpg", "New 2/0002.jpg"}, cbzEntryNames(t, destPath))
}

// addZipEntries rewrites the zip at path with extra entries appended. A name
// ending in "/" is a folder entry.
func addZipEntries(t *testing.T, path string, names ...string) {
	t.Helper()
	r, err := zip.OpenReader(path)
	require.NoError(t, err)
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range r.File {
		entry, err := w.Create(f.Name)
		require.NoError(t, err)
		data, err := readCBZZipFile(f)
		require.NoError(t, err)
		_, err = entry.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, r.Close())
	for _, name := range names {
		entry, err := w.Create(name)
		require.NoError(t, err)
		if !strings.HasSuffix(name, "/") {
			_, err = entry.Write([]byte("notes"))
			require.NoError(t, err)
		}
	}
	require.NoError(t, w.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))
}
