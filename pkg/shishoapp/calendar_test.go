package shishoapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// calendarFixture is a reading fixture with an hour of listening to Dune
// (file 5) and half an hour of Ivanhoe (file 7) yesterday, Emma finished,
// and Ivanhoe since deleted.
type calendarFixture struct {
	*readingFixture
	start time.Time
}

func newCalendarFixture(t *testing.T) *calendarFixture {
	t.Helper()
	f := newReadingFixture(t)
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, audiobook_duration_seconds)
		VALUES (5, 1, 1, '/dune.m4b', 'm4b', 1, 6000), (7, 1, 3, '/ivanhoe.m4b', 'm4b', 1, 6000)`)
	// Yesterday morning, so both Sessions fall on one day whenever the test
	// runs, and within the account's age.
	y, m, d := f.now.Date()
	start := time.Date(y, m, d-1, 10, 0, 0, 0, time.UTC)
	ivanhoe := session(start.Add(time.Hour), start.Add(90*time.Minute))
	ivanhoe["file_id"] = 7
	results := f.send(
		event("dune", JournalEventReadingSession, start.Add(time.Hour), session(start, start.Add(time.Hour))),
		event("ivanhoe", JournalEventReadingSession, start.Add(90*time.Minute), ivanhoe),
		event("emma", JournalEventReadThroughFinish, start, map[string]any{"book_id": 2, "day": start.Format(time.DateOnly)}),
	)
	require.Equal(t, []string{"dune:accepted", "ivanhoe:accepted", "emma:accepted"}, results)
	require.NoError(t, reading.DetachBooks(f.ctx, f.db, []int{3}))
	f.exec(`DELETE FROM books WHERE id = 3`)
	return &calendarFixture{readingFixture: f, start: start}
}

func (f *calendarFixture) get(path string, into any) {
	f.t.Helper()
	rec := f.do(call{path: path, credential: f.credential})
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), into))
}

// refByTitle finds the Book named title among refs.
func refByTitle(t *testing.T, refs []StatsBookRef, title string) StatsBookRef {
	t.Helper()
	for _, ref := range refs {
		if ref.Title == title {
			return ref
		}
	}
	require.Failf(t, "no Book named "+title, "%+v", refs)
	return StatsBookRef{}
}

// The server offers the calendar's routes.
func TestCapabilities_ListReadingCalendar(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	rec := f.do(call{path: "/mobile/server", protocol: "1"})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, strings.Split(rec.Header().Get(HeaderCapabilities), ","), CapabilityReadingCalendar)
}

// The heatmap route sends the last 12 months, or a year, a day at a time.
func TestStatsHeatmap_SendsEachDaysTime(t *testing.T) {
	t.Parallel()
	f := newCalendarFixture(t)
	var resp StatsHeatmapResponse
	f.get("/mobile/stats/heatmap", &resp)
	assert.Equal(t, resp.Today, resp.To)
	require.NotNil(t, resp.FirstDay)
	assert.Equal(t, f.start.Format(time.DateOnly), *resp.FirstDay)
	total := 0
	for _, day := range resp.Days {
		total += day.ListeningSeconds + day.ReadingSeconds
	}
	assert.Equal(t, 90*60, total)

	var year StatsHeatmapResponse
	f.get(fmt.Sprintf("/mobile/stats/heatmap?year=%d", f.now.Year()), &year)
	assert.Equal(t, fmt.Sprintf("%d-01-01", f.now.Year()), year.From)
	assert.Equal(t, fmt.Sprintf("%d-12-31", f.now.Year()), year.To)
}

// The calendar route sends a month's Books and days, and a deleted Book
// keeps its title but no longer names a Book.
func TestStatsCalendar_SendsTheMonthsBooks(t *testing.T) {
	t.Parallel()
	f := newCalendarFixture(t)
	var resp StatsCalendarResponse
	f.get(fmt.Sprintf("/mobile/stats/calendar?year=%d&month=%d", f.start.Year(), int(f.start.Month())), &resp)
	assert.Equal(t, f.start.Year(), resp.Year)
	assert.Equal(t, int(f.start.Month()), resp.Month)

	dune := refByTitle(t, resp.Books, "Dune")
	assert.Equal(t, "book-1", dune.Key)
	assert.False(t, dune.Removed)
	require.NotNil(t, dune.BookID)
	assert.Equal(t, 1, *dune.BookID)
	// Ivanhoe had no Read-through to keep its title, so only its Session
	// names it now.
	removed := 0
	for _, ref := range resp.Books {
		if ref.Removed {
			removed++
			assert.Nil(t, ref.BookID)
			assert.True(t, strings.HasPrefix(ref.Key, "session-"), ref.Key)
		}
	}
	assert.Equal(t, 1, removed, "%+v", resp.Books)
	emma := refByTitle(t, resp.Books, "Emma")

	var startDay *StatsCalendarDay
	for i := range resp.Days {
		if resp.Days[i].Day == f.start.Format(time.DateOnly) {
			startDay = &resp.Days[i]
		}
	}
	require.NotNil(t, startDay, "%+v", resp.Days)
	assert.Contains(t, startDay.Books, StatsCalendarDayBook{Key: emma.Key, Finished: true})
	found := false
	for _, b := range startDay.Books {
		if b.Key == dune.Key {
			found = true
			assert.Positive(t, b.Seconds)
		}
	}
	assert.True(t, found, "%+v", startDay.Books)
}

// The day route sends a day's Sessions with where each came from, and the
// Read-throughs finished on it.
func TestStatsDay_SendsTheDaysSessions(t *testing.T) {
	t.Parallel()
	f := newCalendarFixture(t)
	day := f.start.Format(time.DateOnly)
	var resp StatsDayResponse
	f.get("/mobile/stats/day?day="+day, &resp)
	assert.Equal(t, day, resp.Day)
	assert.Equal(t, "UTC", resp.TimeZone)
	require.Len(t, resp.Finished, 1)
	assert.Equal(t, "Emma", resp.Finished[0].Title)
	require.NotNil(t, resp.Finished[0].BookID)
	assert.Equal(t, 2, *resp.Finished[0].BookID)

	require.NotEmpty(t, resp.Sessions)
	first := resp.Sessions[0]
	assert.Equal(t, "Dune", first.Book.Title)
	require.NotNil(t, first.Book.BookID)
	assert.Equal(t, 1, *first.Book.BookID)
	assert.True(t, first.StartedAt.Equal(f.start), "%s", first.StartedAt)
	assert.True(t, first.EndedAt.Equal(f.start.Add(time.Hour)), "%s", first.EndedAt)
	assert.InDelta(t, 0.1, first.StartPercentage, 0.0001)
	assert.InDelta(t, 0.2, first.EndPercentage, 0.0001)
	assert.Equal(t, "listening", first.Kind)
	assert.Equal(t, "shisho_app", first.Source)
	require.NotNil(t, first.DeviceID)
	assert.Equal(t, deviceIDFor(t, f.fixture, f.credential), *first.DeviceID)
	assert.NotNil(t, first.DeviceName)

	require.Len(t, resp.Sessions, 2)
	assert.True(t, resp.Sessions[1].Book.Removed)
	assert.Nil(t, resp.Sessions[1].Book.BookID)
}

// A day that isn't one is refused.
func TestStatsDay_RefusesABadDay(t *testing.T) {
	t.Parallel()
	f := newReadingFixture(t)
	rec := f.do(call{path: "/mobile/stats/day?day=2026-02-30", credential: f.credential})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	rec = f.do(call{path: "/mobile/stats/calendar?year=2026&month=13", credential: f.credential})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
}
