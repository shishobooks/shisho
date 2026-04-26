package fileutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeSeriesNumberInTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		title     string
		fileType  string
		wantTitle string
		wantUnit  string
		wantOK    bool
	}{
		// Existing volume patterns keep working
		{"v compact", "Naruto v3", "cbz", "Naruto v003", "volume", true},
		{"vol with dot", "Naruto vol.7", "cbz", "Naruto v007", "volume", true},
		{"volume word", "Naruto volume 12", "cbz", "Naruto v012", "volume", true},
		{"hash defaults to volume", "Naruto #001", "cbz", "Naruto v001", "volume", true},
		{"bare trailing number defaults to volume", "Naruto 4", "cbz", "Naruto v004", "volume", true},
		{"fractional volume", "Naruto v7.5", "cbz", "Naruto v007.5", "volume", true},
		// New chapter patterns
		{"chapter word", "One Piece chapter 5", "cbz", "One Piece c005", "chapter", true},
		{"ch with dot", "One Piece Ch.5", "cbz", "One Piece c005", "chapter", true},
		{"ch without dot", "One Piece Ch 42", "cbz", "One Piece c042", "chapter", true},
		{"c compact", "One Piece c042", "cbz", "One Piece c042", "chapter", true},
		{"fractional chapter", "One Piece c5.5", "cbz", "One Piece c005.5", "chapter", true},
		// Non-CBZ short-circuits
		{"epub returns false", "Some Book v3", "epub", "Some Book v3", "", false},
		{"m4b returns false", "Some Book v3", "m4b", "Some Book v3", "", false},
		// No match
		{"no number", "Just A Title", "cbz", "Just A Title", "", false},
		// Compact c/v require leading whitespace — no false positives mid-word
		{"compact c not eaten mid-word", "Abc123", "cbz", "Abc123", "", false},
		{"compact v not eaten mid-word", "Marv7", "cbz", "Marv7", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotTitle, gotUnit, gotOK := NormalizeSeriesNumberInTitle(tt.title, tt.fileType)
			assert.Equal(t, tt.wantTitle, gotTitle)
			assert.Equal(t, tt.wantUnit, gotUnit)
			assert.Equal(t, tt.wantOK, gotOK)
		})
	}
}

func TestSplitNames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single name",
			input:    "John Doe",
			expected: []string{"John Doe"},
		},
		{
			name:     "comma separated",
			input:    "John Doe, Jane Smith",
			expected: []string{"John Doe", "Jane Smith"},
		},
		{
			name:     "semicolon separated",
			input:    "John Doe; Jane Smith",
			expected: []string{"John Doe", "Jane Smith"},
		},
		{
			name:     "mixed comma and semicolon",
			input:    "John Doe, Jane Smith; Bob Wilson",
			expected: []string{"John Doe", "Jane Smith", "Bob Wilson"},
		},
		{
			name:     "with extra whitespace",
			input:    "  John Doe  ,  Jane Smith  ;  Bob Wilson  ",
			expected: []string{"John Doe", "Jane Smith", "Bob Wilson"},
		},
		{
			name:     "empty parts filtered",
			input:    "John Doe,,Jane Smith;;Bob Wilson",
			expected: []string{"John Doe", "Jane Smith", "Bob Wilson"},
		},
		{
			name:     "only delimiters",
			input:    ",;,;",
			expected: nil,
		},
		{
			name:     "multiple semicolons then commas",
			input:    "Author A; Author B, Author C",
			expected: []string{"Author A", "Author B", "Author C"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitNames(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatSeriesNumber(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		number   float64
		unit     string
		fileType string
		want     string
	}{
		{"volume integer", 5, "volume", "cbz", "v5"},
		{"volume empty unit defaults to v", 5, "", "cbz", "v5"},
		{"chapter integer", 42, "chapter", "cbz", "c42"},
		{"volume fractional", 7.5, "volume", "cbz", "v7.5"},
		{"chapter fractional", 7.5, "chapter", "cbz", "c7.5"},
		{"non-cbz still uses #", 3, "", "epub", "#3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatSeriesNumber(tt.number, tt.unit, tt.fileType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractSeriesNumberFromTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		title    string
		wantNum  *float64
		wantUnit string
	}{
		{"volume", "Naruto v003", floatPtr(3), "volume"},
		{"chapter", "One Piece c042", floatPtr(42), "chapter"},
		{"none", "No Number Here", nil, ""},
		{"fractional volume", "Naruto v007.5", floatPtr(7.5), "volume"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotNum, gotUnit := extractSeriesNumberFromTitle(tt.title)
			if tt.wantNum == nil {
				assert.Nil(t, gotNum)
			} else {
				assert.NotNil(t, gotNum)
				assert.InEpsilon(t, *tt.wantNum, *gotNum, 0.0001)
			}
			assert.Equal(t, tt.wantUnit, gotUnit)
		})
	}
}

func TestExtractSeriesFromTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		title      string
		fileType   string
		wantSeries string
		wantNum    *float64
		wantUnit   string
		wantOK     bool
	}{
		{"volume", "Naruto v003", "cbz", "Naruto", floatPtr(3), "volume", true},
		{"chapter", "One Piece c042", "cbz", "One Piece", floatPtr(42), "chapter", true},
		{"non-cbz returns false", "Naruto v003", "epub", "", nil, "", false},
		{"no number returns false", "Just A Title", "cbz", "", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotSeries, gotNum, gotUnit, gotOK := ExtractSeriesFromTitle(tt.title, tt.fileType)
			assert.Equal(t, tt.wantSeries, gotSeries)
			assert.Equal(t, tt.wantUnit, gotUnit)
			assert.Equal(t, tt.wantOK, gotOK)
			if tt.wantNum == nil {
				assert.Nil(t, gotNum)
			} else {
				assert.NotNil(t, gotNum)
				assert.InEpsilon(t, *tt.wantNum, *gotNum, 0.0001)
			}
		})
	}
}

func TestIsOrganizedName_Chapter(t *testing.T) {
	t.Parallel()
	assert.True(t, IsOrganizedName("Naruto c042.cbz"))
	assert.True(t, IsOrganizedName("Naruto v042.cbz"))
	assert.True(t, IsOrganizedName("Naruto #042.cbz"))
	assert.True(t, IsOrganizedName("[Author] Title.cbz"))
}

func floatPtr(f float64) *float64 { return &f }

func TestGenerateOrganizedFolderName_ChapterUnit(t *testing.T) {
	t.Parallel()
	chapterUnit := "chapter"
	got := GenerateOrganizedFolderName(OrganizedNameOptions{
		AuthorNames:      []string{"Eiichiro Oda"},
		Title:            "One Piece",
		SeriesNumber:     floatPtr(42),
		SeriesNumberUnit: &chapterUnit,
		FileType:         "cbz",
	})
	assert.Equal(t, "[Eiichiro Oda] One Piece c42", got)
}

func TestGenerateOrganizedFolderName_VolumeUnitDefault(t *testing.T) {
	t.Parallel()
	got := GenerateOrganizedFolderName(OrganizedNameOptions{
		AuthorNames:  []string{"Eiichiro Oda"},
		Title:        "One Piece",
		SeriesNumber: floatPtr(42),
		FileType:     "cbz",
	})
	// Nil unit on CBZ keeps today's behavior: format as volume.
	assert.Equal(t, "[Eiichiro Oda] One Piece v42", got)
}

func TestGenerateOrganizedFolderName_TitleAlreadyHasChapter(t *testing.T) {
	t.Parallel()
	chapterUnit := "chapter"
	got := GenerateOrganizedFolderName(OrganizedNameOptions{
		AuthorNames:      []string{"Eiichiro Oda"},
		Title:            "One Piece c042",
		SeriesNumber:     floatPtr(42),
		SeriesNumberUnit: &chapterUnit,
		FileType:         "cbz",
	})
	// Title already encodes the number — don't double-stamp.
	assert.Equal(t, "[Eiichiro Oda] One Piece c042", got)
}
