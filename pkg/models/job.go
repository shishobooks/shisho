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
	//tygo:emit export type JobType = typeof JobTypeExport | typeof JobTypeScan;
	JobTypeExport = "export"
	JobTypeScan   = "scan"
)

type Job struct {
	bun.BaseModel `bun:"table:jobs,alias:j" tstype:"-"`

	ID         int         `bun:",pk,nullzero" json:"id"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
	Type       string      `bun:",nullzero" json:"type" tstype:"JobType"`
	Status     string      `bun:",nullzero" json:"status" tstype:"JobStatus"`
	Data       string      `bun:",nullzero" json:"-"`
	DataParsed interface{} `bun:"-" json:"data" tstype:"JobExportData | JobScanData"`
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
	}

	err := json.Unmarshal([]byte(job.Data), job.DataParsed)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

type JobExportData struct{}

type JobScanData struct{}
