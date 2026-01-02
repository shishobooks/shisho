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
	cfg := h.configService.RetrieveConfig()
	return errors.WithStack(c.JSON(http.StatusOK, cfg))
}
