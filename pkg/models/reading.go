package models

import (
	"time"

	"github.com/uptrace/bun"
)

// Reading Statuses. A status is derived from a user's Read-throughs and To
// Read membership on every read and never stored (ADR 0013); see
// reading.StatusExpr for the order.
const (
	//tygo:emit export type ReadingStatus = typeof ReadingStatusCurrentlyReading | typeof ReadingStatusPaused | typeof ReadingStatusToRead | typeof ReadingStatusRead | typeof ReadingStatusDidNotFinish;
	ReadingStatusCurrentlyReading = "currently_reading"
	ReadingStatusPaused           = "paused"
	ReadingStatusToRead           = "to_read"
	ReadingStatusRead             = "read"
	ReadingStatusDidNotFinish     = "did_not_finish"
)

// ReadingStatusFilterNone filters book listings to Books with no Reading
// Status. It's a filter value only; no Book has it as its status.
const (
	//tygo:emit export type ReadingStatusFilter = ReadingStatus | typeof ReadingStatusFilterNone;
	ReadingStatusFilterNone = "none"
)

// Read-through outcomes.
const (
	//tygo:emit export type ReadThroughOutcome = typeof ReadThroughOutcomeUnderway | typeof ReadThroughOutcomeFinished | typeof ReadThroughOutcomeAbandoned;
	ReadThroughOutcomeUnderway  = "underway"
	ReadThroughOutcomeFinished  = "finished"
	ReadThroughOutcomeAbandoned = "abandoned"
)

// ReadThrough is one pass a user makes through a Book. See "Read-through" in
// CONTEXT.md. BeginDay and EndDay are calendar days (YYYY-MM-DD), each
// optional. Paused means something only while the outcome is underway.
type ReadThrough struct {
	bun.BaseModel `bun:"table:read_throughs,alias:rt" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UserID    int       `bun:",nullzero" json:"-"`
	BookID    int       `bun:",nullzero" json:"book_id"`
	BeginDay  *string   `json:"begin_day"`
	EndDay    *string   `json:"end_day"`
	Outcome   string    `bun:",nullzero" json:"outcome" tstype:"ReadThroughOutcome"`
	Paused    bool      `bun:",notnull" json:"paused"`
}

// Page flows: how the Page-based reader lays out a Book's pages, one at a
// time or as one continuous scroll.
const (
	//tygo:emit export type PageFlow = typeof PageFlowPaginated | typeof PageFlowScrolled;
	PageFlowPaginated = "paginated"
	PageFlowScrolled  = "scrolled"
)

// UserBook is one user's record for one Book of what doesn't belong to a
// Read-through: the Rating, in quarter stars from 1 to 20, the position on
// To Read, and the Page flow the user chose for reading it. Nil means
// unrated, not on To Read, and no flow chosen.
type UserBook struct {
	bun.BaseModel `bun:"table:user_books,alias:ub" tstype:"-"`

	UserID         int       `bun:",pk" json:"-"`
	BookID         int       `bun:",pk" json:"book_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Rating         *int      `json:"rating_units"`
	ToReadPosition *int      `json:"to_read_position"`
	PageFlow       *string   `json:"page_flow" tstype:"PageFlow | null"`
}

// Reading sources: what reported a Reading Position update or a Reading
// Session.
const (
	//tygo:emit export type ReadingSource = typeof ReadingSourceWebReader;
	ReadingSourceWebReader = "web_reader"
)

// ReadingLocator is where in a File a reader is, in exactly one form: an
// EPUB CFI from the web ebook reader, a KOReader XPointer, a 0-based page
// for comics and PDFs, seconds into an audiobook, or a Kobo span. Each
// source writes the form it reads.
type ReadingLocator struct {
	CFI      *string  `bun:"cfi" json:"cfi"`
	XPointer *string  `bun:"xpointer" json:"xpointer"`
	Page     *int     `json:"page"`
	Seconds  *float64 `json:"seconds"`
	KoboSpan *string  `json:"kobo_span"`
}

// Equal reports whether l and o are the same position in the same form.
func (l ReadingLocator) Equal(o ReadingLocator) bool {
	return equalPtr(l.CFI, o.CFI) && equalPtr(l.XPointer, o.XPointer) && equalPtr(l.Page, o.Page) &&
		equalPtr(l.Seconds, o.Seconds) && equalPtr(l.KoboSpan, o.KoboSpan)
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// ReadingPosition is one user's place in one File: the latest update by
// the instant it happened (OccurredAt), whichever source sent it.
// Percentage is how far through the File that is, from 0 to 1. See
// "Reading Position" in CONTEXT.md.
type ReadingPosition struct {
	bun.BaseModel `bun:"table:reading_positions,alias:rp" tstype:"-"`

	UserID         int       `bun:",pk" json:"-"`
	FileID         int       `bun:",pk" json:"file_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ReadingLocator `tstype:",extends"`
	Percentage     float64   `bun:",notnull" json:"percentage"`
	Source         string    `bun:",nullzero" json:"source" tstype:"ReadingSource"`
	OccurredAt     time.Time `json:"occurred_at"`
}

// ReadingPositionUpdate is one accepted update in the append-only log. It
// belongs to the Read-through underway when it arrived, or the one it
// finished (AutoFinished), and outlives its File with FileID cleared.
type ReadingPositionUpdate struct {
	bun.BaseModel `bun:"table:reading_position_updates,alias:rpu" tstype:"-"`

	ID             int       `bun:",pk,nullzero" json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UserID         int       `bun:",nullzero" json:"-"`
	FileID         *int      `json:"file_id"`
	ReadThroughID  *int      `json:"read_through_id"`
	ReadingLocator `tstype:",extends"`
	Percentage     float64   `bun:",notnull" json:"percentage"`
	Source         string    `bun:",nullzero" json:"source" tstype:"ReadingSource"`
	OccurredAt     time.Time `json:"occurred_at"`
	AtEnd          bool      `bun:",notnull" json:"at_end"`
	AutoFinished   bool      `bun:",notnull" json:"auto_finished"`
}

// ReadingSession is one stretch of reading or listening to a File. See
// "Reading Session" in CONTEXT.md. Measured is false for a session a source
// estimated rather than timed.
type ReadingSession struct {
	bun.BaseModel `bun:"table:reading_sessions,alias:rs" tstype:"-"`

	ID              int       `bun:",pk,nullzero" json:"id"`
	CreatedAt       time.Time `json:"created_at"`
	UserID          int       `bun:",nullzero" json:"-"`
	FileID          *int      `json:"file_id"`
	ReadThroughID   *int      `json:"read_through_id"`
	Source          string    `bun:",nullzero" json:"source" tstype:"ReadingSource"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	StartPercentage float64   `bun:",notnull" json:"start_percentage"`
	EndPercentage   float64   `bun:",notnull" json:"end_percentage"`
	Measured        bool      `bun:",notnull" json:"measured"`
}
