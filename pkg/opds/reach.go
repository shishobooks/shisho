package opds

import (
	"context"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/reach"
)

// OPDS browses what a request reaches (pkg/reach) a little differently from
// the eReader browser: a Device scoped to a list browses only that list, not
// the libraries its books are in, and a Device scoped to a library browses
// no lists.

// reachFor returns what the request's sign-in reaches, with the Delivery
// Rules it follows: the signed-in Device's, or the user's default when
// signed in with the account password.
func (h *handler) reachFor(c echo.Context) (*reach.Reach, error) {
	user, err := auth.RequireUser(c)
	if err != nil {
		return nil, err
	}
	return h.reachService.Resolve(c.Request().Context(), user, devices.SignedInDevice(c))
}

// scopeFor returns what the request's sign-in reaches without loading the
// Delivery Rules, for routes that serve one book or list nothing.
func (h *handler) scopeFor(c echo.Context) (*reach.Reach, error) {
	user, err := auth.RequireUser(c)
	if err != nil {
		return nil, err
	}
	return h.reachService.Scope(c.Request().Context(), user, devices.SignedInDevice(c))
}

// browsesLibraries reports whether the request browses libraries at all. A
// list-scoped Device browses only its list.
func browsesLibraries(r *reach.Reach) bool {
	return !r.Closed() && r.ListID() == nil
}

// browsesLists reports whether the request can open lists at all. A Device
// scoped to one library browses only that library.
func browsesLists(r *reach.Reach) bool {
	return !r.Closed() && !r.LibraryScoped()
}

// allowsList reports whether listID is a list the request can open. The
// caller still checks that the user can view it.
func allowsList(r *reach.Reach, listID int) bool {
	return browsesLists(r) && (r.ListID() == nil || *r.ListID() == listID)
}

// requireLibrary returns a 404 for a library path the request can't browse.
// The user's own access is checked first, so a library they can't access
// gets the usual library access error.
func requireLibrary(c echo.Context, r *reach.Reach, libraryID int) error {
	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}
	if !browsesLibraries(r) || !r.HoldsLibrary(libraryID) {
		return errcodes.NotFound("Library")
	}
	return nil
}

// requireBook returns resource's 404 for a book outside what the request
// reaches, the same response as an id with no row, so a Device can't probe
// for books its scope leaves out. The user's own library access is checked
// first.
func (h *handler) requireBook(ctx context.Context, c echo.Context, r *reach.Reach, libraryID, bookID int, resource string) error {
	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}
	holds, err := h.reachService.HoldsBook(ctx, r, libraryID, bookID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !holds {
		return errcodes.NotFound(resource)
	}
	return nil
}
