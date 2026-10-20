package devices

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	service     *Service
	listService *lists.Service
}

func newHandler(service *Service, listService *lists.Service) *handler {
	return &handler{service: service, listService: listService}
}

func buildDeviceResponse(device *Device) DeviceResponse {
	resp := DeviceResponse{
		Device:      *device,
		Credential:  device.Credential,
		Connections: device.ConnectionNames(),
	}
	switch {
	case device.ScopeType == ScopeLibrary && device.ScopeLibrary != nil:
		resp.ScopeName = device.ScopeLibrary.Name
	case device.ScopeType == ScopeList && device.ScopeList != nil:
		resp.ScopeName = device.ScopeList.Name
	}
	return resp
}

func buildAdminDeviceResponse(device *Device) AdminDeviceResponse {
	resp := AdminDeviceResponse{
		Device:      *device,
		Connections: device.ConnectionNames(),
	}
	if device.Owner != nil {
		resp.Owner = *device.Owner
	}
	return resp
}

func deviceNotFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return errcodes.NotFound("Device")
	}
	return errors.WithStack(err)
}

// List returns the current user's Devices.
func (h *handler) List(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	devices, err := h.service.List(c.Request().Context(), user.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	resp := make([]DeviceResponse, len(devices))
	for i, device := range devices {
		resp[i] = buildDeviceResponse(device)
	}
	return c.JSON(http.StatusOK, resp)
}

// Create adds a Device for the current user.
func (h *handler) Create(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	var payload CreateDevicePayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	device, err := h.service.Create(c.Request().Context(), user.ID, payload.Name, payload.DeviceType)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusCreated, buildDeviceResponse(device))
}

// Retrieve returns one of the current user's Devices.
func (h *handler) Retrieve(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	device, err := h.service.Retrieve(c.Request().Context(), user.ID, c.Param("id"))
	if err != nil {
		return deviceNotFound(err)
	}

	return c.JSON(http.StatusOK, buildDeviceResponse(device))
}

// Update renames a Device or sets its scope.
func (h *handler) Update(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	var payload UpdateDevicePayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	device, err := h.service.Retrieve(ctx, user.ID, c.Param("id"))
	if err != nil {
		return deviceNotFound(err)
	}

	opts := UpdateDeviceOptions{Name: payload.Name}

	if payload.Scope != nil {
		scope, err := h.validateScope(ctx, user, *payload.Scope)
		if err != nil {
			return err
		}
		opts.Scope = &scope
	}

	device, err = h.service.Update(ctx, user.ID, device.ID, opts)
	if err != nil {
		return deviceNotFound(err)
	}

	return c.JSON(http.StatusOK, buildDeviceResponse(device))
}

// validateScope turns a scope payload into a Scope whose library or list
// the user can see.
func (h *handler) validateScope(ctx context.Context, user *models.User, payload DeviceScopePayload) (Scope, error) {
	switch payload.Type {
	case ScopeLibrary:
		if payload.LibraryID == nil {
			return Scope{}, errcodes.ValidationError("Choose a library for a library scope.")
		}
		if !user.HasLibraryAccess(*payload.LibraryID) {
			return Scope{}, errcodes.ValidationError("You do not have access to that library.")
		}
		exists, err := h.service.libraryExists(ctx, *payload.LibraryID)
		if err != nil {
			return Scope{}, err
		}
		if !exists {
			return Scope{}, errcodes.ValidationError("You do not have access to that library.")
		}
		return Scope{Type: ScopeLibrary, LibraryID: payload.LibraryID}, nil
	case ScopeList:
		if payload.ListID == nil {
			return Scope{}, errcodes.ValidationError("Choose a list for a list scope.")
		}
		canView, err := h.listService.CanView(ctx, *payload.ListID, user.ID)
		if err != nil {
			return Scope{}, errors.WithStack(err)
		}
		if !canView {
			return Scope{}, errcodes.ValidationError("You do not have access to that list.")
		}
		return Scope{Type: ScopeList, ListID: payload.ListID}, nil
	default:
		return Scope{Type: ScopeAll}, nil
	}
}

// RegenerateCredential gives a Device a new credential.
func (h *handler) RegenerateCredential(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	device, err := h.service.RegenerateCredential(c.Request().Context(), user.ID, c.Param("id"))
	if err != nil {
		return deviceNotFound(err)
	}

	return c.JSON(http.StatusOK, buildDeviceResponse(device))
}

// Delete deletes one of the current user's Devices.
func (h *handler) Delete(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	if err := h.service.Delete(c.Request().Context(), user.ID, c.Param("id")); err != nil {
		return deviceNotFound(err)
	}

	return c.NoContent(http.StatusNoContent)
}

// GenerateShortURL creates a temporary short URL for a Device's eReader
// browser.
func (h *handler) GenerateShortURL(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	shortURL, err := h.service.GenerateShortURL(c.Request().Context(), user.ID, c.Param("id"))
	if err != nil {
		return deviceNotFound(err)
	}

	return c.JSON(http.StatusCreated, shortURL)
}

// ClearKoboSync clears a Device's Kobo sync history, forcing a fresh sync.
func (h *handler) ClearKoboSync(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	if err := h.service.ClearKoboSyncHistory(c.Request().Context(), user.ID, c.Param("id")); err != nil {
		return deviceNotFound(err)
	}

	return c.NoContent(http.StatusNoContent)
}

// ListAll returns every user's Devices, without credentials.
func (h *handler) ListAll(c echo.Context) error {
	devices, err := h.service.ListAll(c.Request().Context())
	if err != nil {
		return errors.WithStack(err)
	}

	resp := make([]AdminDeviceResponse, len(devices))
	for i, device := range devices {
		resp[i] = buildAdminDeviceResponse(device)
	}
	return c.JSON(http.StatusOK, resp)
}

// Revoke deletes any user's Device.
func (h *handler) Revoke(c echo.Context) error {
	if err := h.service.Revoke(c.Request().Context(), c.Param("id")); err != nil {
		return deviceNotFound(err)
	}
	return c.NoContent(http.StatusNoContent)
}
