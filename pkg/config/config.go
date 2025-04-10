package config

import (
	"os"
	"time"

	"github.com/pkg/errors"
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
	WorkerProcesses           int
}

const environmentENV = "ENVIRONMENT"

func New() (*Config, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	cfg := &Config{
		DatabaseConnectRetryCount: 5,
		DatabaseConnectRetryDelay: 2 * time.Second,
		Hostname:                  hostname,
		ServerPort:                3689,
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

	return cfg, nil
}
