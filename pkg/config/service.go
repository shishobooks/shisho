package config

type Service struct {
	config *Config
}

func NewService(cfg *Config) *Service {
	return &Service{config: cfg}
}

func (s *Service) RetrieveConfig() *Config {
	return s.config
}
