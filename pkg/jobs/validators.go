package jobs

type CreateJobPayload struct {
	Type      string      `json:"type" validate:"required,oneof=export scan"`
	Data      interface{} `json:"data" validate:"required" tstype:"JobExportData | JobScanData"`
	LibraryID *int        `json:"library_id,omitempty"`
}

type ListJobsQuery struct {
	Limit             int      `query:"limit" json:"limit,omitempty" default:"10" validate:"min=1,max=100"`
	Offset            int      `query:"offset" json:"offset,omitempty" validate:"min=0"`
	Status            []string `query:"status" json:"status,omitempty" validate:"dive,oneof=pending in_progress completed failed"`
	Type              *string  `query:"type" json:"type,omitempty" validate:"omitempty,oneof=export scan"`
	LibraryIDOrGlobal *int     `query:"library_id_or_global" json:"library_id_or_global,omitempty"`
}
