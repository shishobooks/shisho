package reading

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// StatusSummary is one user's reading of one Book as a device that shows
// only a status needs it: the Reading Status, when it last changed, how many
// times the Book was read, and when the latest read began.
type StatusSummary struct {
	BookID int     `bun:"book_id"`
	Status *string `bun:"status"`
	// ReadThroughAt and UserBookAt are the latest changes to the Book's
	// Read-throughs and its To Read place; ChangedAt is the later of them.
	ReadThroughAt *time.Time `bun:"read_through_at"`
	UserBookAt    *time.Time `bun:"user_book_at"`
	ChangedAt     *time.Time `bun:"-"`
	ReadThroughs  int        `bun:"read_throughs"`
	// LastBeginDay is when the Read-through the status comes from began, or
	// the latest one when none does, if it says, and LastBeganAt the start of
	// that day in the user's time zone.
	LastBeginDay *string    `bun:"last_begin_day"`
	LastBeganAt  *time.Time `bun:"-"`
	// RatingUnits is the Rating in quarter-star units, nil when unrated, and
	// RatingAt when it was last set or cleared.
	RatingUnits *int       `bun:"rating"`
	RatingAt    *time.Time `bun:"rating_at"`
}

// RetrieveStatusSummaries returns userID's StatusSummary of each of bookIDs,
// by Book id.
func (svc *Service) RetrieveStatusSummaries(ctx context.Context, userID int, bookIDs []int) (map[int]*StatusSummary, error) {
	summaries := make(map[int]*StatusSummary, len(bookIDs))
	if len(bookIDs) == 0 {
		return summaries, nil
	}
	var rows []*StatusSummary
	err := svc.db.NewSelect().
		TableExpr("books AS b").
		ColumnExpr("b.id AS book_id").
		ColumnExpr(StatusExpr(userID, "b.id")+" AS status").
		ColumnExpr("(SELECT MAX(srt.updated_at) FROM read_throughs srt WHERE srt.user_id = ? AND srt.book_id = b.id) AS read_through_at", userID).
		ColumnExpr("(SELECT sub.updated_at FROM user_books sub WHERE sub.user_id = ? AND sub.book_id = b.id) AS user_book_at", userID).
		ColumnExpr("(SELECT COUNT(*) FROM read_throughs srt WHERE srt.user_id = ? AND srt.book_id = b.id) AS read_throughs", userID).
		ColumnExpr("(SELECT sub.rating FROM user_books sub WHERE sub.user_id = ? AND sub.book_id = b.id) AS rating", userID).
		ColumnExpr("(SELECT sub.rating_updated_at FROM user_books sub WHERE sub.user_id = ? AND sub.book_id = b.id) AS rating_at", userID).
		// The Read-throughs in the order StatusExpr picks the one a status
		// comes from: the one underway, then the latest ended.
		ColumnExpr(`(SELECT srt.begin_day FROM read_throughs srt WHERE srt.user_id = ? AND srt.book_id = b.id
			ORDER BY srt.outcome = ? DESC, srt.end_day IS NULL, srt.end_day DESC, srt.id DESC LIMIT 1) AS last_begin_day`,
			userID, models.ReadThroughOutcomeUnderway).
		Where("b.id IN (?)", bun.List(bookIDs)).
		Scan(ctx, &rows)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	location, err := userLocation(ctx, svc.db, userID)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.LastBeginDay != nil {
			if began, err := time.ParseInLocation(time.DateOnly, *row.LastBeginDay, location); err == nil {
				row.LastBeganAt = &began
			}
		}
		row.ChangedAt = row.ReadThroughAt
		if row.UserBookAt != nil && (row.ChangedAt == nil || row.UserBookAt.After(*row.ChangedAt)) {
			row.ChangedAt = row.UserBookAt
		}
		summaries[row.BookID] = row
	}
	return summaries, nil
}

// RetrievePositions returns userID's Reading Positions in fileIDs, by File
// id, in the form and copy their sources wrote, as RetrievePosition does.
func (svc *Service) RetrievePositions(ctx context.Context, userID int, fileIDs []int) (map[int]*models.ReadingPosition, error) {
	positions := make(map[int]*models.ReadingPosition, len(fileIDs))
	if len(fileIDs) == 0 {
		return positions, nil
	}
	var rows []*models.ReadingPosition
	err := svc.db.NewSelect().
		Model(&rows).
		Where("rp.user_id = ? AND rp.file_id IN (?)", userID, bun.List(fileIDs)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, p := range rows {
		positions[p.FileID] = p
	}
	return positions, nil
}
