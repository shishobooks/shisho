// Package linktoken makes and checks the secret tokens in links Shisho hands
// out without an account behind them: Share Links and Invite Links. A token
// is stored as it is, so a pending link's URL can be copied again.
package linktoken

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/pkg/errors"
)

// tokenBytes is the size of a token before encoding. 256 random bits make
// guessing unrealistic, so the public routes need no rate limiter.
const tokenBytes = 32

// tokenLength is the length of an encoded token: unpadded base64url of
// tokenBytes.
var tokenLength = base64.RawURLEncoding.EncodedLen(tokenBytes)

// Generate returns a new random token: 32 bytes from a cryptographically
// secure source, base64url encoded without padding or a prefix.
func Generate() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", errors.WithStack(err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// WellFormed reports whether token could have come from Generate, so a
// mistyped URL is refused without a database lookup.
func WellFormed(token string) bool {
	if len(token) != tokenLength {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil
}
