package worker

import (
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeEnrichedMetadata_EnricherOverridesFileMetadata verifies that
// enricher data takes precedence over file-parsed metadata for the same field.
func TestMergeEnrichedMetadata_EnricherOverridesFileMetadata(t *testing.T) {
	t.Parallel()

	// Enricher provides a title
	var enrichedMeta mediafile.ParsedMetadata
	enricherResult := &mediafile.ParsedMetadata{
		Title: "Good Title",
	}
	enricherSource := "plugin:test/my-enricher"
	mergeEnrichedMetadata(&enrichedMeta, enricherResult, enricherSource)

	// File parser provides a different title as fallback
	fileMetadata := &mediafile.ParsedMetadata{
		Title:      "Bad Title",
		DataSource: "epub_metadata",
	}
	mergeEnrichedMetadata(&enrichedMeta, fileMetadata, fileMetadata.DataSource)

	// Enricher title should win
	assert.Equal(t, "Good Title", enrichedMeta.Title)
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["title"])
}

// TestMergeEnrichedMetadata_EnricherFillsEmptyFields verifies that when the
// file parser has no value for a field but the enricher provides one, the
// enricher's value is used.
func TestMergeEnrichedMetadata_EnricherFillsEmptyFields(t *testing.T) {
	t.Parallel()

	var enrichedMeta mediafile.ParsedMetadata
	enricherResult := &mediafile.ParsedMetadata{
		Description: "Enricher provided description",
	}
	enricherSource := "plugin:test/my-enricher"
	mergeEnrichedMetadata(&enrichedMeta, enricherResult, enricherSource)

	// File parser has no description
	fileMetadata := &mediafile.ParsedMetadata{
		Title:      "File Title",
		DataSource: "epub_metadata",
	}
	mergeEnrichedMetadata(&enrichedMeta, fileMetadata, fileMetadata.DataSource)

	assert.Equal(t, "Enricher provided description", enrichedMeta.Description)
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["description"])
	// Title should come from file parser since enricher didn't provide it
	assert.Equal(t, "File Title", enrichedMeta.Title)
	assert.Equal(t, "epub_metadata", enrichedMeta.FieldDataSources["title"])
}

// TestMergeEnrichedMetadata_FileParserFillsEnricherGaps verifies that when
// an enricher provides some fields but not others, the file parser fills in
// the gaps with correct per-field sources.
func TestMergeEnrichedMetadata_FileParserFillsEnricherGaps(t *testing.T) {
	t.Parallel()

	var enrichedMeta mediafile.ParsedMetadata
	enricherSource := "plugin:test/my-enricher"

	// Enricher provides title only
	enricherResult := &mediafile.ParsedMetadata{
		Title: "Enricher Title",
	}
	mergeEnrichedMetadata(&enrichedMeta, enricherResult, enricherSource)

	// File parser provides title and authors
	fileSource := "epub_metadata"
	fileMetadata := &mediafile.ParsedMetadata{
		Title:      "File Title",
		Authors:    []mediafile.ParsedAuthor{{Name: "File Author"}},
		DataSource: fileSource,
	}
	mergeEnrichedMetadata(&enrichedMeta, fileMetadata, fileSource)

	// Title from enricher, authors from file parser
	assert.Equal(t, "Enricher Title", enrichedMeta.Title)
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["title"])
	require.Len(t, enrichedMeta.Authors, 1)
	assert.Equal(t, "File Author", enrichedMeta.Authors[0].Name)
	assert.Equal(t, fileSource, enrichedMeta.FieldDataSources["authors"])
}

// TestMergeEnrichedMetadata_EnricherToEnricherPriority verifies that when
// two enrichers both provide the same field, the first enricher's value wins.
func TestMergeEnrichedMetadata_EnricherToEnricherPriority(t *testing.T) {
	t.Parallel()

	var enrichedMeta mediafile.ParsedMetadata

	// First enricher provides title
	enricher1Source := "plugin:test/enricher-first"
	enricher1Result := &mediafile.ParsedMetadata{
		Title: "First Enricher Title",
	}
	mergeEnrichedMetadata(&enrichedMeta, enricher1Result, enricher1Source)

	// Second enricher also provides title
	enricher2Source := "plugin:test/enricher-second"
	enricher2Result := &mediafile.ParsedMetadata{
		Title: "Second Enricher Title",
	}
	mergeEnrichedMetadata(&enrichedMeta, enricher2Result, enricher2Source)

	// First enricher's title should win
	assert.Equal(t, "First Enricher Title", enrichedMeta.Title)
	assert.Equal(t, enricher1Source, enrichedMeta.FieldDataSources["title"])
}

// TestMergeEnrichedMetadata_IdentifiersMergeAdditively verifies that identifiers
// from file parser and enricher are merged additively (both present).
func TestMergeEnrichedMetadata_IdentifiersMergeAdditively(t *testing.T) {
	t.Parallel()

	var enrichedMeta mediafile.ParsedMetadata

	// Enricher provides ASIN
	enricherSource := "plugin:test/enricher"
	enricherResult := &mediafile.ParsedMetadata{
		Identifiers: []mediafile.ParsedIdentifier{
			{Type: "asin", Value: "B01ENRICHED"},
		},
	}
	mergeEnrichedMetadata(&enrichedMeta, enricherResult, enricherSource)

	// File parser provides ISBN
	fileSource := "epub_metadata"
	fileMetadata := &mediafile.ParsedMetadata{
		Identifiers: []mediafile.ParsedIdentifier{
			{Type: "isbn_13", Value: "9781234567890"},
		},
	}
	mergeEnrichedMetadata(&enrichedMeta, fileMetadata, fileSource)

	// Both should be present
	require.Len(t, enrichedMeta.Identifiers, 2)
	types := map[string]string{}
	for _, id := range enrichedMeta.Identifiers {
		types[id.Type] = id.Value
	}
	assert.Equal(t, "B01ENRICHED", types["asin"])
	assert.Equal(t, "9781234567890", types["isbn_13"])
}

// TestMergeEnrichedMetadata_TechnicalFieldsPreserved verifies that technical
// fields (Duration, BitrateBps, Codec, PageCount) from the file parser are
// always preserved in the final result.
func TestMergeEnrichedMetadata_TechnicalFieldsPreserved(t *testing.T) {
	t.Parallel()

	pageCount := 42
	fileMetadata := &mediafile.ParsedMetadata{
		Title:      "File Title",
		Duration:   time.Hour,
		BitrateBps: 128000,
		Codec:      "AAC-LC",
		PageCount:  &pageCount,
		DataSource: "m4b_metadata",
	}

	// Simulate what runMetadataEnrichers does:
	var enrichedMeta mediafile.ParsedMetadata

	// Enricher provides title
	enricherResult := &mediafile.ParsedMetadata{
		Title: "Enricher Title",
	}
	mergeEnrichedMetadata(&enrichedMeta, enricherResult, "plugin:test/enricher")

	// File parser fallback
	mergeEnrichedMetadata(&enrichedMeta, fileMetadata, fileMetadata.DataSource)

	// Copy technical fields (as runMetadataEnrichers does)
	enrichedMeta.Duration = fileMetadata.Duration
	enrichedMeta.BitrateBps = fileMetadata.BitrateBps
	enrichedMeta.Codec = fileMetadata.Codec
	enrichedMeta.PageCount = fileMetadata.PageCount

	// Technical fields from file parser should be preserved
	assert.Equal(t, time.Hour, enrichedMeta.Duration)
	assert.Equal(t, 128000, enrichedMeta.BitrateBps)
	assert.Equal(t, "AAC-LC", enrichedMeta.Codec)
	require.NotNil(t, enrichedMeta.PageCount)
	assert.Equal(t, 42, *enrichedMeta.PageCount)

	// Enricher title should still win
	assert.Equal(t, "Enricher Title", enrichedMeta.Title)
}

// TestMergeEnrichedMetadata_CoverPageMerge verifies that CoverPage is correctly
// merged via mergeEnrichedMetadata.
func TestMergeEnrichedMetadata_CoverPageMerge(t *testing.T) {
	t.Parallel()

	var enrichedMeta mediafile.ParsedMetadata

	// Enricher provides cover page
	coverPage := 3
	enricherSource := "plugin:test/enricher"
	enricherResult := &mediafile.ParsedMetadata{
		CoverPage: &coverPage,
	}
	mergeEnrichedMetadata(&enrichedMeta, enricherResult, enricherSource)

	// File parser also has cover page (should be ignored since enricher already set it)
	fileCoverPage := 0
	fileSource := "cbz_metadata"
	fileMetadata := &mediafile.ParsedMetadata{
		CoverPage: &fileCoverPage,
	}
	mergeEnrichedMetadata(&enrichedMeta, fileMetadata, fileSource)

	require.NotNil(t, enrichedMeta.CoverPage)
	assert.Equal(t, 3, *enrichedMeta.CoverPage)
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["cover"])
}

// TestMergeEnrichedMetadata_ChaptersMerge verifies that Chapters are correctly
// merged via mergeEnrichedMetadata.
func TestMergeEnrichedMetadata_ChaptersMerge(t *testing.T) {
	t.Parallel()

	var enrichedMeta mediafile.ParsedMetadata

	// Enricher provides chapters
	enricherSource := "plugin:test/enricher"
	enricherResult := &mediafile.ParsedMetadata{
		Chapters: []mediafile.ParsedChapter{
			{Title: "Enricher Chapter 1"},
			{Title: "Enricher Chapter 2"},
		},
	}
	mergeEnrichedMetadata(&enrichedMeta, enricherResult, enricherSource)

	// File parser also has chapters (should be ignored)
	fileSource := "epub_metadata"
	fileMetadata := &mediafile.ParsedMetadata{
		Chapters: []mediafile.ParsedChapter{
			{Title: "File Chapter 1"},
		},
	}
	mergeEnrichedMetadata(&enrichedMeta, fileMetadata, fileSource)

	require.Len(t, enrichedMeta.Chapters, 2)
	assert.Equal(t, "Enricher Chapter 1", enrichedMeta.Chapters[0].Title)
	assert.Equal(t, "Enricher Chapter 2", enrichedMeta.Chapters[1].Title)
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["chapters"])
}

// TestMergeEnrichedMetadata_ChaptersFallbackFromFile verifies that when no
// enricher provides chapters, file parser chapters are used as fallback.
func TestMergeEnrichedMetadata_ChaptersFallbackFromFile(t *testing.T) {
	t.Parallel()

	var enrichedMeta mediafile.ParsedMetadata

	// No enricher provides chapters

	// File parser provides chapters
	fileSource := "epub_metadata"
	fileMetadata := &mediafile.ParsedMetadata{
		Chapters: []mediafile.ParsedChapter{
			{Title: "File Chapter 1"},
		},
	}
	mergeEnrichedMetadata(&enrichedMeta, fileMetadata, fileSource)

	require.Len(t, enrichedMeta.Chapters, 1)
	assert.Equal(t, "File Chapter 1", enrichedMeta.Chapters[0].Title)
	assert.Equal(t, fileSource, enrichedMeta.FieldDataSources["chapters"])
}

// TestMergeEnrichedMetadata_AllFields verifies that all content fields are
// correctly handled by the enricher-first merge, with enricher values taking
// precedence over file parser values.
func TestMergeEnrichedMetadata_AllFields(t *testing.T) {
	t.Parallel()

	releaseDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fileReleaseDate := time.Date(2020, 6, 15, 0, 0, 0, 0, time.UTC)
	seriesNum := float64(5)
	fileSeriesNum := float64(1)
	coverPage := 2
	fileCoverPage := 0
	enricherLanguage := "en-US"
	fileLanguage := "fr"
	enricherAbridged := true
	fileAbridged := false

	enricherSource := "plugin:test/enricher"
	fileSource := "epub_metadata"

	// Enricher provides all content fields
	enricherResult := &mediafile.ParsedMetadata{
		Title:         "Enricher Title",
		Subtitle:      "Enricher Subtitle",
		Authors:       []mediafile.ParsedAuthor{{Name: "Enricher Author"}},
		Narrators:     []string{"Enricher Narrator"},
		Series:        "Enricher Series",
		SeriesNumber:  &seriesNum,
		Genres:        []string{"Enricher Genre"},
		Tags:          []string{"enricher-tag"},
		Description:   "Enricher Description",
		Publisher:     "Enricher Publisher",
		Imprint:       "Enricher Imprint",
		URL:           "https://enricher.example.com",
		ReleaseDate:   &releaseDate,
		Language:      &enricherLanguage,
		Abridged:      &enricherAbridged,
		CoverData:     []byte("enricher cover"),
		CoverMimeType: "image/png",
		CoverPage:     &coverPage,
		Identifiers:   []mediafile.ParsedIdentifier{{Type: "asin", Value: "B01"}},
		Chapters:      []mediafile.ParsedChapter{{Title: "Enricher Ch1"}},
	}

	// File parser provides all content fields with different values
	fileMetadata := &mediafile.ParsedMetadata{
		Title:         "File Title",
		Subtitle:      "File Subtitle",
		Authors:       []mediafile.ParsedAuthor{{Name: "File Author"}},
		Narrators:     []string{"File Narrator"},
		Series:        "File Series",
		SeriesNumber:  &fileSeriesNum,
		Genres:        []string{"File Genre"},
		Tags:          []string{"file-tag"},
		Description:   "File Description",
		Publisher:     "File Publisher",
		Imprint:       "File Imprint",
		URL:           "https://file.example.com",
		ReleaseDate:   &fileReleaseDate,
		Language:      &fileLanguage,
		Abridged:      &fileAbridged,
		CoverData:     []byte("file cover"),
		CoverMimeType: "image/jpeg",
		CoverPage:     &fileCoverPage,
		Identifiers:   []mediafile.ParsedIdentifier{{Type: "isbn_13", Value: "978"}},
		Chapters:      []mediafile.ParsedChapter{{Title: "File Ch1"}},
		DataSource:    fileSource,
	}

	// Enricher merge
	var enrichedMeta mediafile.ParsedMetadata
	mergeEnrichedMetadata(&enrichedMeta, enricherResult, enricherSource)

	// File parser fallback
	mergeEnrichedMetadata(&enrichedMeta, fileMetadata, fileSource)

	// All content fields should have enricher values
	assert.Equal(t, "Enricher Title", enrichedMeta.Title)
	assert.Equal(t, "Enricher Subtitle", enrichedMeta.Subtitle)
	require.Len(t, enrichedMeta.Authors, 1)
	assert.Equal(t, "Enricher Author", enrichedMeta.Authors[0].Name)
	require.Len(t, enrichedMeta.Narrators, 1)
	assert.Equal(t, "Enricher Narrator", enrichedMeta.Narrators[0])
	assert.Equal(t, "Enricher Series", enrichedMeta.Series)
	require.NotNil(t, enrichedMeta.SeriesNumber)
	assert.InDelta(t, 5.0, *enrichedMeta.SeriesNumber, 0.01)
	require.Len(t, enrichedMeta.Genres, 1)
	assert.Equal(t, "Enricher Genre", enrichedMeta.Genres[0])
	require.Len(t, enrichedMeta.Tags, 1)
	assert.Equal(t, "enricher-tag", enrichedMeta.Tags[0])
	assert.Equal(t, "Enricher Description", enrichedMeta.Description)
	assert.Equal(t, "Enricher Publisher", enrichedMeta.Publisher)
	assert.Equal(t, "Enricher Imprint", enrichedMeta.Imprint)
	assert.Equal(t, "https://enricher.example.com", enrichedMeta.URL)
	require.NotNil(t, enrichedMeta.ReleaseDate)
	assert.Equal(t, 2025, enrichedMeta.ReleaseDate.Year())
	require.NotNil(t, enrichedMeta.Language)
	assert.Equal(t, "en-US", *enrichedMeta.Language)
	require.NotNil(t, enrichedMeta.Abridged)
	assert.True(t, *enrichedMeta.Abridged)
	assert.Equal(t, []byte("enricher cover"), enrichedMeta.CoverData)
	assert.Equal(t, "image/png", enrichedMeta.CoverMimeType)
	require.NotNil(t, enrichedMeta.CoverPage)
	assert.Equal(t, 2, *enrichedMeta.CoverPage)
	require.Len(t, enrichedMeta.Chapters, 1)
	assert.Equal(t, "Enricher Ch1", enrichedMeta.Chapters[0].Title)

	// Identifiers should be merged additively (both types present)
	require.Len(t, enrichedMeta.Identifiers, 2)

	// All field sources should point to enricher
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["title"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["subtitle"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["authors"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["narrators"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["series"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["genres"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["tags"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["description"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["publisher"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["imprint"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["url"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["releaseDate"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["language"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["abridged"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["cover"])
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["chapters"])
}

// TestMergeEnrichedMetadata_NoEnrichers_FileParserOnly verifies that when no
// enrichers run, the file parser metadata is used as-is.
func TestMergeEnrichedMetadata_NoEnrichers_FileParserOnly(t *testing.T) {
	t.Parallel()

	var enrichedMeta mediafile.ParsedMetadata
	fileSource := "epub_metadata"
	fileMetadata := &mediafile.ParsedMetadata{
		Title:       "File Title",
		Authors:     []mediafile.ParsedAuthor{{Name: "File Author"}},
		Description: "File Description",
		DataSource:  fileSource,
	}

	// Only file parser fallback, no enricher results
	mergeEnrichedMetadata(&enrichedMeta, fileMetadata, fileSource)

	assert.Equal(t, "File Title", enrichedMeta.Title)
	assert.Equal(t, fileSource, enrichedMeta.FieldDataSources["title"])
	require.Len(t, enrichedMeta.Authors, 1)
	assert.Equal(t, "File Author", enrichedMeta.Authors[0].Name)
	assert.Equal(t, "File Description", enrichedMeta.Description)
}

// TestCoverPageProtection_EnricherCannotOverridePageCover verifies that when a
// file parser sets CoverPage (e.g. CBZ/PDF), the enricher's downloaded cover
// image does not replace the file parser's page-derived cover. This simulates
// the post-merge logic in runMetadataEnrichers.
func TestCoverPageProtection_EnricherCannotOverridePageCover(t *testing.T) {
	t.Parallel()

	fileCoverPage := 0
	fileSource := "cbz_metadata"

	// File parser provides cover from page content
	fileMetadata := &mediafile.ParsedMetadata{
		Title:         "My CBZ Book",
		CoverData:     []byte("page-derived cover"),
		CoverMimeType: "image/jpeg",
		CoverPage:     &fileCoverPage,
		DataSource:    fileSource,
		FieldDataSources: map[string]string{
			"cover": fileSource,
		},
	}

	// Enricher provides a downloaded cover image (no CoverPage)
	enricherSource := "plugin:test/enricher"
	enricherResult := &mediafile.ParsedMetadata{
		CoverData:     []byte("enricher-downloaded cover"),
		CoverMimeType: "image/png",
	}

	// Simulate runMetadataEnrichers merge order:
	// 1. Enricher results merge first into empty target
	var enrichedMeta mediafile.ParsedMetadata
	mergeEnrichedMetadata(&enrichedMeta, enricherResult, enricherSource)

	// 2. File parser merges as fallback
	mergeEnrichedMetadata(&enrichedMeta, fileMetadata, fileSource)

	// At this point, enricher's CoverData is in enrichedMeta (it merged first).
	// But CoverPage came from file parser (enricher didn't set it).
	// Verify the problematic intermediate state:
	assert.Equal(t, []byte("enricher-downloaded cover"), enrichedMeta.CoverData)
	require.NotNil(t, enrichedMeta.CoverPage)
	assert.Equal(t, 0, *enrichedMeta.CoverPage)

	// 3. Apply page-based file type protection (as runMetadataEnrichers does).
	// The reset only touches image data; CoverPage is intentionally left alone
	// since the merge already handled "enricher wins if set, file parser fills
	// in otherwise". Since the enricher did NOT set CoverPage here, the
	// "cover" source is restored to the file parser.
	fileType := models.FileTypeCBZ
	if models.IsPageBasedFileType(fileType) {
		enrichedMeta.CoverData = fileMetadata.CoverData
		enrichedMeta.CoverMimeType = fileMetadata.CoverMimeType
		enrichedMeta.FieldDataSources["cover"] = fileMetadata.SourceForField("cover")
	}

	// File parser's image data should be restored; CoverPage came from file
	// parser via merge (enricher didn't set one) and stays there.
	assert.Equal(t, []byte("page-derived cover"), enrichedMeta.CoverData)
	assert.Equal(t, "image/jpeg", enrichedMeta.CoverMimeType)
	require.NotNil(t, enrichedMeta.CoverPage)
	assert.Equal(t, 0, *enrichedMeta.CoverPage)
	assert.Equal(t, fileSource, enrichedMeta.FieldDataSources["cover"])
}

// TestMergeFileParserFallback_EnricherCoverPagePreserved is a regression test
// for a bug where the page-based file protection block unconditionally reset
// CoverPage to the file parser's value, silently discarding an enricher-
// provided coverPage. The feature spec requires enrichers on CBZ/PDF to be
// able to set coverPage — this test exercises mergeFileParserFallback
// directly so the production code path is under test.
func TestMergeFileParserFallback_EnricherCoverPagePreserved(t *testing.T) {
	t.Parallel()

	fileCoverPage := 0
	fileSource := "cbz_metadata"

	// File parser provides cover page 0 (default)
	fileMetadata := &mediafile.ParsedMetadata{
		Title:         "My CBZ Book",
		CoverData:     []byte("page-derived cover"),
		CoverMimeType: "image/jpeg",
		CoverPage:     &fileCoverPage,
		DataSource:    fileSource,
		FieldDataSources: map[string]string{
			"cover": fileSource,
		},
	}

	// Enricher tells Shisho to use page 3 for the cover.
	enricherCoverPage := 3
	enricherSource := "plugin:test/enricher"
	enricherResult := &mediafile.ParsedMetadata{
		CoverPage: &enricherCoverPage,
	}

	// 1. Enricher merges into empty target (simulates runMetadataEnrichers).
	var enrichedMeta mediafile.ParsedMetadata
	mergeEnrichedMetadata(&enrichedMeta, enricherResult, enricherSource)

	// 2. File parser fallback + page-based protection (production code path).
	mergeFileParserFallback(&enrichedMeta, fileMetadata, models.FileTypeCBZ)

	// Enricher's CoverPage must be preserved — this is the whole point of the
	// coverPage feature for page-based formats.
	require.NotNil(t, enrichedMeta.CoverPage)
	assert.Equal(t, 3, *enrichedMeta.CoverPage, "enricher coverPage must not be overwritten by file parser")
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["cover"], "source should reflect enricher, not file parser")

	// File parser's image data should still be restored (enricher image data,
	// if any, is rejected for page-based formats).
	assert.Equal(t, []byte("page-derived cover"), enrichedMeta.CoverData)
	assert.Equal(t, "image/jpeg", enrichedMeta.CoverMimeType)
}

// TestMergeFileParserFallback_PDFEnricherCoverPagePreserved covers the PDF
// variant of the regression fix — same behavior as CBZ.
func TestMergeFileParserFallback_PDFEnricherCoverPagePreserved(t *testing.T) {
	t.Parallel()

	fileCoverPage := 0
	fileSource := "pdf_metadata"
	fileMetadata := &mediafile.ParsedMetadata{
		Title:      "My PDF",
		CoverPage:  &fileCoverPage,
		DataSource: fileSource,
	}

	enricherCoverPage := 5
	enricherSource := "plugin:test/enricher"
	enricherResult := &mediafile.ParsedMetadata{
		CoverPage: &enricherCoverPage,
	}

	var enrichedMeta mediafile.ParsedMetadata
	mergeEnrichedMetadata(&enrichedMeta, enricherResult, enricherSource)
	mergeFileParserFallback(&enrichedMeta, fileMetadata, models.FileTypePDF)

	require.NotNil(t, enrichedMeta.CoverPage)
	assert.Equal(t, 5, *enrichedMeta.CoverPage)
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["cover"])
}

// TestMergeFileParserFallback_NoEnricherCoverPage_FileParserUsed verifies
// that when no enricher provides a CoverPage, the file parser's value is
// used and the source tracks the file parser.
func TestMergeFileParserFallback_NoEnricherCoverPage_FileParserUsed(t *testing.T) {
	t.Parallel()

	fileCoverPage := 0
	fileSource := "cbz_metadata"
	fileMetadata := &mediafile.ParsedMetadata{
		CoverData:     []byte("page-derived cover"),
		CoverMimeType: "image/jpeg",
		CoverPage:     &fileCoverPage,
		DataSource:    fileSource,
	}

	// Enricher provides only downloaded image data (no CoverPage).
	enricherSource := "plugin:test/enricher"
	enricherResult := &mediafile.ParsedMetadata{
		CoverData:     []byte("enricher-downloaded cover"),
		CoverMimeType: "image/png",
	}

	var enrichedMeta mediafile.ParsedMetadata
	mergeEnrichedMetadata(&enrichedMeta, enricherResult, enricherSource)
	mergeFileParserFallback(&enrichedMeta, fileMetadata, models.FileTypeCBZ)

	// Enricher image data is rejected; file parser's cover applied.
	assert.Equal(t, []byte("page-derived cover"), enrichedMeta.CoverData)
	assert.Equal(t, "image/jpeg", enrichedMeta.CoverMimeType)
	require.NotNil(t, enrichedMeta.CoverPage)
	assert.Equal(t, 0, *enrichedMeta.CoverPage)
	assert.Equal(t, fileSource, enrichedMeta.FieldDataSources["cover"])
}

// TestMergeFileParserFallback_NonPageBased_EnricherCoverPreserved verifies
// that EPUB/M4B (non-page-based) files keep the enricher's cover data.
func TestMergeFileParserFallback_NonPageBased_EnricherCoverPreserved(t *testing.T) {
	t.Parallel()

	fileSource := "epub_metadata"
	fileMetadata := &mediafile.ParsedMetadata{
		CoverData:     []byte("epub-embedded cover"),
		CoverMimeType: "image/jpeg",
		DataSource:    fileSource,
	}

	enricherSource := "plugin:test/enricher"
	enricherResult := &mediafile.ParsedMetadata{
		CoverData:     []byte("enricher high-res cover"),
		CoverMimeType: "image/png",
	}

	var enrichedMeta mediafile.ParsedMetadata
	mergeEnrichedMetadata(&enrichedMeta, enricherResult, enricherSource)
	mergeFileParserFallback(&enrichedMeta, fileMetadata, models.FileTypeEPUB)

	// Enricher cover preserved (EPUB is not page-based).
	assert.Equal(t, []byte("enricher high-res cover"), enrichedMeta.CoverData)
	assert.Equal(t, "image/png", enrichedMeta.CoverMimeType)
	assert.Nil(t, enrichedMeta.CoverPage)
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["cover"])
}

// TestCoverPageProtection_NoCoverPage_EnricherCoverPreserved verifies that when
// a file parser does NOT set CoverPage (e.g. EPUB/M4B), the enricher's cover
// is preserved as normal.
func TestCoverPageProtection_NoCoverPage_EnricherCoverPreserved(t *testing.T) {
	t.Parallel()

	fileSource := "epub_metadata"

	// File parser provides cover but no CoverPage
	fileMetadata := &mediafile.ParsedMetadata{
		Title:         "My EPUB Book",
		CoverData:     []byte("epub-embedded cover"),
		CoverMimeType: "image/jpeg",
		DataSource:    fileSource,
	}

	// Enricher provides a better cover
	enricherSource := "plugin:test/enricher"
	enricherResult := &mediafile.ParsedMetadata{
		CoverData:     []byte("enricher high-res cover"),
		CoverMimeType: "image/png",
	}

	// Simulate runMetadataEnrichers merge order
	var enrichedMeta mediafile.ParsedMetadata
	mergeEnrichedMetadata(&enrichedMeta, enricherResult, enricherSource)
	mergeEnrichedMetadata(&enrichedMeta, fileMetadata, fileSource)

	// Page-based file type protection should NOT trigger (EPUB is not page-based)
	fileType := models.FileTypeEPUB
	if models.IsPageBasedFileType(fileType) {
		enrichedMeta.CoverData = fileMetadata.CoverData
		enrichedMeta.CoverMimeType = fileMetadata.CoverMimeType
	}

	// Enricher's cover should be preserved
	assert.Equal(t, []byte("enricher high-res cover"), enrichedMeta.CoverData)
	assert.Equal(t, "image/png", enrichedMeta.CoverMimeType)
	assert.Nil(t, enrichedMeta.CoverPage)
	assert.Equal(t, enricherSource, enrichedMeta.FieldDataSources["cover"])
}
