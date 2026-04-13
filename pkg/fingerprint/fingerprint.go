// Package fingerprint computes content fingerprints for files.
//
// For the MVP this is sha256 over the raw file bytes, used for exact-match
// move/rename detection and future dedup work. The package is structured so
// additional algorithms (pHash, SimHash, Chromaprint, TLSH, etc.) can be added
// as siblings to ComputeSHA256 without callers needing to know which one
// applies to which file type.
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"github.com/pkg/errors"
)

// ComputeSHA256 returns the lowercase hex-encoded sha256 of the file's contents.
// It streams the file in fixed-size chunks so it can handle multi-GB files
// without loading them into memory.
func ComputeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errors.Wrap(err, "open file for sha256")
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", errors.Wrap(err, "read file for sha256")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
