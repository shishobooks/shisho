package shishoapp

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/changefeed"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/uptrace/bun"
)

const refusedBook = "The Book doesn't exist, or you can no longer reach it."

// applyPageFlow saves the Page flow the app chose for a Book at the time it
// was chosen. A choice made later elsewhere, as on the web while the phone
// was offline, keeps, and the Book's record moves so the app shows it
// again in place of its own.
func (h *handler) applyPageFlow(ctx context.Context, user *models.User, clock deviceClock, at time.Time,
	p JournalPageFlowPayload, claim claimFunc) (eventOutcome, error) {
	libraryID, err := h.reading.BookLibraryID(ctx, p.BookID)
	if errors.Is(err, sql.ErrNoRows) {
		return eventOutcome{refused: refusedBook}, nil
	}
	if err != nil {
		return eventOutcome{}, err
	}
	if !user.HasLibraryAccess(libraryID) {
		return eventOutcome{refused: refusedBook}, nil
	}
	chosen, ok := clock.instant(at)
	if !ok {
		return eventOutcome{refused: refusedBeforeAccount}, nil
	}
	err = h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// The claim writes first, so the transaction holds the write lock
		// before it reads.
		if err := claim(ctx, tx); err != nil {
			return err
		}
		saved, err := reading.SetPageFlowAt(ctx, tx, user.ID, p.BookID, p.PageFlow, chosen)
		if err != nil || saved {
			return err
		}
		return h.feed.MoveOwn(ctx, tx, changefeed.KindUserBook, p.BookID, user.ID)
	})
	return eventOutcome{}, err
}
