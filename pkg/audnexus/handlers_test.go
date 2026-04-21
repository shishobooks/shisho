package audnexus

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHandler(t *testing.T, upstreamStatus int, upstreamBody string) *handler {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(upstreamStatus)
		_, _ = io.WriteString(w, upstreamBody)
	}))
	t.Cleanup(upstream.Close)

	svc := NewService(ServiceConfig{BaseURL: upstream.URL})
	return &handler{service: svc}
}

func invokeHandler(t *testing.T, h *handler, asin string) (statusCode int, errOut error, body string) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/audnexus/books/"+asin+"/chapters", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("asin")
	c.SetParamValues(asin)

	err := h.getChapters(c)
	return rec.Code, err, rec.Body.String()
}

func TestHandler_GetChapters_Success(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, http.StatusOK, `{"asin":"B0036UC2LO","chapters":[{"title":"C1","startOffsetMs":0,"lengthMs":1000}]}`)
	status, err, body := invokeHandler(t, h, "B0036UC2LO")
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
	h := newTestHandler(t, http.StatusOK, `{}`)
	_, err, _ := invokeHandler(t, h, "short")
	require.Error(t, err)
	ec := asErrcodesError(t, err)
	assert.Equal(t, http.StatusBadRequest, ec.HTTPCode)
}

func TestHandler_GetChapters_NotFound(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, http.StatusNotFound, ``)
	_, err, _ := invokeHandler(t, h, "B0036UC2LO")
	require.Error(t, err)
	ec := asErrcodesError(t, err)
	assert.Equal(t, http.StatusNotFound, ec.HTTPCode)
}

func TestHandler_GetChapters_UpstreamError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, http.StatusInternalServerError, ``)
	_, err, _ := invokeHandler(t, h, "B0036UC2LO")
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
