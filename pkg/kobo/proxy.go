package kobo

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

var koboStoreClient = &http.Client{
	Timeout: 30 * time.Second,
}

// koboStoreBaseURL is the upstream store API. Declared as a var so tests can
// point it at an httptest.Server.
var koboStoreBaseURL = "https://storeapi.kobo.com"

// outgoingProxyHeaderAllowlist names device-supplied request headers that are
// safe to forward to the Kobo store. Any X-Kobo-* header is also forwarded
// EXCEPT X-Kobo-SyncToken — that token is scoped to our sync points and would
// confuse the upstream store API.
var outgoingProxyHeaderAllowlist = map[string]bool{
	"Authorization":   true,
	"Content-Type":    true,
	"User-Agent":      true,
	"Accept":          true,
	"Accept-Language": true,
}

// applyOutgoingProxyHeaders copies allowlisted request headers from src to dst.
func applyOutgoingProxyHeaders(dst, src http.Header) {
	for k, vals := range src {
		ck := http.CanonicalHeaderKey(k)
		if outgoingProxyHeaderAllowlist[ck] {
			for _, v := range vals {
				dst.Add(ck, v)
			}
			continue
		}
		// http.CanonicalHeaderKey lowercases letters after the first per
		// segment, so "X-Kobo-SyncToken" canonicalizes to "X-Kobo-Synctoken".
		if strings.HasPrefix(ck, "X-Kobo-") && ck != "X-Kobo-Synctoken" {
			for _, v := range vals {
				dst.Add(ck, v)
			}
		}
	}
}

// applyIncomingProxyHeaders copies safe response headers from the Kobo store
// back to the device — Content-Type so the device can parse the body, plus any
// X-Kobo-* response header. Everything else (Set-Cookie, WWW-Authenticate,
// CORS, cache, server fingerprints) is dropped to avoid leaking store state
// onto our connection.
func applyIncomingProxyHeaders(dst, src http.Header) {
	for k, vals := range src {
		ck := http.CanonicalHeaderKey(k)
		if ck == "Content-Type" || strings.HasPrefix(ck, "X-Kobo-") {
			for _, v := range vals {
				dst.Add(ck, v)
			}
		}
	}
}

// proxyToKoboStore forwards the request to the real Kobo store API.
func proxyToKoboStore(c echo.Context) error {
	koboPath := StripKoboPrefix(c.Request().URL.Path)
	targetURL := koboStoreBaseURL + koboPath

	if c.Request().URL.RawQuery != "" {
		targetURL += "?" + c.Request().URL.RawQuery
	}

	proxyReq, err := http.NewRequestWithContext(
		c.Request().Context(),
		c.Request().Method,
		targetURL,
		c.Request().Body,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	applyOutgoingProxyHeaders(proxyReq.Header, c.Request().Header)

	resp, err := koboStoreClient.Do(proxyReq)
	if err != nil {
		// If we can't reach Kobo, return a minimal OK response
		return c.JSON(http.StatusOK, map[string]interface{}{})
	}
	defer resp.Body.Close()

	// If the Kobo store returns a client error (4xx), return an empty 200.
	// The device treats 4xx from endpoints like /v1/user/wishlist, /v1/deals, etc.
	// as fatal errors and aborts sync. Returning 200 lets the device proceed.
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return c.JSON(http.StatusOK, map[string]interface{}{})
	}

	applyIncomingProxyHeaders(c.Response().Header(), resp.Header)

	c.Response().WriteHeader(resp.StatusCode)
	_, err = io.Copy(c.Response().Writer, resp.Body)
	return errors.WithStack(err)
}
