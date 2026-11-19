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
	detectBudget      detectLimits
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

// detect counts the recognized files in the folders a new library will scan,
// for Setup's plugin recommendations. It stops early on a large or slow
// library and says so.
func (h *handler) detect(c echo.Context) error {
	payload := DetectFileTypesPayload{}
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	limits := h.detectBudget
	limits.foldersOnly = payload.FoldersOnly
	resp, err := h.filesystemService.DetectFileTypes(c.Request().Context(), payload.Paths, limits)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, resp))
}
