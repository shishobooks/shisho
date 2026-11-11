package reading

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// Late data is placed by its own time (ADR 0016): a Reading Session belongs
// to the Read-through underway when it happened, whatever is underway when it
// arrives.

// readThroughAt is userID's Read-through of bookID that was underway at
// instant, by its days in the user's time zone, or nil when none was. A
// Read-through with no begin day ran from before anything was recorded. One
// that ended with no end day ended at some unknown time, so it covers no day
// after its begin day for certain and takes nothing. The latest wins a day
// two of them share, as when a reread began the day the read before it
// ended.
func readThroughAt(ctx context.Context, db bun.IDB, userID, bookID int, instant time.Time) (*models.ReadThrough, error) {
	day, err := userDay(ctx, db, userID, instant)
	if err != nil {
		return nil, err
	}
	rt := &models.ReadThrough{}
	err = db.NewSelect().
		Model(rt).
		Where("rt.user_id = ? AND rt.book_id = ?", userID, bookID).
		Where("rt.begin_day IS NULL OR rt.begin_day <= ?", day).
		Where("rt.outcome = ? OR rt.end_day >= ?", models.ReadThroughOutcomeUnderway, day).
		OrderExpr("rt.id DESC").
		Limit(1).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return rt, nil
}

// coversDay reports whether rt was underway on day, by readThroughAt's rule.
func coversDay(rt *models.ReadThrough, day string) bool {
	return (rt.BeginDay == nil || *rt.BeginDay <= day) &&
		(rt.Outcome == models.ReadThroughOutcomeUnderway || rt.EndDay != nil && *rt.EndDay >= day)
}
