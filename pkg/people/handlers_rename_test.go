package people

import (
	"context"
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

func callPersonUpdate(t *testing.T, h *handler, user *models.User, id int, body string) error {
	t.Helper()
	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(id))
	auth.SetUser(c, user)
	return h.update(c)
}

// Renaming a Person onto another Person's name (in any case) is rejected
// instead of tripping ux_persons_name_library_id.
func TestUpdatePerson_RenameToExistingName(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newTestHandler(db)
	lib := createTestLibrary(t, db)
	seedPersonWithAuthoredBooks(t, db, lib, "Brandon Sanderson", []string{"Mistborn"})
	other := seedPersonWithAuthoredBooks(t, db, lib, "Robert Jordan", []string{"The Eye of the World"})

	err := callPersonUpdate(t, h, userWithLibraryAccess(lib.ID), other.ID, `{"name":"brandon sanderson"}`)
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, http.StatusUnprocessableEntity, codeErr.HTTPCode)

	unchanged, err := h.personService.RetrievePerson(context.Background(), RetrievePersonOptions{ID: &other.ID})
	require.NoError(t, err)
	assert.Equal(t, "Robert Jordan", unchanged.Name)
}

// Changing only the case of a Person's own name is not a collision.
func TestUpdatePerson_RenameCaseOnly_Succeeds(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newTestHandler(db)
	lib := createTestLibrary(t, db)
	person := seedPersonWithAuthoredBooks(t, db, lib, "Robert Jordan", []string{"The Eye of the World"})

	require.NoError(t, callPersonUpdate(t, h, userWithLibraryAccess(lib.ID), person.ID, `{"name":"ROBERT JORDAN"}`))

	renamed, err := h.personService.RetrievePerson(context.Background(), RetrievePersonOptions{ID: &person.ID})
	require.NoError(t, err)
	assert.Equal(t, "ROBERT JORDAN", renamed.Name)
}
