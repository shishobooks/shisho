package audnexus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	// Valid lowercase ASIN should pass validation (and would reach upstream,
	// which we're not testing here — but it must not return invalid_asin).
	svc := NewService(ServiceConfig{})
	_, err := svc.GetChapters(context.Background(), "b0036uc2lo")
	if e := AsAudnexusError(err); e != nil {
		assert.NotEqual(t, ErrCodeInvalidASIN, e.Code, "lowercase ASIN should normalize to valid")
	}
}

func TestService_GetChapters_HappyPath(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/books/B0036UC2LO/chapters", r.URL.Path)
		assert.Equal(t, "Shisho/test", r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"asin": "B0036UC2LO",
			"isAccurate": true,
			"runtimeLengthMs": 163938000,
			"brandIntroDurationMs": 38000,
			"brandOutroDurationMs": 62000,
			"chapters": [
				{"title": "Prelude", "startOffsetMs": 0, "lengthMs": 272000},
				{"title": "Prologue", "startOffsetMs": 272000, "lengthMs": 1063000}
			]
		}`))
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{
		BaseURL:   upstream.URL,
		UserAgent: "Shisho/test",
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
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{BaseURL: upstream.URL})
	_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.Error(t, err)
	assert.Equal(t, ErrCodeNotFound, AsAudnexusError(err).Code)
}

func TestService_GetChapters_UpstreamError_5xx(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{BaseURL: upstream.URL})
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
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer upstream.Close()

			svc := NewService(ServiceConfig{BaseURL: upstream.URL})
			_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
			require.Error(t, err)
			assert.Equal(t, ErrCodeRateLimited, AsAudnexusError(err).Code)
		})
	}
}

func TestService_GetChapters_InvalidJSON(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{BaseURL: upstream.URL})
	_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.Error(t, err)
	assert.Equal(t, ErrCodeUpstreamError, AsAudnexusError(err).Code)
}

func TestService_GetChapters_Timeout(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{
		BaseURL:    upstream.URL,
		HTTPClient: &http.Client{Timeout: 50 * time.Millisecond},
	})
	_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.Error(t, err)
	assert.Equal(t, ErrCodeTimeout, AsAudnexusError(err).Code)
}

func TestService_GetChapters_CacheHit(t *testing.T) {
	t.Parallel()
	var hits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"asin":"B0036UC2LO","chapters":[]}`))
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{BaseURL: upstream.URL, CacheTTL: time.Hour})

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
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"asin":"B0036UC2LO","chapters":[]}`))
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{BaseURL: upstream.URL, CacheTTL: 10 * time.Millisecond})

	_, _ = svc.GetChapters(context.Background(), "B0036UC2LO")
	time.Sleep(20 * time.Millisecond)
	_, _ = svc.GetChapters(context.Background(), "B0036UC2LO")

	assert.Equal(t, int32(2), atomic.LoadInt32(&hits), "expected cache to expire and re-fetch")
}

func TestService_GetChapters_ErrorsNotCached(t *testing.T) {
	t.Parallel()
	var hits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{BaseURL: upstream.URL, CacheTTL: time.Hour})

	_, _ = svc.GetChapters(context.Background(), "B0036UC2LO")
	_, _ = svc.GetChapters(context.Background(), "B0036UC2LO")

	assert.Equal(t, int32(2), atomic.LoadInt32(&hits), "errors must not be cached")
}
