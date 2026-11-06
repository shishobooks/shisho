// Package fingerprint computes content fingerprints for files.
//
// For the MVP this is sha256 over the raw file bytes, used for exact-match
// move/rename detection and future dedup work. The package is structured so
// additional algorithms (pHash, SimHash, Chromaprint, TLSH, etc.) can be added
// as siblings to ComputeSHA256 without callers needing to know which one
// applies to which file type.
package fingerprint

import (
	"crypto/md5" //nolint:gosec // KOReader names documents by MD5.
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

// koreaderSampleSize is how much KOReader's partial MD5 reads at each offset.
const koreaderSampleSize = 1024

// ComputeKOReaderPartialMD5 returns the document hash KOReader's progress
// sync names a file by (ADR 0014), matching its util.partialMD5: the MD5 of 1
// KiB read at offset 0, then at 1 KiB, 4 KiB, 16 KiB, and so on up to 1 GiB,
// stopping at the end of the file. A short final read still counts.
//
// KOReader's loop shifts 1024 by 2*i for i from -1 to 10. The first shift
// count is negative, which LuaJIT masks to a shift that overflows to zero, so
// the first sample is at offset 0, not 256.
func ComputeKOReaderPartialMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errors.Wrap(err, "open file for partial md5")
	}
	defer f.Close()

	h := md5.New() //nolint:gosec // KOReader's protocol names documents by MD5; it isn't protecting anything.
	sample := make([]byte, koreaderSampleSize)
	offsets := []int64{0}
	for shift := 0; shift <= 20; shift += 2 {
		offsets = append(offsets, int64(koreaderSampleSize)<<shift)
	}
	for _, offset := range offsets {
		n, err := f.ReadAt(sample, offset)
		if err != nil && !errors.Is(err, io.EOF) {
			return "", errors.Wrap(err, "read file for partial md5")
		}
		if n == 0 {
			break
		}
		h.Write(sample[:n])
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
