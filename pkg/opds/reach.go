package opds

import (
	"context"
	"slices"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

// reach is what one OPDS request can reach. Signed in with the account
// password, that is every library the user can access, delivered by the
// user's default Delivery Rules. Signed in as a Device, it is the Device's
// rules and scope: one library, or one list and nothing else, as on the
// Device's other connections.
type reach struct {
	user  *models.User
	rules []delivery.Rule
	// libraryIDs holds the libraries books can come from (nil means every
	// library).
	libraryIDs []int
	// listID is the one list a list-scoped Device reaches. Its libraries
	// aren't browsable, only the list.
	listID *int
	// libraryScoped is set for a Device scoped to one library, which browses
	// that library and no lists.
	libraryScoped bool
	// closed is set for a scope whose library or list is gone or no longer
	// viewable, which reaches nothing, as on Kobo sync.
	closed bool
}

// reachFor returns what the request's sign-in reaches, with the Delivery
// Rules it follows.
func (h *handler) reachFor(c echo.Context) (*reach, error) {
	r, err := h.scopeFor(c)
	if err != nil {
		return nil, err
	}
	ctx := c.Request().Context()
	if device := devices.SignedInDevice(c); device != nil {
		r.rules, err = h.deviceService.Rules(ctx, device)
	} else {
		var userDefault *delivery.UserRules
		userDefault, err = h.deliveryService.UserDefault(ctx, r.user.ID)
		if userDefault != nil {
			r.rules = userDefault.Rules
		}
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return r, nil
}

// scopeFor returns what the request's sign-in reaches, without loading the
// Delivery Rules, for routes that serve one book.
func (h *handler) scopeFor(c echo.Context) (*reach, error) {
	user, err := auth.RequireUser(c)
	if err != nil {
		return nil, err
	}
	r := &reach{user: user, libraryIDs: user.GetAccessibleLibraryIDs()}
	device := devices.SignedInDevice(c)
	if device == nil {
		return r, nil
	}

	scope := device.Scope()
	switch scope.Type {
	case devices.ScopeLibrary:
		r.libraryScoped = true
		if scope.LibraryID == nil || !user.HasLibraryAccess(*scope.LibraryID) {
			r.closed = true
		} else {
			r.libraryIDs = []int{*scope.LibraryID}
		}
	case devices.ScopeList:
		if scope.ListID == nil {
			r.closed = true
			break
		}
		canView, err := h.listsService.CanView(c.Request().Context(), *scope.ListID, user.ID)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if canView {
			r.listID = scope.ListID
		} else {
			r.closed = true
		}
	}
	if r.closed {
		r.libraryIDs = []int{}
	}
	return r, nil
}

// browsableLibraryIDs returns the libraries the catalog lists (nil means
// every library). A list-scoped Device browses only its list.
func (r *reach) browsableLibraryIDs() []int {
	if r.listID != nil {
		return []int{}
	}
	return r.libraryIDs
}

func (r *reach) holdsLibrary(libraryID int) bool {
	return !r.closed && (r.libraryIDs == nil || slices.Contains(r.libraryIDs, libraryID))
}

// requireLibrary returns a 404 for a library path the request can't browse.
// The user's own access is checked first, so a library they can't access
// gets the usual library access error.
func (r *reach) requireLibrary(c echo.Context, libraryID int) error {
	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}
	if r.listID != nil || !r.holdsLibrary(libraryID) {
		return errcodes.NotFound("Library")
	}
	return nil
}

// browsesLists reports whether the request can open lists at all. A Device
// scoped to one library browses only that library.
func (r *reach) browsesLists() bool {
	return !r.closed && !r.libraryScoped
}

// allowsList reports whether listID is a list the request can open. The
// caller still checks that the user can view it.
func (r *reach) allowsList(listID int) bool {
	return r.browsesLists() && (r.listID == nil || *r.listID == listID)
}

// requireBook returns resource's 404 for a book outside what the request
// reaches, the same response as an id with no row, so a Device can't probe
// for books its scope leaves out. The user's own library access is checked
// first.
func (h *handler) requireBook(ctx context.Context, c echo.Context, r *reach, libraryID, bookID int, resource string) error {
	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}
	if !r.holdsLibrary(libraryID) {
		return errcodes.NotFound(resource)
	}
	if r.listID != nil {
		onList, err := h.listsService.ContainsBook(ctx, *r.listID, bookID)
		if err != nil {
			return errors.WithStack(err)
		}
		if !onList {
			return errcodes.NotFound(resource)
		}
	}
	return nil
}
