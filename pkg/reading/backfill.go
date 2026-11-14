package reading

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// History is what a device recorded before it began syncing, such as
// KOReader's statistics and statuses from before the plugin signed in (ADR
// 0016). It only fills in what Shisho has none of, so sending it again
// changes nothing.

// RecordHistoryStatus makes the Read-through a status from a device's
// history stands for, s, when the Book has none, so it makes at most one per
// Book: the device keeps no record of rereads, only the status a Book has
// now. It begins on the day of the Book's first Reading Session, or on s.Day
// when that's earlier or there's none, and a Read Book's ends on s.Day:
//
//   - Read records a finished Read-through, leaving the Book on To Read
//     when it's there, since it's no newer than a reread queued since
//   - Paused records one underway, paused
//   - Currently Reading records one underway
//
// Reading Sessions on no Read-through are then placed again by their time.
// A Book with a Read-through already has a record of its own, which history
// can't improve on, and a supplement isn't the Book, so neither changes.
func (svc *Service) RecordHistoryStatus(ctx context.Context, s DeviceStatus) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		now := svc.now()
		bookID, err := statusBook(ctx, tx, s.FileID)
		if err != nil || bookID == 0 {
			return err
		}
		read, err := tx.NewSelect().
			Model((*models.ReadThrough)(nil)).
			Where("rt.user_id = ? AND rt.book_id = ?", s.UserID, bookID).
			Exists(ctx)
		if err != nil || read {
			return errors.WithStack(err)
		}

		begin, err := firstSessionDay(ctx, tx, s.UserID, bookID)
		if err != nil {
			return err
		}
		// Days are YYYY-MM-DD, so they compare as strings.
		if begin == "" || begin > s.Day {
			begin = s.Day
		}
		switch s.Status {
		case models.ReadingStatusRead:
			rt := &models.ReadThrough{
				CreatedAt: now,
				UpdatedAt: now,
				UserID:    s.UserID,
				BookID:    &bookID,
				BeginDay:  &begin,
				EndDay:    &s.Day,
				Outcome:   models.ReadThroughOutcomeFinished,
			}
			if _, err := tx.NewInsert().Model(rt).Exec(ctx); err != nil {
				return errors.WithStack(err)
			}
		case models.ReadingStatusPaused, models.ReadingStatusCurrentlyReading:
			rt, err := startReadThrough(ctx, tx, s.UserID, bookID, begin, now, s.FileID)
			if err != nil {
				return err
			}
			if s.Status == models.ReadingStatusPaused {
				rt.Paused = true
				if _, err := tx.NewUpdate().Model(rt).Column("paused").WherePK().Exec(ctx); err != nil {
					return errors.WithStack(err)
				}
			}
		default:
			return errors.Errorf("unknown history status %q", s.Status)
		}
		return placeBookSessions(ctx, tx, s.UserID, bookID)
	})
}

// FillRating sets userID's Rating of bookID to units, in quarter stars, from
// a device's history, only when the Book has none.
func (svc *Service) FillRating(ctx context.Context, userID, bookID, units int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		rated, err := tx.NewSelect().
			Model((*models.UserBook)(nil)).
			Where("ub.user_id = ? AND ub.book_id = ? AND ub.rating IS NOT NULL", userID, bookID).
			Exists(ctx)
		if err != nil || rated {
			return errors.WithStack(err)
		}
		return setRating(ctx, tx, userID, bookID, &units, svc.now())
	})
}

// firstSessionDay is the day userID's first Reading Session of bookID
// began, in their time zone, or "" when they have none.
func firstSessionDay(ctx context.Context, tx bun.Tx, userID, bookID int) (string, error) {
	session := &models.ReadingSession{}
	err := tx.NewSelect().
		Model(session).
		Column("rs.started_at").
		Where("rs.user_id = ?", userID).
		Where("rs.file_id IN (SELECT id FROM files WHERE book_id = ?)", bookID).
		OrderExpr("rs.started_at ASC").
		Limit(1).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", errors.WithStack(err)
	}
	return userDay(ctx, tx, userID, session.StartedAt)
}
