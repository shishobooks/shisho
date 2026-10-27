package libraries

import "github.com/shishobooks/shisho/pkg/models"

// ListLibrariesResponse is the list-endpoint envelope.
type ListLibrariesResponse struct {
	Items []*models.Library `json:"items" tstype:"Library[]"`
	Total int               `json:"total"`
}

// LibrarySummary is one library the caller can access, as listed by
// GET /user/libraries. It carries only what reader pages need (the library
// picker, breadcrumbs, cover aspect ratio, and the
// Merge and Move dialogs), not paths or timestamps, because the route
// requires no permission beyond authentication.
type LibrarySummary struct {
	ID                    int    `json:"id"`
	Name                  string `json:"name"`
	CoverAspectRatio      string `json:"cover_aspect_ratio" tstype:"CoverAspectRatio"`
	OrganizeFileStructure bool   `json:"organize_file_structure"`
	// TagNamespaces group the library's tags on the tags page, the tag
	// filter, and the book page.
	TagNamespaces []string `json:"tag_namespaces"`
}

type CreateLibraryPayload struct {
	Name                  string   `json:"name" validate:"required,max=100"`
	OrganizeFileStructure *bool    `json:"organize_file_structure,omitempty"`
	CoverAspectRatio      string   `json:"cover_aspect_ratio" validate:"required,oneof=book audiobook book_fallback_audiobook audiobook_fallback_book" tstype:"CoverAspectRatio"`
	LibraryPaths          []string `json:"library_paths" validate:"required,min=1,max=50,dive"`
	TagNamespaces         []string `json:"tag_namespaces,omitempty" mod:"dive,trim" validate:"omitempty,max=20,dive,max=50"`
}

type ListLibrariesQuery struct {
	Limit  int `query:"limit" json:"limit,omitempty" default:"10" validate:"min=1,max=100"`
	Offset int `query:"offset" json:"offset,omitempty" validate:"min=0"`
}

type UpdateLibraryPayload struct {
	Name                  *string  `json:"name,omitempty" validate:"omitempty,max=100"`
	OrganizeFileStructure *bool    `json:"organize_file_structure,omitempty"`
	CoverAspectRatio      *string  `json:"cover_aspect_ratio,omitempty" validate:"omitempty,oneof=book audiobook book_fallback_audiobook audiobook_fallback_book" tstype:"CoverAspectRatio"`
	LibraryPaths          []string `json:"library_paths,omitempty" validate:"omitempty,min=1,max=50,dive"`
	// TagNamespaces replaces the library's list when present; [] clears it.
	TagNamespaces []string `json:"tag_namespaces,omitempty" mod:"dive,trim" validate:"omitempty,max=20,dive,max=50"`
}
