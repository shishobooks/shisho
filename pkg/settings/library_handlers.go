package settings

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortspec"
)

// libraryHandler handles per-(user x library) settings endpoints.
type libraryHandler struct {
	settingsService *Service
}

func (h *libraryHandler) getLibrarySettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("Authentication required")
	}

	libraryID, err := strconv.Atoi(c.Param("library_id"))
	if err != nil || libraryID < 1 {
		return errcodes.ValidationError("invalid library_id")
	}

	if !user.HasLibraryAccess(libraryID) {
		return errcodes.Forbidden("Access to this library")
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

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("Authentication required")
	}

	libraryID, err := strconv.Atoi(c.Param("library_id"))
	if err != nil || libraryID < 1 {
		return errcodes.ValidationError("invalid library_id")
	}

	if !user.HasLibraryAccess(libraryID) {
		return errcodes.Forbidden("Access to this library")
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
