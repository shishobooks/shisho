// Package shishoapp is the Shisho app's Device surface (ADR 0020): signing
// in, server info, the catalog change feed (ADR 0023), the journal (ADR
// 0024), Original File downloads and streams (ADR 0022), and covers. It's
// versioned apart from the web app's /api, because a phone app can't be
// forced to update and a self-hosted server upgrades whenever its owner
// gets to it.
package shishoapp

import (
	"slices"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

const (
	// HeaderAppProtocol carries the app's protocol on every app request,
	// including its /api requests. HeaderAppVersion carries the app's
	// version, which its Device records.
	HeaderAppProtocol = "X-Shisho-App-Protocol"
	HeaderAppVersion  = "X-Shisho-App-Version"

	// HeaderCapabilities lists, comma-separated, the capabilities the
	// server offers to the protocol a request names. Every /mobile and /api
	// response carries it, errors included, so a server rolled back to an
	// older release is noticed mid-session. The KOReader plugin's routes
	// use the same header for their own list.
	HeaderCapabilities = "X-Shisho-Capabilities"

	// CookieCredential and CookieProtocol carry the Device credential and
	// the app's protocol to an audiobook stream, since the system player
	// makes its own requests and takes cookies but no headers. Only the
	// stream route reads them.
	CookieCredential = "shisho_app_credential" //nolint:gosec // A cookie name, not a credential.
	CookieProtocol   = "shisho_app_protocol"
	// CookieAppVersion carries the app's version to a stream, for the access
	// log.
	CookieAppVersion = "shisho_app_version"

	// The headers a refused app that signed in reads what to tell the user
	// from, since the refusal has Shisho's usual error body.
	HeaderProtocolMin   = "X-Shisho-App-Protocol-Min"
	HeaderProtocolMax   = "X-Shisho-App-Protocol-Max"
	HeaderServerVersion = "X-Shisho-Server-Version"

	// Protocol is the newest app protocol this server knows. An app
	// protocol stays accepted for at least 12 months after a newer one
	// replaces it, and the release notes announce when one is dropped
	// (ADR 0020). Raise ProtocolMin only then.
	Protocol    = 1
	ProtocolMin = 1
)

// capabilities holds, by protocol, the capabilities added while it was the
// newest. An app turns such a feature on for a server only when that
// server lists it, so a server from before the feature, or rolled back to
// before it, never offers it. A protocol bump starts a new entry, since an
// app on the new protocol assumes everything listed before it. Never remove
// one while its protocol is accepted. Protocol 1's baseline is signing in,
// the feed, covers, Original File downloads and streams, and the journal's
// position and session events. A journal event type added later is a
// capability (ADR 0024).
var capabilities = map[int][]string{
	1: {CapabilityBookmarks},
}

// Contract is the range of app protocols this server accepts and what it
// offers each. Development servers can narrow the range to try the
// too-old and too-new paths.
type Contract struct {
	Min, Max     int
	capabilities map[int][]string
}

// NewContract returns the server's contract. devRange, "MIN-MAX" such as
// "2-2", replaces the accepted range to try an app against a server too new
// or too old for it; production servers leave it empty.
func NewContract(devRange string) (*Contract, error) {
	if devRange == "" {
		return newContract(ProtocolMin, Protocol), nil
	}
	minText, maxText, ok := strings.Cut(devRange, "-")
	minProtocol, minErr := strconv.Atoi(strings.TrimSpace(minText))
	maxProtocol, maxErr := strconv.Atoi(strings.TrimSpace(maxText))
	if !ok || minErr != nil || maxErr != nil || minProtocol < 0 || maxProtocol < minProtocol {
		return nil, errors.Errorf("SHISHO_DEV_APP_PROTOCOLS is %q; set it to a range such as 2-2, or leave it empty", devRange)
	}
	return newContract(minProtocol, maxProtocol), nil
}

func newContract(minProtocol, maxProtocol int) *Contract {
	return &Contract{Min: minProtocol, Max: maxProtocol, capabilities: capabilities}
}

// requestProtocol is the protocol a request names, or 0 for none.
func requestProtocol(c echo.Context) int {
	protocol, err := strconv.Atoi(c.Request().Header.Get(HeaderAppProtocol))
	if err != nil {
		return 0
	}
	return protocol
}

// accepts reports whether the server accepts protocol.
func (k *Contract) accepts(protocol int) bool {
	return protocol >= k.Min && protocol <= k.Max
}

// Capabilities is what the server offers an app speaking protocol, and
// nothing for a protocol it doesn't accept.
func (k *Contract) Capabilities(protocol int) []string {
	if !k.accepts(protocol) {
		return []string{}
	}
	return slices.Clone(k.capabilities[protocol])
}

// info is the server info for a request naming protocol.
func (k *Contract) info(protocol int, demoMode bool, renderKey string) ServerInfo {
	return ServerInfo{
		ProtocolMin:        k.Min,
		ProtocolMax:        k.Max,
		Capabilities:       k.Capabilities(protocol),
		DemoMode:           demoMode,
		ThumbnailRenderKey: renderKey,
	}
}

// CapabilitiesHeader sets HeaderCapabilities on every /mobile and /api
// response, before any route runs, so refusals and errors carry it too. A
// request without an accepted protocol, such as the web app's, gets an
// empty list. Register it with Use before the Demo Mode middleware.
func (k *Contract) CapabilitiesHeader(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		path := c.Request().URL.Path
		if path == "/api" || path == "/mobile" || strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/mobile/") {
			c.Response().Header().Set(HeaderCapabilities, strings.Join(k.Capabilities(requestProtocol(c)), ","))
		}
		return next(c)
	}
}

// refuseProtocol answers a request whose protocol the server doesn't accept
// with 426. A request that signed in learns the accepted range and the
// server's version from the headers, so the app can name both versions.
func (k *Contract) refuseProtocol(c echo.Context, protocol int, serverVersion string) error {
	header := c.Response().Header()
	header.Set(HeaderProtocolMin, strconv.Itoa(k.Min))
	header.Set(HeaderProtocolMax, strconv.Itoa(k.Max))
	header.Set(HeaderServerVersion, serverVersion)
	if protocol > k.Max {
		return errcodes.ServerUpdateRequired()
	}
	return errcodes.AppUpdateRequired()
}
