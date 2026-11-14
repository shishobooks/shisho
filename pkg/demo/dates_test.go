package demo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// seedDemo writes a library, a Book with a File, and one user's reading of
// it, with the latest reading at latest and everything else before it. It
// leaves an underway Read-through's end day empty, a zero time in a column
// without nullzero, and values in the columns the shift leaves alone.
func seedDemo(t *testing.T, db *bun.DB, latest time.Time) {
	t.Helper()
	ctx := context.Background()
	exec := func(query string, args ...any) {
		t.Helper()
		_, err := db.ExecContext(ctx, query, args...)
		require.NoError(t, err)
	}
	added := latest.AddDate(0, -14, 0)
	begun := latest.AddDate(0, 0, -20)
	at := func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05.999999-07:00") }

	exec(`INSERT INTO users (id, username, password_hash, role_id, is_active, created_at, updated_at) VALUES (90, 'demo', 'x', 2, TRUE, ?, ?)`, at(added), at(added))
	exec(`INSERT INTO libraries (id, name, cover_aspect_ratio, created_at, updated_at) VALUES (90, 'Demo', 'book', ?, ?)`, at(added), at(added))
	// A zero time, as Bun writes for a time.Time without nullzero.
	exec(`INSERT INTO library_paths (library_id, filepath, created_at, updated_at) VALUES (90, '/media', ?, '0001-01-01 00:00:00+00:00')`, at(added))
	exec(`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source, author_source, created_at, updated_at)
		VALUES (90, 90, '/media/Demo', 'Demo', 'filepath', 'Demo', 'filepath', 'filepath', ?, ?)`, at(added), at(added))
	exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, file_role, filesize_bytes, created_at, updated_at, file_modified_at, release_date)
		VALUES (90, 90, 90, '/media/Demo/demo.epub', 'epub', 'main', 1, ?, ?, '2020-05-01 10:00:00+00:00', '1900-01-01 00:00:00+00:00')`, at(added), at(added))

	exec(`INSERT INTO read_throughs (id, created_at, updated_at, user_id, book_id, begin_day, end_day, outcome) VALUES (90, ?, ?, 90, 90, ?, ?, 'finished')`,
		at(begun), at(latest.Add(-time.Hour)), begun.Format(time.DateOnly), latest.Format(time.DateOnly))
	exec(`INSERT INTO read_throughs (id, created_at, updated_at, user_id, book_id, begin_day, outcome) VALUES (91, ?, ?, 90, 90, ?, 'underway')`,
		at(begun), at(begun), begun.Format(time.DateOnly))
	// A column default writes CURRENT_TIMESTAMP's own form, without a
	// fraction or an offset, so the shift keeps that form too.
	exec(`UPDATE read_throughs SET created_at = ? WHERE id = 90`, begun.UTC().Format(time.DateTime))
	for i, start := range []time.Time{begun, latest.Add(-45 * time.Minute)} {
		exec(`INSERT INTO reading_sessions (created_at, user_id, file_id, read_through_id, source, started_at, ended_at, start_percentage, end_percentage, measured, kind, duration)
			VALUES (?, 90, 90, 90, 'web_reader', ?, ?, ?, ?, TRUE, 'reading', 2700)`,
			at(start.Add(45*time.Minute)), at(start), at(start.Add(45*time.Minute)), float64(i)/2, float64(i+1)/2)
	}
	exec(`INSERT INTO user_books (user_id, book_id, created_at, updated_at, rating) VALUES (90, 90, ?, ?, 16)`, at(begun), at(latest.Add(-2*time.Hour)))
	exec(`INSERT INTO reading_positions (user_id, file_id, created_at, updated_at, cfi, percentage, source, occurred_at) VALUES (90, 90, ?, ?, 'epubcfi(/6/4!/4)', 1, 'web_reader', ?)`,
		at(begun), at(latest.Add(-time.Minute)), at(latest.Add(-time.Minute)))
	exec(`INSERT INTO reading_position_updates (created_at, user_id, file_id, read_through_id, cfi, percentage, source, occurred_at) VALUES (?, 90, 90, 90, 'epubcfi(/6/4!/4)', 1, 'web_reader', ?)`,
		at(latest.Add(-time.Minute)), at(latest.Add(-time.Minute)))
}

type cell struct {
	table, column string
	row           int
}

// readDates returns the stored text of every non-null date or time value
// in the database, keyed by table, column, and row.
func readDates(t *testing.T, db *bun.DB) map[cell]string {
	t.Helper()
	ctx := context.Background()
	cells := map[cell]string{}
	for _, c := range dateLikeColumns(t, db) {
		rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT CAST("%s" AS TEXT) FROM "%s" ORDER BY rowid`, c.column, c.table))
		require.NoError(t, err)
		for i := 0; rows.Next(); i++ {
			var v sql.NullString
			require.NoError(t, rows.Scan(&v))
			if v.Valid {
				cells[cell{c.table, c.column, i}] = v.String
			}
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
	}
	return cells
}

type schemaColumn struct {
	table, column, declared string
}

// dateLikeColumns is every column that looks like it holds a date or time,
// by its declared type or its name, a wider net than the shift's own.
func dateLikeColumns(t *testing.T, db *bun.DB) []schemaColumn {
	t.Helper()
	var columns []schemaColumn
	rows, err := db.QueryContext(context.Background(), `
		SELECT m.name, p.name, p.type FROM sqlite_master AS m, pragma_table_info(m.name) AS p
		WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite\_%' ESCAPE '\' AND m.sql NOT LIKE 'CREATE VIRTUAL%'
		ORDER BY m.name, p.cid`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var c schemaColumn
		require.NoError(t, rows.Scan(&c.table, &c.column, &c.declared))
		declared := strings.ToUpper(c.declared)
		if strings.Contains(declared, "DATE") || strings.Contains(declared, "TIME") ||
			strings.HasSuffix(c.column, "_at") || strings.HasSuffix(c.column, "_day") || strings.HasSuffix(c.column, "_date") {
			columns = append(columns, c)
		}
	}
	require.NoError(t, rows.Err())
	return columns
}

func latestReadingDay(t *testing.T, db *bun.DB) string {
	t.Helper()
	var day string
	require.NoError(t, db.NewRaw(`SELECT substr(MAX(ended_at), 1, 10) FROM reading_sessions`).Scan(context.Background(), &day))
	return day
}

func TestShiftDates_MovesEveryDateSoTheLatestReadingIsYesterday(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	seedDemo(t, db, time.Date(2026, 3, 10, 21, 30, 15, 250_000_000, time.UTC))
	before := readDates(t, db)

	days, err := ShiftDates(context.Background(), db, time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, 247, days)

	after := readDates(t, db)
	require.Len(t, after, len(before), "an empty date stays empty")
	for c, was := range before {
		got := after[c]
		name := c.table + "." + c.column
		// Excluded columns, zero times, and rows written after the reading
		// (here, the ones migrations stamp when the test database is built)
		// keep their value.
		if notShifted[name] != "" || strings.HasPrefix(was, "0001-") || was[:10] > "2026-03-10" {
			assert.Equal(t, was, got, "%s stays as it was", name)
			continue
		}
		// The date moves and the rest of the text, the time of day and its
		// form, stays as it was.
		wasDay, err := time.Parse(time.DateOnly, was[:10])
		require.NoError(t, err, name)
		assert.Equal(t, wasDay.AddDate(0, 0, days).Format(time.DateOnly), got[:10], name)
		assert.Equal(t, was[10:], got[10:], name)
	}
	for _, name := range []string{"books.created_at", "files.created_at", "libraries.created_at", "users.created_at", "user_books.updated_at", "reading_positions.occurred_at"} {
		table, column, _ := strings.Cut(name, ".")
		found := false
		for c := range before {
			found = found || (c.table == table && c.column == column)
		}
		assert.True(t, found, "seed a value for %s", name)
	}
	assert.Equal(t, "2026-11-12", latestReadingDay(t, db))

	// A publication date and a time read off the disk aren't history.
	var released, modified string
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT release_date, file_modified_at FROM files WHERE id = 90`).Scan(&released, &modified))
	assert.Equal(t, "1900-01-01 00:00:00+00:00", released)
	assert.Equal(t, "2020-05-01 10:00:00+00:00", modified)
}

func TestShiftDates_ASecondRunMovesNothing(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	seedDemo(t, db, time.Date(2025, 12, 31, 23, 59, 0, 0, time.UTC))
	now := time.Date(2026, 11, 13, 0, 30, 0, 0, time.UTC)
	_, err := ShiftDates(context.Background(), db, now)
	require.NoError(t, err)
	shifted := readDates(t, db)

	days, err := ShiftDates(context.Background(), db, now.Add(20*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 0, days)
	assert.Equal(t, shifted, readDates(t, db))
}

func TestShiftDates_ReadingAfterYesterdayMovesBack(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	seedDemo(t, db, time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC))

	days, err := ShiftDates(context.Background(), db, time.Date(2026, 11, 13, 2, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, -1, days)
	assert.Equal(t, "2026-11-12", latestReadingDay(t, db))
}

func TestShiftDates_NoReadingMovesNothing(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)

	days, err := ShiftDates(context.Background(), db, time.Now())
	require.NoError(t, err)
	assert.Equal(t, 0, days)
}

// The latest reading decides the shift, not a bookkeeping time written
// after it, such as another user's row or a backfill by a later migration.
func TestShiftDates_LatestReadingDecidesTheShift(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	seedDemo(t, db, time.Date(2026, 3, 10, 21, 30, 0, 0, time.UTC))
	_, err := db.ExecContext(context.Background(), `UPDATE books SET updated_at = '2026-09-01 00:00:00+00:00'`)
	require.NoError(t, err)

	days, err := ShiftDates(context.Background(), db, time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, 247, days)
	assert.Equal(t, "2026-11-12", latestReadingDay(t, db))

	// Written after the reading, it is already about as recent as anything
	// in the demo, and moving it with the reading would put it in the future.
	var updated string
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT updated_at FROM books WHERE id = 90`).Scan(&updated))
	assert.Equal(t, "2026-09-01 00:00:00+00:00", updated)
}

// A suspended Machine resumes without restarting, so the shift has to run
// again once the day changes, and only then.
func TestDateShifter_ShiftsAgainOnceTheUTCDayChanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)
	seedDemo(t, db, time.Date(2026, 3, 10, 21, 30, 0, 0, time.UTC))
	now := time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC)
	shifter := NewDateShifter(db, func() time.Time { return now })

	days, err := shifter.Refresh(ctx)
	require.NoError(t, err)
	assert.Equal(t, 247, days)

	// Reading that lands after yesterday would move everything back, but
	// the shift already ran today, so a later request today doesn't run it.
	_, err = db.ExecContext(ctx, `UPDATE reading_sessions SET started_at = '2026-11-14 20:00:00+00:00', ended_at = '2026-11-14 21:00:00+00:00' WHERE id = (SELECT MAX(id) FROM reading_sessions)`)
	require.NoError(t, err)
	now = now.Add(19 * time.Hour)
	days, err = shifter.Refresh(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, days)

	// The first request of the next day does.
	now = now.Add(2 * time.Hour)
	days, err = shifter.Refresh(ctx)
	require.NoError(t, err)
	assert.Equal(t, -1, days)
	assert.Equal(t, "2026-11-13", latestReadingDay(t, db))
}

// Every column that holds a date or time either moves with the rest or is
// listed in notShifted with the reason, so a new one can't be left months
// away from everything else. This casts a wider net than the shift finds
// by declared type, to catch a date stored under another type.
func TestShiftDates_EveryDateColumnIsShiftedOrExcluded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)

	tables, err := dateColumns(ctx, db)
	require.NoError(t, err)
	shifted := map[string]bool{}
	for _, table := range tables {
		for _, column := range table.columns {
			shifted[table.name+"."+column] = true
		}
	}

	all := map[string]bool{}
	for _, c := range dateLikeColumns(t, db) {
		name := c.table + "." + c.column
		all[name] = true
		if notShifted[name] != "" {
			assert.False(t, shifted[name], "%s is excluded but still shifted", name)
			continue
		}
		assert.True(t, shifted[name],
			"%s looks like a date or time but the shift doesn't move it: declare it TIMESTAMP, or add it to notShifted with the reason", name)
	}
	for name := range notShifted {
		assert.True(t, all[name], "notShifted names %s, which the schema doesn't have", name)
	}
	for _, c := range demoActivityColumns {
		assert.True(t, shifted[c.table+"."+c.column], "the shift is anchored on %s.%s, so it must move", c.table, c.column)
	}
}
