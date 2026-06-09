package logs

import "time"

// LogEntry represents a parsed log entry stored in the ring buffer.
type LogEntry struct {
	ID        uint64         `json:"id"`
	Level     string         `json:"level" tstype:"LogLevel"`
	Timestamp time.Time      `json:"timestamp"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data,omitempty"`
	Error     *string        `json:"error,omitempty"`
}

// ListLogsQuery defines query parameters for GET /logs.
type ListLogsQuery struct {
	Level   *string `query:"level" json:"level,omitempty" validate:"omitempty,oneof=debug info warn error fatal"`
	Search  *string `query:"search" json:"search,omitempty"`
	Limit   *int    `query:"limit" json:"limit,omitempty" validate:"omitempty,min=1,max=1000"`
	AfterID *uint64 `query:"after_id" json:"after_id,omitempty"`
}

// ListLogsResponse is the GET /logs list-endpoint envelope.
type ListLogsResponse struct {
	Items []LogEntry `json:"items"`
	Total int        `json:"total"`
}
