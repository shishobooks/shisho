package koreaderplugin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/fingerprints"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/koreader"
	"github.com/shishobooks/shisho/pkg/reach"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/shishobooks/shisho/pkg/search"
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

	// HeaderCapabilities lists, comma-separated, the capabilities the
	// server offers to the protocol a request names. Every plugin response
	// carries it, errors included, so the plugin always knows what the
	// server it just heard from supports, and a server from before
	// capabilities is told apart by its absence.
	HeaderCapabilities = "X-Shisho-Capabilities"

	// HeaderSignInExpires is when a plugin downloaded from a Device's page
	// stops signing itself in, RFC 3339, so the page can say until when.
	HeaderSignInExpires = "X-Shisho-Sign-In-Expires"

	// Protocol is the protocol the plugin this server serves speaks, and
	// ProtocolMin and ProtocolMax the protocols the server accepts. Raise
	// ProtocolMin only when the server drops what an older plugin sends:
	// until then an older plugin keeps working against a newer server.
	// shisho/protocol.lua in the plugin must match Protocol.
	Protocol    = 1
	ProtocolMin = 1
	ProtocolMax = 1

	// CapabilityResendHistory says the server keeps a status or Rating sent
	// again with history only for an unrecognized copy holding none, so the
	// plugin can offer Send reading history again.
	CapabilityResendHistory = "resend_history"

	// maxReportedVersion bounds the version a plugin reports, which is
	// stored on its Device.
	maxReportedVersion = 64

	zipName = "shisho.koplugin.zip"
)

// capabilities holds, by protocol, the capabilities added while it was the
// newest. The plugin turns such a feature on for a server only when that
// server lists it, so a server from before the feature, or rolled back to
// before it, never gets it. A protocol bump starts a new entry, since a
// plugin on the new protocol assumes everything listed before it. Never
// remove one while its protocol is accepted: releasedCapabilities in the
// tests holds each one shipped. shisho/protocol.lua names the ones the
// plugin checks for.
var capabilities = map[int][]string{
	1: {CapabilityResendHistory},
}

// requestProtocol is the protocol a plugin request names, and whether the
// server accepts it. A request that names none is refused.
func requestProtocol(c echo.Context) (int, bool) {
	protocol, err := strconv.Atoi(c.Request().Header.Get(HeaderPluginProtocol))
	return protocol, err == nil && protocol >= ProtocolMin && protocol <= ProtocolMax
}

// capabilitiesFor is what the server offers to a plugin speaking protocol,
// and nothing for a protocol it doesn't accept.
func capabilitiesFor(protocol int) []string {
	if protocol < ProtocolMin || protocol > ProtocolMax {
		return nil
	}
	return capabilities[protocol]
}

// listCapabilities sets HeaderCapabilities before the route's own
// middleware runs, so every response from a plugin route carries it,
// including a refused sign-in or protocol. The list names features, not
// the server's version or build, so it isn't kept from callers that aren't
// signed in. A route that doesn't exist, or one Demo Mode refuses, answers
// before it runs; the plugin reads that as listing none.
func listCapabilities(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		var listed []string
		if protocol, ok := requestProtocol(c); ok {
			listed = capabilitiesFor(protocol)
		}
		c.Response().Header().Set(HeaderCapabilities, strings.Join(listed, ","))
		return next(c)
	}
}

type handler struct {
	db      *bun.DB
	devices *devices.Service
	// connections records the plugin's connections and announces its first.
	connections *devices.Connections
	reading     *reading.Service
	documents   *koreader.Documents
	hashes      *fingerprints.Service
	reach       *reach.Service
	search      *search.Service
	bundle      *Bundle
	limiter     *addressLimiter
}

// newHandler builds the plugin's handler. broker hears of each Device's
// first connection; nil announces nothing.
func newHandler(db *bun.DB, bundle *Bundle, bookService *books.Service, broker *events.Broker) *handler {
	deviceService := devices.NewService(db)
	return &handler{
		db:          db,
		devices:     deviceService,
		connections: devices.NewConnections(deviceService, broker),
		reading:     reading.NewService(db),
		documents:   koreader.NewDocuments(db, bookService),
		hashes:      fingerprints.NewService(db),
		reach:       reach.NewService(db, bookService),
		search:      search.NewService(db),
		bundle:      bundle,
		limiter:     newAddressLimiter(),
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
		if _, ok := requestProtocol(c); !ok {
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
	if err := h.connections.RecordPluginConnection(ctx, device, reportedVersion(c)); err != nil {
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
	if err := h.connections.RecordPluginConnection(c.Request().Context(), device, reportedVersion(c)); err != nil {
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
	if err := h.connections.RecordPluginConnection(c.Request().Context(), device, reportedVersion(c)); err != nil {
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
	user, device, err := h.ownDevice(c)
	if err != nil {
		return err
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
	c.Response().Header().Set(HeaderSignInExpires, short.ExpiresAt.UTC().Format(time.RFC3339))
	return sendZip(c, data)
}

func sendZip(c echo.Context, data []byte) error {
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+zipName+`"`)
	return errors.WithStack(c.Blob(http.StatusOK, "application/zip", data))
}
