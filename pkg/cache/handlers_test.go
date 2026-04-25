package cache

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCache struct {
	bytes       int64
	count       int
	sizeErr     error
	clearErr    error
	clearCalled int
}

func (f *fakeCache) SizeBytes() (int64, int, error) {
	return f.bytes, f.count, f.sizeErr
}

func (f *fakeCache) Clear() error {
	f.clearCalled++
	return f.clearErr
}

func newTestFakes() (*fakeCache, *fakeCache, *fakeCache) {
	return &fakeCache{bytes: 100, count: 2},
		&fakeCache{bytes: 50, count: 5},
		&fakeCache{bytes: 25, count: 1}
}

func newTestHandler() (*Handler, *fakeCache, *fakeCache, *fakeCache) {
	dl, cbz, pdf := newTestFakes()
	return NewHandler(dl, cbz, pdf), dl, cbz, pdf
}

func newTestHandlerOnly() *Handler {
	dl, cbz, pdf := newTestFakes()
	return NewHandler(dl, cbz, pdf)
}

func TestList_ReturnsAllThreeCaches(t *testing.T) {
	t.Parallel()
	h := newTestHandlerOnly()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/cache", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.list(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Caches, 3)

	ids := []string{resp.Caches[0].ID, resp.Caches[1].ID, resp.Caches[2].ID}
	assert.Contains(t, ids, "downloads")
	assert.Contains(t, ids, "cbz_pages")
	assert.Contains(t, ids, "pdf_pages")

	for _, ci := range resp.Caches {
		switch ci.ID {
		case "downloads":
			assert.Equal(t, int64(100), ci.SizeBytes)
			assert.Equal(t, 2, ci.FileCount)
		case "cbz_pages":
			assert.Equal(t, int64(50), ci.SizeBytes)
			assert.Equal(t, 5, ci.FileCount)
		case "pdf_pages":
			assert.Equal(t, int64(25), ci.SizeBytes)
			assert.Equal(t, 1, ci.FileCount)
		}
	}
}

func TestClear_DispatchesByID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id          string
		expectDL    int
		expectCBZ   int
		expectPDF   int
		expectBytes int64
		expectFiles int
	}{
		{"downloads", 1, 0, 0, 100, 2},
		{"cbz_pages", 0, 1, 0, 50, 5},
		{"pdf_pages", 0, 0, 1, 25, 1},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			h, dl, cbz, pdf := newTestHandler()

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/cache/"+tc.id+"/clear", strings.NewReader(""))
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues(tc.id)

			require.NoError(t, h.clear(c))
			assert.Equal(t, http.StatusOK, rec.Code)

			assert.Equal(t, tc.expectDL, dl.clearCalled)
			assert.Equal(t, tc.expectCBZ, cbz.clearCalled)
			assert.Equal(t, tc.expectPDF, pdf.clearCalled)

			var resp ClearResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tc.expectBytes, resp.ClearedBytes)
			assert.Equal(t, tc.expectFiles, resp.ClearedFiles)
		})
	}
}

func TestClear_UnknownIDReturns404(t *testing.T) {
	t.Parallel()
	h := newTestHandlerOnly()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/cache/unknown/clear", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("unknown")

	err := h.clear(c)
	require.Error(t, err)
	var ce *errcodes.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, http.StatusNotFound, ce.HTTPCode)
}

func TestClear_ReportsPreClearSize(t *testing.T) {
	t.Parallel()
	h, dl, _, _ := newTestHandler()
	dl.bytes = 500
	dl.count = 7

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/cache/downloads/clear", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("downloads")

	require.NoError(t, h.clear(c))

	var resp ClearResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, int64(500), resp.ClearedBytes)
	assert.Equal(t, 7, resp.ClearedFiles)
}

func TestList_ReturnsErrorWhenSizeBytesFails(t *testing.T) {
	t.Parallel()
	h, dl, _, _ := newTestHandler()
	dl.sizeErr = errors.New("disk read failed")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/cache", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.list(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk read failed")
}

func TestClear_ReturnsErrorWhenClearFails(t *testing.T) {
	t.Parallel()
	h, dl, _, _ := newTestHandler()
	dl.clearErr = errors.New("permission denied")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/cache/downloads/clear", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("downloads")

	err := h.clear(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	assert.Equal(t, 1, dl.clearCalled)
}

func TestClear_ReturnsErrorWhenPreClearSizeFails(t *testing.T) {
	t.Parallel()
	h, dl, _, _ := newTestHandler()
	dl.sizeErr = errors.New("stat failed")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/cache/downloads/clear", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("downloads")

	err := h.clear(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat failed")
	assert.Equal(t, 0, dl.clearCalled, "Clear should not be called if SizeBytes fails")
}
