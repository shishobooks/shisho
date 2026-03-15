package plugins

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/stretchr/testify/assert"
)

func testLogger() logger.Logger {
	return logger.New()
}

// testServerHost returns the host:port from an httptest server URL,
// suitable for use in the allowedDomains list.
func testServerHost(srv *httptest.Server) string {
	u, _ := url.Parse(srv.URL)
	return u.Host
}

func TestDownloadCoverFromURL_Success(t *testing.T) {
	t.Parallel()

	fakeJPEG := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(fakeJPEG)
	}))
	defer srv.Close()

	md := &mediafile.ParsedMetadata{CoverURL: srv.URL + "/cover.jpg"}
	ok := downloadCoverFromURL(context.Background(), md, []string{testServerHost(srv)}, testLogger())

	assert.True(t, ok)
	assert.Equal(t, fakeJPEG, md.CoverData)
	assert.Equal(t, "image/jpeg", md.CoverMimeType)
}

func TestDownloadCoverFromURL_CoverDataTakesPrecedence(t *testing.T) {
	t.Parallel()

	existing := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	md := &mediafile.ParsedMetadata{
		CoverData:     existing,
		CoverMimeType: "image/jpeg",
		CoverURL:      "https://should-not-be-called.example.com/cover.jpg",
	}

	ok := downloadCoverFromURL(context.Background(), md, []string{"example.com"}, testLogger())

	assert.False(t, ok, "should skip when CoverData already set")
	assert.Equal(t, existing, md.CoverData, "CoverData should be unchanged")
}

func TestDownloadCoverFromURL_NonImageRejected(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>Not an image</html>"))
	}))
	defer srv.Close()

	md := &mediafile.ParsedMetadata{CoverURL: srv.URL + "/not-image"}
	ok := downloadCoverFromURL(context.Background(), md, []string{testServerHost(srv)}, testLogger())

	assert.False(t, ok, "should reject non-image content type")
	assert.Nil(t, md.CoverData, "CoverData should remain nil")
}

func TestDownloadCoverFromURL_EmptyURL(t *testing.T) {
	t.Parallel()

	md := &mediafile.ParsedMetadata{}
	ok := downloadCoverFromURL(context.Background(), md, nil, testLogger())

	assert.False(t, ok, "should return false for empty URL")
}

func TestDownloadCoverFromURL_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	md := &mediafile.ParsedMetadata{CoverURL: srv.URL + "/error"}
	ok := downloadCoverFromURL(context.Background(), md, []string{testServerHost(srv)}, testLogger())

	assert.False(t, ok, "should return false on server error")
	assert.Nil(t, md.CoverData, "CoverData should remain nil")
}

func TestDownloadCoverFromURL_DomainNotAllowed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0})
	}))
	defer srv.Close()

	md := &mediafile.ParsedMetadata{CoverURL: srv.URL + "/cover.jpg"}
	ok := downloadCoverFromURL(context.Background(), md, []string{"example.com"}, testLogger())

	assert.False(t, ok, "should reject URL with domain not in allowed list")
	assert.Nil(t, md.CoverData, "CoverData should remain nil")
}

func TestDownloadCoverFromURL_NoHTTPAccess(t *testing.T) {
	t.Parallel()

	md := &mediafile.ParsedMetadata{CoverURL: "https://example.com/cover.jpg"}
	ok := downloadCoverFromURL(context.Background(), md, nil, testLogger())

	assert.False(t, ok, "should reject when no domains are allowed")
	assert.Nil(t, md.CoverData, "CoverData should remain nil")
}

func TestDownloadCoverFromURL_RedirectToDisallowedDomain(t *testing.T) {
	t.Parallel()

	// Target server (will be on a different port, not in allowed list)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0})
	}))
	defer target.Close()

	// Redirect server that redirects to target
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/cover.jpg", http.StatusFound)
	}))
	defer redirector.Close()

	md := &mediafile.ParsedMetadata{CoverURL: redirector.URL + "/cover.jpg"}
	// Only allow the redirector's host:port, not the target's
	ok := downloadCoverFromURL(context.Background(), md, []string{testServerHost(redirector)}, testLogger())

	assert.False(t, ok, "should reject redirect to disallowed domain")
	assert.Nil(t, md.CoverData, "CoverData should remain nil")
}

func TestDownloadCoverFromURL_UnsupportedScheme(t *testing.T) {
	t.Parallel()

	md := &mediafile.ParsedMetadata{CoverURL: "ftp://example.com/cover.jpg"}
	ok := downloadCoverFromURL(context.Background(), md, []string{"example.com"}, testLogger())

	assert.False(t, ok, "should reject non-http/https scheme")
	assert.Nil(t, md.CoverData, "CoverData should remain nil")
}
