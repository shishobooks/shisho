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

func newTestHandler() (*Handler, *fakeCache, *fakeCache, *fakeCache) {
	dl := &fakeCache{bytes: 100, count: 2}
	cbz := &fakeCache{bytes: 50, count: 5}
	pdf := &fakeCache{bytes: 25, count: 1}
	h := NewHandler(dl, cbz, pdf)
	return h, dl, cbz, pdf
}

func TestList_ReturnsAllThreeCaches(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestHandler()

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
		id        string
		expectDL  int
		expectCBZ int
		expectPDF int
	}{
		{"downloads", 1, 0, 0},
		{"cbz_pages", 0, 1, 0},
		{"pdf_pages", 0, 0, 1},
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
			assert.NotEmpty(t, resp)
		})
	}
}

func TestClear_UnknownIDReturns404(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestHandler()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/cache/unknown/clear", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("unknown")

	err := h.clear(c)
	require.Error(t, err)
	var ce *errcodes.Error
	require.True(t, errors.As(err, &ce), "expected *errcodes.Error, got %T", err)
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
