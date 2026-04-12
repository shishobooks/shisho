package models

import (
	"time"

	"github.com/pkg/errors"
	"github.com/segmentio/encoding/json"
	"github.com/uptrace/bun"
)

const (
	//tygo:emit export type JobStatus = typeof JobStatusPending | typeof JobStatusInProgress | typeof JobStatusCompleted | typeof JobStatusFailed;
	JobStatusPending    = "pending"
	JobStatusInProgress = "in_progress"
	JobStatusCompleted  = "completed"
	JobStatusFailed     = "failed"
)

const (
	//tygo:emit export type JobType = typeof JobTypeExport | typeof JobTypeScan | typeof JobTypeBulkDownload | typeof JobTypeHashGeneration;
	JobTypeExport         = "export"
	JobTypeScan           = "scan"
	JobTypeBulkDownload   = "bulk_download"
	JobTypeHashGeneration = "hash_generation"
)

type Job struct {
	bun.BaseModel `bun:"table:jobs,alias:j" tstype:"-"`

	ID         int         `bun:",pk,nullzero" json:"id"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
	Type       string      `bun:",nullzero" json:"type" tstype:"JobType"`
	Status     string      `bun:",nullzero" json:"status" tstype:"JobStatus"`
	Data       string      `bun:",nullzero" json:"-"`
	DataParsed interface{} `bun:"-" json:"data" tstype:"JobExportData | JobScanData | JobBulkDownloadData | JobHashGenerationData"`
	Progress   int         `json:"progress"`
	ProcessID  *string     `json:"process_id,omitempty"`
	LibraryID  *int        `json:"library_id,omitempty"`
}

func (job *Job) UnmarshalData() error {
	switch job.Type {
	case JobTypeExport:
		job.DataParsed = &JobExportData{}
	case JobTypeScan:
		job.DataParsed = &JobScanData{}
	case JobTypeBulkDownload:
		job.DataParsed = &JobBulkDownloadData{}
	case JobTypeHashGeneration:
		job.DataParsed = &JobHashGenerationData{}
	}

	err := json.Unmarshal([]byte(job.Data), job.DataParsed)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

type JobExportData struct{}

type JobScanData struct{}

// JobHashGenerationData is the payload for a hash generation job.
// The job processes all files in the given library that do not yet have
// a sha256 fingerprint in file_fingerprints.
type JobHashGenerationData struct {
	LibraryID int `json:"library_id"`
}

type JobBulkDownloadData struct {
	// Input (set on creation)
	FileIDs            []int `json:"file_ids"`
	EstimatedSizeBytes int64 `json:"estimated_size_bytes"`

	// Result (set on completion)
	ZipFilename     string `json:"zip_filename,omitempty"`
	SizeBytes       int64  `json:"size_bytes,omitempty"`
	FileCount       int    `json:"file_count,omitempty"`
	FingerprintHash string `json:"fingerprint_hash,omitempty"`
}
