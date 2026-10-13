package cache

// Info describes one cache. GET /cache returns a bare array of them.
// The type name avoids stuttering (cache.Info instead of cache.CacheInfo).
type Info struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SizeBytes   int64  `json:"size_bytes"`
	FileCount   int    `json:"file_count"`
}

// ClearResponse is returned from POST /cache/:id/clear.
type ClearResponse struct {
	ClearedBytes int64 `json:"cleared_bytes"`
	ClearedFiles int   `json:"cleared_files"`
}

// Limit is one cache's disk budget.
type Limit struct {
	// MaxSizeGB is kept while Unlimited is set, so lifting the limit and
	// restoring it later brings back the same size.
	MaxSizeGB float64 `json:"max_size_gb"`
	Unlimited bool    `json:"unlimited"`
}

// SettingsResponse contains the admin-editable cache policy. Each field is
// named after the id of its cache in GET /cache.
type SettingsResponse struct {
	CoverThumbnails Limit `json:"cover_thumbnails"`
	Downloads       Limit `json:"downloads"`
	CBZPages        Limit `json:"cbz_pages"`
	PDFPages        Limit `json:"pdf_pages"`
}

// LimitPayload sets one cache's limit. Both fields are required, so a client
// that leaves out the size cannot empty the cache by reading it as 0.
type LimitPayload struct {
	MaxSizeGB *float64 `json:"max_size_gb" validate:"required,min=0,max=1024" tstype:"number,required"`
	Unlimited *bool    `json:"unlimited" validate:"required" tstype:"boolean,required"`
}

// UpdateSettingsPayload changes the limits it names and leaves the rest.
type UpdateSettingsPayload struct {
	CoverThumbnails *LimitPayload `json:"cover_thumbnails,omitempty"`
	Downloads       *LimitPayload `json:"downloads,omitempty"`
	CBZPages        *LimitPayload `json:"cbz_pages,omitempty"`
	PDFPages        *LimitPayload `json:"pdf_pages,omitempty"`
}
