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
	"github.com/stretchr/testify/require"
)

// fakeEnricherRuntime is a minimal enricherRuntime for testing the search
// aggregation loop without a real Goja VM.
type fakeEnricherRuntime struct {
	scope    string
	id       string
	manifest *Manifest
}

func (f *fakeEnricherRuntime) Scope() string       { return f.scope }
func (f *fakeEnricherRuntime) PluginID() string    { return f.id }
func (f *fakeEnricherRuntime) Manifest() *Manifest { return f.manifest }

func newFakeEnricher(id string, fileTypes []string) *fakeEnricherRuntime {
	return &fakeEnricherRuntime{
		scope: "shisho",
		id:    id,
		manifest: &Manifest{
			Name: id,
			Capabilities: Capabilities{
				MetadataEnricher: &MetadataEnricherCap{FileTypes: fileTypes},
			},
		},
	}
}

func TestAggregateEnricherSearches_StopsOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	runtimes := []*fakeEnricherRuntime{
		newFakeEnricher("first", []string{"epub"}),
		newFakeEnricher("second", []string{"epub"}),
		newFakeEnricher("third", []string{"epub"}),
	}

	var invoked []string
	runSearch := func(_ context.Context, rt *fakeEnricherRuntime) (*SearchResponse, error) {
		invoked = append(invoked, rt.PluginID())
		// The client disconnects after the first plugin runs. The aggregator
		// must notice the cancellation and stop before invoking the rest.
		if rt.PluginID() == "first" {
			cancel()
		}
		return &SearchResponse{Results: []mediafile.ParsedMetadata{{Title: rt.PluginID()}}}, nil
	}
	disabled := func(_ context.Context, _ *fakeEnricherRuntime) []string { return nil }

	results, pluginErrors, _ := aggregateEnricherSearches(ctx, runtimes, "epub", runSearch, disabled, testLogger())

	assert.Equal(t, []string{"first"}, invoked, "should stop invoking plugins once the context is cancelled")
	require.Len(t, results, 1)
	assert.Equal(t, "first", results[0].Title)
	assert.Empty(t, pluginErrors, "client cancellation must not be reported as a plugin error")
}

func TestAggregateEnricherSearches_CancellationNotReportedAsError(t *testing.T) {
	t.Parallel()

	runtimes := []*fakeEnricherRuntime{
		newFakeEnricher("cancelled", []string{"epub"}),
	}
	runSearch := func(_ context.Context, _ *fakeEnricherRuntime) (*SearchResponse, error) {
		// A per-plugin search that fails specifically due to client
		// cancellation must not be surfaced as a plugin failure.
		return nil, context.Canceled
	}
	disabled := func(_ context.Context, _ *fakeEnricherRuntime) []string { return nil }

	results, pluginErrors, _ := aggregateEnricherSearches(context.Background(), runtimes, "epub", runSearch, disabled, testLogger())

	assert.Empty(t, results)
	assert.Empty(t, pluginErrors, "context.Canceled must not be added to plugin errors")
}

func TestAggregateEnricherSearches_TimeoutStillReported(t *testing.T) {
	t.Parallel()

	runtimes := []*fakeEnricherRuntime{
		newFakeEnricher("timed-out", []string{"epub"}),
	}
	runSearch := func(_ context.Context, _ *fakeEnricherRuntime) (*SearchResponse, error) {
		// A genuine per-plugin timeout (deadline exceeded) is a real failure
		// and must still be reported to the user.
		return nil, context.DeadlineExceeded
	}
	disabled := func(_ context.Context, _ *fakeEnricherRuntime) []string { return nil }

	results, pluginErrors, _ := aggregateEnricherSearches(context.Background(), runtimes, "epub", runSearch, disabled, testLogger())

	assert.Empty(t, results)
	require.Len(t, pluginErrors, 1, "per-plugin timeout must still be reported")
	assert.Equal(t, "timed-out", pluginErrors[0].PluginID)
}

func TestAggregateEnricherSearches_SkipsUnsupportedFileType(t *testing.T) {
	t.Parallel()

	runtimes := []*fakeEnricherRuntime{
		newFakeEnricher("epub-only", []string{"epub"}),
		newFakeEnricher("cbz-only", []string{"cbz"}),
	}
	var invoked []string
	runSearch := func(_ context.Context, rt *fakeEnricherRuntime) (*SearchResponse, error) {
		invoked = append(invoked, rt.PluginID())
		return &SearchResponse{Results: []mediafile.ParsedMetadata{{Title: rt.PluginID()}}}, nil
	}
	disabled := func(_ context.Context, _ *fakeEnricherRuntime) []string { return nil }

	results, _, skipped := aggregateEnricherSearches(context.Background(), runtimes, "epub", runSearch, disabled, testLogger())

	assert.Equal(t, []string{"epub-only"}, invoked)
	require.Len(t, results, 1)
	assert.Equal(t, "epub-only", results[0].Title)
	require.Len(t, skipped, 1)
	assert.Equal(t, "cbz-only", skipped[0].PluginID)
}

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
	ok := DownloadCoverFromURL(context.Background(), md, []string{testServerHost(srv)}, testLogger())

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

	ok := DownloadCoverFromURL(context.Background(), md, []string{"example.com"}, testLogger())

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
	ok := DownloadCoverFromURL(context.Background(), md, []string{testServerHost(srv)}, testLogger())

	assert.False(t, ok, "should reject non-image content type")
	assert.Nil(t, md.CoverData, "CoverData should remain nil")
}

func TestDownloadCoverFromURL_EmptyURL(t *testing.T) {
	t.Parallel()

	md := &mediafile.ParsedMetadata{}
	ok := DownloadCoverFromURL(context.Background(), md, nil, testLogger())

	assert.False(t, ok, "should return false for empty URL")
}

func TestDownloadCoverFromURL_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	md := &mediafile.ParsedMetadata{CoverURL: srv.URL + "/error"}
	ok := DownloadCoverFromURL(context.Background(), md, []string{testServerHost(srv)}, testLogger())

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
	ok := DownloadCoverFromURL(context.Background(), md, []string{"example.com"}, testLogger())

	assert.False(t, ok, "should reject URL with domain not in allowed list")
	assert.Nil(t, md.CoverData, "CoverData should remain nil")
}

func TestDownloadCoverFromURL_NoHTTPAccess(t *testing.T) {
	t.Parallel()

	md := &mediafile.ParsedMetadata{CoverURL: "https://example.com/cover.jpg"}
	ok := DownloadCoverFromURL(context.Background(), md, nil, testLogger())

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
	ok := DownloadCoverFromURL(context.Background(), md, []string{testServerHost(redirector)}, testLogger())

	assert.False(t, ok, "should reject redirect to disallowed domain")
	assert.Nil(t, md.CoverData, "CoverData should remain nil")
}

func TestDownloadCoverFromURL_UnsupportedScheme(t *testing.T) {
	t.Parallel()

	md := &mediafile.ParsedMetadata{CoverURL: "ftp://example.com/cover.jpg"}
	ok := DownloadCoverFromURL(context.Background(), md, []string{"example.com"}, testLogger())

	assert.False(t, ok, "should reject non-http/https scheme")
	assert.Nil(t, md.CoverData, "CoverData should remain nil")
}
