package koreaderplugin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/fingerprints"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/koreader"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/uptrace/bun"
)

const (
	// HeaderPluginVersion and HeaderPluginProtocol carry the plugin's
	// version and protocol version on every plugin request.
	HeaderPluginVersion  = "X-Shisho-Plugin-Version"
	HeaderPluginProtocol = "X-Shisho-Plugin-Protocol"

	// The headers a refused, signed-in plugin reads ServerInfo from.
	HeaderServerProtocolMin    = "X-Shisho-Protocol-Min"
	HeaderServerProtocolMax    = "X-Shisho-Protocol-Max"
	HeaderServerPluginVersion  = "X-Shisho-Server-Plugin-Version"
	HeaderServerPluginHash     = "X-Shisho-Server-Plugin-Hash"
	HeaderServerPluginProtocol = "X-Shisho-Server-Plugin-Protocol"

	// Protocol is the protocol the plugin this server serves speaks, and
	// ProtocolMin and ProtocolMax the protocols the server accepts. Raise
	// ProtocolMin only when the server drops what an older plugin sends:
	// until then an older plugin keeps working against a newer server.
	// shisho/protocol.lua in the plugin must match Protocol.
	Protocol    = 1
	ProtocolMin = 1
	ProtocolMax = 1

	// maxReportedVersion bounds the version a plugin reports, which is
	// stored on its Device.
	maxReportedVersion = 64

	zipName = "shisho.koplugin.zip"
)

type handler struct {
	db        *bun.DB
	devices   *devices.Service
	reading   *reading.Service
	documents *koreader.Documents
	hashes    *fingerprints.Service
	bundle    *Bundle
	limiter   *addressLimiter
}

func newHandler(db *bun.DB, bundle *Bundle, bookService *books.Service) *handler {
	return &handler{
		db:        db,
		devices:   devices.NewService(db),
		reading:   reading.NewService(db),
		documents: koreader.NewDocuments(db, bookService),
		hashes:    fingerprints.NewService(db),
		bundle:    bundle,
		limiter:   newAddressLimiter(),
	}
}

func (h *handler) serverInfo() ServerInfo {
	return ServerInfo{
		ProtocolMin:    ProtocolMin,
		ProtocolMax:    ProtocolMax,
		PluginVersion:  h.bundle.Version,
		PluginHash:     h.bundle.Hash,
		PluginProtocol: Protocol,
	}
}

// requireProtocol refuses a plugin whose protocol the server doesn't
// accept, including one that sends none. A refused plugin that signed in
// learns what to update to from the response's headers, since the refusal
// has Shisho's usual error body.
func (h *handler) requireProtocol(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		protocol, err := strconv.Atoi(c.Request().Header.Get(HeaderPluginProtocol))
		if err != nil || protocol < ProtocolMin || protocol > ProtocolMax {
			if devices.SignedInDevice(c) != nil {
				info := h.serverInfo()
				header := c.Response().Header()
				header.Set(HeaderServerProtocolMin, strconv.Itoa(info.ProtocolMin))
				header.Set(HeaderServerProtocolMax, strconv.Itoa(info.ProtocolMax))
				header.Set(HeaderServerPluginVersion, info.PluginVersion)
				header.Set(HeaderServerPluginHash, info.PluginHash)
				header.Set(HeaderServerPluginProtocol, strconv.Itoa(info.PluginProtocol))
			}
			return errcodes.PluginUpdateRequired()
		}
		return next(c)
	}
}

// reportedVersion is the plugin version the request carries.
func reportedVersion(c echo.Context) string {
	v := strings.TrimSpace(c.Request().Header.Get(HeaderPluginVersion))
	if len(v) > maxReportedVersion {
		v = v[:maxReportedVersion]
	}
	return v
}

// pair exchanges a Device short code for the Device's sign-in. It needs no
// sign-in, so attempts are limited per address. A code works for its whole
// lifetime, as the eReader browser's short URL does, and reveals nothing
// that URL's redirect doesn't.
func (h *handler) pair(c echo.Context) error {
	ctx := c.Request().Context()
	if !h.limiter.allow(clientAddress(c.Request())) {
		return errcodes.TooManyRequests()
	}
	var payload PairPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}
	device, err := h.devices.ResolveShortCode(ctx, payload.Code)
	if err != nil {
		return errors.WithStack(err)
	}
	if device == nil || !device.HasConnection(devices.ConnectionKOReaderPlugin) {
		return errcodes.NotFound("Pairing code")
	}
	user, err := h.devices.AuthenticateOwner(ctx, device)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := h.devices.RecordPluginConnection(ctx, device.ID, reportedVersion(c)); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(c.JSON(http.StatusOK, SignInResponse{
		Username:   user.Username,
		Credential: device.Credential,
		DeviceID:   device.ID,
		DeviceName: device.Name,
		Server:     h.serverInfo(),
	}))
}

// status is a signed-in plugin checking in: it records the connection and
// answers with what the server serves. Signing in with a username and
// Device password is a status call that succeeds.
func (h *handler) status(c echo.Context) error {
	device, err := devices.RequireDevice(c)
	if err != nil {
		return err
	}
	if err := h.devices.RecordPluginConnection(c.Request().Context(), device.ID, reportedVersion(c)); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(c.JSON(http.StatusOK, StatusResponse{
		DeviceID:   device.ID,
		DeviceName: device.Name,
		Server:     h.serverInfo(),
	}))
}

// download serves the plugin to update itself from. It accepts any
// protocol, so a plugin too old for the server can still fetch the one
// that replaces it.
func (h *handler) download(c echo.Context) error {
	device, err := devices.RequireDevice(c)
	if err != nil {
		return err
	}
	if err := h.devices.RecordPluginConnection(c.Request().Context(), device.ID, reportedVersion(c)); err != nil {
		return errors.WithStack(err)
	}
	return sendZip(c, h.bundle.Zip())
}

// ownerDownload serves the plugin from a KOReader Device's page, carrying
// the server's URL as the owner reached it and a fresh Device short code,
// so it signs itself in on first launch. The new code replaces any earlier
// one, as getting a new setup URL does.
func (h *handler) ownerDownload(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	device, err := h.devices.Retrieve(ctx, user.ID, c.Param("id"))
	if errors.Is(err, devices.ErrNotFound) {
		return errcodes.NotFound("Device")
	}
	if err != nil {
		return errors.WithStack(err)
	}
	if !device.HasConnection(devices.ConnectionKOReaderPlugin) {
		return errcodes.InvalidState("Only a KOReader Device can use the KOReader plugin.")
	}
	short, err := h.devices.GenerateShortURL(ctx, user.ID, device.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	// As OPDS and Kobo do, behind a proxy too.
	serverURL := httputil.PublicOrigin(c.Request()) + c.Request().Header.Get("X-Forwarded-Prefix")
	data, err := h.bundle.PairedZip(serverURL, short.ShortCode, short.ExpiresAt)
	if err != nil {
		return err
	}
	return sendZip(c, data)
}

func sendZip(c echo.Context, data []byte) error {
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+zipName+`"`)
	return errors.WithStack(c.Blob(http.StatusOK, "application/zip", data))
}
