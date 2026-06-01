package opds

// PaginationQuery represents pagination query parameters.
type PaginationQuery struct {
	Limit  int `query:"limit"`
	Offset int `query:"offset"`
}

// SearchQuery represents search query parameters.
type SearchQuery struct {
	Q      string `query:"q"`
	Limit  int    `query:"limit"`
	Offset int    `query:"offset"`
}
