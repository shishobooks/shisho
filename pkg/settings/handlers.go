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

func (h *handler) getUserSettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("Authentication required")
	}

	settings, err := h.settingsService.GetUserSettings(ctx, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, UserSettingsResponse{
		PreloadCount: settings.ViewerPreloadCount,
		FitMode:      settings.ViewerFitMode,
		EpubFontSize: settings.EpubFontSize,
		EpubTheme:    settings.EpubTheme,
		EpubFlow:     settings.EpubFlow,
	})
}

func (h *handler) updateUserSettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("Authentication required")
	}

	var payload UserSettingsPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	// Validate only the fields the client sent. Omitted fields are left
	// untouched by the service layer.
	if payload.PreloadCount != nil && (*payload.PreloadCount < 1 || *payload.PreloadCount > 10) {
		return errcodes.ValidationError("preload_count must be between 1 and 10")
	}
	if payload.FitMode != nil && !IsValidFitMode(*payload.FitMode) {
		return errcodes.ValidationError("fit_mode must be 'fit-height' or 'original'")
	}
	if payload.EpubFontSize != nil && (*payload.EpubFontSize < 50 || *payload.EpubFontSize > 200) {
		return errcodes.ValidationError("viewer_epub_font_size must be between 50 and 200")
	}
	if payload.EpubTheme != nil && !IsValidEpubTheme(*payload.EpubTheme) {
		return errcodes.ValidationError("viewer_epub_theme must be 'light', 'dark', or 'sepia'")
	}
	if payload.EpubFlow != nil && !IsValidEpubFlow(*payload.EpubFlow) {
		return errcodes.ValidationError("viewer_epub_flow must be 'paginated' or 'scrolled'")
	}

	settings, err := h.settingsService.UpdateUserSettings(
		ctx, user.ID, UserSettingsUpdate(payload))
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, UserSettingsResponse{
		PreloadCount: settings.ViewerPreloadCount,
		FitMode:      settings.ViewerFitMode,
		EpubFontSize: settings.EpubFontSize,
		EpubTheme:    settings.EpubTheme,
		EpubFlow:     settings.EpubFlow,
	})
}
