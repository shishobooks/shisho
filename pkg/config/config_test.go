package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_RequiredFieldMissing(t *testing.T) {
	t.Setenv("DATABASE_FILE_PATH", "")
	t.Setenv("CONFIG_FILE", "/nonexistent/config.yaml")

	cfg, err := New()
	assert.Nil(t, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required config")
	assert.Contains(t, err.Error(), "DATABASE_FILE_PATH")
	assert.Contains(t, err.Error(), "database_file_path")
}

func TestNew_WithEnvVar(t *testing.T) {
	t.Setenv("DATABASE_FILE_PATH", "/tmp/test.db")
	t.Setenv("JWT_SECRET", "test-secret-key")
	t.Setenv("CONFIG_FILE", "/nonexistent/config.yaml")

	cfg, err := New()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/test.db", cfg.DatabaseFilePath)
}

func TestNew_WithConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
database_file_path: /data/shisho.db
server_port: 8080
database_debug: true
jwt_secret: test-secret-from-file
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	t.Setenv("CONFIG_FILE", configPath)
	// Note: We don't set SHISHO_DATABASE_FILE_PATH so file value is used

	cfg, err := New()
	require.NoError(t, err)
	assert.Equal(t, "/data/shisho.db", cfg.DatabaseFilePath)
	assert.Equal(t, 8080, cfg.ServerPort)
	assert.True(t, cfg.DatabaseDebug)
}

func TestNew_EnvVarOverridesConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
database_file_path: /data/from-file.db
server_port: 8080
jwt_secret: test-secret-from-file
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	t.Setenv("CONFIG_FILE", configPath)
	t.Setenv("DATABASE_FILE_PATH", "/data/from-env.db")
	t.Setenv("SERVER_PORT", "9090")

	cfg, err := New()
	require.NoError(t, err)
	// Env vars should override config file
	assert.Equal(t, "/data/from-env.db", cfg.DatabaseFilePath)
	assert.Equal(t, 9090, cfg.ServerPort)
}

func TestNew_Defaults(t *testing.T) {
	t.Setenv("DATABASE_FILE_PATH", "/tmp/test.db")
	t.Setenv("JWT_SECRET", "test-secret-key")
	t.Setenv("CONFIG_FILE", "/nonexistent/config.yaml")

	cfg, err := New()
	require.NoError(t, err)

	// Check defaults are applied
	assert.Equal(t, 5, cfg.DatabaseConnectRetryCount)
	assert.Equal(t, 2*time.Second, cfg.DatabaseConnectRetryDelay)
	assert.False(t, cfg.DatabaseDebug)
	assert.Equal(t, "0.0.0.0", cfg.ServerHost)
	assert.Equal(t, 3689, cfg.ServerPort)
	assert.Equal(t, 60, cfg.SyncIntervalMinutes)
	assert.Equal(t, 2, cfg.WorkerProcesses)
}

func TestNew_SyncInterval(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
database_file_path: /data/shisho.db
sync_interval_minutes: 30
jwt_secret: test-secret-key
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	t.Setenv("CONFIG_FILE", configPath)

	cfg, err := New()
	require.NoError(t, err)
	assert.Equal(t, 30, cfg.SyncIntervalMinutes)
}

func TestNew_SyncIntervalFromEnv(t *testing.T) {
	t.Setenv("DATABASE_FILE_PATH", "/tmp/test.db")
	t.Setenv("JWT_SECRET", "test-secret-key")
	t.Setenv("SYNC_INTERVAL_MINUTES", "15")
	t.Setenv("CONFIG_FILE", "/nonexistent/config.yaml")

	cfg, err := New()
	require.NoError(t, err)
	assert.Equal(t, 15, cfg.SyncIntervalMinutes)
}

func TestNewForTest(t *testing.T) {
	cfg := NewForTest()
	assert.Equal(t, ":memory:", cfg.DatabaseFilePath)
	assert.Equal(t, "127.0.0.1", cfg.ServerHost)
	assert.Equal(t, 60, cfg.SyncIntervalMinutes)
}

func TestToSnakeCase(t *testing.T) {
	assert.Equal(t, "database_file_path", toSnakeCase("DatabaseFilePath"))
	assert.Equal(t, "server_port", toSnakeCase("ServerPort"))
	assert.Equal(t, "sync_interval_minutes", toSnakeCase("SyncIntervalMinutes"))
}
