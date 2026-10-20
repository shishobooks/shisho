package devices

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCredential_SixteenLettersWithoutLookalikes(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for range 200 {
		credential, err := generateCredential()
		require.NoError(t, err)
		require.Len(t, credential, 16)
		for _, r := range credential {
			assert.True(t, r >= 'a' && r <= 'z', "credential %q has %q, which is not a lowercase letter", credential, r)
		}
		assert.False(t, strings.ContainsAny(credential, "lio"), "credential %q has a lookalike letter", credential)
		seen[credential] = true
	}
	assert.Len(t, seen, 200, "credentials repeat")
}

func TestGenerateCredential_UsesTheWholeAlphabet(t *testing.T) {
	t.Parallel()
	letters := map[rune]bool{}
	for range 200 {
		credential, err := generateCredential()
		require.NoError(t, err)
		for _, r := range credential {
			letters[r] = true
		}
	}
	assert.Len(t, letters, 23)
}
