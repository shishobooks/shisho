package plugins

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/models"
)

var validPluginModes = map[string]bool{
	models.PluginModeEnabled:    true,
	models.PluginModeManualOnly: true,
	models.PluginModeDisabled:   true,
}

var validHookTypes = map[string]bool{
	models.PluginHookInputConverter:   true,
	models.PluginHookFileParser:       true,
	models.PluginHookMetadataEnricher: true,
}

// hookTypeParam returns the :hookType path parameter, rejecting a hook type
// that plugins cannot provide so no order row is read or stored for it.
func hookTypeParam(c echo.Context) (string, error) {
	hookType := c.Param("hookType")
	if !validHookTypes[hookType] {
		return "", errcodes.ValidationError(fmt.Sprintf("unknown hook type %q", hookType))
	}
	return hookType, nil
}

func (h *handler) getOrder(c echo.Context) error {
	ctx := c.Request().Context()

	hookType, err := hookTypeParam(c)
	if err != nil {
		return err
	}

	orders, err := h.service.GetOrder(ctx, hookType)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, orders))
}

func (h *handler) setOrder(c echo.Context) error {
	ctx := c.Request().Context()

	hookType, err := hookTypeParam(c)
	if err != nil {
		return err
	}

	var payload SetOrderPayload
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

	libraryID, err := httputil.ParamID(c, "id", "Library")
	if err != nil {
		return err
	}
	hookType, err := hookTypeParam(c)
	if err != nil {
		return err
	}

	customized, err := h.service.IsLibraryCustomized(ctx, libraryID, hookType)
	if err != nil {
		return errors.WithStack(err)
	}

	var plugins []LibraryPluginOrderPlugin

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
			plugins = append(plugins, LibraryPluginOrderPlugin{
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
			plugins = append(plugins, LibraryPluginOrderPlugin{
				Scope: order.Scope,
				ID:    order.PluginID,
				Name:  name,
				Mode:  order.Mode,
			})
		}
	}

	return c.JSON(http.StatusOK, LibraryPluginOrderResponse{
		Customized: customized,
		Plugins:    plugins,
	})
}

func (h *handler) setLibraryOrder(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := httputil.ParamID(c, "id", "Library")
	if err != nil {
		return err
	}
	hookType, err := hookTypeParam(c)
	if err != nil {
		return err
	}

	var payload SetLibraryOrderPayload
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

	libraryID, err := httputil.ParamID(c, "id", "Library")
	if err != nil {
		return err
	}
	hookType, err := hookTypeParam(c)
	if err != nil {
		return err
	}

	if err := h.service.ResetLibraryOrder(ctx, libraryID, hookType); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) resetAllLibraryOrders(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := httputil.ParamID(c, "id", "Library")
	if err != nil {
		return err
	}

	if err := h.service.ResetAllLibraryOrders(ctx, libraryID); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}
