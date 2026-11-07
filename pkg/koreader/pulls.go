package koreader

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/uptrace/bun"
)

// pullStore keeps each Device's latest pull of each document, which the
// pushes that follow make a Reading Session from.
type pullStore struct {
	db             *bun.DB
	readingService *reading.Service
}

// record notes that deviceID pulled document, the copy of fileID, at now,
// finding the position at percentage, or none when it's nil. It starts a new
// sitting.
func (s *pullStore) record(ctx context.Context, deviceID, document string, fileID int, percentage *float64, now time.Time) error {
	_, err := s.db.NewInsert().
		Model(&models.KOReaderPull{DeviceID: deviceID, Document: document, FileID: fileID, PulledAt: now, Percentage: percentage}).
		On("CONFLICT (device_id, document) DO UPDATE").
		Set("file_id = EXCLUDED.file_id").
		Set("pulled_at = EXCLUDED.pulled_at").
		Set("percentage = EXCLUDED.percentage").
		Set("paired_at = NULL").
		Set("session_id = NULL").
		Set("ended_at = NULL").
		Set("end_percentage = NULL").
		Set("extended_at = NULL").
		Set("previous_ended_at = NULL").
		Set("previous_end_percentage = NULL").
		Exec(ctx)
	return errors.WithStack(err)
}

// sitting records userID's push of document from deviceID, at percentage at
// at, arriving now, as the end of the sitting its pull began so far: the
// first push after the pull records an estimated Reading Session, and each
// later one, as KOReader pushes every few pages, moves its end, for up to
// sessionWindow after the pull. A push with no pull of the document before
// it records nothing, and neither does one dated before the session's end.
func (s *pullStore) sitting(ctx context.Context, userID int, deviceID, document string, fileID int, at time.Time, percentage float64, now time.Time) error {
	pull, err := s.find(ctx, deviceID, document)
	if err != nil || pull == nil {
		return err
	}
	// A pull that a replay used up, or whose session was deleted, starts no
	// sitting.
	if pull.PairedAt != nil && pull.SessionID == nil {
		return nil
	}
	if pull.FileID != fileID || !at.After(pull.PulledAt) || at.Sub(pull.PulledAt) > sessionWindow {
		return nil
	}
	update := s.db.NewUpdate().Model((*models.KOReaderPull)(nil)).
		Where("device_id = ? AND document = ?", deviceID, document).
		Set("ended_at = ?", at).
		Set("end_percentage = ?", percentage)

	if pull.SessionID != nil {
		if pull.EndedAt != nil && !at.After(*pull.EndedAt) {
			return nil
		}
		if err := s.readingService.SetEstimatedSessionEnd(ctx, userID, *pull.SessionID, at, percentage); err != nil {
			return err
		}
		_, err = update.
			Set("extended_at = ?", now).
			Set("previous_ended_at = ?", pull.EndedAt).
			Set("previous_end_percentage = ?", pull.EndPercentage).
			Exec(ctx)
		return errors.WithStack(err)
	}

	start := percentage
	if pull.Percentage != nil {
		start = *pull.Percentage
	}
	id, err := s.readingService.RecordEstimatedSession(ctx, userID, fileID, models.ReadingSourceKOReader, reading.SessionInput{
		StartedAt: pull.PulledAt, EndedAt: at, StartPercentage: start, EndPercentage: percentage,
	})
	if err != nil {
		return err
	}
	_, err = update.Set("session_id = ?", id).Set("paired_at = ?", now).Exec(ctx)
	return errors.WithStack(err)
}

// replay handles a push of document from deviceID that came seconds after
// another, as KOReader sends what it queued offline: a replay is no sitting.
// It undoes what the replay's first push, which arrived before the rest
// showed it was one, did to a session since since: deleting one it started,
// and putting back the end of one it moved. And it uses up the pulls of the
// replay's documents, so a push hours later can't make a sitting of the time
// spent offline.
func (s *pullStore) replay(ctx context.Context, userID int, deviceID, document string, since, now time.Time) error {
	var touched []*models.KOReaderPull
	err := s.db.NewSelect().
		Model(&touched).
		Where("kp.device_id = ? AND kp.session_id IS NOT NULL", deviceID).
		Where("kp.paired_at >= ? OR kp.extended_at >= ?", since, since).
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, pull := range touched {
		switch {
		case !pull.PairedAt.Before(since):
			err = s.readingService.DeleteSession(ctx, userID, *pull.SessionID)
		case pull.PreviousEndedAt != nil && pull.PreviousEndPercentage != nil:
			err = s.readingService.SetEstimatedSessionEnd(ctx, userID, *pull.SessionID, *pull.PreviousEndedAt, *pull.PreviousEndPercentage)
		}
		if err != nil {
			return err
		}
	}
	_, err = s.db.NewUpdate().
		Model((*models.KOReaderPull)(nil)).
		Set("session_id = NULL").
		Set("paired_at = COALESCE(paired_at, ?)", now).
		Where("device_id = ?", deviceID).
		Where("document = ? OR (session_id IS NOT NULL AND (paired_at >= ? OR extended_at >= ?))", document, since, since).
		Exec(ctx)
	return errors.WithStack(err)
}

func (s *pullStore) find(ctx context.Context, deviceID, document string) (*models.KOReaderPull, error) {
	pull := &models.KOReaderPull{}
	err := s.db.NewSelect().Model(pull).Where("kp.device_id = ? AND kp.document = ?", deviceID, document).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return pull, errors.WithStack(err)
}
