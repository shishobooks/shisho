package bookpreviews

// PreviewBook is a book shown as a cover on a list entry.
type PreviewBook struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	CoverCacheKey string `json:"cover_cache_key"`
	// The shape the gallery gives this book's cover in its library.
	CoverAspect string `json:"cover_aspect" tstype:"ThumbnailAspect"`
}
