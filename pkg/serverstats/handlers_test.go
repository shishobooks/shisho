package serverstats

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/cache"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// fakeCaches stands in for the cache admin page's sizes.
type fakeCaches []cache.Info

func (f fakeCaches) Sizes() ([]cache.Info, error) { return f, nil }

// failingCaches can't measure its caches.
type failingCaches struct{}

func (failingCaches) Sizes() ([]cache.Info, error) { return nil, errors.New("permission denied") }

type statsFixture struct {
	t    *testing.T
	ctx  context.Context
	db   *bun.DB
	cfg  *config.Config
	h    *handler
	user *models.User
}

// The fixed clock every fixture reads: late in 2026, in the server's zone.
var fixtureNow = time.Date(2026, time.November, 11, 12, 0, 0, 0, time.UTC)

func newStatsFixture(t *testing.T) *statsFixture {
	t.Helper()
	db := testdb.New(t)
	cfg := config.NewForTest()
	cfg.CacheDir = t.TempDir()
	h := newHandler(db, cfg, fakeCaches{})
	h.now = func() time.Time { return fixtureNow }
	h.location = time.UTC
	// Every path is on one volume unless a test says otherwise.
	h.statVolume = func(string, bool) (volumeStat, error) {
		return volumeStat{id: 1, total: 100, free: 40, used: 60}, nil
	}
	f := &statsFixture{t: t, ctx: context.Background(), db: db, cfg: cfg, h: h}
	f.user = f.createUser("admin")
	// An admin: Libraries Read and every library.
	f.user.Role = &models.Role{Permissions: []*models.Permission{
		{Resource: models.ResourceConfig, Operation: models.OperationRead},
		{Resource: models.ResourceLibraries, Operation: models.OperationRead},
	}}
	f.user.LibraryAccess = []*models.UserLibraryAccess{{UserID: f.user.ID}}
	return f
}

func (f *statsFixture) insert(model any) {
	f.t.Helper()
	_, err := f.db.NewInsert().Model(model).Exec(f.ctx)
	require.NoError(f.t, err)
}

func (f *statsFixture) createUser(username string) *models.User {
	f.t.Helper()
	user := &models.User{Username: username, PasswordHash: "test", RoleID: 1, IsActive: true}
	f.insert(user)
	return user
}

func (f *statsFixture) library(name string, paths ...string) *models.Library {
	f.t.Helper()
	lib := &models.Library{Name: name, CoverAspectRatio: models.CoverAspectRatioBook}
	f.insert(lib)
	for _, p := range paths {
		f.insert(&models.LibraryPath{LibraryID: lib.ID, Filepath: p})
	}
	return lib
}

func (f *statsFixture) book(lib *models.Library, title string, added time.Time) *models.Book {
	f.t.Helper()
	book := &models.Book{
		CreatedAt:    added,
		UpdatedAt:    added,
		LibraryID:    lib.ID,
		Title:        title,
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    title,
		AuthorSource: models.DataSourceFilepath,
		Filepath:     "/test/" + title,
	}
	f.insert(book)
	return book
}

func (f *statsFixture) file(book *models.Book, fileType, role string, size int64) {
	f.t.Helper()
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		Filepath:      filepath.Join(book.Filepath, book.Title+"-"+role+"."+fileType),
		FileType:      fileType,
		FileRole:      role,
		FilesizeBytes: size,
	}
	f.insert(file)
}

func (f *statsFixture) session(user *models.User, kind string, started time.Time, seconds int) {
	f.t.Helper()
	f.insert(&models.ReadingSession{
		CreatedAt: started,
		UserID:    user.ID,
		Source:    models.ReadingSourceWebReader,
		StartedAt: started,
		EndedAt:   started.Add(time.Duration(seconds) * time.Second),
		Measured:  true,
		Kind:      kind,
		Duration:  seconds,
	})
}

func (f *statsFixture) readThrough(user *models.User, bookID *int, outcome string, endDay *string) {
	f.t.Helper()
	rt := &models.ReadThrough{
		CreatedAt: fixtureNow,
		UpdatedAt: fixtureNow,
		UserID:    user.ID,
		BookID:    bookID,
		EndDay:    endDay,
		Outcome:   outcome,
	}
	if bookID == nil {
		title := "Deleted Book"
		rt.BookTitle = &title
	}
	f.insert(rt)
}

func (f *statsFixture) get() (*httptest.ResponseRecorder, StatsResponse) {
	f.t.Helper()
	e := echo.New()
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	e.GET("/server-stats", f.h.retrieve, func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			auth.SetUser(c, f.user)
			return next(c)
		}
	})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/server-stats", nil))
	var resp StatsResponse
	if rec.Code == http.StatusOK {
		require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	}
	return rec, resp
}

func (f *statsFixture) mustGet() StatsResponse {
	f.t.Helper()
	rec, resp := f.get()
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	return resp
}

func strp(s string) *string { return &s }

func TestServerStats_CountsContentPerLibraryAndInTotal(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	added := time.Date(2026, time.March, 3, 0, 0, 0, 0, time.UTC)

	fiction := f.library("Fiction")
	dune := f.book(fiction, "Dune", added)
	f.file(dune, models.FileTypeEPUB, models.FileRoleMain, 1000)
	f.file(dune, models.FileTypeM4B, models.FileRoleMain, 5000)
	f.file(dune, models.FileTypePDF, models.FileRoleSupplement, 300)
	emma := f.book(fiction, "Emma", added)
	f.file(emma, models.FileTypeEPUB, models.FileRoleMain, 700)
	f.insert(&models.Series{LibraryID: fiction.ID, Name: "Dune", NameSource: models.DataSourceFilepath, SortName: "Dune", SortNameSource: models.DataSourceFilepath})
	f.insert(&models.Person{LibraryID: fiction.ID, Name: "Frank Herbert", SortName: "Herbert, Frank", SortNameSource: models.DataSourceFilepath})
	f.insert(&models.Person{LibraryID: fiction.ID, Name: "Jane Austen", SortName: "Austen, Jane", SortNameSource: models.DataSourceFilepath})
	f.insert(&models.Genre{LibraryID: fiction.ID, Name: "Science Fiction"})
	f.insert(&models.Tag{LibraryID: fiction.ID, Name: "Classic"})
	f.insert(&models.Tag{LibraryID: fiction.ID, Name: "Desert"})
	f.insert(&models.Publisher{LibraryID: fiction.ID, Name: "Ace"})

	comics := f.library("Comics")
	saga := f.book(comics, "Saga", added)
	f.file(saga, models.FileTypeCBZ, models.FileRoleMain, 2000)
	f.library("Empty")

	resp := f.mustGet()

	require.Len(t, resp.Libraries, 3)
	assert.Equal(t, []string{"Comics", "Empty", "Fiction"},
		[]string{resp.Libraries[0].Name, resp.Libraries[1].Name, resp.Libraries[2].Name}, "ordered by name")

	assert.Equal(t, LibraryStats{ID: fiction.ID, Name: "Fiction", ContentCounts: ContentCounts{
		Books: 2,
		Files: []FileTypeCount{
			{FileType: models.FileTypeEPUB, Count: 2},
			{FileType: models.FileTypeM4B, Count: 1},
		},
		SupplementFiles: 1,
		Series:          1,
		Persons:         2,
		Genres:          1,
		Tags:            2,
		Publishers:      1,
		SizeBytes:       7000,
	}}, resp.Libraries[2])
	assert.Equal(t, ContentCounts{
		Books:     1,
		Files:     []FileTypeCount{{FileType: models.FileTypeCBZ, Count: 1}},
		SizeBytes: 2000,
	}, resp.Libraries[0].ContentCounts)
	assert.Equal(t, ContentCounts{Files: []FileTypeCount{}}, resp.Libraries[1].ContentCounts)

	assert.Equal(t, ContentCounts{
		Books: 3,
		Files: []FileTypeCount{
			{FileType: models.FileTypeCBZ, Count: 1},
			{FileType: models.FileTypeEPUB, Count: 2},
			{FileType: models.FileTypeM4B, Count: 1},
		},
		SupplementFiles: 1,
		Series:          1,
		Persons:         2,
		Genres:          1,
		Tags:            2,
		Publishers:      1,
		SizeBytes:       9000,
	}, resp.Totals)
}

func TestServerStats_UsersAndDevices(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	reader := f.createUser("reader")
	_, err := f.db.ExecContext(f.ctx,
		`INSERT INTO devices (id, user_id, name, device_type, credential, scope_type, delivery_rules_mode, created_at, updated_at)
		 VALUES ('d1', ?, 'Kobo', 'kobo', 'c1', 'all', 'user', ?, ?), ('d2', ?, 'Phone', 'koreader', 'c2', 'all', 'user', ?, ?)`,
		reader.ID, fixtureNow, fixtureNow, f.user.ID, fixtureNow, fixtureNow)
	require.NoError(t, err)

	resp := f.mustGet()
	assert.Equal(t, 2, resp.Users)
	assert.Equal(t, 2, resp.Devices)
}

func TestServerStats_GrowthByMonthAdded(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	lib := f.library("Fiction")
	f.book(lib, "One", time.Date(2026, time.August, 31, 23, 0, 0, 0, time.UTC))
	f.book(lib, "Two", time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC))
	f.book(lib, "Three", time.Date(2026, time.November, 10, 0, 0, 0, 0, time.UTC))

	resp := f.mustGet()
	assert.Equal(t, []GrowthMonth{
		{Month: "2026-08", Books: 1},
		{Month: "2026-09", Books: 0},
		{Month: "2026-10", Books: 0},
		{Month: "2026-11", Books: 2},
	}, resp.Growth)
}

func TestServerStats_GrowthUsesServerTimeZone(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	chicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)
	f.h.location = chicago
	lib := f.library("Fiction")
	// Still October 31 in Chicago.
	f.book(lib, "Late", time.Date(2026, time.November, 1, 3, 0, 0, 0, time.UTC))

	resp := f.mustGet()
	assert.Equal(t, []GrowthMonth{
		{Month: "2026-10", Books: 1},
		{Month: "2026-11", Books: 0},
	}, resp.Growth)
}

func TestServerStats_EmptyServer(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	resp := f.mustGet()
	assert.Empty(t, resp.Libraries)
	assert.NotNil(t, resp.Libraries)
	assert.Equal(t, []GrowthMonth{}, resp.Growth)
	assert.Equal(t, ContentCounts{Files: []FileTypeCount{}}, resp.Totals)
}

func TestServerStats_ReadingTotals(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	lib := f.library("Fiction")
	book := f.book(lib, "Dune", fixtureNow)
	recent := f.createUser("recent")
	lapsed := f.createUser("lapsed")

	// This year: 1h reading and 30m listening.
	f.session(recent, models.ReadingKindReading, fixtureNow.AddDate(0, 0, -5), 3600)
	f.session(recent, models.ReadingKindListening, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), 1800)
	// Last year only counts toward all time.
	f.session(lapsed, models.ReadingKindReading, time.Date(2025, time.December, 31, 23, 0, 0, 0, time.UTC), 600)
	f.session(lapsed, models.ReadingKindListening, time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC), 1200)

	f.readThrough(recent, &book.ID, models.ReadThroughOutcomeFinished, strp("2026-02-01"))
	// A reread of the same Book counts again.
	f.readThrough(lapsed, &book.ID, models.ReadThroughOutcomeFinished, strp("2026-10-01"))
	// A Read-through whose Book was deleted keeps counting.
	f.readThrough(lapsed, nil, models.ReadThroughOutcomeFinished, strp("2026-03-01"))
	f.readThrough(lapsed, nil, models.ReadThroughOutcomeFinished, strp("2025-12-31"))
	// Finished with no end day counts only for all time.
	f.readThrough(recent, &book.ID, models.ReadThroughOutcomeFinished, nil)
	f.readThrough(recent, &book.ID, models.ReadThroughOutcomeAbandoned, strp("2026-04-01"))
	f.readThrough(recent, &book.ID, models.ReadThroughOutcomeUnderway, nil)

	resp := f.mustGet()
	assert.Equal(t, ReadingTotals{
		Year: 2026,
		ThisYear: ReadingPeriod{
			ReadingSeconds:       3600,
			ListeningSeconds:     1800,
			ReadThroughsFinished: 3,
		},
		AllTime: ReadingPeriod{
			ReadingSeconds:       4200,
			ListeningSeconds:     3000,
			ReadThroughsFinished: 5,
		},
		ActiveUsers: 1,
	}, resp.Reading)
}

func TestServerStats_ActiveUsersInLast30Days(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	a := f.createUser("a")
	b := f.createUser("b")
	c := f.createUser("c")
	f.session(a, models.ReadingKindReading, fixtureNow.Add(-2*time.Hour), 60)
	f.session(a, models.ReadingKindReading, fixtureNow.AddDate(0, 0, -3), 60)
	// Started 31 days ago but ran into the window.
	f.session(b, models.ReadingKindReading, fixtureNow.AddDate(0, 0, -30).Add(-time.Hour), 2*3600)
	f.session(c, models.ReadingKindReading, fixtureNow.AddDate(0, 0, -31), 60)

	assert.Equal(t, 2, f.mustGet().Reading.ActiveUsers)
}

func TestServerStats_ReadingTotalsUseServerTimeZone(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	chicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)
	f.h.location = chicago
	reader := f.createUser("reader")
	// January 1 in UTC, still December 31 in Chicago.
	f.session(reader, models.ReadingKindReading, time.Date(2026, time.January, 1, 3, 0, 0, 0, time.UTC), 600)
	f.session(reader, models.ReadingKindReading, time.Date(2026, time.January, 1, 7, 0, 0, 0, time.UTC), 60)

	resp := f.mustGet()
	assert.Equal(t, int64(60), resp.Reading.ThisYear.ReadingSeconds)
	assert.Equal(t, int64(660), resp.Reading.AllTime.ReadingSeconds)
}

func TestServerStats_NothingIdentifiesAUserOrABook(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	lib := f.library("Fiction")
	book := f.book(lib, "Secret Diary Of Someone", fixtureNow)
	reader := f.createUser("privatereader")
	f.session(reader, models.ReadingKindReading, fixtureNow.AddDate(0, 0, -1), 600)
	f.readThrough(reader, &book.ID, models.ReadThroughOutcomeFinished, strp("2026-11-01"))

	rec, _ := f.get()
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, "privatereader")
	assert.NotContains(t, body, "Secret Diary")
	assert.NotContains(t, body, `"user_id"`)
	assert.NotContains(t, body, `"book_id"`)
}

func TestServerStats_VolumesReportEachMountOnce(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	f.cfg.DatabaseFilePath = "/data/shisho.db"
	f.cfg.CacheDir = "/fast/cache"
	f.h = newHandler(f.db, f.cfg, fakeCaches{})
	f.h.now = func() time.Time { return fixtureNow }
	f.h.location = time.UTC
	f.h.statVolume = func(path string, _ bool) (volumeStat, error) {
		switch path {
		case "/fast/cache":
			return volumeStat{id: 2, total: 500, free: 400, used: 100}, nil
		default:
			return volumeStat{id: 1, total: 1000, free: 250, used: 750}, nil
		}
	}
	f.library("Books", "/media/books", "/media/more-books")
	f.library("Audio", "/media/audio")

	resp := f.mustGet()
	assert.Equal(t, []Volume{
		{
			Paths: []VolumePath{
				{Kind: VolumePathKindLibrary, Name: "Audio", Path: "/media/audio"},
				{Kind: VolumePathKindLibrary, Name: "Books", Path: "/media/books"},
				{Kind: VolumePathKindLibrary, Name: "Books", Path: "/media/more-books"},
				{Kind: VolumePathKindData, Name: "Data", Path: "/data"},
			},
			TotalBytes: 1000, UsedBytes: 750, FreeBytes: 250,
		},
		{
			Paths:      []VolumePath{{Kind: VolumePathKindCache, Name: "Cache", Path: "/fast/cache"}},
			TotalBytes: 500, UsedBytes: 100, FreeBytes: 400,
		},
	}, resp.Volumes)
}

func (f *statsFixture) fileAt(book *models.Book, path string, size int64) {
	f.t.Helper()
	f.insert(&models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		Filepath:      path,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: size,
	})
}

// Each volume says how much of it is Shisho's: library files, the
// database, and the caches. Each listed path says how much it holds.
func TestServerStats_VolumesSayWhatShishoKeepsOnThem(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shisho.db")
	require.NoError(t, os.WriteFile(dbPath, make([]byte, 4000), 0o600))
	f.h.databasePath = dbPath
	f.h.cacheDir = "/fast/cache"
	f.h.caches = fakeCaches{
		{ID: "downloads", Name: "Downloads", SizeBytes: 300},
		{ID: "cbz_pages", Name: "CBZ Pages", SizeBytes: 50},
	}
	f.h.statVolume = func(path string, _ bool) (volumeStat, error) {
		if path == "/fast/cache" {
			return volumeStat{id: 2, total: 5000, free: 4000, used: 1000}, nil
		}
		return volumeStat{id: 1, total: 100000, free: 25000, used: 75000}, nil
	}
	added := time.Date(2026, time.March, 3, 0, 0, 0, 0, time.UTC)
	books := f.library("Books", "/media/books", "/media/más libros")
	dune := f.book(books, "Dune", added)
	f.fileAt(dune, "/media/books/Dune/Dune.epub", 1000)
	f.fileAt(dune, "/media/books/Dune/cover.jpg", 200)
	f.fileAt(dune, "/media/más libros/Emma.epub", 30)
	// A sibling folder whose name starts the same isn't inside /media/books.
	f.fileAt(dune, "/media/books-extra/Other.epub", 7)
	hidden := f.library("Hidden", "/media/hidden")
	secret := f.book(hidden, "Secret", added)
	f.fileAt(secret, "/media/hidden/Secret.epub", 500)
	f.user.LibraryAccess = []*models.UserLibraryAccess{{UserID: f.user.ID, LibraryID: &books.ID}}

	resp := f.mustGet()
	require.Len(t, resp.Volumes, 2)
	assert.Equal(t, Volume{
		Paths: []VolumePath{
			{Kind: VolumePathKindLibrary, Name: "Books", Path: "/media/books", SizeBytes: 1200},
			{Kind: VolumePathKindLibrary, Name: "Books", Path: "/media/más libros", SizeBytes: 30},
			{Kind: VolumePathKindData, Name: "Data", Path: dir, SizeBytes: 4000},
		},
		TotalBytes: 100000, UsedBytes: 75000, FreeBytes: 25000,
		// The hidden library's files count toward the volume, unnamed.
		LibraryBytes:  1730,
		DatabaseBytes: 4000,
	}, resp.Volumes[0])
	assert.Equal(t, Volume{
		Paths:      []VolumePath{{Kind: VolumePathKindCache, Name: "Cache", Path: "/fast/cache", SizeBytes: 350}},
		TotalBytes: 5000, UsedBytes: 1000, FreeBytes: 4000,
		CacheBytes: 350,
	}, resp.Volumes[1])
}

func TestServerStats_VolumeThatCannotBeReadIsLeftOut(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	f.h.statVolume = func(path string, _ bool) (volumeStat, error) {
		if path == f.cfg.CacheDir {
			return volumeStat{}, errors.New("permission denied")
		}
		return volumeStat{id: 1, total: 10, free: 5, used: 5}, nil
	}
	f.library("Books", "/media/books")
	resp := f.mustGet()
	require.Len(t, resp.Volumes, 1)
	assert.Equal(t, []VolumePath{{Kind: VolumePathKindLibrary, Name: "Books", Path: "/media/books"}}, resp.Volumes[0].Paths)
}

// The real stat call reports a plausible volume for a directory that exists.
func TestStatVolume_RealDirectory(t *testing.T) {
	t.Parallel()
	stat, err := statVolume(t.TempDir(), true)
	require.NoError(t, err)
	assert.Positive(t, stat.total)
	assert.LessOrEqual(t, stat.free, stat.total)
	assert.LessOrEqual(t, stat.used, stat.total)
}

// A cache or data directory that does not exist yet is reported on the
// volume of its nearest existing parent, where it will be created.
func TestStatVolume_MissingPathUsesParent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	parent, err := statVolume(dir, false)
	require.NoError(t, err)
	missing, err := statVolume(filepath.Join(dir, "not", "yet"), false)
	require.NoError(t, err)
	assert.Equal(t, parent.id, missing.id)
}

// A missing library folder is an error, not its parent's volume: a drive
// that isn't mounted leaves its mount point behind on another filesystem.
func TestStatVolume_MissingLibraryFolderIsAnError(t *testing.T) {
	t.Parallel()
	_, err := statVolume(filepath.Join(t.TempDir(), "unmounted"), true)
	require.Error(t, err)
}

// A stat that never returns (a hung network mount) leaves that path out
// instead of holding up the page.
func TestServerStats_HungVolumeIsLeftOut(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	f.h.statTimeout = 50 * time.Millisecond
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	f.h.statVolume = func(path string, _ bool) (volumeStat, error) {
		if path == "/mnt/nas" {
			<-block
		}
		return volumeStat{id: 1, total: 10, free: 5, used: 5}, nil
	}
	f.library("NAS", "/mnt/nas")
	f.library("Local", "/media/local")

	resp := f.mustGet()
	require.Len(t, resp.Volumes, 1)
	for _, p := range resp.Volumes[0].Paths {
		assert.NotEqual(t, "/mnt/nas", p.Path)
	}
}

// One cache that can't be measured is left out rather than failing the page.
func TestServerStats_CacheErrorLeavesThatCacheOut(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	f.h.caches = failingCaches{}
	resp := f.mustGet()
	assert.Equal(t, []CacheSize{}, resp.Caches)
}

// A Book dated in the future counts in the current month, and one with no
// real date in the first month with a real one, so the chart stays bounded
// and adds up to the total.
func TestServerStats_GrowthFoldsImpossibleDates(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	lib := f.library("Fiction")
	f.book(lib, "Real", time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC))
	f.book(lib, "Future", fixtureNow.AddDate(1, 0, 0))
	f.book(lib, "Zero", time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC))

	assert.Equal(t, []GrowthMonth{
		{Month: "2026-10", Books: 2},
		{Month: "2026-11", Books: 1},
	}, f.mustGet().Growth)
}

func TestServerStats_DatabaseAndCacheSizes(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shisho.db")
	require.NoError(t, os.WriteFile(dbPath, make([]byte, 4096), 0o600))
	require.NoError(t, os.WriteFile(dbPath+"-wal", make([]byte, 1024), 0o600))
	f.cfg.DatabaseFilePath = dbPath
	f.h.databasePath = dbPath
	f.h.caches = fakeCaches{
		{ID: "downloads", Name: "Downloads", Description: "Generated", SizeBytes: 300, FileCount: 2},
		{ID: "cbz_pages", Name: "CBZ Pages", Description: "Pages", SizeBytes: 50, FileCount: 5},
	}

	resp := f.mustGet()
	assert.Equal(t, int64(5120), resp.DatabaseBytes)
	assert.Equal(t, []CacheSize{
		{ID: "downloads", Name: "Downloads", SizeBytes: 300, FileCount: 2},
		{ID: "cbz_pages", Name: "CBZ Pages", SizeBytes: 50, FileCount: 5},
	}, resp.Caches)
}

func TestServerStats_DatabaseWithoutWriteAheadLog(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	dbPath := filepath.Join(t.TempDir(), "shisho.db")
	require.NoError(t, os.WriteFile(dbPath, make([]byte, 2048), 0o600))
	f.h.databasePath = dbPath
	assert.Equal(t, int64(2048), f.mustGet().DatabaseBytes)
}

func TestServerStats_FailedQueryIs500(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	_, err := f.db.ExecContext(f.ctx, `DROP TABLE reading_sessions`)
	require.NoError(t, err)
	rec, _ := f.get()
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// A Config Read role sees only the libraries it can access, and no library
// folders without Libraries Read, but the totals and volumes still cover the
// whole server.
func TestServerStats_LibraryAccessLimitsLibraryRowsAndFolders(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	added := time.Date(2026, time.March, 3, 0, 0, 0, 0, time.UTC)
	visible := f.library("Visible", "/media/visible")
	hidden := f.library("Hidden", "/media/hidden")
	f.book(visible, "One", added)
	f.book(hidden, "Two", added)
	f.book(hidden, "Three", added)
	f.h.statVolume = func(path string, _ bool) (volumeStat, error) {
		if path == "/media/hidden" {
			return volumeStat{id: 2, total: 10, free: 5, used: 5}, nil
		}
		return volumeStat{id: 1, total: 100, free: 40, used: 60}, nil
	}

	f.user.LibraryAccess = []*models.UserLibraryAccess{{UserID: f.user.ID, LibraryID: &visible.ID}}
	resp := f.mustGet()
	require.Len(t, resp.Libraries, 1)
	assert.Equal(t, "Visible", resp.Libraries[0].Name)
	assert.Equal(t, 3, resp.Totals.Books, "totals cover every library")
	// Libraries are listed by name, so Hidden's volume comes first.
	require.Len(t, resp.Volumes, 2, "every volume is still reported")
	assert.Empty(t, resp.Volumes[0].Paths, "a library the user can't access shows no folder")
	assert.Equal(t, VolumePath{Kind: VolumePathKindLibrary, Name: "Visible", Path: "/media/visible"}, resp.Volumes[1].Paths[0])

	f.user.Role = &models.Role{Permissions: []*models.Permission{
		{Resource: models.ResourceConfig, Operation: models.OperationRead},
	}}
	f.user.LibraryAccess = []*models.UserLibraryAccess{{UserID: f.user.ID}}
	resp = f.mustGet()
	assert.Len(t, resp.Libraries, 2, "library names follow library access")
	for _, v := range resp.Volumes {
		for _, p := range v.Paths {
			assert.NotEqual(t, VolumePathKindLibrary, p.Kind, "no library folders without Libraries Read")
		}
	}
}
