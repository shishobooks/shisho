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
	t.Setenv("JWT_SECRET", testJWTSecret)
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
jwt_secret: config-test-secret-from-file-at-least-32-chars
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
jwt_secret: config-test-secret-from-file-at-least-32-chars
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
	t.Setenv("JWT_SECRET", testJWTSecret)
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
	assert.True(t, cfg.LibraryMonitorEnabled)
	assert.Equal(t, 60, cfg.LibraryMonitorDelaySeconds)
	assert.Equal(t, 200, cfg.PDFRenderDPI)
	assert.Equal(t, 85, cfg.PDFRenderQuality)
}

func TestNew_PDFRenderDPI_Validation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// DPI below minimum (72)
	configContent := `
database_file_path: /data/shisho.db
jwt_secret: config-test-secret-at-least-32-characters
pdf_render_dpi: 10
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	t.Setenv("CONFIG_FILE", configPath)

	_, err = New()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pdf_render_dpi")
}

func TestNew_PDFRenderQuality_Validation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Quality above maximum (100)
	configContent := `
database_file_path: /data/shisho.db
jwt_secret: config-test-secret-at-least-32-characters
pdf_render_quality: 200
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	t.Setenv("CONFIG_FILE", configPath)

	_, err = New()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pdf_render_quality")
}

func TestNew_SyncInterval(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
database_file_path: /data/shisho.db
sync_interval_minutes: 30
jwt_secret: config-test-secret-at-least-32-characters
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
	t.Setenv("JWT_SECRET", testJWTSecret)
	t.Setenv("SYNC_INTERVAL_MINUTES", "15")
	t.Setenv("CONFIG_FILE", "/nonexistent/config.yaml")

	cfg, err := New()
	require.NoError(t, err)
	assert.Equal(t, 15, cfg.SyncIntervalMinutes)
}

func TestNew_LibraryMonitorDefaultWithPartialConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Config file that omits library_monitor_enabled (like shisho.dev.yaml).
	configContent := `
database_file_path: /data/shisho.db
database_debug: true
jwt_secret: config-test-secret-at-least-32-characters
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	t.Setenv("CONFIG_FILE", configPath)

	cfg, err := New()
	require.NoError(t, err)

	// LibraryMonitorEnabled should still be true from defaults.
	assert.True(t, cfg.LibraryMonitorEnabled)
	assert.Equal(t, 60, cfg.LibraryMonitorDelaySeconds)
}

func TestNewForTest(t *testing.T) {
	cfg := NewForTest()
	assert.Equal(t, ":memory:", cfg.DatabaseFilePath)
	assert.Equal(t, "127.0.0.1", cfg.ServerHost)
	assert.Equal(t, 60, cfg.SyncIntervalMinutes)
}

// testJWTSecret is long enough to pass the jwt_secret length floor.
const testJWTSecret = "config-test-secret-at-least-32-characters"

// loadFromYAML writes content to a temp config file and loads it with New.
// Env vars that would override the file are cleared by the caller's
// t.Setenv calls, since tests in this package never run in parallel.
func loadFromYAML(t *testing.T, content string) (*Config, error) {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))
	t.Setenv("CONFIG_FILE", configPath)
	return New()
}

func TestNew_ListEnvVarsSplitOnCommas(t *testing.T) {
	t.Setenv("DATABASE_FILE_PATH", "/tmp/test.db")
	t.Setenv("JWT_SECRET", testJWTSecret)
	t.Setenv("CONFIG_FILE", "/nonexistent/config.yaml")
	t.Setenv("SUPPLEMENT_EXCLUDE_PATTERNS", "a,b")
	t.Setenv("PDF_SUPPLEMENT_FILENAMES", "bonus, liner notes ,")

	cfg, err := New()
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, cfg.SupplementExcludePatterns)
	assert.Equal(t, []string{"bonus", "liner notes"}, cfg.PDFSupplementFilenames)
}

func TestNew_EmptyYAMLListClearsDefault(t *testing.T) {
	cfg, err := loadFromYAML(t, `
database_file_path: /data/shisho.db
jwt_secret: `+testJWTSecret+`
pdf_supplement_filenames: []
supplement_exclude_patterns: []
`)
	require.NoError(t, err)
	assert.Empty(t, cfg.PDFSupplementFilenames)
	assert.Empty(t, cfg.SupplementExcludePatterns)
}

func TestNew_ShortYAMLListReplacesDefault(t *testing.T) {
	cfg, err := loadFromYAML(t, `
database_file_path: /data/shisho.db
jwt_secret: `+testJWTSecret+`
supplement_exclude_patterns: ["*.nfo"]
`)
	require.NoError(t, err)
	assert.Equal(t, []string{"*.nfo"}, cfg.SupplementExcludePatterns)
}

func TestNew_ValidationNamesKeyAndRange(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantKey string
		wantMsg string
	}{
		{"worker_processes zero", "worker_processes: 0", "worker_processes", "at least 1"},
		{"worker_processes negative", "worker_processes: -1", "worker_processes", "at least 1"},
		{"database_max_retries negative", "database_max_retries: -1", "database_max_retries", "at least 0"},
		{"database_connect_retry_count negative", "database_connect_retry_count: -1", "database_connect_retry_count", "at least 0"},
		{"threshold above one", "enrichment_confidence_threshold: 1.5", "enrichment_confidence_threshold", "between 0 and 1"},
		{"threshold below zero", "enrichment_confidence_threshold: -0.1", "enrichment_confidence_threshold", "between 0 and 1"},
		{"bare number busy timeout", "database_busy_timeout: 5", "database_busy_timeout", "at least 1ms"},
		{"sub-millisecond busy timeout", "database_busy_timeout: 500us", "database_busy_timeout", "at least 1ms"},
		{"sub-millisecond retry delay", "database_connect_retry_delay: 10ns", "database_connect_retry_delay", "at least 1ms"},
		{"pdf dpi range", "pdf_render_dpi: 10", "pdf_render_dpi", "between 72 and 600"},
		{"session duration", "session_duration_days: 0", "session_duration_days", "at least 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadFromYAML(t, "database_file_path: /data/shisho.db\njwt_secret: "+testJWTSecret+"\n"+tt.yaml+"\n")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantKey)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestNew_DurationFloorAcceptsMilliseconds(t *testing.T) {
	cfg, err := loadFromYAML(t, `
database_file_path: /data/shisho.db
jwt_secret: `+testJWTSecret+`
database_busy_timeout: 1ms
database_connect_retry_delay: 250ms
worker_processes: 1
database_max_retries: 0
database_connect_retry_count: 0
enrichment_confidence_threshold: 1
`)
	require.NoError(t, err)
	assert.Equal(t, time.Millisecond, cfg.DatabaseBusyTimeout)
	assert.Equal(t, 250*time.Millisecond, cfg.DatabaseConnectRetryDelay)
}

func TestNew_RejectsPlaceholderJWTSecret(t *testing.T) {
	t.Setenv("DATABASE_FILE_PATH", "/tmp/test.db")
	t.Setenv("CONFIG_FILE", "/nonexistent/config.yaml")
	t.Setenv("JWT_SECRET", "your-secret-key-here-change-me")

	cfg, err := New()
	assert.Nil(t, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jwt_secret")
	assert.Contains(t, err.Error(), "openssl rand -hex 32")
	assert.NotContains(t, err.Error(), "your-secret-key-here-change-me", "the error must not echo the secret")
}

// A short secret still starts, so an upgrade does not restart-loop, but it
// produces a startup warning that names the key and the fix.
func TestNew_WarnsAboutShortJWTSecret(t *testing.T) {
	t.Setenv("DATABASE_FILE_PATH", "/tmp/test.db")
	t.Setenv("CONFIG_FILE", "/nonexistent/config.yaml")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcde")

	cfg, err := New()
	require.NoError(t, err)
	assert.True(t, cfg.JWTSecretTooShort())
	warnings := cfg.StartupWarnings()
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "jwt_secret")
	assert.Contains(t, warnings[0], "32")
	assert.Contains(t, warnings[0], "openssl rand -hex 32")
	assert.NotContains(t, warnings[0], "0123456789abcdef0123456789abcde", "the warning must not echo the secret")
}

func TestNew_AcceptsJWTSecretOf32Characters(t *testing.T) {
	t.Setenv("DATABASE_FILE_PATH", "/tmp/test.db")
	t.Setenv("CONFIG_FILE", "/nonexistent/config.yaml")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")

	cfg, err := New()
	require.NoError(t, err)
	assert.False(t, cfg.JWTSecretTooShort())
	assert.Empty(t, cfg.StartupWarnings())
}

func TestNew_GenericEnvironmentVariableDoesNotEnableTestMode(t *testing.T) {
	t.Setenv("DATABASE_FILE_PATH", "/tmp/test.db")
	t.Setenv("JWT_SECRET", testJWTSecret)
	t.Setenv("CONFIG_FILE", "/nonexistent/config.yaml")
	t.Setenv("ENVIRONMENT", "test")

	cfg, err := New()
	require.NoError(t, err)
	assert.False(t, cfg.IsTestMode())

	t.Setenv("SHISHO_TEST_MODE", "true")
	cfg, err = New()
	require.NoError(t, err)
	assert.True(t, cfg.IsTestMode())
}

func TestNew_DevLibraryPathOnlyForDevConfigBasename(t *testing.T) {
	t.Setenv("DATABASE_FILE_PATH", "/tmp/test.db")
	t.Setenv("JWT_SECRET", testJWTSecret)

	// A path that merely contains "dev" is a normal deployment.
	t.Setenv("CONFIG_FILE", "/srv/devices/shisho.yaml")
	cfg, err := New()
	require.NoError(t, err)
	assert.Empty(t, cfg.DevLibraryPath)

	// The dev config file enables it wherever it lives.
	devPath := filepath.Join(t.TempDir(), "shisho.dev.yaml")
	require.NoError(t, os.WriteFile(devPath, []byte("jwt_secret: "+testJWTSecret+"\n"), 0644))
	t.Setenv("CONFIG_FILE", devPath)
	cfg, err = New()
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.DevLibraryPath)
}

// A duration without a unit fails to parse before validation runs. It must
// still name the key, the env variable and the expected format.
func TestNew_UnparseableDurationNamesKeyAndFormat(t *testing.T) {
	t.Run("env", func(t *testing.T) {
		t.Setenv("DATABASE_FILE_PATH", "/tmp/test.db")
		t.Setenv("JWT_SECRET", testJWTSecret)
		t.Setenv("CONFIG_FILE", "/nonexistent/config.yaml")
		t.Setenv("DATABASE_BUSY_TIMEOUT", "5000")

		_, err := New()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid config database_busy_timeout (env DATABASE_BUSY_TIMEOUT)")
		assert.Contains(t, err.Error(), `got "5000"`)
		assert.Contains(t, err.Error(), "such as 500ms, 5s or 1m")
	})

	t.Run("yaml", func(t *testing.T) {
		_, err := loadFromYAML(t, "database_file_path: /data/shisho.db\njwt_secret: "+testJWTSecret+"\ndatabase_connect_retry_delay: 2 seconds\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid config database_connect_retry_delay (env DATABASE_CONNECT_RETRY_DELAY)")
		assert.Contains(t, err.Error(), "such as 500ms, 5s or 1m")
	})
}
