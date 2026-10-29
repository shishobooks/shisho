package reading

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// readingAPI drives the reading routes as one signed-in user.
type readingAPI struct {
	t    *testing.T
	db   *bun.DB
	e    *echo.Echo
	user *models.User
}

func newReadingAPI(t *testing.T, db *bun.DB, user *models.User) *readingAPI {
	t.Helper()
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	g := e.Group("/reading", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			auth.SetUser(c, user)
			return next(c)
		}
	})
	RegisterRoutes(g, db)
	return &readingAPI{t: t, db: db, e: e, user: user}
}

func (a *readingAPI) do(method, path string, body any) *httptest.ResponseRecorder {
	a.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(a.t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "/reading"+path, reader)
	if body != nil {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	a.e.ServeHTTP(rec, req)
	return rec
}

// ok runs a request that must succeed and returns the reading record.
func (a *readingAPI) ok(method, path string, body any) BookReadingResponse {
	a.t.Helper()
	rec := a.do(method, path, body)
	require.Equal(a.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp BookReadingResponse
	require.NoError(a.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func (a *readingAPI) get(bookID int) BookReadingResponse {
	a.t.Helper()
	return a.ok(http.MethodGet, fmt.Sprintf("/books/%d", bookID), nil)
}

func (a *readingAPI) transition(bookID int, action string, today string) *httptest.ResponseRecorder {
	a.t.Helper()
	var body any
	if today != "" {
		body = DayPayload{Today: today}
	}
	return a.do(http.MethodPost, fmt.Sprintf("/books/%d/%s", bookID, action), body)
}

func (a *readingAPI) mustTransition(bookID int, action string, today string) BookReadingResponse {
	a.t.Helper()
	rec := a.transition(bookID, action, today)
	require.Equal(a.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp BookReadingResponse
	require.NoError(a.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func createUser(t *testing.T, db *bun.DB, username string, roleID int) *models.User {
	t.Helper()
	user := &models.User{Username: username, PasswordHash: "test", RoleID: roleID, IsActive: true}
	_, err := db.NewInsert().Model(user).Exec(context.Background())
	require.NoError(t, err)
	// A nil library ID grants every library.
	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID}}
	return user
}

func createBook(t *testing.T, db *bun.DB, title string) *models.Book {
	t.Helper()
	library := &models.Library{Name: "Lib " + title, CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(context.Background())
	require.NoError(t, err)
	book := &models.Book{
		LibraryID:    library.ID,
		Title:        title,
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    title,
		AuthorSource: models.DataSourceFilepath,
		Filepath:     "/test/" + title,
	}
	_, err = db.NewInsert().Model(book).Exec(context.Background())
	require.NoError(t, err)
	return book
}

func status(resp BookReadingResponse) string {
	if resp.Status == nil {
		return ""
	}
	return *resp.Status
}

func strp(s string) *string { return &s }

func TestRetrieve_UnreadBookHasNoStatus(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")

	resp := api.get(book.ID)
	assert.Equal(t, book.ID, resp.BookID)
	assert.Nil(t, resp.Status)
	assert.Nil(t, resp.Rating)
	assert.False(t, resp.OnToRead)
	assert.Empty(t, resp.ReadThroughs)
	assert.NotNil(t, resp.ReadThroughs, "read_throughs is an empty array, not null")
}

func TestRetrieve_MissingBookIs404(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))

	assert.Equal(t, http.StatusNotFound, api.do(http.MethodGet, "/books/999", nil).Code)
	assert.Equal(t, http.StatusNotFound, api.transition(999, "start", "2026-10-01").Code)
}

func TestRetrieve_NoLibraryAccessIsForbidden(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	other := createBook(t, db, "Other")
	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID, LibraryID: intp(other.LibraryID + 100)}}
	api := newReadingAPI(t, db, user)

	assert.Equal(t, http.StatusForbidden, api.do(http.MethodGet, fmt.Sprintf("/books/%d", other.ID), nil).Code)
	assert.Equal(t, http.StatusForbidden, api.transition(other.ID, "start", "2026-10-01").Code)
}

func intp(i int) *int { return &i }

func TestStatusDerivation(t *testing.T) {
	t.Parallel()

	type readThrough struct {
		begin, end *string
		outcome    string
		paused     bool
	}
	underway := func(paused bool) readThrough {
		return readThrough{begin: strp("2026-01-01"), outcome: models.ReadThroughOutcomeUnderway, paused: paused}
	}
	ended := func(outcome string, end *string) readThrough {
		return readThrough{end: end, outcome: outcome}
	}
	finished := models.ReadThroughOutcomeFinished
	abandoned := models.ReadThroughOutcomeAbandoned

	cases := []struct {
		name     string
		history  []readThrough // in the order they were recorded
		onToRead bool
		want     string
	}{
		{"nothing", nil, false, ""},
		{"underway", []readThrough{underway(false)}, false, models.ReadingStatusCurrentlyReading},
		{"underway and paused", []readThrough{underway(true)}, false, models.ReadingStatusPaused},
		{"finished", []readThrough{ended(finished, strp("2026-02-01"))}, false, models.ReadingStatusRead},
		{"abandoned", []readThrough{ended(abandoned, strp("2026-02-01"))}, false, models.ReadingStatusDidNotFinish},
		{"underway beats an earlier finish", []readThrough{ended(finished, strp("2026-02-01")), underway(false)}, false, models.ReadingStatusCurrentlyReading},
		{"paused beats an earlier abandon", []readThrough{ended(abandoned, strp("2026-02-01")), underway(true)}, false, models.ReadingStatusPaused},
		{"latest end day wins over recording order", []readThrough{ended(finished, strp("2026-05-01")), ended(abandoned, strp("2026-03-01"))}, false, models.ReadingStatusRead},
		{"latest end day wins, abandoned later", []readThrough{ended(finished, strp("2026-03-01")), ended(abandoned, strp("2026-05-01"))}, false, models.ReadingStatusDidNotFinish},
		{"undated ranks below dated", []readThrough{ended(abandoned, strp("2026-03-01")), ended(finished, nil)}, false, models.ReadingStatusDidNotFinish},
		{"tie goes to the one recorded last", []readThrough{ended(finished, strp("2026-03-01")), ended(abandoned, strp("2026-03-01"))}, false, models.ReadingStatusDidNotFinish},
		{"both undated, recorded last wins", []readThrough{ended(abandoned, nil), ended(finished, nil)}, false, models.ReadingStatusRead},
		{"on To Read", nil, true, models.ReadingStatusToRead},
		{"To Read beats a finish", []readThrough{ended(finished, strp("2026-02-01"))}, true, models.ReadingStatusToRead},
		{"underway beats To Read", []readThrough{underway(false)}, true, models.ReadingStatusCurrentlyReading},
		{"paused beats To Read", []readThrough{underway(true)}, true, models.ReadingStatusPaused},
		{"To Read beats a Did Not Finish", []readThrough{ended(abandoned, strp("2026-02-01"))}, true, models.ReadingStatusToRead},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			user := createUser(t, db, "reader", 2)
			api := newReadingAPI(t, db, user)
			book := createBook(t, db, "Dune")
			ctx := context.Background()
			for _, rt := range tc.history {
				_, err := db.NewInsert().Model(&models.ReadThrough{
					UserID: user.ID, BookID: book.ID, BeginDay: rt.begin, EndDay: rt.end,
					Outcome: rt.outcome, Paused: rt.paused,
				}).Exec(ctx)
				require.NoError(t, err)
			}
			if tc.onToRead {
				// To Read has no API in this phase yet; record the position directly.
				_, err := db.NewInsert().Model(&models.UserBook{UserID: user.ID, BookID: book.ID, ToReadPosition: intp(1)}).Exec(ctx)
				require.NoError(t, err)
			}

			assert.Equal(t, tc.want, status(api.get(book.ID)))
		})
	}
}

func TestTransitions_StartPauseResumeFinishReadAgainAbandon(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")

	resp := api.mustTransition(book.ID, "start", "2026-03-01")
	assert.Equal(t, models.ReadingStatusCurrentlyReading, status(resp))
	require.Len(t, resp.ReadThroughs, 1)
	assert.Equal(t, strp("2026-03-01"), resp.ReadThroughs[0].BeginDay)
	assert.Nil(t, resp.ReadThroughs[0].EndDay)
	assert.Equal(t, models.ReadThroughOutcomeUnderway, resp.ReadThroughs[0].Outcome)

	resp = api.mustTransition(book.ID, "pause", "")
	assert.Equal(t, models.ReadingStatusPaused, status(resp))

	resp = api.mustTransition(book.ID, "resume", "")
	assert.Equal(t, models.ReadingStatusCurrentlyReading, status(resp))
	assert.Equal(t, strp("2026-03-01"), resp.ReadThroughs[0].BeginDay, "resuming keeps the start day")

	resp = api.mustTransition(book.ID, "finish", "2026-03-20")
	assert.Equal(t, models.ReadingStatusRead, status(resp))
	require.Len(t, resp.ReadThroughs, 1)
	assert.Equal(t, models.ReadThroughOutcomeFinished, resp.ReadThroughs[0].Outcome)
	assert.Equal(t, strp("2026-03-20"), resp.ReadThroughs[0].EndDay)

	// Reading again starts a new Read-through and keeps the first.
	resp = api.mustTransition(book.ID, "start", "2026-06-01")
	assert.Equal(t, models.ReadingStatusCurrentlyReading, status(resp))
	require.Len(t, resp.ReadThroughs, 2)
	assert.Equal(t, models.ReadThroughOutcomeUnderway, resp.ReadThroughs[0].Outcome, "the one underway is listed first")
	assert.Equal(t, models.ReadThroughOutcomeFinished, resp.ReadThroughs[1].Outcome)

	resp = api.mustTransition(book.ID, "abandon", "2026-06-10")
	assert.Equal(t, models.ReadingStatusDidNotFinish, status(resp))
	require.Len(t, resp.ReadThroughs, 2)
	assert.Equal(t, models.ReadThroughOutcomeAbandoned, resp.ReadThroughs[0].Outcome)
	assert.Equal(t, strp("2026-06-10"), resp.ReadThroughs[0].EndDay)
	assert.Equal(t, models.ReadThroughOutcomeFinished, resp.ReadThroughs[1].Outcome)
}

func TestTransitions_FinishWithNothingUnderwayCreatesUndatedStart(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")

	resp := api.mustTransition(book.ID, "finish", "2026-10-28")
	assert.Equal(t, models.ReadingStatusRead, status(resp))
	require.Len(t, resp.ReadThroughs, 1)
	assert.Nil(t, resp.ReadThroughs[0].BeginDay)
	assert.Equal(t, strp("2026-10-28"), resp.ReadThroughs[0].EndDay)
	assert.Equal(t, models.ReadThroughOutcomeFinished, resp.ReadThroughs[0].Outcome)
}

func TestTransitions_FinishingAPausedReadThroughEndsIt(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")

	api.mustTransition(book.ID, "start", "2026-03-01")
	api.mustTransition(book.ID, "pause", "")
	resp := api.mustTransition(book.ID, "finish", "2026-04-01")
	assert.Equal(t, models.ReadingStatusRead, status(resp))
	require.Len(t, resp.ReadThroughs, 1)
	assert.False(t, resp.ReadThroughs[0].Paused)
}

func TestTransitions_StartTakesBookOffToRead(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	book := createBook(t, db, "Dune")
	_, err := db.NewInsert().Model(&models.UserBook{UserID: user.ID, BookID: book.ID, ToReadPosition: intp(1)}).Exec(context.Background())
	require.NoError(t, err)

	resp := api.mustTransition(book.ID, "start", "2026-03-01")
	assert.False(t, resp.OnToRead)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, status(resp))
}

func TestTransitions_InvalidOnesAreRefused(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		setup  []string // actions run first
		action string
	}{
		{"pause with nothing underway", nil, "pause"},
		{"resume with nothing underway", nil, "resume"},
		{"abandon with nothing underway", nil, "abandon"},
		{"pause after finishing", []string{"start", "finish"}, "pause"},
		{"start while underway", []string{"start"}, "start"},
		{"start while paused", []string{"start", "pause"}, "start"},
		{"pause while paused", []string{"start", "pause"}, "pause"},
		{"resume while not paused", []string{"start"}, "resume"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
			book := createBook(t, db, "Dune")
			for _, action := range tc.setup {
				api.mustTransition(book.ID, action, "2026-03-01")
			}
			before := api.get(book.ID)

			rec := api.transition(book.ID, tc.action, "2026-03-02")
			assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), "invalid_state")
			assert.Equal(t, before, api.get(book.ID), "a refused transition changes nothing")
		})
	}
}

func TestTransitions_DatedOnesRequireToday(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")

	for _, action := range []string{"start", "finish", "abandon"} {
		assert.Equal(t, http.StatusBadRequest, api.transition(book.ID, action, "").Code, action)
		for _, day := range []string{"2026-13-40", "2026-02-30", "yesterday"} {
			assert.Equal(t, http.StatusUnprocessableEntity, api.transition(book.ID, action, day).Code, "%s %s", action, day)
		}
	}
	assert.Nil(t, api.get(book.ID).Status)
}

func TestOneUnderwayPerUserAndBook_EnforcedByDatabase(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	book := createBook(t, db, "Dune")
	ctx := context.Background()

	insert := func(outcome string) error {
		_, err := db.NewInsert().Model(&models.ReadThrough{UserID: user.ID, BookID: book.ID, Outcome: outcome}).Exec(ctx)
		return err
	}
	require.NoError(t, insert(models.ReadThroughOutcomeUnderway))
	require.NoError(t, insert(models.ReadThroughOutcomeFinished))
	require.NoError(t, insert(models.ReadThroughOutcomeFinished))
	assert.Error(t, insert(models.ReadThroughOutcomeUnderway))
}

func (a *readingAPI) patchReadThrough(id int, begin, end *string) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(http.MethodPatch, fmt.Sprintf("/read-throughs/%d", id), UpdateReadThroughPayload{BeginDay: begin, EndDay: end})
}

func TestUpdateReadThrough_EditsAndClearsDays(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")
	api.mustTransition(book.ID, "start", "2026-03-01")
	resp := api.mustTransition(book.ID, "finish", "2026-03-20")
	id := resp.ReadThroughs[0].ID

	rec := api.patchReadThrough(id, strp("2026-02-15"), strp("2026-03-18"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	resp = api.get(book.ID)
	assert.Equal(t, strp("2026-02-15"), resp.ReadThroughs[0].BeginDay)
	assert.Equal(t, strp("2026-03-18"), resp.ReadThroughs[0].EndDay)

	rec = api.patchReadThrough(id, nil, strp("2026-03-18"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	resp = api.get(book.ID)
	assert.Nil(t, resp.ReadThroughs[0].BeginDay)
	assert.Equal(t, models.ReadingStatusRead, status(resp))
}

func TestUpdateReadThrough_EditingAnEndDayChangesStatus(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")
	first := api.mustTransition(book.ID, "finish", "2026-03-01").ReadThroughs[0]
	api.mustTransition(book.ID, "start", "2026-04-01")
	resp := api.mustTransition(book.ID, "abandon", "2026-04-10")
	assert.Equal(t, models.ReadingStatusDidNotFinish, status(resp))

	rec := api.patchReadThrough(first.ID, nil, strp("2026-05-01"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, models.ReadingStatusRead, status(api.get(book.ID)))
}

func TestUpdateReadThrough_RejectsInvalidDays(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")
	underway := api.mustTransition(book.ID, "start", "2026-03-01").ReadThroughs[0]

	assert.Equal(t, http.StatusUnprocessableEntity, api.patchReadThrough(underway.ID, strp("2026-03-01"), strp("2026-03-05")).Code,
		"a Read-through underway has no end day")
	assert.Equal(t, http.StatusUnprocessableEntity, api.patchReadThrough(underway.ID, strp("not-a-day"), nil).Code)

	api.mustTransition(book.ID, "finish", "2026-03-20")
	assert.Equal(t, http.StatusUnprocessableEntity, api.patchReadThrough(underway.ID, strp("2026-04-01"), strp("2026-03-20")).Code,
		"the end day can't come before the begin day")
	assert.Equal(t, strp("2026-03-01"), api.get(book.ID).ReadThroughs[0].BeginDay)
}

func TestDeleteReadThrough(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")
	first := api.mustTransition(book.ID, "finish", "2026-03-01").ReadThroughs[0]
	api.mustTransition(book.ID, "start", "2026-04-01")
	resp := api.mustTransition(book.ID, "abandon", "2026-04-10")
	second := resp.ReadThroughs[0]

	rec := api.do(http.MethodDelete, fmt.Sprintf("/read-throughs/%d", second.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	resp = api.get(book.ID)
	assert.Equal(t, models.ReadingStatusRead, status(resp))
	require.Len(t, resp.ReadThroughs, 1)
	assert.Equal(t, first.ID, resp.ReadThroughs[0].ID)

	assert.Equal(t, http.StatusNotFound, api.do(http.MethodDelete, fmt.Sprintf("/read-throughs/%d", second.ID), nil).Code)
}

func TestDeleteReadThrough_UnderwayFreesStart(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")
	underway := api.mustTransition(book.ID, "start", "2026-03-01").ReadThroughs[0]

	rec := api.do(http.MethodDelete, fmt.Sprintf("/read-throughs/%d", underway.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Nil(t, api.get(book.ID).Status)
	api.mustTransition(book.ID, "start", "2026-03-02")
}

func (a *readingAPI) setRating(bookID int, rating *float64) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(http.MethodPut, fmt.Sprintf("/books/%d/rating", bookID), SetRatingPayload{Rating: rating})
}

func TestRating_SetAndClear(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")

	for _, stars := range []float64{0.25, 3.75, 5} {
		rec := api.setRating(book.ID, &stars)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		got := api.get(book.ID).Rating
		require.NotNil(t, got)
		assert.InDelta(t, stars, *got, 0.0001)
	}

	rec := api.setRating(book.ID, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Nil(t, api.get(book.ID).Rating)
}

func TestRating_RejectsValuesOffTheQuarterStarScale(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")

	for _, stars := range []float64{0, 0.1, 3.3, 5.25, -1} {
		rec := api.setRating(book.ID, &stars)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, "%v: %s", stars, rec.Body.String())
	}
}

func TestRating_DoesNotDependOnStatus(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")
	api.mustTransition(book.ID, "start", "2026-03-01")
	api.mustTransition(book.ID, "abandon", "2026-03-02")

	stars := 1.5
	require.Equal(t, http.StatusOK, api.setRating(book.ID, &stars).Code)
	resp := api.get(book.ID)
	assert.Equal(t, models.ReadingStatusDidNotFinish, status(resp))
	require.NotNil(t, resp.Rating)

	// It survives a reread.
	api.mustTransition(book.ID, "start", "2026-05-01")
	assert.NotNil(t, api.get(book.ID).Rating)
}

func TestPrivacy_OtherUsersAndAdminsSeeNothing(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	owner := newReadingAPI(t, db, createUser(t, db, "owner", 2))
	other := newReadingAPI(t, db, createUser(t, db, "other", 2))
	admin := newReadingAPI(t, db, createUser(t, db, "admin", 1))
	book := createBook(t, db, "Dune")

	resp := owner.mustTransition(book.ID, "finish", "2026-03-01")
	readThroughID := resp.ReadThroughs[0].ID
	owner.mustTransition(book.ID, "start", "2026-04-01")
	stars := 4.0
	require.Equal(t, http.StatusOK, owner.setRating(book.ID, &stars).Code)

	for _, api := range []*readingAPI{other, admin} {
		resp := api.get(book.ID)
		assert.Nil(t, resp.Status, api.user.Username)
		assert.Nil(t, resp.Rating, api.user.Username)
		assert.Empty(t, resp.ReadThroughs, api.user.Username)

		// Another user's Read-throughs don't exist for them.
		assert.Equal(t, http.StatusNotFound, api.patchReadThrough(readThroughID, nil, nil).Code, api.user.Username)
		assert.Equal(t, http.StatusNotFound, api.do(http.MethodDelete, fmt.Sprintf("/read-throughs/%d", readThroughID), nil).Code, api.user.Username)

		// Their own transitions don't touch the owner's records.
		api.mustTransition(book.ID, "start", "2026-05-01")
	}

	resp = owner.get(book.ID)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, status(resp))
	assert.Len(t, resp.ReadThroughs, 2)
	require.NotNil(t, resp.Rating)
	assert.InDelta(t, 4.0, *resp.Rating, 0.0001)
}

func TestAttachReading_SetsOnlyTheUsersOwnStatusAndRating(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ownerUser := createUser(t, db, "owner", 2)
	otherUser := createUser(t, db, "other", 2)
	owner := newReadingAPI(t, db, ownerUser)
	reading := createBook(t, db, "Reading")
	finished := createBook(t, db, "Finished")
	untouched := createBook(t, db, "Untouched")
	owner.mustTransition(reading.ID, "start", "2026-03-01")
	owner.mustTransition(finished.ID, "finish", "2026-03-01")
	stars := 4.5
	require.Equal(t, http.StatusOK, owner.setRating(finished.ID, &stars).Code)

	books := []*models.Book{{ID: reading.ID}, {ID: finished.ID}, {ID: untouched.ID}}
	require.NoError(t, AttachReading(context.Background(), db, ownerUser.ID, books))
	assert.Equal(t, strp(models.ReadingStatusCurrentlyReading), books[0].ReadingStatus)
	assert.Equal(t, strp(models.ReadingStatusRead), books[1].ReadingStatus)
	assert.Nil(t, books[2].ReadingStatus)
	assert.Nil(t, books[0].Rating)
	require.NotNil(t, books[1].Rating)
	assert.InDelta(t, 4.5, *books[1].Rating, 0.0001)

	others := []*models.Book{{ID: reading.ID}, {ID: finished.ID}}
	require.NoError(t, AttachReading(context.Background(), db, otherUser.ID, others))
	assert.Nil(t, others[0].ReadingStatus)
	assert.Nil(t, others[1].ReadingStatus)
	assert.Nil(t, others[1].Rating, "another user's Rating stays private")
}

// A failed lookup is a 500, not a 404.
func TestRetrieve_DatabaseFailureIs500(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")
	_, err := db.Exec("DROP TABLE read_throughs")
	require.NoError(t, err)

	assert.Equal(t, http.StatusInternalServerError, api.do(http.MethodGet, fmt.Sprintf("/books/%d", book.ID), nil).Code)
	assert.Equal(t, http.StatusInternalServerError, api.transition(book.ID, "pause", "").Code)
	assert.Equal(t, http.StatusInternalServerError, api.patchReadThrough(1, nil, nil).Code)
}

// The Book lookup's 404 is only for a missing row: a Book that can't be read
// is a 500.
func TestRetrieve_BookLookupFailureIs500(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")
	_, err := db.Exec("DROP TABLE books")
	require.NoError(t, err)

	assert.Equal(t, http.StatusInternalServerError, api.do(http.MethodGet, fmt.Sprintf("/books/%d", book.ID), nil).Code)
}

// A Read-through in a library the user can no longer reach is hidden, not
// deleted: it's back when access returns.
func TestReadThrough_HiddenWithoutLibraryAccess(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	book := createBook(t, db, "Dune")
	rt := newReadingAPI(t, db, user).mustTransition(book.ID, "finish", "2026-03-01").ReadThroughs[0]

	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID, LibraryID: intp(book.LibraryID + 100)}}
	api := newReadingAPI(t, db, user)
	assert.Equal(t, http.StatusNotFound, api.patchReadThrough(rt.ID, nil, nil).Code)
	assert.Equal(t, http.StatusNotFound, api.do(http.MethodDelete, fmt.Sprintf("/read-throughs/%d", rt.ID), nil).Code)

	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID}}
	assert.Len(t, newReadingAPI(t, db, user).get(book.ID).ReadThroughs, 1)
}
