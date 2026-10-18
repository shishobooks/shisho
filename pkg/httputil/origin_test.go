package httputil

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPublicOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		host      string
		headers   map[string]string
		tls       bool
		localPort int
		want      string
	}{
		{
			name:      "direct request falls back to the connection's port",
			host:      "192.168.1.10",
			localPort: 5173,
			want:      "http://192.168.1.10:5173",
		},
		{
			name:      "direct request on the scheme's default port adds none",
			host:      "192.168.1.10",
			localPort: 80,
			want:      "http://192.168.1.10",
		},
		{
			name:      "direct TLS request on a non-default port",
			host:      "nas.local",
			tls:       true,
			localPort: 8443,
			want:      "https://nas.local:8443",
		},
		{
			name:      "direct TLS request on 443 adds none",
			host:      "nas.local",
			tls:       true,
			localPort: 443,
			want:      "https://nas.local",
		},
		{
			name:      "proxy without X-Forwarded-Port never uses the connection's port",
			host:      "shisho.example.com",
			headers:   map[string]string{"X-Forwarded-Proto": "https"},
			localPort: 5173,
			want:      "https://shisho.example.com",
		},
		{
			name:      "X-Forwarded-Host alone also marks a proxied request",
			host:      "localhost:5173",
			headers:   map[string]string{"X-Forwarded-Host": "shisho.example.com"},
			localPort: 5173,
			want:      "http://shisho.example.com",
		},
		{
			name:      "Host that already has a port keeps it",
			host:      "192.168.1.10:8080",
			localPort: 5173,
			want:      "http://192.168.1.10:8080",
		},
		{
			name:      "bare IPv6 Host gets the port appended",
			host:      "[fd00::1]",
			localPort: 5173,
			want:      "http://[fd00::1]:5173",
		},
		{
			name:      "IPv6 Host with a port keeps it",
			host:      "[fd00::1]:8080",
			localPort: 5173,
			want:      "http://[fd00::1]:8080",
		},
		{
			name: "X-Forwarded-Port is appended to a forwarded host",
			host: "localhost",
			headers: map[string]string{
				"X-Forwarded-Proto": "https",
				"X-Forwarded-Host":  "shisho.example.com",
				"X-Forwarded-Port":  "8443",
			},
			localPort: 5173,
			want:      "https://shisho.example.com:8443",
		},
		{
			name:      "X-Forwarded-Port equal to the scheme's default adds none",
			host:      "shisho.example.com",
			headers:   map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Port": "443"},
			localPort: 5173,
			want:      "https://shisho.example.com",
		},
		{
			name:      "X-Forwarded-Port 80 behind an upstream TLS terminator adds none",
			host:      "shisho.example.com",
			headers:   map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Port": "80"},
			localPort: 5173,
			want:      "https://shisho.example.com",
		},
		{
			name:      "X-Forwarded-For alone marks a proxied request",
			host:      "192.168.1.10",
			headers:   map[string]string{"X-Forwarded-For": "192.168.1.50"},
			localPort: 5173,
			want:      "http://192.168.1.10",
		},
		{
			name:      "X-Forwarded-Prefix alone marks a proxied request",
			host:      "192.168.1.10",
			headers:   map[string]string{"X-Forwarded-Prefix": "/books"},
			localPort: 5173,
			want:      "http://192.168.1.10",
		},
		{
			name:      "X-Forwarded-Port does not override a port already in the host",
			host:      "shisho.example.com:9000",
			headers:   map[string]string{"X-Forwarded-Port": "8443"},
			localPort: 5173,
			want:      "http://shisho.example.com:9000",
		},
		{
			name:      "first entry of a chained X-Forwarded-Proto wins",
			host:      "shisho.example.com",
			headers:   map[string]string{"X-Forwarded-Proto": "https, http"},
			localPort: 5173,
			want:      "https://shisho.example.com",
		},
		{
			name: "unknown local address adds no port",
			host: "192.168.1.10",
			want: "http://192.168.1.10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tt.host
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			for name, value := range tt.headers {
				req.Header.Set(name, value)
			}
			if tt.localPort != 0 {
				local := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: tt.localPort}
				req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, local))
			}
			assert.Equal(t, tt.want, PublicOrigin(req))
		})
	}
}
