package reading

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RevertWindow is how soon after an automatic finish the next update for
// the File, back before the end, undoes the finish: someone who jumped to
// the end to check the back matter and went back hasn't finished.
const RevertWindow = 15 * time.Minute

// maxSessionLength and sessionClockSlack bound the Reading Sessions the web
// readers report.
const (
	maxSessionLength  = 24 * time.Hour
	sessionClockSlack = 5 * time.Minute
)

// ErrFileNotFound is returned for a position update on a File that doesn't
// exist.
var ErrFileNotFound = errors.New("file not found")

// ErrNotAutoFinished is returned for undoing a finish that a position
// update didn't make, or that was already undone. A finish the user chose
// is never undone this way.
var ErrNotAutoFinished = errors.New("the read-through was not finished automatically")

// PositionUpdate is one source reporting where a user is in a File. It is
// the single input of the position update service for every source: the
// web readers now, devices later.
type PositionUpdate struct {
	UserID int
	FileID int
	Source string
	// Locator holds the position in the one form the source reads.
	Locator models.ReadingLocator
	// Percentage is how far through the File the position is, from 0 to 1.
	Percentage float64
	// AtEnd is the source reporting the end of the File, which finishes
	// the Read-through underway.
	AtEnd bool
	// OccurredAt is when the source was at the position, for an update
	// sent later. Nil is now, and an instant after now counts as now.
	OccurredAt *time.Time
	// Session is a Reading Session the source finished, recorded with the
	// update.
	Session *SessionInput
}

// SessionInput is a finished Reading Session handed in with an update.
type SessionInput struct {
	StartedAt       time.Time
	EndedAt         time.Time
	StartPercentage float64
	EndPercentage   float64
	// Measured is false for a session the source estimated.
	Measured bool
}

// UpdatePosition is the position update service. In one transaction it:
//
//  1. ignores an update older than the stored position, recording only its
//     session
//  2. undoes the File's automatic finish when this is the next update back
//     before the end, within RevertWindow
//  3. starts a Read-through when none is underway, the update is real
//     progress, and the Book is unread or on To Read, taking it off To Read
//     and resetting its other positions
//  4. resumes a paused Read-through on real progress
//  5. finishes the Read-through underway on reaching the end, dated in the
//     user's time zone
//  6. stores the position, logs the update, and records the session, both
//     on the Read-through underway or just finished
func (svc *Service) UpdatePosition(ctx context.Context, u PositionUpdate) (*UpdatePositionResponse, error) {
	result := &UpdatePositionResponse{}
	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		now := svc.now()
		occurredAt := now
		if u.OccurredAt != nil && u.OccurredAt.Before(now) {
			occurredAt = *u.OccurredAt
		}

		var bookID int
		err := tx.NewSelect().Model((*models.File)(nil)).Column("f.book_id").Where("f.id = ?", u.FileID).Scan(ctx, &bookID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFileNotFound
		}
		if err != nil {
			return errors.WithStack(err)
		}
		result.BookID = bookID

		stored, err := retrievePosition(ctx, tx, u.UserID, u.FileID)
		if err != nil {
			return err
		}
		underway, err := retrieveUnderway(ctx, tx, u.UserID, bookID)
		if err != nil {
			return err
		}

		if stored != nil && occurredAt.Before(stored.OccurredAt) {
			return recordSession(ctx, tx, u, underway, now)
		}
		result.Accepted = true

		// Turning past the first screen is reading; reopening where the
		// reader already was is not.
		progress := u.Percentage > 0 && (stored == nil || !stored.Equal(u.Locator))

		if underway == nil && !u.AtEnd && progress {
			reverted, err := revertAutoFinish(ctx, tx, u.UserID, u.FileID, occurredAt, now)
			if err != nil {
				return err
			}
			if reverted != nil {
				underway = reverted
				result.Reverted = true
			}
		}

		starting := false
		if underway == nil && progress {
			starting, err = startsOnProgress(ctx, tx, u.UserID, bookID)
			if err != nil {
				return err
			}
		}
		// Staying at the end, as a reader closed there after Undo reports,
		// doesn't finish again.
		finishing := u.AtEnd && progress
		var day string
		if starting || finishing && underway != nil {
			day, err = userDay(ctx, tx, u.UserID, occurredAt)
			if err != nil {
				return err
			}
		}

		switch {
		case starting:
			underway, err = startReadThrough(ctx, tx, u.UserID, bookID, day, now, u.FileID)
			if err != nil {
				return err
			}
			result.Started = true
		case underway != nil && underway.Paused && progress && !u.AtEnd:
			underway.Paused = false
			underway.UpdatedAt = now
			if _, err := tx.NewUpdate().Model(underway).Column("paused", "updated_at").WherePK().Exec(ctx); err != nil {
				return errors.WithStack(err)
			}
		}

		autoFinished := false
		if underway != nil && finishing {
			underway.Outcome = models.ReadThroughOutcomeFinished
			underway.Paused = false
			underway.EndDay = &day
			underway.UpdatedAt = now
			_, err := tx.NewUpdate().Model(underway).Column("outcome", "paused", "end_day", "updated_at").WherePK().Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
			if err := leaveToRead(ctx, tx, u.UserID, bookID, now); err != nil {
				return err
			}
			autoFinished = true
			result.FinishedReadThroughID = &underway.ID
		}

		position := &models.ReadingPosition{
			UserID:         u.UserID,
			FileID:         u.FileID,
			CreatedAt:      now,
			UpdatedAt:      now,
			ReadingLocator: u.Locator,
			Percentage:     u.Percentage,
			Source:         u.Source,
			OccurredAt:     occurredAt,
		}
		_, err = tx.NewInsert().
			Model(position).
			On("CONFLICT (user_id, file_id) DO UPDATE").
			Set("updated_at = EXCLUDED.updated_at").
			Set("cfi = EXCLUDED.cfi").
			Set("xpointer = EXCLUDED.xpointer").
			Set("page = EXCLUDED.page").
			Set("seconds = EXCLUDED.seconds").
			Set("kobo_span = EXCLUDED.kobo_span").
			Set("percentage = EXCLUDED.percentage").
			Set("source = EXCLUDED.source").
			Set("occurred_at = EXCLUDED.occurred_at").
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		fileID := u.FileID
		_, err = tx.NewInsert().Model(&models.ReadingPositionUpdate{
			CreatedAt:      now,
			UserID:         u.UserID,
			FileID:         &fileID,
			ReadThroughID:  readThroughID(underway),
			ReadingLocator: u.Locator,
			Percentage:     u.Percentage,
			Source:         u.Source,
			OccurredAt:     occurredAt,
			AtEnd:          u.AtEnd,
			AutoFinished:   autoFinished,
		}).Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		return recordSession(ctx, tx, u, underway, now)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func readThroughID(rt *models.ReadThrough) *int {
	if rt == nil {
		return nil
	}
	return &rt.ID
}

// RecordSession records a Reading Session a source finished without
// moving, such as a reader opened and closed where it was. It changes no
// Reading Position and no Read-through, and belongs to the Read-through
// underway, if any. It returns the File's Book.
func (svc *Service) RecordSession(ctx context.Context, userID, fileID int, source string, session SessionInput) (int, error) {
	var bookID int
	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		err := tx.NewSelect().Model((*models.File)(nil)).Column("f.book_id").Where("f.id = ?", fileID).Scan(ctx, &bookID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFileNotFound
		}
		if err != nil {
			return errors.WithStack(err)
		}
		underway, err := retrieveUnderway(ctx, tx, userID, bookID)
		if err != nil {
			return err
		}
		return recordSession(ctx, tx, PositionUpdate{UserID: userID, FileID: fileID, Source: source, Session: &session}, underway, svc.now())
	})
	return bookID, err
}

// recordSession records u's session, if it has one, on rt.
func recordSession(ctx context.Context, tx bun.Tx, u PositionUpdate, rt *models.ReadThrough, now time.Time) error {
	if u.Session == nil {
		return nil
	}
	fileID := u.FileID
	_, err := tx.NewInsert().Model(&models.ReadingSession{
		CreatedAt:       now,
		UserID:          u.UserID,
		FileID:          &fileID,
		ReadThroughID:   readThroughID(rt),
		Source:          u.Source,
		StartedAt:       u.Session.StartedAt,
		EndedAt:         u.Session.EndedAt,
		StartPercentage: u.Session.StartPercentage,
		EndPercentage:   u.Session.EndPercentage,
		Measured:        u.Session.Measured,
	}).Exec(ctx)
	return errors.WithStack(err)
}

// revertAutoFinish undoes the automatic finish on fileID for an update
// back before the end at occurredAt, returning the Read-through it made
// underway again, or nil when there's nothing to undo. Only the update
// after the finish undoes it, and only within RevertWindow; updates still
// at the end in between keep the window open. The caller checks that
// nothing is underway, since a Book has only one Read-through underway.
func revertAutoFinish(ctx context.Context, tx bun.Tx, userID, fileID int, occurredAt, now time.Time) (*models.ReadThrough, error) {
	finish := &models.ReadingPositionUpdate{}
	err := tx.NewSelect().
		Model(finish).
		Where("rpu.user_id = ? AND rpu.file_id = ? AND rpu.auto_finished", userID, fileID).
		Where("NOT EXISTS (SELECT 1 FROM reading_position_updates AS later WHERE later.user_id = rpu.user_id AND later.file_id = rpu.file_id AND later.id > rpu.id AND NOT later.at_end)").
		OrderExpr("rpu.id DESC").
		Limit(1).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if finish.ReadThroughID == nil || occurredAt.Sub(finish.OccurredAt) > RevertWindow {
		return nil, nil
	}
	return reopenAutoFinished(ctx, tx, *finish.ReadThroughID, now)
}

// reopenAutoFinished makes the finished Read-through rtID underway again
// and clears its update's automatic finish, so it's undone only once. It
// returns nil when rtID is no longer finished.
func reopenAutoFinished(ctx context.Context, tx bun.Tx, rtID int, now time.Time) (*models.ReadThrough, error) {
	rt := &models.ReadThrough{}
	err := tx.NewSelect().Model(rt).Where("rt.id = ? AND rt.outcome = ?", rtID, models.ReadThroughOutcomeFinished).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	rt.Outcome = models.ReadThroughOutcomeUnderway
	rt.EndDay = nil
	rt.UpdatedAt = now
	if _, err := tx.NewUpdate().Model(rt).Column("outcome", "end_day", "updated_at").WherePK().Exec(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	_, err = tx.NewUpdate().
		Model((*models.ReadingPositionUpdate)(nil)).
		Set("auto_finished = FALSE").
		Where("read_through_id = ? AND auto_finished", rtID).
		Exec(ctx)
	return rt, errors.WithStack(err)
}

// UndoAutoFinish undoes the automatic finish of rt, for Undo on the finish
// toast, refusing with ErrNotAutoFinished a finish no position update made,
// and with ErrAlreadyUnderway when another Read-through is underway.
func (svc *Service) UndoAutoFinish(ctx context.Context, rt *models.ReadThrough) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		if rt.Outcome != models.ReadThroughOutcomeFinished {
			return ErrNotAutoFinished
		}
		exists, err := tx.NewSelect().
			Model((*models.ReadingPositionUpdate)(nil)).
			Where("read_through_id = ? AND auto_finished", rt.ID).
			Exists(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		if !exists {
			return ErrNotAutoFinished
		}
		underway, err := retrieveUnderway(ctx, tx, rt.UserID, rt.BookID)
		if err != nil {
			return err
		}
		if underway != nil {
			return ErrAlreadyUnderway
		}
		_, err = reopenAutoFinished(ctx, tx, rt.ID, svc.now())
		return err
	})
}

// RetrievePosition loads userID's Reading Position in fileID, or nil when
// there is none.
func (svc *Service) RetrievePosition(ctx context.Context, userID, fileID int) (*models.ReadingPosition, error) {
	return retrievePosition(ctx, svc.db, userID, fileID)
}

func retrievePosition(ctx context.Context, db bun.IDB, userID, fileID int) (*models.ReadingPosition, error) {
	position := &models.ReadingPosition{}
	err := db.NewSelect().
		Model(position).
		Where("rp.user_id = ? AND rp.file_id = ?", userID, fileID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return position, nil
}

// resetPositions deletes userID's Reading Positions in bookID's Files
// except keepFileID (0 keeps none), so their readers open at the start.
// The update log keeps its history.
func resetPositions(ctx context.Context, db bun.IDB, userID, bookID, keepFileID int) error {
	_, err := db.NewDelete().
		Model((*models.ReadingPosition)(nil)).
		Where("user_id = ? AND file_id != ?", userID, keepFileID).
		Where("file_id IN (SELECT id FROM files WHERE book_id = ?)", bookID).
		Exec(ctx)
	return errors.WithStack(err)
}

// userDay is the day of instant in userID's time zone at the moment, or in
// UTC when the user has none, as automatic Read-through days are dated.
func userDay(ctx context.Context, db bun.IDB, userID int, instant time.Time) (string, error) {
	var zone *string
	err := db.NewSelect().
		Model((*models.UserSettings)(nil)).
		Column("us.time_zone").
		Where("us.user_id = ?", userID).
		Scan(ctx, &zone)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", errors.WithStack(err)
	}
	location := time.UTC
	if zone != nil {
		// The settings handler validates the zone, so one that fails to
		// load here means the server's zone database lost it: fall back
		// rather than refuse the reading.
		if loaded, err := time.LoadLocation(*zone); err == nil {
			location = loaded
		}
	}
	return instant.In(location).Format(time.DateOnly), nil
}

// startsOnProgress reports whether reading userID's bookID, with nothing
// underway, starts a Read-through: for a Book never read, or one on To Read,
// which plans a reread. Opening a Book already read or abandoned is looking
// something up until the user chooses Read again.
func startsOnProgress(ctx context.Context, db bun.IDB, userID, bookID int) (bool, error) {
	var starts bool
	err := db.NewSelect().
		ColumnExpr(`NOT EXISTS (SELECT 1 FROM read_throughs WHERE user_id = ? AND book_id = ?)
			OR EXISTS (SELECT 1 FROM user_books WHERE user_id = ? AND book_id = ? AND to_read_position IS NOT NULL)`,
			userID, bookID, userID, bookID).
		Scan(ctx, &starts)
	return starts, errors.WithStack(err)
}
