package auth

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

// userContextKey is the echo context key every authenticating middleware
// (Authenticate, BasicAuth, and apikeys.Middleware) stores the user under.
const userContextKey = "user"

// SetUser stores the authenticated user for RequireUser to read. Only
// authenticating middleware, and tests standing in for it, call it.
func SetUser(c echo.Context, user *models.User) {
	c.Set(userContextKey, user)
}

// RequireUser returns the user an authenticating middleware stored in the
// context, or a 401 when there is none. Handlers call it instead of reading
// the context themselves, so a route registered without its middleware fails
// closed rather than skipping the checks that need a user.
func RequireUser(c echo.Context) (*models.User, error) {
	user, ok := c.Get(userContextKey).(*models.User)
	if !ok || user == nil {
		return nil, errcodes.AuthenticationRequired()
	}
	return user, nil
}

// RequireLibraryAccessFor checks that the context's user can access
// libraryID. It returns 401 when there is no user and 403 when the user
// lacks access. Use it when the library ID comes from a loaded entity rather
// than a route param (for a param, mount RequireLibraryAccess instead).
func RequireLibraryAccessFor(c echo.Context, libraryID int) error {
	user, err := RequireUser(c)
	if err != nil {
		return err
	}
	if !user.HasLibraryAccess(libraryID) {
		return errcodes.Forbidden("You don't have access to this library")
	}
	return nil
}
