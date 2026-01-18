package apikeys

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	pkgerrors "github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	service *Service
}

func newHandler(service *Service) *handler {
	return &handler{service: service}
}

// getUserFromContext retrieves the authenticated user from Echo context.
func getUserFromContext(c echo.Context) (*models.User, error) {
	user, ok := c.Get("user").(*models.User)
	if !ok || user == nil {
		return nil, errcodes.Unauthorized("Authentication required")
	}
	return user, nil
}

// List returns all API keys for the current user.
func (h *handler) List(c echo.Context) error {
	user, err := getUserFromContext(c)
	if err != nil {
		return err
	}

	keys, err := h.service.List(c.Request().Context(), user.ID)
	if err != nil {
		return pkgerrors.WithStack(err)
	}

	return c.JSON(http.StatusOK, keys)
}

// CreateRequest is the payload for creating an API key.
type CreateRequest struct {
	Name string `json:"name"`
}

// Create creates a new API key for the current user.
func (h *handler) Create(c echo.Context) error {
	user, err := getUserFromContext(c)
	if err != nil {
		return err
	}

	var req CreateRequest
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

// UpdateNameRequest is the payload for updating an API key's name.
type UpdateNameRequest struct {
	Name string `json:"name"`
}

// UpdateName updates an API key's name.
func (h *handler) UpdateName(c echo.Context) error {
	user, err := getUserFromContext(c)
	if err != nil {
		return err
	}

	keyID := c.Param("id")
	if keyID == "" {
		return errcodes.ValidationError("Key ID required")
	}

	var req UpdateNameRequest
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
	user, err := getUserFromContext(c)
	if err != nil {
		return err
	}

	keyID := c.Param("id")
	if keyID == "" {
		return errcodes.ValidationError("Key ID required")
	}

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
	user, err := getUserFromContext(c)
	if err != nil {
		return err
	}

	keyID := c.Param("id")
	permission := c.Param("permission")
	if keyID == "" || permission == "" {
		return errcodes.ValidationError("Key ID and permission required")
	}

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
	user, err := getUserFromContext(c)
	if err != nil {
		return err
	}

	keyID := c.Param("id")
	permission := c.Param("permission")
	if keyID == "" || permission == "" {
		return errcodes.ValidationError("Key ID and permission required")
	}

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
	user, err := getUserFromContext(c)
	if err != nil {
		return err
	}

	keyID := c.Param("id")
	if keyID == "" {
		return errcodes.ValidationError("Key ID required")
	}

	shortURL, err := h.service.GenerateShortURL(c.Request().Context(), user.ID, keyID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return errcodes.NotFound("API key")
		}
		return pkgerrors.WithStack(err)
	}

	return c.JSON(http.StatusCreated, shortURL)
}
