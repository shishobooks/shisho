package config

import (
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
)

type Config struct {
	DatabaseConnectRetryCount int
	DatabaseConnectRetryDelay time.Duration
	DatabaseDebug             bool
	DatabaseFilePath          string
	FrontendURL               string
	Hostname                  string
	ServerHost                string
	ServerPort                int
	UserConfig                *UserConfig
	UserConfigFilePath        string
	WorkerProcesses           int
}

type UserConfig struct {
	SyncIntervalMinutes int `json:"sync_interval_minutes"`
}

const environmentENV = "ENVIRONMENT"

func New() (*Config, error) {
	log := logger.New()

	hostname, err := os.Hostname()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	cfg := &Config{
		DatabaseConnectRetryCount: 5,
		DatabaseConnectRetryDelay: 2 * time.Second,
		Hostname:                  hostname,
		ServerPort:                3689,
		UserConfigFilePath:        userConfigFilePath(),
		WorkerProcesses:           2,
	}

	switch os.Getenv(environmentENV) {
	case "development", "":
		loadDevelopmentConfig(cfg)
	case "test":
		loadTestConfig(cfg)
	case "production":
		loadProductionConfig(cfg)
	}

	// Load user config from config.json file.
	userConfig, err := loadUserConfig(cfg.UserConfigFilePath)
	if err != nil {
		// Log error but don't fail startup - use defaults
		// TODO: Add proper logging here when available
		log.Err(err).Warn("couldn't load user config; using defaults", logger.Data{"file_path": cfg.UserConfigFilePath})
		cfg.UserConfig = loadDefaultUserConfig()
	} else {
		cfg.UserConfig = userConfig
		// Save immediately after loading so that we can confirm that we can save it.
		err = saveUserConfigFile(userConfig, cfg.UserConfigFilePath)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return cfg, nil
}
