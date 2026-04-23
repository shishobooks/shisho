package search

// GlobalSearchQuery represents the query parameters for global search.
type GlobalSearchQuery struct {
	Query     string `query:"q" json:"q" validate:"required,min=1,max=100"`
	LibraryID int    `query:"library_id" json:"library_id" validate:"required,min=1"`
}

// GlobalSearchResponse represents the response from global search.
// Returns up to 5 results per resource type for popover display.
type GlobalSearchResponse struct {
	Books  []BookSearchResult   `json:"books"`
	Series []SeriesSearchResult `json:"series"`
	People []PersonSearchResult `json:"people"`
}

// BookSearchResult represents a book in search results.
type BookSearchResult struct {
	ID        int      `json:"id"`
	Title     string   `json:"title"`
	Subtitle  *string  `json:"subtitle"`
	Authors   string   `json:"authors"`    // Comma-separated author names
	FileTypes []string `json:"file_types"` // Unique file types for this book (e.g., ["epub", "m4b"])
	LibraryID int      `json:"library_id"`
}

// SeriesSearchResult represents a series in search results.
type SeriesSearchResult struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	BookCount int    `json:"book_count"`
	LibraryID int    `json:"library_id"`
}

// PersonSearchResult represents a person in search results.
type PersonSearchResult struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	SortName  string `json:"sort_name"`
	LibraryID int    `json:"library_id"`
}
