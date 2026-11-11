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
	// Progress is how far through the Book the user is, from 0 to 1: the
	// furthest of its main Files' positions, while a Read-through is underway.
	Progress *float64 `json:"progress" tstype:"number | null"`
	// Positions is the Reading Position of each of the Book's Files that has
	// one, whatever its status. Progress is what says the Book is in progress.
	Positions []*models.ReadingPosition `json:"positions" tstype:"ReadingPosition[]"`
	// LastReadAt is the latest reading in the Read-through underway, on any of
	// the Book's main Files. Null with nothing underway or no reading yet.
	LastReadAt *time.Time `json:"last_read_at" tstype:"string | null"`
}

// ReadThroughChangeResponse answers a change to one of the user's
// Read-throughs: the Read-through as the change left it (as it was, for a
// delete), and its Book's reading record, which is null when the Book was
// deleted or is in a library the user can no longer reach.
type ReadThroughChangeResponse struct {
	ReadThrough *models.ReadThrough  `json:"read_through" tstype:"ReadThrough,required"`
	Reading     *BookReadingResponse `json:"reading" tstype:"BookReadingResponse | null,required"`
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
// File. It carries the position in the one form the reader keeps (a CFI for
// the ebook reader, a 0-based page for comics and PDFs, seconds for
// audiobooks), and optionally a Reading Session the reader just finished. A
// reader that hasn't moved since opening sends only the session, which
// changes no position or status.
type UpdatePositionPayload struct {
	CFI     *string  `json:"cfi,omitempty" validate:"omitempty,min=1,max=4096"`
	Page    *int     `json:"page,omitempty" validate:"omitempty,min=0"`
	Seconds *float64 `json:"seconds,omitempty" validate:"omitempty,min=0"`
	// Percentage is required with a position.
	Percentage *float64 `json:"percentage,omitempty" validate:"omitempty,min=0,max=1"`
	// AtEnd is the reader at the end of the File, or playback ending, which
	// finishes the Read-through underway.
	AtEnd bool `json:"at_end,omitempty"`
	// ListenedSeconds is how long the player has played in this sitting,
	// including any session sent with the update. A minute of it starts or
	// resumes a Read-through.
	ListenedSeconds *float64 `json:"listened_seconds,omitempty" validate:"omitempty,min=0"`
	// ListeningSince is when this sitting began: a Read-through it starts
	// begins that day and takes what it already recorded. Out of range, more
	// than a day ago or in the future, it's ignored.
	ListeningSince *time.Time `json:"listening_since,omitempty"`
	// Opened is the reader opened at this place by a link, which saves it as
	// the place without counting as reading.
	Opened bool `json:"opened,omitempty"`
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

// ListReadingHistoryQuery pages through the signed-in user's Read-throughs
// for all time. Outcome keeps only Read-throughs with that outcome, and
// Removed only those whose Book was deleted.
type ListReadingHistoryQuery struct {
	Limit   int     `query:"limit" json:"limit,omitempty" default:"50" validate:"min=1,max=100"`
	Offset  int     `query:"offset" json:"offset,omitempty" validate:"min=0"`
	Outcome *string `query:"outcome" json:"outcome,omitempty" validate:"omitempty,oneof=underway finished abandoned" tstype:"ReadThroughOutcome"`
	Removed bool    `query:"removed" json:"removed,omitempty"`
}

// ListReadingHistoryResponse is a page of Reading History, newest first.
type ListReadingHistoryResponse struct {
	Items []*ReadingHistoryEntry `json:"items" tstype:"ReadingHistoryEntry[]"`
	Total int                    `json:"total"`
}

// ReadingHistoryEntry is one Read-through on Reading History, with what it
// takes to show it.
//
//nolint:revive // named for the Reading History page, as generated TS types are global
type ReadingHistoryEntry struct {
	ReadThrough *models.ReadThrough `json:"read_through" tstype:"ReadThrough,required"`
	// Title and Authors are the Book's, or the copy kept when it was deleted.
	Title   string   `json:"title"`
	Authors []string `json:"authors"`
	// Removed is true when the Book was deleted.
	Removed bool `json:"removed"`
	// Book is the Book when the user can still reach it: null when it was
	// deleted or sits in a library they no longer have access to.
	Book *ReadingHistoryBook `json:"book" tstype:"ReadingHistoryBook | null,required"`
	// CanReread is whether "Start a reread" applies: the user can reach the
	// Book and has no Read-through of it underway.
	CanReread bool `json:"can_reread"`
}

// ReadingHistoryBook is what a Reading History row needs to link to its
// Book and show its cover.
//
//nolint:revive // named for the Reading History page, as generated TS types are global
type ReadingHistoryBook struct {
	ID            int    `json:"id"`
	LibraryID     int    `json:"library_id"`
	CoverCacheKey string `json:"cover_cache_key"`
}
