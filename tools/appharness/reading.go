package main

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/sqliteconn"
)

// readingState is what the server holds of listening to one Book, for the
// flows to check against what the app showed. Times are seconds since 1970.
type readingState struct {
	// Seconds and Source are the Reading Position, Seconds -1 without one.
	Seconds    float64          `json:"seconds"`
	Source     string           `json:"source"`
	OccurredAt int64            `json:"occurred_at"`
	Sessions   []readingSession `json:"sessions"`
	// ReadThrough is the latest Read-through's outcome, or empty.
	ReadThrough string `json:"read_through"`
	// OfflineAt and OnlineAt are when a flow last took the server offline
	// and brought it back, or 0.
	OfflineAt int64 `json:"offline_at"`
	OnlineAt  int64 `json:"online_at"`
}

type readingSession struct {
	StartedAt int64  `json:"started_at"`
	EndedAt   int64  `json:"ended_at"`
	Duration  int    `json:"duration"`
	Source    string `json:"source"`
	// Device is the name of the Device that sent it.
	Device string `json:"device"`
}

func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// readReading reads the harness user's listening to the Book titled title
// from the server's database, read-only.
func readReading(ctx context.Context, path, title string) (*readingState, error) {
	connector, err := sqliteconn.NewConnector("file:"+path+"?mode=ro", "PRAGMA busy_timeout = 5000")
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	defer db.Close()

	state := &readingState{Seconds: -1, Sessions: []readingSession{}}
	err = db.QueryRowContext(ctx, `
		SELECT COALESCE(rp.seconds, -1), rp.source, CAST(strftime('%s', rp.occurred_at) AS INTEGER)
		FROM reading_positions rp JOIN files f ON f.id = rp.file_id JOIN books b ON b.id = f.book_id
		WHERE b.title = ?`, title).Scan(&state.Seconds, &state.Source, &state.OccurredAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.WithStack(err)
	}
	rows, err := db.QueryContext(ctx, `
		SELECT CAST(strftime('%s', rs.started_at) AS INTEGER), CAST(strftime('%s', rs.ended_at) AS INTEGER),
			rs.duration, rs.source, COALESCE(d.name, '')
		FROM reading_sessions rs JOIN files f ON f.id = rs.file_id JOIN books b ON b.id = f.book_id
		LEFT JOIN devices d ON d.id = rs.source_device_id
		WHERE b.title = ? ORDER BY rs.started_at`, title)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()
	for rows.Next() {
		var session readingSession
		if err := rows.Scan(&session.StartedAt, &session.EndedAt, &session.Duration, &session.Source, &session.Device); err != nil {
			return nil, errors.WithStack(err)
		}
		state.Sessions = append(state.Sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	err = db.QueryRowContext(ctx, `
		SELECT rt.outcome FROM read_throughs rt JOIN books b ON b.id = rt.book_id
		WHERE b.title = ? ORDER BY rt.id DESC LIMIT 1`, title).Scan(&state.ReadThrough)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.WithStack(err)
	}
	return state, nil
}
