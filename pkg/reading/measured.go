package reading

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// SessionGap is the longest stretch without reading inside one measured
// Reading Session. A longer one starts the next session.
const SessionGap = 10 * time.Minute

// maxRowDuration is the longest one page visit is taken to last. KOReader
// leaves time asleep out, so a longer one is a clock gone wrong.
const maxRowDuration = 24 * time.Hour

// PageRow is one visit to one page that a device timed, as KOReader's
// statistics record it (ADR 0016).
type PageRow struct {
	Document string
	// Page is 1-based, out of TotalPages in the layout it was read in.
	Page       int
	TotalPages int
	// StartTime is the device's own Unix time, part of the row's key, and
	// StartedAt the instant after correcting the device's clock.
	StartTime int64
	StartedAt time.Time
	Duration  int
}

// RecordPageRows takes in rows of one document from deviceID for userID.
// Rows already taken in are ignored, including ones behind a session or
// Read-through the user has since deleted. When the document is a File
// (fileID), the new rows are grouped into measured Reading Sessions: a gap
// of more than SessionGap starts a new one, and rows next to a session from
// an earlier upload extend it. A new session is placed by its time: on the
// Read-through underway when it started, or none. One that's real progress
// in a Book never read, or on To Read, starts a Read-through on its own day,
// except in history: rows the device recorded before it began syncing,
// whose Read-throughs come only from its statuses (RecordHistoryStatus).
// Estimated sessions deviceID recorded in the File are deleted where a
// measured one overlaps them, so the same time isn't counted twice. Rows of
// a document Shisho doesn't recognize (fileID nil) are stored ungrouped and
// count toward nothing until something groups them.
func (svc *Service) RecordPageRows(ctx context.Context, userID int, deviceID string, fileID *int, rows []PageRow, history bool) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		now := svc.now()
		var added []*models.KOReaderPageRow
		for _, r := range rows {
			row := &models.KOReaderPageRow{
				CreatedAt:  now,
				UserID:     userID,
				DeviceID:   deviceID,
				Document:   r.Document,
				Page:       r.Page,
				TotalPages: r.TotalPages,
				StartTime:  r.StartTime,
				StartedAt:  r.StartedAt.UTC(),
				Duration:   r.Duration,
				FileID:     fileID,
			}
			res, err := tx.NewInsert().Model(row).On("CONFLICT DO NOTHING").Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
			if n, err := res.RowsAffected(); err != nil {
				return errors.WithStack(err)
			} else if n == 1 {
				added = append(added, row)
			}
		}
		if fileID == nil || len(added) == 0 {
			return nil
		}
		var sessions []*models.ReadingSession
		for _, cluster := range clusterRows(added) {
			session, err := groupCluster(ctx, tx, userID, deviceID, *fileID, cluster, !history, now)
			if err != nil {
				return err
			}
			if session == nil {
				continue
			}
			if err := replaceEstimates(ctx, tx, userID, deviceID, *fileID, session); err != nil {
				return err
			}
			sessions = append(sessions, session)
		}
		return placeUnattached(ctx, tx, userID, *fileID, sessions)
	})
}

// replaceEstimates deletes the estimated Reading Sessions deviceID recorded
// in fileID, as KOReader's progress sync does, whose spans overlap session,
// which measured the same time. One that only touches it shares no time.
func replaceEstimates(ctx context.Context, tx bun.Tx, userID int, deviceID string, fileID int, session *models.ReadingSession) error {
	_, err := tx.NewDelete().
		Model((*models.ReadingSession)(nil)).
		Where("user_id = ? AND file_id = ? AND source_device_id = ? AND NOT measured", userID, fileID, deviceID).
		Where("started_at < ? AND ended_at > ?", session.EndedAt, session.StartedAt).
		Exec(ctx)
	return errors.WithStack(err)
}

// placeUnattached places again each of sessions on no Read-through, since a
// later session in the same upload may have started one that covers it, as
// reading in earnest an hour after opening a book on its first page does.
func placeUnattached(ctx context.Context, tx bun.Tx, userID, fileID int, sessions []*models.ReadingSession) error {
	var bookID int
	for _, session := range sessions {
		if session.ReadThroughID != nil {
			continue
		}
		if bookID == 0 {
			err := tx.NewSelect().Model((*models.File)(nil)).Column("f.book_id").Where("f.id = ?", fileID).Scan(ctx, &bookID)
			if err != nil {
				return errors.WithStack(err)
			}
		}
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

// clusterRows splits rows into runs with no gap longer than SessionGap
// between one row's end and the next row's start, in order.
func clusterRows(rows []*models.KOReaderPageRow) [][]*models.KOReaderPageRow {
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].StartedAt.Before(rows[j].StartedAt) })
	var clusters [][]*models.KOReaderPageRow
	var end time.Time
	for _, row := range rows {
		if len(clusters) == 0 || row.StartedAt.Sub(end) > SessionGap {
			clusters = append(clusters, nil)
			end = row.EndedAt()
		}
		clusters[len(clusters)-1] = append(clusters[len(clusters)-1], row)
		if row.EndedAt().After(end) {
			end = row.EndedAt()
		}
	}
	return clusters
}

// groupCluster makes cluster, new rows with no long gap between them, part
// of a measured session: the earliest one from deviceID in fileID within
// SessionGap of it, or a new one. Sessions the cluster joins become one. A
// cluster within SessionGap of a session the user deleted is the rest of
// that sitting, so it's taken in as deleted too. A new session may start a
// Read-through only when startsRead. It returns the session, or nil for a
// deleted one.
func groupCluster(ctx context.Context, tx bun.Tx, userID int, deviceID string, fileID int, cluster []*models.KOReaderPageRow, startsRead bool, now time.Time) (*models.ReadingSession, error) {
	start, end := cluster[0].StartedAt, cluster[0].EndedAt()
	ids := make([]int, 0, len(cluster))
	for _, row := range cluster {
		if row.EndedAt().After(end) {
			end = row.EndedAt()
		}
		ids = append(ids, row.ID)
	}

	deleted, err := nearDeletedSession(ctx, tx, deviceID, fileID, start, end)
	if err != nil {
		return nil, err
	}
	if deleted {
		_, err := tx.NewUpdate().
			Model((*models.KOReaderPageRow)(nil)).
			Set("grouped = TRUE").
			Where("id IN (?)", bun.List(ids)).
			Exec(ctx)
		return nil, errors.WithStack(err)
	}

	var near []*models.ReadingSession
	err = tx.NewSelect().
		Model(&near).
		Where("rs.user_id = ? AND rs.file_id = ? AND rs.source_device_id = ?", userID, fileID, deviceID).
		Where("rs.measured AND rs.source = ?", models.ReadingSourceKOReader).
		Where("rs.started_at <= ? AND rs.ended_at >= ?", end.Add(SessionGap), start.Add(-SessionGap)).
		OrderExpr("rs.started_at ASC, rs.id ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var session *models.ReadingSession
	if len(near) == 0 {
		session, err = newMeasuredSession(ctx, tx, userID, deviceID, fileID, cluster, start, startsRead, now)
		if err != nil {
			return nil, err
		}
	} else {
		session = near[0]
		// A cluster bridging two sessions joins them: the later ones' rows
		// move to the first before they go, so they don't read as deleted.
		for _, other := range near[1:] {
			_, err := tx.NewUpdate().
				Model((*models.KOReaderPageRow)(nil)).
				Set("reading_session_id = ?", session.ID).
				Where("reading_session_id = ?", other.ID).
				Exec(ctx)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			if _, err := tx.NewDelete().Model(other).WherePK().Exec(ctx); err != nil {
				return nil, errors.WithStack(err)
			}
		}
	}

	_, err = tx.NewUpdate().
		Model((*models.KOReaderPageRow)(nil)).
		Set("grouped = TRUE").
		Set("reading_session_id = ?", session.ID).
		Where("id IN (?)", bun.List(ids)).
		Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return session, measureSession(ctx, tx, session)
}

// nearDeletedSession reports whether rows from deviceID in fileID that
// belonged to a session the user deleted lie within SessionGap of the span
// from start to end.
func nearDeletedSession(ctx context.Context, tx bun.Tx, deviceID string, fileID int, start, end time.Time) (bool, error) {
	var rows []*models.KOReaderPageRow
	// A row's end is its start plus its duration, so rows are looked for
	// from a day before, the longest a page visit can last.
	err := tx.NewSelect().
		Model(&rows).
		Where("kpr.device_id = ? AND kpr.file_id = ? AND kpr.grouped AND kpr.reading_session_id IS NULL", deviceID, fileID).
		Where("kpr.started_at >= ? AND kpr.started_at <= ?", start.Add(-SessionGap-maxRowDuration), end.Add(SessionGap)).
		Scan(ctx)
	if err != nil {
		return false, errors.WithStack(err)
	}
	for _, row := range rows {
		if !row.EndedAt().Before(start.Add(-SessionGap)) {
			return true, nil
		}
	}
	return false, nil
}

// newMeasuredSession records the session cluster begins, placed by start,
// and, when startsRead, starts a Read-through for it when it's real
// progress in a Book reading would start.
func newMeasuredSession(ctx context.Context, tx bun.Tx, userID int, deviceID string, fileID int, cluster []*models.KOReaderPageRow, start time.Time, startsRead bool, now time.Time) (*models.ReadingSession, error) {
	file := &models.File{}
	err := tx.NewSelect().Model(file).Column("f.book_id", "f.file_type", "f.file_role").Where("f.id = ?", fileID).Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	rt, err := readThroughAt(ctx, tx, userID, file.BookID, start)
	if err != nil {
		return nil, err
	}
	if rt == nil && startsRead && file.FileRole != models.FileRoleSupplement && pastFirstPage(cluster) {
		starts, err := startsOnProgress(ctx, tx, userID, file.BookID)
		if err != nil {
			return nil, err
		}
		underway, err := retrieveUnderway(ctx, tx, userID, file.BookID)
		if err != nil {
			return nil, err
		}
		if starts && underway == nil {
			day, err := userDay(ctx, tx, userID, start)
			if err != nil {
				return nil, err
			}
			rt, err = startReadThrough(ctx, tx, userID, file.BookID, day, now, fileID)
			if err != nil {
				return nil, err
			}
		}
	}
	session := &models.ReadingSession{
		CreatedAt:      now,
		UserID:         userID,
		FileID:         &fileID,
		ReadThroughID:  readThroughID(rt),
		Source:         models.ReadingSourceKOReader,
		StartedAt:      start,
		EndedAt:        start,
		Measured:       true,
		Kind:           models.ReadingKindForFileType(file.FileType),
		SourceDeviceID: &deviceID,
	}
	if _, err := tx.NewInsert().Model(session).Exec(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	return session, nil
}

// pastFirstPage reports whether rows read anything past a book's first
// page, which opening it doesn't.
func pastFirstPage(rows []*models.KOReaderPageRow) bool {
	for _, row := range rows {
		if row.Page > 1 {
			return true
		}
	}
	return false
}

// measureSession sets session's span, time, and percentages from its rows:
// it runs from the first row's start to the last row's end, its duration is
// the sum of theirs, and its percentages are the first and last rows' pages
// out of their total pages.
func measureSession(ctx context.Context, tx bun.Tx, session *models.ReadingSession) error {
	var rows []*models.KOReaderPageRow
	err := tx.NewSelect().
		Model(&rows).
		Where("kpr.reading_session_id = ?", session.ID).
		OrderExpr("kpr.started_at ASC, kpr.id ASC").
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	if len(rows) == 0 {
		return nil
	}
	session.StartedAt = rows[0].StartedAt
	session.EndedAt = rows[0].EndedAt()
	session.Duration = 0
	for _, row := range rows {
		if row.EndedAt().After(session.EndedAt) {
			session.EndedAt = row.EndedAt()
		}
		session.Duration += row.Duration
	}
	session.StartPercentage = pagePercentage(rows[0])
	session.EndPercentage = pagePercentage(rows[len(rows)-1])
	_, err = tx.NewUpdate().
		Model(session).
		Column("started_at", "ended_at", "duration", "start_percentage", "end_percentage").
		WherePK().
		Exec(ctx)
	return errors.WithStack(err)
}

func pagePercentage(row *models.KOReaderPageRow) float64 {
	if row.TotalPages <= 0 {
		return 0
	}
	return min(1, max(0, float64(row.Page)/float64(row.TotalPages)))
}
