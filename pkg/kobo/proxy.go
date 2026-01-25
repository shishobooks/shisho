package kobo

import (
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

var koboStoreClient = &http.Client{
	Timeout: 30 * time.Second,
}

const koboStoreBaseURL = "https://storeapi.kobo.com"

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

	// Copy relevant headers from device
	headersToForward := []string{
		"Authorization",
		"Content-Type",
		"X-Kobo-SyncToken",
		"User-Agent",
	}
	for _, h := range headersToForward {
		if v := c.Request().Header.Get(h); v != "" {
			proxyReq.Header.Set(h, v)
		}
	}

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

	// Copy response headers
	for k, v := range resp.Header {
		for _, val := range v {
			c.Response().Header().Add(k, val)
		}
	}

	c.Response().WriteHeader(resp.StatusCode)
	_, err = io.Copy(c.Response().Writer, resp.Body)
	return errors.WithStack(err)
}
