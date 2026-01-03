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
	ID        int     `json:"id"`
	Title     string  `json:"title"`
	Subtitle  *string `json:"subtitle"`
	Authors   string  `json:"authors"` // Comma-separated author names
	LibraryID int     `json:"library_id"`
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

// BooksQuery represents the query parameters for book search.
type BooksQuery struct {
	Query     string   `query:"search" json:"search,omitempty" validate:"omitempty,max=100"`
	LibraryID *int     `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	FileTypes []string `query:"file_types" json:"file_types,omitempty"` // Filter by file types (e.g., ["epub", "m4b"])
	Limit     int      `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset    int      `query:"offset" json:"offset,omitempty" validate:"min=0"`
}

// SeriesQuery represents the query parameters for series search.
type SeriesQuery struct {
	Query     string `query:"search" json:"search,omitempty" validate:"omitempty,max=100"`
	LibraryID *int   `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	Limit     int    `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset    int    `query:"offset" json:"offset,omitempty" validate:"min=0"`
}

// PeopleQuery represents the query parameters for people search.
type PeopleQuery struct {
	Query     string `query:"search" json:"search,omitempty" validate:"omitempty,max=100"`
	LibraryID *int   `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	Limit     int    `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset    int    `query:"offset" json:"offset,omitempty" validate:"min=0"`
}
