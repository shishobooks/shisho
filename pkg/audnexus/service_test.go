package audnexus

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundTripFunc adapts a function to an http.RoundTripper so tests can serve
// canned responses without a real socket. The live httptest servers this
// replaced could flake under CI load: when the localhost connection raced the
// response, http.Client.Do returned a connection error that the service maps to
// upstream_error, instead of the status the test set (observed on the 503 case
// of TestService_GetChapters_RateLimited).
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// stubService builds a Service whose HTTP client routes every request through
// rt instead of the network.
func stubService(cfg ServiceConfig, rt roundTripFunc) *Service {
	cfg.HTTPClient = &http.Client{Transport: rt}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://audnexus.test"
	}
	return NewService(cfg)
}

// respondWith returns a transport that serves the given status and body.
func respondWith(status int, body string) roundTripFunc {
	return func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	}
}

func TestService_GetChapters_InvalidASIN(t *testing.T) {
	t.Parallel()
	svc := NewService(ServiceConfig{})

	cases := []string{
		"",
		"short",
		"TOO-LONG-ASIN",
		"1234567890X!", // non-alphanumeric
		"B0036UC2L",    // 9 chars
		"B0036UC2LOX",  // 11 chars
	}
	for _, asin := range cases {
		asin := asin
		t.Run(asin, func(t *testing.T) {
			t.Parallel()
			_, err := svc.GetChapters(context.Background(), asin)
			require.Error(t, err)
			e := AsAudnexusError(err)
			require.NotNil(t, e, "expected *Error")
			assert.Equal(t, ErrCodeInvalidASIN, e.Code)
		})
	}
}

func TestService_GetChapters_NormalizesASINToUppercase(t *testing.T) {
	t.Parallel()
	// A valid lowercase ASIN must pass validation and reach upstream rather than
	// being rejected as invalid_asin. The transport asserts the ASIN arrived
	// normalized to uppercase in the request path.
	svc := stubService(ServiceConfig{}, func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "/books/B0036UC2LO/chapters", r.URL.Path)
		return respondWith(http.StatusOK, `{"asin":"B0036UC2LO","chapters":[]}`)(r)
	})
	_, err := svc.GetChapters(context.Background(), "b0036uc2lo")
	require.NoError(t, err)
}

func TestService_GetChapters_HappyPath(t *testing.T) {
	t.Parallel()

	svc := stubService(ServiceConfig{UserAgent: "Shisho/test"}, func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "/books/B0036UC2LO/chapters", r.URL.Path)
		assert.Equal(t, "Shisho/test", r.Header.Get("User-Agent"))
		return respondWith(http.StatusOK, `{
			"asin": "B0036UC2LO",
			"isAccurate": true,
			"runtimeLengthMs": 163938000,
			"brandIntroDurationMs": 38000,
			"brandOutroDurationMs": 62000,
			"chapters": [
				{"title": "Prelude", "startOffsetMs": 0, "lengthMs": 272000},
				{"title": "Prologue", "startOffsetMs": 272000, "lengthMs": 1063000}
			]
		}`)(r)
	})

	resp, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "B0036UC2LO", resp.ASIN)
	assert.True(t, resp.IsAccurate)
	assert.Equal(t, int64(163938000), resp.RuntimeLengthMs)
	assert.Equal(t, int64(38000), resp.BrandIntroDurationMs)
	assert.Equal(t, int64(62000), resp.BrandOutroDurationMs)
	require.Len(t, resp.Chapters, 2)
	assert.Equal(t, "Prelude", resp.Chapters[0].Title)
	assert.Equal(t, int64(0), resp.Chapters[0].StartOffsetMs)
	assert.Equal(t, int64(272000), resp.Chapters[0].LengthMs)
}

func TestService_GetChapters_NotFound(t *testing.T) {
	t.Parallel()
	svc := stubService(ServiceConfig{}, respondWith(http.StatusNotFound, ""))
	_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.Error(t, err)
	assert.Equal(t, ErrCodeNotFound, AsAudnexusError(err).Code)
}

func TestService_GetChapters_UpstreamError_5xx(t *testing.T) {
	t.Parallel()
	svc := stubService(ServiceConfig{}, respondWith(http.StatusInternalServerError, ""))
	_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.Error(t, err)
	assert.Equal(t, ErrCodeUpstreamError, AsAudnexusError(err).Code)
}

func TestService_GetChapters_RateLimited(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
	}{
		{"429_too_many_requests", http.StatusTooManyRequests},
		{"503_service_unavailable", http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := stubService(ServiceConfig{}, respondWith(tc.status, ""))
			_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
			require.Error(t, err)
			assert.Equal(t, ErrCodeRateLimited, AsAudnexusError(err).Code)
		})
	}
}

func TestService_GetChapters_InvalidJSON(t *testing.T) {
	t.Parallel()
	svc := stubService(ServiceConfig{}, respondWith(http.StatusOK, "not json"))
	_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.Error(t, err)
	assert.Equal(t, ErrCodeUpstreamError, AsAudnexusError(err).Code)
}

func TestService_GetChapters_Timeout(t *testing.T) {
	t.Parallel()
	// http.Client.Timeout surfaces as a context.DeadlineExceeded from the
	// transport; the service must classify that as a timeout rather than a
	// generic upstream error.
	svc := stubService(ServiceConfig{}, func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})
	_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.Error(t, err)
	assert.Equal(t, ErrCodeTimeout, AsAudnexusError(err).Code)
}

func TestService_GetChapters_CacheHit(t *testing.T) {
	t.Parallel()
	var hits int32
	svc := stubService(ServiceConfig{CacheTTL: time.Hour}, func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&hits, 1)
		return respondWith(http.StatusOK, `{"asin":"B0036UC2LO","chapters":[]}`)(r)
	})

	// First call hits upstream.
	_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.NoError(t, err)
	// Second call with same ASIN hits cache.
	_, err = svc.GetChapters(context.Background(), "B0036UC2LO")
	require.NoError(t, err)
	// Lowercase should normalize to same cache key.
	_, err = svc.GetChapters(context.Background(), "b0036uc2lo")
	require.NoError(t, err)

	assert.Equal(t, int32(1), atomic.LoadInt32(&hits), "expected exactly one upstream call")
}

func TestService_GetChapters_CacheExpires(t *testing.T) {
	t.Parallel()
	var hits int32
	svc := stubService(ServiceConfig{CacheTTL: 10 * time.Millisecond}, func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&hits, 1)
		return respondWith(http.StatusOK, `{"asin":"B0036UC2LO","chapters":[]}`)(r)
	})

	_, _ = svc.GetChapters(context.Background(), "B0036UC2LO")
	time.Sleep(20 * time.Millisecond)
	_, _ = svc.GetChapters(context.Background(), "B0036UC2LO")

	assert.Equal(t, int32(2), atomic.LoadInt32(&hits), "expected cache to expire and re-fetch")
}

func TestService_GetChapters_ErrorsNotCached(t *testing.T) {
	t.Parallel()
	var hits int32
	svc := stubService(ServiceConfig{CacheTTL: time.Hour}, func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&hits, 1)
		return respondWith(http.StatusInternalServerError, "")(r)
	})

	_, _ = svc.GetChapters(context.Background(), "B0036UC2LO")
	_, _ = svc.GetChapters(context.Background(), "B0036UC2LO")

	assert.Equal(t, int32(2), atomic.LoadInt32(&hits), "errors must not be cached")
}
