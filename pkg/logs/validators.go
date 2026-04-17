package logs

// ListLogsQuery defines query parameters for GET /logs.
type ListLogsQuery struct {
	Level   *string `query:"level" json:"level,omitempty" validate:"omitempty,oneof=debug info warn error"`
	Search  *string `query:"search" json:"search,omitempty"`
	Limit   *int    `query:"limit" json:"limit,omitempty" validate:"omitempty,min=1,max=1000"`
	AfterID *uint64 `query:"after_id" json:"after_id,omitempty"`
}
