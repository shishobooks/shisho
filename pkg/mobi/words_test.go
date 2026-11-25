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
