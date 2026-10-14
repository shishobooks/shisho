package cbz

import (
	"cmp"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/shishobooks/shisho/pkg/mediafile"
)

// KOReader shows a CBZ's folders as its chapters: koreader-base patches
// MuPDF (thirdparty/mupdf/mupdf_cbz_chapter_support.patch) to turn every
// folder of pages into a chapter named after the folder. ChapterPagePaths
// lays chapters out that way for downloads, and DetectChapters reads them
// back, so a downloaded CBZ scans with the chapters it was written with.

// ChapterPagePaths returns the archive path for each page, in the order of
// pageNames (the PageImages order), that puts every chapter's pages in a
// folder named after the chapter. Pages before the first chapter stay at
// the root, and every page is named after its 1-indexed page number,
// keeping its extension.
//
// Folders are named after the chapter titles when those paths keep the
// pages in reading order for both PageImages and MuPDF, on every platform
// KOReader runs on, and scan back as the chapters. Otherwise every folder
// is numbered in reading order ("1. Prologue", "2. Chapter 1"), which
// DetectChapters strips, and pages before the first chapter start with
// "0-" so they still sort first. Numbers are visible rather than hidden
// characters because readers that sort like Finder ignore zero-width
// characters but read digits as numbers.
//
// Chapters without a start page, or with one outside the pages, are
// dropped, and of chapters sharing a start page only the first is kept,
// since a folder needs at least one page. It returns nil when no chapter is
// left or when the folders of pageNames already hold exactly these
// chapters, numbered or not, so the archive keeps its names.
func ChapterPagePaths(pageNames []string, chapters []mediafile.ParsedChapter) []string {
	chapters = usableChapters(chapters, len(pageNames))
	// A file scanned before scans stripped folder numbers keeps them in its
	// chapter titles, so its folders match either way. One folder is never a
	// scan's chapter but still matches a single chapter.
	if len(chapters) == 0 ||
		sameChapters(chapters, detectChaptersFromFolders(pageNames)) ||
		sameChapters(chapters, folderChapters(pageNames)) {
		return nil
	}

	folders := make([]string, len(chapters))
	for i, ch := range chapters {
		folders[i] = folderName(ch.Title)
	}
	paths := layoutPages(pageNames, chapters, folders, "")
	if readsInOrder(paths, chapters, folders) {
		return paths
	}

	for i := range folders {
		folders[i] = numberedFolder(i+1, folders[i])
	}
	return layoutPages(pageNames, chapters, folders, "0-")
}

// numberedFolder is the name of the nth folder when folders are numbered.
func numberedFolder(n int, name string) string {
	if name == "" {
		return strconv.Itoa(n)
	}
	return strconv.Itoa(n) + ". " + name
}

// usableChapters returns the chapters that can be folders, sorted by start
// page.
func usableChapters(chapters []mediafile.ParsedChapter, pageCount int) []mediafile.ParsedChapter {
	usable := make([]mediafile.ParsedChapter, 0, len(chapters))
	for _, ch := range chapters {
		if ch.StartPage != nil && *ch.StartPage >= 0 && *ch.StartPage < pageCount {
			usable = append(usable, ch)
		}
	}
	slices.SortStableFunc(usable, func(a, b mediafile.ParsedChapter) int {
		return cmp.Compare(*a.StartPage, *b.StartPage)
	})
	return slices.CompactFunc(usable, func(a, b mediafile.ParsedChapter) bool {
		return *a.StartPage == *b.StartPage
	})
}

func sameChapters(a, b []mediafile.ParsedChapter) bool {
	return slices.EqualFunc(a, b, func(x, y mediafile.ParsedChapter) bool {
		return x.Title == y.Title && *x.StartPage == *y.StartPage
	})
}

// pageNumberWidth is how many digits page names are padded to.
func pageNumberWidth(pageCount int) int {
	return max(4, len(strconv.Itoa(pageCount)))
}

// folderName makes a chapter title usable as one folder name.
func folderName(title string) string {
	title = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\':
			return '-'
		case r < ' ' || r == 0x7f:
			return ' '
		}
		return r
	}, title)
	return strings.TrimSpace(title)
}

// layoutPages names each page after its page number, in its chapter's
// folder, or at the root with rootPrefix before the first chapter.
func layoutPages(pageNames []string, chapters []mediafile.ParsedChapter, folders []string, rootPrefix string) []string {
	width := pageNumberWidth(len(pageNames))
	paths := make([]string, len(pageNames))
	chapter := -1
	for i, name := range pageNames {
		for chapter+1 < len(chapters) && *chapters[chapter+1].StartPage <= i {
			chapter++
		}
		page := fmt.Sprintf("%0*d", width, i+1) + path.Ext(name)
		if chapter >= 0 {
			page = folders[chapter] + "/" + page
		} else {
			page = rootPrefix + page
		}
		paths[i] = page
	}
	return paths
}

// readsInOrder reports whether the folders read back as the chapters, in
// Shisho and in KOReader, with the pages in reading order.
func readsInOrder(paths []string, chapters []mediafile.ParsedChapter, folders []string) bool {
	for i, folder := range folders {
		// MuPDF strips a leading "./" or "." from page names, and
		// PageImages skips pages under __MACOSX.
		if folder == "" || strings.HasPrefix(folder, ".") || folder == "__MACOSX" {
			return false
		}
		for _, other := range folders[:i] {
			if strings.EqualFold(folder, other) {
				return false
			}
		}
		// KOReader's patch takes a folder whose name starts the previous
		// chapter's name for that chapter.
		if i > 0 && strings.HasPrefix(folders[i-1], folder) {
			return false
		}
	}
	for i := 1; i < len(paths); i++ {
		if naturalCompare(paths[i-1], paths[i]) >= 0 ||
			mupdfCompare(paths[i-1], paths[i], false) >= 0 ||
			mupdfCompare(paths[i-1], paths[i], true) >= 0 ||
			!sortsBeforeEverywhere(paths[i-1], paths[i]) {
			return false
		}
	}
	// A scan finds chapters only in two or more folders, and strips
	// numbers from folders that read as numbered.
	if len(chapters) < 2 {
		return true
	}
	named := make([]mediafile.ParsedChapter, len(chapters))
	for i, ch := range chapters {
		named[i] = mediafile.ParsedChapter{Title: folders[i], StartPage: ch.StartPage}
	}
	return sameChapters(named, detectChaptersFromFolders(paths))
}

// stripFolderNumbers returns the chapters, in reading order, without the
// folder numbers ChapterPagePaths adds. The numbers are stripped only when
// every folder is numbered 1, 2, 3, and so on, unpadded, and at least one
// has a title after its number, so folders a person numbered differently,
// or named only by number like a series' volumes, keep their names.
func stripFolderNumbers(chapters []mediafile.ParsedChapter) []mediafile.ParsedChapter {
	stripped := slices.Clone(chapters)
	titled := false
	for i, ch := range chapters {
		n := strconv.Itoa(i + 1)
		switch {
		case ch.Title == n:
			stripped[i].Title = ""
		case strings.HasPrefix(ch.Title, n+". "):
			stripped[i].Title = ch.Title[len(n)+2:]
			titled = true
		default:
			return chapters
		}
	}
	if !titled {
		return chapters
	}
	return stripped
}

// sortsBeforeEverywhere reports whether a sorts before b in sorters that
// order names like Finder, which put punctuation and spaces before digits
// and digits before letters, compare digit runs by value, and ignore case
// and accents. A folder name ending where the next one goes on with a
// letter or digit ("Act I/" before "Act II/") sorts first for the same
// reason. Ordering among punctuation and non-ASCII letters is left
// unmodeled: a first difference there reports false.
func sortsBeforeEverywhere(a, b string) bool {
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	isLetter := func(c byte) bool { return (c|0x20) >= 'a' && (c|0x20) <= 'z' }
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case isDigit(a[i]) && isDigit(b[j]):
			x, y := i, j
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			if an, bn := strings.TrimLeft(a[x:i], "0"), strings.TrimLeft(b[y:j], "0"); an != bn {
				return len(an) < len(bn) || len(an) == len(bn) && an < bn
			}
		case a[i] == b[j] || isLetter(a[i]) && isLetter(b[j]) && a[i]|0x20 == b[j]|0x20:
			i++
			j++
		case isLetter(a[i]) && isLetter(b[j]):
			return a[i]|0x20 < b[j]|0x20
		case isDigit(a[i]) && isLetter(b[j]), a[i] == '/' && (isLetter(b[j]) || isDigit(b[j])):
			return true
		default:
			return false
		}
	}
	return i == len(a) && j < len(b)
}

// mupdfCompare orders page names the way MuPDF's cbz_strnatcmp does, which
// is how KOReader orders a CBZ's pages: ASCII letters compare without case,
// and digit runs compare by value. Other bytes compare as C chars, so
// whether a non-ASCII byte sorts before or after ASCII depends on the
// platform: char is unsigned on ARM Linux, as on a Kobo, and signed on x86
// and Apple platforms.
func mupdfCompare(a, b string, signedChar bool) int {
	at := func(s string, i int) int {
		if i >= len(s) {
			return 0
		}
		if signedChar {
			return int(int8(s[i])) //nolint:gosec // reinterpreting the byte as a signed C char is the point
		}
		return int(s[i])
	}
	isDigit := func(s string, i int) bool {
		return i < len(s) && s[i] >= '0' && s[i] <= '9'
	}
	upper := func(c int) int {
		if c >= 'a' && c <= 'z' {
			return c - 'a' + 'A'
		}
		return c
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		var x, y int
		if isDigit(a, i) && isDigit(b, j) {
			for ; isDigit(a, i); i++ {
				x = x*10 + int(a[i]-'0')
			}
			for ; isDigit(b, j); j++ {
				y = y*10 + int(b[j]-'0')
			}
		} else {
			x, y = upper(at(a, i)), upper(at(b, j))
			i++
			j++
		}
		if x != y {
			return cmp.Compare(x, y)
		}
	}
	return 0
}
