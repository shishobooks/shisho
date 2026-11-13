// Package demo keeps the Public Demo's prepared database looking current.
package demo

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

// notShifted are date and time columns that don't record when something
// happened in Shisho, so they keep their value, each with the reason.
var notShifted = map[string]string{
	"bun_migrations.migrated_at":              "when a migration ran, which only the migrator reads",
	"files.file_modified_at":                  "the file's modification time on disk, which a scan compares against",
	"files.release_date":                      "the work's publication date",
	"kobo_sync_point_books.cover_modified_at": "the cover file's modification time on disk, which Kobo sync compares against",
}

type activityColumn struct {
	table, column string
}

// demoActivityColumns record reading itself. The latest of them decides the
// shift, so a bookkeeping time written after the reading, such as a
// migration's backfill or another user's row, can't.
var demoActivityColumns = []activityColumn{
	{"read_throughs", "begin_day"},
	{"read_throughs", "end_day"},
	{"reading_sessions", "ended_at"},
	{"reading_positions", "occurred_at"},
	{"reading_position_updates", "occurred_at"},
}

type dateTable struct {
	name    string
	columns []string
}

// dateColumns finds the columns the shift moves in the live schema: every
// column declared as a date or time, and the calendar days stored as text
// under a _day name, less notShifted.
func dateColumns(ctx context.Context, db bun.IDB) ([]dateTable, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT m.name, p.name, p.type FROM sqlite_master AS m, pragma_table_info(m.name) AS p
		WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite\_%' ESCAPE '\' AND m.sql NOT LIKE 'CREATE VIRTUAL%'
		ORDER BY m.name, p.cid`)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()
	var tables []dateTable
	for rows.Next() {
		var table, column, declared string
		if err := rows.Scan(&table, &column, &declared); err != nil {
			return nil, errors.WithStack(err)
		}
		declared = strings.ToUpper(declared)
		isDate := strings.Contains(declared, "DATE") || strings.Contains(declared, "TIME") || strings.HasSuffix(column, "_day")
		if !isDate || notShifted[table+"."+column] != "" {
			continue
		}
		if len(tables) == 0 || tables[len(tables)-1].name != table {
			tables = append(tables, dateTable{name: table})
		}
		tables[len(tables)-1].columns = append(tables[len(tables)-1].columns, column)
	}
	return tables, errors.WithStack(rows.Err())
}

// DateShifter keeps the Public Demo's dates current. The demo's Machine
// suspends when idle and resumes without restarting, so a shift at startup
// alone would leave the dates on the day it last booted.
type DateShifter struct {
	db  *bun.DB
	now func() time.Time

	mu sync.Mutex
	// day is the UTC day the shift last ran.
	day string
}

func NewDateShifter(db *bun.DB, now func() time.Time) *DateShifter {
	return &DateShifter{db: db, now: now}
}

// Refresh runs ShiftDates unless it already ran on this UTC day, and
// returns the days moved. Requests wait for it, so none sees the dates half
// moved. A failure isn't retried until the next day, so a broken shift
// costs one attempt a day rather than one a request.
func (s *DateShifter) Refresh(ctx context.Context) (int, error) {
	now := s.now()
	day := now.UTC().Format(time.DateOnly)
	s.mu.Lock()
	defer s.mu.Unlock()
	if day == s.day {
		return 0, nil
	}
	s.day = day
	return ShiftDates(ctx, s.db, now)
}

// ShiftDates moves every date in the database by the same whole number of
// days, so that the latest reading falls on the day before now in UTC, and
// returns the days moved. The Public Demo's database is prepared once, and
// this keeps its reading and the library around it looking current. The
// shift comes from the data, so a second run moves nothing.
//
// Only the leading YYYY-MM-DD of each value changes. Instants are stored in
// UTC, so that moves them by whole days exactly, and the rest of the text
// keeps the form queries compare against. A zero time (year 1), and
// anything that isn't a date, stays as it is.
func ShiftDates(ctx context.Context, db *bun.DB, now time.Time) (int, error) {
	var days int
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var latest sql.NullString
		if err := tx.NewRaw(latestReadingDaySQL()).Scan(ctx, &latest); err != nil {
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

		tables, err := dateColumns(ctx, tx)
		if err != nil {
			return err
		}
		modifier := fmt.Sprintf("%+d days", days)
		for _, table := range tables {
			sets := make([]string, len(table.columns))
			args := make([]any, len(table.columns))
			for i, column := range table.columns {
				// The year is compared as substr's text: against the column
				// itself, its numeric affinity would make '1000' a number.
				sets[i] = fmt.Sprintf(
					`"%[1]s" = CASE WHEN substr("%[1]s", 1, 4) < '1000' THEN "%[1]s" ELSE COALESCE(date(substr("%[1]s", 1, 10), ?) || substr("%[1]s", 11), "%[1]s") END`,
					column)
				args[i] = modifier
			}
			query := fmt.Sprintf(`UPDATE "%s" SET %s`, table.name, strings.Join(sets, ", "))
			if _, err := tx.ExecContext(ctx, query, args...); err != nil {
				return errors.Wrapf(err, "shifting %s", table.name)
			}
		}
		return nil
	})
	return days, errors.WithStack(err)
}

// latestReadingDaySQL selects the latest day the reading happened on.
func latestReadingDaySQL() string {
	parts := make([]string, len(demoActivityColumns))
	for i, c := range demoActivityColumns {
		parts[i] = fmt.Sprintf("SELECT MAX(substr(%s, 1, 10)) AS day FROM %s", c.column, c.table)
	}
	return "SELECT MAX(day) FROM (" + strings.Join(parts, " UNION ALL ") + ")"
}
