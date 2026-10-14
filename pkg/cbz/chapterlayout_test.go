package cbz

import (
	"slices"
	"testing"

	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chapterAt(title string, page int) mediafile.ParsedChapter {
	return mediafile.ParsedChapter{Title: title, StartPage: &page}
}

func flatPages(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = "page" + string(rune('a'+i)) + ".jpg"
	}
	return names
}

// assertChapterLayout checks that paths keep the pages in reading order for
// both Shisho and KOReader's MuPDF, and that scanning them finds want.
func assertChapterLayout(t *testing.T, paths []string, want []expectedChapter) {
	t.Helper()
	for i := 1; i < len(paths); i++ {
		assert.Negative(t, naturalCompare(paths[i-1], paths[i]), "Shisho order: %q before %q", paths[i-1], paths[i])
		assert.Negative(t, mupdfCompare(paths[i-1], paths[i], false), "MuPDF order, unsigned char: %q before %q", paths[i-1], paths[i])
		assert.Negative(t, mupdfCompare(paths[i-1], paths[i], true), "MuPDF order, signed char: %q before %q", paths[i-1], paths[i])
		assert.True(t, sortsBeforeEverywhere(paths[i-1], paths[i]), "Finder-style order: %q before %q", paths[i-1], paths[i])
	}
	got := DetectChapters(paths)
	require.Len(t, got, len(want), "paths: %q", paths)
	for i, ch := range got {
		assert.Equal(t, want[i].title, ch.Title)
		require.NotNil(t, ch.StartPage)
		assert.Equal(t, want[i].startPage, *ch.StartPage)
	}
}

func TestChapterPagePaths(t *testing.T) {
	t.Parallel()

	t.Run("puts each chapter's pages in a folder named after it", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(6), []mediafile.ParsedChapter{
			chapterAt("Chapter 1", 0),
			chapterAt("Chapter 2", 2),
			chapterAt("Chapter 10", 4),
		})
		assert.Equal(t, []string{
			"Chapter 1/0001.jpg",
			"Chapter 1/0002.jpg",
			"Chapter 2/0003.jpg",
			"Chapter 2/0004.jpg",
			"Chapter 10/0005.jpg",
			"Chapter 10/0006.jpg",
		}, paths)
		assertChapterLayout(t, paths, []expectedChapter{
			{title: "Chapter 1", startPage: 0},
			{title: "Chapter 2", startPage: 2},
			{title: "Chapter 10", startPage: 4},
		})
	})

	t.Run("keeps pages before the first chapter at the root", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(5), []mediafile.ParsedChapter{
			chapterAt("Chapter 1", 2),
			chapterAt("Chapter 2", 3),
		})
		assert.Equal(t, []string{
			"0001.jpg",
			"0002.jpg",
			"Chapter 1/0003.jpg",
			"Chapter 2/0004.jpg",
			"Chapter 2/0005.jpg",
		}, paths)
		assertChapterLayout(t, paths, []expectedChapter{
			{title: "Chapter 1", startPage: 2},
			{title: "Chapter 2", startPage: 3},
		})
	})

	t.Run("numbers every folder when the titles do not sort in page order", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(4), []mediafile.ParsedChapter{
			chapterAt("Prologue", 0),
			chapterAt("Chapter 1", 1),
			chapterAt("Epilogue", 3),
		})
		assert.Equal(t, []string{
			"1. Prologue/0001.jpg",
			"2. Chapter 1/0002.jpg",
			"2. Chapter 1/0003.jpg",
			"3. Epilogue/0004.jpg",
		}, paths)
		assertChapterLayout(t, paths, []expectedChapter{
			{title: "Prologue", startPage: 0},
			{title: "Chapter 1", startPage: 1},
			{title: "Epilogue", startPage: 3},
		})
	})

	t.Run("keeps pages before the first chapter first when it numbers the folders", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(4), []mediafile.ParsedChapter{
			chapterAt("Prologue", 2),
			chapterAt("Chapter 1", 3),
		})
		assert.Equal(t, []string{
			"0-0001.jpg",
			"0-0002.jpg",
			"1. Prologue/0003.jpg",
			"2. Chapter 1/0004.jpg",
		}, paths)
		assertChapterLayout(t, paths, []expectedChapter{
			{title: "Prologue", startPage: 2},
			{title: "Chapter 1", startPage: 3},
		})
	})

	t.Run("numbers when a root page would sort after a chapter", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(3), []mediafile.ParsedChapter{
			chapterAt("1 Start", 1),
			chapterAt("2 End", 2),
		})
		assertChapterLayout(t, paths, []expectedChapter{
			{title: "1 Start", startPage: 1},
			{title: "2 End", startPage: 2},
		})
	})

	t.Run("numbers titles that a scan would read as numbered", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(2), []mediafile.ParsedChapter{
			chapterAt("1. Start", 0),
			chapterAt("2. End", 1),
		})
		assert.Equal(t, []string{"1. 1. Start/0001.jpg", "2. 2. End/0002.jpg"}, paths)
		assertChapterLayout(t, paths, []expectedChapter{
			{title: "1. Start", startPage: 0},
			{title: "2. End", startPage: 1},
		})
	})

	t.Run("does not number a single chapter", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(2), []mediafile.ParsedChapter{chapterAt("Chapter 1", 0)})
		assert.Equal(t, []string{"Chapter 1/0001.jpg", "Chapter 1/0002.jpg"}, paths)
	})

	t.Run("numbers titles that differ only in case", func(t *testing.T) {
		t.Parallel()
		// MuPDF compares case-insensitively, so "extra" and "Extra" pages
		// would interleave.
		paths := ChapterPagePaths(flatPages(4), []mediafile.ParsedChapter{
			chapterAt("Extra", 0),
			chapterAt("extra", 2),
		})
		assertChapterLayout(t, paths, []expectedChapter{
			{title: "Extra", startPage: 0},
			{title: "extra", startPage: 2},
		})
	})

	t.Run("numbers non-ASCII titles that sort before a root page where char is signed", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(3), []mediafile.ParsedChapter{
			chapterAt("第1話", 1),
			chapterAt("第2話", 2),
		})
		assertChapterLayout(t, paths, []expectedChapter{
			{title: "第1話", startPage: 1},
			{title: "第2話", startPage: 2},
		})
	})

	t.Run("keeps non-ASCII titles that sort in order", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(2), []mediafile.ParsedChapter{
			chapterAt("第1話", 0),
			chapterAt("第2話", 1),
		})
		assert.Equal(t, []string{"第1話/0001.jpg", "第2話/0002.jpg"}, paths)
	})

	t.Run("numbers titles that sort differently where punctuation sorts before letters", func(t *testing.T) {
		t.Parallel()
		// Finder-style sorting puts "[" before letters and ":" before "/".
		for _, titles := range [][]string{{"Intro", "[Bonus]"}, {"Chapter 1", "Chapter 1: Start"}} {
			paths := ChapterPagePaths(flatPages(2), []mediafile.ParsedChapter{
				chapterAt(titles[0], 0),
				chapterAt(titles[1], 1),
			})
			assert.Equal(t, []string{"1. " + titles[0] + "/0001.jpg", "2. " + titles[1] + "/0002.jpg"}, paths)
		}
	})

	t.Run("keeps titles that start the next one", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(3), []mediafile.ParsedChapter{
			chapterAt("Act I", 0),
			chapterAt("Act II", 1),
			chapterAt("Act III", 2),
		})
		assert.Equal(t, []string{"Act I/0001.jpg", "Act II/0002.jpg", "Act III/0003.jpg"}, paths)
	})

	t.Run("round-trips an untitled chapter when it numbers the folders", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(2), []mediafile.ParsedChapter{
			chapterAt("Prologue", 0),
			chapterAt("", 1),
		})
		assert.Equal(t, []string{"1. Prologue/0001.jpg", "2/0002.jpg"}, paths)
		assertChapterLayout(t, paths, []expectedChapter{
			{title: "Prologue", startPage: 0},
			{title: "", startPage: 1},
		})
	})

	t.Run("numbers a title that starts the previous one", func(t *testing.T) {
		t.Parallel()
		// KOReader's CBZ chapter patch merges a folder into the previous
		// chapter when its name is a prefix of that chapter's name.
		paths := ChapterPagePaths(flatPages(2), []mediafile.ParsedChapter{
			chapterAt("Bonus Part", 0),
			chapterAt("Bonus", 1),
		})
		assert.Equal(t, []string{"1. Bonus Part/0001.jpg", "2. Bonus/0002.jpg"}, paths)
	})

	t.Run("numbers a title MuPDF would strip as a leading dot", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(2), []mediafile.ParsedChapter{
			chapterAt(".hack", 0),
			chapterAt("Other", 1),
		})
		assert.Equal(t, []string{"1. .hack/0001.jpg", "2. Other/0002.jpg"}, paths)
	})

	t.Run("replaces path separators in titles", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(2), []mediafile.ParsedChapter{
			chapterAt("Part 1/2", 0),
			chapterAt(`Part 2\2`, 1),
		})
		assert.Equal(t, []string{"Part 1-2/0001.jpg", "Part 2-2/0002.jpg"}, paths)
	})

	t.Run("widens page numbers past 9999 pages", func(t *testing.T) {
		t.Parallel()
		names := make([]string, 10000)
		for i := range names {
			names[i] = "p.jpg"
		}
		paths := ChapterPagePaths(names, []mediafile.ParsedChapter{
			chapterAt("A", 0),
			chapterAt("B", 9999),
		})
		assert.Equal(t, "A/00001.jpg", paths[0])
		assert.Equal(t, "B/10000.jpg", paths[9999])
	})

	t.Run("keeps each page's extension", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths([]string{"a.PNG", "b.webp"}, []mediafile.ParsedChapter{
			chapterAt("A", 0),
			chapterAt("B", 1),
		})
		assert.Equal(t, []string{"A/0001.PNG", "B/0002.webp"}, paths)
	})

	t.Run("sorts chapters by start page and keeps the first of a shared start", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(4), []mediafile.ParsedChapter{
			chapterAt("Chapter 2", 2),
			chapterAt("Chapter 1", 0),
			chapterAt("Chapter 1 again", 0),
		})
		assertChapterLayout(t, paths, []expectedChapter{
			{title: "Chapter 1", startPage: 0},
			{title: "Chapter 2", startPage: 2},
		})
	})

	t.Run("drops chapters with no start page or one past the last page", func(t *testing.T) {
		t.Parallel()
		paths := ChapterPagePaths(flatPages(3), []mediafile.ParsedChapter{
			chapterAt("Chapter 1", 0),
			{Title: "No page"},
			chapterAt("Chapter 2", 1),
			chapterAt("Too far", 3),
			chapterAt("Negative", -1),
		})
		assertChapterLayout(t, paths, []expectedChapter{
			{title: "Chapter 1", startPage: 0},
			{title: "Chapter 2", startPage: 1},
		})
	})

	t.Run("returns nil when there are no usable chapters", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, ChapterPagePaths(flatPages(3), nil))
		assert.Nil(t, ChapterPagePaths(flatPages(3), []mediafile.ParsedChapter{chapterAt("Too far", 5)}))
	})

	t.Run("returns nil when the folders already hold the chapters", func(t *testing.T) {
		t.Parallel()
		names := []string{"Vol/Ch 1/a.jpg", "Vol/Ch 1/b.jpg", "Vol/Ch 2/c.jpg"}
		assert.Nil(t, ChapterPagePaths(names, []mediafile.ParsedChapter{
			chapterAt("Ch 1", 0),
			chapterAt("Ch 2", 2),
		}))
	})

	t.Run("returns nil when one folder holds the only chapter", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, ChapterPagePaths([]string{"Ch 1/a.jpg", "Ch 1/b.jpg"}, []mediafile.ParsedChapter{chapterAt("Ch 1", 0)}))
		assert.Nil(t, ChapterPagePaths([]string{"cover.jpg", "Ch 1/a.jpg"}, []mediafile.ParsedChapter{chapterAt("Ch 1", 1)}))
	})

	t.Run("returns nil when numbered folders hold the chapters with or without their numbers", func(t *testing.T) {
		t.Parallel()
		// A file scanned before scans stripped folder numbers keeps them in
		// its chapter titles.
		names := []string{"1. Origins/a.jpg", "2. Return/b.jpg"}
		assert.Nil(t, ChapterPagePaths(names, []mediafile.ParsedChapter{
			chapterAt("1. Origins", 0),
			chapterAt("2. Return", 1),
		}))
		assert.Nil(t, ChapterPagePaths(names, []mediafile.ParsedChapter{
			chapterAt("Origins", 0),
			chapterAt("Return", 1),
		}))
	})

	t.Run("rewrites folders whose chapters were edited", func(t *testing.T) {
		t.Parallel()
		names := []string{"Ch 1/a.jpg", "Ch 1/b.jpg", "Ch 2/c.jpg"}
		paths := ChapterPagePaths(names, []mediafile.ParsedChapter{
			chapterAt("Ch 1", 0),
			chapterAt("Renamed", 1),
		})
		assertChapterLayout(t, paths, []expectedChapter{
			{title: "Ch 1", startPage: 0},
			{title: "Renamed", startPage: 1},
		})
	})

	t.Run("rewrites chapters found only in file names", func(t *testing.T) {
		t.Parallel()
		// KOReader shows only folders as chapters.
		names := []string{"ch01_p1.jpg", "ch01_p2.jpg", "ch02_p1.jpg"}
		paths := ChapterPagePaths(names, DetectChapters(names))
		assert.Equal(t, []string{"Chapter 1/0001.jpg", "Chapter 1/0002.jpg", "Chapter 2/0003.jpg"}, paths)
	})
}

func TestDetectChapters_StripsFolderNumbers(t *testing.T) {
	t.Parallel()

	t.Run("strips the numbers ChapterPagePaths writes", func(t *testing.T) {
		t.Parallel()
		chapters := DetectChapters([]string{
			"0-0001.jpg",
			"1. Prologue/0002.jpg",
			"2. Chapter 1/0003.jpg",
			"2. Chapter 1/0004.jpg",
		})
		require.Len(t, chapters, 2)
		assert.Equal(t, "Prologue", chapters[0].Title)
		assert.Equal(t, 1, *chapters[0].StartPage)
		assert.Equal(t, "Chapter 1", chapters[1].Title)
		assert.Equal(t, 2, *chapters[1].StartPage)
	})

	t.Run("keeps numbers that are not 1, 2, 3 in order", func(t *testing.T) {
		t.Parallel()
		chapters := DetectChapters([]string{"1. Prologue/a.jpg", "3. Chapter 1/b.jpg"})
		require.Len(t, chapters, 2)
		assert.Equal(t, "1. Prologue", chapters[0].Title)
		assert.Equal(t, "3. Chapter 1", chapters[1].Title)
	})

	t.Run("keeps numbers on only some folders", func(t *testing.T) {
		t.Parallel()
		chapters := DetectChapters([]string{"1. Prologue/a.jpg", "Chapter 1/b.jpg"})
		require.Len(t, chapters, 2)
		assert.Equal(t, "1. Prologue", chapters[0].Title)
	})

	t.Run("keeps folders named only by a number", func(t *testing.T) {
		t.Parallel()
		chapters := DetectChapters([]string{"1/a.jpg", "2/b.jpg", "3/c.jpg"})
		require.Len(t, chapters, 3)
		assert.Equal(t, "1", chapters[0].Title)
		assert.Equal(t, "2", chapters[1].Title)
		assert.Equal(t, "3", chapters[2].Title)
	})

	t.Run("strips a bare number beside numbered titles", func(t *testing.T) {
		t.Parallel()
		chapters := DetectChapters([]string{"1. Prologue/a.jpg", "2/b.jpg"})
		require.Len(t, chapters, 2)
		assert.Equal(t, "Prologue", chapters[0].Title)
		assert.Empty(t, chapters[1].Title)
	})

	t.Run("keeps zero-padded numbers", func(t *testing.T) {
		t.Parallel()
		chapters := DetectChapters([]string{"01. Origins/a.jpg", "02. Return/b.jpg"})
		require.Len(t, chapters, 2)
		assert.Equal(t, "01. Origins", chapters[0].Title)
		assert.Equal(t, "02. Return", chapters[1].Title)
	})
}

func TestMupdfCompare(t *testing.T) {
	t.Parallel()

	t.Run("ignores ASCII case and compares numbers by value", func(t *testing.T) {
		t.Parallel()
		names := []string{"B/2.jpg", "a/10.jpg", "a/2.jpg", "A/1.jpg"}
		slices.SortFunc(names, func(a, b string) int { return mupdfCompare(a, b, false) })
		assert.Equal(t, []string{"A/1.jpg", "a/2.jpg", "a/10.jpg", "B/2.jpg"}, names)
	})

	t.Run("sorts non-ASCII bytes by the platform's char signedness", func(t *testing.T) {
		t.Parallel()
		assert.Negative(t, mupdfCompare("0001.jpg", "第1話/0002.jpg", false))
		assert.Positive(t, mupdfCompare("0001.jpg", "第1話/0002.jpg", true))
	})
}
