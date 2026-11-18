package books

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

// sidecarImpact answers GET /books/sidecar-impact: how many distinct Books'
// sidecars renaming the resource, or merging it into another, rewrites in
// the background. The rename and merge dialogs show it when it's large. It needs
// the write permission the rename and merge routes need, and access to each
// resource's library.
func (h *handler) sidecarImpact(c echo.Context) error {
	ctx := c.Request().Context()
	params := SidecarImpactQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	kind := sidecarKinds[params.Kind]

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	if !user.HasPermission(kind.resource, models.OperationWrite) {
		return errcodes.PermissionDenied(kind.resource, models.OperationWrite)
	}

	libraryID, found, err := h.bookService.sidecarResourceLibrary(ctx, params.Kind, params.ID)
	if err != nil {
		return err
	}
	if !found {
		return errcodes.NotFound(kind.name)
	}
	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	count, err := h.bookService.CountSidecarBooks(ctx, params.Kind, params.ID)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, SidecarImpactResponse{BookCount: count}))
}
