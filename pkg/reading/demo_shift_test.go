package reading

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// seedDemoReading writes one dated row into every reading table for a
// fresh user, with the latest activity at latest, and an underway
// Read-through whose end day is still empty.
func seedDemoReading(t *testing.T, db *bun.DB, latest time.Time) {
	t.Helper()
	ctx := context.Background()
	user := createUser(t, db, "demo", 2)
	book := createBook(t, db, "Demo")
	file := &models.File{
		LibraryID: book.LibraryID, BookID: book.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain,
		Filepath: book.Filepath + "/demo.epub", FilesizeBytes: 1,
	}
	_, err := db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	begun := latest.AddDate(0, 0, -20)
	rt := &models.ReadThrough{
		CreatedAt: begun, UpdatedAt: latest.Add(-time.Hour), UserID: user.ID, BookID: &book.ID,
		BeginDay: strp(begun.Format(time.DateOnly)), EndDay: strp(latest.Format(time.DateOnly)),
		Outcome: models.ReadThroughOutcomeFinished,
	}
	_, err = db.NewInsert().Model(rt).Exec(ctx)
	require.NoError(t, err)
	underway := seedReadThrough(t, db, user, createBook(t, db, "Underway"), begun.Format(time.DateOnly), "", models.ReadThroughOutcomeUnderway)
	underway.CreatedAt, underway.UpdatedAt = begun, begun
	_, err = db.NewUpdate().Model(underway).Column("created_at", "updated_at").WherePK().Exec(ctx)
	require.NoError(t, err)

	seedSession(t, db, user, sessionSpec{file: file, readThrough: rt, start: latest.Add(-45 * time.Minute), end: latest})
	seedSession(t, db, user, sessionSpec{file: file, readThrough: rt, start: begun, end: begun.Add(30 * time.Minute)})

	rating := 3
	_, err = db.NewInsert().Model(&models.UserBook{
		UserID: user.ID, BookID: book.ID, CreatedAt: begun.AddDate(0, 0, -5), UpdatedAt: latest.Add(-2 * time.Hour), Rating: &rating,
	}).Exec(ctx)
	require.NoError(t, err)

	cfi := "epubcfi(/6/4!/4/2/1:0)"
	locator := models.ReadingLocator{CFI: &cfi}
	_, err = db.NewInsert().Model(&models.ReadingPosition{
		UserID: user.ID, FileID: file.ID, CreatedAt: begun, UpdatedAt: latest.Add(-time.Minute),
		ReadingLocator: locator, Percentage: 1, Source: models.ReadingSourceWebReader, OccurredAt: latest.Add(-time.Minute),
	}).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.ReadingPositionUpdate{
		CreatedAt: latest.Add(-time.Minute), UserID: user.ID, FileID: &file.ID, ReadThroughID: &rt.ID,
		ReadingLocator: locator, Percentage: 1, Source: models.ReadingSourceWebReader, OccurredAt: latest.Add(-time.Minute),
	}).Exec(ctx)
	require.NoError(t, err)

	// A column default writes CURRENT_TIMESTAMP's own format, without a
	// fraction or an offset, so the shift has to keep that form too.
	_, err = db.ExecContext(ctx, `UPDATE read_throughs SET created_at = ? WHERE id = ?`,
		begun.UTC().Format(time.DateTime), rt.ID)
	require.NoError(t, err)
}

type dateCell struct {
	table, column string
	row           int
}

// readDemoDates returns the stored text of every non-null column the shift
// covers, keyed by table, column, and row.
func readDemoDates(t *testing.T, db *bun.DB) map[dateCell]string {
	t.Helper()
	cells := map[dateCell]string{}
	for _, table := range demoShiftTables {
		for _, column := range table.columns {
			rows, err := db.QueryContext(context.Background(),
				fmt.Sprintf(`SELECT CAST(%s AS TEXT) FROM %s ORDER BY rowid`, column, table.name))
			require.NoError(t, err)
			for i := 0; rows.Next(); i++ {
				var v sql.NullString
				require.NoError(t, rows.Scan(&v))
				if v.Valid {
					cells[dateCell{table.name, column, i}] = v.String
				}
			}
			require.NoError(t, rows.Err())
			require.NoError(t, rows.Close())
		}
	}
	return cells
}

func TestShiftDemoReadingDates_MovesEveryDateSoTheLatestActivityIsYesterday(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	seedDemoReading(t, db, time.Date(2026, 3, 10, 21, 30, 15, 250_000_000, time.UTC))
	before := readDemoDates(t, db)
	for _, table := range demoShiftTables {
		for _, column := range table.columns {
			assert.True(t, hasCell(before, table.name, column), "seed a value for %s.%s", table.name, column)
		}
	}

	now := time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC)
	days, err := ShiftDemoReadingDates(context.Background(), db, now)
	require.NoError(t, err)
	assert.Equal(t, 247, days)

	after := readDemoDates(t, db)
	require.Len(t, after, len(before), "an empty date stays empty")
	for cell, was := range before {
		got := after[cell]
		// The date moves and the rest of the text, the time of day and its
		// format, stays as it was.
		wasDay, err := time.Parse(time.DateOnly, was[:10])
		require.NoError(t, err)
		assert.Equal(t, wasDay.AddDate(0, 0, days).Format(time.DateOnly), got[:10], "%s.%s", cell.table, cell.column)
		assert.Equal(t, was[10:], got[10:], "%s.%s", cell.table, cell.column)
	}
	assert.Equal(t, "2026-11-12", latestDemoDay(after))
}

func TestShiftDemoReadingDates_ASecondRunMovesNothing(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	seedDemoReading(t, db, time.Date(2025, 12, 31, 23, 59, 0, 0, time.UTC))
	now := time.Date(2026, 11, 13, 0, 30, 0, 0, time.UTC)
	_, err := ShiftDemoReadingDates(context.Background(), db, now)
	require.NoError(t, err)
	shifted := readDemoDates(t, db)

	days, err := ShiftDemoReadingDates(context.Background(), db, now.Add(20*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 0, days)
	assert.Equal(t, shifted, readDemoDates(t, db))
}

func TestShiftDemoReadingDates_ReadingAfterYesterdayMovesBack(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	seedDemoReading(t, db, time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC))

	days, err := ShiftDemoReadingDates(context.Background(), db, time.Date(2026, 11, 13, 2, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, -1, days)
	assert.Equal(t, "2026-11-12", latestDemoDay(readDemoDates(t, db)))
}

// The latest reading decides the shift, not a bookkeeping time written
// after it, such as another user's row or a backfill by a later migration.
func TestShiftDemoReadingDates_LatestReadingDecidesTheShift(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)
	seedDemoReading(t, db, time.Date(2026, 3, 10, 21, 30, 0, 0, time.UTC))
	other := createUser(t, db, "admin", 1)
	_, err := db.NewInsert().Model(&models.UserBook{
		UserID: other.ID, BookID: createBook(t, db, "Curated").ID,
		CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	}).Exec(ctx)
	require.NoError(t, err)

	days, err := ShiftDemoReadingDates(ctx, db, time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, 247, days)
	var ended string
	require.NoError(t, db.NewRaw(`SELECT substr(MAX(ended_at), 1, 10) FROM reading_sessions`).Scan(ctx, &ended))
	assert.Equal(t, "2026-11-12", ended)
}

// A suspended Machine resumes without restarting, so the shift has to run
// again once the day changes, and only then.
func TestDemoDateShifter_ShiftsAgainOnceTheUTCDayChanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)
	seedDemoReading(t, db, time.Date(2026, 3, 10, 21, 30, 0, 0, time.UTC))
	now := time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC)
	shifter := NewDemoDateShifter(db, func() time.Time { return now })

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
	assert.Equal(t, "2026-11-13", latestDemoDay(readDemoDates(t, db)))
}

func TestShiftDemoReadingDates_NoReadingMovesNothing(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)

	days, err := ShiftDemoReadingDates(context.Background(), db, time.Now())
	require.NoError(t, err)
	assert.Equal(t, 0, days)
}

// notDemoReading are tables the coverage test finds that the demo never
// shows, each with the reason.
var notDemoReading = map[string]string{
	"koreader_pulls": "a KOReader handoff's state, and Demo Mode has no KOReader sync",
}

// Every date or time column in a reading table has to move with the rest,
// or the demo shows it months away from everything else. A reading table
// is one the shift lists, one named for reading, or one hanging off
// Read-throughs or Reading Sessions.
func TestShiftDemoReadingDates_CoversEveryDateColumnOfTheReadingTables(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)

	covered := map[string]map[string]bool{}
	for _, table := range demoShiftTables {
		covered[table.name] = map[string]bool{}
		for _, column := range table.columns {
			covered[table.name][column] = true
		}
	}

	var tables []string
	err := db.NewRaw(`
		SELECT m.name FROM sqlite_master AS m
		WHERE m.type = 'table' AND (
			m.name LIKE 'reading\_%' ESCAPE '\' OR m.name IN ('read_throughs', 'user_books')
			OR EXISTS (SELECT 1 FROM pragma_foreign_key_list(m.name) AS fk WHERE fk."table" IN ('read_throughs', 'reading_sessions'))
		)
		ORDER BY m.name`).Scan(ctx, &tables)
	require.NoError(t, err)
	for name := range covered {
		assert.Contains(t, tables, name, "the shift names a table the schema doesn't have")
	}

	for _, table := range tables {
		if notDemoReading[table] != "" {
			continue
		}
		var columns []struct {
			Name string `bun:"name"`
			Type string `bun:"type"`
		}
		err := db.NewRaw(`SELECT name, type FROM pragma_table_info(?)`, table).Scan(ctx, &columns)
		require.NoError(t, err)
		for _, c := range columns {
			if isDateColumn(c.Name, c.Type) {
				assert.True(t, covered[table][c.Name],
					"%s.%s holds a date or time, so add it to demoShiftTables", table, c.Name)
			}
		}
	}
}

func isDateColumn(name, declared string) bool {
	declared = strings.ToUpper(declared)
	return strings.Contains(declared, "DATE") || strings.Contains(declared, "TIME") ||
		strings.HasSuffix(name, "_at") || strings.HasSuffix(name, "_day")
}

func latestDemoDay(cells map[dateCell]string) string {
	latest := ""
	for _, v := range cells {
		latest = max(latest, v[:10])
	}
	return latest
}

func hasCell(cells map[dateCell]string, table, column string) bool {
	for cell := range cells {
		if cell.table == table && cell.column == column {
			return true
		}
	}
	return false
}
