package sharelinks

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/appsettings"
)

// settingsHandler handles the sharing settings endpoints.
type settingsHandler struct {
	appSettingsService *appsettings.Service
}

func (h *settingsHandler) get(c echo.Context) error {
	s, err := LoadSettings(c.Request().Context(), h.appSettingsService)
	if err != nil {
		return errors.WithStack(err)
	}
	return c.JSON(http.StatusOK, SharingSettingsResponse(s))
}

// update merges the pointer fields into the saved settings so each switch
// saves on its own. The load and save are not one transaction, so two admins
// changing different switches at the same instant can lose one change, which
// is accepted for rarely edited settings.
func (h *settingsHandler) update(c echo.Context) error {
	var payload UpdateSharingSettingsPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	ctx := c.Request().Context()
	s, err := LoadSettings(ctx, h.appSettingsService)
	if err != nil {
		return errors.WithStack(err)
	}
	if payload.Enabled != nil {
		s.Enabled = *payload.Enabled
	}
	if payload.RequireExpiration != nil {
		s.RequireExpiration = *payload.RequireExpiration
	}
	if err := SaveSettings(ctx, h.appSettingsService, s); err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, SharingSettingsResponse(s))
}
