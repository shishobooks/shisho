package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// statsFixture seeds one user's Read-throughs, Sessions, and Ratings
// directly, with exact days and instants, and reads Reading Stats back
// through the endpoint.
type statsFixture struct {
	t    *testing.T
	ctx  context.Context
	db   *bun.DB
	user *models.User
	api  *readingAPI
	// files numbers each File's path apart.
	files int
}

func newStatsFixture(t *testing.T) *statsFixture {
	t.Helper()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	return &statsFixture{t: t, ctx: context.Background(), db: db, user: user, api: newReadingAPI(t, db, user)}
}

func (f *statsFixture) stats(query string) ReadingStatsResponse {
	f.t.Helper()
	return statsFor(f.t, f.api, query)
}

func statsFor(t *testing.T, api *readingAPI, query string) ReadingStatsResponse {
	t.Helper()
	rec := api.do(http.MethodGet, "/stats"+query, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp ReadingStatsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func optDay(day string) *string {
	if day == "" {
		return nil
	}
	return &day
}

// readThrough records a Read-through of book for user; "" leaves a day unset.
func seedReadThrough(t *testing.T, db *bun.DB, user *models.User, book *models.Book, begin, end, outcome string) *models.ReadThrough {
	t.Helper()
	now := time.Now()
	rt := &models.ReadThrough{
		CreatedAt: now, UpdatedAt: now, UserID: user.ID, BookID: &book.ID,
		BeginDay: optDay(begin), EndDay: optDay(end), Outcome: outcome,
	}
	_, err := db.NewInsert().Model(rt).Exec(context.Background())
	require.NoError(t, err)
	return rt
}

func (f *statsFixture) finished(book *models.Book, begin, end string) *models.ReadThrough {
	f.t.Helper()
	return seedReadThrough(f.t, f.db, f.user, book, begin, end, models.ReadThroughOutcomeFinished)
}

func (f *statsFixture) addFile(book *models.Book, fileType string) *models.File {
	f.t.Helper()
	f.files++
	file := &models.File{
		LibraryID: book.LibraryID, BookID: book.ID, FileType: fileType, FileRole: models.FileRoleMain,
		Filepath: fmt.Sprintf("%s/file%d.%s", book.Filepath, f.files, fileType), FilesizeBytes: 1,
	}
	_, err := f.db.NewInsert().Model(file).Exec(f.ctx)
	require.NoError(f.t, err)
	return file
}

type sessionSpec struct {
	file        *models.File
	readThrough *models.ReadThrough
	start, end  time.Time
	duration    int // 0 is the span
	kind        string
	source      string
	deviceID    *string
	measured    bool
}

func seedSession(t *testing.T, db *bun.DB, user *models.User, spec sessionSpec) {
	t.Helper()
	session := &models.ReadingSession{
		CreatedAt: spec.end, UserID: user.ID, Source: spec.source,
		StartedAt: spec.start, EndedAt: spec.end, Measured: spec.measured,
		Kind: spec.kind, Duration: spec.duration, SourceDeviceID: spec.deviceID,
	}
	if session.Source == "" {
		session.Source = models.ReadingSourceWebReader
	}
	if session.Duration == 0 {
		session.Duration = int(spec.end.Sub(spec.start).Seconds())
	}
	if spec.file != nil {
		session.FileID = &spec.file.ID
		if session.Kind == "" {
			session.Kind = models.ReadingKindForFileType(spec.file.FileType)
		}
	}
	if session.Kind == "" {
		session.Kind = models.ReadingKindReading
	}
	if spec.readThrough != nil {
		session.ReadThroughID = &spec.readThrough.ID
	}
	_, err := db.NewInsert().Model(session).Exec(context.Background())
	require.NoError(t, err)
}

func (f *statsFixture) session(spec sessionSpec) {
	f.t.Helper()
	seedSession(f.t, f.db, f.user, spec)
}

func (f *statsFixture) setTimeZone(zone string) {
	f.t.Helper()
	now := time.Now()
	_, err := f.db.NewInsert().Model(&models.UserSettings{
		UserID: f.user.ID, CreatedAt: now, UpdatedAt: now,
		ViewerFitMode: models.FitModeHeight, ReflowableTheme: models.ReflowableThemeLight,
		ReflowableFlow: models.ReflowableFlowPaginated, GallerySize: models.GallerySizeMedium,
		PlaybackSpeed: 1, ViewerPreloadCount: 3, ReflowableFontSize: 100, TimeZone: &zone,
	}).Exec(f.ctx)
	require.NoError(f.t, err)
}

func (f *statsFixture) rate(book *models.Book, units int) {
	f.t.Helper()
	now := time.Now()
	_, err := f.db.NewInsert().Model(&models.UserBook{
		UserID: f.user.ID, BookID: book.ID, CreatedAt: now, UpdatedAt: now, Rating: &units,
	}).Exec(f.ctx)
	require.NoError(f.t, err)
}

func (f *statsFixture) addGenre(book *models.Book, name string) {
	f.t.Helper()
	genre := &models.Genre{LibraryID: book.LibraryID, Name: name}
	_, err := f.db.NewInsert().Model(genre).Exec(f.ctx)
	require.NoError(f.t, err)
	_, err = f.db.NewInsert().Model(&models.BookGenre{BookID: book.ID, GenreID: genre.ID}).Exec(f.ctx)
	require.NoError(f.t, err)
}

func (f *statsFixture) addTag(book *models.Book, name string) {
	f.t.Helper()
	tag := &models.Tag{LibraryID: book.LibraryID, Name: name}
	_, err := f.db.NewInsert().Model(tag).Exec(f.ctx)
	require.NoError(f.t, err)
	_, err = f.db.NewInsert().Model(&models.BookTag{BookID: book.ID, TagID: tag.ID}).Exec(f.ctx)
	require.NoError(f.t, err)
}

func at(t *testing.T, zone, value string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	require.NoError(t, err)
	parsed, err := time.ParseInLocation("2006-01-02 15:04", value, loc)
	require.NoError(t, err)
	return parsed
}

func month(resp ReadingStatsResponse, m int) ReadingStatsPeriod {
	for _, p := range resp.Periods {
		if p.Month != nil && *p.Month == m {
			return p
		}
	}
	return ReadingStatsPeriod{}
}

func finishedTitles(resp ReadingStatsResponse) []string {
	out := make([]string, 0, len(resp.Finished))
	for _, row := range resp.Finished {
		out = append(out, row.Title)
	}
	return out
}

func TestStats_CountsRereadsAndDidNotFinishByTheDayTheyEnded(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	emma := createBook(t, f.db, "Emma")
	ulysses := createBook(t, f.db, "Ulysses")
	beloved := createBook(t, f.db, "Beloved")
	current := createBook(t, f.db, "Current")

	f.finished(beloved, "2025-12-01", "2025-12-30")
	f.finished(dune, "2026-02-01", "2026-02-10")
	f.finished(emma, "2026-05-05", "2026-05-05")
	f.finished(dune, "2026-07-20", "2026-08-01")
	seedReadThrough(t, f.db, f.user, ulysses, "2026-02-15", "2026-03-01", models.ReadThroughOutcomeAbandoned)
	seedReadThrough(t, f.db, f.user, current, "2026-09-01", "", models.ReadThroughOutcomeUnderway)

	year := f.stats("?year=2026")
	require.NotNil(t, year.Year)
	assert.Equal(t, 2026, *year.Year)
	assert.Equal(t, 3, year.Totals.Finished, "a reread counts each time")
	assert.Equal(t, 1, year.Totals.Rereads)
	assert.Equal(t, 1, year.Totals.DidNotFinish)
	require.NotNil(t, year.Totals.MedianDaysToFinish)
	assert.InDelta(t, 10, *year.Totals.MedianDaysToFinish, 0.001, "10, 1, and 13 days, counted inclusively")
	assert.Equal(t, []int{2026, 2025}, year.Years)

	require.Len(t, year.Periods, 12)
	assert.Equal(t, 1, month(year, 2).Finished)
	assert.Equal(t, 1, month(year, 5).Finished)
	assert.Equal(t, 1, month(year, 8).Finished)
	assert.Equal(t, 0, month(year, 3).Finished, "Did Not Finish isn't a finish")

	assert.Equal(t, []string{"Dune", "Emma", "Dune"}, finishedTitles(year), "latest first")
	require.NotNil(t, year.Finished[1].DaysTaken)
	assert.Equal(t, 1, *year.Finished[1].DaysTaken)
	require.NotNil(t, year.Finished[0].Book)
	assert.Equal(t, dune.ID, year.Finished[0].Book.ID)

	last := f.stats("?year=2025")
	assert.Equal(t, 1, last.Totals.Finished)
	assert.Equal(t, 0, last.Totals.Rereads)
	assert.Equal(t, 0, last.Totals.DidNotFinish)

	all := f.stats("")
	assert.Nil(t, all.Year)
	assert.Equal(t, 4, all.Totals.Finished)
	assert.Equal(t, 1, all.Totals.Rereads)
	assert.Equal(t, 1, all.Totals.DidNotFinish)
	require.Len(t, all.Periods, 2, "all time charts by year")
	assert.Equal(t, 2025, all.Periods[0].Year)
	assert.Nil(t, all.Periods[0].Month)
	assert.Equal(t, 1, all.Periods[0].Finished)
	assert.Equal(t, 3, all.Periods[1].Finished)
}

func TestStats_ReadThroughsWithNoEndDayCountOnlyUnderAllTime(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	emma := createBook(t, f.db, "Emma")
	f.finished(dune, "", "")
	seedReadThrough(t, f.db, f.user, emma, "2026-01-05", "", models.ReadThroughOutcomeAbandoned)

	year := f.stats("?year=2026")
	assert.Equal(t, 0, year.Totals.Finished)
	assert.Equal(t, 0, year.Totals.DidNotFinish)
	assert.Empty(t, year.Finished)

	all := f.stats("")
	assert.Equal(t, 1, all.Totals.Finished)
	assert.Equal(t, 1, all.Totals.DidNotFinish)
	assert.Nil(t, all.Totals.MedianDaysToFinish, "no Read-through has both days")
	require.Len(t, all.Finished, 1)
	assert.Nil(t, all.Finished[0].DaysTaken)
	assert.Nil(t, all.Finished[0].Seconds, "no Sessions")
}

func TestStats_EmptyRangeHasZeroesAndEmptyLists(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)

	resp := f.stats("?year=2026")
	assert.Equal(t, ReadingStatsTotals{}, resp.Totals)
	assert.Len(t, resp.Periods, 12)
	assert.NotNil(t, resp.Years)
	assert.Empty(t, resp.Years)
	assert.NotNil(t, resp.Finished)
	assert.NotNil(t, resp.Genres.Items)
	assert.Nil(t, resp.Genres.Other)
	assert.NotNil(t, resp.Kinds)
	assert.NotNil(t, resp.Ratings)
	assert.NotNil(t, resp.Places)

	assert.Empty(t, f.stats("").Periods, "all time with no activity has no years to chart")
}

func TestStats_ASessionCrossingMidnightIsSplitAtLocalMidnight(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	f.setTimeZone("America/Chicago")
	book := createBook(t, f.db, "Dune")
	file := f.addFile(book, models.FileTypeEPUB)
	f.session(sessionSpec{
		file:  file,
		start: at(t, "America/Chicago", "2026-04-30 23:30"),
		end:   at(t, "America/Chicago", "2026-05-01 00:30"),
	})

	resp := f.stats("?year=2026")
	assert.Equal(t, 1800, month(resp, 4).ReadingSeconds)
	assert.Equal(t, 1800, month(resp, 5).ReadingSeconds)
	assert.Equal(t, 3600, resp.Totals.ReadingSeconds)
	assert.Equal(t, []int{2026}, resp.Years)
}

// A session's stored duration is what counts, split by the share of its
// span on each side of midnight.
func TestStats_ADurationShorterThanItsSpanSplitsInProportion(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	file := f.addFile(book, models.FileTypeEPUB)
	f.session(sessionSpec{
		file:     file,
		start:    at(t, "UTC", "2026-04-30 23:00"),
		end:      at(t, "UTC", "2026-05-01 01:00"),
		duration: 1200,
		source:   models.ReadingSourceKOReader,
	})

	resp := f.stats("?year=2026")
	assert.Equal(t, 600, month(resp, 4).ReadingSeconds)
	assert.Equal(t, 600, month(resp, 5).ReadingSeconds)
	assert.Equal(t, 1200, resp.Totals.ReadingSeconds, "estimated Sessions count the same")
}

// 2027-10-31 is 25 hours long in Berlin: clocks go back an hour that
// morning, so its midnight is 25 hours after the one before.
func TestStats_ASessionOnADaylightSavingDaySplitsAtTheRealMidnight(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	f.setTimeZone("Europe/Berlin")
	book := createBook(t, f.db, "Dune")
	file := f.addFile(book, models.FileTypeEPUB)
	f.session(sessionSpec{
		file:  file,
		start: at(t, "Europe/Berlin", "2027-10-31 23:30"),
		end:   at(t, "Europe/Berlin", "2027-11-01 00:30"),
	})

	resp := f.stats("?year=2027")
	assert.Equal(t, 1800, month(resp, 10).ReadingSeconds)
	assert.Equal(t, 1800, month(resp, 11).ReadingSeconds)
}

func TestStats_SessionsNearAYearBoundaryLandInTheUsersYear(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	f.setTimeZone("America/Chicago")
	book := createBook(t, f.db, "Dune")
	file := f.addFile(book, models.FileTypeEPUB)
	// 2027-01-01 in UTC, but New Year's Eve in Chicago.
	f.session(sessionSpec{
		file:  file,
		start: at(t, "UTC", "2027-01-01 03:00"),
		end:   at(t, "UTC", "2027-01-01 04:00"),
	})
	// Across midnight into 2027 in Chicago.
	f.session(sessionSpec{
		file:  file,
		start: at(t, "America/Chicago", "2027-12-31 23:00"),
		end:   at(t, "America/Chicago", "2028-01-01 01:00"),
	})

	assert.Equal(t, 3600, f.stats("?year=2026").Totals.ReadingSeconds)
	assert.Equal(t, 3600, month(f.stats("?year=2026"), 12).ReadingSeconds)
	assert.Equal(t, 3600, f.stats("?year=2027").Totals.ReadingSeconds)
	assert.Equal(t, 3600, f.stats("?year=2028").Totals.ReadingSeconds)

	all := f.stats("")
	assert.Equal(t, []int{2028, 2027, 2026}, all.Years)
	assert.Equal(t, 3*3600, all.Totals.ReadingSeconds)
}

func TestStats_AUserWithNoTimeZoneIsGroupedInUTC(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	file := f.addFile(book, models.FileTypeEPUB)
	f.session(sessionSpec{
		file:  file,
		start: at(t, "UTC", "2026-12-31 23:30"),
		end:   at(t, "UTC", "2027-01-01 00:30"),
	})

	assert.Equal(t, 1800, f.stats("?year=2026").Totals.ReadingSeconds)
	assert.Equal(t, 1800, f.stats("?year=2027").Totals.ReadingSeconds)
}

func TestStats_ReadingAndListeningAreKeptApart(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	ebook := f.addFile(book, models.FileTypeEPUB)
	audio := f.addFile(book, models.FileTypeM4B)
	rt := f.finished(book, "2026-03-01", "2026-03-02")
	f.session(sessionSpec{file: ebook, readThrough: rt, start: at(t, "UTC", "2026-03-01 10:00"), end: at(t, "UTC", "2026-03-01 10:30")})
	f.session(sessionSpec{file: audio, readThrough: rt, start: at(t, "UTC", "2026-03-02 10:00"), end: at(t, "UTC", "2026-03-02 11:00")})
	// A Session whose File was deleted keeps the kind it recorded.
	gone := f.addFile(book, models.FileTypeM4B)
	f.session(sessionSpec{file: gone, start: at(t, "UTC", "2026-03-03 10:00"), end: at(t, "UTC", "2026-03-03 10:15")})
	_, err := f.db.NewDelete().Model((*models.File)(nil)).Where("id = ?", gone.ID).Exec(f.ctx)
	require.NoError(t, err)

	resp := f.stats("?year=2026")
	assert.Equal(t, 1800, resp.Totals.ReadingSeconds)
	assert.Equal(t, 4500, resp.Totals.ListeningSeconds)
	assert.Equal(t, 1800, month(resp, 3).ReadingSeconds)
	assert.Equal(t, 4500, month(resp, 3).ListeningSeconds)
	require.Len(t, resp.Finished, 1)
	require.NotNil(t, resp.Finished[0].Seconds)
	assert.Equal(t, 5400, *resp.Finished[0].Seconds, "only the Read-through's own Sessions")
	assert.ElementsMatch(t, []ReadingStatsKindCount{
		{Kind: ReadingStatsKindEbook, Books: 1},
		{Kind: ReadingStatsKindAudiobook, Books: 1},
	}, resp.Kinds)
}

func TestStats_BreakdownsCountEachBookOnceUnderEachName(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	emma := createBook(t, f.db, "Emma")
	comic := createBook(t, f.db, "Saga")
	f.addFile(dune, models.FileTypeEPUB)
	f.addFile(emma, models.FileTypePDF)
	f.addFile(comic, models.FileTypeCBZ)
	f.addGenre(dune, "Science Fiction")
	f.addGenre(dune, "Classics")
	f.addGenre(emma, "Classics")
	f.addTag(dune, "Desert")
	addAuthor(t, f.db, dune, "Frank Herbert", 1)
	addAuthor(t, f.db, emma, "Jane Austen", 1)
	f.finished(dune, "2026-01-01", "2026-01-10")
	f.finished(dune, "2026-06-01", "2026-06-10")
	f.finished(emma, "2026-02-01", "2026-02-10")
	f.finished(comic, "2026-03-01", "2026-03-01")

	resp := f.stats("?year=2026")
	assert.Equal(t, []ReadingStatsCount{{Name: "Classics", Books: 2}, {Name: "Science Fiction", Books: 1}}, resp.Genres.Items,
		"a reread doesn't count its Book twice, and the same name in two libraries is one Genre")
	assert.Equal(t, []ReadingStatsCount{{Name: "Desert", Books: 1}}, resp.Tags.Items)
	assert.Equal(t, []ReadingStatsCount{{Name: "Frank Herbert", Books: 1}, {Name: "Jane Austen", Books: 1}}, resp.Authors.Items)
	assert.Equal(t, []ReadingStatsKindCount{
		{Kind: ReadingStatsKindEbook, Books: 2},
		{Kind: ReadingStatsKindComic, Books: 1},
	}, resp.Kinds, "with no Sessions, the Book's Files say the kind")
}

func TestStats_KindComesFromTheFilesEachReadThroughWasReadIn(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	day := func(d string) time.Time { return at(t, "UTC", "2026-03-"+d+" 10:00") }

	// A comic read in its CBZ, though the Book also has an EPUB.
	saga := createBook(t, f.db, "Saga")
	cbz := f.addFile(saga, models.FileTypeCBZ)
	f.addFile(saga, models.FileTypeEPUB)
	rt := f.finished(saga, "2026-03-01", "2026-03-01")
	f.session(sessionSpec{file: cbz, readThrough: rt, start: day("01"), end: day("01").Add(time.Hour)})

	// No Sessions: the Book's Files say it's an audiobook.
	dune := createBook(t, f.db, "Dune")
	f.addFile(dune, models.FileTypeM4B)
	f.finished(dune, "2026-03-02", "2026-03-02")

	// Its only Session's File is gone and was read, not listened to, so the
	// Book's remaining Files decide.
	emma := createBook(t, f.db, "Emma")
	gone := f.addFile(emma, models.FileTypeCBZ)
	f.addFile(emma, models.FileTypePDF)
	rt = f.finished(emma, "2026-03-03", "2026-03-03")
	f.session(sessionSpec{file: gone, readThrough: rt, start: day("03"), end: day("03").Add(time.Hour)})

	// Its only Session's File is gone, but it was listening.
	ubik := createBook(t, f.db, "Ubik")
	audio := f.addFile(ubik, models.FileTypeM4B)
	f.addFile(ubik, models.FileTypeEPUB)
	rt = f.finished(ubik, "2026-03-04", "2026-03-04")
	f.session(sessionSpec{file: audio, readThrough: rt, start: day("04"), end: day("04").Add(time.Hour)})

	// No Sessions and no Files.
	bare := createBook(t, f.db, "Bare")
	f.finished(bare, "2026-03-05", "2026-03-05")

	_, err := f.db.NewDelete().Model((*models.File)(nil)).Where("id IN (?)", bun.List([]int{gone.ID, audio.ID})).Exec(f.ctx)
	require.NoError(t, err)

	assert.Equal(t, []ReadingStatsKindCount{
		{Kind: ReadingStatsKindAudiobook, Books: 2},
		{Kind: ReadingStatsKindEbook, Books: 1},
		{Kind: ReadingStatsKindComic, Books: 1},
		{Kind: ReadingStatsKindUnknown, Books: 1},
	}, f.stats("?year=2026").Kinds)
}

func TestStats_BreakdownsFoldTheRestIntoOther(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	for i := range breakdownLimit + 2 {
		book := createBook(t, f.db, "Book "+string(rune('A'+i)))
		f.addGenre(book, "Genre "+string(rune('A'+i)))
		if i == 0 {
			f.addGenre(book, "Genre Z")
		}
		f.finished(book, "2026-01-01", "2026-01-02")
	}

	resp := f.stats("?year=2026")
	assert.Len(t, resp.Genres.Items, breakdownLimit)
	require.NotNil(t, resp.Genres.Other)
	assert.Equal(t, 3, resp.Genres.Other.Names)
	assert.Equal(t, 3, resp.Genres.Other.Books, "distinct Books under any folded name")
}

func TestStats_RatingsCountEachFinishedBookOnceAtItsCurrentRating(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	emma := createBook(t, f.db, "Emma")
	unrated := createBook(t, f.db, "Unrated")
	elsewhere := createBook(t, f.db, "Elsewhere")
	f.finished(dune, "2026-01-01", "2026-01-10")
	f.finished(dune, "2026-06-01", "2026-06-10")
	f.finished(emma, "2026-02-01", "2026-02-10")
	f.finished(unrated, "2026-02-01", "2026-02-10")
	f.finished(elsewhere, "2025-02-01", "2025-02-10")
	f.rate(dune, 18)
	f.rate(emma, 12)
	f.rate(elsewhere, 4)

	resp := f.stats("?year=2026")
	assert.Equal(t, []ReadingStatsRatingCount{{Rating: 3, Books: 1}, {Rating: 4.5, Books: 1}}, resp.Ratings)
	require.NotNil(t, resp.Totals.AverageRating)
	assert.InDelta(t, 3.75, *resp.Totals.AverageRating, 0.001)

	assert.Nil(t, f.stats("?year=2024").Totals.AverageRating)
}

func TestStats_DetachedReadThroughsCountButAreLeftOutOfBreakdowns(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	file := f.addFile(dune, models.FileTypeEPUB)
	f.addGenre(dune, "Science Fiction")
	addAuthor(t, f.db, dune, "Frank Herbert", 1)
	rt := f.finished(dune, "2026-01-01", "2026-01-10")
	f.session(sessionSpec{file: file, readThrough: rt, start: at(t, "UTC", "2026-01-02 10:00"), end: at(t, "UTC", "2026-01-02 11:00")})
	f.rate(dune, 20)
	deleteBook(t, f.db, dune)

	resp := f.stats("?year=2026")
	assert.Equal(t, 1, resp.Totals.Finished)
	assert.Equal(t, 3600, resp.Totals.ReadingSeconds)
	assert.Empty(t, resp.Genres.Items)
	assert.Empty(t, resp.Authors.Items)
	assert.Empty(t, resp.Kinds)
	assert.Empty(t, resp.Ratings, "the Rating went with the Book")
	require.Len(t, resp.Finished, 1)
	row := resp.Finished[0]
	assert.Equal(t, "Dune", row.Title)
	assert.Equal(t, []string{"Frank Herbert"}, row.Authors)
	assert.True(t, row.Removed)
	assert.Nil(t, row.Book, "a removed Book has no link")
	require.NotNil(t, row.Seconds)
	assert.Equal(t, 3600, *row.Seconds)
}

func TestStats_ABookInALibraryOutOfReachStillCountsWithNoLink(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dune := createBook(t, f.db, "Dune")
	f.addGenre(dune, "Science Fiction")
	f.finished(dune, "2026-01-01", "2026-01-10")

	f.user.LibraryAccess = []*models.UserLibraryAccess{{UserID: f.user.ID, LibraryID: intp(dune.LibraryID + 100)}}
	resp := statsFor(t, newReadingAPI(t, f.db, f.user), "?year=2026")
	assert.Equal(t, 1, resp.Totals.Finished)
	assert.Equal(t, []ReadingStatsCount{{Name: "Science Fiction", Books: 1}}, resp.Genres.Items)
	require.Len(t, resp.Finished, 1)
	assert.Equal(t, "Dune", resp.Finished[0].Title)
	assert.False(t, resp.Finished[0].Removed)
	assert.Nil(t, resp.Finished[0].Book, "no link to a Book the user can't reach")
}

func TestStats_TimeIsSplitByWhereItWasRead(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	book := createBook(t, f.db, "Dune")
	ebook := f.addFile(book, models.FileTypeEPUB)
	audio := f.addFile(book, models.FileTypeM4B)
	addDevice := func(user *models.User, id, name, deviceType string) string {
		_, err := f.db.ExecContext(f.ctx, `INSERT INTO devices (id, user_id, name, device_type, credential, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, id, user.ID, name, deviceType, "cred-"+id, time.Now(), time.Now())
		require.NoError(t, err)
		return id
	}
	kobo := addDevice(f.user, "kobo-1", "Bedside Kobo", "kobo")
	koreader := addDevice(f.user, "koreader-1", "Boox", "koreader")
	gone := addDevice(f.user, "koreader-2", "Old Phone", "koreader")
	day := func(hour string) time.Time { return at(t, "UTC", "2026-03-01 "+hour) }

	f.session(sessionSpec{file: ebook, start: day("08:00"), end: day("08:10")})
	f.session(sessionSpec{file: audio, start: day("09:00"), end: day("09:20")})
	f.session(sessionSpec{file: ebook, start: day("10:00"), end: day("11:00"), source: models.ReadingSourceKobo, deviceID: &kobo})
	f.session(sessionSpec{file: ebook, start: day("12:00"), end: day("12:25"), source: models.ReadingSourceKOReader, deviceID: &koreader})
	f.session(sessionSpec{file: ebook, start: day("13:00"), end: day("13:05"), source: models.ReadingSourceKOReader, deviceID: &gone})
	f.session(sessionSpec{file: ebook, start: day("14:00"), end: day("14:02"), source: models.ReadingSourceKobo})
	_, err := f.db.ExecContext(f.ctx, `DELETE FROM devices WHERE id = ?`, gone)
	require.NoError(t, err)

	// Another user's Device and time never appear.
	other := createUser(t, f.db, "other", 2)
	theirs := addDevice(other, "kobo-9", "Their Kobo", "kobo")
	seedSession(t, f.db, other, sessionSpec{file: ebook, start: day("15:00"), end: day("17:00"), source: models.ReadingSourceKobo, deviceID: &theirs})

	name := func(s string) *string { return &s }
	assert.Equal(t, []ReadingStatsPlace{
		{Source: models.ReadingSourceKobo, DeviceName: name("Bedside Kobo"), ReadingSeconds: 3600},
		{Source: models.ReadingSourceWebReader, ReadingSeconds: 600, ListeningSeconds: 1200},
		{Source: models.ReadingSourceKOReader, DeviceName: name("Boox"), ReadingSeconds: 1500},
		{Source: models.ReadingSourceKOReader, ReadingSeconds: 300},
		{Source: models.ReadingSourceKobo, ReadingSeconds: 120},
	}, f.stats("?year=2026").Places)
}

func TestStats_NeverShowsAnotherUsersReading(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	other := createUser(t, f.db, "other", 2)
	book := createBook(t, f.db, "Dune")
	file := f.addFile(book, models.FileTypeEPUB)
	f.addGenre(book, "Science Fiction")
	seedReadThrough(t, f.db, other, book, "2026-01-01", "2026-01-10", models.ReadThroughOutcomeFinished)
	seedReadThrough(t, f.db, other, book, "2026-02-01", "2026-02-10", models.ReadThroughOutcomeAbandoned)
	seedSession(t, f.db, other, sessionSpec{file: file, start: at(t, "UTC", "2026-01-02 10:00"), end: at(t, "UTC", "2026-01-02 11:00")})
	units := 20
	_, err := f.db.NewInsert().Model(&models.UserBook{UserID: other.ID, BookID: book.ID, CreatedAt: time.Now(), UpdatedAt: time.Now(), Rating: &units}).Exec(f.ctx)
	require.NoError(t, err)

	resp := f.stats("?year=2026")
	assert.Equal(t, ReadingStatsTotals{}, resp.Totals)
	assert.Empty(t, resp.Years)
	assert.Empty(t, resp.Finished)
	assert.Empty(t, resp.Genres.Items)
	assert.Empty(t, resp.Ratings)
	assert.Empty(t, resp.Places)

	assert.Equal(t, 1, statsFor(t, newReadingAPI(t, f.db, other), "?year=2026").Totals.Finished)
}

func TestStats_RejectsAnInvalidYear(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	assert.Equal(t, http.StatusUnprocessableEntity, f.api.do(http.MethodGet, "/stats?year=0", nil).Code)
}
