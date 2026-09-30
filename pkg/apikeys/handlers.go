package apikeys

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	pkgerrors "github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

type handler struct {
	service *Service
}

func newHandler(service *Service) *handler {
	return &handler{service: service}
}

// List returns all API keys for the current user.
func (h *handler) List(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	keys, err := h.service.List(c.Request().Context(), user.ID)
	if err != nil {
		return pkgerrors.WithStack(err)
	}

	return c.JSON(http.StatusOK, keys)
}

// Create creates a new API key for the current user.
func (h *handler) Create(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	var req CreateAPIKeyPayload
	if err := c.Bind(&req); err != nil {
		return pkgerrors.WithStack(err)
	}

	if req.Name == "" {
		return errcodes.ValidationError("Name is required")
	}
	if len(req.Name) > 100 {
		return errcodes.ValidationError("Name must be 100 characters or less")
	}

	apiKey, err := h.service.Create(c.Request().Context(), user.ID, req.Name)
	if err != nil {
		return pkgerrors.WithStack(err)
	}

	return c.JSON(http.StatusCreated, apiKey)
}

// UpdateName updates an API key's name.
func (h *handler) UpdateName(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	keyID := c.Param("id")

	var req UpdateAPIKeyNamePayload
	if err := c.Bind(&req); err != nil {
		return pkgerrors.WithStack(err)
	}

	if req.Name == "" {
		return errcodes.ValidationError("Name is required")
	}
	if len(req.Name) > 100 {
		return errcodes.ValidationError("Name must be 100 characters or less")
	}

	apiKey, err := h.service.UpdateName(c.Request().Context(), user.ID, keyID, req.Name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return errcodes.NotFound("API key")
		}
		return pkgerrors.WithStack(err)
	}

	return c.JSON(http.StatusOK, apiKey)
}

// Delete deletes an API key.
func (h *handler) Delete(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	keyID := c.Param("id")

	err = h.service.Delete(c.Request().Context(), user.ID, keyID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return errcodes.NotFound("API key")
		}
		return pkgerrors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

// AddPermission adds a permission to an API key.
func (h *handler) AddPermission(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	keyID := c.Param("id")
	permission := c.Param("permission")

	apiKey, err := h.service.AddPermission(c.Request().Context(), user.ID, keyID, permission)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return errcodes.NotFound("API key")
		}
		return pkgerrors.WithStack(err)
	}

	return c.JSON(http.StatusOK, apiKey)
}

// RemovePermission removes a permission from an API key.
func (h *handler) RemovePermission(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	keyID := c.Param("id")
	permission := c.Param("permission")

	apiKey, err := h.service.RemovePermission(c.Request().Context(), user.ID, keyID, permission)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return errcodes.NotFound("API key")
		}
		return pkgerrors.WithStack(err)
	}

	return c.JSON(http.StatusOK, apiKey)
}

// GenerateShortURL creates a temporary short URL for an API key.
func (h *handler) GenerateShortURL(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	keyID := c.Param("id")

	shortURL, err := h.service.GenerateShortURL(c.Request().Context(), user.ID, keyID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return errcodes.NotFound("API key")
		}
		return pkgerrors.WithStack(err)
	}

	return c.JSON(http.StatusCreated, shortURL)
}

// ClearKoboSync clears Kobo sync history for an API key, forcing a fresh sync.
func (h *handler) ClearKoboSync(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	keyID := c.Param("id")

	err = h.service.ClearKoboSyncHistory(c.Request().Context(), user.ID, keyID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return errcodes.NotFound("API key")
		}
		return pkgerrors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}
