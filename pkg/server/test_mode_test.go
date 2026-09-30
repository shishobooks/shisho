package server

import (
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNew_TestModeKey loads the config from the environment, as the binary
// does, so it covers the key name and not just the Config field. It sets
// environment variables, so it cannot run in parallel.
func TestNew_TestModeKey(t *testing.T) {
	originalHosts := plugins.AllowedDownloadHosts
	t.Cleanup(func() { plugins.AllowedDownloadHosts = originalHosts })

	hasTestRoutes := func(t *testing.T) bool {
		t.Helper()
		cfg, err := config.New()
		require.NoError(t, err)
		cfg.CacheDir = t.TempDir()
		srv, err := New(cfg, nil, &worker.Worker{}, nil, nil, nil, nil, nil, nil, nil)
		require.NoError(t, err)
		for _, route := range srv.Handler.(*echo.Echo).Routes() {
			if strings.HasPrefix(route.Path, "/api/test/") {
				return true
			}
		}
		return false
	}

	t.Setenv("CONFIG_FILE", "/nonexistent/config.yaml")
	t.Setenv("DATABASE_FILE_PATH", "/tmp/test.db")
	t.Setenv("JWT_SECRET", "server-test-secret-at-least-32-characters")

	t.Run("generic ENVIRONMENT=test does not mount test routes", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "test")
		assert.False(t, hasTestRoutes(t))
	})

	t.Run("SHISHO_TEST_MODE mounts test routes", func(t *testing.T) {
		t.Setenv("SHISHO_TEST_MODE", "true")
		assert.True(t, hasTestRoutes(t))
	})
}
