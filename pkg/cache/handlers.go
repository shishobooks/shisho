package cache

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

// Provider is the minimal interface a cache must implement to be managed.
type Provider interface {
	SizeBytes() (int64, int, error)
	Clear() error
}

// Handler exposes HTTP endpoints for cache management.
type Handler struct {
	downloads Provider
	cbzPages  Provider
	pdfPages  Provider
}

// NewHandler returns a new cache management handler.
func NewHandler(downloads, cbzPages, pdfPages Provider) *Handler {
	return &Handler{
		downloads: downloads,
		cbzPages:  cbzPages,
		pdfPages:  pdfPages,
	}
}

// CacheInfo describes a single cache in the list response.
type CacheInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SizeBytes   int64  `json:"size_bytes"`
	FileCount   int    `json:"file_count"`
}

// ListResponse is returned from GET /cache.
type ListResponse struct {
	Caches []CacheInfo `json:"caches"`
}

// ClearResponse is returned from POST /cache/:id/clear.
type ClearResponse struct {
	ClearedBytes int64 `json:"cleared_bytes"`
	ClearedFiles int   `json:"cleared_files"`
}

type cacheEntry struct {
	id          string
	name        string
	description string
	provider    Provider
}

func (h *Handler) entries() []cacheEntry {
	return []cacheEntry{
		{
			id:          "downloads",
			name:        "Downloads",
			description: "Generated format conversions (e.g. kepub), plugin-generated files, and bulk-download zips.",
			provider:    h.downloads,
		},
		{
			id:          "cbz_pages",
			name:        "CBZ Pages",
			description: "Page images extracted from CBZ files for the in-app reader.",
			provider:    h.cbzPages,
		},
		{
			id:          "pdf_pages",
			name:        "PDF Pages",
			description: "JPEGs rendered from PDF pages for the in-app reader.",
			provider:    h.pdfPages,
		},
	}
}

func (h *Handler) list(c echo.Context) error {
	entries := h.entries()
	resp := ListResponse{Caches: make([]CacheInfo, 0, len(entries))}
	for _, e := range entries {
		bytes, count, err := e.provider.SizeBytes()
		if err != nil {
			return errors.Wrapf(err, "failed to compute size for %s", e.id)
		}
		resp.Caches = append(resp.Caches, CacheInfo{
			ID:          e.id,
			Name:        e.name,
			Description: e.description,
			SizeBytes:   bytes,
			FileCount:   count,
		})
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) clear(c echo.Context) error {
	id := c.Param("id")

	entries := h.entries()
	var entry *cacheEntry
	for i := range entries {
		if entries[i].id == id {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		return errcodes.NotFound("cache")
	}

	bytes, count, err := entry.provider.SizeBytes()
	if err != nil {
		return errors.Wrapf(err, "failed to compute size for %s before clear", id)
	}
	if err := entry.provider.Clear(); err != nil {
		return errors.Wrapf(err, "failed to clear %s", id)
	}

	return c.JSON(http.StatusOK, ClearResponse{
		ClearedBytes: bytes,
		ClearedFiles: count,
	})
}
