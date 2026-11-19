package filesystem

// BrowseQuery contains query parameters for the browse endpoint.
type BrowseQuery struct {
	Path       string `query:"path" json:"path,omitempty" default:"/"`
	ShowHidden bool   `query:"show_hidden" json:"show_hidden,omitempty"`
	Limit      int    `query:"limit" json:"limit,omitempty" default:"50" validate:"min=1,max=100"`
	Offset     int    `query:"offset" json:"offset,omitempty" validate:"min=0"`
	Search     string `query:"search" json:"search,omitempty"`
}

// Entry represents a filesystem entry (file or directory).
type Entry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// BrowseResponse contains the response for the browse endpoint.
type BrowseResponse struct {
	CurrentPath string  `json:"current_path"`
	ParentPath  string  `json:"parent_path,omitempty"`
	Entries     []Entry `json:"entries"`
	Total       int     `json:"total"`
	HasMore     bool    `json:"has_more"`
}

// DetectFileTypesPayload is the body of POST /filesystem/detect: the folders
// a new library will scan.
type DetectFileTypesPayload struct {
	Paths []string `json:"paths" validate:"required,min=1,max=100,dive,required" mod:"dive,trim"`
	// FoldersOnly checks that the server can open each path and counts
	// nothing, which answers at once however large the folders are.
	FoldersOnly bool `json:"folders_only,omitempty"`
}

// DetectFileTypesResponse counts the recognized files found in the paths by
// category. StoppedEarly says the walk ran out of time or reached its file
// limit before it looked at everything, so the counts cover part of the
// folders. MissingPaths lists, as given, each path that isn't a folder the
// server can open and read, which a library can't scan.
type DetectFileTypesResponse struct {
	Ebooks       int      `json:"ebooks"`
	Audiobooks   int      `json:"audiobooks"`
	Comics       int      `json:"comics"`
	StoppedEarly bool     `json:"stopped_early"`
	MissingPaths []string `json:"missing_paths,omitempty"`
}
