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

// KOReaderPull is a KOReader Device's latest pull of one document. KOReader
// pulls when it opens or wakes with a book, and pushes when it closes or
// sleeps, and as you read, so the pushes of the same document that follow
// make an estimated Reading Session from the pull. PairedAt is when the
// first did, and SessionID the session they make.
type KOReaderPull struct {
	bun.BaseModel `bun:"table:koreader_pulls,alias:kp" tstype:"-"`

	DeviceID string    `bun:",pk" json:"device_id"`
	Document string    `bun:",pk" json:"document"`
	FileID   int       `bun:",nullzero" json:"file_id"`
	PulledAt time.Time `bun:",notnull" json:"pulled_at"`
	// Percentage is the position the pull found, which the session starts
	// from, or nil when there was none.
	Percentage *float64   `json:"percentage"`
	PairedAt   *time.Time `json:"paired_at"`
	SessionID  *int       `json:"session_id"`
	// EndedAt and EndPercentage are the session's end as the pushes last set
	// it. PreviousEndedAt and PreviousEndPercentage are its end before the
	// push at ExtendedAt moved it, which a replay puts back.
	EndedAt               *time.Time `json:"ended_at"`
	EndPercentage         *float64   `json:"end_percentage"`
	ExtendedAt            *time.Time `json:"extended_at"`
	PreviousEndedAt       *time.Time `json:"previous_ended_at"`
	PreviousEndPercentage *float64   `json:"previous_end_percentage"`
}

// KOReaderPageRow is one row of a KOReader Device's statistics, sent through
// the KOReader plugin: one visit to one page (ADR 0016). Device, Document,
// Page, and StartTime are KOReader's own key. FileID is nil for a document
// Shisho didn't recognize. A Grouped row belongs to ReadingSessionID, or,
// once that's nil, to a session the user deleted, and is never grouped
// again.
type KOReaderPageRow struct {
	bun.BaseModel `bun:"table:koreader_page_rows,alias:kpr" tstype:"-"`

	ID         int       `bun:",pk,autoincrement"`
	CreatedAt  time.Time `bun:",notnull"`
	UserID     int       `bun:",notnull"`
	DeviceID   string    `bun:",notnull"`
	Document   string    `bun:",notnull"`
	Page       int       `bun:",notnull"`
	TotalPages int       `bun:",notnull"`
	// StartTime is the Unix time on the device's clock, as KOReader keys the
	// row, and StartedAt the instant after correcting that clock.
	StartTime        int64     `bun:",notnull"`
	StartedAt        time.Time `bun:",notnull"`
	Duration         int       `bun:",notnull"`
	FileID           *int
	Grouped          bool `bun:",notnull"`
	ReadingSessionID *int
}

// EndedAt is when the visit ended.
func (r *KOReaderPageRow) EndedAt() time.Time {
	return r.StartedAt.Add(time.Duration(r.Duration) * time.Second)
}

// KOReaderPluginEvent records that a Device's journal event was taken in, so
// a repeat is ignored.
type KOReaderPluginEvent struct {
	bun.BaseModel `bun:"table:koreader_plugin_events,alias:kpe" tstype:"-"`

	DeviceID  string    `bun:",pk"`
	EventID   string    `bun:",pk"`
	CreatedAt time.Time `bun:",notnull"`
}
