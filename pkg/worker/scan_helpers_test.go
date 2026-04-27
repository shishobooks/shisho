package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldUpdateScalar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		newValue       string
		existingValue  string
		newSource      string
		existingSource string
		forceRefresh   bool
		want           bool
	}{
		{
			name:           "higher priority source with value updates",
			newValue:       "New Title",
			existingValue:  "Old Title",
			newSource:      models.DataSourceFileMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "higher priority source with empty value does not update",
			newValue:       "",
			existingValue:  "Old Title",
			newSource:      models.DataSourceFileMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "higher priority source with same value does not update",
			newValue:       "Same Title",
			existingValue:  "Same Title",
			newSource:      models.DataSourceFileMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "same priority with different value updates",
			newValue:       "New Title",
			existingValue:  "Old Title",
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "same priority with same value does not update",
			newValue:       "Same Title",
			existingValue:  "Same Title",
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "same priority with empty new value does not update",
			newValue:       "",
			existingValue:  "Old Title",
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "lower priority source never updates",
			newValue:       "New Title",
			existingValue:  "Old Title",
			newSource:      models.DataSourceFilepath,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "manual source is never overwritten",
			newValue:       "New Title",
			existingValue:  "Manual Title",
			newSource:      models.DataSourceFileMetadata,
			existingSource: models.DataSourceManual,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "empty existing source treated as filepath priority",
			newValue:       "New Title",
			existingValue:  "Old Title",
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: "",
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "forceRefresh updates even with lower priority source",
			newValue:       "New Title",
			existingValue:  "Manual Title",
			newSource:      models.DataSourceFilepath,
			existingSource: models.DataSourceManual,
			forceRefresh:   true,
			want:           true,
		},
		{
			name:           "forceRefresh still skips empty new value",
			newValue:       "",
			existingValue:  "Manual Title",
			newSource:      models.DataSourceFilepath,
			existingSource: models.DataSourceManual,
			forceRefresh:   true,
			want:           false,
		},
		{
			name:           "forceRefresh updates source when value matches but source differs",
			newValue:       "Same Title",
			existingValue:  "Same Title",
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourcePlugin,
			forceRefresh:   true,
			want:           true,
		},
		{
			name:           "forceRefresh skips when both value and source match",
			newValue:       "Same Title",
			existingValue:  "Same Title",
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   true,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldUpdateScalar(tt.newValue, tt.existingValue, tt.newSource, tt.existingSource, tt.forceRefresh)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestShouldUpdateRelationship(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		newItems       []string
		existingItems  []string
		newSource      string
		existingSource string
		forceRefresh   bool
		want           bool
	}{
		{
			name:           "higher priority source with items updates",
			newItems:       []string{"Author A"},
			existingItems:  []string{"Author B"},
			newSource:      models.DataSourceFileMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "higher priority source with empty items does not update",
			newItems:       []string{},
			existingItems:  []string{"Author B"},
			newSource:      models.DataSourceFileMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "higher priority source with same items does not update",
			newItems:       []string{"Author A"},
			existingItems:  []string{"Author A"},
			newSource:      models.DataSourceFileMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "same priority with different items updates",
			newItems:       []string{"Author A", "Author B"},
			existingItems:  []string{"Author C"},
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "same priority with same items does not update",
			newItems:       []string{"Author A", "Author B"},
			existingItems:  []string{"Author A", "Author B"},
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "same priority with same items different order updates",
			newItems:       []string{"Author B", "Author A"},
			existingItems:  []string{"Author A", "Author B"},
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "same priority with empty new items does not update",
			newItems:       []string{},
			existingItems:  []string{"Author A"},
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "lower priority source never updates",
			newItems:       []string{"Author A"},
			existingItems:  []string{"Author B"},
			newSource:      models.DataSourceFilepath,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "manual source is never overwritten",
			newItems:       []string{"Author A"},
			existingItems:  []string{"Manual Author"},
			newSource:      models.DataSourceFileMetadata,
			existingSource: models.DataSourceManual,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "empty existing source treated as filepath priority",
			newItems:       []string{"Author A"},
			existingItems:  []string{"Author B"},
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: "",
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "nil existing items with new items updates",
			newItems:       []string{"Author A"},
			existingItems:  nil,
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceFilepath,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "forceRefresh updates even with lower priority source",
			newItems:       []string{"New Author"},
			existingItems:  []string{"Manual Author"},
			newSource:      models.DataSourceFilepath,
			existingSource: models.DataSourceManual,
			forceRefresh:   true,
			want:           true,
		},
		{
			name:           "forceRefresh still skips empty new items",
			newItems:       []string{},
			existingItems:  []string{"Manual Author"},
			newSource:      models.DataSourceFilepath,
			existingSource: models.DataSourceManual,
			forceRefresh:   true,
			want:           false,
		},
		{
			name:           "forceRefresh updates source when items match but source differs",
			newItems:       []string{"Same Author"},
			existingItems:  []string{"Same Author"},
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourcePlugin,
			forceRefresh:   true,
			want:           true,
		},
		{
			name:           "forceRefresh skips when both items and source match",
			newItems:       []string{"Same Author"},
			existingItems:  []string{"Same Author"},
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   true,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldUpdateRelationship(tt.newItems, tt.existingItems, tt.newSource, tt.existingSource, tt.forceRefresh)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateCBZFileName(t *testing.T) {
	t.Parallel()
	floatPtr := func(f float64) *float64 { return &f }

	tests := []struct {
		name     string
		metadata *mediafile.ParsedMetadata
		filename string
		want     string
	}{
		{
			name: "title from metadata is preferred over series+number",
			metadata: &mediafile.ParsedMetadata{
				Title:        "My Awesome Comic",
				Series:       "Some Series",
				SeriesNumber: floatPtr(1),
			},
			filename: "[Author] Some Series v1.cbz",
			want:     "My Awesome Comic",
		},
		{
			name: "series+number used when title is empty",
			metadata: &mediafile.ParsedMetadata{
				Title:        "",
				Series:       "Demon Slayer",
				SeriesNumber: floatPtr(1),
			},
			filename: "[Koyoharu Gotouge] Demon Slayer v1.cbz",
			want:     "Demon Slayer v001",
		},
		{
			name: "series+number used when title looks like filename with brackets",
			metadata: &mediafile.ParsedMetadata{
				Title:        "[Author] Comic Title v1",
				Series:       "Comic Title",
				SeriesNumber: floatPtr(1),
			},
			filename: "[Author] Comic Title v1.cbz",
			want:     "Comic Title v001",
		},
		{
			name: "series only when no number",
			metadata: &mediafile.ParsedMetadata{
				Title:        "",
				Series:       "One Piece",
				SeriesNumber: nil,
			},
			filename: "[Oda] One Piece.cbz",
			want:     "One Piece",
		},
		{
			name: "decimal series number preserved",
			metadata: &mediafile.ParsedMetadata{
				Title:        "",
				Series:       "Naruto",
				SeriesNumber: floatPtr(1.5),
			},
			filename: "[Kishimoto] Naruto v1.5.cbz",
			want:     "Naruto v001.5",
		},
		{
			name: "parse from filename when no metadata",
			metadata: &mediafile.ParsedMetadata{
				Title:        "",
				Series:       "",
				SeriesNumber: nil,
			},
			filename: "[Author Name] Comic Title v1.cbz",
			want:     "Comic Title v001",
		},
		{
			name: "parse from filename with multiple bracket sections",
			metadata: &mediafile.ParsedMetadata{
				Title:        "",
				Series:       "",
				SeriesNumber: nil,
			},
			filename: "[Author] [Publisher] Comic Title.cbz",
			want:     "Comic Title",
		},
		{
			name: "whitespace-only title falls through to series",
			metadata: &mediafile.ParsedMetadata{
				Title:        "   ",
				Series:       "My Series",
				SeriesNumber: floatPtr(5),
			},
			filename: "whatever.cbz",
			want:     "My Series v005",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateCBZFileName(tt.metadata, tt.filename)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCleanCBZFilename(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{
			name:     "removes author brackets and extension",
			filename: "[Author Name] Comic Title v1.cbz",
			want:     "Comic Title v001",
		},
		{
			name:     "removes multiple bracket sections",
			filename: "[Author] [Publisher] [Year] Comic Title.cbz",
			want:     "Comic Title",
		},
		{
			name:     "handles no brackets",
			filename: "Comic Title v1.cbz",
			want:     "Comic Title v001",
		},
		{
			name:     "collapses multiple spaces",
			filename: "[Author]   Comic   Title.cbz",
			want:     "Comic Title",
		},
		{
			name:     "removes parenthesized metadata after volume",
			filename: "Comic Title v02 (2020) (Digital) (group).cbz",
			want:     "Comic Title v002",
		},
		{
			name:     "removes parenthesized metadata with brackets",
			filename: "[Author] Comic Title v01 (2023) (Digital).cbz",
			want:     "Comic Title v001",
		},
		{
			name:     "removes parenthesized metadata without volume",
			filename: "Comic Title (2020) (Digital).cbz",
			want:     "Comic Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanCBZFilename(tt.filename)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatSeriesNumber(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		num  float64
		want string
	}{
		{name: "whole number", num: 1, want: "001"},
		{name: "whole number larger", num: 42, want: "042"},
		{name: "three digits", num: 100, want: "100"},
		{name: "decimal", num: 1.5, want: "001.5"},
		{name: "decimal with trailing zeros", num: 2.50, want: "002.5"},
		{name: "zero", num: 0, want: "000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSeriesNumber(tt.num)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestShouldApplySidecarScalar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		newValue       string
		existingValue  string
		existingSource string
		forceRefresh   bool
		want           bool
	}{
		{
			name:           "sidecar overrides lower priority filepath source",
			newValue:       "Sidecar Title",
			existingValue:  "Filepath Title",
			existingSource: models.DataSourceFilepath,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "sidecar overrides lower priority file_metadata",
			newValue:       "Sidecar Title",
			existingValue:  "Other Sidecar Title",
			existingSource: models.DataSourceFileMetadata,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "sidecar overrides lower priority epub_metadata",
			newValue:       "Sidecar Title",
			existingValue:  "EPUB Title",
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "sidecar does not override higher priority manual",
			newValue:       "Sidecar Title",
			existingValue:  "Manual Title",
			existingSource: models.DataSourceManual,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "sidecar skips empty value",
			newValue:       "",
			existingValue:  "Existing Title",
			existingSource: models.DataSourceFilepath,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "sidecar skips same value",
			newValue:       "Same Title",
			existingValue:  "Same Title",
			existingSource: models.DataSourceFilepath,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "forceRefresh skips sidecar entirely",
			newValue:       "Sidecar Title",
			existingValue:  "Filepath Title",
			existingSource: models.DataSourceFilepath,
			forceRefresh:   true,
			want:           false,
		},
		{
			name:           "forceRefresh skips sidecar even for lower priority source",
			newValue:       "Sidecar Title",
			existingValue:  "EPUB Title",
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   true,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldApplySidecarScalar(tt.newValue, tt.existingValue, tt.existingSource, tt.forceRefresh)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestShouldApplySidecarRelationship(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		newItems       []string
		existingItems  []string
		existingSource string
		forceRefresh   bool
		want           bool
	}{
		{
			name:           "sidecar overrides lower priority filepath source",
			newItems:       []string{"Sidecar Author"},
			existingItems:  []string{"Filepath Author"},
			existingSource: models.DataSourceFilepath,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "sidecar overrides lower priority file_metadata",
			newItems:       []string{"Sidecar Author"},
			existingItems:  []string{"Other Sidecar Author"},
			existingSource: models.DataSourceFileMetadata,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "sidecar overrides lower priority epub_metadata",
			newItems:       []string{"Sidecar Author"},
			existingItems:  []string{"EPUB Author"},
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "sidecar does not override higher priority manual",
			newItems:       []string{"Sidecar Author"},
			existingItems:  []string{"Manual Author"},
			existingSource: models.DataSourceManual,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "sidecar skips empty items",
			newItems:       []string{},
			existingItems:  []string{"Existing Author"},
			existingSource: models.DataSourceFilepath,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "sidecar skips same items",
			newItems:       []string{"Same Author"},
			existingItems:  []string{"Same Author"},
			existingSource: models.DataSourceFilepath,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "forceRefresh skips sidecar entirely",
			newItems:       []string{"Sidecar Author"},
			existingItems:  []string{"Filepath Author"},
			existingSource: models.DataSourceFilepath,
			forceRefresh:   true,
			want:           false,
		},
		{
			name:           "forceRefresh skips sidecar even for lower priority source",
			newItems:       []string{"Sidecar Author"},
			existingItems:  []string{"EPUB Author"},
			existingSource: models.DataSourceEPUBMetadata,
			forceRefresh:   true,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldApplySidecarRelationship(tt.newItems, tt.existingItems, tt.existingSource, tt.forceRefresh)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLooksLikePDFSupplement(t *testing.T) {
	t.Parallel()

	defaultNames := []string{
		"supplement", "supplemental", "bonus", "bonus material", "bonus content",
		"companion", "notes", "liner notes", "errata", "booklet", "digital booklet",
		"appendix", "map", "maps", "insert", "guide", "reference",
		"cheat sheet", "cheatsheet", "cribsheet", "pamphlet", "extras",
	}

	tests := []struct {
		name     string
		filename string
		names    []string
		want     bool
	}{
		{name: "exact match lowercase", filename: "supplement.pdf", names: defaultNames, want: true},
		{name: "exact match uppercase ext", filename: "Supplement.PDF", names: defaultNames, want: true},
		{name: "all caps basename", filename: "BONUS MATERIAL.pdf", names: defaultNames, want: true},
		{name: "trims surrounding whitespace", filename: "  supplement  .pdf", names: defaultNames, want: true},
		{name: "multi-word entry matches", filename: "liner notes.pdf", names: defaultNames, want: true},
		{name: "non-pdf extension does not match", filename: "supplement.txt", names: defaultNames, want: false},
		{name: "substring does not match", filename: "my book - supplement.pdf", names: defaultNames, want: false},
		{name: "unrelated name does not match", filename: "Companion Guide.pdf", names: defaultNames, want: false},
		{name: "empty names list disables matching", filename: "supplement.pdf", names: []string{}, want: false},
		{name: "nil names list disables matching", filename: "supplement.pdf", names: nil, want: false},
		{name: "custom list overrides default", filename: "extra.pdf", names: []string{"extra"}, want: true},
		{name: "no extension does not match", filename: "supplement", names: defaultNames, want: false},
		{name: "trims whitespace in names list entry", filename: "supplement.pdf", names: []string{"  supplement  "}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := looksLikePDFSupplement(tt.filename, tt.names)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestIdentifierDiff_StableAcrossRescans locks in the fix for a regression
// where each rescan of a book with hyphenated/prefixed/mixed-case identifiers
// would thrash delete+insert because the stored (normalized) value and the
// parser-emitted (raw) value built different diff keys. This test drives the
// exact helpers that pkg/worker/scan_unified.go now calls (fileIdentifierKeys
// and parsedIdentifierKeys) — so reverting the scanner to inline
// "id.Type+':'+id.Value" key building would break it, locking in the
// integration and not just the helper contract.
func TestIdentifierDiff_StableAcrossRescans(t *testing.T) {
	t.Parallel()

	// Simulate what file.Identifiers holds after the first scan: values are
	// already normalized because the books service canonicalizes on write.
	stored := []*models.FileIdentifier{
		{Type: models.IdentifierTypeISBN13, Value: "9780316769488"},
		{Type: models.IdentifierTypeASIN, Value: "B08N5WRWNW"},
		{Type: models.IdentifierTypeUUID, Value: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"},
	}
	// Simulate what the parser returns on the next scan: raw cosmetic forms.
	parsed := []mediafile.ParsedIdentifier{
		{Type: models.IdentifierTypeISBN13, Value: "978-0-316-76948-8"},
		{Type: models.IdentifierTypeASIN, Value: "b08n5wrwnw"},
		{Type: models.IdentifierTypeUUID, Value: "URN:UUID:A1B2C3D4-E5F6-7890-ABCD-EF1234567890"},
	}

	existingKeys := fileIdentifierKeys(stored)
	newKeys := parsedIdentifierKeys(parsed)

	// With a matching source (steady state), the diff must report no change.
	got := shouldUpdateRelationship(newKeys, existingKeys, models.DataSourceEPUBMetadata, models.DataSourceEPUBMetadata, false)
	assert.False(t, got, "rescan must not report identifier change when only cosmetic formatting differs")

	// Sidecar path uses the same key-building helpers.
	got = shouldApplySidecarRelationship(newKeys, existingKeys, models.DataSourceSidecar, false)
	assert.False(t, got, "sidecar rescan must not report identifier change when only cosmetic formatting differs")

	// A genuinely new identifier must still be detected as a change.
	parsedWithAddition := append(parsed, mediafile.ParsedIdentifier{
		Type:  models.IdentifierTypeGoodreads,
		Value: "12345678",
	})
	newKeysWithAddition := parsedIdentifierKeys(parsedWithAddition)
	got = shouldUpdateRelationship(newKeysWithAddition, existingKeys, models.DataSourceEPUBMetadata, models.DataSourceEPUBMetadata, false)
	assert.True(t, got, "rescan must still detect a real addition even when the existing entries have cosmetic variants")
}

func TestHasNonPDFMainSibling(t *testing.T) {
	t.Parallel()

	t.Run("epub sibling", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "supplement.pdf"))
		writeFile(t, filepath.Join(dir, "book.epub"))

		got, err := hasNonPDFMainSibling(dir, nil)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("cbz sibling", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "supplement.pdf"))
		writeFile(t, filepath.Join(dir, "comic.cbz"))

		got, err := hasNonPDFMainSibling(dir, nil)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("m4b sibling", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "supplement.pdf"))
		writeFile(t, filepath.Join(dir, "audio.m4b"))

		got, err := hasNonPDFMainSibling(dir, nil)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("pdf-only directory has no sibling", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "supplement.pdf"))
		writeFile(t, filepath.Join(dir, "another.pdf"))

		got, err := hasNonPDFMainSibling(dir, nil)
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("unrelated extension is not a sibling", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "supplement.pdf"))
		writeFile(t, filepath.Join(dir, "notes.txt"))

		got, err := hasNonPDFMainSibling(dir, nil)
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("plugin-registered extension is a sibling", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "supplement.pdf"))
		writeFile(t, filepath.Join(dir, "book.azw3"))

		// Plugin extensions are stored without leading dot, lowercase.
		pluginExts := map[string]struct{}{"azw3": {}}
		got, err := hasNonPDFMainSibling(dir, pluginExts)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("recursive sibling in subdirectory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "supplement.pdf"))
		sub := filepath.Join(dir, "extras")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		writeFile(t, filepath.Join(sub, "book.epub"))

		got, err := hasNonPDFMainSibling(dir, nil)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("missing directory returns error", func(t *testing.T) {
		t.Parallel()
		_, err := hasNonPDFMainSibling(filepath.Join(t.TempDir(), "does-not-exist"), nil)
		assert.Error(t, err)
	})
}

// writeFile creates an empty file at path, failing the test on error.
func writeFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte{}, 0o644))
}
