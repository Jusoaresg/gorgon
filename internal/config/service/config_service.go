package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/jusoaresg/gorgon/internal/config/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas"
)

type ConfigServiceInterface interface {
	Get() schemas.ConfigFile
	Update(input *schemas.UpdateConfigInput) (*schemas.ConfigFile, error)
	Save(cfg *schemas.ConfigFile) error
	MigrateLegacyConfigFile(legacyPath string) error
	Reload() error
}

type ConfigService struct {
	repo   repository.AppConfigRepositoryInterface
	logger *slog.Logger
	mu     sync.RWMutex
	cached schemas.ConfigFile
}

func NewConfigService(repo repository.AppConfigRepositoryInterface, logger *slog.Logger) (*ConfigService, error) {
	cfg, err := repo.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load initial app config from repository: %w", err)
	}

	return &ConfigService{
		repo:   repo,
		logger: logger,
		cached: *cfg,
	}, nil
}

func (s *ConfigService) Get() schemas.ConfigFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cached
}

func (s *ConfigService) Update(input *schemas.UpdateConfigInput) (*schemas.ConfigFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	updated := s.cached
	updated.Apply(input)

	if err := s.repo.SaveConfig(&updated); err != nil {
		return nil, fmt.Errorf("failed to save updated config to repository: %w", err)
	}

	s.cached = updated
	if s.logger != nil {
		s.logger.Info("App config updated successfully in SQLite app_settings table")
	}

	return &s.cached, nil
}

func (s *ConfigService) Save(cfg *schemas.ConfigFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.repo.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config to repository: %w", err)
	}

	s.cached = *cfg
	return nil
}

func (s *ConfigService) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.repo.GetConfig()
	if err != nil {
		return err
	}
	s.cached = *cfg
	return nil
}

func (s *ConfigService) MigrateLegacyConfigFile(legacyPath string) error {
	if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
		return nil
	}

	if s.logger != nil {
		s.logger.Info("Legacy config file found, checking migration to SQLite database", slog.String("path", legacyPath))
	}

	file, err := os.Open(legacyPath)
	if err != nil {
		return fmt.Errorf("failed to open legacy config file: %w", err)
	}
	defer file.Close()

	var legacyConfig schemas.ConfigFile
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&legacyConfig); err != nil {
		return fmt.Errorf("failed to decode legacy config.json: %w", err)
	}
	file.Close()

	s.mu.Lock()
	if legacyConfig.ProwlarrApiKey != "" {
		s.cached.ProwlarrApiKey = legacyConfig.ProwlarrApiKey
	}
	if legacyConfig.ProwlarrHost != "" {
		s.cached.ProwlarrHost = legacyConfig.ProwlarrHost
	}
	if legacyConfig.ProwlarrPort != "" {
		s.cached.ProwlarrPort = legacyConfig.ProwlarrPort
	}
	if legacyConfig.QBittorrentHost != "" {
		s.cached.QBittorrentHost = legacyConfig.QBittorrentHost
	}
	if legacyConfig.QBittorrentPort != "" {
		s.cached.QBittorrentPort = legacyConfig.QBittorrentPort
	}
	if legacyConfig.QBittorrentUsername != "" {
		s.cached.QBittorrentUsername = legacyConfig.QBittorrentUsername
	}
	if legacyConfig.QBittorrentPassword != "" {
		s.cached.QBittorrentPassword = legacyConfig.QBittorrentPassword
	}
	if legacyConfig.QBittorrentDownloadFolder != "" {
		s.cached.QBittorrentDownloadFolder = legacyConfig.QBittorrentDownloadFolder
	}
	if legacyConfig.ShowsFolder != "" {
		s.cached.ShowsFolder = legacyConfig.ShowsFolder
	}
	if legacyConfig.DefaultShowInfoFolder != "" {
		s.cached.DefaultShowInfoFolder = legacyConfig.DefaultShowInfoFolder
	}
	if legacyConfig.TelegramBotApiKey != "" {
		s.cached.TelegramBotApiKey = legacyConfig.TelegramBotApiKey
	}
	if legacyConfig.TelegramChatID != "" {
		s.cached.TelegramChatID = legacyConfig.TelegramChatID
	}
	if legacyConfig.TelegramDailySummaryTime != "" {
		s.cached.TelegramDailySummaryTime = legacyConfig.TelegramDailySummaryTime
	}
	s.cached.TelegramDailySummaryEnabled = legacyConfig.TelegramDailySummaryEnabled
	s.cached.TelegramNotifyEmptySummary = legacyConfig.TelegramNotifyEmptySummary

	if err := s.repo.SaveConfig(&s.cached); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to save migrated config to database: %w", err)
	}
	s.mu.Unlock()

	if s.logger != nil {
		s.logger.Info("Successfully migrated legacy config.json to SQLite database")
	}

	bakPath := legacyPath + ".bak"
	if err := os.Rename(legacyPath, bakPath); err != nil {
		if s.logger != nil {
			s.logger.Warn("Could not rename legacy config.json to .bak", slog.String("error", err.Error()))
		}
	} else if s.logger != nil {
		s.logger.Info("Renamed legacy config.json to .bak", slog.String("bakPath", bakPath))
	}

	return nil
}

var _ ConfigServiceInterface = (*ConfigService)(nil)
