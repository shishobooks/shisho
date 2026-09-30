package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func retrieveConfigJSON(t *testing.T, cfg *Config) map[string]any {
	t.Helper()
	h := &handler{configService: NewService(cfg)}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/config", nil), rec)
	require.NoError(t, h.retrieve(c))
	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

func TestRetrieveConfig_ReportsEffectiveMonitorDelay(t *testing.T) {
	cfg := NewForTest()
	cfg.LibraryMonitorDelaySeconds = 2

	body := retrieveConfigJSON(t, cfg)
	assert.InDelta(t, 2, body["library_monitor_delay_seconds"], 0)
	assert.InDelta(t, MinLibraryMonitorDelaySeconds, body["library_monitor_effective_delay_seconds"], 0)

	cfg.LibraryMonitorDelaySeconds = 90
	body = retrieveConfigJSON(t, cfg)
	assert.InDelta(t, 90, body["library_monitor_effective_delay_seconds"], 0)
}

func TestRetrieveConfig_NeverIncludesJWTSecret(t *testing.T) {
	cfg := NewForTest()
	body := retrieveConfigJSON(t, cfg)
	assert.NotContains(t, body, "jwt_secret")
	for _, v := range body {
		assert.NotEqual(t, cfg.JWTSecret, v)
	}
	assert.Equal(t, false, body["test_mode"])
}

func TestRetrieveConfig_ReportsShortJWTSecret(t *testing.T) {
	cfg := NewForTest()
	body := retrieveConfigJSON(t, cfg)
	assert.Equal(t, false, body["jwt_secret_too_short"])

	cfg.JWTSecret = "short-secret"
	body = retrieveConfigJSON(t, cfg)
	assert.Equal(t, true, body["jwt_secret_too_short"])
	for _, v := range body {
		assert.NotEqual(t, cfg.JWTSecret, v)
	}
}
