package kobo

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveDirect sends a request the way a Kobo does when it reaches Shisho
// without a proxy: a Host header with no port, no forwarded headers, and a
// connection that arrived on port 5173.
func (f *koboScopeFixture) serveDirect(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "192.168.1.10"
	local := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 5173}
	req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, local))
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return rec
}

func (f *koboScopeFixture) directBase() string {
	return "http://192.168.1.10:5173" + f.prefix("all")
}

func TestHandleInitialization_DirectNonDefaultPort_URLsCarryPort(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	rec := f.serveDirect(t, f.prefix("all")+"/v1/initialization")

	var body map[string]map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	resources := body["Resources"]
	base := f.directBase()
	assert.Equal(t, base, resources["image_host"])
	assert.Equal(t, base+"/v1/books/{ImageId}/thumbnail/{Width}/{Height}/false/image.jpg", resources["image_url_template"])
	assert.Equal(t, base+"/v1/books/{ImageId}/thumbnail/{Width}/{Height}/{Quality}/{IsGreyscale}/image.jpg", resources["image_url_quality_template"])
}

func TestHandleSync_DirectNonDefaultPort_DownloadURLsCarryPort(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	rec := f.serveDirect(t, f.prefix("all")+"/v1/library/sync")

	var items []syncItem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &items))
	item := itemFor(t, items, f.fileA.ID)
	require.NotNil(t, item.NewEntitlement)
	assertDownloadURLsUnder(t, item.NewEntitlement.BookMetadata["DownloadUrls"], f.directBase())
}

func TestHandleMetadata_DirectNonDefaultPort_DownloadURLsCarryPort(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	rec := f.serveDirect(t, f.prefix("all")+"/v1/library/"+ShishoID(f.fileA.ID)+"/metadata")

	var metadata []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &metadata))
	require.Len(t, metadata, 1)
	assertDownloadURLsUnder(t, metadata[0]["DownloadUrls"], f.directBase())
}

func assertDownloadURLsUnder(t *testing.T, raw any, base string) {
	t.Helper()
	urls, ok := raw.([]any)
	require.True(t, ok, "DownloadUrls missing: %v", raw)
	require.NotEmpty(t, urls)
	for _, u := range urls {
		entry, ok := u.(map[string]any)
		require.True(t, ok)
		assert.Contains(t, entry["Url"], base+"/", "download URL must carry the connection's port")
	}
}
