package reading

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// DeviceStatus is a Reading Status a device's own reader set on purpose, as
// KOReader's finished, On hold, and reading are (ADR 0016), dated by the day
// it was set.
type DeviceStatus struct {
	UserID int
	// FileID is the File the device holds a copy of.
	FileID int
	// Status is ReadingStatusRead, ReadingStatusPaused, or
	// ReadingStatusCurrentlyReading.
	Status string
	// Day is when the device set it, in the user's time zone.
	Day string
}

// SetDeviceStatus applies a status a device set on purpose:
//
//   - Read finishes the Read-through underway, ending on Day, or records a
//     finished one ending on Day when none is underway
//   - Paused pauses the Read-through underway, or starts one on Day and
//     pauses it
//   - Currently Reading resumes the Read-through underway, or starts one on
//     Day, which is a reread when the Book was read before
//
// Reading Sessions on no Read-through are then placed again by their time,
// since sessions a device sent before the status may now belong to the
// Read-through it started or finished. A supplement isn't the Book, so its
// status changes nothing. The caller decides whether the status is new: a
// Read with nothing underway records another finished Read-through.
func (svc *Service) SetDeviceStatus(ctx context.Context, s DeviceStatus) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		now := svc.now()
		file := &models.File{}
		err := tx.NewSelect().Model(file).Column("f.book_id", "f.file_role").Where("f.id = ?", s.FileID).Scan(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFileNotFound
		}
		if err != nil {
			return errors.WithStack(err)
		}
		if file.FileRole == models.FileRoleSupplement {
			return nil
		}
		bookID := file.BookID
		underway, err := retrieveUnderway(ctx, tx, s.UserID, bookID)
		if err != nil {
			return err
		}

		switch s.Status {
		case models.ReadingStatusRead:
			if underway == nil {
				if _, err := insertFinishedReadThrough(ctx, tx, s.UserID, bookID, s.Day, now); err != nil {
					return err
				}
				break
			}
			endDay := s.Day
			// Days are YYYY-MM-DD, so they compare as strings.
			if underway.BeginDay != nil && endDay < *underway.BeginDay {
				endDay = *underway.BeginDay
			}
			underway.Outcome = models.ReadThroughOutcomeFinished
			underway.Paused = false
			underway.EndDay = &endDay
			underway.UpdatedAt = now
			_, err := tx.NewUpdate().Model(underway).Column("outcome", "paused", "end_day", "updated_at").WherePK().Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
			if err := leaveToRead(ctx, tx, s.UserID, bookID, now); err != nil {
				return err
			}
		case models.ReadingStatusPaused, models.ReadingStatusCurrentlyReading:
			paused := s.Status == models.ReadingStatusPaused
			if underway == nil {
				// The device's copy keeps its place: it's where the device is.
				underway, err = startReadThrough(ctx, tx, s.UserID, bookID, s.Day, now, s.FileID)
				if err != nil {
					return err
				}
			}
			if underway.Paused != paused {
				underway.Paused = paused
				underway.UpdatedAt = now
				if _, err := tx.NewUpdate().Model(underway).Column("paused", "updated_at").WherePK().Exec(ctx); err != nil {
					return errors.WithStack(err)
				}
			}
		default:
			return errors.Errorf("unknown device status %q", s.Status)
		}
		return placeBookSessions(ctx, tx, s.UserID, bookID)
	})
}

// placeBookSessions places each of userID's Reading Sessions of bookID that
// are on no Read-through by its time, on the Read-through underway then.
func placeBookSessions(ctx context.Context, tx bun.Tx, userID, bookID int) error {
	var sessions []*models.ReadingSession
	err := tx.NewSelect().
		Model(&sessions).
		Where("rs.user_id = ? AND rs.read_through_id IS NULL", userID).
		Where("rs.file_id IN (SELECT id FROM files WHERE book_id = ?)", bookID).
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, session := range sessions {
		rt, err := readThroughAt(ctx, tx, userID, bookID, session.StartedAt)
		if err != nil {
			return err
		}
		if rt == nil {
			continue
		}
		session.ReadThroughID = &rt.ID
		if _, err := tx.NewUpdate().Model(session).Column("read_through_id").WherePK().Exec(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// ChangedSince returns userID's Books whose Read-throughs or Rating changed
// after since. A deleted Read-through leaves nothing to find.
func (svc *Service) ChangedSince(ctx context.Context, userID int, since time.Time) ([]int, error) {
	var bookIDs []int
	err := svc.db.NewSelect().
		ColumnExpr("rt.book_id").
		TableExpr("read_throughs AS rt").
		Where("rt.user_id = ? AND rt.book_id IS NOT NULL AND rt.updated_at > ?", userID, since).
		Union(svc.db.NewSelect().
			ColumnExpr("ub.book_id").
			TableExpr("user_books AS ub").
			Where("ub.user_id = ? AND ub.rating_updated_at > ?", userID, since)).
		Scan(ctx, &bookIDs)
	return bookIDs, errors.WithStack(err)
}

// Location is userID's time zone, which their days are in, or UTC when they
// have none.
func (svc *Service) Location(ctx context.Context, userID int) (*time.Location, error) {
	return userLocation(ctx, svc.db, userID)
}

// KeepFinished makes a finish of the Book fileID belongs to that a position
// update made automatically one the reader chose, as when a device reports
// it finished after reaching the end, so going back no longer undoes it.
func (svc *Service) KeepFinished(ctx context.Context, userID, fileID int) error {
	_, err := svc.db.NewUpdate().
		Model((*models.ReadingPositionUpdate)(nil)).
		Set("auto_finished = FALSE").
		Where("user_id = ? AND auto_finished", userID).
		Where("file_id IN (SELECT id FROM files WHERE book_id = (SELECT book_id FROM files WHERE id = ?))", fileID).
		Exec(ctx)
	return errors.WithStack(err)
}
