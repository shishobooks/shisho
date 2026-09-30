package config

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/pkg/errors"
)

// Config holds all application configuration.
// Configure via YAML file (/config/shisho.yaml) or environment variables.
// Environment variables use uppercase with underscores (e.g., DATABASE_FILE_PATH).
type Config struct {
	// Database settings

	// DatabaseConnectRetryCount is the total number of startup connection
	// attempts; 0 skips the check.
	DatabaseConnectRetryCount int `koanf:"database_connect_retry_count" json:"database_connect_retry_count" validate:"min=0"`
	// Durations use Go's duration format ("500ms", "5s"). A bare YAML number
	// is nanoseconds, so the 1ms floor rejects it.
	DatabaseConnectRetryDelay time.Duration `koanf:"database_connect_retry_delay" json:"database_connect_retry_delay" validate:"min=1ms"`
	DatabaseDebug             bool          `koanf:"database_debug" json:"database_debug"`
	DatabaseFilePath          string        `koanf:"database_file_path" json:"database_file_path" validate:"required"`
	DatabaseBusyTimeout       time.Duration `koanf:"database_busy_timeout" json:"database_busy_timeout" validate:"min=1ms"`
	DatabaseMaxRetries        int           `koanf:"database_max_retries" json:"database_max_retries" validate:"min=0"`

	// Server settings
	ServerHost string `koanf:"server_host" json:"server_host"`
	ServerPort int    `koanf:"server_port" json:"server_port"`

	// Application settings
	DemoMode            bool `koanf:"demo_mode" json:"demo_mode"`
	SyncIntervalMinutes int  `koanf:"sync_interval_minutes" json:"sync_interval_minutes"`
	WorkerProcesses     int  `koanf:"worker_processes" json:"worker_processes" validate:"min=1"`

	// Job retention settings
	JobRetentionDays int `koanf:"job_retention_days" json:"job_retention_days"`

	// Cache settings
	CacheDir               string `koanf:"cache_dir" json:"cache_dir"`
	DownloadCacheMaxSizeGB int    `koanf:"download_cache_max_size_gb" json:"download_cache_max_size_gb"`

	// PDF viewer rendering settings
	PDFRenderDPI     int `koanf:"pdf_render_dpi" json:"pdf_render_dpi" validate:"min=72,max=600"`
	PDFRenderQuality int `koanf:"pdf_render_quality" json:"pdf_render_quality" validate:"min=1,max=100"`

	// Plugin settings
	PluginDir     string `koanf:"plugin_dir" json:"plugin_dir"`
	PluginDataDir string `koanf:"plugin_data_dir" json:"plugin_data_dir"`

	// Enrichment settings
	EnrichmentConfidenceThreshold float64 `koanf:"enrichment_confidence_threshold" json:"enrichment_confidence_threshold" validate:"min=0,max=1"`

	// Library monitor settings
	LibraryMonitorEnabled      bool `koanf:"library_monitor_enabled" json:"library_monitor_enabled"`
	LibraryMonitorDelaySeconds int  `koanf:"library_monitor_delay_seconds" json:"library_monitor_delay_seconds"`

	// Supplement discovery settings
	// SupplementExcludePatterns only hides files from supplement discovery.
	// Directory cleanup deletes a fixed list instead (see
	// fileutils.DirectoryCleanupPatterns), so a user pattern never deletes
	// files from disk.
	SupplementExcludePatterns []string `koanf:"supplement_exclude_patterns" json:"supplement_exclude_patterns"`
	PDFSupplementFilenames    []string `koanf:"pdf_supplement_filenames" json:"pdf_supplement_filenames"`

	// Authentication settings
	// JWTSecret should be at least MinJWTSecretLength characters. A shorter
	// one only warns (StartupWarnings), so existing installs keep starting;
	// the public example placeholder is rejected.
	JWTSecret           string `koanf:"jwt_secret" json:"-" validate:"required"` // Never expose in JSON
	SessionDurationDays int    `koanf:"session_duration_days" json:"session_duration_days" validate:"min=1"`

	// TestMode mounts the unauthenticated /api/test routes that the E2E suite
	// uses to seed and reset data. It is read from SHISHO_TEST_MODE, a name no
	// other tool sets, so a host that sets a generic variable such as
	// ENVIRONMENT=test for another reason cannot expose those routes. It is
	// left out of the public docs and the example config on purpose.
	TestMode bool `koanf:"shisho_test_mode" json:"test_mode"`

	// DevLibraryPath is the computed path to tmp/library in the main git repo.
	// Used by the frontend to create a default dev library.
	// Computed at startup, not from config file. Only set in development.
	DevLibraryPath string `koanf:"-" json:"dev_library_path,omitempty"`
}

// IsTestMode returns true if the server is running in test mode.
func (c *Config) IsTestMode() bool {
	return c.TestMode
}

// MinLibraryMonitorDelaySeconds is the shortest debounce delay the library
// monitor uses. Lower configured values are raised to it.
const MinLibraryMonitorDelaySeconds = 5

// EffectiveLibraryMonitorDelaySeconds returns the debounce delay the library
// monitor actually uses: the configured value, raised to the minimum.
func (c *Config) EffectiveLibraryMonitorDelaySeconds() int {
	return max(c.LibraryMonitorDelaySeconds, MinLibraryMonitorDelaySeconds)
}

// MinJWTSecretLength is the recommended minimum jwt_secret length.
const MinJWTSecretLength = 32

// JWTSecretTooShort reports whether jwt_secret is shorter than
// MinJWTSecretLength.
func (c *Config) JWTSecretTooShort() bool {
	return len(c.JWTSecret) < MinJWTSecretLength
}

// StartupWarnings returns problems that do not stop the server but that the
// admin should fix. The server logs each one at startup. None includes a
// secret value.
func (c *Config) StartupWarnings() []string {
	var warnings []string
	if c.JWTSecretTooShort() {
		warnings = append(warnings, fmt.Sprintf(
			"jwt_secret (env JWT_SECRET) is shorter than %d characters, so login sessions are easier to forge. "+
				"Replace it with the output of: openssl rand -hex 32 (everyone is signed out once)",
			MinJWTSecretLength,
		))
	}
	return warnings
}

// exampleJWTSecret is the placeholder in shisho.example.yaml. It is public,
// so a server that signs sessions with it can have its sessions forged.
const exampleJWTSecret = "your-secret-key-here-change-me"

// devConfigFilename is the config file `mise start` uses. Loading it turns on
// the development-only DevLibraryPath.
const devConfigFilename = "shisho.dev.yaml"

// defaults returns a Config with default values.
func defaults() *Config {
	return &Config{
		DatabaseConnectRetryCount:     5,
		DatabaseConnectRetryDelay:     2 * time.Second,
		DatabaseDebug:                 false,
		DatabaseFilePath:              "/config/shisho.db",
		DatabaseBusyTimeout:           5 * time.Second,
		DatabaseMaxRetries:            5,
		ServerHost:                    "0.0.0.0",
		ServerPort:                    3689,
		DemoMode:                      false,
		SyncIntervalMinutes:           60,
		WorkerProcesses:               2,
		JobRetentionDays:              30,
		CacheDir:                      "/config/cache",
		PluginDir:                     "/config/plugins/installed",
		PluginDataDir:                 "/config/plugins/data",
		EnrichmentConfidenceThreshold: 0.85,
		DownloadCacheMaxSizeGB:        5,
		PDFRenderDPI:                  200,
		PDFRenderQuality:              85,
		LibraryMonitorEnabled:         true,
		LibraryMonitorDelaySeconds:    60,
		SupplementExcludePatterns:     []string{".*", ".DS_Store", "Thumbs.db", "desktop.ini"},
		PDFSupplementFilenames: []string{
			"supplement", "supplemental", "bonus", "bonus material", "bonus content",
			"companion", "notes", "liner notes", "errata", "booklet", "digital booklet",
			"appendix", "map", "maps", "insert", "guide", "reference",
			"cheat sheet", "cheatsheet", "cribsheet", "pamphlet", "extras",
		},
		SessionDurationDays: 30,
		JWTSecret:           "", // Must be set via config or env var
	}
}

// New creates a new Config by loading from file and environment variables.
// Load order (later sources override earlier):
//  1. Defaults
//  2. Config file (/config/shisho.yaml or CONFIG_FILE env var)
//  3. Environment variables (unprefixed, e.g. DATABASE_FILE_PATH)
func New() (*Config, error) {
	k := koanf.New(".")

	// 1. Load defaults
	cfg := defaults()

	// 2. Load from config file (if exists)
	configPath := os.Getenv("CONFIG_FILE")
	if configPath == "" {
		configPath = "/config/shisho.yaml"
	}
	if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
		// File not existing is fine - we'll use defaults and env vars
		if !os.IsNotExist(err) {
			return nil, errors.Wrapf(err, "failed to load config file %s", configPath)
		}
	}

	// 3. Load environment variables (DATABASE_FILE_PATH -> database_file_path).
	// List keys split on commas, since an environment variable holds one
	// string; YAML lists are unaffected.
	listKeys := stringListKeys()
	err := k.Load(env.ProviderWithValue("", ".", func(key, value string) (string, any) {
		key = strings.ToLower(key)
		if _, ok := listKeys[key]; ok {
			return key, splitList(value)
		}
		return key, value
	}), nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load environment variables")
	}

	// Unmarshal cannot parse a duration string without a unit ("5000" from
	// the environment, "2 seconds" in YAML) and its error names neither the
	// env variable nor the format, so check those strings first.
	if msgs := durationStringErrors(k); len(msgs) > 0 {
		return nil, errors.New("configuration validation failed:\n\n" + strings.Join(msgs, "\n\n"))
	}

	// Unmarshal into config struct
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}

	// Compute dev library path only in development (when using shisho.dev.yaml)
	if filepath.Base(configPath) == devConfigFilename {
		cfg.DevLibraryPath = computeDevLibraryPath()
	}

	// Validate required fields
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// NewForTest creates a Config for testing with minimal required fields.
func NewForTest() *Config {
	cfg := defaults()
	cfg.DatabaseFilePath = ":memory:"
	cfg.DatabaseBusyTimeout = 1 * time.Second // Shorter timeout for tests
	cfg.DatabaseMaxRetries = 3                // Fewer retries for tests
	cfg.ServerHost = "127.0.0.1"
	cfg.ServerPort = 0
	cfg.WorkerProcesses = 1
	cfg.LibraryMonitorEnabled = false
	cfg.CacheDir = "" // Must be set by test
	cfg.DownloadCacheMaxSizeGB = 1
	cfg.SupplementExcludePatterns = []string{".*", ".DS_Store", "Thumbs.db", "desktop.ini"}
	cfg.PDFSupplementFilenames = []string{
		"supplement", "supplemental", "bonus", "bonus material", "bonus content",
		"companion", "notes", "liner notes", "errata", "booklet", "digital booklet",
		"appendix", "map", "maps", "insert", "guide", "reference",
		"cheat sheet", "cheatsheet", "cribsheet", "pamphlet", "extras",
	}
	cfg.JWTSecret = "test-secret-key-for-testing-only"
	return cfg
}

// SessionDuration returns the session duration as a time.Duration.
func (c *Config) SessionDuration() time.Duration {
	return time.Duration(c.SessionDurationDays) * 24 * time.Hour
}

// DownloadCacheMaxSizeBytes returns the maximum cache size in bytes.
func (c *Config) DownloadCacheMaxSizeBytes() int64 {
	return int64(c.DownloadCacheMaxSizeGB) * 1024 * 1024 * 1024
}

// stringListKeys returns the config keys of every []string field.
func stringListKeys() map[string]struct{} {
	keys := make(map[string]struct{})
	t := reflect.TypeOf(Config{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type == reflect.TypeOf([]string{}) {
			keys[f.Tag.Get("koanf")] = struct{}{}
		}
	}
	return keys
}

// durationFormatHint explains the duration format in validation errors.
const durationFormatHint = "Durations use Go's format, such as 500ms, 5s or 1m"

// durationStringErrors describes every duration setting whose value is a
// string that time.ParseDuration rejects.
func durationStringErrors(k *koanf.Koanf) []string {
	var msgs []string
	t := reflect.TypeOf(Config{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type != reflect.TypeOf(time.Duration(0)) {
			continue
		}
		key := f.Tag.Get("koanf")
		value, ok := k.Get(key).(string)
		if !ok {
			continue
		}
		if _, err := time.ParseDuration(value); err != nil {
			msgs = append(msgs, fmt.Sprintf("invalid config %s (env %s): must be a duration with a unit, got %q. %s",
				key, strings.ToUpper(key), value, durationFormatHint))
		}
	}
	return msgs
}

// splitList splits a comma-separated environment value, trimming spaces and
// dropping empty items, so an empty variable is an empty list.
func splitList(value string) []string {
	items := []string{}
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

// validateConfig validates the config and returns user-friendly error
// messages that name each config key, its environment variable, and the
// allowed range.
func validateConfig(cfg *Config) error {
	validate := validator.New()
	validate.RegisterTagNameFunc(func(f reflect.StructField) string {
		return f.Tag.Get("koanf")
	})

	var msgs []string
	if cfg.JWTSecret == exampleJWTSecret {
		msgs = append(msgs, "invalid config jwt_secret (env JWT_SECRET): the value from shisho.example.yaml is public. "+
			"Generate a private one with: openssl rand -hex 32 (everyone is signed out once)")
	}

	err := validate.Struct(cfg)
	if err != nil {
		validationErrors, ok := err.(validator.ValidationErrors)
		if !ok {
			return errors.Wrap(err, "config validation failed")
		}
		for _, e := range validationErrors {
			msgs = append(msgs, validationMessage(e))
		}
	}

	if len(msgs) == 0 {
		return nil
	}
	return errors.New("configuration validation failed:\n\n" + strings.Join(msgs, "\n\n"))
}

// validationMessage describes one failed rule in terms of the config key.
func validationMessage(e validator.FieldError) string {
	key := e.Field()
	envVar := strings.ToUpper(key)

	if e.Tag() == "required" {
		return fmt.Sprintf(
			"missing required config: %s\n  Set via environment variable: %s\n  Or in config file: %s",
			key, envVar, key,
		)
	}

	field, _ := reflect.TypeOf(Config{}).FieldByName(e.StructField())
	isSecret := field.Tag.Get("json") == "-"
	isDuration := field.Type == reflect.TypeOf(time.Duration(0))
	isString := field.Type.Kind() == reflect.String

	var rule string
	lo, hi := ruleParams(field.Tag.Get("validate"))
	switch {
	case isString && e.Tag() == "min":
		rule = fmt.Sprintf("must be at least %s characters", e.Param())
	case lo != "" && hi != "":
		rule = fmt.Sprintf("must be between %s and %s", lo, hi)
	case e.Tag() == "min":
		rule = "must be at least " + e.Param()
	case e.Tag() == "max":
		rule = "must be at most " + e.Param()
	default:
		rule = "failed the " + e.Tag() + " rule"
	}

	msg := fmt.Sprintf("invalid config %s (env %s): %s", key, envVar, rule)
	if !isSecret {
		msg += fmt.Sprintf(", got %v", e.Value())
	}
	if isDuration {
		msg += ". " + durationFormatHint + "; a bare number in YAML is nanoseconds"
	}
	return msg
}

// ruleParams returns the min and max parameters of a validate tag.
func ruleParams(tag string) (lo, hi string) {
	for _, rule := range strings.Split(tag, ",") {
		if v, ok := strings.CutPrefix(rule, "min="); ok {
			lo = v
		}
		if v, ok := strings.CutPrefix(rule, "max="); ok {
			hi = v
		}
	}
	return lo, hi
}

// computeDevLibraryPath computes the path to tmp/library in the main git repo.
// This handles git worktrees by finding the main worktree location.
// Returns empty string if not in a git repository or on error.
func computeDevLibraryPath() string {
	// Get the git toplevel directory
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	gitRoot := strings.TrimSpace(string(output))
	if gitRoot == "" {
		return ""
	}

	// Check if we're in a worktree by looking at .git
	gitPath := filepath.Join(gitRoot, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}

	var mainRepoRoot string
	if info.IsDir() {
		// Regular git repo, .git is a directory
		mainRepoRoot = gitRoot
	} else {
		// Worktree, .git is a file containing "gitdir: /path/to/main/.git/worktrees/<name>"
		content, err := os.ReadFile(gitPath)
		if err != nil {
			return ""
		}
		// Parse "gitdir: /path/to/main/.git/worktrees/<name>"
		gitdirLine := strings.TrimSpace(string(content))
		if !strings.HasPrefix(gitdirLine, "gitdir: ") {
			return ""
		}
		worktreeGitDir := strings.TrimPrefix(gitdirLine, "gitdir: ")
		// Go from /path/to/main/.git/worktrees/<name> to /path/to/main
		// The worktree gitdir is always <main>/.git/worktrees/<name>
		mainRepoRoot = filepath.Dir(filepath.Dir(filepath.Dir(worktreeGitDir)))
	}

	return filepath.Join(mainRepoRoot, "tmp", "library")
}
