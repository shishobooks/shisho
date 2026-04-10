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
