package reflowable

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A place the web reader or KOReader saved goes to a Kobo by the estimate:
// the nearest span of the KePub Shisho delivers, which the KePub itself has,
// in the same spine item and the same paragraph region, with how far
// through the chapter the place is.
func TestTranslateForKoboLandsInTheSameParagraph(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	words := loadJSON[harnessWord](t, "handoff-xpointers.json")
	epubPath, kepubPath := buildFixture(t)
	plain, err := Open(ctx, epubPath, false)
	require.NoError(t, err)
	// The KePub a Kobo holds, read with its own span markup.
	delivered, err := Open(ctx, kepubPath, true)
	require.NoError(t, err)

	for _, word := range words["epub"] {
		start, err := plain.FromXPointer(word.XPointer)
		require.NoError(t, err)
		cfi := plain.CFI(start)
		position := &models.ReadingPosition{ReadingLocator: models.ReadingLocator{CFI: &cfi}, Percentage: 0.5}

		bookmark, err := TranslateForKobo(ctx, epubPath, models.FileTypeEPUB, position, models.FileTypeKePub)
		require.NoError(t, err, cfi)
		landed, err := delivered.FromKoboSpan(bookmark.Span)
		require.NoError(t, err, "%s: span %v isn't in the delivered KePub", cfi, bookmark.Span)
		assert.Equal(t, start.Spine, landed.Spine, cfi)
		assert.InDelta(t, start.Offset, landed.Offset, 100, "%s landed at %q", word.XPointer, plain.textAt(landed, 30))
		assert.InDelta(t, start.Fraction, bookmark.ChapterFraction, 0.001, cfi)
	}
}

// A Kobo's place comes back to the web reader from its span, in the same
// paragraph region, whether the File is the EPUB the KePub was made from or
// the KePub itself.
func TestTranslateFromKoboSpan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	words := loadJSON[harnessWord](t, "handoff-xpointers.json")
	epubPath, kepubPath := buildFixture(t)
	plain, err := Open(ctx, epubPath, false)
	require.NoError(t, err)
	kobo, err := Open(ctx, kepubPath, true)
	require.NoError(t, err)
	kepubType := models.FileTypeKePub

	for _, file := range []struct {
		path, fileType string
		locatorType    *string
	}{
		{epubPath, models.FileTypeEPUB, &kepubType},
		{kepubPath, models.FileTypeKePub, nil},
	} {
		t.Run(file.fileType, func(t *testing.T) {
			t.Parallel()
			for _, word := range words["epub"] {
				start, err := plain.FromXPointer(word.XPointer)
				require.NoError(t, err)
				span, err := kobo.KoboSpan(start)
				require.NoError(t, err)
				stored := span.String()
				position := &models.ReadingPosition{
					ReadingLocator:  models.ReadingLocator{KoboSpan: &stored},
					LocatorFileType: file.locatorType,
					Percentage:      0.5,
				}

				locator, err := Translate(ctx, file.path, file.fileType, position, file.fileType, FormCFI)
				require.NoError(t, err)
				require.NotNil(t, locator.CFI)
				web, err := Open(ctx, file.path, file.fileType == models.FileTypeKePub)
				require.NoError(t, err)
				landed, err := web.FromCFI(*locator.CFI)
				require.NoError(t, err)
				assert.Equal(t, start.Spine, landed.Spine, stored)
				assert.InDelta(t, start.Offset, landed.Offset, 100, "%s landed at %q", stored, plain.textAt(landed, 30))
			}
		})
	}
}

// A span the KePub doesn't have, such as one from a KePub made another way,
// lands in its chapter at the Kobo's percentage through it, and a chapter
// the book doesn't have at the percentage through the book.
func TestLocateKoboSpanFallsBackToTheEstimate(t *testing.T) {
	t.Parallel()
	books := variants(t)
	kobo := books["kepub"]["KePub as itself"]
	half := 0.5

	place := kobo.LocateKoboSpan(KoboSpan{Source: "OEBPS/text/two.xhtml", ID: "kobo.999.9"}, &half, 0.1)
	assert.False(t, place.Exact)
	assert.Equal(t, 3, place.Spine)
	assert.InDelta(t, 0.5, place.Fraction, 0.001)

	// Without the chapter's percentage, the start of the chapter.
	place = kobo.LocateKoboSpan(KoboSpan{Source: "OEBPS/text/two.xhtml", ID: "kobo.999.9"}, nil, 0.1)
	assert.Equal(t, 3, place.Spine)
	assert.Zero(t, place.Offset)

	place = kobo.LocateKoboSpan(KoboSpan{Source: "OEBPS/text/missing.xhtml", ID: "kobo.1.1"}, &half, 0.9)
	assert.False(t, place.Exact)
	assert.Equal(t, kobo.AtPercentage(0.9), place)
}

// A Kobo may name a chapter relative to the package document rather than
// the archive's root.
func TestLocateKoboSpanMatchesARelativeSource(t *testing.T) {
	t.Parallel()
	books := variants(t)
	kobo := books["kepub"]["KePub as itself"]

	exact, err := kobo.FromKoboSpan(KoboSpan{Source: "OEBPS/text/one.xhtml", ID: "kobo.2.1"})
	require.NoError(t, err)
	relative, err := kobo.FromKoboSpan(KoboSpan{Source: "text/one.xhtml", ID: "kobo.2.1"})
	require.NoError(t, err)
	assert.Equal(t, exact, relative)
}

func TestKoboSpanStringRoundTrips(t *testing.T) {
	t.Parallel()
	span := KoboSpan{Source: "OEBPS/text/one.xhtml", ID: "kobo.12.3"}
	parsed, ok := ParseKoboSpan(span.String())
	require.True(t, ok)
	assert.Equal(t, span, parsed)

	for _, bad := range []string{"", "kobo.1.1", "#kobo.1.1", "OEBPS/one.xhtml#"} {
		_, ok := ParseKoboSpan(bad)
		assert.False(t, ok, bad)
	}
}
