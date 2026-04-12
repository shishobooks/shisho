package models

import (
	"time"

	"github.com/uptrace/bun"
)

// Fingerprint algorithm identifiers stored in FileFingerprint.Algorithm.
const (
	// FingerprintAlgorithmSHA256 is the exact-content sha256 hash over the
	// file's raw bytes. Used for move/rename detection.
	FingerprintAlgorithmSHA256 = "sha256"
)

// FileFingerprint is a content fingerprint for a file. A single file may have
// multiple fingerprints, one per algorithm (e.g. sha256 for exact matching,
// phash for cover similarity, simhash for text similarity).
type FileFingerprint struct {
	bun.BaseModel `bun:"table:file_fingerprints,alias:ffp" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	FileID    int       `bun:",nullzero" json:"file_id"`
	Algorithm string    `bun:",nullzero" json:"algorithm"`
	Value     string    `bun:",nullzero" json:"value"`

	File *File `bun:"rel:belongs-to" json:"-"`
}
