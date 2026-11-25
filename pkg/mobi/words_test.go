package mobi

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_WordCount(t *testing.T) {
	t.Parallel()

	for name, opts := range map[string]testgen.MOBIOptions{
		"MOBI6": testgen.KF7Sample(),
		"KF8":   testgen.KF8Sample(testgen.MOBIKindKF8),
		"combo": testgen.KF8Sample(testgen.MOBIKindCombo),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := testgen.GenerateMOBI(t, t.TempDir(), "book.mobi", opts)
			meta, err := Parse(path)
			require.NoError(t, err)
			require.NotNil(t, meta.WordCount)
			// The generated chapters hold 120 seven-word filler paragraphs
			// and a few short ones, and no markup counts.
			assert.Greater(t, *meta.WordCount, 840)
			assert.Less(t, *meta.WordCount, 1000)

			n, err := WordCount(context.Background(), path)
			require.NoError(t, err)
			assert.Equal(t, *meta.WordCount, n)
		})
	}
}

func TestWordCount_DRMProtected(t *testing.T) {
	t.Parallel()

	opts := testgen.KF7Sample()
	opts.Encrypted = true
	path := testgen.GenerateMOBI(t, t.TempDir(), "book.mobi", opts)
	_, err := WordCount(context.Background(), path)
	assert.ErrorIs(t, err, mediafile.ErrDRMProtected)
}

func TestWordCount_PictureBook(t *testing.T) {
	t.Parallel()

	opts := testgen.KF8Sample(testgen.MOBIKindKF8)
	opts.KF8.Parts = []testgen.MOBIKF8Part{
		{Fragments: []string{`<p>A short introduction to the pictures that follow.</p>`}},
		{Fragments: []string{`<p><img src="kindle:embed:img:0" alt=""/></p>`}},
		{Fragments: []string{`<p><img src="kindle:embed:img:0" alt=""/></p>`}},
	}
	opts.TOC = nil
	path := testgen.GenerateMOBI(t, t.TempDir(), "comic.azw3", opts)
	_, err := WordCount(context.Background(), path)
	require.Error(t, err, "two picture pages of three")

	// The same book with words on its pages is counted.
	opts.KF8.Parts[1].Fragments = []string{`<p>One two three.</p>`}
	opts.KF8.Parts[2].Fragments = []string{`<p>Four five six.</p>`}
	path = testgen.GenerateMOBI(t, t.TempDir(), "book.azw3", opts)
	n, err := WordCount(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, 14, n)
}
