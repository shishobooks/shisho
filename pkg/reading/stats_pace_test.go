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

func TestStats_Pace(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	epub := f.lengthFile(book, models.FileTypeEPUB, 100_000)
	cbz := f.lengthFile(book, models.FileTypeCBZ, 200)
	m4b := f.lengthFile(book, models.FileTypeM4B, 36_000)
	day := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)

	// 5,000 and 4,000 words in half an hour each.
	f.session(timed(epub, day, 30, 0.10, 0.15))
	f.session(timed(epub, day.Add(2*time.Hour), 30, 0.15, 0.19))
	// A Kobo counts the minutes it spent: 3,000 words in half an hour.
	kobo := timed(epub, day.Add(4*time.Hour), 30, 0.19, 0.22)
	kobo.measured, kobo.source = false, models.ReadingSourceKobo
	f.session(kobo)
	// KOReader without the plugin guesses its time from when it synced, so
	// its 2,000 words count as read but not toward pace.
	guess := timed(epub, day.Add(6*time.Hour), 300, 0.22, 0.24)
	guess.measured, guess.source = false, models.ReadingSourceKOReader
	f.session(guess)
	// A skip ahead isn't reading: 80,000 words in two minutes.
	f.session(timed(epub, day.Add(12*time.Hour), 2, 0.24, 0.94))
	// Going back counts nothing.
	f.session(timed(epub, day.Add(13*time.Hour), 20, 0.94, 0.50))
	// Under a minute is too short to time: 100 words counted as read only.
	short := timed(epub, day.Add(14*time.Hour), 0, 0.50, 0.501)
	short.end = short.start.Add(30 * time.Second)
	f.session(short)

	// 100 pages in an hour.
	f.session(timed(cbz, day, 60, 0, 0.5))
	// 1.5 hours of audio in an hour.
	f.session(timed(m4b, day, 60, 0.20, 0.35))

	resp := f.stats("?year=2026")
	require.NotNil(t, resp.Pace.WordsPerMinute)
	assert.InDelta(t, 12_000.0/90, *resp.Pace.WordsPerMinute, 0.01, "measured and Kobo Sessions, weighted by time")
	require.NotNil(t, resp.Pace.PagesPerHour)
	assert.InDelta(t, 100, *resp.Pace.PagesPerHour, 0.01)
	require.NotNil(t, resp.Pace.ListeningSpeed)
	assert.InDelta(t, 1.5, *resp.Pace.ListeningSpeed, 0.001)
	assert.Equal(t, 14_100, resp.Totals.WordsRead, "every Session's way forward but the skip")

	assert.Equal(t, ReadingStatsPace{}, f.stats("?year=2025").Pace, "nothing read in the year")
	assert.Zero(t, f.stats("?year=2025").Totals.WordsRead)
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
