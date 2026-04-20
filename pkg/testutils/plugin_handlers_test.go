package testutils

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixtureZipIsDeterministicAndMatchesInfo(t *testing.T) {
	t.Parallel()

	e := echo.New()
	h := &handler{} // db not needed for fixture endpoints
	e.GET("/test/plugins/fixture.zip", h.fixtureZip)
	e.GET("/test/plugins/fixture-info", h.fixtureInfo)

	// Fetch the zip
	req := httptest.NewRequest(http.MethodGet, "/test/plugins/fixture.zip", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/zip", rec.Header().Get("Content-Type"))

	zipBytes, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	require.NotEmpty(t, zipBytes)

	h1 := sha256.Sum256(zipBytes)

	// Fetch it again: bytes must be identical (deterministic build).
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/test/plugins/fixture.zip", nil))
	h2 := sha256.Sum256(rec2.Body.Bytes())
	assert.Equal(t, h1, h2, "fixture.zip must be deterministic across requests")

	// Fetch info and check the sha256 matches what we got from fixture.zip.
	rec3 := httptest.NewRecorder()
	e.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/test/plugins/fixture-info", nil))
	require.Equal(t, http.StatusOK, rec3.Code)

	var info struct {
		Scope       string `json:"scope"`
		ID          string `json:"id"`
		Version     string `json:"version"`
		DownloadURL string `json:"download_url"`
		SHA256      string `json:"sha256"`
	}
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &info))

	assert.Equal(t, "test", info.Scope)
	assert.Equal(t, "fixture", info.ID)
	assert.Equal(t, "1.0.0", info.Version)
	assert.Equal(t, hex.EncodeToString(h1[:]), info.SHA256)
}
