package reading

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (f *statsFixture) calendar(year, month int) ReadingStatsCalendarResponse {
	f.t.Helper()
	return calendarFor(f.t, f.api, year, month)
}

func calendarFor(t *testing.T, api *readingAPI, year, month int) ReadingStatsCalendarResponse {
	t.Helper()
	rec := api.do(http.MethodGet, fmt.Sprintf("/stats/calendar?year=%d&month=%d", year, month), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp ReadingStatsCalendarResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func (f *statsFixture) day(day string) ReadingStatsDayResponse {
	f.t.Helper()
	return dayFor(f.t, f.api, day)
}

func dayFor(t *testing.T, api *readingAPI, day string) ReadingStatsDayResponse {
	t.Helper()
	rec := api.do(http.MethodGet, "/stats/day?day="+day, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp ReadingStatsDayResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func bookKey(book *models.Book) string { return fmt.Sprintf("book-%d", book.ID) }

// calendarDay is the books on day, or nil when the calendar has nothing on it.
func calendarDay(resp ReadingStatsCalendarResponse, day string) []ReadingStatsCalendarDayBook {
	for _, d := range resp.Days {
		if d.Day == day {
			return d.Books
		}
	}
	return nil
}

func calendarDays(resp ReadingStatsCalendarResponse) []string {
	days := []string{}
	for _, d := range resp.Days {
		days = append(days, d.Day)
	}
	return days
}

func (f *statsFixture) heatmap() ReadingStatsHeatmapResponse {
	f.t.Helper()
	return heatmapFor(f.t, f.api, "")
}

func heatmapFor(t *testing.T, api *readingAPI, query string) ReadingStatsHeatmapResponse {
	t.Helper()
	rec := api.do(http.MethodGet, "/stats/heatmap"+query, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp ReadingStatsHeatmapResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

// daysAgo is n days before today in loc, at hh:mm there.
func daysAgo(t *testing.T, loc *time.Location, n, hh, mm int) time.Time {
	t.Helper()
	y, m, d := time.Now().In(loc).Date()
	return time.Date(y, m, d-n, hh, mm, 0, 0, loc)
}

func dayOf(instant time.Time) string { return instant.Format(time.DateOnly) }

// The heatmap's year ends today in the user's time zone, so its sessions
// are placed relative to now; a test run across midnight still sees them in
// the window.
func TestHeatmap_HasEachDaysTimeForTheYearUpToToday(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	f.setTimeZone("America/Chicago")
	chicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)
	book := createBook(t, f.db, "Dune")
	ebook := f.addFile(book, models.FileTypeEPUB)
	audio := f.addFile(book, models.FileTypeM4B)
	lateNight := daysAgo(t, chicago, 30, 23, 30)
	nextMorning := daysAgo(t, chicago, 29, 8, 0)
	f.session(sessionSpec{file: ebook, start: lateNight, end: lateNight.Add(time.Hour)})
	f.session(sessionSpec{file: audio, start: nextMorning, end: nextMorning.Add(20 * time.Minute)})
	old := daysAgo(t, chicago, 400, 8, 0)
	f.session(sessionSpec{file: ebook, start: old, end: old.Add(10 * time.Minute)})

	resp := f.heatmap()
	today, err := time.Parse(time.DateOnly, resp.Today)
	require.NoError(t, err)
	assert.Contains(t, []string{dayOf(daysAgo(t, chicago, 0, 12, 0)), dayOf(daysAgo(t, chicago, -1, 12, 0))}, resp.Today)
	assert.Equal(t, today.AddDate(-1, 0, 1).Format(time.DateOnly), resp.From)
	assert.Equal(t, resp.Today, resp.To)
	assert.Equal(t, []ReadingStatsDay{
		{Day: dayOf(lateNight), ReadingSeconds: 1800},
		{Day: dayOf(nextMorning), ReadingSeconds: 1800, ListeningSeconds: 1200},
	}, resp.Days, "split at local midnight, and nothing from more than a year ago")
	require.NotNil(t, resp.FirstDay)
	assert.Equal(t, dayOf(old), *resp.FirstDay, "the first day counts reading outside the heatmap")
}

func TestHeatmap_ShowsAWholePastYear(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	f.setTimeZone("America/Chicago")
	book := createBook(t, f.db, "Dune")
	file := f.addFile(book, models.FileTypeEPUB)
	f.session(sessionSpec{file: file, start: at(t, "America/Chicago", "2024-01-01 09:00"), end: at(t, "America/Chicago", "2024-01-01 10:00")})
	f.session(sessionSpec{file: file, start: at(t, "America/Chicago", "2024-12-31 23:30"), end: at(t, "America/Chicago", "2025-01-01 00:30")})

	resp := heatmapFor(t, f.api, "?year=2024")
	assert.Equal(t, "2024-01-01", resp.From)
	assert.Equal(t, "2024-12-31", resp.To)
	assert.Equal(t, []ReadingStatsDay{
		{Day: "2024-01-01", ReadingSeconds: 3600},
		{Day: "2024-12-31", ReadingSeconds: 1800},
	}, resp.Days)
	assert.Equal(t, []ReadingStatsDay{{Day: "2025-01-01", ReadingSeconds: 1800}}, heatmapFor(t, f.api, "?year=2025").Days)
	assert.Equal(t, http.StatusUnprocessableEntity, f.api.do(http.MethodGet, "/stats/heatmap?year=0", nil).Code)
}

// This year, like any other, runs January to December, its days yet to
// come shown empty.
func TestHeatmap_ThisYearRunsToDecember(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	year := f.heatmap().Today[:4]
	resp := heatmapFor(t, f.api, "?year="+year)
	assert.Equal(t, year+"-01-01", resp.From)
	assert.Equal(t, year+"-12-31", resp.To)
}

func TestHeatmap_FirstDayCountsReadThroughsWithNoSessions(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	empty := f.heatmap()
	assert.Nil(t, empty.FirstDay)
	assert.NotNil(t, empty.Days)
	assert.Empty(t, empty.Days)

	f.finished(createBook(t, f.db, "Dune"), "2019-05-01", "2019-05-20")
	f.finished(createBook(t, f.db, "Emma"), "", "2018-02-03")
	resp := f.heatmap()
	require.NotNil(t, resp.FirstDay)
	assert.Equal(t, "2018-02-03", *resp.FirstDay)
}

func TestHeatmap_DropsADeletedSessionAndNeverShowsAnotherUsers(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	other := createUser(t, f.db, "other", 2)
	book := createBook(t, f.db, "Dune")
	file := f.addFile(book, models.FileTypeEPUB)
	when := daysAgo(t, time.UTC, 10, 8, 0)
	seedSession(t, f.db, other, sessionSpec{file: file, start: when, end: when.Add(time.Hour)})
	assert.Empty(t, f.heatmap().Days)
	assert.Nil(t, f.heatmap().FirstDay)

	f.session(sessionSpec{file: file, start: when, end: when.Add(time.Hour)})
	f.session(sessionSpec{file: file, start: when.Add(2 * time.Hour), end: when.Add(150 * time.Minute)})
	assert.Equal(t, []ReadingStatsDay{{Day: dayOf(when), ReadingSeconds: 5400}}, f.heatmap().Days)
	first := f.day(dayOf(when)).Sessions[0]
	require.Equal(t, http.StatusNoContent, f.api.do(http.MethodDelete, fmt.Sprintf("/sessions/%d", first.ID), nil).Code)
	assert.Equal(t, []ReadingStatsDay{{Day: dayOf(when), ReadingSeconds: 1800}}, f.heatmap().Days)
}

func TestCalendar_ASessionCrossingMidnightLandsOnBothDays(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	f.setTimeZone("America/Chicago")
	dune := createBook(t, f.db, "Dune")
	file := f.addFile(dune, models.FileTypeEPUB)
	rt := f.finished(dune, "2026-04-29", "2026-05-01")
	f.session(sessionSpec{
		file: file, readThrough: rt,
		start: at(t, "America/Chicago", "2026-04-30 23:30"),
		end:   at(t, "America/Chicago", "2026-05-01 00:30"),
	})

	april := f.calendar(2026, 4)
	assert.Equal(t, 2026, april.Year)
	assert.Equal(t, 4, april.Month)
	assert.Equal(t, []string{"2026-04-30"}, calendarDays(april))
	assert.Equal(t, []ReadingStatsCalendarDayBook{{Key: bookKey(dune), Seconds: 1800}}, calendarDay(april, "2026-04-30"))
	require.Len(t, april.Books, 1)
	assert.Equal(t, bookKey(dune), april.Books[0].Key)
	assert.Equal(t, "Dune", april.Books[0].Title)
	assert.False(t, april.Books[0].Removed)
	require.NotNil(t, april.Books[0].Book)
	assert.Equal(t, dune.ID, april.Books[0].Book.ID)

	may := f.calendar(2026, 5)
	assert.Equal(t, []ReadingStatsCalendarDayBook{{Key: bookKey(dune), Seconds: 1800, Finished: true}}, calendarDay(may, "2026-05-01"))

	for _, day := range []string{"2026-04-30", "2026-05-01"} {
		detail := f.day(day)
		assert.Equal(t, day, detail.Day)
		assert.Equal(t, "America/Chicago", detail.TimeZone)
		require.Len(t, detail.Sessions, 1, day)
		s := detail.Sessions[0]
		assert.Equal(t, "Dune", s.Book.Title)
		assert.Equal(t, 1800, s.Seconds, "only the part on %s", day)
		assert.True(t, s.StartedAt.Equal(at(t, "America/Chicago", "2026-04-30 23:30")), "the whole Session's start")
		assert.True(t, s.EndedAt.Equal(at(t, "America/Chicago", "2026-05-01 00:30")))
	}
	assert.Empty(t, f.day("2026-04-30").Finished)
	finished := f.day("2026-05-01").Finished
	require.Len(t, finished, 1)
	assert.Equal(t, bookKey(dune), finished[0].Key)
}

// 2027-10-31 is 25 hours long in Berlin: clocks go back an hour that
// morning. A day taken as 24 hours from its start would end at 23:00 and put
// the first half of this Session on November 1.
func TestCalendar_ADaySplitByDaylightSavingEndsAtItsRealMidnight(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	f.setTimeZone("Europe/Berlin")
	dune := createBook(t, f.db, "Dune")
	file := f.addFile(dune, models.FileTypeEPUB)
	f.session(sessionSpec{
		file:  file,
		start: at(t, "Europe/Berlin", "2027-10-31 23:30"),
		end:   at(t, "Europe/Berlin", "2027-11-01 00:30"),
	})

	assert.Equal(t, []ReadingStatsCalendarDayBook{{Key: bookKey(dune), Seconds: 1800}}, calendarDay(f.calendar(2027, 10), "2027-10-31"))
	assert.Equal(t, []ReadingStatsCalendarDayBook{{Key: bookKey(dune), Seconds: 1800}}, calendarDay(f.calendar(2027, 11), "2027-11-01"))
	require.Len(t, f.day("2027-10-31").Sessions, 1)
	assert.Equal(t, 1800, f.day("2027-10-31").Sessions[0].Seconds)
}

func TestCalendar_AFinishWithNoSessionsShowsOnlyItsMarker(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	f.finished(dune, "", "2026-03-14")
	// Underway and abandoned Read-throughs mark nothing.
	seedReadThrough(t, f.db, f.user, createBook(t, f.db, "Abandoned"), "2026-03-01", "2026-03-10", models.ReadThroughOutcomeAbandoned)
	seedReadThrough(t, f.db, f.user, createBook(t, f.db, "Underway"), "2026-03-01", "", models.ReadThroughOutcomeUnderway)

	march := f.calendar(2026, 3)
	assert.Equal(t, []string{"2026-03-14"}, calendarDays(march))
	assert.Equal(t, []ReadingStatsCalendarDayBook{{Key: bookKey(dune), Finished: true}}, calendarDay(march, "2026-03-14"))
	require.Len(t, march.Books, 1)
	assert.Equal(t, "Dune", march.Books[0].Title)

	detail := f.day("2026-03-14")
	assert.Empty(t, detail.Sessions)
	assert.NotNil(t, detail.Sessions)
	require.Len(t, detail.Finished, 1)
	assert.Equal(t, "Dune", detail.Finished[0].Title)
}

func TestCalendar_ADetachedBookShowsByItsCopiedTitle(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	file := f.addFile(dune, models.FileTypeEPUB)
	rt := f.finished(dune, "2026-02-01", "2026-02-02")
	f.session(sessionSpec{file: file, readThrough: rt, start: at(t, "UTC", "2026-02-01 10:00"), end: at(t, "UTC", "2026-02-01 11:00")})
	f.session(sessionSpec{file: file, readThrough: rt, start: at(t, "UTC", "2026-02-02 10:00"), end: at(t, "UTC", "2026-02-02 10:30")})
	deleteBook(t, f.db, dune)

	key := fmt.Sprintf("read-through-%d", rt.ID)
	feb := f.calendar(2026, 2)
	assert.Equal(t, []ReadingStatsCalendarDayBook{{Key: key, Seconds: 3600}}, calendarDay(feb, "2026-02-01"))
	assert.Equal(t, []ReadingStatsCalendarDayBook{{Key: key, Seconds: 1800, Finished: true}}, calendarDay(feb, "2026-02-02"))
	assert.Equal(t, []ReadingStatsBookRef{{Key: key, Title: "Dune", Removed: true}}, feb.Books)

	detail := f.day("2026-02-01")
	require.Len(t, detail.Sessions, 1)
	assert.Equal(t, ReadingStatsBookRef{Key: key, Title: "Dune", Removed: true}, detail.Sessions[0].Book)
}

func TestCalendar_ABookOutOfReachShowsWithNoLink(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	file := f.addFile(dune, models.FileTypeEPUB)
	f.session(sessionSpec{file: file, start: at(t, "UTC", "2026-02-01 10:00"), end: at(t, "UTC", "2026-02-01 11:00")})

	f.user.LibraryAccess = []*models.UserLibraryAccess{{UserID: f.user.ID, LibraryID: intp(dune.LibraryID + 100)}}
	api := newReadingAPI(t, f.db, f.user)
	assert.Equal(t, []ReadingStatsBookRef{{Key: bookKey(dune), Title: "Dune"}}, calendarFor(t, api, 2026, 2).Books)
	detail := dayFor(t, api, "2026-02-01")
	require.Len(t, detail.Sessions, 1)
	assert.Nil(t, detail.Sessions[0].Book.Book)
}

func TestCalendar_DayDetailListsSessionsInOrderWithHowFarEachWent(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	hyperion := createBook(t, f.db, "Hyperion")
	duneFile := f.addFile(dune, models.FileTypeEPUB)
	hyperionAudio := f.addFile(hyperion, models.FileTypeM4B)
	f.session(sessionSpec{file: hyperionAudio, start: at(t, "UTC", "2026-06-10 20:00"), end: at(t, "UTC", "2026-06-10 21:00")})
	f.session(sessionSpec{
		file: duneFile, start: at(t, "UTC", "2026-06-10 08:00"), end: at(t, "UTC", "2026-06-10 08:45"),
		source: models.ReadingSourceKOReader, duration: 1800,
	})
	_, err := f.db.ExecContext(f.ctx, `UPDATE reading_sessions SET start_percentage = 0.1, end_percentage = 0.25 WHERE file_id = ?`, duneFile.ID)
	require.NoError(t, err)
	// Another day's Session isn't listed.
	f.session(sessionSpec{file: duneFile, start: at(t, "UTC", "2026-06-11 08:00"), end: at(t, "UTC", "2026-06-11 09:00")})

	detail := f.day("2026-06-10")
	assert.Equal(t, "UTC", detail.TimeZone)
	require.Len(t, detail.Sessions, 2)
	first, second := detail.Sessions[0], detail.Sessions[1]
	assert.Equal(t, "Dune", first.Book.Title)
	assert.Equal(t, 1800, first.Seconds)
	assert.InDelta(t, 0.1, first.StartPercentage, 0.0001)
	assert.InDelta(t, 0.25, first.EndPercentage, 0.0001)
	assert.Equal(t, models.ReadingSourceKOReader, first.Source)
	assert.Equal(t, models.ReadingKindReading, first.Kind)
	assert.Equal(t, "Hyperion", second.Book.Title)
	assert.Equal(t, models.ReadingKindListening, second.Kind)
	assert.Equal(t, 3600, second.Seconds)

	june := f.calendar(2026, 6)
	assert.ElementsMatch(t, []ReadingStatsCalendarDayBook{
		{Key: bookKey(dune), Seconds: 1800},
		{Key: bookKey(hyperion), Seconds: 3600},
	}, calendarDay(june, "2026-06-10"))
	assert.Equal(t, []string{"Dune", "Hyperion"}, []string{june.Books[0].Title, june.Books[1].Title})
}

func TestCalendar_NeverShowsAnotherUsersReading(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	other := createUser(t, f.db, "other", 2)
	dune := createBook(t, f.db, "Dune")
	file := f.addFile(dune, models.FileTypeEPUB)
	seedReadThrough(t, f.db, other, dune, "2026-01-01", "2026-01-02", models.ReadThroughOutcomeFinished)
	seedSession(t, f.db, other, sessionSpec{file: file, start: at(t, "UTC", "2026-01-02 10:00"), end: at(t, "UTC", "2026-01-02 11:00")})

	jan := f.calendar(2026, 1)
	assert.Empty(t, jan.Days)
	assert.NotNil(t, jan.Days)
	assert.Empty(t, jan.Books)
	assert.NotNil(t, jan.Books)
	detail := f.day("2026-01-02")
	assert.Empty(t, detail.Sessions)
	assert.Empty(t, detail.Finished)

	theirs := calendarFor(t, newReadingAPI(t, f.db, other), 2026, 1)
	assert.Len(t, theirs.Days, 1)
}

func TestCalendar_RejectsAnInvalidMonthOrDay(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	for _, path := range []string{
		"/stats/calendar?year=2026",
		"/stats/calendar?year=2026&month=13",
		"/stats/calendar?year=0&month=1",
		"/stats/day",
		"/stats/day?day=2026-02-30",
		"/stats/day?day=yesterday",
	} {
		assert.Equal(t, http.StatusUnprocessableEntity, f.api.do(http.MethodGet, path, nil).Code, path)
	}
}

func TestSessions_DeletingOnesOwnSessionRemovesItsTimeEverywhere(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	file := f.addFile(dune, models.FileTypeEPUB)
	rt := f.finished(dune, "2026-06-10", "2026-06-10")
	f.session(sessionSpec{file: file, readThrough: rt, start: at(t, "UTC", "2026-06-10 08:00"), end: at(t, "UTC", "2026-06-10 09:00")})
	f.session(sessionSpec{file: file, readThrough: rt, start: at(t, "UTC", "2026-06-10 20:00"), end: at(t, "UTC", "2026-06-10 20:30")})

	sessions := f.day("2026-06-10").Sessions
	require.Len(t, sessions, 2)
	rec := f.api.do(http.MethodDelete, fmt.Sprintf("/sessions/%d", sessions[0].ID), nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	remaining := f.day("2026-06-10").Sessions
	require.Len(t, remaining, 1)
	assert.Equal(t, sessions[1].ID, remaining[0].ID)
	stats := f.stats("?year=2026")
	assert.Equal(t, 1800, stats.Totals.ReadingSeconds)
	assert.Equal(t, []ReadingStatsCalendarDayBook{{Key: bookKey(dune), Seconds: 1800, Finished: true}}, calendarDay(f.calendar(2026, 6), "2026-06-10"))
	finished := f.finishedList("?year=2026").Items
	require.Len(t, finished, 1)
	require.NotNil(t, finished[0].Seconds)
	assert.Equal(t, 1800, *finished[0].Seconds)

	assert.Equal(t, http.StatusNotFound, f.api.do(http.MethodDelete, fmt.Sprintf("/sessions/%d", sessions[0].ID), nil).Code, "already gone")
}

func TestSessions_AnotherUsersSessionIsNotFound(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	other := createUser(t, f.db, "other", 2)
	dune := createBook(t, f.db, "Dune")
	file := f.addFile(dune, models.FileTypeEPUB)
	seedSession(t, f.db, other, sessionSpec{file: file, start: at(t, "UTC", "2026-06-10 08:00"), end: at(t, "UTC", "2026-06-10 09:00")})
	theirs := dayFor(t, newReadingAPI(t, f.db, other), "2026-06-10").Sessions
	require.Len(t, theirs, 1)

	assert.Equal(t, http.StatusNotFound, f.api.do(http.MethodDelete, fmt.Sprintf("/sessions/%d", theirs[0].ID), nil).Code)
	assert.Equal(t, http.StatusNotFound, f.api.do(http.MethodDelete, "/sessions/abc", nil).Code)
	assert.Len(t, dayFor(t, newReadingAPI(t, f.db, other), "2026-06-10").Sessions, 1, "theirs is untouched")
}

func TestSessions_DeleteFailureIs500(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	file := f.addFile(dune, models.FileTypeEPUB)
	f.session(sessionSpec{file: file, start: at(t, "UTC", "2026-06-10 08:00"), end: at(t, "UTC", "2026-06-10 09:00")})
	id := f.day("2026-06-10").Sessions[0].ID
	_, err := f.db.Exec(`CREATE TRIGGER fail_delete BEFORE DELETE ON reading_sessions BEGIN SELECT RAISE(ABORT, 'boom'); END`)
	require.NoError(t, err)

	assert.Equal(t, http.StatusInternalServerError,
		f.api.do(http.MethodDelete, fmt.Sprintf("/sessions/%d", id), nil).Code,
		"a failed delete isn't a missing Reading Session")
}

// With no Read-through, a Session names its Book only through its File.
// Once both are gone, nothing says two such Sessions were the same Book, so
// they never join into one bar.
func TestCalendar_SessionsWithNothingLeftToNameThemStayApart(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	emma := createBook(t, f.db, "Emma")
	f.session(sessionSpec{file: f.addFile(dune, models.FileTypeEPUB), start: at(t, "UTC", "2026-06-10 08:00"), end: at(t, "UTC", "2026-06-10 09:00")})
	f.session(sessionSpec{file: f.addFile(emma, models.FileTypeEPUB), start: at(t, "UTC", "2026-06-11 08:00"), end: at(t, "UTC", "2026-06-11 09:00")})
	deleteBook(t, f.db, dune)
	deleteBook(t, f.db, emma)

	june := f.calendar(2026, 6)
	first, second := calendarDay(june, "2026-06-10"), calendarDay(june, "2026-06-11")
	require.Len(t, first, 1)
	require.Len(t, second, 1)
	assert.NotEqual(t, first[0].Key, second[0].Key)
	require.Len(t, june.Books, 2)
	for _, ref := range june.Books {
		assert.Empty(t, ref.Title)
		assert.True(t, ref.Removed)
	}
	detail := f.day("2026-06-10")
	require.Len(t, detail.Sessions, 1)
	assert.Equal(t, first[0].Key, detail.Sessions[0].Book.Key)
}

func (f *statsFixture) readThroughSessions(rt *models.ReadThrough) ListReadThroughSessionsResponse {
	f.t.Helper()
	rec := f.api.do(http.MethodGet, fmt.Sprintf("/read-throughs/%d/sessions", rt.ID), nil)
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp ListReadThroughSessionsResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func TestSessions_AReadThroughListsItsSessionsInOrder(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	f.setTimeZone("America/Chicago")
	dune := createBook(t, f.db, "Dune")
	file := f.addFile(dune, models.FileTypeEPUB)
	rt := f.finished(dune, "2026-06-01", "2026-06-03")
	other := f.finished(createBook(t, f.db, "Emma"), "2026-06-01", "2026-06-02")
	f.session(sessionSpec{file: file, readThrough: rt, start: at(t, "UTC", "2026-06-03 20:00"), end: at(t, "UTC", "2026-06-03 20:30"), source: models.ReadingSourceKobo, duration: 1500})
	f.session(sessionSpec{file: file, readThrough: rt, start: at(t, "UTC", "2026-06-01 08:00"), end: at(t, "UTC", "2026-06-01 09:00")})
	f.session(sessionSpec{readThrough: other, start: at(t, "UTC", "2026-06-02 08:00"), end: at(t, "UTC", "2026-06-02 09:00")})

	resp := f.readThroughSessions(rt)
	assert.Equal(t, "America/Chicago", resp.TimeZone)
	require.Len(t, resp.Items, 2)
	first, second := resp.Items[0], resp.Items[1]
	assert.True(t, first.StartedAt.Equal(at(t, "UTC", "2026-06-01 08:00")))
	assert.Equal(t, 3600, first.Seconds)
	assert.Equal(t, models.ReadingSourceKobo, second.Source)
	assert.Equal(t, 1500, second.Seconds, "the time the Session recorded, not its span")
	assert.False(t, second.Measured)

	// Deleting one from here is the same delete as from a day.
	require.Equal(t, http.StatusNoContent, f.api.do(http.MethodDelete, fmt.Sprintf("/sessions/%d", first.ID), nil).Code)
	assert.Len(t, f.readThroughSessions(rt).Items, 1)

	empty := f.readThroughSessions(f.finished(createBook(t, f.db, "Hand"), "", "2026-06-05"))
	assert.NotNil(t, empty.Items)
	assert.Empty(t, empty.Items)
}

func TestSessions_ADetachedReadThroughStillListsItsSessions(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	file := f.addFile(dune, models.FileTypeEPUB)
	rt := f.finished(dune, "2026-06-01", "2026-06-01")
	f.session(sessionSpec{file: file, readThrough: rt, start: at(t, "UTC", "2026-06-01 08:00"), end: at(t, "UTC", "2026-06-01 09:00")})
	deleteBook(t, f.db, dune)

	assert.Len(t, f.readThroughSessions(rt).Items, 1)
}

func TestSessions_AnotherUsersReadThroughIsNotFound(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	other := createUser(t, f.db, "other", 2)
	theirs := seedReadThrough(t, f.db, other, createBook(t, f.db, "Dune"), "2026-06-01", "2026-06-02", models.ReadThroughOutcomeFinished)
	seedSession(t, f.db, other, sessionSpec{readThrough: theirs, start: at(t, "UTC", "2026-06-01 08:00"), end: at(t, "UTC", "2026-06-01 09:00")})

	assert.Equal(t, http.StatusNotFound, f.api.do(http.MethodGet, fmt.Sprintf("/read-throughs/%d/sessions", theirs.ID), nil).Code)
}

func TestHistory_EachReadThroughCountsItsSessions(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	rt := f.finished(dune, "2026-06-01", "2026-06-02")
	f.finished(createBook(t, f.db, "Emma"), "2026-05-01", "2026-05-02")
	for _, day := range []string{"2026-06-01", "2026-06-02"} {
		f.session(sessionSpec{readThrough: rt, start: at(t, "UTC", day+" 08:00"), end: at(t, "UTC", day+" 09:00")})
	}

	counts := map[string]int{}
	for _, item := range f.api.history("").Items {
		counts[item.Title] = item.SessionCount
	}
	assert.Equal(t, map[string]int{"Dune": 2, "Emma": 0}, counts)
}

func TestSessions_ABooksReadingRecordCountsEachReadThroughsSessions(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	first := f.finished(dune, "2026-01-01", "2026-01-05")
	again := f.finished(dune, "2026-03-01", "2026-03-02")
	for _, day := range []string{"2026-01-01", "2026-01-02", "2026-01-03"} {
		f.session(sessionSpec{readThrough: first, start: at(t, "UTC", day+" 08:00"), end: at(t, "UTC", day+" 09:00")})
	}
	// Another user's Session of the same Book isn't counted.
	other := createUser(t, f.db, "other", 2)
	seedSession(t, f.db, other, sessionSpec{readThrough: seedReadThrough(t, f.db, other, dune, "2026-01-01", "2026-01-02", models.ReadThroughOutcomeFinished), start: at(t, "UTC", "2026-01-01 08:00"), end: at(t, "UTC", "2026-01-01 09:00")})

	rec := f.api.do(http.MethodGet, fmt.Sprintf("/books/%d", dune.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp BookReadingResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, map[int]int{first.ID: 3, again.ID: 0}, resp.SessionCounts)
}
