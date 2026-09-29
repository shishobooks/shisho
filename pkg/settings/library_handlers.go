package settings

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/sortspec"
)

// libraryHandler handles per-(user x library) settings endpoints.
type libraryHandler struct {
	settingsService *Service
}

func (h *libraryHandler) getLibrarySettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	libraryID, err := httputil.ParamID(c, "library_id", "Library")
	if err != nil {
		return err
	}

	if !user.HasLibraryAccess(libraryID) {
		return errcodes.Forbidden("You don't have access to this library")
	}

	row, err := h.settingsService.GetLibrarySettings(ctx, user.ID, libraryID)
	if err != nil {
		return errors.WithStack(err)
	}

	resp := LibrarySettingsResponse{}
	if row != nil {
		resp.SortSpec = row.SortSpec
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *libraryHandler) updateLibrarySettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	libraryID, err := httputil.ParamID(c, "library_id", "Library")
	if err != nil {
		return err
	}

	if !user.HasLibraryAccess(libraryID) {
		return errcodes.Forbidden("You don't have access to this library")
	}

	var payload UpdateLibrarySettingsPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	// Validate sort_spec if present (nil means "clear").
	if payload.SortSpec != nil && *payload.SortSpec != "" {
		if _, err := sortspec.Parse(*payload.SortSpec); err != nil {
			return errcodes.ValidationError(err.Error())
		}
	}
	// Treat empty string as equivalent to null (clear).
	var toStore *string
	if payload.SortSpec != nil && *payload.SortSpec != "" {
		toStore = payload.SortSpec
	}

	row, err := h.settingsService.UpsertLibrarySort(ctx, user.ID, libraryID, toStore)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, LibrarySettingsResponse{SortSpec: row.SortSpec})
}
