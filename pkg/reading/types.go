package reading

import (
	"time"

	"github.com/shishobooks/shisho/pkg/models"
)

// BookReadingResponse is the signed-in user's reading record for one Book:
// the derived Reading Status, every Read-through, the Rating, and whether
// and where the Book is on To Read. Read-throughs come newest first: the one underway,
// then ended ones by end day, undated ones last.
type BookReadingResponse struct {
	BookID       int                   `json:"book_id"`
	Status       *string               `json:"status" tstype:"ReadingStatus | null"`
	Rating       *float64              `json:"rating" tstype:"number | null"` // Stars, in quarter steps from 0.25 to 5
	OnToRead     bool                  `json:"on_to_read"`
	ToReadPlace  *int                  `json:"to_read_place" tstype:"number | null"` // 1-based place on To Read as the To Read page shows it, among Books in libraries the user can reach
	ReadThroughs []*models.ReadThrough `json:"read_throughs" tstype:"ReadThrough[]"`
	PageFlow     *string               `json:"page_flow" tstype:"PageFlow | null"` // The flow chosen for the Page-based reader; null lets the reader pick
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

// SetPageFlowPayload saves the flow the Page-based reader uses for the
// Book. Null clears it, so the reader picks one from the page shapes again.
type SetPageFlowPayload struct {
	PageFlow *string `json:"page_flow" validate:"omitempty,oneof=paginated scrolled" tstype:"PageFlow | null"`
}

// UpdatePositionPayload is a web reader reporting where the user is in a
// File. It carries the position in the form the reader keeps (a CFI for
// the ebook reader), and optionally a Reading Session the reader just
// finished. A reader that hasn't moved since opening sends only the
// session, which changes no position or status.
type UpdatePositionPayload struct {
	CFI *string `json:"cfi,omitempty" validate:"omitempty,min=1,max=4096"`
	// Percentage is required with a position.
	Percentage *float64 `json:"percentage,omitempty" validate:"omitempty,min=0,max=1"`
	// AtEnd is the reader at the end of the File, which finishes the
	// Read-through underway.
	AtEnd bool `json:"at_end,omitempty"`
	// OccurredAt is when the reader was at the position, so an update that
	// arrives late can't replace a newer one. Omitted is now.
	OccurredAt *time.Time      `json:"occurred_at,omitempty"`
	Session    *SessionPayload `json:"session,omitempty"`
}

// SessionPayload is a finished Reading Session the web reader timed.
type SessionPayload struct {
	StartedAt       time.Time `json:"started_at" validate:"required"`
	EndedAt         time.Time `json:"ended_at" validate:"required"`
	StartPercentage *float64  `json:"start_percentage" validate:"required,min=0,max=1" tstype:",required"`
	EndPercentage   *float64  `json:"end_percentage" validate:"required,min=0,max=1" tstype:",required"`
}

// UpdatePositionResponse says what a position update did, so the reader
// can show the finish toast. The update service returns it for every
// source.
type UpdatePositionResponse struct {
	BookID int `json:"book_id"`
	// Accepted is false for an update older than the saved position, or
	// one that carried only a session: neither changed the position.
	Accepted bool `json:"accepted"`
	// Started is true when the update started a Read-through.
	Started bool `json:"started"`
	// Resumed is true when the update resumed a paused Read-through.
	Resumed bool `json:"resumed"`
	// Reverted is true when the update undid an automatic finish.
	Reverted bool `json:"reverted"`
	// FinishedReadThroughID is the Read-through the update finished, which
	// the toast's Undo names.
	FinishedReadThroughID *int `json:"finished_read_through_id" tstype:"number | null,required"`
}

// FilePositionResponse is the signed-in user's Reading Position in a File,
// or null when they have none. The wrapper keeps "no position yet" a 200
// the reader can open at the start from, rather than an error.
type FilePositionResponse struct {
	Position *models.ReadingPosition `json:"position" tstype:"ReadingPosition | null,required"`
}
