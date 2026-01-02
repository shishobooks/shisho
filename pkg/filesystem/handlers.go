package filesystem

import (
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

type handler struct {
	filesystemService *Service
}

func (h *handler) browse(c echo.Context) error {
	// Bind query params.
	params := BrowseQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Set defaults.
	if params.Limit == 0 {
		params.Limit = 50
	}
	if params.Path == "" {
		params.Path = "/"
	}

	resp, err := h.filesystemService.Browse(BrowseOptions(params))
	if err != nil {
		if os.IsNotExist(err) {
			return errcodes.NotFound("Directory")
		}
		if os.IsPermission(err) {
			return errcodes.Forbidden("Access denied to this directory")
		}
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, resp))
}
