package mp4

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertRawMetadata_Language tests language extraction from freeform atoms.
func TestConvertRawMetadata_Language(t *testing.T) {
	t.Parallel()

	t.Run("tone LANGUAGE atom", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{
			freeform: map[string]string{
				"com.pilabor.tone:LANGUAGE": "en",
			},
		}
		meta := convertRawMetadata(raw)
		require.NotNil(t, meta.Language)
		assert.Equal(t, "en", *meta.Language)
	})

	t.Run("iTunes LANGUAGE atom", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{
			freeform: map[string]string{
				"com.apple.iTunes:LANGUAGE": "fr",
			},
		}
		meta := convertRawMetadata(raw)
		require.NotNil(t, meta.Language)
		assert.Equal(t, "fr", *meta.Language)
	})

	t.Run("tone LANGUAGE takes precedence over iTunes LANGUAGE", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{
			freeform: map[string]string{
				"com.pilabor.tone:LANGUAGE": "de",
				"com.apple.iTunes:LANGUAGE": "fr",
			},
		}
		meta := convertRawMetadata(raw)
		require.NotNil(t, meta.Language)
		assert.Equal(t, "de", *meta.Language)
	})

	t.Run("no language freeform atom", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{}
		meta := convertRawMetadata(raw)
		assert.Nil(t, meta.Language)
	})

	t.Run("invalid language tag returns nil", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{
			freeform: map[string]string{
				"com.pilabor.tone:LANGUAGE": "not-a-valid-language-tag-xyz",
			},
		}
		meta := convertRawMetadata(raw)
		assert.Nil(t, meta.Language)
	})
}

// TestConvertRawMetadata_Abridged tests abridged status extraction from freeform atoms.
func TestConvertRawMetadata_Abridged(t *testing.T) {
	t.Parallel()

	t.Run("ABRIDGED true", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{
			freeform: map[string]string{
				"com.pilabor.tone:ABRIDGED": "true",
			},
		}
		meta := convertRawMetadata(raw)
		require.NotNil(t, meta.Abridged)
		assert.True(t, *meta.Abridged)
	})

	t.Run("ABRIDGED false", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{
			freeform: map[string]string{
				"com.pilabor.tone:ABRIDGED": "false",
			},
		}
		meta := convertRawMetadata(raw)
		require.NotNil(t, meta.Abridged)
		assert.False(t, *meta.Abridged)
	})

	t.Run("ABRIDGED case insensitive true", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{
			freeform: map[string]string{
				"com.pilabor.tone:ABRIDGED": "TRUE",
			},
		}
		meta := convertRawMetadata(raw)
		require.NotNil(t, meta.Abridged)
		assert.True(t, *meta.Abridged)
	})

	t.Run("ABRIDGED case insensitive false", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{
			freeform: map[string]string{
				"com.pilabor.tone:ABRIDGED": "False",
			},
		}
		meta := convertRawMetadata(raw)
		require.NotNil(t, meta.Abridged)
		assert.False(t, *meta.Abridged)
	})

	t.Run("ABRIDGED with surrounding whitespace", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{
			freeform: map[string]string{
				"com.pilabor.tone:ABRIDGED": "  true  ",
			},
		}
		meta := convertRawMetadata(raw)
		require.NotNil(t, meta.Abridged)
		assert.True(t, *meta.Abridged)
	})

	t.Run("ABRIDGED unrecognized value returns nil", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{
			freeform: map[string]string{
				"com.pilabor.tone:ABRIDGED": "yes",
			},
		}
		meta := convertRawMetadata(raw)
		assert.Nil(t, meta.Abridged)
	})

	t.Run("no ABRIDGED freeform atom", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{}
		meta := convertRawMetadata(raw)
		assert.Nil(t, meta.Abridged)
	})
}

// TestConvertRawMetadata_NoFreeform tests that Language and Abridged are nil when
// there are no freeform atoms at all.
func TestConvertRawMetadata_NoFreeform(t *testing.T) {
	t.Parallel()
	raw := &rawMetadata{
		title:  "Test Book",
		artist: "Test Author",
	}
	meta := convertRawMetadata(raw)
	assert.Nil(t, meta.Language)
	assert.Nil(t, meta.Abridged)
}

// TestConvertRawMetadata_SeriesFromFreeform tests that series is parsed from
// the Audible-style freeform SERIES / SERIES-PART atoms.
func TestConvertRawMetadata_SeriesFromFreeform(t *testing.T) {
	t.Parallel()

	t.Run("freeform SERIES and SERIES-PART", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{
			freeform: map[string]string{
				"com.apple.iTunes:SERIES":      "Expanse",
				"com.apple.iTunes:SERIES-PART": "3",
			},
		}
		meta := convertRawMetadata(raw)
		assert.Equal(t, "Expanse", meta.Series)
		require.NotNil(t, meta.SeriesNumber)
		assert.InDelta(t, 3.0, *meta.SeriesNumber, 0.001)
	})

	t.Run("freeform SERIES with decimal SERIES-PART", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{
			freeform: map[string]string{
				"com.apple.iTunes:SERIES":      "Expanse",
				"com.apple.iTunes:SERIES-PART": "3.5",
			},
		}
		meta := convertRawMetadata(raw)
		assert.Equal(t, "Expanse", meta.Series)
		require.NotNil(t, meta.SeriesNumber)
		assert.InDelta(t, 3.5, *meta.SeriesNumber, 0.001)
	})

	t.Run("freeform SERIES without SERIES-PART", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{
			freeform: map[string]string{
				"com.apple.iTunes:SERIES": "Expanse",
			},
		}
		meta := convertRawMetadata(raw)
		assert.Equal(t, "Expanse", meta.Series)
		assert.Nil(t, meta.SeriesNumber)
	})

	t.Run("freeform beats grouping", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{
			grouping: "Fallback Series #9",
			freeform: map[string]string{
				"com.apple.iTunes:SERIES":      "Preferred Series",
				"com.apple.iTunes:SERIES-PART": "1",
			},
		}
		meta := convertRawMetadata(raw)
		assert.Equal(t, "Preferred Series", meta.Series)
		require.NotNil(t, meta.SeriesNumber)
		assert.InDelta(t, 1.0, *meta.SeriesNumber, 0.001)
	})
}

// TestConvertRawMetadata_SeriesFromGrouping tests that series falls back to
// parsing from the ©grp grouping atom when no freeform SERIES exists.
func TestConvertRawMetadata_SeriesFromGrouping(t *testing.T) {
	t.Parallel()

	t.Run("grouping with hash format", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{grouping: "Dungeon Crawler Carl #7"}
		meta := convertRawMetadata(raw)
		assert.Equal(t, "Dungeon Crawler Carl", meta.Series)
		require.NotNil(t, meta.SeriesNumber)
		assert.InDelta(t, 7.0, *meta.SeriesNumber, 0.001)
	})

	t.Run("grouping with book format", func(t *testing.T) {
		t.Parallel()
		raw := &rawMetadata{grouping: "Mistborn, Book 3"}
		meta := convertRawMetadata(raw)
		assert.Equal(t, "Mistborn", meta.Series)
		require.NotNil(t, meta.SeriesNumber)
		assert.InDelta(t, 3.0, *meta.SeriesNumber, 0.001)
	})
}

// TestConvertRawMetadata_AlbumIsNotSeriesSource verifies that series is NOT
// extracted from the album atom, regardless of origin. Files that stored
// series in album (older Shisho versions or third-party taggers) will lose
// their parsed series on re-scan; for Shisho-managed books the series still
// lives in the database, and regenerating the file writes the new atoms.
func TestConvertRawMetadata_AlbumIsNotSeriesSource(t *testing.T) {
	t.Parallel()
	raw := &rawMetadata{album: "Some Series #2"}
	meta := convertRawMetadata(raw)
	assert.Equal(t, "Some Series #2", meta.Album) // album still readable
	assert.Empty(t, meta.Series)
	assert.Nil(t, meta.SeriesNumber)
}
