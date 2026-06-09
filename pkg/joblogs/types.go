package joblogs

import "github.com/shishobooks/shisho/pkg/models"

type ListJobLogsQuery struct {
	AfterID *int     `query:"after_id" json:"after_id,omitempty"`
	Level   []string `query:"level" json:"level,omitempty" validate:"dive,oneof=debug info warn error fatal" tstype:"LogLevel[]"`
	Search  *string  `query:"search" json:"search,omitempty"`
	Plugin  *string  `query:"plugin" json:"plugin,omitempty"`
}

// ListJobLogsResponse is the GET /jobs/:id/logs list-endpoint envelope. The job
// itself is no longer bundled here; clients fetch it separately via GET /jobs/:id.
type ListJobLogsResponse struct {
	Items []*models.JobLog `json:"items" tstype:"JobLog[]"`
	Total int              `json:"total"`
}
