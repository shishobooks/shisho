package libraries

type CreateLibraryPayload struct {
	Name                  string   `json:"name" validate:"required,max=100"`
	OrganizeFileStructure *bool    `json:"organize_file_structure,omitempty"`
	LibraryPaths          []string `json:"library_paths" validate:"required,min=1,max=50,dive"`
}

type ListLibrariesQuery struct {
	Limit   int  `query:"limit" json:"limit,omitempty" default:"10" validate:"min=1,max=100"`
	Offset  int  `query:"offset" json:"offset,omitempty" validate:"min=0"`
	Deleted bool `query:"deleted" json:"deleted,omitempty"`
}

type UpdateLibraryPayload struct {
	Name                  *string  `json:"name,omitempty" validate:"omitempty,max=100"`
	OrganizeFileStructure *bool    `json:"organize_file_structure,omitempty"`
	LibraryPaths          []string `json:"library_paths,omitempty" validate:"omitempty,min=1,max=50,dive"`
	Deleted               *bool    `json:"deleted,omitempty" validate:"omitempty"`
}
