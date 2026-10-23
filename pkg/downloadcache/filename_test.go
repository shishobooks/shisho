package downloadcache

import (
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
)

func authors(names ...string) []*models.Author {
	out := make([]*models.Author, len(names))
	for i, name := range names {
		out[i] = &models.Author{SortOrder: i, Person: &models.Person{Name: name}}
	}
	return out
}

func series(name string, number, numberEnd *float64) []*models.BookSeries {
	return []*models.BookSeries{{SortOrder: 0, SeriesNumber: number, SeriesNumberEnd: numberEnd, Series: &models.Series{Name: name}}}
}

func TestDownloadFilenames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		book          *models.Book
		file          *models.File
		deliveredType string
		expected      string
	}{
		// Shape, the same for every delivered File Type.
		{
			name:          "epub with author, series, and title",
			book:          &models.Book{Title: "The Way of Kings", Authors: authors("Brandon Sanderson"), BookSeries: series("The Stormlight Archive", pointerutil.Float64(1), nil)},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings.epub",
		},
		{
			name:          "kepub with author, series, and title",
			book:          &models.Book{Title: "Harry Potter and the Sorcerer's Stone", Authors: authors("J.K. Rowling"), BookSeries: series("Harry Potter", pointerutil.Float64(1), nil)},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeKePub,
			expected:      "J.K. Rowling - Harry Potter 001 - Harry Potter and the Sorcerer's Stone.kepub.epub",
		},
		{
			name:          "cbz",
			book:          &models.Book{Title: "Chapter One", Authors: authors("Author"), BookSeries: series("Story", pointerutil.Float64(1), nil)},
			file:          &models.File{FileType: models.FileTypeCBZ},
			deliveredType: models.FileTypeCBZ,
			expected:      "Author - Story 001 - Chapter One.cbz",
		},
		{
			name:          "pdf",
			book:          &models.Book{Title: "Manual", Authors: authors("Author")},
			file:          &models.File{FileType: models.FileTypePDF},
			deliveredType: models.FileTypePDF,
			expected:      "Author - Manual.pdf",
		},
		{
			name:          "mobi",
			book:          &models.Book{Title: "Novel", Authors: authors("Author")},
			file:          &models.File{FileType: models.FileTypeMOBI},
			deliveredType: models.FileTypeMOBI,
			expected:      "Author - Novel.mobi",
		},
		{
			name:          "azw3",
			book:          &models.Book{Title: "Novel", Authors: authors("Author")},
			file:          &models.File{FileType: models.FileTypeAZW3},
			deliveredType: models.FileTypeAZW3,
			expected:      "Author - Novel.azw3",
		},
		{
			name:          "a file delivered as another File Type takes that type's extension",
			book:          &models.Book{Title: "Comic", Authors: authors("Author")},
			file:          &models.File{FileType: models.FileTypeCBZ},
			deliveredType: models.FileTypeEPUB,
			expected:      "Author - Comic.epub",
		},
		{
			name:          "a KePub delivered as KePub keeps .kepub.epub",
			book:          &models.Book{Title: "Book Title", Authors: authors("Author")},
			file:          &models.File{FileType: models.FileTypeKePub},
			deliveredType: models.FileTypeKePub,
			expected:      "Author - Book Title.kepub.epub",
		},
		{
			name:          "no series",
			book:          &models.Book{Title: "1984", Authors: authors("George Orwell")},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "George Orwell - 1984.epub",
		},
		{
			name:          "no author",
			book:          &models.Book{Title: "Anonymous Work", BookSeries: series("Mystery Series", pointerutil.Float64(2), nil)},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "Mystery Series 002 - Anonymous Work.epub",
		},
		{
			name:          "no author and no series",
			book:          &models.Book{Title: "Just a Title"},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeKePub,
			expected:      "Just a Title.kepub.epub",
		},
		{
			name:          "series without number",
			book:          &models.Book{Title: "The Book", Authors: authors("Some Author"), BookSeries: series("Some Series", nil, nil)},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "Some Author - Some Series - The Book.epub",
		},
		{
			name:          "decimal series number",
			book:          &models.Book{Title: "Interlude", Authors: authors("Author"), BookSeries: series("Series", pointerutil.Float64(1.5), nil)},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "Author - Series 001.5 - Interlude.epub",
		},
		{
			name:          "whole number series displays without decimal",
			book:          &models.Book{Title: "Book", Authors: authors("Author"), BookSeries: series("Series", pointerutil.Float64(10.0), nil)},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "Author - Series 010 - Book.epub",
		},
		{
			name: "multiple authors and series pick the first by sort order",
			book: &models.Book{
				Title: "Crossover",
				Authors: []*models.Author{
					{SortOrder: 1, Person: &models.Person{Name: "Second Author"}},
					{SortOrder: 0, Person: &models.Person{Name: "First Author"}},
				},
				BookSeries: []*models.BookSeries{
					{SortOrder: 1, SeriesNumber: pointerutil.Float64(5), Series: &models.Series{Name: "Second Series"}},
					{SortOrder: 0, SeriesNumber: pointerutil.Float64(3), Series: &models.Series{Name: "First Series"}},
				},
			},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "First Author - First Series 003 - Crossover.epub",
		},

		// Narrator.
		{
			name:          "m4b with series and narrator",
			book:          &models.Book{Title: "Project Hail Mary", Authors: authors("Andy Weir"), BookSeries: series("Standalone", pointerutil.Float64(1), nil)},
			file:          &models.File{FileType: models.FileTypeM4B, Narrators: []*models.Narrator{{SortOrder: 0, Person: &models.Person{Name: "Ray Porter"}}}},
			deliveredType: models.FileTypeM4B,
			expected:      "Andy Weir - Standalone 001 - Project Hail Mary (Ray Porter).m4b",
		},
		{
			name:          "m4b without narrator",
			book:          &models.Book{Title: "Audiobook", Authors: authors("Some Author")},
			file:          &models.File{FileType: models.FileTypeM4B},
			deliveredType: models.FileTypeM4B,
			expected:      "Some Author - Audiobook.m4b",
		},
		{
			name: "m4b multiple narrators pick the first by sort order",
			book: &models.Book{Title: "Multi-Narrator Book", Authors: authors("Author")},
			file: &models.File{FileType: models.FileTypeM4B, Narrators: []*models.Narrator{
				{SortOrder: 1, Person: &models.Person{Name: "Second Narrator"}},
				{SortOrder: 0, Person: &models.Person{Name: "First Narrator"}},
			}},
			deliveredType: models.FileTypeM4B,
			expected:      "Author - Multi-Narrator Book (First Narrator).m4b",
		},
		{
			name:          "unsafe characters in narrator name are removed",
			book:          &models.Book{Title: "Audiobook", Authors: authors("Author")},
			file:          &models.File{FileType: models.FileTypeM4B, Narrators: []*models.Narrator{{SortOrder: 0, Person: &models.Person{Name: "Narrator/Reader"}}}},
			deliveredType: models.FileTypeM4B,
			expected:      "Author - Audiobook (NarratorReader).m4b",
		},

		// Unsafe characters.
		{
			name:          "filesystem-unsafe characters are removed",
			book:          &models.Book{Title: `Title: With <Special> *Characters*? "Quoted" a|b`, Authors: authors("Author/Writer"), BookSeries: series(`Series\ The Saga`, pointerutil.Float64(1), nil)},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "AuthorWriter - Series The Saga 001 - Title With Special Characters Quoted ab.epub",
		},
		{
			name:          "brackets, braces, and hash are removed",
			book:          &models.Book{Title: "[Draft] Notes {v2} #1", Authors: authors("[Author]")},
			file:          &models.File{FileType: models.FileTypePDF},
			deliveredType: models.FileTypePDF,
			expected:      "Author - Draft Notes v002 1.pdf",
		},
		{
			name:          "control characters and shell substitution characters are removed, whitespace collapses",
			book:          &models.Book{Title: "  Price\tof $5 `cmd`\n100%\x00  ", Authors: authors("Author")},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "Author - Price of 5 cmd 100.epub",
		},
		{
			name:          "bidirectional override characters are removed",
			book:          &models.Book{Title: "Evil\u202Ebupe.exe"},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "Evilbupe.exe.epub",
		},

		// Accents and letters that don't decompose.
		{
			name:          "accented letters become plain for epub",
			book:          &models.Book{Title: "Les Misérables", Authors: authors("Gabriel García Márquez"), BookSeries: series("Año Niño", pointerutil.Float64(1), nil)},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "Gabriel Garcia Marquez - Ano Nino 001 - Les Miserables.epub",
		},
		{
			name:          "accented letters become plain for kepub",
			book:          &models.Book{Title: `Title: "Quoted" Ünïcode`, Authors: authors("Dvořák Ŝtefan")},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeKePub,
			expected:      "Dvorak Stefan - Title Quoted Unicode.kepub.epub",
		},
		{
			name:          "letters that don't decompose are spelled out",
			book:          &models.Book{Title: "Straße Æsir Øresund Łódź Œuvre Đorđe Þór", Authors: authors("Ærø")},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "Aero - Strasse Aesir Oresund Lodz Oeuvre Dorde Thor.epub",
		},
		{
			name:          "letters that don't decompose are spelled out for kepub",
			book:          &models.Book{Title: "Straße æble smørrebrød"},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeKePub,
			expected:      "Strasse aeble smorrebrod.kepub.epub",
		},
		{
			name:          "typographic apostrophes survive kepub as plain ones",
			book:          &models.Book{Title: "Harry’s Book"},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeKePub,
			expected:      "Harry's Book.kepub.epub",
		},

		// Other scripts.
		{
			name:          "japanese is kept for epub",
			book:          &models.Book{Title: "ガンダム がっこう", Authors: authors("著者"), BookSeries: series("進撃の巨人", pointerutil.Float64(3), nil)},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "著者 - 進撃の巨人 003 - ガンダム がっこう.epub",
		},
		{
			name:          "cyrillic is kept for epub",
			book:          &models.Book{Title: "Война и мир, й", Authors: authors("Лев Толстой")},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "Лев Толстой - Война и мир, й.epub",
		},
		{
			name:          "devanagari keeps its combining marks",
			book:          &models.Book{Title: "हिन्दी किताब"},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "हिन्दी किताब.epub",
		},
		{
			name:          "japanese title falls back to the file id for kepub, empty author and series are omitted",
			book:          &models.Book{ID: 7, Title: "ガンダム", Authors: authors("著者"), BookSeries: series("進撃の巨人", pointerutil.Float64(3), nil)},
			file:          &models.File{ID: 42, FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeKePub,
			expected:      "File 42.kepub.epub",
		},
		{
			name:          "cyrillic title falls back to the file id for kepub, latin author kept",
			book:          &models.Book{Title: "Война и мир", Authors: authors("Leo Tolstoy")},
			file:          &models.File{ID: 9, FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeKePub,
			expected:      "Leo Tolstoy - File 9.kepub.epub",
		},
		{
			name:          "a title of only unsafe characters falls back to the file id",
			book:          &models.Book{Title: "???"},
			file:          &models.File{ID: 5, FileType: models.FileTypePDF},
			deliveredType: models.FileTypePDF,
			expected:      "File 5.pdf",
		},

		// Volume markers and omnibus ranges.
		{
			name:          "title with v1 skips series and pads to 3 digits",
			book:          &models.Book{Title: "My Manga v1", Authors: authors("Author"), BookSeries: series("My Manga", pointerutil.Float64(1), nil)},
			file:          &models.File{FileType: models.FileTypeCBZ},
			deliveredType: models.FileTypeCBZ,
			expected:      "Author - My Manga v001.cbz",
		},
		{
			name:          "title with v1 skips series for kepub",
			book:          &models.Book{Title: "My Manga v1", Authors: authors("Author"), BookSeries: series("My Manga", pointerutil.Float64(1), nil)},
			file:          &models.File{FileType: models.FileTypeCBZ},
			deliveredType: models.FileTypeKePub,
			expected:      "Author - My Manga v001.kepub.epub",
		},
		{
			name:          "title with vol. 2 pads to 3 digits",
			book:          &models.Book{Title: "Comic Title vol. 2", Authors: authors("Author"), BookSeries: series("Comic Title", pointerutil.Float64(2), nil)},
			file:          &models.File{FileType: models.FileTypeCBZ},
			deliveredType: models.FileTypeCBZ,
			expected:      "Author - Comic Title vol. 002.cbz",
		},
		{
			name:          "title with Vol 10 pads to 3 digits",
			book:          &models.Book{Title: "Manga Vol 10", Authors: authors("Artist"), BookSeries: series("Manga", pointerutil.Float64(10), nil)},
			file:          &models.File{FileType: models.FileTypeCBZ},
			deliveredType: models.FileTypeCBZ,
			expected:      "Artist - Manga Vol 010.cbz",
		},
		{
			name:          "title with V3 uppercase pads to 3 digits",
			book:          &models.Book{Title: "Comic V3", Authors: authors("Author"), BookSeries: series("Comic", pointerutil.Float64(3), nil)},
			file:          &models.File{FileType: models.FileTypeCBZ},
			deliveredType: models.FileTypeCBZ,
			expected:      "Author - Comic V003.cbz",
		},
		{
			name:          "title with v100 keeps 3 digits",
			book:          &models.Book{Title: "Long Series v100", Authors: authors("Author"), BookSeries: series("Long Series", pointerutil.Float64(100), nil)},
			file:          &models.File{FileType: models.FileTypeCBZ},
			deliveredType: models.FileTypeCBZ,
			expected:      "Author - Long Series v100.cbz",
		},
		{
			name:          "omnibus series range",
			book:          &models.Book{Title: "Collected Stories", BookSeries: series("Series", pointerutil.Float64(1), pointerutil.Float64(3))},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "Series 001-003 - Collected Stories.epub",
		},
		{
			name:          "omnibus range remains visible when title has a volume marker",
			book:          &models.Book{Title: "Series v1", BookSeries: series("Series", pointerutil.Float64(1), pointerutil.Float64(3))},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "Series 001-003 - Series v001.epub",
		},
		{
			name:          "omnibus range remains visible for kepub",
			book:          &models.Book{Title: "My Manga v1", Authors: authors("Author"), BookSeries: series("My Manga", pointerutil.Float64(1), pointerutil.Float64(3))},
			file:          &models.File{FileType: models.FileTypeCBZ},
			deliveredType: models.FileTypeKePub,
			expected:      "Author - My Manga 001-003 - My Manga v001.kepub.epub",
		},

		// Title source.
		{
			name:          "file.Name is preferred over book.Title",
			book:          &models.Book{Title: "Book Title", Authors: authors("Author")},
			file:          &models.File{FileType: models.FileTypeEPUB, Name: pointerutil.String("File Name Override")},
			deliveredType: models.FileTypeEPUB,
			expected:      "Author - File Name Override.epub",
		},
		{
			name:          "file.Name with volume indicator skips series",
			book:          &models.Book{Title: "Book Title", Authors: authors("Author"), BookSeries: series("Series", pointerutil.Float64(1), nil)},
			file:          &models.File{FileType: models.FileTypeCBZ, Name: pointerutil.String("Comic v1")},
			deliveredType: models.FileTypeKePub,
			expected:      "Author - Comic v001.kepub.epub",
		},
		{
			name:          "empty file.Name falls back to book.Title",
			book:          &models.Book{Title: "Book Title", Authors: authors("Author")},
			file:          &models.File{FileType: models.FileTypeEPUB, Name: pointerutil.String("")},
			deliveredType: models.FileTypeEPUB,
			expected:      "Author - Book Title.epub",
		},
		{
			name:          "a title of only dots falls back to the file",
			book:          &models.Book{Title: "..."},
			file:          &models.File{ID: 7, FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "File 7.epub",
		},
		{
			name:          "a leading dot is dropped, so the file is not hidden",
			book:          &models.Book{Title: ".hack Infection"},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "hack Infection.epub",
		},
		{
			name:          "a name Windows reserves gets a prefix",
			book:          &models.Book{Title: "Con"},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeEPUB,
			expected:      "_Con.epub",
		},
		{
			name:          "a reserved name is matched in any case, for KePub too",
			book:          &models.Book{Title: "nul"},
			file:          &models.File{FileType: models.FileTypeEPUB},
			deliveredType: models.FileTypeKePub,
			expected:      "_nul.kepub.epub",
		},
		{
			name:          "a numbered device name is reserved",
			book:          &models.Book{Title: "COM1"},
			file:          &models.File{FileType: models.FileTypePDF},
			deliveredType: models.FileTypePDF,
			expected:      "_COM1.pdf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			name, _ := DownloadFilenames(tt.book, tt.file, tt.deliveredType)
			assert.Equal(t, tt.expected, name)
		})
	}
}

// A name longer than filesystems allow is shortened, title first, at a
// character boundary, keeping its extension.
func TestDownloadFilename_LongNamesFit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		book          *models.Book
		deliveredType string
		prefix        string
		suffix        string
	}{
		{
			name:          "a long Latin title",
			book:          &models.Book{Title: strings.Repeat("Long Title ", 40), Authors: authors("Author"), BookSeries: series("Series", pointerutil.Float64(1), nil)},
			deliveredType: models.FileTypeEPUB,
			prefix:        "Author - Series 001 - Long Title Long",
			suffix:        ".epub",
		},
		{
			name:          "a long Japanese title, three bytes a character",
			book:          &models.Book{Title: strings.Repeat("進撃の巨人", 30), Authors: authors("諫山創")},
			deliveredType: models.FileTypeEPUB,
			prefix:        "諫山創 - 進撃の巨人",
			suffix:        ".epub",
		},
		{
			name:          "a long series and title in a KePub",
			book:          &models.Book{Title: strings.Repeat("Title ", 60), Authors: authors("Author"), BookSeries: series(strings.Repeat("Series ", 60), pointerutil.Float64(2), nil)},
			deliveredType: models.FileTypeKePub,
			prefix:        "Author - Series",
			suffix:        ".kepub.epub",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, _ := DownloadFilenames(tt.book, &models.File{FileType: models.FileTypeEPUB}, tt.deliveredType)
			assert.LessOrEqual(t, len(got), maxFilenameBytes)
			assert.True(t, utf8.ValidString(got), got)
			assert.True(t, strings.HasPrefix(got, tt.prefix), got)
			assert.True(t, strings.HasSuffix(got, tt.suffix), got)
			assert.NotContains(t, got, " .", "no space is left before the extension")
		})
	}
}

// Every download also has a name in ASCII alone, for clients that ignore
// the UTF-8 one. It is built from the same parts with the KePub character
// rules, so it matches the KePub name and never leaves a dangling dash.
func TestDownloadFilenames_ASCII(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		book          *models.Book
		deliveredType string
		want          string
		wantASCII     string
	}{
		{
			name:          "a Japanese title",
			book:          &models.Book{Title: "吾輩は猫である", Authors: authors("Soseki Natsume")},
			deliveredType: models.FileTypeEPUB,
			want:          "Soseki Natsume - 吾輩は猫である.epub",
			wantASCII:     "Soseki Natsume - File 3.epub",
		},
		{
			name:          "an accented title",
			book:          &models.Book{Title: "Les Misérables", Authors: authors("Victor Hugo")},
			deliveredType: models.FileTypePDF,
			want:          "Victor Hugo - Les Miserables.pdf",
			wantASCII:     "Victor Hugo - Les Miserables.pdf",
		},
		{
			name:          "punctuation only KePub names keep",
			book:          &models.Book{Title: "Pride & Prejudice!", Authors: authors("Jane Austen"), BookSeries: series("Classics #1", pointerutil.Float64(2), nil)},
			deliveredType: models.FileTypeEPUB,
			want:          "Jane Austen - Classics 1 002 - Pride & Prejudice!.epub",
			wantASCII:     "Jane Austen - Classics 1 002 - Pride Prejudice.epub",
		},
		{
			name:          "a KePub's names are the same",
			book:          &models.Book{Title: "吾輩は猫である", Authors: authors("夏目漱石")},
			deliveredType: models.FileTypeKePub,
			want:          "File 3.kepub.epub",
			wantASCII:     "File 3.kepub.epub",
		},
		{
			name:          "a narrator in another script is left out",
			book:          &models.Book{Title: "Book", Authors: authors("Author")},
			deliveredType: models.FileTypeM4B,
			want:          "Author - Book (Нарратор).m4b",
			wantASCII:     "Author - Book.m4b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			file := &models.File{ID: 3, FileType: tt.deliveredType}
			if tt.deliveredType == models.FileTypeM4B {
				file.Narrators = []*models.Narrator{{Person: &models.Person{Name: "Нарратор"}}}
			}
			name, ascii := DownloadFilenames(tt.book, file, tt.deliveredType)
			assert.Equal(t, tt.want, name)
			assert.Equal(t, tt.wantASCII, ascii)
		})
	}
}

// Series numbers are padded to three digits, as volume numbers in titles
// are, so a series' files sort in order by name.
func TestDownloadFilenames_PadsSeriesNumbers(t *testing.T) {
	t.Parallel()
	for number, want := range map[float64]string{
		2:    "Series 002",
		10:   "Series 010",
		1.5:  "Series 001.5",
		1000: "Series 1000",
		0:    "Series 000",
	} {
		book := &models.Book{Title: "Book", BookSeries: series("Series", pointerutil.Float64(number), nil)}
		name, _ := DownloadFilenames(book, &models.File{FileType: models.FileTypeEPUB}, models.FileTypeEPUB)
		assert.Equal(t, want+" - Book.epub", name)
	}
	assert.Equal(t, "000", padSeriesNumber(math.Copysign(0, -1)), "negative zero")
	assert.Equal(t, "-2", padSeriesNumber(-2), "a negative number is left as it is")
}

// An Original File downloads under the same name as a Generated one, with
// its own extension, and a supplement under its book's name followed by its
// own, so a book's files sort together.
func TestOriginalFilenames(t *testing.T) {
	t.Parallel()
	book := &models.Book{Title: "The Way of Kings", Authors: authors("Brandon Sanderson"), BookSeries: series("The Stormlight Archive", pointerutil.Float64(1), nil)}
	tests := []struct {
		name      string
		book      *models.Book
		file      *models.File
		ext       string
		want      string
		wantASCII string
	}{
		{
			name:      "a main file",
			book:      book,
			file:      &models.File{ID: 5, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: "/lib/kings.epub"},
			ext:       ".epub",
			want:      "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings.epub",
			wantASCII: "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings.epub",
		},
		{
			name:      "a main file keeps a bare .kepub extension",
			book:      book,
			file:      &models.File{ID: 5, FileType: models.FileTypeKePub, FileRole: models.FileRoleMain, Filepath: "/lib/kings.kepub"},
			ext:       ".kepub",
			want:      "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings.kepub",
			wantASCII: "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings.kepub",
		},
		{
			name:      "a main file with a Japanese title",
			book:      &models.Book{Title: "吾輩は猫である", Authors: authors("Soseki Natsume")},
			file:      &models.File{ID: 5, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: "/lib/neko.epub"},
			ext:       ".epub",
			want:      "Soseki Natsume - 吾輩は猫である.epub",
			wantASCII: "Soseki Natsume - File 5.epub",
		},
		{
			name:      "a supplement follows its book's name",
			book:      book,
			file:      &models.File{ID: 9, FileType: "pdf", FileRole: models.FileRoleSupplement, Filepath: "/lib/Supplement.pdf"},
			ext:       ".pdf",
			want:      "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings - Supplement.pdf",
			wantASCII: "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings - Supplement.pdf",
		},
		{
			name:      "an original KePub's names are both Kobo-safe",
			book:      &models.Book{Title: "吾輩は猫である", Authors: authors("Soseki Natsume")},
			file:      &models.File{ID: 5, FileType: models.FileTypeKePub, FileRole: models.FileRoleMain, Filepath: "/lib/neko.kepub"},
			ext:       ".kepub.epub",
			want:      "Soseki Natsume - File 5.kepub.epub",
			wantASCII: "Soseki Natsume - File 5.kepub.epub",
		},
		{
			name:      "a supplement's stored Name from the scanner is not used",
			book:      book,
			file:      &models.File{ID: 9, FileType: "pdf", FileRole: models.FileRoleSupplement, Filepath: "/lib/Map.pdf", Name: pointerutil.String("Old Stem"), NameSource: pointerutil.String(models.DataSourceFilepath)},
			ext:       ".pdf",
			want:      "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings - Map.pdf",
			wantASCII: "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings - Map.pdf",
		},
		{
			name:      "a KePub supplement's two-part extension is not doubled",
			book:      book,
			file:      &models.File{ID: 9, FileType: models.FileTypeKePub, FileRole: models.FileRoleSupplement, Filepath: "/lib/Guide.kepub.epub"},
			ext:       ".kepub.epub",
			want:      "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings - Guide.kepub.epub",
			wantASCII: "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings - Guide.kepub.epub",
		},
		{
			name:      "a supplement named after its book does not repeat the title",
			book:      book,
			file:      &models.File{ID: 9, FileType: "pdf", FileRole: models.FileRoleSupplement, Filepath: "/lib/The Way of Kings - Map.pdf"},
			ext:       ".pdf",
			want:      "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings - Map.pdf",
			wantASCII: "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings - Map.pdf",
		},
		{
			name:      "a supplement whose name only starts like the title keeps it",
			book:      book,
			file:      &models.File{ID: 9, FileType: "pdf", FileRole: models.FileRoleSupplement, Filepath: "/lib/The Way of Kingsley.pdf"},
			ext:       ".pdf",
			want:      "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings - The Way of Kingsley.pdf",
			wantASCII: "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings - The Way of Kingsley.pdf",
		},
		{
			name:      "a supplement's name comes from its Name when the user set it",
			book:      book,
			file:      &models.File{ID: 9, FileType: "pdf", FileRole: models.FileRoleSupplement, Filepath: "/lib/x.pdf", Name: pointerutil.String("Map of Roshar"), NameSource: pointerutil.String(models.DataSourceManual)},
			ext:       ".pdf",
			want:      "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings - Map of Roshar.pdf",
			wantASCII: "Brandon Sanderson - The Stormlight Archive 001 - The Way of Kings - Map of Roshar.pdf",
		},
		{
			name:      "a supplement with nothing left in ASCII falls back to the file",
			book:      &models.Book{Title: "Book", Authors: authors("Author")},
			file:      &models.File{ID: 9, FileType: "txt", FileRole: models.FileRoleSupplement, Filepath: "/lib/解説.txt"},
			ext:       ".txt",
			want:      "Author - Book - 解説.txt",
			wantASCII: "Author - Book - File 9.txt",
		},
		{
			name:      "a supplement keeps the book's title, not a file name",
			book:      &models.Book{Title: "Manga v1", Authors: authors("Author"), BookSeries: series("Manga", pointerutil.Float64(1), nil)},
			file:      &models.File{ID: 9, FileType: "jpg", FileRole: models.FileRoleSupplement, Filepath: "/lib/cover art.jpg"},
			ext:       ".jpg",
			want:      "Author - Manga v001 - cover art.jpg",
			wantASCII: "Author - Manga v001 - cover art.jpg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			name, ascii := OriginalFilenames(tt.book, tt.file, tt.ext)
			assert.Equal(t, tt.want, name)
			assert.Equal(t, tt.wantASCII, ascii)
		})
	}
}

// A long supplement name is shortened with the book's title, and the name
// fits.
func TestOriginalFilenames_LongSupplementFits(t *testing.T) {
	t.Parallel()
	book := &models.Book{Title: strings.Repeat("Long Title ", 30), Authors: authors("Author"), BookSeries: series("Series", pointerutil.Float64(1), nil)}
	file := &models.File{ID: 9, FileType: "pdf", FileRole: models.FileRoleSupplement, Filepath: "/lib/" + strings.Repeat("Appendix ", 40) + ".pdf"}
	name, ascii := OriginalFilenames(book, file, ".pdf")
	for _, got := range []string{name, ascii} {
		assert.LessOrEqual(t, len(got), maxFilenameBytes, got)
		assert.True(t, strings.HasSuffix(got, ".pdf"), got)
		assert.Contains(t, got, " - Appendix", "the supplement's own name survives")
	}
}
