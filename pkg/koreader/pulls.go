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

// record notes that deviceID pulled document, the copy of fileID, finding
// the position at percentage, or none when it's nil. It starts a new sitting.
func (s *pullStore) record(ctx context.Context, deviceID, document string, fileID int, percentage *float64) error {
	_, err := s.db.NewInsert().
		Model(&models.KOReaderPull{DeviceID: deviceID, Document: document, FileID: fileID, PulledAt: time.Now(), Percentage: percentage}).
		On("CONFLICT (device_id, document) DO UPDATE").
		Set("file_id = EXCLUDED.file_id").
		Set("pulled_at = EXCLUDED.pulled_at").
		Set("percentage = EXCLUDED.percentage").
		Set("paired_at = NULL").
		Set("session_id = NULL").
		Exec(ctx)
	return errors.WithStack(err)
}

// sitting records userID's push of document from deviceID, at percentage at
// at, as the end of the sitting its pull began: the first push after the pull
// records an estimated Reading Session, and each later one, as KOReader
// pushes every few pages, moves its end, for up to sessionWindow after the
// pull. A push with no pull of the document before it records nothing.
func (s *pullStore) sitting(ctx context.Context, userID int, deviceID, document string, fileID int, at time.Time, percentage float64) error {
	pull, err := s.find(ctx, deviceID, document)
	if err != nil || pull == nil {
		return err
	}
	// A pull that a replay used up starts no sitting.
	if pull.PairedAt != nil && pull.SessionID == nil {
		return nil
	}
	if pull.FileID != fileID || !at.After(pull.PulledAt) || at.Sub(pull.PulledAt) > sessionWindow {
		return nil
	}
	start := percentage
	if pull.Percentage != nil {
		start = *pull.Percentage
	}
	id, err := s.readingService.RecordEstimatedSession(ctx, userID, fileID, models.ReadingSourceKOReader, pull.SessionID, reading.SessionInput{
		StartedAt: pull.PulledAt, EndedAt: at, StartPercentage: start, EndPercentage: percentage,
	})
	if err != nil {
		return err
	}
	if pull.SessionID != nil && *pull.SessionID == id {
		return nil
	}
	_, err = s.db.NewUpdate().
		Model((*models.KOReaderPull)(nil)).
		Set("session_id = ?", id).
		Set("paired_at = ?", time.Now()).
		Where("device_id = ? AND document = ?", deviceID, document).
		Exec(ctx)
	return errors.WithStack(err)
}

// replay handles a push of document from deviceID that came seconds after
// another, as KOReader sends what it queued offline: a replay is no sitting.
// It deletes the sessions the replay's first push started, which arrived
// before the rest showed it was one, and uses up document's pull, so a push
// hours later can't make a sitting of the time spent offline.
func (s *pullStore) replay(ctx context.Context, userID int, deviceID, document string, since time.Time) error {
	var started []*models.KOReaderPull
	err := s.db.NewSelect().
		Model(&started).
		Where("kp.device_id = ? AND kp.paired_at >= ? AND kp.session_id IS NOT NULL", deviceID, since).
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, pull := range started {
		if err := s.readingService.DeleteSession(ctx, userID, *pull.SessionID); err != nil {
			return err
		}
	}
	_, err = s.db.NewUpdate().
		Model((*models.KOReaderPull)(nil)).
		Set("session_id = NULL").
		Set("paired_at = COALESCE(paired_at, ?)", time.Now()).
		Where("device_id = ? AND (document = ? OR (paired_at >= ? AND session_id IS NOT NULL))", deviceID, document, since).
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
