package reading

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

type demoShiftTable struct {
	name    string
	columns []string
}

// demoShiftTables lists every date and time column in the reading tables.
// A test reads the schema and fails when a reading table gains one this
// list misses.
var demoShiftTables = []demoShiftTable{
	{"read_throughs", []string{"created_at", "updated_at", "begin_day", "end_day"}},
	{"reading_sessions", []string{"created_at", "started_at", "ended_at"}},
	{"reading_positions", []string{"created_at", "updated_at", "occurred_at"}},
	{"reading_position_updates", []string{"created_at", "occurred_at"}},
	{"user_books", []string{"created_at", "updated_at"}},
}

// demoActivityColumns are the columns that record reading itself. The
// latest of them decides the shift, so a bookkeeping time written after the
// reading, such as a migration's backfill or another user's row, can't.
var demoActivityColumns = []demoShiftTable{
	{"read_throughs", []string{"begin_day", "end_day"}},
	{"reading_sessions", []string{"ended_at"}},
	{"reading_positions", []string{"occurred_at"}},
	{"reading_position_updates", []string{"occurred_at"}},
}

// DemoDateShifter keeps the Public Demo's reading current. The demo's
// Machine suspends when idle and resumes without restarting, so a shift at
// startup alone would leave the reading on the day it last booted.
type DemoDateShifter struct {
	db  *bun.DB
	now func() time.Time

	mu sync.Mutex
	// day is the UTC day the shift last ran.
	day string
}

func NewDemoDateShifter(db *bun.DB, now func() time.Time) *DemoDateShifter {
	return &DemoDateShifter{db: db, now: now}
}

// Refresh runs ShiftDemoReadingDates unless it already ran on this UTC day,
// and returns the days moved. Requests wait for it, so none sees the
// reading half moved. A failure isn't retried until the next day, so a
// broken shift costs one attempt a day rather than one a request.
func (s *DemoDateShifter) Refresh(ctx context.Context) (int, error) {
	now := s.now()
	day := now.UTC().Format(time.DateOnly)
	s.mu.Lock()
	defer s.mu.Unlock()
	if day == s.day {
		return 0, nil
	}
	s.day = day
	return ShiftDemoReadingDates(ctx, s.db, now)
}

// ShiftDemoReadingDates moves every date in the reading tables by the same
// whole number of days, so that the latest reading falls on the day before
// now in UTC, and returns the days moved. The Public Demo's database is
// prepared once, and this keeps the reading in it looking current. The
// shift comes from the data, so a second run moves nothing.
//
// Only the leading YYYY-MM-DD of each value changes. Instants are stored in
// UTC, so that moves them by whole days exactly, and the rest of the text
// keeps the form the stats queries compare against.
func ShiftDemoReadingDates(ctx context.Context, db *bun.DB, now time.Time) (int, error) {
	var days int
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var latest sql.NullString
		if err := tx.NewRaw(latestDemoReadingDaySQL()).Scan(ctx, &latest); err != nil {
			return errors.WithStack(err)
		}
		if !latest.Valid {
			return nil
		}
		latestDay, err := time.Parse(time.DateOnly, latest.String)
		if err != nil {
			return errors.Wrapf(err, "reading the latest reading day %q", latest.String)
		}
		yesterday := now.UTC().AddDate(0, 0, -1)
		target := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
		days = int(target.Sub(latestDay).Hours() / 24)
		if days == 0 {
			return nil
		}

		modifier := fmt.Sprintf("%+d days", days)
		for _, table := range demoShiftTables {
			sets := make([]string, len(table.columns))
			for i, column := range table.columns {
				sets[i] = fmt.Sprintf("%[1]s = date(substr(%[1]s, 1, 10), ?) || substr(%[1]s, 11)", column)
			}
			args := make([]any, len(table.columns))
			for i := range args {
				args[i] = modifier
			}
			query := fmt.Sprintf("UPDATE %s SET %s", table.name, strings.Join(sets, ", "))
			if _, err := tx.ExecContext(ctx, query, args...); err != nil {
				return errors.Wrapf(err, "shifting %s", table.name)
			}
		}
		return nil
	})
	return days, errors.WithStack(err)
}

// latestDemoReadingDaySQL selects the latest day the reading happened on.
func latestDemoReadingDaySQL() string {
	var parts []string
	for _, table := range demoActivityColumns {
		for _, column := range table.columns {
			parts = append(parts, fmt.Sprintf("SELECT MAX(substr(%s, 1, 10)) AS day FROM %s", column, table.name))
		}
	}
	return "SELECT MAX(day) FROM (" + strings.Join(parts, " UNION ALL ") + ")"
}
