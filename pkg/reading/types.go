package reading

import "github.com/shishobooks/shisho/pkg/models"

// BookReadingResponse is the signed-in user's reading record for one Book:
// the derived Reading Status, every Read-through, the Rating, and whether
// the Book is on To Read. Read-throughs come newest first: the one underway,
// then ended ones by end day, undated ones last.
type BookReadingResponse struct {
	BookID       int                   `json:"book_id"`
	Status       *string               `json:"status" tstype:"ReadingStatus | null"`
	Rating       *float64              `json:"rating" tstype:"number | null"` // Stars, in quarter steps from 0.25 to 5
	OnToRead     bool                  `json:"on_to_read"`
	ReadThroughs []*models.ReadThrough `json:"read_throughs" tstype:"ReadThrough[]"`
}

// DayPayload carries the user's "today" for a transition that dates
// a Read-through. It is the browser's local date, since a day is in the
// user's time zone.
type DayPayload struct {
	Today string `json:"today" validate:"required,date"`
}

// UpdateReadThroughPayload replaces both days of a Read-through. Null
// clears a day. A Read-through still underway has no end day.
type UpdateReadThroughPayload struct {
	BeginDay *string `json:"begin_day" validate:"omitempty,date" tstype:"string | null"`
	EndDay   *string `json:"end_day" validate:"omitempty,date" tstype:"string | null"`
}

// SetRatingPayload sets the Rating in stars, in quarter steps from 0.25 to
// 5. Null clears it.
type SetRatingPayload struct {
	Rating *float64 `json:"rating" validate:"omitempty,min=0.25,max=5" tstype:"number | null"`
}

// ReorderToReadPayload puts the named Books, all on To Read, in this order.
// Books on To Read it leaves out keep their places, so a page showing only
// the Books in libraries the user can reach can reorder those.
type ReorderToReadPayload struct {
	BookIDs []int `json:"book_ids" validate:"required,min=1,max=500,dive,min=1"`
}
