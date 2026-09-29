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
