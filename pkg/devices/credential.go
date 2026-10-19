package devices

import (
	"crypto/rand"
	"math/big"

	"github.com/pkg/errors"
)

// credentialAlphabet is the lowercase letters without l, i, and o, which an
// e-ink keyboard's font makes easy to mistake for 1, j, and 0. People type
// credentials on those keyboards, so a credential is shown and accepted only
// as one unbroken run of these letters: no grouping, hyphens, or case
// folding, since a reading app's input field can't be controlled.
const credentialAlphabet = "abcdefghjkmnpqrstuvwxyz"

// credentialLength gives 23^16, about 2^72, possible credentials.
const credentialLength = 16

// generateCredential returns a new random Device credential.
func generateCredential() (string, error) {
	alphabetSize := big.NewInt(int64(len(credentialAlphabet)))
	b := make([]byte, credentialLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, alphabetSize)
		if err != nil {
			return "", errors.WithStack(err)
		}
		b[i] = credentialAlphabet[n.Int64()]
	}
	return string(b), nil
}
