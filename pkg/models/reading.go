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
