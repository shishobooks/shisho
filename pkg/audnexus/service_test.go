package audnexus

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	svc := NewService(ServiceConfig{BaseURL: upstream.URL})
	_, err := svc.GetChapters(context.Background(), "B0036UC2LO")
	require.Error(t, err)
	assert.Equal(t, ErrCodeUpstreamError, AsAudnexusError(err).Code)
}

func TestService_GetChapters_InvalidJSON(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
