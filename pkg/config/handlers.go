package config

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

type handler struct {
	configService *Service
}

func (h *handler) retrieve(c echo.Context) error {
	userConfig, err := h.configService.RetrieveUserConfig()
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, userConfig))
}

func (h *handler) update(c echo.Context) error {
	params := UpdateConfigPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	userConfig, err := h.configService.RetrieveUserConfig()
	if err != nil {
		return errors.WithStack(err)
	}

	opts := UpdateUserConfigOptions{}

	if params.SyncIntervalMinutes != nil && userConfig.SyncIntervalMinutes != *params.SyncIntervalMinutes {
		userConfig.SyncIntervalMinutes = *params.SyncIntervalMinutes
		opts.UpdateFile = true
	}

	if err := h.configService.UpdateUserConfig(userConfig, opts); err != nil {
		return errors.WithStack(err)
	}

	userConfig, err = h.configService.RetrieveUserConfig()
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, userConfig))
}
