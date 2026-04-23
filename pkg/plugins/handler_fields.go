package plugins

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

// setFieldSettingsPayload is the payload for setting field settings.
type setFieldSettingsPayload struct {
	Fields map[string]bool `json:"fields" validate:"required"`
}

// fieldSettingsResponse is the response for field settings endpoints.
type fieldSettingsResponse struct {
	Fields map[string]bool `json:"fields"`
}

// libraryFieldSettingsResponse is the response for library field settings endpoints.
type libraryFieldSettingsResponse struct {
	Fields     map[string]bool `json:"fields"`
	Customized bool            `json:"customized"`
}

func (h *handler) getFieldSettings(c echo.Context) error {
	ctx := c.Request().Context()
	scope := c.Param("scope")
	id := c.Param("id")

	// Validate plugin exists and is a metadata enricher
	rt := h.manager.GetRuntime(scope, id)
	if rt == nil {
		return errcodes.NotFound("Plugin")
	}
	if rt.Manifest().Capabilities.MetadataEnricher == nil {
		// Not an enricher - return empty response
		return c.JSON(http.StatusOK, fieldSettingsResponse{Fields: nil})
	}

	settings, err := h.service.GetFieldSettings(ctx, scope, id)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, fieldSettingsResponse{
		Fields: settings,
	})
}

func (h *handler) setFieldSettings(c echo.Context) error {
	ctx := c.Request().Context()
	scope := c.Param("scope")
	id := c.Param("id")

	// Validate plugin exists
	rt := h.manager.GetRuntime(scope, id)
	if rt == nil {
		return errcodes.NotFound("Plugin")
	}

	// Validate plugin is a metadata enricher
	enricherCap := rt.Manifest().Capabilities.MetadataEnricher
	if enricherCap == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "plugin is not a metadata enricher")
	}

	var payload setFieldSettingsPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	if payload.Fields == nil {
		return errcodes.ValidationError("Fields are required.")
	}

	// Validate field names are declared by the plugin
	declared := make(map[string]bool, len(enricherCap.Fields))
	for _, f := range enricherCap.Fields {
		declared[f] = true
	}
	for field := range payload.Fields {
		if !declared[field] {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown field: "+field)
		}
	}

	for field, enabled := range payload.Fields {
		if err := h.service.SetFieldSetting(ctx, scope, id, field, enabled); err != nil {
			return errors.WithStack(err)
		}
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) getLibraryFieldSettings(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	scope := c.Param("scope")
	pluginID := c.Param("pluginId")

	// Validate plugin exists and is a metadata enricher
	rt := h.manager.GetRuntime(scope, pluginID)
	if rt == nil {
		return errcodes.NotFound("Plugin")
	}
	if rt.Manifest().Capabilities.MetadataEnricher == nil {
		// Not an enricher - return empty response
		return c.JSON(http.StatusOK, libraryFieldSettingsResponse{Fields: nil, Customized: false})
	}

	settings, err := h.service.GetLibraryFieldSettings(ctx, libraryID, scope, pluginID)
	if err != nil {
		return errors.WithStack(err)
	}

	customized := len(settings) > 0

	return c.JSON(http.StatusOK, libraryFieldSettingsResponse{
		Fields:     settings,
		Customized: customized,
	})
}

func (h *handler) setLibraryFieldSettings(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	scope := c.Param("scope")
	pluginID := c.Param("pluginId")

	// Validate plugin exists
	rt := h.manager.GetRuntime(scope, pluginID)
	if rt == nil {
		return errcodes.NotFound("Plugin")
	}

	// Validate plugin is a metadata enricher
	enricherCap := rt.Manifest().Capabilities.MetadataEnricher
	if enricherCap == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "plugin is not a metadata enricher")
	}

	var payload setFieldSettingsPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	if payload.Fields == nil {
		return errcodes.ValidationError("Fields are required.")
	}

	// Validate field names are declared by the plugin
	declared := make(map[string]bool, len(enricherCap.Fields))
	for _, f := range enricherCap.Fields {
		declared[f] = true
	}
	for field := range payload.Fields {
		if !declared[field] {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown field: "+field)
		}
	}

	for field, enabled := range payload.Fields {
		if err := h.service.SetLibraryFieldSetting(ctx, libraryID, scope, pluginID, field, enabled); err != nil {
			return errors.WithStack(err)
		}
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) resetLibraryFieldSettings(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	scope := c.Param("scope")
	pluginID := c.Param("pluginId")

	if err := h.service.ResetLibraryFieldSettings(ctx, libraryID, scope, pluginID); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}
