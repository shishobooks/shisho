package ereader

import (
	"context"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reach"
)

// The eReader browser shows what the Device DeviceAuth stored reaches
// (pkg/reach). Unlike OPDS, a list-scoped Device browses the libraries
// holding books on its list, showing only those books.

// reach returns what the Device reaches, with its Delivery Rules.
func (h *handler) reach(c echo.Context) (*reach.Reach, error) {
	user, device, err := deviceAndOwner(c)
	if err != nil {
		return nil, err
	}
	return h.reachService.Resolve(c.Request().Context(), user, device)
}

// scopeOnly returns what the Device reaches without loading its Delivery
// Rules, for the routes that serve one book's cover or file.
func (h *handler) scopeOnly(c echo.Context) (*reach.Reach, error) {
	user, device, err := deviceAndOwner(c)
	if err != nil {
		return nil, err
	}
	return h.reachService.Scope(c.Request().Context(), user, device)
}

func deviceAndOwner(c echo.Context) (*models.User, *devices.Device, error) {
	device, err := devices.RequireDevice(c)
	if err != nil {
		return nil, nil, err
	}
	user, err := auth.RequireUser(c)
	if err != nil {
		return nil, nil, err
	}
	return user, device, nil
}

// requireLibrary returns what the Device reaches for a library path, or a
// 404 for a library outside it. The owner's access is checked first, so a
// library they cannot access gets the usual library access error.
func (h *handler) requireLibrary(c echo.Context, libraryID int) (*reach.Reach, error) {
	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return nil, err
	}
	r, err := h.reach(c)
	if err != nil {
		return nil, err
	}
	if !r.HoldsLibrary(libraryID) {
		return nil, errcodes.NotFound("Library")
	}
	return r, nil
}

// requireBookInScope returns resource's 404 for a book the owner cannot
// access or that is outside r, the same response as an id with no row, so a
// Device cannot probe for books it does not show.
func (h *handler) requireBookInScope(c echo.Context, r *reach.Reach, libraryID, bookID int, resource string) error {
	if err := requireEntityAccess(c, libraryID, resource); err != nil {
		return err
	}
	holds, err := h.reachService.HoldsBook(c.Request().Context(), r, libraryID, bookID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !holds {
		return errcodes.NotFound(resource)
	}
	return nil
}

// requireListedBooks returns resource's 404 when a list-scoped Device has no
// book matching opts on its list, so a series or author page cannot reveal
// the name of one the list leaves out.
func (h *handler) requireListedBooks(ctx context.Context, r *reach.Reach, opts books.ListBooksOptions, resource string) error {
	if r.ListID() == nil {
		return nil
	}
	has, err := h.reachService.HasBooks(ctx, r, opts)
	if err != nil {
		return err
	}
	if !has {
		return errcodes.NotFound(resource)
	}
	return nil
}
