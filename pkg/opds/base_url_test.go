package opds

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestGetBaseURL_DirectRequestWithoutPortInHost_UsesConnectionPort(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/opds/v1/catalog", nil)
	req.Host = "192.168.1.10"
	local := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 5173}
	req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, local))
	c := echo.New().NewContext(req, httptest.NewRecorder())

	assert.Equal(t, "http://192.168.1.10:5173/opds/v1", getBaseURL(c))
}

func TestGetBaseURL_ProxiedRequestKeepsForwardedOriginAndPrefix(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/opds/v1/catalog", nil)
	req.Host = "shisho.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Prefix", "/books")
	local := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 5173}
	req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, local))
	c := echo.New().NewContext(req, httptest.NewRecorder())

	assert.Equal(t, "https://shisho.example.com/books/opds/v1", getBaseURL(c))
}
