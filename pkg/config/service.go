package config

import (
	"github.com/pkg/errors"
)

type Service struct {
	config *Config
}

type UpdateUserConfigOptions struct {
	UpdateFile bool
}

func NewService(cfg *Config) *Service {
	return &Service{config: cfg}
}

func (s *Service) RetrieveUserConfig() (*UserConfig, error) {
	return s.config.UserConfig, nil
}

func (s *Service) UpdateUserConfig(userConfig *UserConfig, opts UpdateUserConfigOptions) error {
	if !opts.UpdateFile {
		// No updates.
		return nil
	}

	err := saveUserConfigFile(userConfig, s.config.UserConfigFilePath)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
