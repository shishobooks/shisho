package downloadcache

import (
	"testing"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestFormatDownloadFilename(t *testing.T) {
	tests := []struct {
		name     string
		book     *models.Book
		file     *models.File
		expected string
	}{
		{
			name: "full format with author, series, and title",
			book: &models.Book{
				Title: "The Way of Kings",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "Brandon Sanderson"}},
				},
				BookSeries: []*models.BookSeries{
					{SortOrder: 0, SeriesNumber: pointerutil.Float64(1), Series: &models.Series{Name: "The Stormlight Archive"}},
				},
			},
			file:     &models.File{FileType: "epub"},
			expected: "[Brandon Sanderson] The Stormlight Archive #1 - The Way of Kings.epub",
		},
		{
			name: "no series",
			book: &models.Book{
				Title: "1984",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "George Orwell"}},
				},
				BookSeries: nil,
			},
			file:     &models.File{FileType: "epub"},
			expected: "[George Orwell] 1984.epub",
		},
		{
			name: "no author",
			book: &models.Book{
				Title:   "Anonymous Work",
				Authors: nil,
				BookSeries: []*models.BookSeries{
					{SortOrder: 0, SeriesNumber: pointerutil.Float64(2), Series: &models.Series{Name: "Mystery Series"}},
				},
			},
			file:     &models.File{FileType: "epub"},
			expected: "Mystery Series #2 - Anonymous Work.epub",
		},
		{
			name: "no author and no series",
			book: &models.Book{
				Title:      "Just a Title",
				Authors:    nil,
				BookSeries: nil,
			},
			file:     &models.File{FileType: "epub"},
			expected: "Just a Title.epub",
		},
		{
			name: "series without number",
			book: &models.Book{
				Title: "The Book",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "Some Author"}},
				},
				BookSeries: []*models.BookSeries{
					{SortOrder: 0, SeriesNumber: nil, Series: &models.Series{Name: "Some Series"}},
				},
			},
			file:     &models.File{FileType: "epub"},
			expected: "[Some Author] Some Series - The Book.epub",
		},
		{
			name: "decimal series number",
			book: &models.Book{
				Title: "Interlude",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "Author"}},
				},
				BookSeries: []*models.BookSeries{
					{SortOrder: 0, SeriesNumber: pointerutil.Float64(1.5), Series: &models.Series{Name: "Series"}},
				},
			},
			file:     &models.File{FileType: "epub"},
			expected: "[Author] Series #1.5 - Interlude.epub",
		},
		{
			name: "multiple authors - picks first by sort order",
			book: &models.Book{
				Title: "Collaboration",
				Authors: []*models.Author{
					{SortOrder: 1, Person: &models.Person{Name: "Second Author"}},
					{SortOrder: 0, Person: &models.Person{Name: "First Author"}},
					{SortOrder: 2, Person: &models.Person{Name: "Third Author"}},
				},
				BookSeries: nil,
			},
			file:     &models.File{FileType: "epub"},
			expected: "[First Author] Collaboration.epub",
		},
		{
			name: "multiple series - picks first by sort order",
			book: &models.Book{
				Title: "Crossover",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "Author"}},
				},
				BookSeries: []*models.BookSeries{
					{SortOrder: 1, SeriesNumber: pointerutil.Float64(5), Series: &models.Series{Name: "Second Series"}},
					{SortOrder: 0, SeriesNumber: pointerutil.Float64(3), Series: &models.Series{Name: "First Series"}},
				},
			},
			file:     &models.File{FileType: "epub"},
			expected: "[Author] First Series #3 - Crossover.epub",
		},
		{
			name: "m4b file type with narrator",
			book: &models.Book{
				Title: "Audiobook Title",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "Author Name"}},
				},
				BookSeries: nil,
			},
			file: &models.File{
				FileType: "m4b",
				Narrators: []*models.Narrator{
					{SortOrder: 0, Person: &models.Person{Name: "Ray Porter"}},
				},
			},
			expected: "[Author Name] Audiobook Title {Ray Porter}.m4b",
		},
		{
			name: "m4b full format with series and narrator",
			book: &models.Book{
				Title: "Project Hail Mary",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "Andy Weir"}},
				},
				BookSeries: []*models.BookSeries{
					{SortOrder: 0, SeriesNumber: pointerutil.Float64(1), Series: &models.Series{Name: "Standalone"}},
				},
			},
			file: &models.File{
				FileType: "m4b",
				Narrators: []*models.Narrator{
					{SortOrder: 0, Person: &models.Person{Name: "Ray Porter"}},
				},
			},
			expected: "[Andy Weir] Standalone #1 - Project Hail Mary {Ray Porter}.m4b",
		},
		{
			name: "m4b without narrator",
			book: &models.Book{
				Title: "Audiobook Without Narrator",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "Some Author"}},
				},
				BookSeries: nil,
			},
			file:     &models.File{FileType: "m4b"},
			expected: "[Some Author] Audiobook Without Narrator.m4b",
		},
		{
			name: "m4b multiple narrators - picks first by sort order",
			book: &models.Book{
				Title: "Multi-Narrator Book",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "Author"}},
				},
				BookSeries: nil,
			},
			file: &models.File{
				FileType: "m4b",
				Narrators: []*models.Narrator{
					{SortOrder: 1, Person: &models.Person{Name: "Second Narrator"}},
					{SortOrder: 0, Person: &models.Person{Name: "First Narrator"}},
					{SortOrder: 2, Person: &models.Person{Name: "Third Narrator"}},
				},
			},
			expected: "[Author] Multi-Narrator Book {First Narrator}.m4b",
		},
		{
			name: "invalid characters in narrator name are removed",
			book: &models.Book{
				Title: "Audiobook",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "Author"}},
				},
				BookSeries: nil,
			},
			file: &models.File{
				FileType: "m4b",
				Narrators: []*models.Narrator{
					{SortOrder: 0, Person: &models.Person{Name: "Narrator/Reader"}},
				},
			},
			expected: "[Author] Audiobook {NarratorReader}.m4b",
		},
		{
			name: "invalid characters in title are removed",
			book: &models.Book{
				Title: "Title: With <Special> *Characters*?",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "Author"}},
				},
				BookSeries: nil,
			},
			file:     &models.File{FileType: "epub"},
			expected: "[Author] Title With Special Characters.epub",
		},
		{
			name: "invalid characters in author name are removed",
			book: &models.Book{
				Title: "Book",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "Author/Writer"}},
				},
				BookSeries: nil,
			},
			file:     &models.File{FileType: "epub"},
			expected: "[AuthorWriter] Book.epub",
		},
		{
			name: "invalid characters in series name are removed",
			book: &models.Book{
				Title: "Book",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "Author"}},
				},
				BookSeries: []*models.BookSeries{
					{SortOrder: 0, SeriesNumber: pointerutil.Float64(1), Series: &models.Series{Name: "Series: The Saga"}},
				},
			},
			file:     &models.File{FileType: "epub"},
			expected: "[Author] Series The Saga #1 - Book.epub",
		},
		{
			name: "whole number series displays without decimal",
			book: &models.Book{
				Title: "Book",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "Author"}},
				},
				BookSeries: []*models.BookSeries{
					{SortOrder: 0, SeriesNumber: pointerutil.Float64(10.0), Series: &models.Series{Name: "Series"}},
				},
			},
			file:     &models.File{FileType: "epub"},
			expected: "[Author] Series #10 - Book.epub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDownloadFilename(tt.book, tt.file)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal name", "normal name"},
		{"with/slash", "withslash"},
		{"with\\backslash", "withbackslash"},
		{"with:colon", "withcolon"},
		{"with*asterisk", "withasterisk"},
		{"with?question", "withquestion"},
		{"with\"quotes", "withquotes"},
		{"with<less", "withless"},
		{"with>greater", "withgreater"},
		{"with|pipe", "withpipe"},
		{"multiple   spaces", "multiple spaces"},
		{"  leading spaces", "leading spaces"},
		{"trailing spaces  ", "trailing spaces"},
		{"all:invalid/chars\\here*?\"<>|", "allinvalidcharshere"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeFilename(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatSeriesNumber(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{1, "1"},
		{1.0, "1"},
		{10, "10"},
		{1.5, "1.5"},
		{2.25, "2.25"},
		{0.5, "0.5"},
		{100, "100"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatSeriesNumber(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
