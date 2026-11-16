package updatecheck

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

type handler struct {
	service *Service
}

func (h *handler) retrieve(c echo.Context) error {
	return errors.WithStack(c.JSON(http.StatusOK, h.service.Status(c.Request().Context())))
}
