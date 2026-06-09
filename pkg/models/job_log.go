package models

import (
	"time"

	"github.com/uptrace/bun"
)

type JobLog struct {
	bun.BaseModel `bun:"table:job_logs,alias:jl" tstype:"-"`

	ID         int       `bun:",pk,nullzero" json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	JobID      int       `bun:",nullzero" json:"job_id"`
	Level      string    `bun:",nullzero" json:"level" tstype:"LogLevel"`
	Message    string    `bun:",nullzero" json:"message"`
	Plugin     *string   `json:"plugin,omitempty"`
	Data       *string   `json:"data,omitempty"`
	StackTrace *string   `json:"stack_trace,omitempty"`
}
