package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/jusoaresg/gorgon/pkg/schemas"
)

var configPath string

func InitializeOrUpdateConfigFile() error {
	configPath = filepath.Join(ConfigFolder, "config.json")

	var updated bool

	config := schemas.ConfigFile{
		ProwlarrApiKey:            "",
		ProwlarrHost:              "",
		ProwlarrPort:              "",
		QBittorrentHost:           "",
		QBittorrentPort:           "",
		QBittorrentUsername:       "",
		QBittorrentPassword:       "",
		QBittorrentDownloadFolder: "downloads",
		DefaultShowInfoFolder:     "shows",
		ShowsFolder:               "/home/user/Videos/shows",
		TelegramBotApiKey:         "",
		TelegramChatID:            "",
		TelegramDailySummaryEnabled: true,
		TelegramDailySummaryTime:    "08:00",
		TelegramNotifyEmptySummary:  true,
	}

	if InDocker {
		config.QBittorrentDownloadFolder = "/downloads"
		config.ShowsFolder = "/shows"
		config.DefaultShowInfoFolder = "/app/assets/shows"

		config.ProwlarrHost = "http://gorgon-prowlarr"
		config.ProwlarrPort = "9696"

		config.QBittorrentHost = "http://gorgon-qbittorrent"
		config.QBittorrentPort = "9191"
		config.QBittorrentUsername = "admin"
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("Creating new config file...")
		return saveConfig(config)
	}

	existingConfig, err := LoadConfig()
	if err != nil {
		return err
	}

	//Fill in the missing fields
	existingVal := reflect.ValueOf(existingConfig).Elem()
	defaultVal := reflect.ValueOf(config)
	for i := 0; i < existingVal.NumField(); i++ {
		field := existingVal.Field(i)
		if field.Kind() == reflect.String && field.String() == "" {
			field.SetString(defaultVal.Field(i).String())
			updated = true
		}
	}

	if updated {
		fmt.Println("Updating config file with missing fields...")
		return saveConfig(*existingConfig)
	}

	fmt.Println("Config file already up to date.")
	return nil
}

func saveConfig(config schemas.ConfigFile) error {
	file, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(config)
}

func LoadConfig() (*schemas.ConfigFile, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var configFile schemas.ConfigFile
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&configFile)
	if err != nil {
		return nil, err
	}

	return &configFile, nil
}

func SaveConfig(config *schemas.ConfigFile) error {
	return saveConfig(*config)
}
