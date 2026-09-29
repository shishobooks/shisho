package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRequireContext(user *models.User) echo.Context {
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	if user != nil {
		SetUser(c, user)
	}
	return c
}

func requireStatus(t *testing.T, err error, status int) {
	t.Helper()
	var ec *errcodes.Error
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, status, ec.HTTPCode)
}

func TestRequireUser_NoUser_Returns401(t *testing.T) {
	t.Parallel()
	_, err := RequireUser(newRequireContext(nil))
	requireStatus(t, err, http.StatusUnauthorized)
}

func TestRequireUser_TypedNilUser_Returns401(t *testing.T) {
	t.Parallel()
	c := newRequireContext(nil)
	SetUser(c, (*models.User)(nil))
	_, err := RequireUser(c)
	requireStatus(t, err, http.StatusUnauthorized)
}

func TestRequireUser_ReturnsStoredUser(t *testing.T) {
	t.Parallel()
	user := &models.User{ID: 7}
	got, err := RequireUser(newRequireContext(user))
	require.NoError(t, err)
	assert.Same(t, user, got)
}

func TestRequireLibraryAccessFor(t *testing.T) {
	t.Parallel()
	libraryID := 3
	scoped := &models.User{LibraryAccess: []*models.UserLibraryAccess{{LibraryID: &libraryID}}}
	all := &models.User{LibraryAccess: []*models.UserLibraryAccess{{LibraryID: nil}}}

	requireStatus(t, RequireLibraryAccessFor(newRequireContext(nil), libraryID), http.StatusUnauthorized)
	requireStatus(t, RequireLibraryAccessFor(newRequireContext(scoped), libraryID+1), http.StatusForbidden)
	require.NoError(t, RequireLibraryAccessFor(newRequireContext(scoped), libraryID))
	require.NoError(t, RequireLibraryAccessFor(newRequireContext(all), libraryID+1))
}
