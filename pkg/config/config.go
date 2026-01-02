package config

import (
	"fmt"
	"os"
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
	DatabaseConnectRetryCount int           `koanf:"database_connect_retry_count" json:"database_connect_retry_count"`
	DatabaseConnectRetryDelay time.Duration `koanf:"database_connect_retry_delay" json:"database_connect_retry_delay"`
	DatabaseDebug             bool          `koanf:"database_debug" json:"database_debug"`
	DatabaseFilePath          string        `koanf:"database_file_path" json:"database_file_path" validate:"required"`

	// Server settings
	ServerHost string `koanf:"server_host" json:"server_host"`
	ServerPort int    `koanf:"server_port" json:"server_port"`

	// Application settings
	SyncIntervalMinutes int `koanf:"sync_interval_minutes" json:"sync_interval_minutes"`
	WorkerProcesses     int `koanf:"worker_processes" json:"worker_processes"`

	// Internal settings (computed, not from config file)
	Hostname string `koanf:"-" json:"-"`
}

// defaults returns a Config with default values.
func defaults() *Config {
	return &Config{
		DatabaseConnectRetryCount: 5,
		DatabaseConnectRetryDelay: 2 * time.Second,
		DatabaseDebug:             false,
		ServerHost:                "0.0.0.0",
		ServerPort:                3689,
		SyncIntervalMinutes:       60,
		WorkerProcesses:           2,
	}
}

// New creates a new Config by loading from file and environment variables.
// Load order (later sources override earlier):
//  1. Defaults
//  2. Config file (/config/shisho.yaml or CONFIG_FILE env var)
//  3. Environment variables (prefixed with SHISHO_)
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

	// 3. Load environment variables (DATABASE_FILE_PATH -> database_file_path)
	err := k.Load(env.Provider("", ".", strings.ToLower), nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load environment variables")
	}

	// Unmarshal into config struct
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get hostname")
	}
	cfg.Hostname = hostname

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
	cfg.DatabaseDebug = true
	cfg.ServerHost = "127.0.0.1"
	cfg.ServerPort = 0
	cfg.Hostname = "test-host"
	cfg.WorkerProcesses = 1
	return cfg
}

// validateConfig validates the config and returns user-friendly error messages.
func validateConfig(cfg *Config) error {
	validate := validator.New()
	err := validate.Struct(cfg)
	if err == nil {
		return nil
	}

	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return errors.Wrap(err, "config validation failed")
	}

	var msgs []string
	for _, e := range validationErrors {
		field := e.StructField()
		tag := e.Tag()

		switch tag {
		case "required":
			envVar := strings.ToUpper(toSnakeCase(field))
			yamlKey := toSnakeCase(field)
			msgs = append(msgs, fmt.Sprintf(
				"missing required config: %s\n  Set via environment variable: %s\n  Or in config file: %s",
				field, envVar, yamlKey,
			))
		default:
			msgs = append(msgs, fmt.Sprintf("invalid config %s: %s", field, tag))
		}
	}

	return errors.New("configuration validation failed:\n\n" + strings.Join(msgs, "\n\n"))
}

// toSnakeCase converts PascalCase to snake_case.
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}
