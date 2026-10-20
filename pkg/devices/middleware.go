package devices

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

// deviceContextKey is the echo context key DeviceAuth stores the Device under.
const deviceContextKey = "device"

// connectionDeniedMessages holds the 403 message for a Device whose type
// does not use a connection, worded for the connection's own clients.
var connectionDeniedMessages = map[string]string{
	ConnectionKoboSync: "Only a Kobo Device can use Kobo sync.",
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
// its Device's type uses connection, and loads its owner with AuthenticateOwner,
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

// OPDSAuth signs an OPDS request in with Basic Auth. A username with the
// credential of one of that user's OPDS app Devices signs in as the Device,
// so feeds and downloads follow its Delivery Rules and scope. Anything else
// goes to passwordAuth, so the account password still signs in without a
// Device. A Device is looked up before the password is checked, so signing
// in with a credential costs no bcrypt comparison, and only for a password
// shaped like a credential, so a password sign-in answered from
// passwordAuth's cache costs no query. A Device whose owner is deactivated
// gets the Basic challenge, as a wrong password does; an owner without
// books:read gets the 403 password sign-in gets from RequirePermission.
// Like DeviceAuth, a Device sign-in does not wait on a forced password
// reset, which only the account password is subject to.
func (m *Middleware) OPDSAuth(passwordAuth echo.MiddlewareFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		withPassword := passwordAuth(next)
		return func(c echo.Context) error {
			username, credential, ok := auth.BasicCredentials(c.Request())
			if !ok || !isCredentialShaped(credential) {
				return withPassword(c)
			}
			device, err := m.service.GetByOwnerCredential(c.Request().Context(), username, credential)
			if err != nil {
				return errors.WithStack(err)
			}
			if device == nil || !device.HasConnection(ConnectionOPDS) {
				return withPassword(c)
			}

			user, err := m.service.AuthenticateOwner(c.Request().Context(), device)
			var codeErr *errcodes.Error
			if errors.As(err, &codeErr) && codeErr.HTTPCode == http.StatusUnauthorized {
				return auth.RespondBasicAuthRequired(c)
			}
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

// isCredentialShaped reports whether s could be a Device credential: the
// right length, in the credential alphabet.
func isCredentialShaped(s string) bool {
	if len(s) != credentialLength {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune(credentialAlphabet, r) {
			return false
		}
	}
	return true
}

// SetDevice stores device for RequireDevice. Only DeviceAuth, OPDSAuth,
// middleware that updates the Device, and tests standing in for them call
// it.
func SetDevice(c echo.Context, device *Device) {
	c.Set(deviceContextKey, device)
}

// SignedInDevice returns the Device a request signed in as, or nil for one
// signed in without a Device, as OPDS allows with the account password.
// Routes that always need a Device use RequireDevice.
func SignedInDevice(c echo.Context) *Device {
	device, _ := c.Get(deviceContextKey).(*Device)
	return device
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
