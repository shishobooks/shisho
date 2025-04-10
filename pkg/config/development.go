package config

import (
	"os"
	"strconv"
)

func loadDevelopmentConfig(cfg *Config) {
	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err == nil {
		cfg.ServerPort = port
	}

	cfg.DatabaseDebug = true
	cfg.DatabaseFilePath = "./tmp/data.sqlite"
	cfg.FrontendURL = "http://localhost:6060"
	cfg.ServerHost = "127.0.0.1"
}
