package settings

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/sharelinks"
)

// sharingHandler handles the sharing settings endpoints.
type sharingHandler struct {
	appSettingsService *appsettings.Service
}

func (h *sharingHandler) getSharingSettings(c echo.Context) error {
	s, err := sharelinks.LoadSettings(c.Request().Context(), h.appSettingsService)
	if err != nil {
		return errors.WithStack(err)
	}
	return c.JSON(http.StatusOK, SharingSettingsResponse(s))
}

func (h *sharingHandler) updateSharingSettings(c echo.Context) error {
	var payload UpdateSharingSettingsPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	ctx := c.Request().Context()
	s, err := sharelinks.LoadSettings(ctx, h.appSettingsService)
	if err != nil {
		return errors.WithStack(err)
	}
	if payload.Enabled != nil {
		s.Enabled = *payload.Enabled
	}
	if payload.RequireExpiration != nil {
		s.RequireExpiration = *payload.RequireExpiration
	}
	if err := sharelinks.SaveSettings(ctx, h.appSettingsService, s); err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, SharingSettingsResponse(s))
}
