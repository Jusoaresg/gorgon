package config

import (
	"testing"
	"time"

	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/stretchr/testify/assert"
)

type mockConfigService struct {
	cfg schemas.ConfigFile
}

func (m *mockConfigService) Get() schemas.ConfigFile {
	return m.cfg
}

func (m *mockConfigService) Update(input *schemas.UpdateConfigInput) (*schemas.ConfigFile, error) {
	m.cfg.Apply(input)
	return &m.cfg, nil
}

func (m *mockConfigService) Save(cfg *schemas.ConfigFile) error {
	m.cfg = *cfg
	return nil
}

func (m *mockConfigService) MigrateLegacyConfigFile(legacyPath string) error {
	return nil
}

func (m *mockConfigService) Reload() error {
	return nil
}

func TestGetAppLocation(t *testing.T) {
	oldService := GetConfigService()
	defer SetConfigService(oldService)

	t.Run("defaults to time.Local when timezone is empty", func(t *testing.T) {
		SetConfigService(&mockConfigService{
			cfg: schemas.ConfigFile{Timezone: ""},
		})

		loc := GetAppLocation()
		assert.Equal(t, time.Local, loc)
	})

	t.Run("loads valid IANA timezone", func(t *testing.T) {
		SetConfigService(&mockConfigService{
			cfg: schemas.ConfigFile{Timezone: "America/Sao_Paulo"},
		})

		loc := GetAppLocation()
		assert.Equal(t, "America/Sao_Paulo", loc.String())
		assert.Equal(t, "America/Sao_Paulo", GetAppTimezoneName())
	})

	t.Run("loads UTC timezone", func(t *testing.T) {
		SetConfigService(&mockConfigService{
			cfg: schemas.ConfigFile{Timezone: "UTC"},
		})

		loc := GetAppLocation()
		assert.Equal(t, time.UTC, loc)
	})

	t.Run("falls back to time.Local on invalid timezone", func(t *testing.T) {
		SetConfigService(&mockConfigService{
			cfg: schemas.ConfigFile{Timezone: "Invalid/Not_A_Real_Timezone"},
		})

		loc := GetAppLocation()
		assert.Equal(t, time.Local, loc)
	})
}
