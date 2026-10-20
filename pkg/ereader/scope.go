package ereader

import (
	"context"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

// deviceScope is the part of the owner's libraries a Device's scope lets
// the eReader browser show: one library, the books on one list, or
// everything. A scope whose library or list is gone, or a list the owner can
// no longer see, shows nothing, as Kobo sync does.
type deviceScope struct {
	libraryID *int
	listID    *int
	closed    bool
}

// scope returns the scope of the Device DeviceAuth stored.
func (h *handler) scope(c echo.Context) (deviceScope, error) {
	device, err := devices.RequireDevice(c)
	if err != nil {
		return deviceScope{}, err
	}
	scope := device.Scope()
	switch scope.Type {
	case devices.ScopeLibrary:
		if scope.LibraryID == nil {
			return deviceScope{closed: true}, nil
		}
		return deviceScope{libraryID: scope.LibraryID}, nil
	case devices.ScopeList:
		if scope.ListID == nil {
			return deviceScope{closed: true}, nil
		}
		user, err := auth.RequireUser(c)
		if err != nil {
			return deviceScope{}, err
		}
		canView, err := h.listService.CanView(c.Request().Context(), *scope.ListID, user.ID)
		if err != nil {
			return deviceScope{}, errors.WithStack(err)
		}
		if !canView {
			return deviceScope{closed: true}, nil
		}
		return deviceScope{listID: scope.ListID}, nil
	}
	return deviceScope{}, nil
}

// allowsLibrary reports whether the scope can include books from libraryID.
func (s deviceScope) allowsLibrary(libraryID int) bool {
	return !s.closed && (s.libraryID == nil || *s.libraryID == libraryID)
}

// requireLibrary returns the scope for a library path, or a 404 for a
// library outside it. The owner's access is checked first, so a library
// they cannot access gets the usual library access error.
func (h *handler) requireLibrary(c echo.Context, libraryID int) (deviceScope, error) {
	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return deviceScope{}, err
	}
	scope, err := h.scope(c)
	if err != nil {
		return deviceScope{}, err
	}
	if !scope.allowsLibrary(libraryID) {
		return deviceScope{}, errcodes.NotFound("Library")
	}
	return scope, nil
}

// requireBookInScope returns resource's 404 for a book the owner cannot
// access or that is outside the Device's scope, the same response as an id
// with no row, so a Device cannot probe for books it does not show.
func (h *handler) requireBookInScope(c echo.Context, libraryID, bookID int, resource string) error {
	if err := requireEntityAccess(c, libraryID, resource); err != nil {
		return err
	}
	scope, err := h.scope(c)
	if err != nil {
		return err
	}
	if !scope.allowsLibrary(libraryID) {
		return errcodes.NotFound(resource)
	}
	if scope.listID != nil {
		onList, err := h.listService.ContainsBook(c.Request().Context(), *scope.listID, bookID)
		if err != nil {
			return errors.WithStack(err)
		}
		if !onList {
			return errcodes.NotFound(resource)
		}
	}
	return nil
}

// libraryHasBooks reports whether libraryID has a book in the scope, so the
// libraries page of a list-scoped Device only shows libraries with books on
// the list.
func (h *handler) libraryHasBooks(ctx context.Context, scope deviceScope, libraryID int) (bool, error) {
	if scope.listID == nil {
		return scope.allowsLibrary(libraryID), nil
	}
	_, total, err := h.listBooksPaginated(ctx, scope, books.ListBooksOptions{LibraryID: &libraryID}, 1, "")
	if err != nil {
		return false, err
	}
	return total > 0, nil
}

// requireListedBooks returns resource's 404 when a list-scoped Device has no
// book matching opts on its list, so a series or author page cannot reveal
// the name of one the list leaves out.
func (h *handler) requireListedBooks(ctx context.Context, scope deviceScope, opts books.ListBooksOptions, resource string) error {
	if scope.listID == nil {
		return nil
	}
	_, total, err := h.listBooksPaginated(ctx, scope, opts, 1, "")
	if err != nil {
		return err
	}
	if total == 0 {
		return errcodes.NotFound(resource)
	}
	return nil
}
