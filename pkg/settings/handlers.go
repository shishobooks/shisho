package settings

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	settingsService *Service
}

// newUserSettingsResponse maps the settings model onto the wire shape.
// Shared by get and update so the two endpoints can't drift as fields are
// added.
func newUserSettingsResponse(settings *models.UserSettings) UserSettingsResponse {
	return UserSettingsResponse{
		PreloadCount:       settings.ViewerPreloadCount,
		FitMode:            settings.ViewerFitMode,
		ReflowableFontSize: settings.ReflowableFontSize,
		ReflowableTheme:    settings.ReflowableTheme,
		ReflowableFlow:     settings.ReflowableFlow,
		GallerySize:        settings.GallerySize,
		HideChrome:         settings.ViewerHideChrome,
		PlaybackSpeed:      settings.PlaybackSpeed,
	}
}

func (h *handler) getUserSettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	settings, err := h.settingsService.GetUserSettings(ctx, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, newUserSettingsResponse(settings))
}

func (h *handler) updateUserSettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
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
		return errcodes.ValidationError("fit_mode must be 'fit-height' or 'fit-width'")
	}
	if payload.ReflowableFontSize != nil && (*payload.ReflowableFontSize < 50 || *payload.ReflowableFontSize > 200) {
		return errcodes.ValidationError("viewer_reflowable_font_size must be between 50 and 200")
	}
	if payload.ReflowableTheme != nil && !IsValidReflowableTheme(*payload.ReflowableTheme) {
		return errcodes.ValidationError("viewer_reflowable_theme must be 'light', 'dark', or 'sepia'")
	}
	if payload.ReflowableFlow != nil && !IsValidReflowableFlow(*payload.ReflowableFlow) {
		return errcodes.ValidationError("viewer_reflowable_flow must be 'paginated' or 'scrolled'")
	}
	if payload.GallerySize != nil && !IsValidGallerySize(*payload.GallerySize) {
		return errcodes.ValidationError("gallery_size must be 's', 'm', 'l', or 'xl'")
	}
	if payload.PlaybackSpeed != nil && !IsValidPlaybackSpeed(*payload.PlaybackSpeed) {
		return errcodes.ValidationError("viewer_playback_speed must be one of 0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, or 3")
	}

	settings, err := h.settingsService.UpdateUserSettings(
		ctx, user.ID, UserSettingsUpdate(payload))
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, newUserSettingsResponse(settings))
}
