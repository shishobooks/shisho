package kobo

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyOutgoingProxyHeaders(t *testing.T) {
	t.Parallel()
	src := http.Header{}
	src.Set("Authorization", "Bearer abc")
	src.Set("Content-Type", "application/json")
	src.Set("User-Agent", "Kobo")
	src.Set("Accept", "application/json")
	src.Set("Accept-Language", "en-US")
	src.Set("X-Kobo-DeviceId", "kobo-123")
	src.Set("X-Kobo-Affiliate", "Kobo")
	src.Set("X-Kobo-SyncToken", "should-not-forward")
	src.Set("Cookie", "session=secret")
	src.Set("Host", "shisho.local")

	dst := http.Header{}
	applyOutgoingProxyHeaders(dst, src)

	assert.Equal(t, "Bearer abc", dst.Get("Authorization"))
	assert.Equal(t, "application/json", dst.Get("Content-Type"))
	assert.Equal(t, "Kobo", dst.Get("User-Agent"))
	assert.Equal(t, "application/json", dst.Get("Accept"))
	assert.Equal(t, "en-US", dst.Get("Accept-Language"))
	assert.Equal(t, "kobo-123", dst.Get("X-Kobo-DeviceId"))
	assert.Equal(t, "Kobo", dst.Get("X-Kobo-Affiliate"))
	assert.Empty(t, dst.Get("X-Kobo-SyncToken"), "outbound sync token must not be forwarded")
	assert.Empty(t, dst.Get("Cookie"))
	assert.Empty(t, dst.Get("Host"))
}

func TestApplyIncomingProxyHeaders(t *testing.T) {
	t.Parallel()
	src := http.Header{}
	src.Set("Content-Type", "application/json")
	src.Set("X-Kobo-Apitoken", "e30=")
	src.Set("X-Kobo-RecentReads", "1")
	src.Add("Set-Cookie", "auth=value")
	src.Set("WWW-Authenticate", "Bearer realm=kobo")
	src.Set("Cache-Control", "no-store")
	src.Set("Server", "nginx")

	dst := http.Header{}
	applyIncomingProxyHeaders(dst, src)

	assert.Equal(t, "application/json", dst.Get("Content-Type"))
	assert.Equal(t, "e30=", dst.Get("X-Kobo-Apitoken"))
	assert.Equal(t, "1", dst.Get("X-Kobo-RecentReads"))
	assert.Empty(t, dst.Get("Set-Cookie"))
	assert.Empty(t, dst.Get("WWW-Authenticate"))
	assert.Empty(t, dst.Get("Cache-Control"))
	assert.Empty(t, dst.Get("Server"))
}
