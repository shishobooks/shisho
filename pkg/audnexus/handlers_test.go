package audnexus

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestHandler builds a handler whose service serves the given canned upstream
// response through a stubbed transport (see stubService), avoiding a real socket
// that can flake under CI load.
func newTestHandler(upstreamStatus int, upstreamBody string) *handler {
	svc := stubService(ServiceConfig{}, respondWith(upstreamStatus, upstreamBody))
	return &handler{service: svc}
}

func invokeHandler(t *testing.T, h *handler, asin string) (statusCode int, body string, errOut error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/audnexus/books/"+asin+"/chapters", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("asin")
	c.SetParamValues(asin)

	err := h.getChapters(c)
	return rec.Code, rec.Body.String(), err
}

func TestHandler_GetChapters_Success(t *testing.T) {
	t.Parallel()
	h := newTestHandler(http.StatusOK, `{"asin":"B0036UC2LO","chapters":[{"title":"C1","startOffsetMs":0,"lengthMs":1000}]}`)
	status, body, err := invokeHandler(t, h, "B0036UC2LO")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	var resp Response
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	assert.Equal(t, "B0036UC2LO", resp.ASIN)
	require.Len(t, resp.Chapters, 1)
	assert.Equal(t, "C1", resp.Chapters[0].Title)
}

func TestHandler_GetChapters_InvalidASIN(t *testing.T) {
	t.Parallel()
	h := newTestHandler(http.StatusOK, `{}`)
	_, _, err := invokeHandler(t, h, "short")
	require.Error(t, err)
	ec := asErrcodesError(t, err)
	assert.Equal(t, http.StatusBadRequest, ec.HTTPCode)
}

func TestHandler_GetChapters_NotFound(t *testing.T) {
	t.Parallel()
	h := newTestHandler(http.StatusNotFound, ``)
	_, _, err := invokeHandler(t, h, "B0036UC2LO")
	require.Error(t, err)
	ec := asErrcodesError(t, err)
	assert.Equal(t, http.StatusNotFound, ec.HTTPCode)
}

func TestHandler_GetChapters_UpstreamError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(http.StatusInternalServerError, ``)
	_, _, err := invokeHandler(t, h, "B0036UC2LO")
	require.Error(t, err)
	ec := asErrcodesError(t, err)
	assert.Equal(t, http.StatusBadGateway, ec.HTTPCode)
}

// asErrcodesError extracts an *errcodes.Error from err via errors.As, failing
// the test if absent.
func asErrcodesError(t *testing.T, err error) *errcodes.Error {
	t.Helper()
	var ec *errcodes.Error
	if !errors.As(err, &ec) {
		t.Fatalf("expected *errcodes.Error, got %T: %v", err, err)
	}
	return ec
}

// The frontend switches on errcodes.Error.Code. Confirm each service error
// preserves its audnexus-specific slug end-to-end, not a generic HTTP family
// code.
func TestMapServiceError_PreservesAudnexusCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		err      error
		wantCode string
		wantHTTP int
	}{
		{"invalid_asin", newErr(ErrCodeInvalidASIN, "x"), "invalid_asin", http.StatusBadRequest},
		{"not_found", newErr(ErrCodeNotFound, "x"), "not_found", http.StatusNotFound},
		{"timeout", newErr(ErrCodeTimeout, "x"), "timeout", http.StatusGatewayTimeout},
		{"upstream_error", newErr(ErrCodeUpstreamError, "x"), "upstream_error", http.StatusBadGateway},
		{"rate_limited", newErr(ErrCodeRateLimited, "x"), "rate_limited", http.StatusTooManyRequests},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mapped := mapServiceError(tc.err)
			ec := asErrcodesError(t, mapped)
			assert.Equal(t, tc.wantCode, ec.Code, "Code")
			assert.Equal(t, tc.wantHTTP, ec.HTTPCode, "HTTPCode")
		})
	}
}
