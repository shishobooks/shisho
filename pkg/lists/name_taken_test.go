package lists

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const nameTakenMessage = "You already have a list with this name."

func jsonContext(e *echo.Echo, user *models.User, method, body string) echo.Context {
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c := e.NewContext(req, httptest.NewRecorder())
	auth.SetUser(c, user)
	return c
}

func assertNameTaken(t *testing.T, err error) {
	t.Helper()
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr, "want an errcodes error, got %T: %v", err, err)
	assert.Equal(t, http.StatusUnprocessableEntity, codeErr.HTTPCode)
	assert.Equal(t, "validation_error", codeErr.Code)
	assert.Equal(t, nameTakenMessage, codeErr.Message)
}

// List names are unique per owner, ignoring case.
func TestService_CreateList_NameTakenIgnoringCase(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	svc := NewService(db)
	owner := createTestUser(t, db, "owner")
	other := createTestUser(t, db, "other")
	_, err := svc.CreateList(t.Context(), CreateListOptions{UserID: owner.ID, Name: "Favorites"})
	require.NoError(t, err)

	_, err = svc.CreateList(t.Context(), CreateListOptions{UserID: owner.ID, Name: "favorites"})
	require.ErrorIs(t, err, ErrListNameTaken)

	_, err = svc.CreateList(t.Context(), CreateListOptions{UserID: other.ID, Name: "Favorites"})
	require.NoError(t, err, "another user may use the same name")
}

func TestService_UpdateList_RenameToTakenName(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	svc := NewService(db)
	owner := createTestUser(t, db, "owner")
	_, err := svc.CreateList(t.Context(), CreateListOptions{UserID: owner.ID, Name: "Favorites"})
	require.NoError(t, err)
	list, err := svc.CreateList(t.Context(), CreateListOptions{UserID: owner.ID, Name: "Classics"})
	require.NoError(t, err)

	list.Name = "FAVORITES"
	err = svc.UpdateList(t.Context(), list, UpdateListOptions{Columns: []string{"name"}})
	require.ErrorIs(t, err, ErrListNameTaken)

	list.Name = "classics"
	err = svc.UpdateList(t.Context(), list, UpdateListOptions{Columns: []string{"name"}})
	require.NoError(t, err, "a list may change the case of its own name")
}

func TestCreate_NameTakenIs422(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	e := newTestEcho(t)
	h := newTestHandler(db)
	user := createTestUser(t, db, "owner")
	_, err := h.listsService.CreateList(t.Context(), CreateListOptions{UserID: user.ID, Name: "Favorites"})
	require.NoError(t, err)

	c := jsonContext(e, user, http.MethodPost, `{"name":"favorites"}`)
	assertNameTaken(t, h.create(c))
}

func TestUpdate_RenameToTakenNameIs422(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	e := newTestEcho(t)
	h := newTestHandler(db)
	user := createTestUser(t, db, "owner")
	_, err := h.listsService.CreateList(t.Context(), CreateListOptions{UserID: user.ID, Name: "Favorites"})
	require.NoError(t, err)
	list, err := h.listsService.CreateList(t.Context(), CreateListOptions{UserID: user.ID, Name: "Classics"})
	require.NoError(t, err)

	c := jsonContext(e, user, http.MethodPatch, `{"name":"Favorites"}`)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(list.ID))
	assertNameTaken(t, h.update(c))
}

func TestCreateFromTemplate_NameTakenIs422(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	e := newTestEcho(t)
	h := newTestHandler(db)
	user := createTestUser(t, db, "owner")
	_, err := h.listsService.CreateList(t.Context(), CreateListOptions{UserID: user.ID, Name: "Favorites"})
	require.NoError(t, err)

	c := jsonContext(e, user, http.MethodPost, "")
	c.SetParamNames("name")
	c.SetParamValues("favorites")
	assertNameTaken(t, h.createFromTemplate(c))
}

// Only the name index maps to the 422: any other failed insert is a 500.
func TestCreate_DatabaseFailureIs500(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	e := newTestEcho(t)
	h := newTestHandler(db)
	user := createTestUser(t, db, "owner")
	_, err := db.Exec(`CREATE TRIGGER fail_list_insert BEFORE INSERT ON lists
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)

	c := jsonContext(e, user, http.MethodPost, `{"name":"Favorites"}`)
	err = h.create(c)
	require.Error(t, err)
	var codeErr *errcodes.Error
	assert.NotErrorAs(t, err, &codeErr, "a failed insert is not a 4xx")
}

func TestUpdate_DatabaseFailureIs500(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	e := newTestEcho(t)
	h := newTestHandler(db)
	user := createTestUser(t, db, "owner")
	list, err := h.listsService.CreateList(t.Context(), CreateListOptions{UserID: user.ID, Name: "Classics"})
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TRIGGER fail_list_update BEFORE UPDATE ON lists
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)

	c := jsonContext(e, user, http.MethodPatch, `{"name":"Favorites"}`)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(list.ID))
	err = h.update(c)
	require.Error(t, err)
	var codeErr *errcodes.Error
	assert.NotErrorAs(t, err, &codeErr, "a failed update is not a 4xx")
}
