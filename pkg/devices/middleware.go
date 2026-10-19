package devices

import (
	"context"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

// deviceContextKey is the echo context key DeviceAuth stores the Device under.
const deviceContextKey = "device"

// connectionDeniedMessages holds the 403 message for a Device without a
// connection, worded for the connection's own clients.
var connectionDeniedMessages = map[string]string{
	ConnectionKoboSync:       "This Device does not have Kobo sync turned on.",
	ConnectionEReaderBrowser: "This Device does not have the eReader browser turned on.",
}

// Middleware authenticates the device routes (Kobo and eReader) that carry a
// Device credential in the URL.
type Middleware struct {
	service *Service
}

// NewMiddleware creates Device middleware backed by service.
func NewMiddleware(service *Service) *Middleware {
	return &Middleware{service: service}
}

// DeviceAuth validates the credential in c.Param("credential"), checks that
// its Device has connection, and loads its owner with AuthenticateOwner,
// which rejects a missing or deactivated owner (401) and one without
// books:read (403). It touches the Device's last accessed time without
// waiting, stores the Device for RequireDevice, and stores the owner with
// auth.SetUser, so handlers read it with auth.RequireUser and
// auth.RequireLibraryAccessFor like every other route.
func (m *Middleware) DeviceAuth(connection string) echo.MiddlewareFunc {
	deniedMessage, ok := connectionDeniedMessages[connection]
	if !ok {
		deniedMessage = "This Device cannot connect this way."
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			credential := c.Param("credential")
			if credential == "" {
				return errcodes.Unauthorized("Device credential required")
			}

			device, err := m.service.GetByCredential(c.Request().Context(), credential)
			if err != nil {
				return errors.WithStack(err)
			}
			if device == nil {
				return errcodes.Unauthorized("Invalid device credential")
			}

			if !device.HasConnection(connection) {
				return errcodes.Forbidden(deniedMessage)
			}

			user, err := m.service.AuthenticateOwner(c.Request().Context(), device)
			if err != nil {
				return errors.WithStack(err)
			}

			go func() {
				_ = m.service.TouchLastAccessed(context.Background(), device.ID)
			}()

			SetDevice(c, device)
			auth.SetUser(c, user)

			return next(c)
		}
	}
}

// SetDevice stores device for RequireDevice. Only DeviceAuth, middleware
// that updates the Device, and tests standing in for it call it.
func SetDevice(c echo.Context, device *Device) {
	c.Set(deviceContextKey, device)
}

// RequireDevice returns the Device DeviceAuth stored, or a 401 when there is
// none, so a route registered without DeviceAuth fails closed.
func RequireDevice(c echo.Context) (*Device, error) {
	device, ok := c.Get(deviceContextKey).(*Device)
	if !ok || device == nil {
		return nil, errcodes.Unauthorized("Device not found")
	}
	return device, nil
}
