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
	// FingerprintAlgorithmKOReaderPartialMD5 is KOReader's partial MD5 of the
	// file's current bytes. Its row says those bytes are in the document hash
	// map (DocumentHash); a change to the file deletes the row, not the map's.
	FingerprintAlgorithmKOReaderPartialMD5 = "koreader_partial_md5"
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

// DocumentHash maps a KOReader document hash to a File and the File Type it
// was delivered as: the Original File, or a Generated File the download cache
// published (ADR 0014). One hash can map to several Files, when identical
// bytes exist more than once. A row lasts as long as its File, since a copy on
// a device keeps its hash after the File's bytes or metadata change.
type DocumentHash struct {
	bun.BaseModel `bun:"table:koreader_document_hashes,alias:kdh" tstype:"-"`

	Hash              string    `bun:",pk" json:"hash"`
	FileID            int       `bun:",pk" json:"file_id"`
	DeliveredFileType string    `bun:",pk" json:"delivered_file_type"`
	CreatedAt         time.Time `json:"created_at"`

	File *File `bun:"rel:belongs-to,join:file_id=id" json:"-"`
}
