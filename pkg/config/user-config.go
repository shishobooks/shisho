package config

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

func userConfigFilePath() string {
	configDir := os.Getenv("CONFIG_DIRECTORY")
	if configDir == "" {
		configDir = "/config"
	}

	return filepath.Join(configDir, "config.json")
}

func loadUserConfig(configFilePath string) (*UserConfig, error) {
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// File doesn't exist, return defaults
			return loadDefaultUserConfig(), nil
		}
		return nil, errors.WithStack(err)
	}

	userConfig := loadDefaultUserConfig()
	if err := json.Unmarshal(data, userConfig); err != nil {
		return nil, errors.WithStack(err)
	}

	return userConfig, nil
}

func loadDefaultUserConfig() *UserConfig {
	return &UserConfig{
		SyncIntervalMinutes: 60, // 1 hour
	}
}

func saveUserConfigFile(userConfig *UserConfig, userConfigFilePath string) error {
	// Ensure config directory exists.
	if err := os.MkdirAll(filepath.Dir(userConfigFilePath), 0755); err != nil {
		return errors.WithStack(err)
	}

	// Write updated settings to file.
	data, err := json.MarshalIndent(userConfig, "", "  ")
	if err != nil {
		return errors.WithStack(err)
	}

	err = os.WriteFile(userConfigFilePath, data, 0644) //nolint:gosec
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
