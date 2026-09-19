package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/internal/config/repository"
	"github.com/jusoaresg/gorgon/internal/config/service"
	"github.com/jusoaresg/gorgon/pkg/schemas"
)

type SafeDB struct {
	Db    *sqlx.DB
	Write *sync.Mutex
}

var (
	InDocker            = false
	BaseDir             = "."
	ConfigFolder string = "./configs"

	LogsPath              string = filepath.Join(ConfigFolder, "logs")
	Port                  string = "8181"
	safeDB                SafeDB
	logger                *slog.Logger
	appConfigService      service.ConfigServiceInterface
	ProwlarrCooldownCache sync.Map
)

func Init() error {
	InDocker = os.Getenv("IN_DOCKER") == "true"

	if dataDirEnv := os.Getenv("GORGON_DATA_DIR"); dataDirEnv != "" {
		ConfigFolder = dataDirEnv
		BaseDir = dataDirEnv
	} else if baseDirEnv := os.Getenv("GORGON_BASE_DIR"); baseDirEnv != "" {
		BaseDir = baseDirEnv
		ConfigFolder = filepath.Join(BaseDir, "configs")
	} else if InDocker {
		ConfigFolder = "/configs"
		BaseDir = "/"
	} else {
		ConfigFolder = "./configs"
		BaseDir = "."
	}

	LogsPath = filepath.Join(ConfigFolder, "logs")

	if portEnv := os.Getenv("GORGON_PORT"); portEnv != "" {
		Port = portEnv
	}

	if err := ReloadFolders(); err != nil {
		return err
	}

	logger = NewLogger()

	dbInstance, err := InitializeDb()
	if err != nil {
		return err
	}
	safeDB = SafeDB{
		Db:    dbInstance,
		Write: &sync.Mutex{},
	}

	// Initialize SQLite Key-Value AppConfig Repository & Service
	appConfigRepo := repository.NewAppConfigRepository(dbInstance, InDocker)
	svc, err := service.NewConfigService(appConfigRepo, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize config service: %w", err)
	}
	appConfigService = svc

	// Migrate legacy config.json if present
	legacyConfigPath := filepath.Join(ConfigFolder, "config.json")
	if err := appConfigService.MigrateLegacyConfigFile(legacyConfigPath); err != nil {
		logger.Warn("Failed to run legacy config migration", slog.String("error", err.Error()))
	}

	return nil
}

func ReloadFolders() error {
	LogsPath = filepath.Join(ConfigFolder, "logs")

	dirs := []string{
		ConfigFolder,
		LogsPath,
		filepath.Join(BaseDir, "downloads"),
	}

	for _, dir := range dirs {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

func GetLogger() *slog.Logger {
	if logger == nil {
		logger = NewLogger()
	}
	return logger
}

func GetSQLite() *sqlx.DB {
	if safeDB.Db == nil {
		panic("database is not initialized")
	}
	return safeDB.Db
}

func GetSafeDB() *SafeDB {
	if safeDB.Db == nil {
		panic("database is not initialized")
	}
	return &safeDB
}

func GetAppConfig() schemas.ConfigFile {
	if appConfigService != nil {
		return appConfigService.Get()
	}
	return repository.DefaultConfigFile(InDocker)
}

func GetConfigService() service.ConfigServiceInterface {
	return appConfigService
}

func SetConfigService(svc service.ConfigServiceInterface) {
	appConfigService = svc
}
