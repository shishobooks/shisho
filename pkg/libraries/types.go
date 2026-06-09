package libraries

import "github.com/shishobooks/shisho/pkg/models"

// LibraryResponse is the single-library API response. It embeds the Library
// model by value so tygo emits `extends Library`; the wire format is identical
// to returning the bare model.
type LibraryResponse struct {
	models.Library `tstype:",extends"`
}

// ListLibrariesResponse is the list-endpoint envelope.
type ListLibrariesResponse struct {
	Items []*models.Library `json:"items" tstype:"Library[]"`
	Total int               `json:"total"`
}

type CreateLibraryPayload struct {
	Name                     string   `json:"name" validate:"required,max=100"`
	OrganizeFileStructure    *bool    `json:"organize_file_structure,omitempty"`
	CoverAspectRatio         string   `json:"cover_aspect_ratio" validate:"required,oneof=book audiobook book_fallback_audiobook audiobook_fallback_book" tstype:"CoverAspectRatio"`
	DownloadFormatPreference *string  `json:"download_format_preference,omitempty" validate:"omitempty,oneof=original kepub ask" tstype:"DownloadFormat"`
	LibraryPaths             []string `json:"library_paths" validate:"required,min=1,max=50,dive"`
}

type ListLibrariesQuery struct {
	Limit  int `query:"limit" json:"limit,omitempty" default:"10" validate:"min=1,max=100"`
	Offset int `query:"offset" json:"offset,omitempty" validate:"min=0"`
}

type UpdateLibraryPayload struct {
	Name                     *string  `json:"name,omitempty" validate:"omitempty,max=100"`
	OrganizeFileStructure    *bool    `json:"organize_file_structure,omitempty"`
	CoverAspectRatio         *string  `json:"cover_aspect_ratio,omitempty" validate:"omitempty,oneof=book audiobook book_fallback_audiobook audiobook_fallback_book" tstype:"CoverAspectRatio"`
	DownloadFormatPreference *string  `json:"download_format_preference,omitempty" validate:"omitempty,oneof=original kepub ask" tstype:"DownloadFormat"`
	LibraryPaths             []string `json:"library_paths,omitempty" validate:"omitempty,min=1,max=50,dive"`
}
