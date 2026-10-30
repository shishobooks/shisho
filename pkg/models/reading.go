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

// UserBook is one user's record for one Book of what doesn't belong to a
// Read-through: the Rating, in quarter stars from 1 to 20, and the position
// on To Read. Nil means unrated and not on To Read.
type UserBook struct {
	bun.BaseModel `bun:"table:user_books,alias:ub" tstype:"-"`

	UserID         int       `bun:",pk" json:"-"`
	BookID         int       `bun:",pk" json:"book_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Rating         *int      `json:"rating_units"`
	ToReadPosition *int      `json:"to_read_position"`
}
