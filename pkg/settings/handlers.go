package settings

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	settingsService *Service
}

func (h *handler) getViewerSettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("Authentication required")
	}

	settings, err := h.settingsService.GetViewerSettings(ctx, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, ViewerSettingsResponse{
		PreloadCount: settings.ViewerPreloadCount,
		FitMode:      settings.ViewerFitMode,
	})
}

func (h *handler) updateViewerSettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("Authentication required")
	}

	var payload ViewerSettingsPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	// Validate preload count (1-10)
	if payload.PreloadCount < 1 || payload.PreloadCount > 10 {
		return errcodes.ValidationError("preload_count must be between 1 and 10")
	}

	// Validate fit mode
	if !IsValidFitMode(payload.FitMode) {
		return errcodes.ValidationError("fit_mode must be 'fit-height' or 'original'")
	}

	settings, err := h.settingsService.UpdateViewerSettings(ctx, user.ID, payload.PreloadCount, payload.FitMode)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, ViewerSettingsResponse{
		PreloadCount: settings.ViewerPreloadCount,
		FitMode:      settings.ViewerFitMode,
	})
}
