package joblogs

type ListJobLogsQuery struct {
	AfterID *int     `query:"after_id" json:"after_id,omitempty"`
	Level   []string `query:"level" json:"level,omitempty" validate:"dive,oneof=info warn error fatal"`
	Search  *string  `query:"search" json:"search,omitempty"`
	Plugin  *string  `query:"plugin" json:"plugin,omitempty"`
}
