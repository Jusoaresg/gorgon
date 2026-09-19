package api

import (
	"log/slog"

	"github.com/jusoaresg/gorgon/internal/config"
	"github.com/jusoaresg/gorgon/internal/config/service"
)

type Handler struct {
	Logger  *slog.Logger
	Service service.ConfigServiceInterface
}

func NewHandler(deps *config.Dependencies) *Handler {
	return &Handler{
		Logger:  deps.Logger,
		Service: deps.Service,
	}
}
