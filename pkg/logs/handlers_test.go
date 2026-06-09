package logs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListLogs_ResponseUsesItemsTotalEnvelope asserts GET /logs returns the
// standard { items, total } envelope (not the legacy { entries } shape).
func TestListLogs_ResponseUsesItemsTotalEnvelope(t *testing.T) {
	t.Parallel()

	buf := NewRingBuffer(10, nil)
	// Feed two zerolog-style JSON lines into the buffer.
	_, err := buf.Write([]byte(`{"level":"info","message":"hello","timestamp":"2026-06-08T00:00:00Z"}` + "\n" +
		`{"level":"warn","message":"careful","timestamp":"2026-06-08T00:00:01Z"}` + "\n"))
	require.NoError(t, err)

	h := &handler{buffer: buf}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/logs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.listLogs(c))
	require.Equal(t, http.StatusOK, rec.Code)

	// Top-level envelope must be { items, total } only.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	_, hasItems := raw["items"]
	_, hasTotal := raw["total"]
	_, hasEntries := raw["entries"]
	assert.True(t, hasItems, "response must have 'items' key")
	assert.True(t, hasTotal, "response must have 'total' key")
	assert.False(t, hasEntries, "response must NOT use legacy 'entries' key")
	assert.Len(t, raw, 2, "response must have exactly 'items' and 'total' keys")

	var resp struct {
		Items []struct {
			Level   string `json:"level"`
			Message string `json:"message"`
		} `json:"items"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 2)
	assert.Equal(t, 2, resp.Total)
	assert.Equal(t, "hello", resp.Items[0].Message)
}
