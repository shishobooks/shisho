package books

import "github.com/shishobooks/shisho/pkg/models"

type ListBooksQuery struct {
	Limit          int      `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset         int      `query:"offset" json:"offset,omitempty" validate:"min=0"`
	LibraryID      *int     `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	SeriesID       *int     `query:"series_id" json:"series_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	Search         *string  `query:"search" json:"search,omitempty" validate:"omitempty,max=100" tstype:"string"`
	FileTypes      []string `query:"file_types" json:"file_types,omitempty"`                                         // Filter by file types (e.g., ["epub", "m4b"])
	GenreIDs       []int    `query:"genre_ids" json:"genre_ids,omitempty"`                                           // Filter by genre IDs
	TagIDs         []int    `query:"tag_ids" json:"tag_ids,omitempty"`                                               // Filter by tag IDs
	Language       *string  `query:"language" json:"language,omitempty" validate:"omitempty,max=35" tstype:"string"` // Filter by language tag
	IDs            []int    `query:"ids" json:"ids,omitempty"`                                                       // Filter by specific book IDs
	Sort           string   `query:"sort" json:"sort,omitempty" validate:"omitempty,max=200"`
	ReviewedFilter string   `query:"reviewed_filter" json:"reviewed_filter,omitempty" validate:"omitempty,oneof=all needs_review reviewed"` // "" or "all" = all books, "needs_review", "reviewed"
}

type UpdateBookPayload struct {
	Title       *string       `json:"title,omitempty" mod:"trim" validate:"omitempty,min=1,max=300"`
	SortTitle   *string       `json:"sort_title,omitempty" validate:"omitempty,max=300"`
	Subtitle    *string       `json:"subtitle,omitempty" validate:"omitempty,max=500"`
	Description *string       `json:"description,omitempty" validate:"omitempty,max=10000"`
	Authors     []AuthorInput `json:"authors,omitempty"`
	Series      []SeriesInput `json:"series,omitempty"`
	Genres      []string      `json:"genres,omitempty" validate:"omitempty,dive,max=100"` // Genre names
	Tags        []string      `json:"tags,omitempty" validate:"omitempty,dive,max=100"`   // Tag names
}

// AuthorInput represents an author with an optional role (for CBZ files).
type AuthorInput struct {
	Name string  `json:"name" validate:"required,max=200"`
	Role *string `json:"role,omitempty" validate:"omitempty,oneof=writer penciller inker colorist letterer cover_artist editor translator"`
}

// SeriesInput represents a series association with optional number.
type SeriesInput struct {
	Name   string   `json:"name" validate:"required,max=200"`
	Number *float64 `json:"number,omitempty"`
}

// IdentifierPayload represents an identifier in update requests.
type IdentifierPayload struct {
	Type  string `json:"type" mod:"trim" validate:"required,min=1,max=50"`
	Value string `json:"value" mod:"trim" validate:"required,max=100"`
}

// UpdateFilePayload is the payload for updating a file's metadata.
type UpdateFilePayload struct {
	FileRole    *string              `json:"file_role,omitempty" validate:"omitempty,oneof=main supplement"`
	Name        *string              `json:"name,omitempty" mod:"trim" validate:"omitempty,max=500"`
	Narrators   []string             `json:"narrators,omitempty" validate:"omitempty,dive,max=200"`
	URL         *string              `json:"url,omitempty" validate:"omitempty,max=500,url"`
	Publisher   *string              `json:"publisher,omitempty" validate:"omitempty,max=200"`
	Imprint     *string              `json:"imprint,omitempty" validate:"omitempty,max=200"`
	ReleaseDate *string              `json:"release_date,omitempty" validate:"omitempty"` // ISO 8601 date string
	Language    *string              `json:"language,omitempty" validate:"omitempty,max=35"`
	Abridged    *string              `json:"abridged,omitempty" validate:"omitempty,oneof=true false"` // "true", "false", or "" to clear
	Identifiers *[]IdentifierPayload `json:"identifiers,omitempty" mod:"dive" validate:"omitempty,dive"`
}

// ResyncPayload contains the request parameters for resync operations.
type ResyncPayload struct {
	Mode    string `json:"mode"`
	Refresh bool   `json:"refresh"` // Deprecated: kept for backwards compatibility
}

// resolveScanMode converts a ResyncPayload into ForceRefresh, SkipPlugins, and Reset flags.
// Supports three modes: "scan" (default), "refresh", and "reset".
// Falls back to the deprecated Refresh boolean if Mode is empty.
func (p ResyncPayload) resolveScanMode() (forceRefresh, skipPlugins, reset bool) {
	switch p.Mode {
	case "refresh":
		return true, false, false
	case "reset":
		return true, true, true
	case "scan", "":
		// For empty mode, check deprecated Refresh field for backwards compatibility
		return p.Refresh, false, false
	default:
		return false, false, false
	}
}

// MoveFilesPayload is the payload for moving files to another book.
type MoveFilesPayload struct {
	FileIDs      []int `json:"file_ids" validate:"required,min=1,dive,min=1"`
	TargetBookID *int  `json:"target_book_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
}

// MoveFilesResponse is the response from a move files operation.
type MoveFilesResponse struct {
	TargetBook        *models.Book `json:"target_book"`
	FilesMoved        int          `json:"files_moved"`
	SourceBookDeleted bool         `json:"source_book_deleted"`
}

// MergeBooksPayload is the payload for merging multiple books.
type MergeBooksPayload struct {
	SourceBookIDs []int `json:"source_book_ids" validate:"required,min=1,dive,min=1"`
	TargetBookID  int   `json:"target_book_id" validate:"required,min=1"`
}

// MergeBooksResponse is the response from a merge books operation.
type MergeBooksResponse struct {
	TargetBook   *models.Book `json:"target_book"`
	FilesMoved   int          `json:"files_moved"`
	BooksDeleted int          `json:"books_deleted"`
}
