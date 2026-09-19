package config

import (
	"github.com/jusoaresg/gorgon/internal/config/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas"
)

// LoadConfig returns the current application configuration.
// It is backed by the thread-safe in-memory SQLite ConfigService.
func LoadConfig() (*schemas.ConfigFile, error) {
	if appConfigService != nil {
		cfg := appConfigService.Get()
		return &cfg, nil
	}
	cfg := repository.DefaultConfigFile(InDocker)
	return &cfg, nil
}

// SaveConfig saves the application configuration to the SQLite database.
func SaveConfig(config *schemas.ConfigFile) error {
	if appConfigService != nil {
		return appConfigService.Save(config)
	}
	return nil
}
