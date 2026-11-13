package reading

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reflowable"
	"github.com/uptrace/bun"
)

// RevertWindow is how soon after an automatic finish the next update for
// the File, back before the end, undoes the finish: someone who jumped to
// the end to check the back matter and went back hasn't finished.
const RevertWindow = 15 * time.Minute

const (
	// AudioStartSeconds is how much of one sitting of listening counts as real
	// progress: a minute of it, not a few seconds of checking what it sounds
	// like or a seek to somewhere later.
	AudioStartSeconds = 60.0
	// AudioEndSeconds is how close to an audiobook's end counts as reaching
	// it, so end credits don't stop it being marked read.
	AudioEndSeconds = 60.0
)

// ErrFileNotFound is returned for a position update on a File that doesn't
// exist.
var ErrFileNotFound = errors.New("file not found")

// ErrNotAutoFinished is returned for undoing a finish that a position
// update didn't make, or that was already undone. A finish the user chose
// is never undone this way.
var ErrNotAutoFinished = errors.New("the read-through was not finished automatically")

// ReportedStatus is a status a source reports outright, as a Kobo does, rather
// than one Shisho infers from the position.
type ReportedStatus int

const (
	// ReportedNothing leaves the status to the position.
	ReportedNothing ReportedStatus = iota
	// ReportedReading is reading, wherever the position is: it starts or
	// resumes the Read-through as real progress would.
	ReportedReading
	// ReportedFinished is the Book finished. It ends the Read-through
	// underway, or records a finished one for a Book reading would start,
	// and unlike reaching the end, it's never undone.
	ReportedFinished
)

// PositionUpdate is one source reporting where a user is in a File. It is
// the single input of the position update service for every source: the
// web readers now, devices later.
type PositionUpdate struct {
	UserID int
	FileID int
	Source string
	// DeviceID is the Device that reported the update, when a Device did.
	DeviceID *string
	// ReportedDevice and ReportedDeviceID are how the source named itself,
	// kept with the position.
	ReportedDevice   *string
	ReportedDeviceID *string
	// Locator holds the position in the one form the source reads.
	Locator models.ReadingLocator
	// LocatorFileType is the File Type of the copy Locator points into, when
	// it isn't the File's own.
	LocatorFileType *string
	// Percentage is how far through the File the position is, from 0 to 1.
	Percentage float64
	// ChapterPercentage is how far through its chapter the position is, from
	// 0 to 1, when the source says.
	ChapterPercentage *float64
	// ReportedStatus is a status the source reports outright.
	ReportedStatus ReportedStatus
	// AtEnd is the source reporting the end of the File, such as a
	// reflowable reader's final location or playback ending, which finishes
	// the Read-through underway. The last page and an audiobook's last
	// minute count as the end whether or not the source says so.
	AtEnd bool
	// ListenedSeconds is how long a source timing audio has played in this
	// sitting, including any session sent with the update. It's only for
	// deciding that this sitting starts or resumes a Read-through.
	ListenedSeconds float64
	// ListeningSince is when this sitting of listening or reading began, for
	// a source that knows. A sitting that starts a Read-through belongs to it
	// from here, first minute included, and dates its begin day.
	ListeningSince *time.Time
	// Opened is a reader opened at this place by a link, such as a page
	// thumbnail or a shared URL, which saves it as the place but isn't
	// reading: it starts, resumes, finishes, and undoes nothing.
	Opened bool
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
	// DeviceID is the Device that reported the session, when a Device did.
	// A session sent with a position update takes the update's.
	DeviceID *string
}

// UpdatePosition is the position update service. In one transaction it:
//
//  1. ignores an update older than the stored position, recording only its
//     session
//  2. undoes the File's automatic finish when this is the next update back
//     before the end, within RevertWindow, from the reader that finished it
//  3. starts a Read-through when none is underway, the update is real
//     progress, and the Book is unread or on To Read, taking it off To Read
//     and resetting its other positions
//  4. resumes a paused Read-through on real progress made since the pause
//  5. finishes the Read-through underway on reaching the end, or on a
//     reported finish, dated in the user's time zone
//  6. stores the position, logs the update, and records the session, both
//     on the Read-through underway or just finished
//
// An update with no locator, a status reported on its own, changes the
// Read-through and stores nothing else.
func (svc *Service) UpdatePosition(ctx context.Context, u PositionUpdate) (*UpdatePositionResponse, error) {
	result := &UpdatePositionResponse{}
	// A session sent with an update comes from the update's Device.
	if u.Session != nil {
		session := *u.Session
		session.DeviceID = u.DeviceID
		u.Session = &session
	}
	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		now := svc.now()
		occurredAt := now
		if u.OccurredAt != nil && u.OccurredAt.Before(now) {
			occurredAt = *u.OccurredAt
		}

		file := &models.File{}
		err := tx.NewSelect().
			Model(file).
			Column("f.book_id", "f.file_role", "f.page_count", "f.audiobook_duration_seconds").
			Where("f.id = ?", u.FileID).
			Scan(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFileNotFound
		}
		if err != nil {
			return errors.WithStack(err)
		}
		bookID := file.BookID
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
			return recordSession(ctx, tx, u.UserID, u.FileID, u.Source, u.Session, underway, now)
		}
		result.Accepted = true

		hasLocator := !u.Locator.IsZero()
		// Turning past the first screen is reading; reopening where the
		// reader already was is not. Going back all the way to the start is
		// still going back, so it can undo a finish.
		moved := hasLocator && (stored == nil || !stored.Equal(u.Locator))
		// A supplement keeps its own place but isn't the Book, and a place a
		// link opened isn't reading: neither changes any Read-through.
		reading := !u.Opened && file.FileRole != models.FileRoleSupplement
		atEnd := reading && hasLocator && (u.AtEnd || locatorAtEnd(file, u.Locator))
		reportedFinish := reading && u.ReportedStatus == ReportedFinished
		reported := reading && u.ReportedStatus != ReportedNothing

		if underway == nil && !atEnd && moved && reading && !reportedFinish {
			reverted, err := revertAutoFinish(ctx, tx, u, occurredAt, now)
			if err != nil {
				return err
			}
			if reverted != nil {
				underway = reverted
				result.Reverted = true
			}
		}

		// Staying at the end, as a reader closed there after Undo reports,
		// isn't reaching it, and neither is playing on through the last minute.
		// Playback ending still is.
		stayedAtEnd := stored != nil && !u.AtEnd && locatorAtEnd(file, stored.ReadingLocator)
		reachedEnd := atEnd && !stayedAtEnd
		// A source that reports its status says outright that this is reading.
		progress := reported || moved && reading && (reachedEnd || isRealProgress(u, underway))

		starting := false
		if underway == nil && progress {
			starting, err = startsOnProgress(ctx, tx, u.UserID, bookID)
			if err != nil {
				return err
			}
		}
		finishing := (reachedEnd || reportedFinish) && progress
		var day string
		if starting || finishing && underway != nil {
			day, err = userDay(ctx, tx, u.UserID, occurredAt)
			if err != nil {
				return err
			}
		}
		// A read that a sitting of listening started began when the sitting did.
		beginDay := day
		listeningSince := occurredAt
		if starting && u.ListeningSince != nil && u.ListeningSince.Before(occurredAt) {
			listeningSince = *u.ListeningSince
			beginDay, err = userDay(ctx, tx, u.UserID, listeningSince)
			if err != nil {
				return err
			}
		}

		switch {
		case starting && reportedFinish:
			// Reported finished before Shisho saw it begin, as a book read on
			// the device first: a finished read with no begin day, like Mark as
			// read with nothing underway, which leaves the other Files' places.
			finished, err := insertFinishedReadThrough(ctx, tx, u.UserID, bookID, day, now)
			if err != nil {
				return err
			}
			result.FinishedReadThroughID = &finished.ID
			// The update and its session belong to the read they finished.
			underway = finished
			finishing = false
		case starting:
			underway, err = startReadThrough(ctx, tx, u.UserID, bookID, beginDay, now, u.FileID)
			if err != nil {
				return err
			}
			if err := claimSitting(ctx, tx, u, underway.ID, listeningSince); err != nil {
				return err
			}
			result.Started = true
		// Reading from before the pause, sent late as a device's offline
		// queue does, saves the place but doesn't undo the pause.
		case underway != nil && underway.Paused && progress && !finishing && !occurredAt.Before(underway.UpdatedAt):
			underway.Paused = false
			underway.UpdatedAt = now
			if _, err := tx.NewUpdate().Model(underway).Column("paused", "updated_at").WherePK().Exec(ctx); err != nil {
				return errors.WithStack(err)
			}
			result.Resumed = true
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
			// A reported finish is the reader's own, so it's never undone.
			autoFinished = !reportedFinish
			result.FinishedReadThroughID = &underway.ID
		}

		if !hasLocator {
			return recordSession(ctx, tx, u.UserID, u.FileID, u.Source, u.Session, underway, now)
		}

		position := &models.ReadingPosition{
			UserID:            u.UserID,
			FileID:            u.FileID,
			CreatedAt:         now,
			UpdatedAt:         now,
			ReadingLocator:    u.Locator,
			LocatorFileType:   u.LocatorFileType,
			Percentage:        u.Percentage,
			ChapterPercentage: u.ChapterPercentage,
			Source:            u.Source,
			SourceDeviceID:    u.DeviceID,
			ReportedDevice:    u.ReportedDevice,
			ReportedDeviceID:  u.ReportedDeviceID,
			OccurredAt:        occurredAt,
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
			Set("locator_file_type = EXCLUDED.locator_file_type").
			Set("percentage = EXCLUDED.percentage").
			Set("chapter_percentage = EXCLUDED.chapter_percentage").
			Set("source = EXCLUDED.source").
			Set("source_device_id = EXCLUDED.source_device_id").
			Set("reported_device = EXCLUDED.reported_device").
			Set("reported_device_id = EXCLUDED.reported_device_id").
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
			SourceDeviceID: u.DeviceID,
			OccurredAt:     occurredAt,
			AtEnd:          atEnd,
			AutoFinished:   autoFinished,
			Opened:         u.Opened,
		}).Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		return recordSession(ctx, tx, u.UserID, u.FileID, u.Source, u.Session, underway, now)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// isRealProgress reports whether u's position is past where merely opening
// the File leaves a reader: past the first page or screen, or a minute of
// listening in one sitting. A Read-through already going needs no such minute.
func isRealProgress(u PositionUpdate, underway *models.ReadThrough) bool {
	switch {
	case u.Locator.Seconds != nil:
		if underway != nil && !underway.Paused {
			return true
		}
		return u.ListenedSeconds >= AudioStartSeconds
	case u.Locator.Page != nil:
		return *u.Locator.Page > 0
	default:
		return u.Percentage > 0
	}
}

// claimSitting puts what u's sitting already recorded, before it knew it would
// start a Read-through, on rtID, so its first minute counts toward the read.
func claimSitting(ctx context.Context, tx bun.Tx, u PositionUpdate, rtID int, since time.Time) error {
	if u.ListeningSince == nil {
		return nil
	}
	_, err := tx.NewUpdate().
		Model((*models.ReadingPositionUpdate)(nil)).
		Set("read_through_id = ?", rtID).
		Where("user_id = ? AND file_id = ? AND read_through_id IS NULL AND occurred_at >= ?", u.UserID, u.FileID, since).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = tx.NewUpdate().
		Model((*models.ReadingSession)(nil)).
		Set("read_through_id = ?", rtID).
		Where("user_id = ? AND file_id = ? AND read_through_id IS NULL AND started_at >= ?", u.UserID, u.FileID, since).
		Exec(ctx)
	return errors.WithStack(err)
}

// locatorAtEnd reports whether the position is at the end of file, for the
// forms where the File's own length says so: its last page, or its last
// minute of audio. A length the scan didn't record leaves it to the source, as
// does audio no longer than that minute, which would be at its end throughout.
func locatorAtEnd(file *models.File, locator models.ReadingLocator) bool {
	switch {
	case locator.Page != nil:
		return file.PageCount != nil && *file.PageCount > 0 && *locator.Page >= *file.PageCount-1
	case locator.Seconds != nil:
		return file.AudiobookDurationSeconds != nil && *file.AudiobookDurationSeconds > AudioEndSeconds &&
			*locator.Seconds >= *file.AudiobookDurationSeconds-AudioEndSeconds
	default:
		return false
	}
}

func readThroughID(rt *models.ReadThrough) *int {
	if rt == nil {
		return nil
	}
	return &rt.ID
}

// RecordSession records a Reading Session a source finished without
// moving, such as a reader opened and closed where it was. It changes no
// Reading Position and no Read-through. It returns the File's Book.
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
		return recordSession(ctx, tx, userID, fileID, source, &session, underway, svc.now())
	})
	return bookID, err
}

// recordSession records session, if there is one, on rt, the Read-through
// underway. With nothing underway, it belongs to the Read-through that was
// while it ran: one that finished during it, say, since a reader sends a
// session when it ends, after the update that finished the Book.
func recordSession(ctx context.Context, tx bun.Tx, userID, fileID int, source string, session *SessionInput, rt *models.ReadThrough, now time.Time) error {
	if session == nil {
		return nil
	}
	_, err := insertSession(ctx, tx, userID, fileID, source, *session, rt, now)
	return err
}

// insertSession records session as recordSession does, returning its id.
func insertSession(ctx context.Context, tx bun.Tx, userID, fileID int, source string, session SessionInput, rt *models.ReadThrough, now time.Time) (int, error) {
	sessionReadThrough := readThroughID(rt)
	if sessionReadThrough == nil {
		var ranDuring int
		err := tx.NewSelect().
			Model((*models.ReadingPositionUpdate)(nil)).
			Column("rpu.read_through_id").
			Where("rpu.user_id = ? AND rpu.file_id = ? AND rpu.read_through_id IS NOT NULL", userID, fileID).
			Where("rpu.occurred_at >= ?", session.StartedAt).
			OrderExpr("rpu.id DESC").
			Limit(1).
			Scan(ctx, &ranDuring)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, errors.WithStack(err)
		}
		if err == nil {
			sessionReadThrough = &ranDuring
		}
	}
	var fileType string
	err := tx.NewSelect().Model((*models.File)(nil)).Column("f.file_type").Where("f.id = ?", fileID).Scan(ctx, &fileType)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	inserted := &models.ReadingSession{
		CreatedAt:       now,
		UserID:          userID,
		FileID:          &fileID,
		ReadThroughID:   sessionReadThrough,
		Source:          source,
		StartedAt:       session.StartedAt,
		EndedAt:         session.EndedAt,
		StartPercentage: session.StartPercentage,
		EndPercentage:   session.EndPercentage,
		Measured:        session.Measured,
		Kind:            models.ReadingKindForFileType(fileType),
		SourceDeviceID:  session.DeviceID,
		Duration:        int(session.EndedAt.Sub(session.StartedAt).Round(time.Second) / time.Second),
	}
	_, err = tx.NewInsert().Model(inserted).Exec(ctx)
	return inserted.ID, errors.WithStack(err)
}

// revertAutoFinish undoes the automatic finish on u's File for u, an update
// back before the end at occurredAt, returning the Read-through it made
// underway again, or nil when there's nothing to undo. Only the update
// after the finish undoes it, only within RevertWindow, and only from the
// reader that finished it: another reader's update going back is one sent
// from where it was before, such as a device's queued while offline.
// Updates still at the end in between keep the window open. The caller
// checks that nothing is underway, since a Book has only one Read-through
// underway.
func revertAutoFinish(ctx context.Context, tx bun.Tx, u PositionUpdate, occurredAt, now time.Time) (*models.ReadThrough, error) {
	userID, fileID := u.UserID, u.FileID
	finish := &models.ReadingPositionUpdate{}
	err := tx.NewSelect().
		Model(finish).
		Where("rpu.user_id = ? AND rpu.file_id = ? AND rpu.auto_finished", userID, fileID).
		// Another reader's updates in between neither undo it nor use up the
		// finishing reader's chance to.
		Where("NOT EXISTS (SELECT 1 FROM reading_position_updates AS later WHERE later.user_id = rpu.user_id AND later.file_id = rpu.file_id AND later.id > rpu.id AND NOT later.at_end AND later.source = rpu.source AND later.source_device_id IS rpu.source_device_id)").
		// A newer Read-through, such as a Mark as read since, stands.
		Where("NOT EXISTS (SELECT 1 FROM read_throughs AS this JOIN read_throughs AS newer ON newer.user_id = this.user_id AND newer.book_id = this.book_id AND newer.id > this.id WHERE this.id = rpu.read_through_id)").
		OrderExpr("rpu.id DESC").
		Limit(1).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	sameReader := finish.Source == u.Source &&
		(finish.SourceDeviceID == nil) == (u.DeviceID == nil) &&
		(finish.SourceDeviceID == nil || *finish.SourceDeviceID == *u.DeviceID)
	if !sameReader {
		return nil, nil
	}
	if finish.ReadThroughID == nil || occurredAt.Sub(finish.OccurredAt) > RevertWindow {
		return nil, nil
	}
	return reopenAutoFinished(ctx, tx, *finish.ReadThroughID, now)
}

// reopenAutoFinished makes the finished Read-through rtID underway again
// and clears its update's automatic finish, so it's undone only once. A Book
// added to To Read since leaves it, as a Book underway can't be on To Read.
// It returns nil when rtID is no longer finished.
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
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if rt.BookID == nil {
		return rt, nil
	}
	return rt, leaveToRead(ctx, tx, rt.UserID, *rt.BookID, now)
}

// UndoAutoFinish undoes the automatic finish of rt, for Undo on the finish
// toast, refusing with ErrNotAutoFinished a finish no position update made,
// and with ErrAlreadyUnderway when another Read-through is underway.
func (svc *Service) UndoAutoFinish(ctx context.Context, rt *models.ReadThrough) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// A detached Read-through's finish has no Book to read again.
		if rt.Outcome != models.ReadThroughOutcomeFinished || rt.BookID == nil {
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
		underway, err := retrieveUnderway(ctx, tx, rt.UserID, *rt.BookID)
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

// RetrieveWebReaderPosition loads userID's Reading Position in fileID as the
// web reader reads it, or nil when there is none. A reflowable position
// another reader wrote, such as KOReader's XPointer, comes as a CFI in the
// File's own copy, which is the one the web reader opens. One that can't be
// translated, as when the File can't be read, comes as it was stored.
func (svc *Service) RetrieveWebReaderPosition(ctx context.Context, userID, fileID int) (*models.ReadingPosition, error) {
	position, err := retrievePosition(ctx, svc.db, userID, fileID)
	if err != nil || position == nil || position.XPointer == nil && position.KoboSpan == nil {
		return position, err
	}
	file := &models.File{}
	err = svc.db.NewSelect().Model(file).Column("f.file_type", "f.filepath").Where("f.id = ?", fileID).Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	locator, err := reflowable.Translate(ctx, file.Filepath, file.FileType, position, file.FileType, reflowable.FormCFI)
	if errors.Is(err, reflowable.ErrNotTranslatable) {
		return position, nil
	}
	if err != nil {
		logger.FromContext(ctx).Warn("reading position not translated for the web reader", logger.Data{
			"file_id": fileID, "error": err.Error(),
		})
		return position, nil
	}
	position.ReadingLocator = locator
	position.LocatorFileType = nil
	return position, nil
}

// DeviceUpdatedSince reports whether deviceID sent an update that arrived at
// or after since.
func (svc *Service) DeviceUpdatedSince(ctx context.Context, deviceID string, since time.Time) (bool, error) {
	exists, err := svc.db.NewSelect().
		Model((*models.ReadingPositionUpdate)(nil)).
		Where("rpu.source_device_id = ? AND rpu.created_at >= ?", deviceID, since).
		Exists(ctx)
	return exists, errors.WithStack(err)
}

// RecordEstimatedSession records session, which source estimated, for
// userID in fileID, returning its id.
func (svc *Service) RecordEstimatedSession(ctx context.Context, userID, fileID int, source string, session SessionInput) (int, error) {
	var id int
	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		var bookID int
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
		session.Measured = false
		id, err = insertSession(ctx, tx, userID, fileID, source, session, underway, svc.now())
		return err
	})
	return id, err
}

// SetEstimatedSessionEnd moves the end of userID's estimated Reading Session
// id, as its source learns more of it. Its duration stays its span.
func (svc *Service) SetEstimatedSessionEnd(ctx context.Context, userID, id int, endedAt time.Time, endPercentage float64) error {
	_, err := svc.db.NewUpdate().
		Model((*models.ReadingSession)(nil)).
		Set("ended_at = ?", endedAt).
		Set("end_percentage = ?", endPercentage).
		Set("duration = MAX(0, CAST(ROUND((julianday(?) - julianday(started_at)) * 86400) AS INTEGER))", endedAt).
		Where("id = ? AND user_id = ? AND NOT measured", id, userID).
		Exec(ctx)
	return errors.WithStack(err)
}

// DeleteSession deletes userID's Reading Session id for good, one that
// turned out not to be reading, and reports whether there was one: another
// user's is never deleted. It's a plain delete, which KOReader sync relies
// on to keep a deleted Session from coming back.
func (svc *Service) DeleteSession(ctx context.Context, userID, id int) (bool, error) {
	res, err := svc.db.NewDelete().Model((*models.ReadingSession)(nil)).Where("id = ? AND user_id = ?", id, userID).Exec(ctx)
	if err != nil {
		return false, errors.WithStack(err)
	}
	n, err := res.RowsAffected()
	return n > 0, errors.WithStack(err)
}

// RetrievePosition loads userID's Reading Position in fileID, or nil when
// there is none, in the form and copy its source wrote: a reader given it
// gets it through reflowable.Translate, as RetrieveWebReaderPosition does.
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

// resetPositions deletes userID's Reading Positions in bookID's main Files
// except keepFileID (0 keeps none), so their readers open at the start. A
// supplement's place has nothing to do with the Read-through, so it stays. The
// update log keeps its history.
func resetPositions(ctx context.Context, db bun.IDB, userID, bookID, keepFileID int) error {
	_, err := db.NewDelete().
		Model((*models.ReadingPosition)(nil)).
		Where("user_id = ? AND file_id != ?", userID, keepFileID).
		Where("file_id IN (SELECT id FROM files WHERE book_id = ? AND file_role = ?)", bookID, models.FileRoleMain).
		Exec(ctx)
	return errors.WithStack(err)
}

// userDay is the day of instant in userID's time zone at the moment, or in
// UTC when the user has none, as automatic Read-through days are dated.
func userDay(ctx context.Context, db bun.IDB, userID int, instant time.Time) (string, error) {
	location, err := userLocation(ctx, db, userID)
	if err != nil {
		return "", err
	}
	return instant.In(location).Format(time.DateOnly), nil
}

// userLocation is userID's time zone at the moment, or UTC when the user has
// none.
func userLocation(ctx context.Context, db bun.IDB, userID int) (*time.Location, error) {
	var zone *string
	err := db.NewSelect().
		Model((*models.UserSettings)(nil)).
		Column("us.time_zone").
		Where("us.user_id = ?", userID).
		Scan(ctx, &zone)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.WithStack(err)
	}
	if zone != nil {
		// The settings handler validates the zone, so one that fails to
		// load here means the server's zone database lost it: fall back
		// rather than refuse the reading.
		if loaded, err := time.LoadLocation(*zone); err == nil {
			return loaded, nil
		}
	}
	return time.UTC, nil
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
