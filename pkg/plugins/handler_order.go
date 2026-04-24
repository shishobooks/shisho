package plugins

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

var validPluginModes = map[string]bool{
	models.PluginModeEnabled:    true,
	models.PluginModeManualOnly: true,
	models.PluginModeDisabled:   true,
}

type orderEntry struct {
	Scope string `json:"scope" validate:"required"`
	ID    string `json:"id" validate:"required"`
	Mode  string `json:"mode"`
}

type setOrderPayload struct {
	Order []orderEntry `json:"order" validate:"required"`
}

type libraryOrderEntry struct {
	Scope string `json:"scope" validate:"required"`
	ID    string `json:"id" validate:"required"`
	Mode  string `json:"mode"`
}

type setLibraryOrderPayload struct {
	Plugins []libraryOrderEntry `json:"plugins" validate:"required"`
}

type libraryOrderResponse struct {
	Customized bool                 `json:"customized"`
	Plugins    []libraryOrderPlugin `json:"plugins"`
}

type libraryOrderPlugin struct {
	Scope string `json:"scope"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Mode  string `json:"mode"`
}

func (h *handler) getOrder(c echo.Context) error {
	ctx := c.Request().Context()

	hookType := c.Param("hookType")

	orders, err := h.service.GetOrder(ctx, hookType)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, orders))
}

func (h *handler) setOrder(c echo.Context) error {
	ctx := c.Request().Context()

	hookType := c.Param("hookType")

	var payload setOrderPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	orderEntries := make([]models.PluginHookConfig, len(payload.Order))
	for i, entry := range payload.Order {
		mode := entry.Mode
		if mode == "" {
			mode = models.PluginModeEnabled
		}
		if !validPluginModes[mode] {
			return errcodes.ValidationError(fmt.Sprintf("invalid mode %q for plugin %s/%s", mode, entry.Scope, entry.ID))
		}
		orderEntries[i] = models.PluginHookConfig{
			Scope:    entry.Scope,
			PluginID: entry.ID,
			Mode:     mode,
		}
	}

	if err := h.service.SetOrder(ctx, hookType, orderEntries); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) getLibraryOrder(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	hookType := c.Param("hookType")

	customized, err := h.service.IsLibraryCustomized(ctx, libraryID, hookType)
	if err != nil {
		return errors.WithStack(err)
	}

	var plugins []libraryOrderPlugin

	if customized {
		entries, err := h.service.GetLibraryOrder(ctx, libraryID, hookType)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, entry := range entries {
			name := entry.Scope + "/" + entry.PluginID
			if p, _ := h.service.GetPlugin(ctx, entry.Scope, entry.PluginID); p != nil {
				name = p.Name
			}
			plugins = append(plugins, libraryOrderPlugin{
				Scope: entry.Scope,
				ID:    entry.PluginID,
				Name:  name,
				Mode:  entry.Mode,
			})
		}
	} else {
		orders, err := h.service.GetOrder(ctx, hookType)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, order := range orders {
			name := order.Scope + "/" + order.PluginID
			if p, _ := h.service.GetPlugin(ctx, order.Scope, order.PluginID); p != nil {
				name = p.Name
			}
			plugins = append(plugins, libraryOrderPlugin{
				Scope: order.Scope,
				ID:    order.PluginID,
				Name:  name,
				Mode:  order.Mode,
			})
		}
	}

	return c.JSON(http.StatusOK, libraryOrderResponse{
		Customized: customized,
		Plugins:    plugins,
	})
}

func (h *handler) setLibraryOrder(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	hookType := c.Param("hookType")

	var payload setLibraryOrderPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	entries := make([]models.LibraryPluginHookConfig, len(payload.Plugins))
	for i, p := range payload.Plugins {
		mode := p.Mode
		if mode == "" {
			mode = models.PluginModeEnabled
		}
		if !validPluginModes[mode] {
			return errcodes.ValidationError(fmt.Sprintf("invalid mode %q for plugin %s/%s", mode, p.Scope, p.ID))
		}
		entries[i] = models.LibraryPluginHookConfig{
			Scope:    p.Scope,
			PluginID: p.ID,
			Mode:     mode,
		}
	}

	if err := h.service.SetLibraryOrder(ctx, libraryID, hookType, entries); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) resetLibraryOrder(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	hookType := c.Param("hookType")

	if err := h.service.ResetLibraryOrder(ctx, libraryID, hookType); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) resetAllLibraryOrders(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}

	if err := h.service.ResetAllLibraryOrders(ctx, libraryID); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}
