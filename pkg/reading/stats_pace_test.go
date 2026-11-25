package reading

import (
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lengthFile adds a File of fileType with its length: words, pages, or
// seconds of audio.
func (f *statsFixture) lengthFile(book *models.Book, fileType string, length int) *models.File {
	f.t.Helper()
	file := f.addFile(book, fileType)
	column := "word_count"
	switch {
	case models.IsPageBasedFileType(fileType):
		column = "page_count"
	case models.IsAudiobookFileType(fileType):
		column = "audiobook_duration_seconds"
	}
	_, err := f.db.NewUpdate().Table("files").Set(column+" = ?", length).Where("id = ?", file.ID).Exec(f.ctx)
	require.NoError(f.t, err)
	return file
}

// timed is a measured Session of minutes in file, from one place to another.
func timed(file *models.File, at time.Time, minutes int, from, to float64) sessionSpec {
	return sessionSpec{file: file, start: at, end: at.Add(time.Duration(minutes) * time.Minute), measured: true, from: from, to: to}
}

// normalMonth adds the reading of one month in epub, a 100,000-word book,
// on the day given: 30, 45, 20, and 60 minutes at 250, 240, 280, and 220
// words a minute.
func (f *statsFixture) normalMonth(epub *models.File, day time.Time) {
	f.t.Helper()
	f.session(timed(epub, day, 30, 0.000, 0.075))
	f.session(timed(epub, day.Add(2*time.Hour), 45, 0.075, 0.183))
	f.session(timed(epub, day.Add(4*time.Hour), 20, 0.183, 0.239))
	f.session(timed(epub, day.Add(6*time.Hour), 60, 0.239, 0.371))
}

func TestStats_PaceLeavesOutASkipAndCreditsItsTimeAtTheReadersAverage(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	epub := f.lengthFile(book, models.FileTypeEPUB, 100_000)
	day := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	f.normalMonth(epub, day)
	// Two minutes jumping 40% on through the contents: 20,000 words a
	// minute.
	f.session(timed(epub, day.Add(8*time.Hour), 2, 0.371, 0.771))

	// The reader's paces have quartiles of 220 and 250, so anything over
	// 250 plus three times that spread, 367 words a minute, is a skip.
	// Pace is the average of the rest, 37,100 words in 155 minutes, and
	// Words read credits the skip's two minutes at that average.
	resp := f.stats("?year=2026")
	require.NotNil(t, resp.Pace.WordsPerMinute)
	assert.InDelta(t, 37_100.0/155, *resp.Pace.WordsPerMinute, 0.01)
	assert.InDelta(t, 37_100+2*37_100.0/155, resp.Totals.WordsRead, 1)
}

func TestStats_PaceLeavesOutTimeWithABookLeftOpen(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	epub := f.lengthFile(book, models.FileTypeEPUB, 100_000)
	day := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	f.normalMonth(epub, day)
	f.session(timed(epub, day.Add(8*time.Hour), 2, 0.371, 0.771))
	// Twenty minutes open on a page while away: 10 words a minute, under
	// the quartile of 220 less three times the spread, 150.
	f.session(timed(epub, day.Add(10*time.Hour), 20, 0.771, 0.773))

	resp := f.stats("?year=2026")
	require.NotNil(t, resp.Pace.WordsPerMinute)
	assert.InDelta(t, 37_100.0/155, *resp.Pace.WordsPerMinute, 0.01, "the idle time doesn't slow it")
	assert.InDelta(t, 37_100+2*37_100.0/155+200, resp.Totals.WordsRead, 1, "the words it covered still count")
}

func TestStats_PaceBlendsDifferentKindsOfReading(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	thriller := f.lengthFile(book, models.FileTypeEPUB, 360_000)
	history := f.lengthFile(book, models.FileTypeEPUB, 216_000)
	day := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	// Ten hours at 300 words a minute and ten at 180.
	for i := range 5 {
		at := day.AddDate(0, 0, i)
		f.session(timed(thriller, at, 120, float64(i)*0.1, float64(i+1)*0.1))
		f.session(timed(history, at.Add(3*time.Hour), 120, float64(i)*0.1, float64(i+1)*0.1))
	}

	resp := f.stats("?year=2026")
	assert.Equal(t, 288_000, resp.Totals.WordsRead, "neither is a skip")
	require.NotNil(t, resp.Pace.WordsPerMinute)
	assert.InDelta(t, 240, *resp.Pace.WordsPerMinute, 0.01, "the two blended, not one picked")
}

func TestStats_PaceLearnsTheLimitFromAllTime(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	epub := f.lengthFile(book, models.FileTypeEPUB, 100_000)
	f.normalMonth(epub, time.Date(2025, 3, 2, 9, 0, 0, 0, time.UTC))
	// 2026 alone has too few Sessions to judge a skip by.
	day := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	f.session(timed(epub, day, 30, 0.000, 0.075))
	f.session(timed(epub, day.Add(2*time.Hour), 2, 0.075, 0.475))

	// What's normal comes from all six timed Sessions: the skip is over
	// 367 words a minute, and the other five average 44,600 words in 185
	// minutes.
	resp := f.stats("?year=2026")
	assert.InDelta(t, 7500+2*44_600.0/185, resp.Totals.WordsRead, 1)
	require.NotNil(t, resp.Pace.WordsPerMinute)
	assert.InDelta(t, 250, *resp.Pace.WordsPerMinute, 0.01, "the year's own reading, without the skip")
}

func TestStats_PaceCountsEverythingWithTooFewSessionsToJudge(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	epub := f.lengthFile(book, models.FileTypeEPUB, 100_000)
	day := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	f.session(timed(epub, day, 30, 0.000, 0.075))
	f.session(timed(epub, day.Add(2*time.Hour), 2, 0.075, 0.475))

	assert.Equal(t, 47_500, f.stats("?year=2026").Totals.WordsRead)
}

func TestStats_PaceCountsOnlyTimedSessions(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	epub := f.lengthFile(book, models.FileTypeEPUB, 100_000)
	day := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	f.session(timed(epub, day, 30, 0.10, 0.15))
	// A Kobo counts the minutes it spent: 3,000 words in half an hour.
	kobo := timed(epub, day.Add(2*time.Hour), 30, 0.15, 0.18)
	kobo.measured, kobo.source = false, models.ReadingSourceKobo
	f.session(kobo)
	// KOReader without the plugin guesses its time from when it synced, so
	// its 2,000 words count as read but not toward pace.
	guess := timed(epub, day.Add(6*time.Hour), 300, 0.18, 0.20)
	guess.measured, guess.source = false, models.ReadingSourceKOReader
	f.session(guess)
	// Going back counts nothing.
	f.session(timed(epub, day.Add(13*time.Hour), 20, 0.50, 0.30))

	resp := f.stats("?year=2026")
	assert.Equal(t, 10_000, resp.Totals.WordsRead)
	require.NotNil(t, resp.Pace.WordsPerMinute)
	assert.InDelta(t, 8000.0/60, *resp.Pace.WordsPerMinute, 0.01)
}

func TestStats_PacePerKind(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	cbz := f.lengthFile(book, models.FileTypeCBZ, 200)
	m4b := f.lengthFile(book, models.FileTypeM4B, 36_000)
	day := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	// 100 pages in an hour, and 1.5 hours of audio in an hour.
	f.session(timed(cbz, day, 60, 0, 0.5))
	f.session(timed(m4b, day, 60, 0.20, 0.35))

	resp := f.stats("?year=2026")
	assert.Nil(t, resp.Pace.WordsPerMinute)
	require.NotNil(t, resp.Pace.PagesPerHour)
	assert.InDelta(t, 100, *resp.Pace.PagesPerHour, 0.01)
	require.NotNil(t, resp.Pace.ListeningSpeed)
	assert.InDelta(t, 1.5, *resp.Pace.ListeningSpeed, 0.001)
	assert.Zero(t, resp.Totals.WordsRead)
	assert.Equal(t, ReadingStatsPace{}, f.stats("?year=2025").Pace, "nothing read in the year")
}

func TestStats_PaceNeedsTenMinutesOfTimedReading(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	epub := f.lengthFile(book, models.FileTypeEPUB, 100_000)
	day := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	f.session(timed(epub, day, 5, 0.10, 0.11))

	resp := f.stats("?year=2026")
	assert.Nil(t, resp.Pace.WordsPerMinute, "five minutes is too little to say")
	assert.Equal(t, 1000, resp.Totals.WordsRead)
}

func TestStats_PaceLeavesOutFilesOfUnknownLength(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	epub := f.addFile(book, models.FileTypeEPUB)
	day := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	f.session(timed(epub, day, 60, 0.10, 0.50))
	f.session(sessionSpec{start: day.Add(2 * time.Hour), end: day.Add(3 * time.Hour), measured: true, from: 0.1, to: 0.5})

	resp := f.stats("?year=2026")
	assert.Equal(t, ReadingStatsPace{}, resp.Pace, "a File not yet counted, and a Session whose File is gone")
	assert.Zero(t, resp.Totals.WordsRead)
}

func TestStats_PaceCountsTheShareOfASessionInTheRange(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	epub := f.lengthFile(book, models.FileTypeEPUB, 100_000)
	// Half an hour on either side of New Year, 6,000 words.
	f.session(timed(epub, time.Date(2025, 12, 31, 23, 30, 0, 0, time.UTC), 60, 0.10, 0.16))

	resp := f.stats("?year=2026")
	assert.Equal(t, 3000, resp.Totals.WordsRead, "half the Session fell in 2026")
	require.NotNil(t, resp.Pace.WordsPerMinute)
	assert.InDelta(t, 100, *resp.Pace.WordsPerMinute, 0.01)
}
