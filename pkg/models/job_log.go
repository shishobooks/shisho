package models

import (
	"time"

	"github.com/uptrace/bun"
)

const (
	//tygo:emit export type JobLogLevel = typeof JobLogLevelInfo | typeof JobLogLevelWarn | typeof JobLogLevelError | typeof JobLogLevelFatal;
	JobLogLevelInfo  = "info"
	JobLogLevelWarn  = "warn"
	JobLogLevelError = "error"
	JobLogLevelFatal = "fatal"
)

type JobLog struct {
	bun.BaseModel `bun:"table:job_logs,alias:jl" tstype:"-"`

	ID         int       `bun:",pk,nullzero" json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	JobID      int       `bun:",nullzero" json:"job_id"`
	Level      string    `bun:",nullzero" json:"level" tstype:"JobLogLevel"`
	Message    string    `bun:",nullzero" json:"message"`
	Plugin     *string   `json:"plugin,omitempty"`
	Data       *string   `json:"data,omitempty"`
	StackTrace *string   `json:"stack_trace,omitempty"`
}
