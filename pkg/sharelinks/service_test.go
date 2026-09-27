package sharelinks

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateToken_LengthAndUniqueness(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for range 1000 {
		token, err := GenerateToken()
		require.NoError(t, err)
		raw, err := base64.RawURLEncoding.DecodeString(token)
		require.NoError(t, err)
		assert.Len(t, raw, 32)
		assert.Len(t, token, 43)
		assert.True(t, wellFormedToken(token))
		assert.False(t, seen[token], "duplicate token %s", token)
		seen[token] = true
	}
}

func TestWellFormedToken_RejectsOtherShapes(t *testing.T) {
	t.Parallel()
	valid, err := GenerateToken()
	require.NoError(t, err)
	for _, token := range []string{"", "abc", valid + "A", valid[:42], valid[:42] + "=", valid[:42] + "+", valid[:42] + "/"} {
		assert.False(t, wellFormedToken(token), token)
	}
}
