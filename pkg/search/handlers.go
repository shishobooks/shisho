package search

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	searchService *Service
}

func (h *handler) globalSearch(c echo.Context) error {
	ctx := c.Request().Context()

	// Bind params
	params := GlobalSearchQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Check library access
	if !user.HasLibraryAccess(params.LibraryID) {
		return c.JSON(http.StatusOK, &GlobalSearchResponse{
			Books:  []BookSearchResult{},
			Series: []SeriesSearchResult{},
			People: []PersonSearchResult{},
		})
	}

	// The route requires books:read. Series and people come back only to
	// roles that can read them.
	sections := GlobalSearchSections{
		Series: user.HasPermission(models.ResourceSeries, models.OperationRead),
		People: user.HasPermission(models.ResourcePeople, models.OperationRead),
	}
	result, err := h.searchService.GlobalSearch(ctx, params.LibraryID, params.Query, sections)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, result))
}
