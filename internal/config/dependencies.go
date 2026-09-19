package config

import (
	"log/slog"

	"github.com/jusoaresg/gorgon/internal/config/service"
)

type Dependencies struct {
	Logger  *slog.Logger
	Service service.ConfigServiceInterface
}

func NewDependencies(svc service.ConfigServiceInterface, logger *slog.Logger) *Dependencies {
	return &Dependencies{
		Logger:  logger,
		Service: svc,
	}
}
