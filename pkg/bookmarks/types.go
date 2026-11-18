package bookmarks

// CreateBookmarkPayload adds a Bookmark at a moment in an audiobook File.
type CreateBookmarkPayload struct {
	// Seconds is the moment, in seconds from the start of the File.
	Seconds *float64 `json:"seconds" validate:"required,gte=0" tstype:"number,required"`
	// Note is optional; a blank one is no note.
	Note *string `json:"note,omitempty" mod:"trim" validate:"omitempty,max=2000" tstype:"string | null"`
}

// UpdateBookmarkPayload replaces a Bookmark's note. The note is required, so
// a request that leaves it out can't clear it by accident; a blank one
// clears it.
type UpdateBookmarkPayload struct {
	Note *string `json:"note" mod:"trim" validate:"required,max=2000" tstype:"string,required"`
}
