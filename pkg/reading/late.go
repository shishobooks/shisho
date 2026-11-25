package reading

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// The Shisho app's reading changes arrive through its journal, each placed
// at the instant it happened on the phone (ADR 0024), which can be long
// before it arrives. Each runs as the web's control does, dated at its
// instant rather than now, after being ordered against what changed since:
//
//   - Status choices (start, Mark as read, Mark as Did Not Finish, Mark or
//     Remove Reference, Add to To Read) are ordered against the user's
//     latest explicit status choice for the Book (user_books
//     status_chosen_at, ADR 0028). One dated before it doesn't apply, and
//     neither do a pause or resume. A finish or abandon dated before a
//     later Reference choice is still recorded, as history only.
//   - A Rating dated before the last change of the Rating doesn't apply.
//   - To Read changes are ordered against the Book's last To Read change
//     (to_read_changed_at): the latest change of each Book wins.
//
// An event dated exactly at the instant it's ordered against applies: the
// later arrival wins a tie, as events of one batch arrive in the order they
// were made.

// LateEvent is one of the app's reading changes to a Book, made at At.
type LateEvent struct {
	UserID int
	BookID int
	At     time.Time
	// DeviceDay is the phone's own day at At, which dates the change when
	// the user has set no time zone.
	DeviceDay string
	// Claim runs first in the change's transaction, so the journal records
	// the event exactly when it's applied.
	Claim func(ctx context.Context, tx bun.Tx) error
}

// ErrLateNothingUnderway refuses a Mark as Did Not Finish with nothing
// underway to end.
var ErrLateNothingUnderway = errors.New("nothing was underway to mark Did Not Finish")

// runLate runs change in a transaction that runs claim first. The change
// stamps the user's record of bookID with when it happened, for ordering,
// so its updated_at, which device sync finds changes by, is then moved to
// now (0 has no record).
func (svc *Service) runLate(ctx context.Context, userID, bookID int, claim func(ctx context.Context, tx bun.Tx) error,
	change func(ctx context.Context, tx bun.Tx) error) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		if claim != nil {
			if err := claim(ctx, tx); err != nil {
				return err
			}
		}
		if err := change(ctx, tx); err != nil || bookID == 0 {
			return err
		}
		_, err := tx.NewUpdate().Model((*models.UserBook)(nil)).Set("updated_at = ?", svc.now()).
			Where("user_id = ? AND book_id = ?", userID, bookID).Exec(ctx)
		return errors.WithStack(err)
	})
}

// userBookAt loads userID's record of bookID, or an empty one.
func userBookAt(ctx context.Context, db bun.IDB, userID, bookID int) (*models.UserBook, error) {
	ub := &models.UserBook{}
	err := db.NewSelect().Model(ub).Where("ub.user_id = ? AND ub.book_id = ?", userID, bookID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return &models.UserBook{UserID: userID, BookID: bookID}, nil
	}
	return ub, errors.WithStack(err)
}

// after reports whether stamp is later than at.
func after(stamp *time.Time, at time.Time) bool {
	return stamp != nil && stamp.After(at)
}

// lateDay is the day e happened: in the user's time zone when they set one,
// as the web dates its changes, or else the phone's own day.
func lateDay(ctx context.Context, db bun.IDB, e LateEvent) (string, error) {
	var zone *string
	err := db.NewSelect().Model((*models.UserSettings)(nil)).Column("us.time_zone").
		Where("us.user_id = ?", e.UserID).Scan(ctx, &zone)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", errors.WithStack(err)
	}
	if zone == nil && e.DeviceDay != "" {
		return e.DeviceDay, nil
	}
	return userDay(ctx, db, e.UserID, e.At)
}

// LateStart starts a Read-through as Start does, begun the day e happened.
// A Book already underway stays as it is.
func (svc *Service) LateStart(ctx context.Context, e LateEvent) error {
	return svc.runLate(ctx, e.UserID, e.BookID, e.Claim, func(ctx context.Context, tx bun.Tx) error {
		ub, err := userBookAt(ctx, tx, e.UserID, e.BookID)
		if err != nil || after(ub.StatusChosenAt, e.At) {
			return err
		}
		underway, err := retrieveUnderway(ctx, tx, e.UserID, e.BookID)
		if err != nil || underway != nil {
			return err
		}
		day, err := lateDay(ctx, tx, e)
		if err != nil {
			return err
		}
		if _, err := startReadThrough(ctx, tx, e.UserID, e.BookID, day, svc.now(), e.At, 0); err != nil {
			return err
		}
		return chooseStatus(ctx, tx, e.UserID, e.BookID, false, e.At)
	})
}

// LateEnd ends the Read-through underway with outcome as End does, dated
// by the day e happened. Dated before a later explicit choice, or before
// the Read-through underway began, it's recorded as an ended Read-through
// with no begin day that takes no Reading Sessions, leaving the status and
// To Read as they are (ADR 0028), so Reading History keeps the pass. A
// later choice that itself ended a Read-through on or after that day, such
// as Mark as read on the web, already says how it ended, and the late one
// adds nothing. Marking Did Not Finish with nothing underway is refused with
// ErrLateNothingUnderway.
func (svc *Service) LateEnd(ctx context.Context, e LateEvent, outcome string) error {
	return svc.runLate(ctx, e.UserID, e.BookID, e.Claim, func(ctx context.Context, tx bun.Tx) error {
		ub, err := userBookAt(ctx, tx, e.UserID, e.BookID)
		if err != nil {
			return err
		}
		day, err := lateDay(ctx, tx, e)
		if err != nil {
			return err
		}
		if after(ub.StatusChosenAt, e.At) {
			if !ub.Reference {
				// Ended since, or on a day as late: Mark as read on the web
				// dates its finish by the last reading, which can be before
				// this.
				ended, err := tx.NewSelect().Model((*models.ReadThrough)(nil)).
					Where("rt.user_id = ? AND rt.book_id = ? AND rt.outcome != ?", e.UserID, e.BookID, models.ReadThroughOutcomeUnderway).
					Where("rt.end_day >= ? OR COALESCE(rt.state_changed_at, rt.updated_at) > ?", day, e.At).
					Exists(ctx)
				if err != nil || ended {
					return errors.WithStack(err)
				}
			}
			return insertHistoryOnly(ctx, tx, e, outcome, day, svc.now())
		}
		underway, err := retrieveUnderway(ctx, tx, e.UserID, e.BookID)
		if err != nil {
			return err
		}
		if underway != nil && (underway.BeginDay != nil && day < *underway.BeginDay || underway.Began().After(e.At)) {
			// Underway since after this, as one a device started from
			// reading later, so this ended an earlier pass.
			return insertHistoryOnly(ctx, tx, e, outcome, day, svc.now())
		}
		err = end(ctx, tx, e.UserID, e.BookID, outcome, day, svc.now(), e.At)
		if errors.Is(err, ErrNothingUnderway) {
			return ErrLateNothingUnderway
		}
		return err
	})
}

// insertHistoryOnly records a Read-through of e's Book that ended with
// outcome on day, with no begin day, as history only.
func insertHistoryOnly(ctx context.Context, tx bun.Tx, e LateEvent, outcome, day string, now time.Time) error {
	bookID := e.BookID
	_, err := tx.NewInsert().Model(&models.ReadThrough{
		CreatedAt:      now,
		UpdatedAt:      now,
		UserID:         e.UserID,
		BookID:         &bookID,
		EndDay:         &day,
		Outcome:        outcome,
		HistoryOnly:    true,
		StateChangedAt: &e.At,
	}).Exec(ctx)
	return errors.WithStack(err)
}

// LateSetPaused pauses or resumes the Read-through underway, unless it
// changed later than e, or the user made a status choice since. One already
// in that state, or nothing underway, is left as it is.
func (svc *Service) LateSetPaused(ctx context.Context, e LateEvent, paused bool) error {
	return svc.runLate(ctx, e.UserID, e.BookID, e.Claim, func(ctx context.Context, tx bun.Tx) error {
		ub, err := userBookAt(ctx, tx, e.UserID, e.BookID)
		if err != nil || after(ub.StatusChosenAt, e.At) {
			return err
		}
		underway, err := retrieveUnderway(ctx, tx, e.UserID, e.BookID)
		if err != nil || underway == nil || underway.Paused == paused || underway.StateChanged().After(e.At) {
			return err
		}
		underway.Paused = paused
		underway.UpdatedAt = svc.now()
		underway.StateChangedAt = &e.At
		_, err = tx.NewUpdate().Model(underway).Column("paused", "updated_at", "state_changed_at").WherePK().Exec(ctx)
		return errors.WithStack(err)
	})
}

// LateReference marks e's Book Reference, or removes the mark, as
// MarkReference and RemoveReference do, unless the user made a status choice
// since. Marking dissolves whatever is underway: the user chose Reference,
// and the Read-through's sessions are kept.
func (svc *Service) LateReference(ctx context.Context, e LateEvent, mark bool) error {
	return svc.runLate(ctx, e.UserID, e.BookID, e.Claim, func(ctx context.Context, tx bun.Tx) error {
		ub, err := userBookAt(ctx, tx, e.UserID, e.BookID)
		if err != nil || after(ub.StatusChosenAt, e.At) {
			return err
		}
		if mark && !ub.Reference {
			return markReference(ctx, tx, e.UserID, e.BookID, e.At)
		}
		// Already as chosen, it's still the latest choice, so an older one
		// from another phone arriving after it doesn't apply.
		return chooseStatus(ctx, tx, e.UserID, e.BookID, mark, e.At)
	})
}

// LateDeleteReadThrough deletes one of the user's Read-throughs as
// DeleteReadThrough does: rtID, or with rtID nil, the one underway in
// bookID, unless that began after e, a pass the phone never saw. One gone
// already stays gone.
func (svc *Service) LateDeleteReadThrough(ctx context.Context, e LateEvent, rtID *int) error {
	return svc.runLate(ctx, e.UserID, e.BookID, e.Claim, func(ctx context.Context, tx bun.Tx) error {
		var rt *models.ReadThrough
		if rtID != nil {
			rt = &models.ReadThrough{}
			err := tx.NewSelect().Model(rt).Where("rt.id = ? AND rt.user_id = ?", *rtID, e.UserID).Scan(ctx)
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			if err != nil {
				return errors.WithStack(err)
			}
		} else {
			underway, err := retrieveUnderway(ctx, tx, e.UserID, e.BookID)
			if err != nil || underway == nil {
				return err
			}
			day, err := lateDay(ctx, tx, e)
			if err != nil {
				return err
			}
			if underway.BeginDay != nil && *underway.BeginDay > day || underway.Began().After(e.At) {
				return nil
			}
			rt = underway
		}
		return deleteReadThrough(ctx, tx, rt, e.At)
	})
}

// LateSetRating sets the Rating in quarter-star units, or clears it with
// nil, unless it changed later than e: the latest instant wins.
func (svc *Service) LateSetRating(ctx context.Context, e LateEvent, units *int) error {
	return svc.runLate(ctx, e.UserID, e.BookID, e.Claim, func(ctx context.Context, tx bun.Tx) error {
		ub, err := userBookAt(ctx, tx, e.UserID, e.BookID)
		if err != nil || after(ub.RatingUpdatedAt, e.At) {
			return err
		}
		return setRating(ctx, tx, e.UserID, e.BookID, units, e.At)
	})
}

// LateAddToRead puts e's Book at the end of To Read as AddToRead does,
// unless the user made a status choice or changed its place on To Read
// since. A Book already on To Read, or underway, stays as it is.
func (svc *Service) LateAddToRead(ctx context.Context, e LateEvent) error {
	return svc.runLate(ctx, e.UserID, e.BookID, e.Claim, func(ctx context.Context, tx bun.Tx) error {
		ub, err := userBookAt(ctx, tx, e.UserID, e.BookID)
		if err != nil || after(ub.StatusChosenAt, e.At) || after(ub.ToReadChangedAt, e.At) {
			return err
		}
		_, err = appendToRead(ctx, tx, e.UserID, []int{e.BookID}, e.At)
		return err
	})
}

// LateRemoveFromToRead takes e's Book off To Read, unless its place there
// changed since.
func (svc *Service) LateRemoveFromToRead(ctx context.Context, e LateEvent) error {
	return svc.runLate(ctx, e.UserID, e.BookID, e.Claim, func(ctx context.Context, tx bun.Tx) error {
		ub, err := userBookAt(ctx, tx, e.UserID, e.BookID)
		if err != nil || after(ub.ToReadChangedAt, e.At) {
			return err
		}
		return leaveToRead(ctx, tx, e.UserID, e.BookID, e.At)
	})
}

// LateMoveToRead moves e's Book on To Read to just after afterBookID, or to
// the top with nil, unless its place changed since: the latest move of each
// Book wins (ADR 0024). A Book no longer on To Read stays off it, and one
// whose neighbor left To Read stays where it is.
func (svc *Service) LateMoveToRead(ctx context.Context, e LateEvent, afterBookID *int) error {
	return svc.runLate(ctx, e.UserID, e.BookID, e.Claim, func(ctx context.Context, tx bun.Tx) error {
		var queued []*models.UserBook
		err := tx.NewSelect().Model(&queued).
			Where("ub.user_id = ? AND ub.to_read_position IS NOT NULL", e.UserID).
			Order("ub.to_read_position ASC", "ub.book_id ASC").
			Scan(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		order := make([]int, 0, len(queued))
		var moving *models.UserBook
		for _, ub := range queued {
			if ub.BookID == e.BookID {
				moving = ub
				continue
			}
			order = append(order, ub.BookID)
		}
		if moving == nil || after(moving.ToReadChangedAt, e.At) {
			return nil
		}
		at := 0
		if afterBookID != nil {
			at = -1
			for i, id := range order {
				if id == *afterBookID {
					at = i + 1
					break
				}
			}
			if at < 0 {
				return nil
			}
		}
		order = append(order[:at], append([]int{e.BookID}, order[at:]...)...)
		now := svc.now()
		for i, bookID := range order {
			position := i + 1
			q := tx.NewUpdate().Model((*models.UserBook)(nil)).
				Set("to_read_position = ?", position).
				Set("updated_at = ?", now).
				Where("user_id = ? AND book_id = ?", e.UserID, bookID)
			if bookID == e.BookID {
				q = q.Set("to_read_changed_at = ?", e.At)
			} else {
				q = q.Where("to_read_position != ?", position)
			}
			if _, err := q.Exec(ctx); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	})
}

// LateClearPosition clears userID's Reading Position in fileID as
// ClearPosition does, but only a place reached at or before at: one saved
// since, such as by another reader, stands. Only an automatic finish from
// before at is made to stand. It returns ErrFileNotFound for a File that's
// gone.
func (svc *Service) LateClearPosition(ctx context.Context, userID, fileID int, at time.Time,
	claim func(ctx context.Context, tx bun.Tx) error) error {
	return svc.runLate(ctx, userID, 0, claim, func(ctx context.Context, tx bun.Tx) error {
		exists, err := tx.NewSelect().Model((*models.File)(nil)).Where("f.id = ?", fileID).Exists(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		if !exists {
			return ErrFileNotFound
		}
		_, err = tx.NewDelete().Model((*models.ReadingPosition)(nil)).
			Where("user_id = ? AND file_id = ? AND occurred_at <= ?", userID, fileID, at).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = tx.NewUpdate().Model((*models.ReadingPositionUpdate)(nil)).
			Set("auto_finished = FALSE").
			Where("user_id = ? AND file_id = ? AND auto_finished AND occurred_at <= ?", userID, fileID, at).
			Exec(ctx)
		return errors.WithStack(err)
	})
}
