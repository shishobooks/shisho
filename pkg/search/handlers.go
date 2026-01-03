package search

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
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

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(params.LibraryID) {
			return c.JSON(http.StatusOK, &GlobalSearchResponse{
				Books:  []BookSearchResult{},
				Series: []SeriesSearchResult{},
				People: []PersonSearchResult{},
			})
		}
	}

	result, err := h.searchService.GlobalSearch(ctx, params.LibraryID, params.Query)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, result))
}
